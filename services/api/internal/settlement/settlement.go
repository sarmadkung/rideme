// Package settlement turns a finished job into money in the books.
//
// Until this package existed the ledger had no writer. Every piece of it was
// built and tested — intents, double-entry posting, commission, earnings — and
// nothing in production called any of it, so a completed trip charged nobody,
// the platform's 20% was a row nothing read, and `GET /driver/earnings`
// answered zero for every driver on the platform because the book it reads was
// empty. This is the caller that closes that gap.
//
// Cash is the only payment method (the owner's decision, 2026-09-14), and cash
// changes the direction money flows. With a card the platform collects and
// pays the driver; with cash the *driver* collects and owes the platform. Both
// are recorded here as the same four movements, because double entry does not
// care who is holding the notes — only that every movement has two sides.
//
// Level 5 by `verification-lite`'s rule: everything in here moves real money.
package settlement

import (
	"context"
	"errors"
	"fmt"
	"log/slog"

	"github.com/jackc/pgx/v5"
	"github.com/sarmadkung/rideme/services/api/internal/finance"
	"github.com/sarmadkung/rideme/services/api/internal/jobs"
	"github.com/sarmadkung/rideme/services/api/internal/merchant"
	"github.com/sarmadkung/rideme/services/api/pkg/money"
)

// MethodCash is the only payment method the platform accepts today.
//
// Named rather than inlined so the day a second method arrives the places that
// must learn about it are the places that mention this constant.
const MethodCash = "CASH"

// ProviderCash is the "provider" for a payment nobody processed. A driver's
// hand is not a payment gateway, and calling it 'none' would make a cash
// payment indistinguishable in the data from one whose provider was never
// recorded.
const ProviderCash = "cash"

// Ledger is the half of finance.Store this package uses.
//
// Declared here rather than taking the concrete store so a test can drive the
// money paths without a database — which matters more here than anywhere else
// in the codebase, because the assertions worth making are about amounts and
// they should not need Postgres to be checked.
type Ledger interface {
	CommissionRateFor(ctx context.Context, jobType string, subject finance.SubjectType) (finance.CommissionRate, error)
	CreateIntent(ctx context.Context, intent finance.Intent, idempotencyKey string) (finance.Intent, error)
	Capture(ctx context.Context, intentID string, amount money.Amount) (finance.Intent, finance.Transaction, error)
	Post(ctx context.Context, t finance.Transaction) (finance.Transaction, error)
}

// Prices is the price a job was confirmed at (document 034's lock).
//
// The locked price and not a recomputed fare: the owner's decision on
// 2026-09-14 is that a customer pays what they were quoted. Reading the lock
// rather than re-running the pricing engine is what makes that true even if a
// tariff changed between the booking and the trip ending.
type Prices interface {
	LockedPrice(ctx context.Context, jobID string) (money.Amount, int, error)
}

// Goods is the shop side of a delivery, when the job came from one.
//
// Optional: a ride has no order behind it, and asking is cheaper than
// threading a job type through every caller.
type Goods interface {
	OrderByJobID(ctx context.Context, jobID string) (merchant.Order, error)
}

// Service settles finished jobs in cash.
type Service struct {
	ledger Ledger
	prices Prices
	// goods is optional; without it only the fare is settled, which is the
	// correct behaviour for a deployment with no grocery.
	goods  Goods
	logger *slog.Logger
}

func NewService(ledger Ledger, prices Prices, logger *slog.Logger) *Service {
	if logger == nil {
		logger = slog.Default()
	}
	return &Service{ledger: ledger, prices: prices, logger: logger}
}

// WithGoods attaches the shop side, so a grocery delivery settles what is in
// the bag as well as what the driver was paid to carry it.
func (s *Service) WithGoods(goods Goods) *Service {
	s.goods = goods
	return s
}

var (
	// ErrNotComplete reports an attempt to settle a job that has not finished.
	// Settling early would charge a customer for a trip that may yet fail.
	ErrNotComplete = errors.New("settlement: the job is not complete")
	// ErrNoDriver reports a completed job with nobody to pay. It cannot happen
	// through the lifecycle — a job reaches COMPLETED only by a driver's own
	// command — so it is a guard against a future path, not a live case.
	ErrNoDriver = errors.New("settlement: the job has no assigned driver")
)

// Breakdown is what a settlement moved, returned for the caller's log and for
// tests to assert on.
type Breakdown struct {
	Fare       money.Amount
	Goods      money.Amount
	Total      money.Amount
	Commission money.Amount
	DriverNet  money.Amount
}

// Settle is SettleCash with the breakdown logged rather than returned.
//
// The shape booking's lifecycle calls: it needs to know whether the books were
// written, not what was written into them. The amounts are logged here, where
// they are already known, so no caller has to carry the type to report them.
func (s *Service) Settle(ctx context.Context, job jobs.Job) error {
	breakdown, err := s.SettleCash(ctx, job)
	if err != nil {
		return err
	}
	s.logger.Info("a completed job was settled in cash",
		slog.String("job_id", job.ID),
		slog.String("job_type", string(job.Type)),
		slog.String("fare", breakdown.Fare.String()),
		slog.String("goods", breakdown.Goods.String()),
		slog.String("collected", breakdown.Total.String()),
		slog.String("commission", breakdown.Commission.String()),
		slog.String("driver_net", breakdown.DriverNet.String()))
	return nil
}

// SettleCash records a completed job's money.
//
// Four movements, in this order:
//
//  1. capture          DR Customer Receivable  CR Platform Clearing
//  2. cash collection  DR Cash In Transit      CR Customer Receivable
//  3. merchant sale    DR Platform Clearing    CR Merchant Payable
//  4. driver earning   DR Driver Expense       CR Driver Payable + Platform Revenue
//
// One and two together are the whole of what cash means: the customer's debt
// is created and immediately discharged by handing notes to a driver, so
// Customer Receivable nets to zero and Cash In Transit is left holding the
// platform's money in somebody's pocket. Nobody owes the customer anything;
// the driver owes the platform.
//
// Every movement is keyed by the job, and Post returns the existing
// transaction when a key repeats. Document 054 requires that earnings "cannot
// be duplicated by repeated completion events", and a driver on a bad
// connection tapping "complete" twice is the ordinary case, not the exotic
// one.
func (s *Service) SettleCash(ctx context.Context, job jobs.Job) (Breakdown, error) {
	if job.Status != jobs.StatusCompleted {
		return Breakdown{}, fmt.Errorf("%w: %s", ErrNotComplete, job.Status)
	}
	if job.AssignedDriverID == "" {
		return Breakdown{}, ErrNoDriver
	}

	zero, err := money.Zero(money.PKR)
	if err != nil {
		return Breakdown{}, err
	}
	breakdown := Breakdown{Fare: zero, Goods: zero, Total: zero, Commission: zero, DriverNet: zero}

	fare, err := s.fareOf(ctx, job.ID)
	if err != nil {
		return Breakdown{}, err
	}
	breakdown.Fare = fare

	basket, err := s.basketOf(ctx, job.ID)
	if err != nil {
		return Breakdown{}, err
	}
	breakdown.Goods = basket.ItemsTotal

	total, err := fare.Add(basket.ItemsTotal)
	if err != nil {
		return Breakdown{}, err
	}
	breakdown.Total = total

	// Nothing to settle is a real state, not an error: a grocery delivery
	// carries no fare today because nothing prices one, and a job with no
	// price lock and no basket would otherwise fail a driver's command over a
	// payment of zero. It is logged because a completed job that moved no
	// money is worth noticing.
	if !total.IsPositive() {
		s.logger.Warn("a completed job settled no money",
			slog.String("job_id", job.ID), slog.String("job_type", string(job.Type)))
		return breakdown, nil
	}

	if err := s.collect(ctx, job, total); err != nil {
		return Breakdown{}, err
	}
	if basket.ItemsTotal.IsPositive() {
		if err := s.payMerchant(ctx, job, basket); err != nil {
			return Breakdown{}, err
		}
	}
	commission, net, err := s.payDriver(ctx, job, fare)
	if err != nil {
		return Breakdown{}, err
	}
	breakdown.Commission = commission
	breakdown.DriverNet = net
	return breakdown, nil
}

// fareOf reads the price the job was confirmed at, treating an absent lock as
// zero rather than as a failure.
//
// A grocery delivery has no price lock — the job is created by the shop when
// an order is ready, and nothing quotes it. That is a gap in pricing, not a
// reason to refuse to record the goods the customer did pay for, so it is a
// zero here and a warning above.
func (s *Service) fareOf(ctx context.Context, jobID string) (money.Amount, error) {
	fare, _, err := s.prices.LockedPrice(ctx, jobID)
	if errors.Is(err, pgx.ErrNoRows) {
		return money.Zero(money.PKR)
	}
	if err != nil {
		return money.Amount{}, fmt.Errorf("settlement: read the locked price for job %s: %w", jobID, err)
	}
	if fare.IsNegative() {
		return money.Amount{}, fmt.Errorf("settlement: job %s has a negative locked price", jobID)
	}
	return fare, nil
}

// basketOf reads the order behind a delivery, if there is one.
func (s *Service) basketOf(ctx context.Context, jobID string) (merchant.Order, error) {
	zero, err := money.Zero(money.PKR)
	if err != nil {
		return merchant.Order{}, err
	}
	empty := merchant.Order{ItemsTotal: zero}
	if s.goods == nil {
		return empty, nil
	}
	order, err := s.goods.OrderByJobID(ctx, jobID)
	if errors.Is(err, merchant.ErrNotFound) {
		return empty, nil
	}
	if err != nil {
		return merchant.Order{}, fmt.Errorf("settlement: load the order behind job %s: %w", jobID, err)
	}
	if order.ItemsTotal.Currency == "" {
		order.ItemsTotal = zero
	}
	return order, nil
}

// collect opens the cash intent, captures it, and records the driver taking
// the notes.
func (s *Service) collect(ctx context.Context, job jobs.Job, total money.Amount) error {
	intent, err := s.ledger.CreateIntent(ctx, finance.Intent{
		JobID:          job.ID,
		CustomerUserID: job.RequesterUserID,
		Amount:         total,
		Method:         MethodCash,
		Provider:       ProviderCash,
	}, "fare:"+job.ID)
	if err != nil {
		return fmt.Errorf("settlement: open the cash intent for job %s: %w", job.ID, err)
	}

	// ErrAlreadyCaptured means a previous settlement got this far. The
	// movements below are keyed and idempotent, so carrying on re-posts
	// nothing and repairs a settlement that failed halfway.
	if _, _, err := s.ledger.Capture(ctx, intent.ID, total); err != nil &&
		!errors.Is(err, finance.ErrAlreadyCaptured) {
		return fmt.Errorf("settlement: capture the cash for job %s: %w", job.ID, err)
	}

	collection, err := finance.CashCollection(total, job.AssignedDriverID, job.RequesterUserID, job.ID)
	if err != nil {
		return err
	}
	collection.IntentID = intent.ID
	collection.IdempotencyKey = "cod:" + job.ID
	if _, err := s.ledger.Post(ctx, collection); err != nil {
		return fmt.Errorf("settlement: record the cash collected on job %s: %w", job.ID, err)
	}
	return nil
}

// payMerchant credits the shop for the goods.
//
// The full goods total: the owner decided on 2026-09-14 that the platform
// takes no cut from a shop. CommissionRateFor is not consulted for MERCHANT at
// all, so the day a shop rate is configured it will not start applying itself
// by accident — adding it will be a deliberate change here.
func (s *Service) payMerchant(ctx context.Context, job jobs.Job, order merchant.Order) error {
	zero, err := money.Zero(order.ItemsTotal.Currency)
	if err != nil {
		return err
	}
	sale, err := finance.MerchantSale(order.ItemsTotal, zero, order.MerchantID, order.ID, job.ID)
	if err != nil {
		return err
	}
	sale.IdempotencyKey = "goods:" + job.ID
	if _, err := s.ledger.Post(ctx, sale); err != nil {
		return fmt.Errorf("settlement: credit the shop for job %s: %w", job.ID, err)
	}
	return nil
}

// payDriver posts the driver's earning on the fare, net of commission.
//
// The fare only. A driver carrying PKR 3,000 of somebody's shopping earned the
// delivery fee, not a share of the groceries, and commissioning the goods
// would take a cut of money that belongs to the shop.
func (s *Service) payDriver(ctx context.Context, job jobs.Job, fare money.Amount) (commission, net money.Amount, err error) {
	zero, err := money.Zero(money.PKR)
	if err != nil {
		return money.Amount{}, money.Amount{}, err
	}
	if !fare.IsPositive() {
		return zero, zero, nil
	}

	rate, err := s.ledger.CommissionRateFor(ctx, string(job.Type), finance.SubjectDriver)
	if err != nil {
		// BD-05's refusal, reaching the surface. A job type with no configured
		// rate must not be settled at a guessed one: every driver of that
		// service would be paid the wrong amount, and discovered by them.
		return money.Amount{}, money.Amount{}, fmt.Errorf("settlement: job %s (%s): %w", job.ID, job.Type, err)
	}
	commission, err = finance.Commission(fare, rate)
	if err != nil {
		return money.Amount{}, money.Amount{}, err
	}
	if net, err = fare.Sub(commission); err != nil {
		return money.Amount{}, money.Amount{}, err
	}

	earning, err := finance.DriverEarning(fare, commission, job.AssignedDriverID, job.ID)
	if err != nil {
		return money.Amount{}, money.Amount{}, err
	}
	earning.IdempotencyKey = "earning:" + job.ID
	if _, err := s.ledger.Post(ctx, earning); err != nil {
		return money.Amount{}, money.Amount{}, fmt.Errorf("settlement: pay the driver for job %s: %w", job.ID, err)
	}
	return commission, net, nil
}
