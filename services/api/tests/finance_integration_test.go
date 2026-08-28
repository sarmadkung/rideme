//go:build integration

package tests

import (
	"context"
	"errors"
	"fmt"
	"sync"
	"testing"
	"time"

	"github.com/jackc/pgx/v5/pgxpool"
	"github.com/sarmadkung/rideme/services/api/internal/finance"
	"github.com/sarmadkung/rideme/services/api/internal/jobs"
	"github.com/sarmadkung/rideme/services/api/pkg/money"
)

// Document 59's definition of done: "Parallel/retry tests cannot create
// duplicate money movements." These are those tests, against real Postgres,
// because every guarantee here is a constraint or a conditional write.

type financeHarness struct {
	store *finance.Store
	jobs  *jobs.Store
	pool  *pgxpool.Pool
}

func newFinanceHarness(t *testing.T) *financeHarness {
	t.Helper()
	pool, err := pgxpool.New(context.Background(), env(t, "DATABASE_URL",
		"postgres://logistics:logistics@localhost:55432/logistics_dev?sslmode=disable"))
	if err != nil {
		t.Fatalf("connect: %v", err)
	}
	t.Cleanup(pool.Close)
	return &financeHarness{store: finance.NewStore(pool), jobs: jobs.NewStore(pool), pool: pool}
}

func (h *financeHarness) aUser(t *testing.T) string {
	t.Helper()
	var id string
	if err := h.pool.QueryRow(context.Background(),
		`INSERT INTO users (phone) VALUES ('+9242' || lpad((floor(random()*100000000))::text, 8, '0'))
		 RETURNING id::text`).Scan(&id); err != nil {
		t.Fatal(err)
	}
	return id
}

func (h *financeHarness) aJob(t *testing.T, customerID string) string {
	t.Helper()
	job, err := h.jobs.Create(context.Background(), jobs.Job{
		Type: jobs.TypeRide, RequesterUserID: customerID, Status: jobs.StatusCompleted,
		Stops: []jobs.Stop{
			{Sequence: 0, Type: jobs.StopPickup, Location: jobs.Coordinate{Latitude: 31.52, Longitude: 74.35}},
		},
	}, jobs.Actor{Type: jobs.ActorCustomer})
	if err != nil {
		t.Fatal(err)
	}
	return job.ID
}

func (h *financeHarness) anIntent(t *testing.T, customerID, jobID string, amount int64) finance.Intent {
	t.Helper()
	intent, err := h.store.CreateIntent(context.Background(), finance.Intent{
		JobID: jobID, CustomerUserID: customerID,
		Amount: money.MustNew(amount, money.PKR), Method: "CARD", Provider: "test",
	}, "")
	if err != nil {
		t.Fatal(err)
	}
	return intent
}

func TestTheLedgerAlwaysBalances(t *testing.T) {
	// The single query that proves the books are consistent. If this is ever
	// non-zero, something wrote entries outside Post.
	h := newFinanceHarness(t)
	ctx := context.Background()

	customerID := h.aUser(t)
	jobID := h.aJob(t, customerID)
	intent := h.anIntent(t, customerID, jobID, 45000)

	if _, _, err := h.store.Capture(ctx, intent.ID, intent.Amount); err != nil {
		t.Fatal(err)
	}
	earning, err := finance.DriverEarning(money.MustNew(45000, money.PKR),
		money.MustNew(9000, money.PKR), h.aUser(t), jobID)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := h.store.Post(ctx, earning); err != nil {
		t.Fatal(err)
	}

	balanced, total, err := h.store.LedgerBalances(ctx)
	if err != nil {
		t.Fatal(err)
	}
	if !balanced {
		t.Fatalf("the ledger sums to %d, not zero", total)
	}

	unbalanced, err := h.store.UnbalancedTransactions(ctx)
	if err != nil {
		t.Fatal(err)
	}
	if len(unbalanced) != 0 {
		t.Fatalf("unbalanced transactions exist: %v", unbalanced)
	}
}

func TestAnUnbalancedTransactionIsRefused(t *testing.T) {
	h := newFinanceHarness(t)
	unbalanced := finance.Transaction{
		Kind: finance.KindAdjustment,
		Entries: []finance.Entry{
			finance.Debit(finance.AccountCustomerReceivable, money.MustNew(100, money.PKR), finance.SubjectCustomer, ""),
			finance.Debit(finance.AccountPlatformClearing, money.MustNew(50, money.PKR), finance.SubjectPlatform, ""),
		},
	}
	if _, err := h.store.Post(context.Background(), unbalanced); !errors.Is(err, finance.ErrUnbalanced) {
		t.Fatalf("want ErrUnbalanced, got %v", err)
	}
}

func TestLedgerEntriesAreImmutable(t *testing.T) {
	// Document 53: "Entries cannot be edited or deleted through ordinary
	// APIs." Enforced by a trigger, so it holds for a migration or an operator
	// with psql, not only for this code.
	h := newFinanceHarness(t)
	ctx := context.Background()
	customerID := h.aUser(t)

	tx, err := finance.CustomerPayment(money.MustNew(45000, money.PKR), customerID, h.aJob(t, customerID), "")
	if err != nil {
		t.Fatal(err)
	}
	posted, err := h.store.Post(ctx, tx)
	if err != nil {
		t.Fatal(err)
	}

	if _, err := h.pool.Exec(ctx,
		`UPDATE ledger_entries SET amount_minor = 1 WHERE transaction_id = $1`, posted.ID); err == nil {
		t.Fatal("a ledger entry was updated")
	}
	if _, err := h.pool.Exec(ctx,
		`DELETE FROM ledger_entries WHERE transaction_id = $1`, posted.ID); err == nil {
		t.Fatal("a ledger entry was deleted")
	}
}

func TestACorrectionIsAReversalNotAnEdit(t *testing.T) {
	h := newFinanceHarness(t)
	ctx := context.Background()
	customerID := h.aUser(t)
	jobID := h.aJob(t, customerID)

	original, err := finance.CustomerPayment(money.MustNew(45000, money.PKR), customerID, jobID, "")
	if err != nil {
		t.Fatal(err)
	}
	posted, err := h.store.Post(ctx, original)
	if err != nil {
		t.Fatal(err)
	}

	reversal, err := finance.Reverse(posted, "duplicate capture")
	if err != nil {
		t.Fatal(err)
	}
	if _, err := h.store.Post(ctx, reversal); err != nil {
		t.Fatal(err)
	}

	// The customer's receivable nets to zero, and both transactions remain.
	balance, err := h.store.Balance(ctx, finance.AccountCustomerReceivable, finance.SubjectCustomer, customerID)
	if err != nil {
		t.Fatal(err)
	}
	if !balance.IsZero() {
		t.Fatalf("balance after reversal = %d, want 0", balance.Minor)
	}
	var count int
	if err := h.pool.QueryRow(ctx,
		`SELECT count(*) FROM ledger_transactions WHERE job_id = $1`, jobID).Scan(&count); err != nil {
		t.Fatal(err)
	}
	if count != 2 {
		t.Fatalf("%d transactions, want the original and its reversal", count)
	}
}

func TestConcurrentCapturesProduceOneCapture(t *testing.T) {
	// Document 59 names "two captures" explicitly. A provider replaying a
	// webhook, or two workers on the same event, must charge once.
	h := newFinanceHarness(t)
	ctx := context.Background()
	customerID := h.aUser(t)
	intent := h.anIntent(t, customerID, h.aJob(t, customerID), 45000)

	const racers = 10
	var wg sync.WaitGroup
	results := make(chan error, racers)
	start := make(chan struct{})
	for i := 0; i < racers; i++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			<-start
			_, _, err := h.store.Capture(ctx, intent.ID, intent.Amount)
			results <- err
		}()
	}
	close(start)
	wg.Wait()
	close(results)

	var captured int
	for err := range results {
		if err == nil {
			captured++
		} else if !errors.Is(err, finance.ErrAlreadyCaptured) {
			t.Fatalf("unexpected error: %v", err)
		}
	}
	if captured != 1 {
		t.Fatalf("%d of %d captures succeeded, want 1", captured, racers)
	}

	// Exactly one ledger movement, and the books still balance.
	var transactions int
	if err := h.pool.QueryRow(ctx,
		`SELECT count(*) FROM ledger_transactions WHERE intent_id = $1 AND kind = 'CAPTURE'`,
		intent.ID).Scan(&transactions); err != nil {
		t.Fatal(err)
	}
	if transactions != 1 {
		t.Fatalf("%d capture transactions for one intent", transactions)
	}
	if balanced, total, _ := h.store.LedgerBalances(ctx); !balanced {
		t.Fatalf("the ledger sums to %d after concurrent captures", total)
	}
}

func TestConcurrentRefundsCannotExceedTheCapture(t *testing.T) {
	// Document 59 names "two refunds". Refunding more than was taken is the
	// platform paying out money it never received.
	h := newFinanceHarness(t)
	ctx := context.Background()
	customerID := h.aUser(t)
	intent := h.anIntent(t, customerID, h.aJob(t, customerID), 10000)

	if _, _, err := h.store.Capture(ctx, intent.ID, intent.Amount); err != nil {
		t.Fatal(err)
	}

	// Ten concurrent refunds of 2000 against a 10000 capture: five may succeed.
	const racers = 10
	var wg sync.WaitGroup
	results := make(chan error, racers)
	start := make(chan struct{})
	for i := 0; i < racers; i++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			<-start
			_, _, err := h.store.Refund(ctx, intent.ID, money.MustNew(2000, money.PKR))
			results <- err
		}()
	}
	close(start)
	wg.Wait()
	close(results)

	var refunded int
	for err := range results {
		if err == nil {
			refunded++
		} else if !errors.Is(err, finance.ErrRefundExceeds) {
			t.Fatalf("unexpected error: %v", err)
		}
	}
	if refunded != 5 {
		t.Fatalf("%d refunds of 2000 succeeded against a 10000 capture, want 5", refunded)
	}

	reloaded, err := h.store.IntentByID(ctx, intent.ID)
	if err != nil {
		t.Fatal(err)
	}
	if reloaded.Refunded.Minor > reloaded.Captured.Minor {
		t.Fatalf("refunded %d against a capture of %d", reloaded.Refunded.Minor, reloaded.Captured.Minor)
	}
	if reloaded.Status != "REFUNDED" {
		t.Fatalf("status = %s after a full refund", reloaded.Status)
	}
}

func TestTheSchemaRefusesAnOverRefundEvenDirectly(t *testing.T) {
	// A provider bug must not become a platform liability, whatever a webhook
	// claims.
	h := newFinanceHarness(t)
	ctx := context.Background()
	customerID := h.aUser(t)
	intent := h.anIntent(t, customerID, h.aJob(t, customerID), 10000)
	if _, _, err := h.store.Capture(ctx, intent.ID, intent.Amount); err != nil {
		t.Fatal(err)
	}

	if _, err := h.pool.Exec(ctx,
		`UPDATE payment_intents SET refunded_minor = 99999 WHERE id = $1`, intent.ID); err == nil {
		t.Fatal("the schema accepted a refund larger than the capture")
	}
	if _, err := h.pool.Exec(ctx,
		`UPDATE payment_intents SET captured_minor = 99999 WHERE id = $1`, intent.ID); err == nil {
		t.Fatal("the schema accepted a capture larger than the intent")
	}
}

func TestOneLiveIntentPerJob(t *testing.T) {
	// Two live intents would let a customer be charged twice for one ride.
	h := newFinanceHarness(t)
	ctx := context.Background()
	customerID := h.aUser(t)
	jobID := h.aJob(t, customerID)
	h.anIntent(t, customerID, jobID, 45000)

	if _, err := h.pool.Exec(ctx,
		`INSERT INTO payment_intents (job_id, customer_user_id, amount_minor, method)
		 VALUES ($1, $2, 45000, 'CARD')`, jobID, customerID); err == nil {
		t.Fatal("a second live intent was created for one job")
	}
}

func TestARetriedIntentCreationReturnsTheOriginal(t *testing.T) {
	h := newFinanceHarness(t)
	ctx := context.Background()
	customerID := h.aUser(t)
	jobID := h.aJob(t, customerID)
	key := fmt.Sprintf("pay-%d", time.Now().UnixNano())

	first, err := h.store.CreateIntent(ctx, finance.Intent{
		JobID: jobID, CustomerUserID: customerID,
		Amount: money.MustNew(45000, money.PKR), Method: "CARD",
	}, key)
	if err != nil {
		t.Fatal(err)
	}
	second, err := h.store.CreateIntent(ctx, finance.Intent{
		JobID: jobID, CustomerUserID: customerID,
		Amount: money.MustNew(45000, money.PKR), Method: "CARD",
	}, key)
	if err != nil {
		t.Fatalf("the retry failed: %v", err)
	}
	if first.ID != second.ID {
		t.Fatalf("a retry created a second intent: %s then %s", first.ID, second.ID)
	}
}

func TestWebhookSignaturesAreVerified(t *testing.T) {
	// Document 58: an unverified webhook is an endpoint anyone can use to mark
	// anything paid.
	const secret = "provider-webhook-secret"
	payload := []byte(`{"event":"payment.captured","amount":45000}`)

	// Correct signature.
	valid := hmacHex(secret, payload)
	if err := finance.VerifySignature(secret, payload, valid); err != nil {
		t.Fatalf("a valid signature was rejected: %v", err)
	}
	// Tampered payload.
	if err := finance.VerifySignature(secret, []byte(`{"amount":99999}`), valid); !errors.Is(err, finance.ErrBadSignature) {
		t.Fatal("a tampered payload passed verification")
	}
	// Wrong secret.
	if err := finance.VerifySignature("other-secret", payload, valid); !errors.Is(err, finance.ErrBadSignature) {
		t.Fatal("a signature from another secret passed")
	}
	if err := finance.VerifySignature(secret, payload, ""); !errors.Is(err, finance.ErrBadSignature) {
		t.Fatal("an empty signature passed")
	}
}

func TestWebhooksAreDeduplicatedDurably(t *testing.T) {
	// Document 52: "Store provider event IDs." An in-memory set forgets on
	// restart, which is exactly when providers replay.
	h := newFinanceHarness(t)
	ctx := context.Background()
	eventID := fmt.Sprintf("evt_%d", time.Now().UnixNano())
	payload := []byte(`{"ok":true}`)

	first, err := h.store.RecordWebhook(ctx, "test", eventID, "payment.captured", payload, true, "")
	if err != nil {
		t.Fatal(err)
	}
	if !first {
		t.Fatal("the first delivery was reported as a duplicate")
	}
	again, err := h.store.RecordWebhook(ctx, "test", eventID, "payment.captured", payload, true, "")
	if err != nil {
		t.Fatal(err)
	}
	if again {
		t.Fatal("a replayed webhook was reported as new")
	}
}

func TestConcurrentWebhookDeliveriesAdmitOneProcessor(t *testing.T) {
	h := newFinanceHarness(t)
	eventID := fmt.Sprintf("evt_%d", time.Now().UnixNano())

	const racers = 8
	results := make(chan bool, racers)
	start := make(chan struct{})
	for i := 0; i < racers; i++ {
		go func() {
			<-start
			isNew, err := h.store.RecordWebhook(context.Background(), "test", eventID,
				"payment.captured", []byte(`{}`), true, "")
			if err != nil {
				t.Error(err)
			}
			results <- isNew
		}()
	}
	close(start)

	var firsts int
	for i := 0; i < racers; i++ {
		if <-results {
			firsts++
		}
	}
	if firsts != 1 {
		t.Fatalf("%d concurrent deliveries were each treated as the first", firsts)
	}
}

func TestEarningsCannotBeComputedWithoutAConfiguredCommission(t *testing.T) {
	// BD-05 is a PRODUCT_DECISION. A guessed rate pays every driver the wrong
	// amount, retroactively.
	h := newFinanceHarness(t)
	ctx := context.Background()

	if _, err := h.store.CommissionRateFor(ctx, "CARGO", finance.SubjectDriver); !errors.Is(err, finance.ErrNoCommission) {
		t.Fatalf("a commission rate appeared from nowhere: %v", err)
	}

	// Once configured, it is used.
	if err := h.store.SetCommissionRate(ctx, finance.CommissionRate{
		JobType: "CARGO", SubjectType: finance.SubjectDriver, RateBPS: 1500, Version: 1,
	}); err != nil {
		t.Fatal(err)
	}
	rate, err := h.store.CommissionRateFor(ctx, "CARGO", finance.SubjectDriver)
	if err != nil {
		t.Fatal(err)
	}
	if rate.RateBPS != 1500 {
		t.Fatalf("rate = %d bps", rate.RateBPS)
	}
	if _, err := h.pool.Exec(ctx, `DELETE FROM commission_rates WHERE job_type = 'CARGO'`); err != nil {
		t.Fatal(err)
	}
}

func TestDriverBalanceIsDerivedFromEntries(t *testing.T) {
	// Document 53: balances derive from immutable entries. A stored balance is
	// a second source of truth that can disagree with the first.
	h := newFinanceHarness(t)
	ctx := context.Background()
	driverID := h.aUser(t)
	customerID := h.aUser(t)

	for i := 0; i < 3; i++ {
		jobID := h.aJob(t, customerID)
		earning, err := finance.DriverEarning(money.MustNew(50000, money.PKR),
			money.MustNew(10000, money.PKR), driverID, jobID)
		if err != nil {
			t.Fatal(err)
		}
		if _, err := h.store.Post(ctx, earning); err != nil {
			t.Fatal(err)
		}
	}

	balance, err := h.store.Balance(ctx, finance.AccountDriverPayable, finance.SubjectDriver, driverID)
	if err != nil {
		t.Fatal(err)
	}
	// Three earnings of 40000 net, held as credits (negative in the signed
	// convention).
	if balance.Minor != -120000 {
		t.Fatalf("driver payable = %d, want -120000", balance.Minor)
	}
}

func TestOnePayoutPerSubjectPerPeriod(t *testing.T) {
	// Document 59 names "two payout requests" as a race. Paying a driver twice
	// for one week is discovered by whoever reconciles the bank.
	h := newFinanceHarness(t)
	ctx := context.Background()
	driverID := h.aUser(t)
	start := time.Now().UTC().Truncate(time.Hour)
	end := start.Add(24 * time.Hour)

	if _, err := h.store.CreatePayout(ctx, finance.SubjectDriver, driverID,
		money.MustNew(120000, money.PKR), start, end); err != nil {
		t.Fatal(err)
	}
	if _, err := h.store.CreatePayout(ctx, finance.SubjectDriver, driverID,
		money.MustNew(120000, money.PKR), start, end); err == nil {
		t.Fatal("a second payout was created for the same period")
	}
}

func TestReconciliationRaisesCasesRatherThanFixingSilently(t *testing.T) {
	// Document 58 and BD-08's zero-tolerance default: a tolerance band hides
	// the bug that caused the gap.
	h := newFinanceHarness(t)
	ctx := context.Background()
	customerID := h.aUser(t)
	intent := h.anIntent(t, customerID, h.aJob(t, customerID), 45000)

	before, err := h.store.OpenCases(ctx)
	if err != nil {
		t.Fatal(err)
	}

	expected := money.MustNew(45000, money.PKR)
	actual := money.MustNew(45001, money.PKR) // one paisa out
	caseID, err := h.store.OpenReconciliationCase(ctx, "test", "AMOUNT_MISMATCH",
		intent.ID, "prov-ref-1", &expected, &actual, map[string]any{"source": "daily settlement file"})
	if err != nil {
		t.Fatal(err)
	}
	if caseID == "" {
		t.Fatal("no case was created")
	}

	after, err := h.store.OpenCases(ctx)
	if err != nil {
		t.Fatal(err)
	}
	if after != before+1 {
		t.Fatalf("open cases went from %d to %d", before, after)
	}

	// Nothing auto-adjusted the intent.
	reloaded, err := h.store.IntentByID(ctx, intent.ID)
	if err != nil {
		t.Fatal(err)
	}
	if reloaded.Amount.Minor != 45000 {
		t.Fatalf("the intent was silently adjusted to %d", reloaded.Amount.Minor)
	}
}

func hmacHex(secret string, payload []byte) string {
	return finance.SignForTest(secret, payload)
}
