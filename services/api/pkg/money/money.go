// Package money is the platform's single representation of a monetary value.
//
// BD-07 locks the representation: integer minor units, never floating point.
// Every fare, fee, commission and ledger entry in the platform is an Amount,
// and no other type may stand in for one. A float64 that holds rupees is a
// defect, not a shortcut — 0.1 + 0.2 is not 0.3, and a ledger that cannot sum
// to zero cannot be reconciled.
//
// Nothing here knows a price, a rate or a fee. This package establishes the
// money *contract*; pricing is CAP-1 and arrives with the ride slice.
package money

import (
	"encoding/json"
	"errors"
	"fmt"
	"math/big"
	"strconv"
)

// Currency is an ISO 4217 code. The platform is single-currency (BD-07); the
// field exists so an amount is never ambiguous and so a second currency is a
// schema change rather than a silent reinterpretation of every stored integer.
type Currency string

const PKR Currency = "PKR"

// minorUnits is the number of minor units in one major unit, per currency.
// PKR has 100 paisa to the rupee.
var minorUnits = map[Currency]int64{
	PKR: 100,
}

// MaxSafeMinor is the largest magnitude an Amount may carry.
//
// It is JavaScript's Number.MAX_SAFE_INTEGER, not int64's maximum. Amounts
// cross the wire as JSON numbers to TypeScript clients, where every number is
// a float64; beyond this bound a value silently loses precision on the client
// while remaining exact on the server. Rejecting the value is the only
// behaviour that keeps both sides in agreement.
//
// The bound is ~90 trillion PKR. It constrains nothing real.
const MaxSafeMinor int64 = 9007199254740991

var (
	ErrUnknownCurrency  = errors.New("money: unknown currency")
	ErrCurrencyMismatch = errors.New("money: currency mismatch")
	ErrOutOfRange       = errors.New("money: amount exceeds the safe range")
	ErrNegativeWeight   = errors.New("money: allocation weights must be non-negative")
	ErrEmptyAllocation  = errors.New("money: allocation requires at least one positive weight")
	ErrDivideByZero     = errors.New("money: rate denominator is zero")
)

// Amount is a quantity of money held as an exact count of minor units.
//
// The zero value is not a valid amount: it carries no currency. Construct with
// New or Zero so an amount without a currency cannot reach arithmetic.
type Amount struct {
	Minor    int64    `json:"amount_minor"`
	Currency Currency `json:"currency"`
}

// New builds an amount from a count of minor units — paisa for PKR.
func New(minor int64, currency Currency) (Amount, error) {
	if _, ok := minorUnits[currency]; !ok {
		return Amount{}, fmt.Errorf("%w: %q", ErrUnknownCurrency, currency)
	}
	if minor > MaxSafeMinor || minor < -MaxSafeMinor {
		return Amount{}, fmt.Errorf("%w: %d", ErrOutOfRange, minor)
	}
	return Amount{Minor: minor, Currency: currency}, nil
}

// MustNew is New for constants and tests. It panics rather than returning an
// error, so it must never be reached with runtime input.
func MustNew(minor int64, currency Currency) Amount {
	a, err := New(minor, currency)
	if err != nil {
		panic(err)
	}
	return a
}

// Zero is the additive identity in a currency.
func Zero(currency Currency) (Amount, error) { return New(0, currency) }

// Validate reports whether an amount decoded from outside is usable.
func (a Amount) Validate() error {
	if _, ok := minorUnits[a.Currency]; !ok {
		return fmt.Errorf("%w: %q", ErrUnknownCurrency, a.Currency)
	}
	if a.Minor > MaxSafeMinor || a.Minor < -MaxSafeMinor {
		return fmt.Errorf("%w: %d", ErrOutOfRange, a.Minor)
	}
	return nil
}

func (a Amount) IsZero() bool     { return a.Minor == 0 }
func (a Amount) IsNegative() bool { return a.Minor < 0 }
func (a Amount) IsPositive() bool { return a.Minor > 0 }

// Compare returns -1, 0 or 1. Amounts in different currencies are not
// comparable and the mismatch is reported rather than guessed at.
func (a Amount) Compare(b Amount) (int, error) {
	if a.Currency != b.Currency {
		return 0, fmt.Errorf("%w: %q vs %q", ErrCurrencyMismatch, a.Currency, b.Currency)
	}
	switch {
	case a.Minor < b.Minor:
		return -1, nil
	case a.Minor > b.Minor:
		return 1, nil
	default:
		return 0, nil
	}
}

func (a Amount) Add(b Amount) (Amount, error) { return a.combine(b, false) }
func (a Amount) Sub(b Amount) (Amount, error) { return a.combine(b, true) }

func (a Amount) combine(b Amount, subtract bool) (Amount, error) {
	if a.Currency != b.Currency {
		return Amount{}, fmt.Errorf("%w: %q vs %q", ErrCurrencyMismatch, a.Currency, b.Currency)
	}
	other := b.Minor
	if subtract {
		// -MinInt64 is not representable; the range check below catches it,
		// but negating first would already have overflowed.
		if other == -1<<63 {
			return Amount{}, fmt.Errorf("%w: %d", ErrOutOfRange, other)
		}
		other = -other
	}
	// Both operands are bounded by MaxSafeMinor, so the sum cannot overflow
	// int64; it can still leave the safe range, which New rejects.
	return New(a.Minor+other, a.Currency)
}

// Neg returns the amount with its sign flipped — a ledger reversal.
func (a Amount) Neg() (Amount, error) { return New(-a.Minor, a.Currency) }

// MulInt scales by a whole number, exactly. Used for quantities: three items
// at one price, not a percentage.
func (a Amount) MulInt(n int64) (Amount, error) {
	product := new(big.Int).Mul(big.NewInt(a.Minor), big.NewInt(n))
	return fromBig(product, a.Currency)
}

// ApplyRate multiplies by the rational num/den and rounds the result to a
// whole minor unit, half away from zero.
//
// Rates are expressed as integers — a 12.5% commission is ApplyRate(125, 1000),
// never 0.125. This is the only rounding entry point in the platform, which is
// what makes "round once, at the end" enforceable rather than aspirational.
//
// It does not know any rate. Callers supply one from configuration; BD-05
// (commission) and BD-01 (cancellation) remain unresolved and are not encoded
// anywhere in this package.
func (a Amount) ApplyRate(num, den int64) (Amount, error) {
	if den == 0 {
		return Amount{}, ErrDivideByZero
	}
	n := new(big.Int).Mul(big.NewInt(a.Minor), big.NewInt(num))
	d := big.NewInt(den)
	if d.Sign() < 0 {
		n.Neg(n)
		d.Neg(d)
	}
	return fromBig(divRoundHalfAway(n, d), a.Currency)
}

// Allocate splits an amount across weights so the parts sum to exactly the
// original — no minor unit is created or destroyed.
//
// Rounding each share independently would leave a remainder unaccounted for,
// which in a ledger is an unexplained imbalance. Shares are taken from a
// running cumulative total instead, so the difference lands on specific parts
// rather than vanishing.
func (a Amount) Allocate(weights []int64) ([]Amount, error) {
	total := big.NewInt(0)
	for _, w := range weights {
		if w < 0 {
			return nil, fmt.Errorf("%w: %d", ErrNegativeWeight, w)
		}
		total.Add(total, big.NewInt(w))
	}
	if total.Sign() == 0 {
		return nil, ErrEmptyAllocation
	}

	minor := big.NewInt(a.Minor)
	out := make([]Amount, len(weights))
	cumulativeWeight := big.NewInt(0)
	previous := big.NewInt(0)

	for i, w := range weights {
		cumulativeWeight.Add(cumulativeWeight, big.NewInt(w))
		// trunc(minor * cumulativeWeight / total); the differences telescope,
		// so the shares sum to minor exactly.
		acc := new(big.Int).Mul(minor, cumulativeWeight)
		acc.Quo(acc, total)

		share, err := fromBig(new(big.Int).Sub(acc, previous), a.Currency)
		if err != nil {
			return nil, err
		}
		out[i] = share
		previous = acc
	}
	return out, nil
}

// Sum adds amounts, which must share a currency. An empty sum has no currency
// to report, so the currency is supplied.
func Sum(currency Currency, amounts ...Amount) (Amount, error) {
	acc, err := Zero(currency)
	if err != nil {
		return Amount{}, err
	}
	for _, amount := range amounts {
		if acc, err = acc.Add(amount); err != nil {
			return Amount{}, err
		}
	}
	return acc, nil
}

// String renders the amount in major units for logs and operator output. It is
// a display form, never a parsing target and never a wire format.
func (a Amount) String() string {
	scale, ok := minorUnits[a.Currency]
	if !ok {
		return fmt.Sprintf("%d %s", a.Minor, a.Currency)
	}
	sign := ""
	minor := a.Minor
	if minor < 0 {
		sign = "-"
		minor = -minor
	}
	digits := len(strconv.FormatInt(scale, 10)) - 1
	return fmt.Sprintf("%s%s %s%d.%0*d", sign, a.Currency, "", minor/scale, digits, minor%scale)
}

// MarshalJSON emits the amount deterministically: the same value always
// produces the same bytes, with no floating point anywhere in the path.
func (a Amount) MarshalJSON() ([]byte, error) {
	if err := a.Validate(); err != nil {
		return nil, err
	}
	return json.Marshal(struct {
		Minor    int64    `json:"amount_minor"`
		Currency Currency `json:"currency"`
	}{a.Minor, a.Currency})
}

// UnmarshalJSON rejects an amount that is malformed, out of range or in an
// unknown currency, so an invalid amount can never enter the system by being
// decoded.
func (a *Amount) UnmarshalJSON(data []byte) error {
	var raw struct {
		Minor    *json.RawMessage `json:"amount_minor"`
		Currency Currency         `json:"currency"`
	}
	if err := json.Unmarshal(data, &raw); err != nil {
		return err
	}
	if raw.Minor == nil {
		return errors.New("money: amount_minor is required")
	}
	// Parsing the raw literal rather than a decoded number rejects, in one
	// step, every shape that is not a whole count of minor units: a quoted
	// string, a fraction, and exponent notation alike.
	minor, err := strconv.ParseInt(string(*raw.Minor), 10, 64)
	if err != nil {
		return fmt.Errorf("money: amount_minor must be a whole number of minor units: %w", err)
	}
	parsed, err := New(minor, raw.Currency)
	if err != nil {
		return err
	}
	*a = parsed
	return nil
}

func fromBig(v *big.Int, currency Currency) (Amount, error) {
	if !v.IsInt64() {
		return Amount{}, fmt.Errorf("%w: %s", ErrOutOfRange, v.String())
	}
	return New(v.Int64(), currency)
}

// divRoundHalfAway divides n by a positive d, rounding a exact half away from
// zero. Half-up on magnitude is the convention BD-07 records.
func divRoundHalfAway(n, d *big.Int) *big.Int {
	quotient, remainder := new(big.Int).QuoRem(n, d, new(big.Int))
	twice := new(big.Int).Abs(remainder)
	twice.Lsh(twice, 1)
	if twice.Cmp(d) >= 0 {
		if n.Sign() < 0 {
			quotient.Sub(quotient, big.NewInt(1))
		} else {
			quotient.Add(quotient, big.NewInt(1))
		}
	}
	return quotient
}
