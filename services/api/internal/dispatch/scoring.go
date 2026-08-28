// Package dispatch matches jobs to drivers (documents 38–46, 49).
//
// This is the platform's highest-risk area. Two drivers holding one job is not
// a bug that produces a wrong number — it sends two people to the same
// customer and pays for both. Every guarantee here is enforced by a database
// constraint or a conditional write, never by a check that happens to run
// before an insert.
package dispatch

import (
	"encoding/json"
	"math"
	"sort"
	"time"
)

// Weights are document 40's scoring terms, in basis points.
//
// BD-03 is a TECHNICAL_DEFAULT: document 005 says the weights "should be
// configurable and learned from real outcomes later". They are loaded from
// `dispatch_config` per job type, never compiled in, and the defaults start
// with ETA dominant and the rest low but non-zero so every term is exercised
// rather than dead.
type Weights struct {
	ETA         int
	Distance    int
	Reliability int
	Acceptance  int
	Idle        int
	Capability  int
}

// Config is the per-service dispatch tuning.
type Config struct {
	JobType           string
	RadiusRings       []int
	MaxAttempts       int
	GeoCandidateLimit int
	ScoreLimit        int
	OfferTTL          time.Duration
	MaxLocationAge    time.Duration
	Weights           Weights
	StrategyVersion   int
	ScoreVersion      int
}

// Candidate is a driver being considered, with the signals document 40 names.
type Candidate struct {
	DriverID  string
	VehicleID string

	// DistanceMeters is the straight-line distance from the geo index — the
	// cheap early filter document 40 describes.
	DistanceMeters float64
	// ETASeconds comes from routing. Document 40: "Prefer estimated arrival
	// time over straight-line distance where routing data is available."
	ETASeconds int64
	// ETAKnown distinguishes a real routing answer from a missing one, so a
	// driver with no ETA is not scored as if they arrive instantly.
	ETAKnown bool

	CompletionRate   *float64
	CancellationRate *float64
	AcceptanceRate   *float64

	// IdleSeconds is how long since this driver last had work. Document 40
	// uses it to "prevent the same high-performing drivers from receiving
	// every job".
	IdleSeconds int64
	// ExactCapabilityMatch is true when the vehicle's capabilities match the
	// job without relying on a broader capability.
	ExactCapabilityMatch bool

	LocationAge time.Duration
}

// Scored is a candidate with its score and the factors that produced it.
type Scored struct {
	Candidate
	Score   float64
	Factors map[string]any
}

// FactorsJSON renders the explanation for storage.
func (s Scored) FactorsJSON() []byte {
	encoded, err := json.Marshal(s.Factors)
	if err != nil {
		return []byte("{}")
	}
	return encoded
}

// Score ranks candidates, best first.
//
// Every term is normalised to [0,1] where 1 is better, then weighted. Document
// 40 requires lower-is-better metrics be normalised before combination —
// without that, seconds of ETA and a 0–1 reliability figure would be added
// together and ETA would swamp everything by unit alone rather than by weight.
func Score(candidates []Candidate, weights Weights, now time.Time) []Scored {
	if len(candidates) == 0 {
		return nil
	}

	// Normalisation bounds come from the candidate set, so a ring where
	// everyone is far still ranks the nearest first.
	maxETA, maxDistance, maxIdle := 1.0, 1.0, 1.0
	for _, c := range candidates {
		if c.ETAKnown && float64(c.ETASeconds) > maxETA {
			maxETA = float64(c.ETASeconds)
		}
		if c.DistanceMeters > maxDistance {
			maxDistance = c.DistanceMeters
		}
		if float64(c.IdleSeconds) > maxIdle {
			maxIdle = float64(c.IdleSeconds)
		}
	}

	total := float64(weights.ETA + weights.Distance + weights.Reliability +
		weights.Acceptance + weights.Idle + weights.Capability)
	if total == 0 {
		total = 1
	}

	out := make([]Scored, 0, len(candidates))
	for _, c := range candidates {
		factors := map[string]any{}

		// ETA — lower is better. A candidate with no routing answer scores
		// zero on this term rather than one: an unknown arrival time is not
		// an instant one.
		etaScore := 0.0
		if c.ETAKnown {
			etaScore = 1 - float64(c.ETASeconds)/maxETA
		}
		factors["eta_seconds"] = c.ETASeconds
		factors["eta_known"] = c.ETAKnown
		factors["eta_score"] = round(etaScore)

		distanceScore := 1 - c.DistanceMeters/maxDistance
		factors["distance_meters"] = round(c.DistanceMeters)
		factors["distance_score"] = round(distanceScore)

		// Reliability from the rolling behaviour document 40 lists. A driver
		// with no history scores neutral, not zero — a new driver who is
		// punished for having no record never gets one.
		reliability := 0.5
		if c.CompletionRate != nil || c.CancellationRate != nil {
			completion, cancellation := 1.0, 0.0
			if c.CompletionRate != nil {
				completion = *c.CompletionRate
			}
			if c.CancellationRate != nil {
				cancellation = *c.CancellationRate
			}
			reliability = clamp(completion - cancellation)
		}
		factors["reliability_score"] = round(reliability)

		acceptance := 0.5
		if c.AcceptanceRate != nil {
			acceptance = clamp(*c.AcceptanceRate)
		}
		factors["acceptance_score"] = round(acceptance)

		// Idle — higher is better, which is the fairness term. Without it the
		// best-scoring drivers take every job and the rest of the fleet
		// starves.
		idleScore := float64(c.IdleSeconds) / maxIdle
		factors["idle_seconds"] = c.IdleSeconds
		factors["idle_score"] = round(idleScore)

		capabilityScore := 0.5
		if c.ExactCapabilityMatch {
			capabilityScore = 1
		}
		factors["capability_exact"] = c.ExactCapabilityMatch

		score := (float64(weights.ETA)*etaScore +
			float64(weights.Distance)*distanceScore +
			float64(weights.Reliability)*reliability +
			float64(weights.Acceptance)*acceptance +
			float64(weights.Idle)*idleScore +
			float64(weights.Capability)*capabilityScore) / total

		factors["weights"] = map[string]int{
			"eta": weights.ETA, "distance": weights.Distance,
			"reliability": weights.Reliability, "acceptance": weights.Acceptance,
			"idle": weights.Idle, "capability": weights.Capability,
		}
		factors["total"] = round(score)

		out = append(out, Scored{Candidate: c, Score: score, Factors: factors})
	}

	// Deterministic order: by score, then by driver id so equal scores do not
	// reorder between runs. Document 39 asks that discovery be "deterministic
	// enough for testing", and a flapping order makes a dispatch complaint
	// impossible to reproduce.
	sort.Slice(out, func(i, j int) bool {
		if out[i].Score != out[j].Score {
			return out[i].Score > out[j].Score
		}
		return out[i].DriverID < out[j].DriverID
	})
	return out
}

func clamp(v float64) float64 {
	return math.Max(0, math.Min(1, v))
}

func round(v float64) float64 {
	return math.Round(v*10000) / 10000
}
