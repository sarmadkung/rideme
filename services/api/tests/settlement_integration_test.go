//go:build integration

package tests

import (
	"context"
	"io"
	"log/slog"
	"sync"
	"testing"
	"time"

	"github.com/jackc/pgx/v5/pgxpool"
	"github.com/sarmadkung/rideme/services/api/internal/booking"
	"github.com/sarmadkung/rideme/services/api/internal/finance"
	"github.com/sarmadkung/rideme/services/api/internal/jobs"
	"github.com/sarmadkung/rideme/services/api/internal/merchant"
	"github.com/sarmadkung/rideme/services/api/internal/pricing"
	"github.com/sarmadkung/rideme/services/api/internal/settlement"
	"github.com/sarmadkung/rideme/services/api/pkg/money"
)

// Settlement is Level 5 by `verification-lite`'s rule and its unit tests run
// against fakes, which is right for the arithmetic and useless for everything
// that makes the arithmetic hold: the unique indexes, the immutability
// trigger, the conditional capture, and the commission rate that lives in a
// migration rather than in Go.
//
// These are the tests for that half.

type settlementHarness struct {
	pool     *pgxpool.Pool
	ledger   *finance.Store
	prices   *booking.Store
	orders   *merchant.Store
	jobs     *jobs.Store
	settling *settlement.Service
}

func newSettlementHarness(t *testing.T) *settlementHarness {
	t.Helper()
	pool, err := pgxpool.New(context.Background(), env(t, "DATABASE_URL",
		"postgres://logistics:logistics@localhost:55432/logistics_dev?sslmode=disable"))
	if err != nil {
		t.Fatalf("connect: %v", err)
	}
	t.Cleanup(pool.Close)

	h := &settlementHarness{
		pool: pool, ledger: finance.NewStore(pool), prices: booking.NewStore(pool),
		orders: merchant.NewStore(pool), jobs: jobs.NewStore(pool),
	}
	h.settling = settlement.NewService(h.ledger, h.prices,
		slog.New(slog.NewTextHandler(io.Discard, nil))).WithGoods(h.orders)
	return h
}

func (h *settlementHarness) aUser(t *testing.T) string {
	t.Helper()
	var id string
	if err := h.pool.QueryRow(context.Background(),
		`INSERT INTO users (phone) VALUES ('+9242' || lpad((floor(random()*100000000))::text, 8, '0'))
		 RETURNING id::text`).Scan(&id); err != nil {
		t.Fatal(err)
	}
	return id
}

func (h *settlementHarness) aDriver(t *testing.T) string {
	t.Helper()
	var id string
	if err := h.pool.QueryRow(context.Background(),
		`INSERT INTO drivers (user_id) VALUES ($1) RETURNING id::text`, h.aUser(t)).Scan(&id); err != nil {
		t.Fatal(err)
	}
	return id
}

// aCompletedJob builds a finished job with a driver and a locked price — the
// exact state `booking.Execute` hands to settlement.
func (h *settlementHarness) aCompletedJob(t *testing.T, jobType jobs.Type, customerID, driverID string, fareMinor int64) jobs.Job {
	t.Helper()
	ctx := context.Background()

	job, err := h.jobs.Create(ctx, jobs.Job{
		Type: jobType, RequesterUserID: customerID, Status: jobs.StatusCompleted,
		Stops: []jobs.Stop{
			{Sequence: 0, Type: jobs.StopPickup, Location: jobs.Coordinate{Latitude: 31.52, Longitude: 74.35}},
			{Sequence: 1, Type: jobs.StopDropoff, Location: jobs.Coordinate{Latitude: 31.55, Longitude: 74.33}},
		},
	}, jobs.Actor{Type: jobs.ActorCustomer})
	if err != nil {
		t.Fatal(err)
	}
	if _, err := h.pool.Exec(ctx,
		`UPDATE jobs SET assigned_driver_id = $2 WHERE id = $1`, job.ID, driverID); err != nil {
		t.Fatal(err)
	}
	job.AssignedDriverID = driverID

	if fareMinor > 0 {
		total := money.MustNew(fareMinor, money.PKR)
		quoteID, err := h.prices.SaveQuote(ctx, customerID, pricing.Quote{
			JobType: string(jobType), Currency: money.PKR, Total: total,
			PricingVersion: 1, ExpiresAt: time.Now().Add(time.Hour),
		})
		if err != nil {
			t.Fatal(err)
		}
		if err := h.prices.LockPrice(ctx, job.ID, booking.StoredQuote{
			ID: quoteID, RequestedBy: customerID, Total: total, Version: 1,
			Snapshot: []byte(`{}`),
		}); err != nil {
			t.Fatal(err)
		}
	}
	return job
}

// balance sums one account for one subject, signed as the ledger stores it.
func (h *settlementHarness) balance(t *testing.T, account finance.Account, subjectID string) int64 {
	t.Helper()
	var sum int64
	if err := h.pool.QueryRow(context.Background(),
		`SELECT COALESCE(sum(amount_minor), 0) FROM ledger_entries
		  WHERE account = $1 AND subject_id = $2`, account, subjectID).Scan(&sum); err != nil {
		t.Fatal(err)
	}
	return sum
}

// jobEntries counts every entry produced by one job, across every transaction.
func (h *settlementHarness) jobEntries(t *testing.T, jobID string) int {
	t.Helper()
	var count int
	if err := h.pool.QueryRow(context.Background(),
		`SELECT count(*) FROM ledger_entries e
		   JOIN ledger_transactions x ON x.id = e.transaction_id
		  WHERE x.job_id = $1`, jobID).Scan(&count); err != nil {
		t.Fatal(err)
	}
	return count
}

// --- tests -------------------------------------------------------------------

// The defect this whole subsystem existed to fix: every piece of
// `internal/finance` was built and never called, so a completed trip charged
// nobody and `GET /driver/earnings` returned zero for every driver because the
// book it reads had no writer.
//
// This asserts the whole path against real Postgres, ending at the earnings
// query the driver's phone actually calls.
func TestACompletedRideReachesTheDriversEarnings(t *testing.T) {
	h := newSettlementHarness(t)
	ctx := context.Background()

	customerID, driverID := h.aUser(t), h.aDriver(t)
	job := h.aCompletedJob(t, jobs.TypeRide, customerID, driverID, 50000) // Rs 500
	before := time.Now().Add(-time.Minute)

	breakdown, err := h.settling.SettleCash(ctx, job)
	if err != nil {
		t.Fatalf("settle: %v", err)
	}

	// BD-05 as revised on 2026-09-14: 10%. The rate comes from migration
	// 000014, so this also proves the migration is applied and that version 2
	// wins over the version 1 row it closed.
	if breakdown.Commission.Minor != 5000 {
		t.Errorf("commission %d, want 5000 — is migration 000014 applied?", breakdown.Commission.Minor)
	}
	if breakdown.DriverNet.Minor != 45000 {
		t.Errorf("driver net %d, want 45000", breakdown.DriverNet.Minor)
	}

	// With cash the driver holds the fare and is owed the net, so the books
	// show them owing the platform its commission. That is the opposite
	// direction from a card payment and it is the correct one.
	if got := h.balance(t, finance.AccountCashInTransit, driverID); got != 50000 {
		t.Errorf("cash in transit %d, want 50000", got)
	}
	if got := h.balance(t, finance.AccountDriverPayable, driverID); got != -45000 {
		t.Errorf("driver payable %d, want -45000", got)
	}
	// Created and discharged in the same settlement: nobody owes the customer.
	if got := h.balance(t, finance.AccountCustomerReceivable, customerID); got != 0 {
		t.Errorf("customer receivable %d, want 0", got)
	}

	balanced, offBy, err := h.ledger.LedgerBalances(ctx)
	if err != nil {
		t.Fatal(err)
	}
	if !balanced {
		t.Errorf("the ledger does not balance, off by %d", offBy)
	}

	// The end of the chain, and the point of all of it.
	earnings, err := h.ledger.EarningsBetween(ctx, driverID, before, time.Now().Add(time.Minute))
	if err != nil {
		t.Fatal(err)
	}
	if earnings.Net.Minor != 45000 {
		t.Errorf("GET /driver/earnings would report %d, want 45000", earnings.Net.Minor)
	}
	if earnings.Trips != 1 {
		t.Errorf("trips %d, want 1", earnings.Trips)
	}
}

// The intent is CASH and it is captured. Document 052's states are what a
// support agent reads when a customer disputes a fare, and an intent stuck at
// REQUIRES_PAYMENT after a completed trip would describe a trip nobody paid
// for.
func TestTheIntentRecordsThatCashWasTaken(t *testing.T) {
	h := newSettlementHarness(t)
	ctx := context.Background()

	customerID, driverID := h.aUser(t), h.aDriver(t)
	job := h.aCompletedJob(t, jobs.TypeRide, customerID, driverID, 30000)
	if _, err := h.settling.SettleCash(ctx, job); err != nil {
		t.Fatal(err)
	}

	var method, status string
	var captured int64
	if err := h.pool.QueryRow(ctx,
		`SELECT method, status, captured_minor FROM payment_intents WHERE job_id = $1`,
		job.ID).Scan(&method, &status, &captured); err != nil {
		t.Fatalf("no intent was created for a completed job: %v", err)
	}
	if method != "CASH" {
		t.Errorf("method %q, want CASH", method)
	}
	if status != "CAPTURED" {
		t.Errorf("status %q, want CAPTURED", status)
	}
	if captured != 30000 {
		t.Errorf("captured %d, want 30000", captured)
	}
}

// Document 054: earnings "cannot be duplicated by repeated completion events".
// A driver tapping "complete" twice on a bad connection is the ordinary case,
// and the guarantee is a unique index rather than a check in Go — so it is
// only really tested here.
func TestSettlingTwiceMovesNoMoneyTwice(t *testing.T) {
	h := newSettlementHarness(t)
	ctx := context.Background()

	customerID, driverID := h.aUser(t), h.aDriver(t)
	job := h.aCompletedJob(t, jobs.TypeRide, customerID, driverID, 20000)

	if _, err := h.settling.SettleCash(ctx, job); err != nil {
		t.Fatal(err)
	}
	entries := h.jobEntries(t, job.ID)

	if _, err := h.settling.SettleCash(ctx, job); err != nil {
		t.Fatalf("a repeated settlement failed: %v", err)
	}
	if got := h.jobEntries(t, job.ID); got != entries {
		t.Errorf("a second settlement wrote %d extra ledger entries", got-entries)
	}
	if got := h.balance(t, finance.AccountDriverPayable, driverID); got != -18000 {
		t.Errorf("driver payable %d after settling twice, want -18000", got)
	}
}

// Document 059 names concurrent settlement as a race to guard, and document
// 185 requires that duplicated messages produce no duplicate financial
// effects. Two workers, a retry storm and a double-tap all look like this.
func TestConcurrentSettlementsPayTheDriverOnce(t *testing.T) {
	h := newSettlementHarness(t)
	ctx := context.Background()

	customerID, driverID := h.aUser(t), h.aDriver(t)
	job := h.aCompletedJob(t, jobs.TypeRide, customerID, driverID, 100000)

	const racers = 8
	var wg sync.WaitGroup
	errs := make(chan error, racers)
	wg.Add(racers)
	for i := 0; i < racers; i++ {
		go func() {
			defer wg.Done()
			if _, err := h.settling.SettleCash(ctx, job); err != nil {
				errs <- err
			}
		}()
	}
	wg.Wait()
	close(errs)

	// Some racers may legitimately lose — what must not happen is money moving
	// more than once.
	if got := h.balance(t, finance.AccountDriverPayable, driverID); got != -90000 {
		t.Errorf("driver payable %d after %d concurrent settlements, want -90000", got, racers)
	}
	if got := h.balance(t, finance.AccountCashInTransit, driverID); got != 100000 {
		t.Errorf("cash in transit %d, want 100000", got)
	}
	balanced, offBy, err := h.ledger.LedgerBalances(ctx)
	if err != nil {
		t.Fatal(err)
	}
	if !balanced {
		t.Errorf("the ledger does not balance after a race, off by %d", offBy)
	}
}

// On a grocery delivery most of the cash in the driver's hand belongs to the
// shop. The shop is credited in full — the owner decided on 2026-09-14 that
// the platform takes no cut from merchants — and the driver is commissioned on
// the delivery fee only.
func TestAGroceryDeliveryPaysTheShopAndTheDriverSeparately(t *testing.T) {
	h := newSettlementHarness(t)
	ctx := context.Background()

	customerID, driverID := h.aUser(t), h.aDriver(t)
	job := h.aCompletedJob(t, jobs.TypeGrocery, customerID, driverID, 20000) // Rs 200 delivery

	var merchantID, storeID string
	if err := h.pool.QueryRow(ctx,
		`INSERT INTO merchants (owner_user_id, name, status) VALUES ($1, 'Settlement Kiryana', 'ACTIVE')
		 RETURNING id::text`, h.aUser(t)).Scan(&merchantID); err != nil {
		t.Fatal(err)
	}
	if err := h.pool.QueryRow(ctx,
		`INSERT INTO stores (merchant_id, name) VALUES ($1, 'Main Branch') RETURNING id::text`,
		merchantID).Scan(&storeID); err != nil {
		t.Fatal(err)
	}
	// The order as it stands when a delivery completes: linked to the job,
	// with the basket's total frozen at checkout.
	if _, err := h.pool.Exec(ctx,
		`INSERT INTO orders (merchant_id, store_id, customer_user_id, status, job_id, items_total_minor)
		 VALUES ($1, $2, $3, 'DELIVERING', $4, 300000)`,
		merchantID, storeID, customerID, job.ID); err != nil {
		t.Fatal(err)
	}

	breakdown, err := h.settling.SettleCash(ctx, job)
	if err != nil {
		t.Fatalf("settle: %v", err)
	}

	if breakdown.Total.Minor != 320000 {
		t.Errorf("collected %d, want 320000 (300000 goods + 20000 fare)", breakdown.Total.Minor)
	}
	// Ten percent of the fare, not of the basket. Commissioning the groceries
	// would take a cut of money that belongs to the shop.
	if breakdown.Commission.Minor != 2000 {
		t.Errorf("commission %d, want 2000", breakdown.Commission.Minor)
	}
	if got := h.balance(t, finance.AccountMerchantPayable, merchantID); got != -300000 {
		t.Errorf("merchant payable %d, want -300000 (the whole basket)", got)
	}
	if got := h.balance(t, finance.AccountDriverPayable, driverID); got != -18000 {
		t.Errorf("driver payable %d, want -18000", got)
	}
	// The driver is holding all of it, and owes the shop's share plus the
	// platform's cut. BD-09 is what decides how any of it comes back.
	if got := h.balance(t, finance.AccountCashInTransit, driverID); got != 320000 {
		t.Errorf("cash in transit %d, want 320000", got)
	}
	balanced, offBy, err := h.ledger.LedgerBalances(ctx)
	if err != nil {
		t.Fatal(err)
	}
	if !balanced {
		t.Errorf("the ledger does not balance, off by %d", offBy)
	}
}

// A grocery delivery created before pricing existed carries no price lock.
// Settlement must still record what the customer paid the shop rather than
// refusing the whole thing over a missing fare.
func TestAJobWithNoPriceLockStillPaysTheShop(t *testing.T) {
	h := newSettlementHarness(t)
	ctx := context.Background()

	customerID, driverID := h.aUser(t), h.aDriver(t)
	job := h.aCompletedJob(t, jobs.TypeGrocery, customerID, driverID, 0) // no lock

	var merchantID, storeID string
	if err := h.pool.QueryRow(ctx,
		`INSERT INTO merchants (owner_user_id, name, status) VALUES ($1, 'Unpriced Kiryana', 'ACTIVE')
		 RETURNING id::text`, h.aUser(t)).Scan(&merchantID); err != nil {
		t.Fatal(err)
	}
	if err := h.pool.QueryRow(ctx,
		`INSERT INTO stores (merchant_id, name) VALUES ($1, 'Main Branch') RETURNING id::text`,
		merchantID).Scan(&storeID); err != nil {
		t.Fatal(err)
	}
	if _, err := h.pool.Exec(ctx,
		`INSERT INTO orders (merchant_id, store_id, customer_user_id, status, job_id, items_total_minor)
		 VALUES ($1, $2, $3, 'DELIVERING', $4, 120000)`,
		merchantID, storeID, customerID, job.ID); err != nil {
		t.Fatal(err)
	}

	breakdown, err := h.settling.SettleCash(ctx, job)
	if err != nil {
		t.Fatalf("settle: %v", err)
	}
	if breakdown.Fare.Minor != 0 {
		t.Errorf("fare %d, want 0", breakdown.Fare.Minor)
	}
	if got := h.balance(t, finance.AccountMerchantPayable, merchantID); got != -120000 {
		t.Errorf("merchant payable %d, want -120000", got)
	}
	// No fare means no earning, rather than an earning of nothing: a driver's
	// trip list must not fill with entries worth zero.
	if got := h.balance(t, finance.AccountDriverPayable, driverID); got != 0 {
		t.Errorf("driver payable %d, want 0", got)
	}
}

// A job that has not finished must not be charged for: the trip may yet fail.
func TestAnUnfinishedJobMovesNoMoney(t *testing.T) {
	h := newSettlementHarness(t)
	ctx := context.Background()

	customerID, driverID := h.aUser(t), h.aDriver(t)
	job := h.aCompletedJob(t, jobs.TypeRide, customerID, driverID, 40000)
	job.Status = jobs.StatusInProgress

	if _, err := h.settling.SettleCash(ctx, job); err == nil {
		t.Fatal("an unfinished job was settled")
	}
	if got := h.jobEntries(t, job.ID); got != 0 {
		t.Errorf("%d ledger entries written for an unfinished job", got)
	}
}
