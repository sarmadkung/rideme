package settlement

import (
	"context"
	"errors"
	"io"
	"log/slog"
	"testing"

	"github.com/jackc/pgx/v5"
	"github.com/sarmadkung/rideme/services/api/internal/finance"
	"github.com/sarmadkung/rideme/services/api/internal/jobs"
	"github.com/sarmadkung/rideme/services/api/internal/merchant"
	"github.com/sarmadkung/rideme/services/api/pkg/money"
)

// --- fakes -------------------------------------------------------------------

// fakeLedger behaves like finance.Store in the ways settlement depends on:
// idempotency keys deduplicate, capture happens once, and every posted
// transaction is kept so a test can re-sum the whole book.
type fakeLedger struct {
	rate     *finance.CommissionRate
	intents  map[string]*finance.Intent
	byKey    map[string]finance.Transaction
	posted   []finance.Transaction
	captured map[string]bool
	nextID   int
}

func newFakeLedger(rateBPS int) *fakeLedger {
	l := &fakeLedger{
		intents:  map[string]*finance.Intent{},
		byKey:    map[string]finance.Transaction{},
		captured: map[string]bool{},
	}
	if rateBPS >= 0 {
		l.rate = &finance.CommissionRate{
			JobType: "RIDE", SubjectType: finance.SubjectDriver,
			RateBPS: rateBPS, Flat: money.MustNew(0, money.PKR), Version: 2,
		}
	}
	return l
}

func (l *fakeLedger) CommissionRateFor(_ context.Context, _ string, _ finance.SubjectType) (finance.CommissionRate, error) {
	if l.rate == nil {
		return finance.CommissionRate{}, finance.ErrNoCommission
	}
	return *l.rate, nil
}

func (l *fakeLedger) CreateIntent(_ context.Context, intent finance.Intent, key string) (finance.Intent, error) {
	if existing, ok := l.intents[key]; ok {
		return *existing, nil
	}
	l.nextID++
	intent.ID = "intent-" + string(rune('a'+l.nextID))
	intent.Status = "REQUIRES_PAYMENT"
	l.intents[key] = &intent
	return intent, nil
}

func (l *fakeLedger) Capture(_ context.Context, intentID string, amount money.Amount) (finance.Intent, finance.Transaction, error) {
	if l.captured[intentID] {
		return finance.Intent{}, finance.Transaction{}, finance.ErrAlreadyCaptured
	}
	l.captured[intentID] = true
	var customer, job string
	for _, intent := range l.intents {
		if intent.ID == intentID {
			intent.Status = "CAPTURED"
			customer, job = intent.CustomerUserID, intent.JobID
		}
	}
	// The real Capture posts this movement inside the same database
	// transaction; posting it here is what lets the balance assertion below
	// see the whole book rather than three quarters of it.
	movement, err := finance.CustomerPayment(amount, customer, job, intentID)
	if err != nil {
		return finance.Intent{}, finance.Transaction{}, err
	}
	posted, err := l.Post(context.Background(), movement)
	return finance.Intent{ID: intentID}, posted, err
}

func (l *fakeLedger) Post(_ context.Context, t finance.Transaction) (finance.Transaction, error) {
	if err := t.Balance(); err != nil {
		return finance.Transaction{}, err
	}
	if t.IdempotencyKey != "" {
		if existing, ok := l.byKey[t.IdempotencyKey]; ok {
			return existing, nil
		}
	}
	l.nextID++
	t.ID = "txn-" + string(rune('a'+l.nextID))
	if t.IdempotencyKey != "" {
		l.byKey[t.IdempotencyKey] = t
	}
	l.posted = append(l.posted, t)
	return t, nil
}

// balances re-sums every entry in the whole book. The invariant document 53
// states is per transaction; summing across all of them catches a settlement
// that balances each movement and still leaves money in the wrong account.
func (l *fakeLedger) balances() int64 {
	var sum int64
	for _, t := range l.posted {
		for _, e := range t.Entries {
			sum += e.Amount.Minor
		}
	}
	return sum
}

// net returns the signed total sitting in one account for one subject.
func (l *fakeLedger) net(account finance.Account, subject string) int64 {
	var sum int64
	for _, t := range l.posted {
		for _, e := range t.Entries {
			if e.Account == account && (subject == "" || e.SubjectID == subject) {
				sum += e.Amount.Minor
			}
		}
	}
	return sum
}

type fakePrices struct {
	minor int64
	none  bool
}

func (p fakePrices) LockedPrice(_ context.Context, _ string) (money.Amount, int, error) {
	if p.none {
		return money.Amount{}, 0, pgx.ErrNoRows
	}
	return money.MustNew(p.minor, money.PKR), 3, nil
}

type fakeGoods struct {
	order *merchant.Order
}

func (g fakeGoods) OrderByJobID(_ context.Context, _ string) (merchant.Order, error) {
	if g.order == nil {
		return merchant.Order{}, merchant.ErrNotFound
	}
	return *g.order, nil
}

// discardLogger keeps the settlement warnings out of the test output; they are
// behaviour worth having and not worth reading five times per run.
func discardLogger() *slog.Logger {
	return slog.New(slog.NewTextHandler(io.Discard, nil))
}

func completedRide() jobs.Job {
	return jobs.Job{
		ID: "job-1", Type: jobs.TypeRide, Status: jobs.StatusCompleted,
		RequesterUserID: "customer-1", AssignedDriverID: "driver-1",
	}
}

// --- tests -------------------------------------------------------------------

// A completed ride charges the customer the price they were quoted and pays
// the driver that price less the configured commission. Before this package
// existed, none of these four movements happened for any trip on the platform.
func TestACompletedRideIsSettledInCash(t *testing.T) {
	ledger := newFakeLedger(1000) // BD-05 as revised: 10%
	service := NewService(ledger, fakePrices{minor: 50000}, discardLogger())

	breakdown, err := service.SettleCash(context.Background(), completedRide())
	if err != nil {
		t.Fatalf("settle: %v", err)
	}

	if breakdown.Total.Minor != 50000 {
		t.Errorf("collected %d, want 50000", breakdown.Total.Minor)
	}
	if breakdown.Commission.Minor != 5000 {
		t.Errorf("commission %d, want 5000 (10%% of 50000)", breakdown.Commission.Minor)
	}
	if breakdown.DriverNet.Minor != 45000 {
		t.Errorf("driver net %d, want 45000", breakdown.DriverNet.Minor)
	}
	if ledger.balances() != 0 {
		t.Errorf("the book does not balance: %d", ledger.balances())
	}

	// The driver is owed 45000 (a credit, so negative) and is holding 50000 of
	// the platform's cash (a debit). With cash the driver ends up owing the
	// commission, which is the direction a card payment would not produce.
	if got := ledger.net(finance.AccountDriverPayable, "driver-1"); got != -45000 {
		t.Errorf("driver payable %d, want -45000", got)
	}
	if got := ledger.net(finance.AccountCashInTransit, "driver-1"); got != 50000 {
		t.Errorf("cash in transit %d, want 50000", got)
	}
	// Nobody owes the customer anything: the receivable was created and
	// discharged by the notes changing hands.
	if got := ledger.net(finance.AccountCustomerReceivable, "customer-1"); got != 0 {
		t.Errorf("customer receivable %d, want 0", got)
	}
	if got := ledger.net(finance.AccountPlatformRevenue, ""); got != -5000 {
		t.Errorf("platform revenue %d, want -5000", got)
	}
}

// Document 054: earnings "cannot be duplicated by repeated completion events".
// A driver on a bad connection tapping "complete" twice is the ordinary case.
func TestSettlingTwicePostsNothingTwice(t *testing.T) {
	ledger := newFakeLedger(1000)
	service := NewService(ledger, fakePrices{minor: 50000}, discardLogger())
	job := completedRide()

	if _, err := service.SettleCash(context.Background(), job); err != nil {
		t.Fatalf("first settle: %v", err)
	}
	first := len(ledger.posted)
	if _, err := service.SettleCash(context.Background(), job); err != nil {
		t.Fatalf("second settle: %v", err)
	}

	if len(ledger.posted) != first {
		t.Errorf("a second settlement posted %d extra transactions", len(ledger.posted)-first)
	}
	if got := ledger.net(finance.AccountDriverPayable, "driver-1"); got != -45000 {
		t.Errorf("driver payable %d after settling twice, want -45000", got)
	}
	if ledger.balances() != 0 {
		t.Errorf("the book does not balance after a repeat: %d", ledger.balances())
	}
}

// On a grocery delivery most of the cash in the driver's hand belongs to the
// shop. The driver is commissioned on the delivery fee and not on the
// groceries: taking a cut of the goods would be taking it from the merchant.
func TestAGroceryDeliverySettlesTheShopAndTheDriverSeparately(t *testing.T) {
	ledger := newFakeLedger(1000)
	order := merchant.Order{
		ID: "order-1", MerchantID: "merchant-1",
		ItemsTotal: money.MustNew(300000, money.PKR),
	}
	service := NewService(ledger, fakePrices{minor: 20000}, discardLogger()).
		WithGoods(fakeGoods{order: &order})

	job := completedRide()
	job.Type = jobs.TypeGrocery

	breakdown, err := service.SettleCash(context.Background(), job)
	if err != nil {
		t.Fatalf("settle: %v", err)
	}

	if breakdown.Total.Minor != 320000 {
		t.Errorf("collected %d, want 320000 (300000 goods + 20000 fare)", breakdown.Total.Minor)
	}
	// 10% of the fare, not of the basket.
	if breakdown.Commission.Minor != 2000 {
		t.Errorf("commission %d, want 2000 (10%% of the 20000 fare)", breakdown.Commission.Minor)
	}
	if got := ledger.net(finance.AccountMerchantPayable, "merchant-1"); got != -300000 {
		t.Errorf("merchant payable %d, want -300000 (the whole basket)", got)
	}
	if got := ledger.net(finance.AccountDriverPayable, "driver-1"); got != -18000 {
		t.Errorf("driver payable %d, want -18000", got)
	}
	if got := ledger.net(finance.AccountCashInTransit, "driver-1"); got != 320000 {
		t.Errorf("cash in transit %d, want 320000", got)
	}
	if ledger.balances() != 0 {
		t.Errorf("the book does not balance: %d", ledger.balances())
	}
}

// BD-05's refusal, reaching the surface. A job type with no configured rate
// must not settle at a guessed one — every driver of that service would be
// paid the wrong amount, and would be the one to discover it.
func TestAnUnconfiguredCommissionRefusesToSettle(t *testing.T) {
	ledger := newFakeLedger(-1) // no rate configured
	service := NewService(ledger, fakePrices{minor: 50000}, discardLogger())

	if _, err := service.SettleCash(context.Background(), completedRide()); !errors.Is(err, finance.ErrNoCommission) {
		t.Fatalf("error %v, want ErrNoCommission", err)
	}
	// The cash was still collected: the customer paid and the driver is
	// holding it, which is true whether or not the platform knows its cut. An
	// unsettled earning is recoverable; a lost collection is not.
	if got := ledger.net(finance.AccountCashInTransit, "driver-1"); got != 50000 {
		t.Errorf("cash in transit %d, want 50000", got)
	}
	if got := ledger.net(finance.AccountDriverPayable, "driver-1"); got != 0 {
		t.Errorf("driver payable %d, want 0 — no rate means no earning", got)
	}
}

// A grocery delivery carries no fare today because nothing prices one. That is
// a gap in pricing, not a reason to refuse to record the goods the customer
// did pay for.
func TestAJobWithNoPriceLockStillSettlesItsGoods(t *testing.T) {
	ledger := newFakeLedger(1000)
	order := merchant.Order{
		ID: "order-2", MerchantID: "merchant-1",
		ItemsTotal: money.MustNew(120000, money.PKR),
	}
	service := NewService(ledger, fakePrices{none: true}, discardLogger()).
		WithGoods(fakeGoods{order: &order})

	job := completedRide()
	job.Type = jobs.TypeGrocery

	breakdown, err := service.SettleCash(context.Background(), job)
	if err != nil {
		t.Fatalf("settle: %v", err)
	}
	if breakdown.Fare.Minor != 0 {
		t.Errorf("fare %d, want 0", breakdown.Fare.Minor)
	}
	if breakdown.Total.Minor != 120000 {
		t.Errorf("collected %d, want 120000", breakdown.Total.Minor)
	}
	// No fare means no earning, rather than an earning of zero: a driver's
	// trip list should not fill with entries worth nothing.
	if got := ledger.net(finance.AccountDriverPayable, "driver-1"); got != 0 {
		t.Errorf("driver payable %d, want 0", got)
	}
	if got := ledger.net(finance.AccountMerchantPayable, "merchant-1"); got != -120000 {
		t.Errorf("merchant payable %d, want -120000", got)
	}
	if ledger.balances() != 0 {
		t.Errorf("the book does not balance: %d", ledger.balances())
	}
}

// Settling before a job finishes would charge a customer for a trip that may
// yet fail.
func TestAnUnfinishedJobIsNotSettled(t *testing.T) {
	ledger := newFakeLedger(1000)
	service := NewService(ledger, fakePrices{minor: 50000}, discardLogger())

	job := completedRide()
	job.Status = jobs.StatusInProgress

	if _, err := service.SettleCash(context.Background(), job); !errors.Is(err, ErrNotComplete) {
		t.Fatalf("error %v, want ErrNotComplete", err)
	}
	if len(ledger.posted) != 0 {
		t.Errorf("%d transactions posted for an unfinished job", len(ledger.posted))
	}
}

// A completed job that moved no money is a real state, not an error: it must
// not fail the driver's command.
func TestAJobWorthNothingSettlesQuietly(t *testing.T) {
	ledger := newFakeLedger(1000)
	service := NewService(ledger, fakePrices{none: true}, discardLogger())

	breakdown, err := service.SettleCash(context.Background(), completedRide())
	if err != nil {
		t.Fatalf("settle: %v", err)
	}
	if breakdown.Total.Minor != 0 {
		t.Errorf("collected %d, want 0", breakdown.Total.Minor)
	}
	if len(ledger.posted) != 0 {
		t.Errorf("%d transactions posted for a job worth nothing", len(ledger.posted))
	}
}
