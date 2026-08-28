package booking

import (
	"context"
	"time"

	"github.com/sarmadkung/rideme/services/api/internal/settings"
	"github.com/sarmadkung/rideme/services/api/pkg/money"
)

// CancellationPolicy is BD-01, resolved on 2026-08-28: cancelling is free for
// two minutes after a driver accepts, and costs PKR 100 after that.
//
// Two properties of that decision are worth stating, because they are what the
// rule is for rather than incidental to it:
//
//   - The clock starts at acceptance, not at booking. A customer whose request
//     never found a driver has cost nobody anything and is never charged, no
//     matter how long they waited before giving up.
//   - The fee is charged for wasted driver effort. That is why it exists, and
//     why the grace window is measured from the moment a driver committed.
//
// The values are configuration (platform_settings), not constants here.
type CancellationPolicy struct {
	Grace time.Duration
	Fee   money.Amount
}

// LoadCancellationPolicy reads the configured policy.
//
// A missing key is an error, not a free cancellation: silently charging
// nothing because a row is absent is a revenue bug that looks like working
// software.
func LoadCancellationPolicy(ctx context.Context, s *settings.Store) (CancellationPolicy, error) {
	grace, err := s.Duration(ctx, settings.KeyCancellationGraceSeconds)
	if err != nil {
		return CancellationPolicy{}, err
	}
	feeMinor, err := s.Int(ctx, settings.KeyCancellationFeeMinor)
	if err != nil {
		return CancellationPolicy{}, err
	}
	fee, err := money.New(feeMinor, money.PKR)
	if err != nil {
		return CancellationPolicy{}, err
	}
	return CancellationPolicy{Grace: grace, Fee: fee}, nil
}

// FeeFor returns what a cancellation costs.
//
// acceptedAt is nil when no driver ever accepted. That is the common free case
// and it is checked first, so a job that never found supply cannot be charged
// by any later branch.
func (p CancellationPolicy) FeeFor(tier CancellationTier, acceptedAt *time.Time, now time.Time) (money.Amount, error) {
	free, err := money.Zero(money.PKR)
	if err != nil {
		return money.Amount{}, err
	}

	// Nobody was dispatched, so nothing was wasted.
	if tier == TierBeforeAssignment || acceptedAt == nil {
		return free, nil
	}
	// Within the grace window. The boundary itself is free: a customer told
	// they have two free minutes should not be charged at exactly two
	// minutes, so the tie goes to them.
	if !now.After(acceptedAt.Add(p.Grace)) {
		return free, nil
	}
	return p.Fee, nil
}
