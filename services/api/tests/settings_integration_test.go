//go:build integration

package tests

import (
	"context"
	"errors"
	"testing"

	"github.com/jackc/pgx/v5/pgxpool"
	"github.com/sarmadkung/rideme/services/api/internal/settings"
)

func newSettings(t *testing.T) (*settings.Store, *pgxpool.Pool) {
	t.Helper()
	pool, err := pgxpool.New(context.Background(), env(t, "DATABASE_URL",
		"postgres://logistics:logistics@localhost:55432/logistics_dev?sslmode=disable"))
	if err != nil {
		t.Fatalf("connect: %v", err)
	}
	t.Cleanup(pool.Close)
	return settings.NewStore(pool), pool
}

func TestEveryDecidedValueIsConfigured(t *testing.T) {
	// The decisions of 2026-08-28, as the platform actually holds them. If a
	// migration ever drops one of these rows the mechanism that reads it
	// starts refusing, so this asserts the seed rather than trusting it.
	s, _ := newSettings(t)
	ctx := context.Background()

	want := map[string]int64{
		settings.KeyCancellationGraceSeconds:           120,   // BD-01: two free minutes
		settings.KeyCancellationFeeMinor:               10000, // BD-01: PKR 100
		settings.KeySurgeMaxBPS:                        15000, // BD-02: 1.5x ceiling
		settings.KeyDispatchSearchDeadlineSeconds:      90,    // BD-04
		settings.KeyMerchantAcceptTimeoutSeconds:       600,   // BD-12: ten minutes
		settings.KeySubstitutionCustomerPaysDifference: 1,     // BD-11
	}
	for key, expected := range want {
		got, err := s.Int(ctx, key)
		if err != nil {
			t.Fatalf("%s is not configured: %v", key, err)
		}
		if got != expected {
			t.Fatalf("%s = %d, want the decided %d", key, got, expected)
		}
	}
}

func TestAMissingSettingIsAnErrorRatherThanZero(t *testing.T) {
	// Zero is meaningful for every one of these — a zero grace window charges
	// every cancellation, a zero timeout cancels every order instantly — so
	// "absent" must not read as "zero".
	s, _ := newSettings(t)

	if _, err := s.Int(context.Background(), "nothing.decided.this"); !errors.Is(err, settings.ErrMissing) {
		t.Fatalf("an unset key returned %v, want ErrMissing", err)
	}
}

func TestAChangedSettingTakesEffectWithoutARestart(t *testing.T) {
	// The whole reason these are rows: an operator changes a value and it
	// applies. A cache that never invalidated would make configuration a lie.
	s, pool := newSettings(t)
	ctx := context.Background()

	const key = "test.roundtrip.value"
	t.Cleanup(func() {
		_, _ = pool.Exec(context.Background(), `DELETE FROM platform_settings WHERE key = $1`, key)
	})

	if err := s.Set(ctx, key, 42, "COUNT", "a value written by the test suite"); err != nil {
		t.Fatal(err)
	}
	got, err := s.Int(ctx, key)
	if err != nil {
		t.Fatal(err)
	}
	if got != 42 {
		t.Fatalf("read back %d, want 42", got)
	}

	if err := s.Set(ctx, key, 43, "COUNT", "a value written by the test suite"); err != nil {
		t.Fatal(err)
	}
	if got, err = s.Int(ctx, key); err != nil || got != 43 {
		t.Fatalf("after an update the value reads %d (%v), want 43", got, err)
	}
}
