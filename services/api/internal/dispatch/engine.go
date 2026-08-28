package dispatch

import (
	"context"
	"errors"
	"fmt"
	"log/slog"
	"time"

	"github.com/sarmadkung/rideme/services/api/internal/eligibility"
	"github.com/sarmadkung/rideme/services/api/internal/jobs"
	"github.com/sarmadkung/rideme/services/api/internal/providers"
	"github.com/sarmadkung/rideme/services/api/internal/tracking"
	"github.com/sarmadkung/rideme/services/api/pkg/routing"
)

// Engine runs document 39's candidate pipeline:
//
//	Geo Search → Availability → Capability → Vehicle → Freshness → Eligibility → Scoring
//
// The order is a cost ordering, not a preference. The geo search is bounded
// and cheap; eligibility touches the database per candidate; routing is a
// network call. Filtering before scoring is what keeps dispatch fast enough to
// run inside a customer's wait.
type Engine struct {
	store     *Store
	jobs      *jobs.Store
	providers *providers.Store
	tracking  *tracking.Store
	routes    *routing.Service
	logger    *slog.Logger
	now       func() time.Time
}

func NewEngine(
	store *Store, jobStore *jobs.Store, providerStore *providers.Store,
	trackingStore *tracking.Store, routes *routing.Service,
	logger *slog.Logger, now func() time.Time,
) *Engine {
	if now == nil {
		now = time.Now
	}
	return &Engine{
		store: store, jobs: jobStore, providers: providerStore,
		tracking: trackingStore, routes: routes, logger: logger, now: now,
	}
}

// Outcome is what one dispatch attempt produced.
type Outcome string

const (
	OutcomeOffered         Outcome = "OFFERED"
	OutcomeNoCandidates    Outcome = "NO_CANDIDATES"
	OutcomeNoEligible      Outcome = "NO_ELIGIBLE"
	OutcomeReservationLost Outcome = "RESERVATION_LOST"
	OutcomeExhausted       Outcome = "EXHAUSTED"
)

// Result describes an attempt.
type Result struct {
	Outcome       Outcome
	Attempt       int
	RadiusMeters  int
	DriverID      string
	AssignmentID  string
	ReservationID string
	Considered    int
	Eligible      int
}

// ErrNoSupply reports that every configured attempt found nobody.
//
// **BD-04 is unresolved.** Document 015 gives EXPIRED as the terminal state and
// document 044 says to "keep job searching where appropriate, notify customer,
// provide cancellation option" — but how long to search, how often to retry,
// and what the customer sees are product decisions the documentation does not
// make. The engine therefore stops after the configured attempts and reports
// this; it does **not** expire the job on its own, because choosing that
// timeout would be inventing the rule.
var ErrNoSupply = errors.New("dispatch: no eligible driver found in any configured ring")

// Dispatch runs one attempt for a job, expanding the radius with each attempt
// (document 44's retry strategy).
func (e *Engine) Dispatch(ctx context.Context, jobID string) (Result, error) {
	job, err := e.jobs.ByID(ctx, jobID)
	if err != nil {
		return Result{}, fmt.Errorf("load job: %w", err)
	}
	if job.Status != jobs.StatusSearching {
		return Result{}, fmt.Errorf("dispatch: job is %s, not SEARCHING", job.Status)
	}
	pickup, ok := job.Pickup()
	if !ok {
		return Result{}, errors.New("dispatch: job has no pickup stop")
	}

	cfg, err := e.store.Config(ctx, string(job.Type))
	if err != nil {
		return Result{}, err
	}
	previous, err := e.store.AttemptCount(ctx, jobID)
	if err != nil {
		return Result{}, err
	}
	if previous >= cfg.MaxAttempts {
		// Bounded retries (document 44: "Do not retry infinitely").
		return Result{Outcome: OutcomeExhausted, Attempt: previous}, ErrNoSupply
	}

	attempt := previous + 1
	radius := cfg.RadiusRings[min(previous, len(cfg.RadiusRings)-1)]

	requirements := eligibility.RequirementsFromJob(string(job.Type), requirementMap(job.Requirements))

	// 1. Geo search — bounded, from Redis (documents 39, 42).
	nearby, err := e.tracking.Nearby(ctx, pickup.Location.Latitude, pickup.Location.Longitude,
		float64(radius), cfg.GeoCandidateLimit)
	if err != nil {
		return Result{}, fmt.Errorf("geo search: %w", err)
	}
	if len(nearby) == 0 {
		id, rerr := e.store.RecordAttempt(ctx, jobID, attempt, radius, 0, 0, string(OutcomeNoCandidates), cfg, nil)
		if rerr != nil {
			e.logger.Error("could not record dispatch attempt", slog.String("error", rerr.Error()))
		}
		_ = id
		return Result{Outcome: OutcomeNoCandidates, Attempt: attempt, RadiusMeters: radius}, nil
	}

	// 2–6. Eligibility, through the one shared implementation. Dispatch has no
	// copy of these rules; that is CAP-5's whole point in Phase 5.
	driverIDs := make([]string, 0, len(nearby))
	byDriver := make(map[string]tracking.NearbyDriver, len(nearby))
	for _, d := range nearby {
		driverIDs = append(driverIDs, d.DriverID)
		byDriver[d.DriverID] = d
	}

	state, err := e.store.CandidateState(ctx, driverIDs, requirements.Capability)
	if err != nil {
		return Result{}, err
	}

	var candidates []Candidate
	for _, driverID := range driverIDs {
		edriver, evehicle, err := e.providers.Candidate(ctx, driverID, "PK")
		if err != nil {
			e.logger.Warn("could not load candidate", slog.String("driver_id", driverID),
				slog.String("error", err.Error()))
			continue
		}
		current, found, err := e.tracking.Current(ctx, driverID)
		if err == nil && found {
			at := current.RecordedAt
			edriver.LocationAt = &at
		}

		decision := eligibility.Evaluate(edriver, evehicle, requirements, eligibility.Options{
			Now:              e.now(),
			RequireAvailable: true,
			MaxLocationAge:   cfg.MaxLocationAge,
		})
		if !decision.Eligible {
			continue
		}

		candidate := state[driverID]
		candidate.DriverID = driverID
		candidate.DistanceMeters = byDriver[driverID].DistanceMeters
		candidates = append(candidates, candidate)
	}

	if len(candidates) == 0 {
		if _, rerr := e.store.RecordAttempt(ctx, jobID, attempt, radius, len(nearby), 0,
			string(OutcomeNoEligible), cfg, nil); rerr != nil {
			e.logger.Error("could not record dispatch attempt", slog.String("error", rerr.Error()))
		}
		return Result{Outcome: OutcomeNoEligible, Attempt: attempt, RadiusMeters: radius,
			Considered: len(nearby)}, nil
	}

	// 7. ETA for the shortlist only. Routing is the expensive call, so it runs
	// after filtering and against a bounded set (document 39's candidate limit).
	shortlist := candidates
	if len(shortlist) > cfg.ScoreLimit {
		shortlist = shortlist[:cfg.ScoreLimit]
	}
	e.attachETAs(ctx, shortlist, pickup, byDriver)

	scored := Score(shortlist, cfg.Weights, e.now())

	attemptID, err := e.store.RecordAttempt(ctx, jobID, attempt, radius, len(nearby),
		len(candidates), string(OutcomeOffered), cfg, scored)
	if err != nil {
		return Result{}, err
	}

	// 8. Reserve and offer, best first. A driver taken by another dispatcher
	// between scoring and reserving is skipped rather than failing the attempt.
	for _, candidate := range scored {
		reservationID, assignmentID, err := e.store.Reserve(ctx, jobID, candidate.DriverID,
			candidate.VehicleID, attemptID, candidate.Score, cfg.OfferTTL)
		switch {
		case err == nil:
			if _, terr := e.jobs.Transition(ctx, jobID, jobs.StatusSearching, jobs.StatusAssigned,
				jobs.Actor{Type: jobs.ActorSystem}, map[string]any{
					"driver_id": candidate.DriverID, "attempt": attempt, "score": candidate.Score,
				}); terr != nil {
				e.logger.Warn("offer created but job did not move to ASSIGNED",
					slog.String("job_id", jobID), slog.String("error", terr.Error()))
			}
			if err := e.tracking.RemoveFromPool(ctx, candidate.DriverID); err != nil {
				e.logger.Warn("could not remove driver from the pool", slog.String("error", err.Error()))
			}
			return Result{
				Outcome: OutcomeOffered, Attempt: attempt, RadiusMeters: radius,
				DriverID: candidate.DriverID, AssignmentID: assignmentID,
				ReservationID: reservationID, Considered: len(nearby), Eligible: len(candidates),
			}, nil

		case errors.Is(err, ErrDriverBusy):
			continue // another dispatcher got there first

		case errors.Is(err, ErrJobClaimed):
			// Another dispatcher is already offering this job.
			return Result{Outcome: OutcomeReservationLost, Attempt: attempt, RadiusMeters: radius}, nil

		default:
			return Result{}, err
		}
	}

	return Result{Outcome: OutcomeReservationLost, Attempt: attempt, RadiusMeters: radius,
		Considered: len(nearby), Eligible: len(candidates)}, nil
}

// attachETAs fills in routing estimates for the shortlist in one matrix call
// (document 96's Drivers × Pickup).
func (e *Engine) attachETAs(ctx context.Context, candidates []Candidate, pickup jobs.Stop, byDriver map[string]tracking.NearbyDriver) {
	if len(candidates) == 0 {
		return
	}
	origins := make([]routing.Point, len(candidates))
	for i, c := range candidates {
		d := byDriver[c.DriverID]
		origins[i] = routing.Point{Lat: d.Lat, Lon: d.Lon}
	}
	destination := []routing.Point{{Lat: pickup.Location.Latitude, Lon: pickup.Location.Longitude}}

	matrix, err := e.routes.Matrix(ctx, origins, destination, routing.Options{})
	if err != nil {
		// Scoring continues without ETA; those candidates score zero on that
		// term rather than being dropped. A routing outage should degrade
		// ranking, not stop dispatch.
		e.logger.Warn("routing matrix failed; scoring without ETA", slog.String("error", err.Error()))
		return
	}
	for _, entry := range matrix.Entries {
		if entry.OriginIndex < len(candidates) {
			candidates[entry.OriginIndex].ETASeconds = entry.DurationSeconds
			candidates[entry.OriginIndex].ETAKnown = true
		}
	}
}

func requirementMap(rows []jobs.Requirement) map[string]string {
	out := make(map[string]string, len(rows))
	for _, r := range rows {
		out[r.Name] = r.Value
	}
	return out
}

func min(a, b int) int {
	if a < b {
		return a
	}
	return b
}
