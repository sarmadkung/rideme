// Package finance holds payments, the double-entry ledger, earnings and
// settlement (documents 19, 51–64).
//
// Everything here is Level 5 by `verification-lite`'s rule, and the reason is
// not caution: a bug in this package moves real money and the mistake is
// discovered by the person who did not receive theirs.
//
// Two properties are enforced rather than intended. Every transaction balances
// to zero — checked in Go before the write and provable in SQL afterwards. And
// entries are immutable, enforced by a database trigger, so a correction is a
// reversal rather than an edit (document 53).
package finance

import (
	"errors"
	"fmt"
	"time"

	"github.com/sarmadkung/rideme/services/api/pkg/money"
)

// Account is a ledger account code (document 53).
type Account string

const (
	AccountCustomerReceivable Account = "CUSTOMER_RECEIVABLE"
	AccountCustomerWallet     Account = "CUSTOMER_WALLET"
	AccountPlatformClearing   Account = "PLATFORM_CLEARING"
	AccountPlatformRevenue    Account = "PLATFORM_REVENUE"
	AccountDriverExpense      Account = "DRIVER_EXPENSE"
	AccountDriverPayable      Account = "DRIVER_PAYABLE"
	AccountMerchantPayable    Account = "MERCHANT_PAYABLE"
	AccountTaxPayable         Account = "TAX_PAYABLE"
	AccountRefundLiability    Account = "REFUND_LIABILITY"
	AccountCashInTransit      Account = "CASH_IN_TRANSIT"
)

// SubjectType is who an entry belongs to.
type SubjectType string

const (
	SubjectCustomer SubjectType = "CUSTOMER"
	SubjectDriver   SubjectType = "DRIVER"
	SubjectMerchant SubjectType = "MERCHANT"
	SubjectPlatform SubjectType = "PLATFORM"
)

// TransactionKind classifies a movement.
type TransactionKind string

const (
	KindPayment       TransactionKind = "PAYMENT"
	KindCapture       TransactionKind = "CAPTURE"
	KindRefund        TransactionKind = "REFUND"
	KindEarning       TransactionKind = "EARNING"
	KindCommission    TransactionKind = "COMMISSION"
	KindPayout        TransactionKind = "PAYOUT"
	KindSettlement    TransactionKind = "SETTLEMENT"
	KindAdjustment    TransactionKind = "ADJUSTMENT"
	KindReversal      TransactionKind = "REVERSAL"
	KindCODCollection TransactionKind = "COD_COLLECTION"
)

// Entry is one side of a movement.
//
// The amount is signed: a debit is positive and a credit is negative. One
// signed column rather than two nullable ones makes "does this balance?" the
// question `sum() = 0` — something anyone can check and nobody can get subtly
// wrong by filling in the wrong column.
type Entry struct {
	ID            int64
	TransactionID string
	Account       Account
	Amount        money.Amount
	SubjectType   SubjectType
	SubjectID     string
	CreatedAt     time.Time
}

// Debit builds a positive entry.
func Debit(account Account, amount money.Amount, subjectType SubjectType, subjectID string) Entry {
	return Entry{Account: account, Amount: amount, SubjectType: subjectType, SubjectID: subjectID}
}

// Credit builds a negative entry.
func Credit(account Account, amount money.Amount, subjectType SubjectType, subjectID string) (Entry, error) {
	negated, err := amount.Neg()
	if err != nil {
		return Entry{}, err
	}
	return Entry{Account: account, Amount: negated, SubjectType: subjectType, SubjectID: subjectID}, nil
}

// Transaction is a balanced set of entries.
type Transaction struct {
	ID             string
	Kind           TransactionKind
	JobID          string
	OrderID        string
	IntentID       string
	ReversesID     string
	IdempotencyKey string
	Description    string
	Entries        []Entry
	CreatedAt      time.Time
}

var (
	ErrUnbalanced      = errors.New("finance: the transaction does not balance")
	ErrNoEntries       = errors.New("finance: a transaction needs at least two entries")
	ErrMixedCurrency   = errors.New("finance: a transaction cannot mix currencies")
	ErrNoCommission    = errors.New("finance: no commission rate is configured (BD-05)")
	ErrNotFound        = errors.New("finance: not found")
	ErrAlreadyCaptured = errors.New("finance: this intent is already captured")
	ErrRefundExceeds   = errors.New("finance: the refund exceeds what was captured")
	ErrBadSignature    = errors.New("finance: the webhook signature is invalid")
	ErrDuplicateEvent  = errors.New("finance: this provider event was already processed")
)

// Balance checks the invariant document 53 states: "Each transaction must
// balance."
//
// Checked in Go before the write as well as being provable in SQL afterwards.
// The database check tells you the ledger is broken; this one stops it
// breaking.
func (t Transaction) Balance() error {
	if len(t.Entries) < 2 {
		return ErrNoEntries
	}
	currency := t.Entries[0].Amount.Currency
	var sum int64
	for _, entry := range t.Entries {
		if entry.Amount.Currency != currency {
			return fmt.Errorf("%w: %s and %s", ErrMixedCurrency, currency, entry.Amount.Currency)
		}
		if entry.Amount.IsZero() {
			return errors.New("finance: a zero entry carries no information")
		}
		sum += entry.Amount.Minor
	}
	if sum != 0 {
		return fmt.Errorf("%w: entries sum to %d", ErrUnbalanced, sum)
	}
	return nil
}

// --- transaction builders ----------------------------------------------------
//
// Each is a named double entry, so the accounting shape lives in one place
// rather than being reconstructed at each call site.

// CustomerPayment records money owed by a customer becoming platform funds.
//
//	DR Customer Receivable
//	CR Platform Clearing
func CustomerPayment(amount money.Amount, customerID, jobID, intentID string) (Transaction, error) {
	credit, err := Credit(AccountPlatformClearing, amount, SubjectPlatform, "")
	if err != nil {
		return Transaction{}, err
	}
	t := Transaction{
		Kind: KindCapture, JobID: jobID, IntentID: intentID,
		Description: "customer payment captured",
		Entries: []Entry{
			Debit(AccountCustomerReceivable, amount, SubjectCustomer, customerID),
			credit,
		},
	}
	return t, t.Balance()
}

// DriverEarning records what a driver is owed for a completed job, net of
// commission.
//
//	DR Driver Expense    (gross)
//	CR Driver Payable    (net)
//	CR Platform Revenue  (commission)
//
// Gross, net and commission are supplied rather than computed here: the rate
// is configuration (BD-05) and this function must not know one.
func DriverEarning(gross, commission money.Amount, driverID, jobID string) (Transaction, error) {
	net, err := gross.Sub(commission)
	if err != nil {
		return Transaction{}, err
	}
	if net.IsNegative() {
		return Transaction{}, errors.New("finance: commission exceeds the gross earning")
	}
	payable, err := Credit(AccountDriverPayable, net, SubjectDriver, driverID)
	if err != nil {
		return Transaction{}, err
	}
	entries := []Entry{
		Debit(AccountDriverExpense, gross, SubjectDriver, driverID),
		payable,
	}
	if !commission.IsZero() {
		revenue, err := Credit(AccountPlatformRevenue, commission, SubjectPlatform, "")
		if err != nil {
			return Transaction{}, err
		}
		entries = append(entries, revenue)
	}
	t := Transaction{
		Kind: KindEarning, JobID: jobID,
		Description: "driver earning net of commission",
		Entries:     entries,
	}
	return t, t.Balance()
}

// Refund records money returned to a customer.
//
//	DR Refund Liability
//	CR Customer Receivable
func Refund(amount money.Amount, customerID, jobID, intentID string) (Transaction, error) {
	credit, err := Credit(AccountCustomerReceivable, amount, SubjectCustomer, customerID)
	if err != nil {
		return Transaction{}, err
	}
	t := Transaction{
		Kind: KindRefund, JobID: jobID, IntentID: intentID,
		Description: "refund to customer",
		Entries: []Entry{
			Debit(AccountRefundLiability, amount, SubjectPlatform, ""),
			credit,
		},
	}
	return t, t.Balance()
}

// CashCollection records cash a driver took on the platform's behalf.
//
//	DR Cash In Transit   (the driver is holding platform money)
//	CR Customer Receivable
//
// BD-09 — who bears the loss if that cash is never remitted — is a legal
// allocation and unresolved. This records the movement and allocates no
// liability, which is the honest position until someone decides.
func CashCollection(amount money.Amount, driverID, customerID, jobID string) (Transaction, error) {
	credit, err := Credit(AccountCustomerReceivable, amount, SubjectCustomer, customerID)
	if err != nil {
		return Transaction{}, err
	}
	t := Transaction{
		Kind: KindCODCollection, JobID: jobID,
		Description: "cash collected by driver",
		Entries: []Entry{
			Debit(AccountCashInTransit, amount, SubjectDriver, driverID),
			credit,
		},
	}
	return t, t.Balance()
}

// Payout records a settlement paid out.
//
//	DR Driver/Merchant Payable
//	CR Platform Clearing
func Payout(amount money.Amount, subjectType SubjectType, subjectID string) (Transaction, error) {
	account := AccountDriverPayable
	if subjectType == SubjectMerchant {
		account = AccountMerchantPayable
	}
	credit, err := Credit(AccountPlatformClearing, amount, SubjectPlatform, "")
	if err != nil {
		return Transaction{}, err
	}
	t := Transaction{
		Kind: KindPayout, Description: "payout",
		Entries: []Entry{
			Debit(account, amount, subjectType, subjectID),
			credit,
		},
	}
	return t, t.Balance()
}

// Reverse builds the exact inverse of a transaction.
//
// Document 53: "Corrections use reversal/adjustment transactions." Every entry
// is negated, so the pair sums to zero and the original stays readable — which
// is the whole reason the ledger is append-only.
func Reverse(original Transaction, reason string) (Transaction, error) {
	if original.ID == "" {
		return Transaction{}, errors.New("finance: cannot reverse an unsaved transaction")
	}
	entries := make([]Entry, 0, len(original.Entries))
	for _, entry := range original.Entries {
		negated, err := entry.Amount.Neg()
		if err != nil {
			return Transaction{}, err
		}
		entries = append(entries, Entry{
			Account: entry.Account, Amount: negated,
			SubjectType: entry.SubjectType, SubjectID: entry.SubjectID,
		})
	}
	t := Transaction{
		Kind: KindReversal, JobID: original.JobID, OrderID: original.OrderID,
		IntentID: original.IntentID, ReversesID: original.ID,
		Description: "reversal: " + reason,
		Entries:     entries,
	}
	return t, t.Balance()
}

// --- commission --------------------------------------------------------------

// CommissionRate is configuration. BD-05 is a PRODUCT_DECISION and no value
// appears anywhere in this package.
type CommissionRate struct {
	JobType     string
	SubjectType SubjectType
	RateBPS     int
	Flat        money.Amount
	Version     int
}

// Commission computes what the platform keeps from a gross earning.
//
// The rate is basis points — a 12.5% commission is 1250, never 0.125 — and the
// rounding is money's single half-away-from-zero entry point, so a commission
// and a fare round the same way.
func Commission(gross money.Amount, rate CommissionRate) (money.Amount, error) {
	percentage, err := gross.ApplyRate(int64(rate.RateBPS), 10000)
	if err != nil {
		return money.Amount{}, err
	}
	if rate.Flat.IsZero() {
		return percentage, nil
	}
	total, err := percentage.Add(rate.Flat)
	if err != nil {
		return money.Amount{}, err
	}
	// Commission can never exceed the earning; the driver would owe money for
	// working.
	if total.Minor > gross.Minor {
		return gross, nil
	}
	return total, nil
}
