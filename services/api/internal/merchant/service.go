package merchant

import (
	"context"
	"errors"
	"strings"
	"time"

	"github.com/sarmadkung/rideme/services/api/internal/jobs"
)

// StatusActive is the only merchant status that may act on an order.
const StatusActive = "ACTIVE"

// MaxRejectionReason bounds the reason a merchant types.
const MaxRejectionReason = 500

// OrderStore is what the merchant surface needs from persistence.
//
// An interface rather than *Store so the handler's behaviour — ownership,
// idempotency, which states each action is legal from — is testable without a
// database. *Store satisfies it.
type OrderStore interface {
	MerchantByOwner(ctx context.Context, userID string) (Merchant, error)
	OrdersFor(ctx context.Context, merchantID string, statuses []OrderStatus,
		before *time.Time, limit int) ([]Order, error)
	OrderByID(ctx context.Context, id string) (Order, error)
	Transition(ctx context.Context, orderID string, from, to OrderStatus,
		actorType, actorID string, metadata map[string]any) (Order, error)
	Reject(ctx context.Context, orderID string, from OrderStatus, actorID, reason string) (Order, error)
	OutletByID(ctx context.Context, id string) (Outlet, error)
	AttachJob(ctx context.Context, orderID, jobID string) error
}

// JobCreator is the delivery half of document 070's two lifecycles.
//
// An order produces a Job; it does not become one. They are separate
// lifecycles with one link, so this is the whole of the coupling: the merchant
// module knows how to ask for a delivery and nothing about how one is
// dispatched. *jobs.Store satisfies it.
type JobCreator interface {
	Create(ctx context.Context, job jobs.Job, actor jobs.Actor) (jobs.Job, error)
}

// Service is the merchant's own view of its orders (document 072).
type Service struct {
	store OrderStore
	jobs  JobCreator
}

func NewService(store OrderStore) *Service { return &Service{store: store} }

// WithJobs attaches the job store, which is what lets an order be handed to a
// driver.
//
// Optional, and nil is a legitimate state: without it Mark Ready refuses
// rather than moving an order to READY_FOR_PICKUP that no driver will ever be
// offered. An order stranded in a state with no delivery is worse than one a
// merchant cannot mark ready yet.
func (s *Service) WithJobs(creator JobCreator) *Service {
	s.jobs = creator
	return s
}

// Queue is one of document 072's five dashboard queues.
type Queue string

const (
	QueueNew       Queue = "new"
	QueuePreparing Queue = "preparing"
	QueueReady     Queue = "ready"
	QueueCompleted Queue = "completed"
	QueueCancelled Queue = "cancelled"
)

// Statuses maps a queue onto the lifecycle states it holds.
//
// Eleven states into five queues, so the mapping is a decision rather than a
// translation. Two are worth stating:
//
//   - CONFIRMED sits in Preparing, not New. Once a merchant has accepted, the
//     order is no longer a question waiting on them, and leaving it in New
//     means the queue that must be watched is never empty.
//   - PICKED_UP and DELIVERING sit in Completed. They are in a driver's hands;
//     from the shop's side the work is done. The customer's view of the same
//     order is "on its way", and that is a different screen.
func (q Queue) Statuses() ([]OrderStatus, bool) {
	switch q {
	case QueueNew:
		// PAYMENT_PENDING is here to be seen, not to be accepted: it is a
		// question for the payment flow, and Accept refuses it.
		return []OrderStatus{StatusPlaced, StatusPaymentPending}, true
	case QueuePreparing:
		return []OrderStatus{StatusConfirmed, StatusPreparing}, true
	case QueueReady:
		return []OrderStatus{StatusReadyForPickup}, true
	case QueueCompleted:
		return []OrderStatus{StatusPickedUp, StatusDelivering, StatusDelivered}, true
	case QueueCancelled:
		return []OrderStatus{StatusCancelled, StatusFailed}, true
	default:
		return nil, false
	}
}

// Queue lists the orders in one queue for the merchant this user operates.
func (s *Service) Queue(ctx context.Context, userID string, queue Queue,
	before *time.Time, limit int) ([]Order, error) {
	statuses, ok := queue.Statuses()
	if !ok {
		return nil, ErrNoSuchQueue
	}
	m, err := s.merchantOf(ctx, userID)
	if err != nil {
		return nil, err
	}
	return s.store.OrdersFor(ctx, m.ID, statuses, before, limit)
}

// Order loads one of this merchant's orders, with its lines.
func (s *Service) Order(ctx context.Context, userID, orderID string) (Order, error) {
	_, order, err := s.owned(ctx, userID, orderID)
	return order, err
}

// Accept confirms an order (document 072's Accept).
//
// Idempotent: a merchant who taps Accept twice on a bad connection has
// answered once, and the second tap must not tell them something went wrong.
func (s *Service) Accept(ctx context.Context, userID, orderID string) (Order, error) {
	m, order, err := s.owned(ctx, userID, orderID)
	if err != nil {
		return Order{}, err
	}
	if err := active(m); err != nil {
		return Order{}, err
	}
	if order.Status == StatusConfirmed {
		return order, nil
	}
	if order.Status != StatusPlaced {
		return Order{}, unacceptable(order.Status)
	}
	return s.store.Transition(ctx, order.ID, StatusPlaced, StatusConfirmed, "MERCHANT", m.ID, nil)
}

// Reject declines an order with a reason (document 072's Reject).
//
// The reason is required. Document 070 allows rejection only before
// preparation, and a rejection with no reason is one the customer cannot act
// on and support cannot explain.
func (s *Service) Reject(ctx context.Context, userID, orderID, reason string) (Order, error) {
	reason = strings.TrimSpace(reason)
	if reason == "" {
		return Order{}, ErrReasonRequired
	}
	if len(reason) > MaxRejectionReason {
		reason = reason[:MaxRejectionReason]
	}
	m, order, err := s.owned(ctx, userID, orderID)
	if err != nil {
		return Order{}, err
	}
	if err := active(m); err != nil {
		return Order{}, err
	}
	if !MerchantCancellable(order.Status) {
		return Order{}, ErrNotCancellable
	}
	return s.store.Reject(ctx, order.ID, order.Status, m.ID, reason)
}

// StartPreparing begins picking (document 072's Start Preparing).
//
// Idempotent for the same reason Accept is.
func (s *Service) StartPreparing(ctx context.Context, userID, orderID string) (Order, error) {
	m, order, err := s.owned(ctx, userID, orderID)
	if err != nil {
		return Order{}, err
	}
	if err := active(m); err != nil {
		return Order{}, err
	}
	if order.Status == StatusPreparing {
		return order, nil
	}
	if order.Status != StatusConfirmed {
		return Order{}, ErrNotPreparable
	}
	return s.store.Transition(ctx, order.ID, StatusConfirmed, StatusPreparing, "MERCHANT", m.ID, nil)
}

// MarkReady finishes the merchant's part and hands the order to a driver
// (document 072's Mark Ready).
//
// This is document 070's "separate lifecycles, one link": reaching
// READY_FOR_PICKUP produces a delivery Job whose pickup is the shop and whose
// dropoff is the address the customer gave at checkout. The order does not
// become the job and the job does not carry the order's states — a driver app
// has no business understanding PREPARING.
//
// The order moves first and the job is created second, which is the safe way
// round when two stores cannot share one transaction. If the job creation
// fails, the order is READY_FOR_PICKUP with no job attached, and calling this
// again creates it: recoverable, and visible in the queue as a ready order
// with nobody coming. The other order would leave an orphan job offered to a
// driver for an order that never became ready.
func (s *Service) MarkReady(ctx context.Context, userID, orderID string) (Order, string, error) {
	if s.jobs == nil {
		return Order{}, "", ErrNoDeliveries
	}
	m, order, err := s.owned(ctx, userID, orderID)
	if err != nil {
		return Order{}, "", err
	}
	if err := active(m); err != nil {
		return Order{}, "", err
	}

	switch order.Status {
	case StatusReadyForPickup:
		// Already answered. If the job exists this is the second tap on a bad
		// connection; if it does not, the last attempt failed after the
		// transition and this is the retry that finishes it.
		if order.JobID != "" {
			return order, order.JobID, nil
		}
	case StatusPreparing:
	default:
		return Order{}, "", ErrNotReadyable
	}

	// Both ends of the route must exist before anything moves. A job with a
	// pickup nobody can route to is one no driver can take, and a job with no
	// destination is one no driver can finish.
	outlet, err := s.store.OutletByID(ctx, order.StoreID)
	if err != nil {
		return Order{}, "", err
	}
	pickup := jobs.Coordinate{Latitude: outlet.Lat, Longitude: outlet.Lon}
	if !pickup.Valid() {
		return Order{}, "", ErrNoPickupPoint
	}
	if !order.Delivery.Valid() {
		return Order{}, "", ErrNoDestination
	}

	ready := order
	if order.Status == StatusPreparing {
		ready, err = s.store.Transition(ctx, order.ID, StatusPreparing, StatusReadyForPickup,
			"MERCHANT", m.ID, nil)
		if err != nil {
			return Order{}, "", err
		}
	}

	delivery, err := s.jobs.Create(ctx, jobs.Job{
		Type: jobs.TypeGrocery,
		// The customer, not the merchant: a delivery exists for the person
		// waiting at the other end, and every customer-facing view of a job
		// is scoped by requester.
		RequesterUserID: ready.CustomerUserID,
		MerchantID:      ready.MerchantID,
		// REQUESTED, so the dispatch round picks it up like any other job.
		// Nothing here knows how dispatch works and nothing there needs to
		// know this came from a shop.
		Status: jobs.StatusRequested,
		Stops: []jobs.Stop{
			{Sequence: 0, Type: jobs.StopPickup, Location: pickup, Address: outlet.Address,
				ContactName: outlet.Name, ContactPhone: m.Phone},
			{Sequence: 1, Type: jobs.StopDropoff,
				Location: jobs.Coordinate{
					Latitude: ready.Delivery.Lat, Longitude: ready.Delivery.Lon,
				},
				Address: ready.Delivery.Address},
		},
	}, jobs.Actor{Type: jobs.ActorMerchant, ID: m.ID})
	if err != nil {
		return ready, "", err
	}

	if err := s.store.AttachJob(ctx, ready.ID, delivery.ID); err != nil {
		// The order is ready and a job exists; they are simply not linked.
		// Reporting it is right — a caller that believed this succeeded would
		// never retry — and the link is what the next retry repairs.
		return ready, delivery.ID, err
	}
	ready.JobID = delivery.ID
	return ready, delivery.ID, nil
}

// owned resolves the caller's merchant and the order, and refuses an order
// belonging to somebody else.
//
// The refusal is ErrNotFound rather than a forbidden: a merchant who can
// discover that an order id exists but belongs to a competitor has learned
// something the platform did not mean to tell them.
func (s *Service) owned(ctx context.Context, userID, orderID string) (Merchant, Order, error) {
	m, err := s.merchantOf(ctx, userID)
	if err != nil {
		return Merchant{}, Order{}, err
	}
	order, err := s.store.OrderByID(ctx, orderID)
	if err != nil {
		return Merchant{}, Order{}, err
	}
	if order.MerchantID != m.ID {
		return Merchant{}, Order{}, ErrNotFound
	}
	return m, order, nil
}

func (s *Service) merchantOf(ctx context.Context, userID string) (Merchant, error) {
	m, err := s.store.MerchantByOwner(ctx, userID)
	if errors.Is(err, ErrNotFound) {
		// The role was granted; onboarding was not finished. Distinct from
		// "not permitted", which would send the merchant to the wrong place.
		return Merchant{}, ErrNotAMerchant
	}
	if err != nil {
		return Merchant{}, err
	}
	return m, nil
}

func active(m Merchant) error {
	if m.Status != StatusActive {
		return ErrNotActive
	}
	return nil
}

func unacceptable(status OrderStatus) error {
	if status == StatusPaymentPending {
		return ErrAwaitingPayment
	}
	return ErrNotAcceptable
}

var (
	// ErrNoSuchQueue reports a queue name outside document 072's five.
	ErrNoSuchQueue = errors.New("merchant: no such queue")
	// ErrReasonRequired reports a rejection with nothing said.
	ErrReasonRequired = errors.New("merchant: a rejection needs a reason")
	// ErrNotAcceptable reports Accept on an order that is not awaiting an answer.
	ErrNotAcceptable = errors.New("merchant: this order is not waiting to be accepted")
	// ErrAwaitingPayment reports Accept on an order whose payment has not
	// settled. Confirming it would commit the shop's stock against a payment
	// that may never arrive, and no payment surface exists to ask.
	ErrAwaitingPayment = errors.New("merchant: this order is waiting on payment")
	// ErrNotPreparable reports Start Preparing before acceptance.
	ErrNotPreparable = errors.New("merchant: accept this order before preparing it")
	// ErrNotReadyable reports Mark Ready on an order nobody is picking yet.
	ErrNotReadyable = errors.New("merchant: start preparing this order before marking it ready")
	// ErrNoDeliveries reports a deployment with no job store behind the
	// merchant surface, where marking an order ready would strand it.
	ErrNoDeliveries = errors.New("merchant: deliveries are unavailable")
)
