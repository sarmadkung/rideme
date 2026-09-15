package finance

import (
	"context"
	"errors"
	"fmt"
	"time"

	"github.com/jackc/pgx/v5"
	"github.com/sarmadkung/rideme/services/api/pkg/money"
)

// The driver credit cap (BD-09, resolved in part 2026-09-15).
//
// Cash inverts who owes whom. A driver collects the fare — and on a grocery
// delivery, the shop's money too — so the platform ends up a creditor to
// every driver on the road. The ledger has recorded that from the first
// settlement; this is the part that acts on it.
//
// The owner's decision is a cap per vehicle type, enforced by withholding
// work: a driver over their limit cannot go online and is offered nothing.
// Not by suspending them, and not by deducting from anything — they simply
// stop being given more of the platform's money to hold until they return
// what they have.

// CreditLimit is how much a driver of one vehicle type may owe.
type CreditLimit struct {
	VehicleType string       `json:"vehicle_type"`
	Cap         money.Amount `json:"cap"`
	// Warn is where the driver's app starts telling them, so being stopped is
	// never a surprise mid-shift.
	Warn    money.Amount `json:"warn"`
	Version int          `json:"-"`
}

// Standing is a driver's position against their cap.
//
// Everything a caller needs to decide and to explain, in one value: the
// refusal, the reason, and what would clear it. A check that returns only a
// boolean forces every caller to re-derive the message, and they drift.
type Standing struct {
	// Owed is what the driver is holding that belongs to the platform.
	Owed money.Amount `json:"owed"`
	// Cap is zero when none is configured, which means unlimited.
	Cap money.Amount `json:"cap"`
	// Warn is zero when no cap applies.
	Warn money.Amount `json:"warn"`
	// Blocked reports whether the driver has reached the cap.
	Blocked bool `json:"blocked"`
	// Warning reports whether they are close enough to be told.
	Warning bool `json:"warning"`
	// Clearing is what they must hand back to work again. Zero unless blocked.
	Clearing money.Amount `json:"clearing"`
	// Limited is false when no cap is configured for this driver at all.
	Limited bool `json:"limited"`
}

// ErrNoCap reports that no limit is configured for a vehicle type.
//
// Unlike ErrNoCommission this is not fatal to the caller. An unset commission
// refuses to settle, because paying a guessed rate is worse than not paying an
// earning yet. An unset cap must not refuse to let drivers work: failing
// closed would take every driver off the road at once over a missing row.
var ErrNoCap = errors.New("finance: no credit limit is configured for this vehicle type")

// OwedBy reports what a driver is currently holding for the platform.
//
// CASH_IN_TRANSIT is debited when a driver collects and credited when they
// settle, so the running sum is the balance. Reading it rather than keeping a
// counter beside it is the same choice earnings makes and for the same reason:
// a second record of what somebody owes is a second record that can be wrong,
// and the one that is wrong is the one being argued about.
func (s *Store) OwedBy(ctx context.Context, driverID string) (money.Amount, error) {
	if driverID == "" {
		return money.Amount{}, fmt.Errorf("credit: a driver is required")
	}
	var minor int64
	err := s.pool.QueryRow(ctx, `
		SELECT COALESCE(sum(amount_minor), 0)
		  FROM ledger_entries
		 WHERE account = $1 AND subject_type = $2 AND subject_id = $3`,
		AccountCashInTransit, SubjectDriver, driverID).Scan(&minor)
	if err != nil {
		return money.Amount{}, fmt.Errorf("sum what the driver holds: %w", err)
	}
	// A negative balance means the driver has handed back more than they
	// collected — an over-payment or a correction. They owe nothing, and
	// reporting a negative debt would render as a nonsense figure in their app.
	if minor < 0 {
		minor = 0
	}
	return money.New(minor, money.PKR)
}

// LimitFor loads the cap that applies to a driver.
//
// The per-driver override wins when set: who is reliable is learned rather
// than configured, and a good driver should not be held to a new joiner's
// float. The warning threshold scales with the override so the proportion a
// driver is warned at stays the same whatever their cap.
func (s *Store) LimitFor(ctx context.Context, vehicleType, driverID string) (CreditLimit, error) {
	var limit CreditLimit
	var capMinor, warnMinor int64
	err := s.pool.QueryRow(ctx, `
		SELECT vehicle_type, cap_minor, warn_minor, version
		  FROM driver_credit_limits
		 WHERE vehicle_type = $1
		   AND active_from <= now() AND (active_to IS NULL OR active_to > now())
		 ORDER BY version DESC LIMIT 1`, vehicleType).
		Scan(&limit.VehicleType, &capMinor, &warnMinor, &limit.Version)
	if errors.Is(err, pgx.ErrNoRows) {
		return CreditLimit{}, ErrNoCap
	}
	if err != nil {
		return CreditLimit{}, fmt.Errorf("load the credit limit: %w", err)
	}

	if driverID != "" {
		var override *int64
		if err := s.pool.QueryRow(ctx,
			`SELECT credit_cap_minor FROM drivers WHERE id = $1`, driverID).Scan(&override); err == nil &&
			override != nil && *override > 0 {
			// Keep the warning at the same proportion of the cap it had, so a
			// driver on a raised limit is still told before they are stopped.
			warnMinor = *override * warnMinor / capMinor
			capMinor = *override
			if warnMinor <= 0 {
				warnMinor = capMinor
			}
		}
	}

	var err2 error
	if limit.Cap, err2 = money.New(capMinor, money.PKR); err2 != nil {
		return CreditLimit{}, err2
	}
	limit.Warn, err2 = money.New(warnMinor, money.PKR)
	return limit, err2
}

// StandingOf answers "may this driver work, and what do they owe".
//
// A missing cap is not an error to the caller: Limited is false, Blocked is
// false, and the driver works. The absence is the caller's to log.
func (s *Store) StandingOf(ctx context.Context, driverID, vehicleType string) (Standing, error) {
	zero, err := money.Zero(money.PKR)
	if err != nil {
		return Standing{}, err
	}
	standing := Standing{Owed: zero, Cap: zero, Warn: zero, Clearing: zero}

	if standing.Owed, err = s.OwedBy(ctx, driverID); err != nil {
		return Standing{}, err
	}

	limit, err := s.LimitFor(ctx, vehicleType, driverID)
	if errors.Is(err, ErrNoCap) {
		return standing, nil
	}
	if err != nil {
		return Standing{}, err
	}

	standing.Limited = true
	standing.Cap, standing.Warn = limit.Cap, limit.Warn
	standing.Blocked = standing.Owed.Minor >= limit.Cap.Minor
	standing.Warning = standing.Owed.Minor >= limit.Warn.Minor

	if standing.Blocked {
		// What clears the block, not what clears the debt. Asking a driver for
		// the whole balance when a fraction would put them back on the road is
		// how a cap becomes a reason to stop driving for you.
		clearing, err := standing.Owed.Sub(limit.Warn)
		if err != nil {
			return Standing{}, err
		}
		if clearing.IsNegative() {
			clearing = zero
		}
		standing.Clearing = clearing
	}
	return standing, nil
}

// SetCreditLimit stores a cap for a vehicle type.
func (s *Store) SetCreditLimit(ctx context.Context, limit CreditLimit) error {
	if limit.Cap.Minor <= 0 || limit.Warn.Minor <= 0 || limit.Warn.Minor > limit.Cap.Minor {
		return fmt.Errorf("credit: a cap must be positive and the warning must sit at or below it")
	}
	version := limit.Version
	if version <= 0 {
		version = 1
	}
	_, err := s.pool.Exec(ctx, `
		INSERT INTO driver_credit_limits (vehicle_type, cap_minor, warn_minor, version)
		VALUES ($1,$2,$3,$4)
		ON CONFLICT (vehicle_type, version) DO UPDATE
		  SET cap_minor = EXCLUDED.cap_minor, warn_minor = EXCLUDED.warn_minor`,
		limit.VehicleType, limit.Cap.Minor, limit.Warn.Minor, version)
	if err != nil {
		return fmt.Errorf("set the credit limit: %w", err)
	}
	return nil
}

// Settlement is money a driver handed back.
type Settlement struct {
	ID         string       `json:"id"`
	DriverID   string       `json:"-"`
	Amount     money.Amount `json:"amount"`
	Method     string       `json:"method"`
	Reference  string       `json:"reference,omitempty"`
	RecordedBy string       `json:"-"`
	Note       string       `json:"note,omitempty"`
	CreatedAt  time.Time    `json:"created_at"`
}

// DriverRemittance records cash coming back from a driver.
//
//	DR Platform Clearing
//	CR Cash In Transit   (the driver is holding less of the platform's money)
//
// The exact inverse of CashCollection, which is what makes the balance a
// running sum rather than two figures to subtract.
func DriverRemittance(amount money.Amount, driverID string) (Transaction, error) {
	credit, err := Credit(AccountCashInTransit, amount, SubjectDriver, driverID)
	if err != nil {
		return Transaction{}, err
	}
	t := Transaction{
		Kind:        KindSettlement,
		Description: "cash remitted by driver",
		Entries: []Entry{
			Debit(AccountPlatformClearing, amount, SubjectPlatform, ""),
			credit,
		},
	}
	return t, t.Balance()
}

// RecordSettlement posts the remittance and records how it arrived, in one
// database transaction.
//
// Both or neither. A ledger entry with no settlement row is money nobody can
// account for; a settlement row with no ledger entry is a driver told they
// paid while the books still say they owe.
func (s *Store) RecordSettlement(ctx context.Context, set Settlement, idempotencyKey string) (Settlement, error) {
	if set.DriverID == "" || !set.Amount.IsPositive() {
		return Settlement{}, fmt.Errorf("credit: a settlement needs a driver and a positive amount")
	}

	movement, err := DriverRemittance(set.Amount, set.DriverID)
	if err != nil {
		return Settlement{}, err
	}
	movement.IdempotencyKey = idempotencyKey

	tx, err := s.pool.Begin(ctx)
	if err != nil {
		return Settlement{}, fmt.Errorf("begin: %w", err)
	}
	defer func() { _ = tx.Rollback(ctx) }()

	posted, err := postWithin(ctx, tx, movement)
	if err != nil {
		return Settlement{}, err
	}

	var recordedBy, reference, note, key any
	if set.RecordedBy != "" {
		recordedBy = set.RecordedBy
	}
	if set.Reference != "" {
		reference = set.Reference
	}
	if set.Note != "" {
		note = set.Note
	}
	if idempotencyKey != "" {
		key = idempotencyKey
	}

	err = tx.QueryRow(ctx, `
		INSERT INTO driver_settlements
		  (driver_id, amount_minor, currency, method, recorded_by, reference, transaction_id, idempotency_key, note)
		VALUES ($1,$2,$3,$4,$5,$6,$7,$8,$9)
		ON CONFLICT (idempotency_key) DO UPDATE SET driver_id = driver_settlements.driver_id
		RETURNING id::text, created_at`,
		set.DriverID, set.Amount.Minor, set.Amount.Currency, set.Method,
		recordedBy, reference, posted.ID, key, note).
		Scan(&set.ID, &set.CreatedAt)
	if err != nil {
		return Settlement{}, fmt.Errorf("record the settlement: %w", err)
	}
	if err := tx.Commit(ctx); err != nil {
		return Settlement{}, fmt.Errorf("commit: %w", err)
	}
	return set, nil
}

// SettlementsOf lists what a driver has handed back.
func (s *Store) SettlementsOf(ctx context.Context, driverID string, limit int) ([]Settlement, error) {
	if limit <= 0 || limit > 100 {
		limit = 50
	}
	rows, err := s.pool.Query(ctx, `
		SELECT id::text, amount_minor, currency, method, COALESCE(reference,''), COALESCE(note,''), created_at
		  FROM driver_settlements WHERE driver_id = $1
		 ORDER BY created_at DESC LIMIT $2`, driverID, limit)
	if err != nil {
		return nil, fmt.Errorf("list settlements: %w", err)
	}
	defer rows.Close()

	out := make([]Settlement, 0, limit)
	for rows.Next() {
		var set Settlement
		var minor int64
		var currency money.Currency
		if err := rows.Scan(&set.ID, &minor, &currency, &set.Method,
			&set.Reference, &set.Note, &set.CreatedAt); err != nil {
			return nil, fmt.Errorf("scan settlement: %w", err)
		}
		if set.Amount, err = money.New(minor, currency); err != nil {
			return nil, err
		}
		set.DriverID = driverID
		out = append(out, set)
	}
	return out, rows.Err()
}
