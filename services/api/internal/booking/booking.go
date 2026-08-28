// Package booking is the customer workflow: quote, confirm, track, cancel
// (documents 34, 35, 36).
//
// It is the ride slice's application layer, and it is deliberately generic over
// job type. Document 04 models every service as a Job, so a booking service
// that only understood rides would have to be written again for parcels. What
// varies per service is the pricing rule set (CAP-1) and the eligibility
// requirements — both of which this package looks up rather than encodes.
package booking

import (
	"context"
	"crypto/sha256"
	"encoding/json"
	"errors"
	"fmt"
	"time"

	"github.com/sarmadkung/rideme/services/api/internal/eligibility"
	"github.com/sarmadkung/rideme/services/api/internal/jobs"
	"github.com/sarmadkung/rideme/services/api/internal/pricing"
	"github.com/sarmadkung/rideme/services/api/internal/settings"
	"github.com/sarmadkung/rideme/services/api/pkg/httpx"
	"github.com/sarmadkung/rideme/services/api/pkg/money"
	"github.com/sarmadkung/rideme/services/api/pkg/routing"
)

// CancellationTier is document 005's structure: what a cancellation costs
// depends on how far the job had progressed.
//
// The tiers are documented; the amounts are not, and BD-01 is a
// PRODUCT_DECISION that cannot be inferred. This type records which tier
// applied so the fee can be applied later without re-deriving it from a job
// whose state has since moved on.
type CancellationTier string

const (
	TierBeforeAssignment CancellationTier = "BEFORE_ASSIGNMENT"
	TierAfterAssignment  CancellationTier = "AFTER_ASSIGNMENT"
	TierAfterArrival     CancellationTier = "AFTER_ARRIVAL"
	TierAfterStart       CancellationTier = "AFTER_START"
)

// TierFor maps a job's status to its cancellation tier (document 005).
func TierFor(status jobs.Status) CancellationTier {
	switch status {
	case jobs.StatusDraft, jobs.StatusQuoted, jobs.StatusRequested, jobs.StatusSearching:
		return TierBeforeAssignment
	case jobs.StatusAssigned, jobs.StatusAccepted, jobs.StatusArriving:
		return TierAfterAssignment
	case jobs.StatusAtPickup:
		return TierAfterArrival
	default:
		return TierAfterStart
	}
}

// Cancellable reports whether a job may still be cancelled by a customer.
//
// Document 036: "After start -> normal cancellation not permitted." A trip in
// progress ends by completing or failing, not by the customer changing their
// mind halfway.
func Cancellable(status jobs.Status) bool {
	return jobs.Machine.Can(status, jobs.StatusCancelled)
}

// QuoteRequest is what a customer asks to be priced.
type QuoteRequest struct {
	JobType      jobs.Type
	VehicleType  string
	City         string
	Stops        []jobs.Stop
	Requirements []jobs.Requirement
	ScheduledAt  *time.Time
	RequestedBy  string
}

// Quote is a priced offer, stored and returned to the customer.
type Quote struct {
	ID    string
	Job   pricing.Quote
	Total money.Amount
}

// Service implements the booking workflow.
type Service struct {
	jobs    *jobs.Store
	quotes  *Store
	pricing *pricing.Engine
	routes  *routing.Service
	// settings supplies the values BD-01 decided. It is a dependency rather
	// than a package-level lookup so a test can drive the policy directly.
	settings *settings.Store
	now      func() time.Time
}

func NewService(jobStore *jobs.Store, quoteStore *Store, engine *pricing.Engine, routes *routing.Service, platformSettings *settings.Store, now func() time.Time) *Service {
	if now == nil {
		now = time.Now
	}
	return &Service{jobs: jobStore, quotes: quoteStore, pricing: engine, routes: routes, settings: platformSettings, now: now}
}

var (
	ErrQuoteExpired   = errors.New("booking: the quote has expired")
	ErrQuoteNotOwned  = errors.New("booking: the quote belongs to another customer")
	ErrQuoteUsed      = errors.New("booking: the quote has already been used")
	ErrNotCancellable = errors.New("booking: this job can no longer be cancelled")
	ErrKeyReused      = errors.New("booking: the idempotency key was reused with a different request")
)

// Quote prices a job before the customer commits to it (document 034's flow:
// requirements → validation → route estimate → eligibility → pricing → quote).
func (s *Service) Quote(ctx context.Context, req QuoteRequest) (Quote, error) {
	if !req.JobType.Valid() {
		return Quote{}, httpx.Validation("unknown service type",
			map[string]string{"job_type": string(req.JobType)})
	}
	if len(req.Stops) < 2 {
		return Quote{}, httpx.Validation("a quote needs a pickup and a destination",
			map[string]string{"stops": "at least two required"})
	}
	for _, stop := range req.Stops {
		if !stop.Location.Valid() {
			return Quote{}, httpx.Validation("a stop has an invalid location",
				map[string]string{"stops": "coordinate out of range"})
		}
	}

	// Route estimate. The mode follows the vehicle: document 095 warns that a
	// car route is not always valid for a truck.
	origin := routing.Point{Lat: req.Stops[0].Location.Latitude, Lon: req.Stops[0].Location.Longitude}
	last := req.Stops[len(req.Stops)-1]
	destination := routing.Point{Lat: last.Location.Latitude, Lon: last.Location.Longitude}

	route, err := s.routes.Route(ctx, origin, destination, routing.Options{
		Mode: routing.ModeForVehicleType(req.VehicleType),
	})
	if err != nil {
		return Quote{}, httpx.Unavailable("could not estimate the route").WithCause(err)
	}

	tariff, err := s.quotes.Tariff(ctx, string(req.JobType), req.VehicleType, req.City)
	if err != nil {
		if errors.Is(err, pricing.ErrNoTariff) {
			// Refusing is right: a quote with no configured rate would be a
			// number this platform invented.
			return Quote{}, httpx.Unavailable("this service is not available here yet")
		}
		return Quote{}, httpx.Internal("could not load pricing").WithCause(err)
	}

	// Demand (BD-02). Measured around the pickup, capped by the tariff and by
	// the platform ceiling. A failure here returns neutral rather than an
	// error: not being able to measure demand is a reason to charge the base
	// fare, never a reason to refuse to quote.
	demandBPS := s.demandBPS(ctx, req.Stops[0].Location.Latitude, req.Stops[0].Location.Longitude)

	requirements := requirementMap(req.Requirements)
	priced, err := s.pricing.Quote(pricing.Request{
		JobType:         string(req.JobType),
		VehicleType:     req.VehicleType,
		City:            req.City,
		DistanceMeters:  route.DistanceMeters,
		DurationSeconds: route.DurationSeconds,
		WeightKG:        weightOf(requirements),
		DemandBPS:       demandBPS,
		RouteConfidence: route.Confidence,
		RequestedBy:     req.RequestedBy,
	}, tariff)
	if err != nil {
		if errors.Is(err, pricing.ErrUnknownService) {
			return Quote{}, httpx.Unavailable("this service is not available yet")
		}
		return Quote{}, httpx.Internal("could not price the job").WithCause(err)
	}

	stored, err := s.quotes.SaveQuote(ctx, req.RequestedBy, priced)
	if err != nil {
		return Quote{}, httpx.Internal("could not store the quote").WithCause(err)
	}
	return Quote{ID: stored, Job: priced, Total: priced.Total}, nil
}

// CreateRequest confirms a quote into a job.
type CreateRequest struct {
	QuoteID        string
	RequesterID    string
	JobType        jobs.Type
	Stops          []jobs.Stop
	Requirements   []jobs.Requirement
	ScheduledAt    *time.Time
	IdempotencyKey string
}

// Create turns a quote into a job, once.
//
// Document 035 requires an Idempotency-Key on job creation, and document 185
// requires that "retries and duplicated messages do not create duplicate
// financial or operational effects". A customer whose network dropped
// mid-request must not find two rides on the way.
func (s *Service) Create(ctx context.Context, req CreateRequest) (jobs.Job, error) {
	fingerprint := fingerprintOf(req)

	if req.IdempotencyKey != "" {
		existing, found, err := s.quotes.LookupIdempotent(ctx, "job.create", req.IdempotencyKey, req.RequesterID, fingerprint)
		switch {
		case err != nil && errors.Is(err, ErrKeyReused):
			// The same key with different content is a client bug. Returning
			// the first result would silently discard this request.
			return jobs.Job{}, httpx.Conflict("this idempotency key was used for a different request")
		case err != nil:
			return jobs.Job{}, httpx.Internal("could not check the idempotency key").WithCause(err)
		case found:
			job, err := s.jobs.ByID(ctx, existing)
			if err != nil {
				return jobs.Job{}, httpx.Internal("could not load the existing job").WithCause(err)
			}
			return job, nil
		}
	}

	quote, err := s.quotes.QuoteByID(ctx, req.QuoteID)
	if err != nil {
		return jobs.Job{}, httpx.NotFound("quote not found")
	}
	if quote.RequestedBy != req.RequesterID {
		// Document 035 requires quote ownership be verified. Without it, one
		// customer could book at another's locked-in price.
		return jobs.Job{}, httpx.Forbidden("not permitted")
	}
	if quote.ExpiresAt != nil && !s.now().UTC().Before(*quote.ExpiresAt) {
		return jobs.Job{}, httpx.Conflict("the quote has expired, please request a new one")
	}
	if quote.Used {
		return jobs.Job{}, httpx.Conflict("this quote has already been used")
	}

	job, err := s.jobs.Create(ctx, jobs.Job{
		Type:            req.JobType,
		RequesterUserID: req.RequesterID,
		Status:          jobs.StatusRequested,
		ScheduledAt:     req.ScheduledAt,
		QuoteID:         req.QuoteID,
		Stops:           req.Stops,
		Requirements:    req.Requirements,
	}, jobs.Actor{Type: jobs.ActorCustomer, ID: req.RequesterID})
	if err != nil {
		if errors.Is(err, jobs.ErrNoStops) || errors.Is(err, jobs.ErrStopOrder) || errors.Is(err, jobs.ErrBadCoordinate) {
			return jobs.Job{}, httpx.Validation("the job is not valid", map[string]string{"stops": err.Error()})
		}
		return jobs.Job{}, httpx.Internal("could not create the job").WithCause(err)
	}

	// Price lock (document 034): the quote as it stood, stored against the
	// job. Historical prices are never recomputed from current configuration.
	if err := s.quotes.LockPrice(ctx, job.ID, quote); err != nil {
		return jobs.Job{}, httpx.Internal("could not lock the price").WithCause(err)
	}
	if req.IdempotencyKey != "" {
		if err := s.quotes.RecordIdempotent(ctx, "job.create", req.IdempotencyKey, req.RequesterID, fingerprint, job.ID); err != nil {
			return jobs.Job{}, httpx.Internal("could not record the idempotency key").WithCause(err)
		}
	}
	return job, nil
}

// Cancel ends a job, records which tier applied, and charges the configured
// fee (BD-01).
//
// The fee is computed from the job's acceptance time rather than from the tier
// alone. The tier says a driver had been assigned; only the acceptance
// timestamp says how long ago, and the two-minute grace window is measured
// from that moment.
func (s *Service) Cancel(ctx context.Context, jobID, actorID string, actorType jobs.ActorType, reason string) (jobs.Job, Cancellation, error) {
	job, err := s.jobs.ByID(ctx, jobID)
	if err != nil {
		return jobs.Job{}, Cancellation{}, httpx.NotFound("job not found")
	}
	if actorType == jobs.ActorCustomer && job.RequesterUserID != actorID {
		return jobs.Job{}, Cancellation{}, httpx.Forbidden("not permitted")
	}
	if !Cancellable(job.Status) {
		return jobs.Job{}, Cancellation{}, httpx.Conflict(fmt.Sprintf("a job that is %s cannot be cancelled", job.Status))
	}

	tier := TierFor(job.Status)

	// The fee is decided before the transition, while the assignment is still
	// readable. Cancelling clears nothing, but reading first keeps the fee a
	// function of the job as the customer left it.
	fee, err := s.cancellationFee(ctx, jobID, tier)
	if err != nil {
		return jobs.Job{}, Cancellation{}, httpx.Internal("could not determine the cancellation fee").WithCause(err)
	}

	cancelled, err := s.jobs.Transition(ctx, jobID, job.Status, jobs.StatusCancelled,
		jobs.Actor{Type: actorType, ID: actorID},
		map[string]any{"reason": reason, "tier": string(tier), "fee_minor": fee.Minor})
	if err != nil {
		if errors.Is(err, jobs.ErrStaleTransition) {
			// The job moved while we were deciding — most often a driver
			// accepting as the customer cancels.
			return jobs.Job{}, Cancellation{}, httpx.Conflict("the job changed, please retry")
		}
		return jobs.Job{}, Cancellation{}, httpx.Internal("could not cancel the job").WithCause(err)
	}

	if err := s.quotes.RecordCancellation(ctx, jobID, string(actorType), actorID, reason, string(tier), fee); err != nil {
		return jobs.Job{}, Cancellation{}, httpx.Internal("could not record the cancellation").WithCause(err)
	}
	return cancelled, Cancellation{Tier: tier, Fee: fee}, nil
}

// Cancellation is what a cancellation cost and why.
type Cancellation struct {
	Tier CancellationTier
	Fee  money.Amount
}

// cancellationFee applies the configured policy to one job.
//
// A cancellation before assignment never reaches the assignment lookup or the
// settings table: there is no driver to compensate, so there is nothing to
// configure and nothing that can fail.
func (s *Service) cancellationFee(ctx context.Context, jobID string, tier CancellationTier) (money.Amount, error) {
	if tier == TierBeforeAssignment {
		return money.Zero(money.PKR)
	}
	policy, err := LoadCancellationPolicy(ctx, s.settings)
	if err != nil {
		return money.Amount{}, err
	}
	assignment, err := s.jobs.LiveAssignment(ctx, jobID)
	if err != nil {
		if errors.Is(err, jobs.ErrNotFound) {
			// The tier says a driver was assigned but no live assignment
			// remains — a rejected or expired offer. Nobody is owed anything.
			return money.Zero(money.PKR)
		}
		return money.Amount{}, err
	}
	return policy.FeeFor(tier, assignment.AcceptedAt, s.now())
}

// StartSearching moves a confirmed job into dispatch.
//
// Dispatch itself is Phase 8. This is the handoff point: the job becomes
// SEARCHING and something else finds a driver.
func (s *Service) StartSearching(ctx context.Context, jobID string) (jobs.Job, error) {
	job, err := s.jobs.ByID(ctx, jobID)
	if err != nil {
		return jobs.Job{}, httpx.NotFound("job not found")
	}
	moved, err := s.jobs.Transition(ctx, jobID, job.Status, jobs.StatusSearching,
		jobs.Actor{Type: jobs.ActorSystem}, nil)
	if err != nil {
		return jobs.Job{}, httpx.Conflict("the job cannot start searching from " + string(job.Status))
	}
	return moved, nil
}

// Command is a driver-side lifecycle command (document 035).
type Command string

const (
	CommandArrive   Command = "arrive"
	CommandStart    Command = "start"
	CommandComplete Command = "complete"
)

// mainFlow is document 015's lifecycle in order. A driver command names a
// destination on this path; Execute walks to it one documented transition at a
// time.
//
// Walking rather than jumping is what keeps the history honest. Document 015
// requires every transition be recorded, and a job that goes ACCEPTED →
// AT_PICKUP in one write has no ARRIVING to show — so "when did the driver set
// off?" becomes unanswerable, and the intermediate states exist in the
// specification but never in the data.
var mainFlow = []jobs.Status{
	jobs.StatusDraft, jobs.StatusQuoted, jobs.StatusRequested, jobs.StatusSearching,
	jobs.StatusAssigned, jobs.StatusAccepted, jobs.StatusArriving, jobs.StatusAtPickup,
	jobs.StatusInProgress, jobs.StatusAtDropoff, jobs.StatusCompleted,
}

func flowIndex(status jobs.Status) int {
	for i, s := range mainFlow {
		if s == status {
			return i
		}
	}
	return -1
}

// commandTargets maps each driver command to the status it drives the job to
// (document 035's accept/reject/arrive/start/complete).
var commandTargets = map[Command]jobs.Status{
	CommandArrive:   jobs.StatusAtPickup,
	CommandStart:    jobs.StatusInProgress,
	CommandComplete: jobs.StatusCompleted,
}

// Execute runs a driver command against a job.
//
// Document 035: "Every command validates assignment ownership and state." A
// driver may only advance a job they actually hold — otherwise any driver
// could complete any trip.
func (s *Service) Execute(ctx context.Context, jobID, driverID string, cmd Command) (jobs.Job, error) {
	target, ok := commandTargets[cmd]
	if !ok {
		return jobs.Job{}, httpx.Validation("unknown command", map[string]string{"command": string(cmd)})
	}

	job, err := s.jobs.ByID(ctx, jobID)
	if err != nil {
		return jobs.Job{}, httpx.NotFound("job not found")
	}
	if job.AssignedDriverID == "" || job.AssignedDriverID != driverID {
		return jobs.Job{}, httpx.Forbidden("not permitted")
	}

	// Idempotency (document 036: "Repeated commands must return the
	// authoritative result without corrupting state"). A driver tapping
	// "arrived" twice on a flaky connection gets the job back, not an error.
	if job.Status == target {
		return job, nil
	}

	current, destination := flowIndex(job.Status), flowIndex(target)
	if current < 0 || destination <= current {
		return jobs.Job{}, httpx.Conflict(fmt.Sprintf("cannot %s a job that is %s", cmd, job.Status))
	}
	// A driver command may not reach back before ACCEPTED; those transitions
	// belong to booking and dispatch, not to the driver.
	if current < flowIndex(jobs.StatusAccepted) {
		return jobs.Job{}, httpx.Conflict(fmt.Sprintf("cannot %s a job that is %s", cmd, job.Status))
	}

	actor := jobs.Actor{Type: jobs.ActorDriver, ID: driverID}
	for i := current; i < destination; i++ {
		moved, err := s.jobs.Transition(ctx, jobID, mainFlow[i], mainFlow[i+1], actor,
			map[string]any{"command": string(cmd)})
		if err != nil {
			if errors.Is(err, jobs.ErrStaleTransition) {
				return jobs.Job{}, httpx.Conflict("the job changed, please retry")
			}
			return jobs.Job{}, httpx.Internal("could not run the command").WithCause(err)
		}
		job = moved
	}
	return job, nil
}

// EligibilityFor builds the requirements a candidate must satisfy for a job.
// It delegates to the shared rules; there is no second copy here.
func EligibilityFor(job jobs.Job) eligibility.Requirements {
	return eligibility.RequirementsFromJob(string(job.Type), requirementMap(job.Requirements))
}

func requirementMap(rows []jobs.Requirement) map[string]string {
	out := make(map[string]string, len(rows))
	for _, r := range rows {
		out[r.Name] = r.Value
	}
	return out
}

func weightOf(requirements map[string]string) float64 {
	req := eligibility.RequirementsFromJob("", requirements)
	if req.WeightKG == nil {
		return 0
	}
	return *req.WeightKG
}

// fingerprintOf hashes the meaningful content of a create request, so a key
// reused with different content is detectable.
func fingerprintOf(req CreateRequest) []byte {
	payload, _ := json.Marshal(struct {
		QuoteID      string             `json:"quote_id"`
		JobType      jobs.Type          `json:"job_type"`
		Stops        []jobs.Stop        `json:"stops"`
		Requirements []jobs.Requirement `json:"requirements"`
		ScheduledAt  *time.Time         `json:"scheduled_at"`
	}{req.QuoteID, req.JobType, req.Stops, req.Requirements, req.ScheduledAt})
	sum := sha256.Sum256(payload)
	return sum[:]
}

// DemandRadiusMeters is the area a quote measures demand over.
//
// It matches dispatch's first search ring: the drivers who would actually be
// offered this job are the ones whose scarcity should move its price.
const DemandRadiusMeters = 2000

// DemandLocationMaxAge bounds what counts as a live driver position.
const DemandLocationMaxAge = 2 * time.Minute

// demandBPS measures demand around a pickup and turns it into a multiplier.
//
// Every failure path returns neutral. Demand is an adjustment to a fare that is
// already correct without it, so an unreachable database or an unreadable
// settings row should cost the platform a surge it might have charged — not
// cost the customer a quote.
func (s *Service) demandBPS(ctx context.Context, lat, lon float64) int {
	policy, err := pricing.LoadDemandPolicy(ctx, s.settings)
	if err != nil {
		return pricing.NeutralBPS
	}
	supply, err := s.quotes.Supply(ctx, lat, lon, DemandRadiusMeters, DemandLocationMaxAge, s.now())
	if err != nil {
		return pricing.NeutralBPS
	}
	return policy.MultiplierBPS(supply)
}
