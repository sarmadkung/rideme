package pricing

import (
	"context"
	"fmt"

	"github.com/sarmadkung/rideme/services/api/internal/settings"
)

// Demand is BD-02, resolved on 2026-08-28: surge is demand-triggered and
// capped at 1.5x.
//
// The multiplier answers one question — are there more people waiting than
// there are drivers to carry them — and it answers it in basis points so no
// float enters a fare. Document 34's constraint is the important one here: "Do
// not introduce uncontrolled surge." The cap is not advisory. It is applied
// twice, once by the tariff's own demand_max_bps and once by the platform
// ceiling below, so no market can configure its way past 1.5x.
const (
	// NeutralBPS is a multiplier of exactly 1.0 — no surge.
	NeutralBPS = 10000
	// AbsoluteMaxBPS is the ceiling the platform will never exceed regardless
	// of configuration, as a last guard against a bad settings row.
	AbsoluteMaxBPS = 15000
)

// Supply is the state of one area at one moment.
type Supply struct {
	// WaitingRequests is the number of jobs currently SEARCHING nearby.
	WaitingRequests int
	// AvailableDrivers is the number of online, idle drivers nearby.
	AvailableDrivers int
}

// DemandPolicy is the configured shape of surge.
type DemandPolicy struct {
	// MaxBPS is the ceiling. 15000 is 1.5x.
	MaxBPS int
	// MinSupply is the number of available drivers below which demand is not
	// computed at all.
	//
	// This guards the degenerate case: one driver online and twenty requests
	// waiting is not a market signal, it is a quiet night or a monitoring gap,
	// and multiplying a fare on the strength of it would be surge by accident.
	MinSupply int
}

// LoadDemandPolicy reads the configured surge policy.
func LoadDemandPolicy(ctx context.Context, s *settings.Store) (DemandPolicy, error) {
	maxBPS, err := s.Int(ctx, settings.KeySurgeMaxBPS)
	if err != nil {
		return DemandPolicy{}, err
	}
	minSupply, err := s.Int(ctx, settings.KeySurgeMinSupply)
	if err != nil {
		return DemandPolicy{}, err
	}
	if maxBPS < NeutralBPS {
		return DemandPolicy{}, fmt.Errorf("pricing: surge ceiling %d is below 1.0x", maxBPS)
	}
	if maxBPS > AbsoluteMaxBPS {
		// A settings row cannot raise the platform ceiling. Clamping rather
		// than erroring keeps quoting alive through a bad edit, which is the
		// safer failure: the customer is charged less than the row asked for,
		// never more.
		maxBPS = AbsoluteMaxBPS
	}
	return DemandPolicy{MaxBPS: int(maxBPS), MinSupply: int(minSupply)}, nil
}

// MultiplierBPS returns the demand multiplier for an area.
//
// The curve is deliberately plain: the multiplier is the ratio of waiting
// requests to available drivers, clamped to [1.0, cap]. Ratios at or below 1.0
// — as many drivers as requests, or more — are not surge and return neutral.
//
// Integer arithmetic throughout. The ratio is computed in basis points before
// the division so that, for example, 3 requests against 2 drivers gives 15000
// rather than a truncated 1.
func (p DemandPolicy) MultiplierBPS(s Supply) int {
	if s.AvailableDrivers < p.MinSupply || s.AvailableDrivers <= 0 {
		return NeutralBPS
	}
	if s.WaitingRequests <= s.AvailableDrivers {
		return NeutralBPS
	}
	ratio := s.WaitingRequests * NeutralBPS / s.AvailableDrivers
	if ratio > p.MaxBPS {
		return p.MaxBPS
	}
	return ratio
}
