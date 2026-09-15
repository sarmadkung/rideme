package payments

import (
	"context"
	"fmt"

	"github.com/jackc/pgx/v5/pgxpool"
)

type Store struct{ pool *pgxpool.Pool }

func NewStore(pool *pgxpool.Pool) *Store { return &Store{pool: pool} }

// Configured lists the methods this deployment has switched on.
//
// Rows rather than a constant, so enabling a method once credentials exist is
// an operator action instead of a deployment. The database constraint already
// refuses an enabled digital method with no provider named; the Gateway then
// refuses one whose provider is not compiled into this binary.
func (s *Store) Configured(ctx context.Context) ([]Option, error) {
	rows, err := s.pool.Query(ctx,
		`SELECT method, label, provider, sort_order
		   FROM payment_methods WHERE enabled ORDER BY sort_order`)
	if err != nil {
		return nil, fmt.Errorf("list payment methods: %w", err)
	}
	defer rows.Close()

	options := make([]Option, 0, 4)
	for rows.Next() {
		var option Option
		if err := rows.Scan(&option.Method, &option.Label, &option.Provider, &option.Order); err != nil {
			return nil, fmt.Errorf("scan payment method: %w", err)
		}
		options = append(options, option)
	}
	return options, rows.Err()
}

// SetEnabled switches a method on or off.
//
// Enabling a digital method without naming a provider is refused by the
// database, not here: a check in Go can be bypassed by the next caller, and
// this is the constraint that stops a customer being shown a button nothing
// can complete.
func (s *Store) SetEnabled(ctx context.Context, method Method, enabled bool, provider string) error {
	if !method.Valid() {
		return fmt.Errorf("%w: %q", ErrUnknownMethod, method)
	}
	_, err := s.pool.Exec(ctx,
		`UPDATE payment_methods SET enabled = $2, provider = $3, updated_at = now()
		  WHERE method = $1`, method, enabled, provider)
	if err != nil {
		return fmt.Errorf("set payment method: %w", err)
	}
	return nil
}
