package dispatch_test

import (
	"testing"
	"time"

	"github.com/sarmadkung/rideme/services/api/internal/dispatch"
)

var now = time.Date(2026, 8, 28, 12, 0, 0, 0, time.UTC)

// defaultWeights mirrors the dispatch_config defaults: ETA dominant, the rest
// low but non-zero, which is what BD-03 recommends as a starting point.
func defaultWeights() dispatch.Weights {
	return dispatch.Weights{ETA: 6000, Distance: 1500, Reliability: 1000,
		Acceptance: 500, Idle: 500, Capability: 500}
}

func ptr(v float64) *float64 { return &v }

func TestNearerAndFasterCandidatesRankHigher(t *testing.T) {
	scored := dispatch.Score([]dispatch.Candidate{
		{DriverID: "far", DistanceMeters: 5000, ETASeconds: 900, ETAKnown: true},
		{DriverID: "near", DistanceMeters: 500, ETASeconds: 120, ETAKnown: true},
		{DriverID: "middle", DistanceMeters: 2000, ETASeconds: 400, ETAKnown: true},
	}, defaultWeights(), now)

	if scored[0].DriverID != "near" {
		t.Fatalf("ranked %s first, want near", scored[0].DriverID)
	}
	if scored[len(scored)-1].DriverID != "far" {
		t.Fatalf("ranked %s last, want far", scored[len(scored)-1].DriverID)
	}
}

func TestETAOutweighsDistanceWhenTheyDisagree(t *testing.T) {
	// Document 40: "Prefer estimated arrival time over straight-line distance."
	// A driver across a river is close in metres and far in minutes.
	scored := dispatch.Score([]dispatch.Candidate{
		{DriverID: "across-the-river", DistanceMeters: 500, ETASeconds: 1200, ETAKnown: true},
		{DriverID: "down-the-road", DistanceMeters: 2500, ETASeconds: 180, ETAKnown: true},
	}, defaultWeights(), now)

	if scored[0].DriverID != "down-the-road" {
		t.Fatalf("ranked %s first; straight-line distance beat ETA", scored[0].DriverID)
	}
}

func TestAnUnknownETAIsNotTreatedAsInstant(t *testing.T) {
	// Scoring a missing ETA as zero seconds would make every unroutable driver
	// the best candidate on the platform.
	scored := dispatch.Score([]dispatch.Candidate{
		{DriverID: "unknown-eta", DistanceMeters: 1000},
		{DriverID: "known-eta", DistanceMeters: 1000, ETASeconds: 300, ETAKnown: true},
	}, defaultWeights(), now)

	if scored[0].DriverID != "known-eta" {
		t.Fatalf("a driver with no ETA outranked one with a known ETA")
	}
}

func TestANewDriverIsNotPunishedForHavingNoHistory(t *testing.T) {
	// Document 40: "Do not permanently punish a driver for isolated events."
	// A driver with no record at all must be able to earn one.
	scored := dispatch.Score([]dispatch.Candidate{
		{DriverID: "new", DistanceMeters: 1000, ETASeconds: 300, ETAKnown: true},
		{DriverID: "poor", DistanceMeters: 1000, ETASeconds: 300, ETAKnown: true,
			CompletionRate: ptr(0.3), CancellationRate: ptr(0.5), AcceptanceRate: ptr(0.2)},
	}, defaultWeights(), now)

	if scored[0].DriverID != "new" {
		t.Fatalf("a new driver ranked below a demonstrably unreliable one")
	}
}

func TestReliabilityRanksAboveAnUnreliableDriver(t *testing.T) {
	scored := dispatch.Score([]dispatch.Candidate{
		{DriverID: "unreliable", DistanceMeters: 1000, ETASeconds: 300, ETAKnown: true,
			CompletionRate: ptr(0.5), CancellationRate: ptr(0.4), AcceptanceRate: ptr(0.3)},
		{DriverID: "reliable", DistanceMeters: 1000, ETASeconds: 300, ETAKnown: true,
			CompletionRate: ptr(0.98), CancellationRate: ptr(0.01), AcceptanceRate: ptr(0.95)},
	}, defaultWeights(), now)

	if scored[0].DriverID != "reliable" {
		t.Fatalf("ranked %s first", scored[0].DriverID)
	}
}

func TestIdleTimeBreaksTiesTowardsFairness(t *testing.T) {
	// Document 40's fairness term: "Prevent the same high-performing drivers
	// from receiving every job."
	scored := dispatch.Score([]dispatch.Candidate{
		{DriverID: "just-finished", DistanceMeters: 1000, ETASeconds: 300, ETAKnown: true, IdleSeconds: 10},
		{DriverID: "waiting-an-hour", DistanceMeters: 1000, ETASeconds: 300, ETAKnown: true, IdleSeconds: 3600},
	}, defaultWeights(), now)

	if scored[0].DriverID != "waiting-an-hour" {
		t.Fatalf("the driver who just finished was preferred over one idle an hour")
	}
}

func TestAnExactCapabilityMatchIsPreferred(t *testing.T) {
	// Document 40: "Exact match should receive a strong preference."
	scored := dispatch.Score([]dispatch.Candidate{
		{DriverID: "broader", DistanceMeters: 1000, ETASeconds: 300, ETAKnown: true},
		{DriverID: "exact", DistanceMeters: 1000, ETASeconds: 300, ETAKnown: true, ExactCapabilityMatch: true},
	}, defaultWeights(), now)

	if scored[0].DriverID != "exact" {
		t.Fatalf("ranked %s first", scored[0].DriverID)
	}
}

func TestWeightsActuallyChangeTheRanking(t *testing.T) {
	// BD-03 says weights are runtime configuration. If changing them did not
	// change the outcome, they would be decoration.
	candidates := []dispatch.Candidate{
		{DriverID: "fast-unreliable", DistanceMeters: 500, ETASeconds: 120, ETAKnown: true,
			CompletionRate: ptr(0.4), CancellationRate: ptr(0.4)},
		{DriverID: "slow-reliable", DistanceMeters: 4000, ETASeconds: 900, ETAKnown: true,
			CompletionRate: ptr(0.99), CancellationRate: ptr(0.0)},
	}

	etaDominant := dispatch.Score(candidates, dispatch.Weights{ETA: 9000, Reliability: 100}, now)
	if etaDominant[0].DriverID != "fast-unreliable" {
		t.Fatalf("ETA-dominant weights ranked %s first", etaDominant[0].DriverID)
	}

	reliabilityDominant := dispatch.Score(candidates, dispatch.Weights{ETA: 100, Reliability: 9000}, now)
	if reliabilityDominant[0].DriverID != "slow-reliable" {
		t.Fatalf("reliability-dominant weights ranked %s first", reliabilityDominant[0].DriverID)
	}
}

func TestScoringIsDeterministic(t *testing.T) {
	// Document 39 asks that discovery be "deterministic enough for testing".
	// A flapping order makes a dispatch complaint impossible to reproduce.
	candidates := []dispatch.Candidate{
		{DriverID: "b", DistanceMeters: 1000, ETASeconds: 300, ETAKnown: true},
		{DriverID: "a", DistanceMeters: 1000, ETASeconds: 300, ETAKnown: true},
		{DriverID: "c", DistanceMeters: 1000, ETASeconds: 300, ETAKnown: true},
	}
	first := dispatch.Score(candidates, defaultWeights(), now)
	for i := 0; i < 20; i++ {
		again := dispatch.Score(candidates, defaultWeights(), now)
		for j := range first {
			if first[j].DriverID != again[j].DriverID {
				t.Fatalf("ordering changed between runs at position %d", j)
			}
		}
	}
	// Equal scores tie-break by id, so the order is stable and explainable.
	if first[0].DriverID != "a" {
		t.Fatalf("tie-break produced %s first, want a", first[0].DriverID)
	}
}

func TestEveryScoreCarriesItsExplanation(t *testing.T) {
	// Document 40: "explain the major factors behind an assignment for
	// support/debugging". The inputs are volatile and gone by the time anyone
	// asks, so the explanation has to be captured with the decision.
	scored := dispatch.Score([]dispatch.Candidate{
		{DriverID: "d1", DistanceMeters: 1200, ETASeconds: 240, ETAKnown: true,
			CompletionRate: ptr(0.9), AcceptanceRate: ptr(0.8), IdleSeconds: 600,
			ExactCapabilityMatch: true},
	}, defaultWeights(), now)

	factors := scored[0].Factors
	for _, key := range []string{
		"eta_seconds", "eta_score", "distance_meters", "distance_score",
		"reliability_score", "acceptance_score", "idle_seconds", "idle_score",
		"capability_exact", "weights", "total",
	} {
		if _, ok := factors[key]; !ok {
			t.Errorf("%s missing from the explanation", key)
		}
	}
	if len(scored[0].FactorsJSON()) < 10 {
		t.Error("the explanation did not serialise")
	}
}

func TestScoresStayWithinRange(t *testing.T) {
	// Normalisation exists so terms in different units combine meaningfully.
	// A score outside [0,1] means a term escaped it.
	scored := dispatch.Score([]dispatch.Candidate{
		{DriverID: "extreme", DistanceMeters: 999999, ETASeconds: 99999, ETAKnown: true,
			CompletionRate: ptr(1), CancellationRate: ptr(0), AcceptanceRate: ptr(1),
			IdleSeconds: 999999, ExactCapabilityMatch: true},
		{DriverID: "zero", DistanceMeters: 0, ETASeconds: 0, ETAKnown: true,
			CompletionRate: ptr(0), CancellationRate: ptr(1), AcceptanceRate: ptr(0)},
	}, defaultWeights(), now)

	for _, s := range scored {
		if s.Score < 0 || s.Score > 1 {
			t.Fatalf("%s scored %f, outside [0,1]", s.DriverID, s.Score)
		}
	}
}

func TestEmptyCandidateSetScoresNothing(t *testing.T) {
	if scored := dispatch.Score(nil, defaultWeights(), now); len(scored) != 0 {
		t.Fatalf("scored %d candidates from an empty set", len(scored))
	}
}
