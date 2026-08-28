package booking

import (
	"testing"
	"time"

	"github.com/sarmadkung/rideme/services/api/pkg/money"
)

// policy is BD-01 as decided: free for two minutes after acceptance, then
// PKR 100.
func policy(t *testing.T) CancellationPolicy {
	t.Helper()
	return CancellationPolicy{Grace: 2 * time.Minute, Fee: money.MustNew(10000, money.PKR)}
}

func TestFeeIsZeroBeforeAnyDriverAccepted(t *testing.T) {
	p := policy(t)
	now := time.Date(2026, 8, 28, 12, 0, 0, 0, time.UTC)

	// The whole point of BD-01's grace window starting at acceptance: a
	// customer who waited ten minutes and never got a driver owes nothing.
	fee, err := p.FeeFor(TierBeforeAssignment, nil, now)
	if err != nil {
		t.Fatalf("FeeFor: %v", err)
	}
	if !fee.IsZero() {
		t.Fatalf("a cancellation before assignment charged %s", fee)
	}
}

func TestFeeIsZeroWithinGraceWindow(t *testing.T) {
	p := policy(t)
	accepted := time.Date(2026, 8, 28, 12, 0, 0, 0, time.UTC)

	for _, elapsed := range []time.Duration{0, time.Second, 90 * time.Second, 119 * time.Second} {
		fee, err := p.FeeFor(TierAfterAssignment, &accepted, accepted.Add(elapsed))
		if err != nil {
			t.Fatalf("FeeFor(%s): %v", elapsed, err)
		}
		if !fee.IsZero() {
			t.Fatalf("cancelling %s after acceptance charged %s, want free", elapsed, fee)
		}
	}
}

func TestFeeAtExactGraceBoundaryIsFree(t *testing.T) {
	p := policy(t)
	accepted := time.Date(2026, 8, 28, 12, 0, 0, 0, time.UTC)

	// A tie goes to the customer. Charging at exactly 120.000s would make the
	// advertised "two free minutes" false by a millisecond.
	fee, err := p.FeeFor(TierAfterAssignment, &accepted, accepted.Add(2*time.Minute))
	if err != nil {
		t.Fatalf("FeeFor: %v", err)
	}
	if !fee.IsZero() {
		t.Fatalf("cancelling at exactly the grace boundary charged %s, want free", fee)
	}
}

func TestFeeAppliesJustAfterGraceWindow(t *testing.T) {
	p := policy(t)
	accepted := time.Date(2026, 8, 28, 12, 0, 0, 0, time.UTC)

	fee, err := p.FeeFor(TierAfterAssignment, &accepted, accepted.Add(2*time.Minute+time.Millisecond))
	if err != nil {
		t.Fatalf("FeeFor: %v", err)
	}
	if fee.Minor != 10000 {
		t.Fatalf("fee after the grace window is %d minor units, want 10000", fee.Minor)
	}
}

func TestFeeAppliesToEveryTierAfterAssignment(t *testing.T) {
	p := policy(t)
	accepted := time.Date(2026, 8, 28, 12, 0, 0, 0, time.UTC)
	late := accepted.Add(10 * time.Minute)

	for _, tier := range []CancellationTier{TierAfterAssignment, TierAfterArrival, TierAfterStart} {
		fee, err := p.FeeFor(tier, &accepted, late)
		if err != nil {
			t.Fatalf("FeeFor(%s): %v", tier, err)
		}
		if fee.Minor != 10000 {
			t.Fatalf("tier %s charged %d minor units, want 10000", tier, fee.Minor)
		}
	}
}

func TestFeeIsZeroWhenTierSaysAssignedButNothingWasAccepted(t *testing.T) {
	p := policy(t)
	now := time.Date(2026, 8, 28, 12, 0, 0, 0, time.UTC)

	// An offer that was made but never accepted: the tier reflects the job's
	// state, the missing timestamp reflects that no driver committed. Nobody
	// is owed compensation.
	fee, err := p.FeeFor(TierAfterAssignment, nil, now)
	if err != nil {
		t.Fatalf("FeeFor: %v", err)
	}
	if !fee.IsZero() {
		t.Fatalf("charged %s for an offer no driver accepted", fee)
	}
}
