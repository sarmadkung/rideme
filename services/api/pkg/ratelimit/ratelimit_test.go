package ratelimit_test

import (
	"context"
	"testing"
	"time"

	"github.com/sarmadkung/rideme/services/api/pkg/ratelimit"
)

func TestLimiterAllowsUpToTheLimitThenRefuses(t *testing.T) {
	limiter := ratelimit.NewMemoryLimiter()
	rule := ratelimit.Rule{Name: "otp", Limit: 3, Window: time.Minute}
	ctx := context.Background()

	for i := 1; i <= 3; i++ {
		decision, err := limiter.Allow(ctx, rule, "+923001234567")
		if err != nil {
			t.Fatal(err)
		}
		if !decision.Allowed {
			t.Fatalf("request %d should have been allowed", i)
		}
		if decision.Remaining != 3-i {
			t.Fatalf("request %d: remaining %d, want %d", i, decision.Remaining, 3-i)
		}
	}

	decision, err := limiter.Allow(ctx, rule, "+923001234567")
	if err != nil {
		t.Fatal(err)
	}
	if decision.Allowed {
		t.Fatal("the fourth request should have been refused")
	}
	if decision.RetryAfter != time.Minute {
		t.Fatalf("RetryAfter = %v, want the window", decision.RetryAfter)
	}
}

func TestLimitsAreIndependentPerKeyAndRule(t *testing.T) {
	limiter := ratelimit.NewMemoryLimiter()
	rule := ratelimit.Rule{Name: "otp", Limit: 1, Window: time.Minute}
	other := ratelimit.Rule{Name: "verify", Limit: 1, Window: time.Minute}
	ctx := context.Background()

	if d, _ := limiter.Allow(ctx, rule, "phone-a"); !d.Allowed {
		t.Fatal("first request for phone-a refused")
	}
	// A different phone must not be limited by another phone's usage.
	if d, _ := limiter.Allow(ctx, rule, "phone-b"); !d.Allowed {
		t.Fatal("phone-b was limited by phone-a's usage")
	}
	// A different rule counts separately.
	if d, _ := limiter.Allow(ctx, other, "phone-a"); !d.Allowed {
		t.Fatal("the verify rule was limited by the otp rule")
	}
	if d, _ := limiter.Allow(ctx, rule, "phone-a"); d.Allowed {
		t.Fatal("phone-a exceeded its own limit and was allowed")
	}
}

func TestWindowRollsOver(t *testing.T) {
	limiter := ratelimit.NewMemoryLimiter()
	clock := time.Date(2026, 8, 28, 12, 0, 0, 0, time.UTC)
	limiter.SetClock(func() time.Time { return clock })

	rule := ratelimit.Rule{Name: "otp", Limit: 1, Window: time.Minute}
	ctx := context.Background()

	if d, _ := limiter.Allow(ctx, rule, "key"); !d.Allowed {
		t.Fatal("first request refused")
	}
	if d, _ := limiter.Allow(ctx, rule, "key"); d.Allowed {
		t.Fatal("second request in the same window allowed")
	}

	clock = clock.Add(time.Minute)
	if d, _ := limiter.Allow(ctx, rule, "key"); !d.Allowed {
		t.Fatal("the limit did not reset after the window rolled over")
	}
}
