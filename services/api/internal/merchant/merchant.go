// Package merchant is the merchant platform and grocery fulfilment
// (documents 65–78).
//
// The structural decision, from document 070: "Order and delivery state remain
// separate and communicate through explicit events." An Order is the
// merchant's fulfilment — cart, acceptance, picking, ready — and it *produces*
// a delivery Job when it reaches READY_FOR_PICKUP. They are two lifecycles with
// one link.
//
// Merging them was the tempting alternative and would have put a merchant's
// preparation states into the same status column every ride uses, so a driver
// app would have to understand PREPARING and a merchant dashboard would have to
// understand ARRIVING.
package merchant

import (
	"errors"
	"time"

	"github.com/sarmadkung/rideme/services/api/pkg/money"
	"github.com/sarmadkung/rideme/services/api/pkg/statemachine"
)

// OrderStatus is document 070's lifecycle.
type OrderStatus string

const (
	StatusCart           OrderStatus = "CART"
	StatusPlaced         OrderStatus = "PLACED"
	StatusPaymentPending OrderStatus = "PAYMENT_PENDING"
	StatusConfirmed      OrderStatus = "CONFIRMED"
	StatusPreparing      OrderStatus = "PREPARING"
	StatusReadyForPickup OrderStatus = "READY_FOR_PICKUP"
	StatusPickedUp       OrderStatus = "PICKED_UP"
	StatusDelivering     OrderStatus = "DELIVERING"
	StatusDelivered      OrderStatus = "DELIVERED"
	StatusCancelled      OrderStatus = "CANCELLED"
	StatusFailed         OrderStatus = "FAILED"
)

// Machine is document 070's flow:
//
//	Cart → Place → Payment → Merchant Confirmation → Preparing → Ready
//	     → Pickup → Delivery → Delivered
//
// Merchant rejection is reachable only before preparation, which document 070
// states directly — once a picker has started, an order is cancelled with a
// reason rather than "rejected".
var Machine = statemachine.New(statemachine.Definition[OrderStatus]{
	Name:    "order",
	Initial: StatusCart,
	Transitions: map[OrderStatus][]OrderStatus{
		StatusCart:           {StatusPlaced, StatusCancelled},
		StatusPlaced:         {StatusPaymentPending, StatusConfirmed, StatusCancelled, StatusFailed},
		StatusPaymentPending: {StatusConfirmed, StatusCancelled, StatusFailed},
		StatusConfirmed:      {StatusPreparing, StatusCancelled},
		StatusPreparing:      {StatusReadyForPickup, StatusCancelled, StatusFailed},
		StatusReadyForPickup: {StatusPickedUp, StatusCancelled, StatusFailed},
		StatusPickedUp:       {StatusDelivering, StatusFailed},
		StatusDelivering:     {StatusDelivered, StatusFailed},
	},
	Terminal: []OrderStatus{StatusDelivered, StatusCancelled, StatusFailed},
})

// MerchantCancellable reports whether a merchant may still reject an order.
//
// Document 070: "Merchant rejection may occur before preparation." After
// picking has started the merchant has consumed stock and staff time, and the
// resolution is an operational one rather than a rejection.
func MerchantCancellable(status OrderStatus) bool {
	return status == StatusPlaced || status == StatusPaymentPending || status == StatusConfirmed
}

// CustomerCancellable reports whether the customer may still cancel.
//
// Document 070: "Customer cancellation rules depend on order state." Once a
// merchant has begun picking, cancelling wastes goods someone has handled.
func CustomerCancellable(status OrderStatus) bool {
	switch status {
	case StatusCart, StatusPlaced, StatusPaymentPending, StatusConfirmed:
		return true
	default:
		return false
	}
}

// SubstitutionPreference is the customer's per-item instruction (document 74).
type SubstitutionPreference string

const (
	PreferAllow      SubstitutionPreference = "ALLOW"
	PreferDoNotAllow SubstitutionPreference = "DO_NOT_ALLOW"
	PreferAsk        SubstitutionPreference = "ASK_ME"
)

// IssueAction is what the merchant proposes for an unavailable item.
type IssueAction string

const (
	ActionSubstitute IssueAction = "SUBSTITUTE"
	ActionRemove     IssueAction = "REMOVE"
	ActionAsk        IssueAction = "REQUEST_CUSTOMER_DECISION"
)

// ResolveIssue decides what may happen to an item, given the customer's
// standing preference.
//
// The customer's instruction is authoritative. A merchant proposing a
// substitution for an item marked DO_NOT_ALLOW does not get to make it — the
// item is removed instead, and the customer receives a partial order rather
// than something they explicitly refused.
func ResolveIssue(preference SubstitutionPreference, proposed IssueAction) IssueAction {
	switch preference {
	case PreferDoNotAllow:
		if proposed == ActionSubstitute {
			return ActionRemove
		}
		return proposed
	case PreferAllow:
		return proposed
	case PreferAsk:
		// The customer asked to be consulted, so a substitution becomes a
		// question rather than a decision. Removal still does not need asking:
		// it is what happens by default when nothing can be supplied.
		if proposed == ActionSubstitute {
			return ActionAsk
		}
		return proposed
	default:
		return ActionAsk
	}
}

// Merchant is a business on the platform.
type Merchant struct {
	ID          string
	OwnerUserID string
	Name        string
	Status      string
	Phone       string
	Address     string
	CreatedAt   time.Time
}

// Config holds per-merchant operational settings.
type Config struct {
	MerchantID string
	// AcceptTimeout is this merchant's own acceptance window, if it set one.
	// Nil means it uses the platform default BD-12 decided (ten minutes);
	// placing an order resolves the two in the database.
	AcceptTimeout   *time.Duration
	DefaultPrepTime *time.Duration
	AutoAccept      bool
}

// Item is one order line, with the price snapshot document 68 requires.
type Item struct {
	ID           string
	OrderID      string
	ProductID    string
	VariantID    string
	NameSnapshot string
	UnitPrice    money.Amount
	Quantity     int
	Preference   SubstitutionPreference
	Status       string
	CreatedAt    time.Time
}

// LineTotal is unit price times quantity, exactly.
func (i Item) LineTotal() (money.Amount, error) {
	return i.UnitPrice.MulInt(int64(i.Quantity))
}

// Order is a merchant fulfilment.
type Order struct {
	ID               string
	MerchantID       string
	StoreID          string
	CustomerUserID   string
	Status           OrderStatus
	JobID            string
	ItemsTotal       money.Amount
	Items            []Item
	AcceptedAt       *time.Time
	PreparationStart *time.Time
	ReadyAt          *time.Time
	ExpectedReadyAt  *time.Time
	AcceptDeadline   *time.Time
	RejectionReason  string
	CreatedAt        time.Time
	UpdatedAt        time.Time
}

// Total sums the order lines exactly.
//
// Money is integer minor units throughout (ADR-008), so this cannot drift from
// the stored total by a rounding step.
func Total(currency money.Currency, items []Item) (money.Amount, error) {
	total, err := money.Zero(currency)
	if err != nil {
		return money.Amount{}, err
	}
	for _, item := range items {
		line, err := item.LineTotal()
		if err != nil {
			return money.Amount{}, err
		}
		if total, err = total.Add(line); err != nil {
			return money.Amount{}, err
		}
	}
	return total, nil
}

// Issue is an item-level problem and its resolution (document 74).
type Issue struct {
	ID              string
	OrderID         string
	OrderItemID     string
	Reason          string
	Action          IssueAction
	SubstituteName  string
	SubstitutePrice *money.Amount
	PriceDifference *money.Amount
	Resolution      string
	CreatedAt       time.Time
}

var (
	// ErrAcceptTimeoutUnset now means a misconfigured deployment rather than
	// an undecided business question: BD-12 set the platform default, so
	// reaching this means the platform_settings row is missing entirely.
	ErrAcceptTimeoutUnset = errors.New(
		"merchant: no acceptance timeout is configured, at the merchant or the platform")
	ErrNotFound       = errors.New("merchant: not found")
	ErrNotCancellable = errors.New("merchant: this order can no longer be cancelled")
	ErrStoreClosed    = errors.New("merchant: the store is not open")
	ErrOutOfStock     = errors.New("merchant: an item is not available in the requested quantity")
	ErrStale          = errors.New("merchant: the order changed since it was read")
)

// StoreOpenAt reports whether a store's hours cover a moment.
func StoreOpenAt(hours []Hours, at time.Time) bool {
	weekday := int(at.Weekday())
	clock := at.Hour()*3600 + at.Minute()*60 + at.Second()
	for _, window := range hours {
		if window.Weekday != weekday {
			continue
		}
		if clock >= window.OpensAt && clock < window.ClosesAt {
			return true
		}
	}
	return false
}

// Hours is one opening window, in seconds from midnight.
type Hours struct {
	Weekday  int
	OpensAt  int
	ClosesAt int
}

// --- substitution pricing (BD-11) --------------------------------------------

// ResolutionAccepted is the resolution that makes a substitution real.
const (
	ResolutionPending          = "PENDING"
	ResolutionCustomerAccepted = "CUSTOMER_ACCEPTED"
	ResolutionCustomerDeclined = "CUSTOMER_DECLINED"
	ResolutionAutoApplied      = "AUTO_APPLIED"
)

// repricesLine reports whether an issue should change what the customer pays.
//
// BD-11 charges the customer the substitute's actual price, but only once the
// substitution is settled: either the customer accepted it, or their standing
// preference was ALLOW and it applied automatically. A substitution still
// awaiting an answer changes nothing.
func repricesLine(issue Issue) bool {
	if issue.Action != ActionSubstitute || issue.SubstitutePrice == nil {
		return false
	}
	return issue.Resolution == ResolutionCustomerAccepted || issue.Resolution == ResolutionAutoApplied
}

// PriceDifference is what a substitution changes for the customer.
//
// Positive means the substitute costs more and the customer pays more;
// negative means they pay less. Both directions reach the customer under
// BD-11 — the platform neither pockets a cheaper substitute nor absorbs a
// dearer one.
func PriceDifference(original, substitute money.Amount, quantity int) (money.Amount, error) {
	perUnit, err := substitute.Sub(original)
	if err != nil {
		return money.Amount{}, err
	}
	return perUnit.MulInt(int64(quantity))
}
