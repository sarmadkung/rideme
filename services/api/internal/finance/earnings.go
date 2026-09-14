package finance

import (
	"context"
	"fmt"
	"time"

	"github.com/sarmadkung/rideme/services/api/pkg/money"
)

// Earnings is what a driver made over a period.
//
// Read from the ledger rather than from a counter kept alongside it. A second
// place recording what a driver earned is a second place that can be wrong,
// and the one that is wrong is always the one the driver is looking at.
type Earnings struct {
	// Net is what the driver keeps: gross less the platform's commission,
	// which is already deducted by the time an entry reaches DRIVER_PAYABLE
	// (see DriverEarning).
	Net money.Amount `json:"net"`
	// Trips is the number of jobs that contributed.
	Trips int       `json:"trips"`
	From  time.Time `json:"from"`
	To    time.Time `json:"to"`
}

// TripEarning is one job's contribution, for the list under the total.
//
// A driver checking their earnings is usually checking one specific trip, and
// a total with no breakdown cannot answer the question they actually have.
type TripEarning struct {
	JobID  string       `json:"job_id"`
	Amount money.Amount `json:"amount"`
	At     time.Time    `json:"at"`
}

// MaxTripEarnings bounds the list. A driver scrolls a shift, not a career, and
// an unbounded query on a busy driver is a slow one on a phone.
const MaxTripEarnings = 100

// EarningsBetween sums what reached the driver's payable account in a window.
//
// Credits are negative in the ledger, so the sum is negated to give a figure a
// driver would recognise. `from` is inclusive and `to` exclusive, so calling it
// for consecutive days counts nothing twice.
func (s *Store) EarningsBetween(ctx context.Context, driverID string, from, to time.Time) (Earnings, error) {
	if driverID == "" {
		return Earnings{}, fmt.Errorf("earnings: a driver is required")
	}
	if !to.After(from) {
		return Earnings{}, fmt.Errorf("earnings: the window ends before it starts")
	}

	var minor int64
	var trips int
	// Only DRIVER_PAYABLE: it is the account that represents money owed to the
	// driver. DRIVER_EXPENSE is the platform's side of the same movement, and
	// summing both would net to nothing.
	//
	// Reversals need no special case. A reversal writes an opposing entry, so
	// a cancelled earning is already subtracted here rather than needing to be
	// remembered and excluded.
	err := s.pool.QueryRow(ctx, `
		SELECT COALESCE(-sum(e.amount_minor), 0),
		       count(DISTINCT t.job_id)
		FROM ledger_entries e
		JOIN ledger_transactions t ON t.id = e.transaction_id
		WHERE e.account = $1
		  AND e.subject_type = $2
		  AND e.subject_id = $3
		  AND e.created_at >= $4
		  AND e.created_at < $5`,
		AccountDriverPayable, SubjectDriver, driverID, from, to,
	).Scan(&minor, &trips)
	if err != nil {
		return Earnings{}, fmt.Errorf("sum earnings: %w", err)
	}

	net, err := money.New(minor, money.PKR)
	if err != nil {
		return Earnings{}, err
	}
	return Earnings{Net: net, Trips: trips, From: from, To: to}, nil
}

// TripEarningsSince lists the individual jobs behind a total.
func (s *Store) TripEarningsSince(ctx context.Context, driverID string, since time.Time, limit int) ([]TripEarning, error) {
	if driverID == "" {
		return nil, fmt.Errorf("earnings: a driver is required")
	}
	if limit <= 0 || limit > MaxTripEarnings {
		limit = MaxTripEarnings
	}

	rows, err := s.pool.Query(ctx, `
		SELECT COALESCE(t.job_id::text, ''), -e.amount_minor, e.created_at
		FROM ledger_entries e
		JOIN ledger_transactions t ON t.id = e.transaction_id
		WHERE e.account = $1
		  AND e.subject_type = $2
		  AND e.subject_id = $3
		  AND e.created_at >= $4
		ORDER BY e.created_at DESC
		LIMIT $5`,
		AccountDriverPayable, SubjectDriver, driverID, since, limit)
	if err != nil {
		return nil, fmt.Errorf("list trip earnings: %w", err)
	}
	defer rows.Close()

	earnings := make([]TripEarning, 0, limit)
	for rows.Next() {
		var jobID string
		var minor int64
		var at time.Time
		if err := rows.Scan(&jobID, &minor, &at); err != nil {
			return nil, fmt.Errorf("scan trip earning: %w", err)
		}
		amount, err := money.New(minor, money.PKR)
		if err != nil {
			return nil, err
		}
		earnings = append(earnings, TripEarning{JobID: jobID, Amount: amount, At: at})
	}
	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("read trip earnings: %w", err)
	}
	return earnings, nil
}
