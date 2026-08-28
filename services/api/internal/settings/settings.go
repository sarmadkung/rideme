// Package settings holds the platform-wide values that are business decisions
// rather than engineering ones.
//
// Every value here was decided by the owner on 2026-08-28 and lives in the
// platform_settings table (migration 000011), not in Go. A rate is a row, so
// changing one is an edit with an audit trail rather than a deploy — which is
// what the business decision register asks for throughout: "build as
// configuration; values are the owner's".
//
// The refusals the mechanisms shipped with are deliberately kept. A key that is
// missing is an error, never a zero: a silently-defaulted commission pays every
// driver the wrong amount, and a silently-defaulted timeout either strands
// orders or cancels ones a merchant was about to accept.
package settings

import (
	"context"
	"errors"
	"fmt"
	"sync"
	"time"

	"github.com/jackc/pgx/v5/pgxpool"
)

// Keys, matching the rows migration 000011 seeds.
const (
	// BD-01 — cancellation.
	KeyCancellationGraceSeconds = "cancellation.grace_seconds"
	KeyCancellationFeeMinor     = "cancellation.fee_minor"

	// BD-02 — demand-triggered surge.
	KeySurgeMaxBPS    = "surge.max_bps"
	KeySurgeMinSupply = "surge.min_supply_for_surge"

	// BD-04 — how long a job may search before it expires.
	KeyDispatchSearchDeadlineSeconds = "dispatch.search_deadline_seconds"

	// BD-12 — the fallback merchant acceptance window.
	KeyMerchantAcceptTimeoutSeconds = "merchant.accept_timeout_seconds"

	// BD-11 — whether a substitution's price difference reaches the customer.
	KeySubstitutionCustomerPaysDifference = "substitution.customer_pays_difference"
)

// ErrMissing reports a key with no row.
//
// This is an error rather than a zero on purpose. Zero is a meaningful value
// for most of these — a zero grace window charges every cancellation, a zero
// timeout cancels every order instantly — so "absent" and "zero" must not be
// the same thing.
var ErrMissing = errors.New("settings: no value is configured for this key")

// Store reads platform settings, with a short cache.
//
// The values change rarely and are read on every quote, cancellation and order
// placement, so re-querying each time would be a database round trip for a
// number that has not moved in weeks. The TTL is short enough that an operator
// changing a rate sees it take effect without a restart.
type Store struct {
	pool *pgxpool.Pool
	ttl  time.Duration

	mu       sync.RWMutex
	cache    map[string]int64
	loadedAt time.Time
	// now is injectable so the cache expiry is testable without sleeping.
	now func() time.Time
}

// NewStore builds a settings store with the default cache lifetime.
func NewStore(pool *pgxpool.Pool) *Store {
	return &Store{pool: pool, ttl: 30 * time.Second, now: time.Now}
}

// Int returns a configured value.
func (s *Store) Int(ctx context.Context, key string) (int64, error) {
	if err := s.ensureFresh(ctx); err != nil {
		return 0, err
	}
	s.mu.RLock()
	value, ok := s.cache[key]
	s.mu.RUnlock()
	if !ok {
		return 0, fmt.Errorf("%w: %s", ErrMissing, key)
	}
	return value, nil
}

// Duration returns a value stored in seconds.
func (s *Store) Duration(ctx context.Context, key string) (time.Duration, error) {
	seconds, err := s.Int(ctx, key)
	if err != nil {
		return 0, err
	}
	if seconds < 0 {
		return 0, fmt.Errorf("settings: %s is negative (%d)", key, seconds)
	}
	return time.Duration(seconds) * time.Second, nil
}

// Bool returns a value stored as 0 or 1.
func (s *Store) Bool(ctx context.Context, key string) (bool, error) {
	value, err := s.Int(ctx, key)
	if err != nil {
		return false, err
	}
	return value != 0, nil
}

// Set writes a value, replacing any existing one.
//
// The unit and description are required on insert so a number in the table can
// always be read without consulting the migration that created it.
func (s *Store) Set(ctx context.Context, key string, value int64, unit, description string) error {
	_, err := s.pool.Exec(ctx,
		`INSERT INTO platform_settings (key, value, unit, description)
		 VALUES ($1, $2, $3, $4)
		 ON CONFLICT (key) DO UPDATE
		    SET value = excluded.value, updated_at = now()`,
		key, value, unit, description)
	if err != nil {
		return fmt.Errorf("set setting %s: %w", key, err)
	}
	s.Invalidate()
	return nil
}

// Invalidate drops the cache, so the next read goes to the database.
func (s *Store) Invalidate() {
	s.mu.Lock()
	s.loadedAt = time.Time{}
	s.cache = nil
	s.mu.Unlock()
}

func (s *Store) ensureFresh(ctx context.Context) error {
	s.mu.RLock()
	fresh := s.cache != nil && s.now().Sub(s.loadedAt) < s.ttl
	s.mu.RUnlock()
	if fresh {
		return nil
	}

	rows, err := s.pool.Query(ctx, `SELECT key, value FROM platform_settings`)
	if err != nil {
		return fmt.Errorf("load settings: %w", err)
	}
	defer rows.Close()

	loaded := make(map[string]int64)
	for rows.Next() {
		var key string
		var value int64
		if err := rows.Scan(&key, &value); err != nil {
			return fmt.Errorf("scan setting: %w", err)
		}
		loaded[key] = value
	}
	if err := rows.Err(); err != nil {
		return fmt.Errorf("read settings: %w", err)
	}

	s.mu.Lock()
	s.cache, s.loadedAt = loaded, s.now()
	s.mu.Unlock()
	return nil
}
