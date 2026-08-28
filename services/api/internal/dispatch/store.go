package dispatch

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"time"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"
	"github.com/sarmadkung/rideme/services/api/internal/jobs"
)

type Store struct{ pool *pgxpool.Pool }

func NewStore(pool *pgxpool.Pool) *Store { return &Store{pool: pool} }

var (
	ErrDriverBusy      = errors.New("dispatch: the driver already holds a reservation")
	ErrJobClaimed      = errors.New("dispatch: the job already has a live assignment")
	ErrReservationLost = errors.New("dispatch: the reservation is no longer valid")
	ErrOfferNotFound   = errors.New("dispatch: no live offer for this driver and job")
	ErrNotEligible     = errors.New("dispatch: the driver is no longer eligible")
)

// Config loads the dispatch tuning for a job type.
func (s *Store) Config(ctx context.Context, jobType string) (Config, error) {
	var c Config
	var offerTTL, locationAge int
	err := s.pool.QueryRow(ctx,
		`SELECT job_type, radius_rings, max_attempts, geo_candidate_limit, score_limit,
		        offer_ttl_seconds, max_location_age_seconds,
		        weight_eta_bps, weight_distance_bps, weight_reliability_bps,
		        weight_acceptance_bps, weight_idle_bps, weight_capability_bps,
		        strategy_version, score_version
		   FROM dispatch_config WHERE job_type = $1`, jobType).
		Scan(&c.JobType, &c.RadiusRings, &c.MaxAttempts, &c.GeoCandidateLimit, &c.ScoreLimit,
			&offerTTL, &locationAge,
			&c.Weights.ETA, &c.Weights.Distance, &c.Weights.Reliability,
			&c.Weights.Acceptance, &c.Weights.Idle, &c.Weights.Capability,
			&c.StrategyVersion, &c.ScoreVersion)
	if errors.Is(err, pgx.ErrNoRows) {
		return Config{}, fmt.Errorf("dispatch: no configuration for %s", jobType)
	}
	if err != nil {
		return Config{}, fmt.Errorf("load dispatch config: %w", err)
	}
	c.OfferTTL = time.Duration(offerTTL) * time.Second
	c.MaxLocationAge = time.Duration(locationAge) * time.Second
	return c, nil
}

// RecordAttempt stores one dispatch attempt and the scores behind it.
func (s *Store) RecordAttempt(ctx context.Context, jobID string, attempt, radius, geoCount, eligibleCount int, outcome string, cfg Config, scored []Scored) (string, error) {
	tx, err := s.pool.Begin(ctx)
	if err != nil {
		return "", fmt.Errorf("begin: %w", err)
	}
	defer func() { _ = tx.Rollback(ctx) }()

	var id string
	err = tx.QueryRow(ctx,
		`INSERT INTO dispatch_attempts
		   (job_id, attempt, radius_meters, geo_candidates, eligible_candidates,
		    outcome, strategy_version, score_version)
		 VALUES ($1,$2,$3,$4,$5,$6,$7,$8) RETURNING id::text`,
		jobID, attempt, radius, geoCount, eligibleCount, outcome, cfg.StrategyVersion, cfg.ScoreVersion).
		Scan(&id)
	if err != nil {
		return "", fmt.Errorf("record dispatch attempt: %w", err)
	}

	for rank, candidate := range scored {
		if _, err := tx.Exec(ctx,
			`INSERT INTO dispatch_scores (attempt_id, driver_id, rank, score, factors)
			 VALUES ($1, $2, $3, $4, $5)`,
			id, candidate.DriverID, rank+1, candidate.Score, candidate.FactorsJSON()); err != nil {
			return "", fmt.Errorf("record dispatch score: %w", err)
		}
	}
	if err := tx.Commit(ctx); err != nil {
		return "", fmt.Errorf("commit: %w", err)
	}
	return id, nil
}

// Reserve holds a driver for a job and creates the offer, atomically.
//
// Document 43 sequences this before the offer is sent, and document 46 gives
// the invariant: one active reservation per driver. Both are enforced here by
// unique indexes rather than by a preceding SELECT, because the window between
// checking and inserting is exactly where two dispatchers collide.
func (s *Store) Reserve(ctx context.Context, jobID, driverID, vehicleID, attemptID string, score float64, ttl time.Duration) (reservationID, assignmentID string, err error) {
	tx, err := s.pool.Begin(ctx)
	if err != nil {
		return "", "", fmt.Errorf("begin: %w", err)
	}
	defer func() { _ = tx.Rollback(ctx) }()

	expiresAt := time.Now().UTC().Add(ttl)

	err = tx.QueryRow(ctx,
		`INSERT INTO driver_reservations (job_id, driver_id, expires_at)
		 VALUES ($1, $2, $3) RETURNING id::text`,
		jobID, driverID, expiresAt).Scan(&reservationID)
	if err != nil {
		if isUnique(err) {
			return "", "", ErrDriverBusy
		}
		return "", "", fmt.Errorf("reserve driver: %w", err)
	}

	var vehicle, attempt any
	if vehicleID != "" {
		vehicle = vehicleID
	}
	if attemptID != "" {
		attempt = attemptID
	}
	err = tx.QueryRow(ctx,
		`INSERT INTO assignments (job_id, driver_id, vehicle_id, status, expires_at, attempt_id, score)
		 VALUES ($1, $2, $3, 'OFFERED', $4, $5, $6) RETURNING id::text`,
		jobID, driverID, vehicle, expiresAt, attempt, score).Scan(&assignmentID)
	if err != nil {
		if isUnique(err) {
			return "", "", ErrJobClaimed
		}
		return "", "", fmt.Errorf("create offer: %w", err)
	}

	if _, err := tx.Exec(ctx,
		`UPDATE driver_reservations SET assignment_id = $2 WHERE id = $1`,
		reservationID, assignmentID); err != nil {
		return "", "", fmt.Errorf("link reservation: %w", err)
	}
	// Only when the offer came from a recorded attempt; direct reservations
	// (tests, operator overrides) have none, and casting an empty string to
	// uuid would abort the transaction.
	if attemptID != "" {
		if _, err := tx.Exec(ctx,
			`UPDATE dispatch_attempts SET offered_driver_id = $2 WHERE id = $1`,
			attemptID, driverID); err != nil {
			return "", "", fmt.Errorf("record offered driver: %w", err)
		}
	}

	if err := tx.Commit(ctx); err != nil {
		return "", "", fmt.Errorf("commit: %w", err)
	}
	return reservationID, assignmentID, nil
}

// Accept is document 43's atomic acceptance, in one transaction:
//
//	BEGIN
//	  verify offer
//	  verify reservation
//	  verify driver eligibility
//	  assign job
//	  consume reservation
//	COMMIT
//
// Every step is a conditional write. If any of them affects no row, the driver
// did not win and the transaction rolls back — which is what makes double
// acceptance impossible rather than merely unlikely.
func (s *Store) Accept(ctx context.Context, jobID, driverID string) (jobs.Assignment, error) {
	tx, err := s.pool.Begin(ctx)
	if err != nil {
		return jobs.Assignment{}, fmt.Errorf("begin: %w", err)
	}
	defer func() { _ = tx.Rollback(ctx) }()

	// Verify and consume the offer. The status predicate means a second
	// acceptance finds nothing to update.
	var assignmentID string
	var expiresAt *time.Time
	err = tx.QueryRow(ctx,
		`UPDATE assignments
		    SET status = 'ACCEPTED', responded_at = now(), accepted_at = now()
		  WHERE job_id = $1 AND driver_id = $2 AND status = 'OFFERED'
		    AND (expires_at IS NULL OR expires_at > now())
		  RETURNING id::text, expires_at`,
		jobID, driverID).Scan(&assignmentID, &expiresAt)
	if errors.Is(err, pgx.ErrNoRows) {
		// Either already answered, expired, or never offered. All are "you did
		// not win", and the driver's client should refresh rather than retry.
		return jobs.Assignment{}, ErrOfferNotFound
	}
	if err != nil {
		return jobs.Assignment{}, fmt.Errorf("accept offer: %w", err)
	}

	// Consume the reservation.
	tag, err := tx.Exec(ctx,
		`UPDATE driver_reservations SET state = 'CONSUMED'
		  WHERE job_id = $1 AND driver_id = $2 AND state = 'RESERVED' AND expires_at > now()`,
		jobID, driverID)
	if err != nil {
		return jobs.Assignment{}, fmt.Errorf("consume reservation: %w", err)
	}
	if tag.RowsAffected() != 1 {
		return jobs.Assignment{}, ErrReservationLost
	}

	// Re-verify eligibility inside the transaction. Document 43: "The driver
	// must still pass an authoritative availability check." A driver suspended
	// between the offer and the tap must not win the job.
	var eligible bool
	err = tx.QueryRow(ctx,
		`SELECT EXISTS (
		   SELECT 1 FROM drivers d
		   JOIN vehicles v ON v.id = d.active_vehicle_id
		    WHERE d.id = $1
		      AND d.verification_status = 'APPROVED'
		      AND d.status NOT IN ('SUSPENDED', 'BLOCKED', 'OFFLINE')
		      AND v.verification_status = 'VERIFIED')`,
		driverID).Scan(&eligible)
	if err != nil {
		return jobs.Assignment{}, fmt.Errorf("verify eligibility: %w", err)
	}
	if !eligible {
		return jobs.Assignment{}, ErrNotEligible
	}

	// Assign the job. The status predicate serialises this against any other
	// transition on the same job.
	tag, err = tx.Exec(ctx,
		`UPDATE jobs SET assigned_driver_id = $2,
		        assigned_vehicle_id = (SELECT active_vehicle_id FROM drivers WHERE id = $2),
		        status = 'ACCEPTED', updated_at = now()
		  WHERE id = $1 AND status IN ('SEARCHING', 'ASSIGNED')`,
		jobID, driverID)
	if err != nil {
		return jobs.Assignment{}, fmt.Errorf("assign job: %w", err)
	}
	if tag.RowsAffected() != 1 {
		return jobs.Assignment{}, ErrJobClaimed
	}

	if _, err := tx.Exec(ctx,
		`INSERT INTO job_status_history (job_id, from_status, to_status, actor_type, actor_id, metadata)
		 SELECT $1, 'ASSIGNED', 'ACCEPTED', 'DRIVER', d.user_id, '{"source":"dispatch"}'::jsonb
		   FROM drivers d WHERE d.id = $2`,
		jobID, driverID); err != nil {
		return jobs.Assignment{}, fmt.Errorf("record history: %w", err)
	}

	// The driver leaves the availability pool by state; the geo index is
	// cleared by the caller, which owns Redis.
	if _, err := tx.Exec(ctx,
		`UPDATE drivers SET status = 'ACCEPTED', updated_at = now()
		  WHERE id = $1 AND status IN ('AVAILABLE', 'OFFERED')`, driverID); err != nil {
		return jobs.Assignment{}, fmt.Errorf("update driver state: %w", err)
	}

	if err := tx.Commit(ctx); err != nil {
		return jobs.Assignment{}, fmt.Errorf("commit: %w", err)
	}
	return jobs.Assignment{
		ID: assignmentID, JobID: jobID, DriverID: driverID,
		Status: jobs.AssignmentAccepted, ExpiresAt: expiresAt,
	}, nil
}

// Reject releases an offer the driver declined.
//
// Document 45: rejection "releases the assignment and returns the job to
// SEARCHING. Never silently overwrite assignment history." The assignment row
// stays; only its status changes.
func (s *Store) Reject(ctx context.Context, jobID, driverID string) error {
	return s.release(ctx, jobID, driverID, "REJECTED", "RELEASED")
}

// ExpireOffer releases an offer nobody answered (document 43's timeout:
// offer → EXPIRED, reservation → RELEASED, job → SEARCHING).
func (s *Store) ExpireOffer(ctx context.Context, jobID, driverID string) error {
	return s.release(ctx, jobID, driverID, "EXPIRED", "EXPIRED")
}

func (s *Store) release(ctx context.Context, jobID, driverID, assignmentState, reservationState string) error {
	tx, err := s.pool.Begin(ctx)
	if err != nil {
		return fmt.Errorf("begin: %w", err)
	}
	defer func() { _ = tx.Rollback(ctx) }()

	tag, err := tx.Exec(ctx,
		`UPDATE assignments SET status = $3, responded_at = now()
		  WHERE job_id = $1 AND driver_id = $2 AND status = 'OFFERED'`,
		jobID, driverID, assignmentState)
	if err != nil {
		return fmt.Errorf("release assignment: %w", err)
	}
	if tag.RowsAffected() != 1 {
		// Already answered — most often the driver accepted as the timeout
		// fired. Not an error: the accepted state is authoritative.
		return ErrOfferNotFound
	}

	if _, err := tx.Exec(ctx,
		`UPDATE driver_reservations SET state = $3, released_at = now()
		  WHERE job_id = $1 AND driver_id = $2 AND state = 'RESERVED'`,
		jobID, driverID, reservationState); err != nil {
		return fmt.Errorf("release reservation: %w", err)
	}

	// The driver becomes available again. Without this a declined offer leaves
	// them stuck as OFFERED and they receive no further work.
	if _, err := tx.Exec(ctx,
		`UPDATE drivers SET status = 'AVAILABLE', updated_at = now()
		  WHERE id = $1 AND status = 'OFFERED'`, driverID); err != nil {
		return fmt.Errorf("release driver: %w", err)
	}

	// The job returns to SEARCHING so the next attempt can find it.
	if _, err := tx.Exec(ctx,
		`UPDATE jobs SET status = 'SEARCHING', updated_at = now()
		  WHERE id = $1 AND status IN ('ASSIGNED', 'SEARCHING')`, jobID); err != nil {
		return fmt.Errorf("return job to searching: %w", err)
	}

	return tx.Commit(ctx)
}

// SweepExpired releases every reservation and offer past its expiry.
//
// A timeout that depends on a timer in one process is a timeout that stops
// working when that process restarts. This sweep is the durable backstop.
func (s *Store) SweepExpired(ctx context.Context, now time.Time) (int64, error) {
	tx, err := s.pool.Begin(ctx)
	if err != nil {
		return 0, fmt.Errorf("begin: %w", err)
	}
	defer func() { _ = tx.Rollback(ctx) }()

	tag, err := tx.Exec(ctx,
		`UPDATE assignments SET status = 'EXPIRED', responded_at = now()
		  WHERE status = 'OFFERED' AND expires_at IS NOT NULL AND expires_at <= $1`, now)
	if err != nil {
		return 0, fmt.Errorf("expire offers: %w", err)
	}
	expired := tag.RowsAffected()

	if _, err := tx.Exec(ctx,
		`UPDATE driver_reservations SET state = 'EXPIRED', released_at = now()
		  WHERE state = 'RESERVED' AND expires_at <= $1`, now); err != nil {
		return 0, fmt.Errorf("expire reservations: %w", err)
	}
	if _, err := tx.Exec(ctx,
		`UPDATE drivers SET status = 'AVAILABLE', updated_at = now()
		  WHERE status = 'OFFERED'
		    AND NOT EXISTS (SELECT 1 FROM driver_reservations r
		                     WHERE r.driver_id = drivers.id AND r.state = 'RESERVED')`); err != nil {
		return 0, fmt.Errorf("release drivers: %w", err)
	}
	if _, err := tx.Exec(ctx,
		`UPDATE jobs SET status = 'SEARCHING', updated_at = now()
		  WHERE status = 'ASSIGNED'
		    AND NOT EXISTS (SELECT 1 FROM assignments a
		                     WHERE a.job_id = jobs.id AND a.status IN ('OFFERED','ACCEPTED'))`); err != nil {
		return 0, fmt.Errorf("return jobs to searching: %w", err)
	}

	if err := tx.Commit(ctx); err != nil {
		return 0, fmt.Errorf("commit: %w", err)
	}
	return expired, nil
}

// CandidateState loads the provider-side signals scoring needs, for a set of
// drivers, in one query.
func (s *Store) CandidateState(ctx context.Context, driverIDs []string, capability string) (map[string]Candidate, error) {
	if len(driverIDs) == 0 {
		return map[string]Candidate{}, nil
	}
	rows, err := s.pool.Query(ctx,
		`SELECT d.id::text, COALESCE(d.active_vehicle_id::text, ''),
		        d.completion_rate, d.cancellation_rate, d.acceptance_rate,
		        EXTRACT(EPOCH FROM (now() - COALESCE(
		          (SELECT max(a.completed_at) FROM assignments a WHERE a.driver_id = d.id),
		          d.created_at)))::bigint AS idle_seconds,
		        EXISTS (SELECT 1 FROM vehicle_capabilities vc
		                 WHERE vc.vehicle_id = d.active_vehicle_id AND vc.capability = $2) AS exact_match
		   FROM drivers d
		  WHERE d.id = ANY($1::uuid[])`,
		driverIDs, capability)
	if err != nil {
		return nil, fmt.Errorf("load candidate state: %w", err)
	}
	defer rows.Close()

	out := make(map[string]Candidate, len(driverIDs))
	for rows.Next() {
		var c Candidate
		if err := rows.Scan(&c.DriverID, &c.VehicleID, &c.CompletionRate,
			&c.CancellationRate, &c.AcceptanceRate, &c.IdleSeconds, &c.ExactCapabilityMatch); err != nil {
			return nil, fmt.Errorf("scan candidate: %w", err)
		}
		out[c.DriverID] = c
	}
	return out, rows.Err()
}

// AttemptCount returns how many dispatch attempts a job has had.
func (s *Store) AttemptCount(ctx context.Context, jobID string) (int, error) {
	var count int
	err := s.pool.QueryRow(ctx,
		`SELECT count(*) FROM dispatch_attempts WHERE job_id = $1`, jobID).Scan(&count)
	if err != nil {
		return 0, fmt.Errorf("count dispatch attempts: %w", err)
	}
	return count, nil
}

// Explain returns the scores behind a job's dispatch attempts, for support.
func (s *Store) Explain(ctx context.Context, jobID string) ([]map[string]any, error) {
	rows, err := s.pool.Query(ctx,
		`SELECT a.attempt, a.radius_meters, a.outcome, a.strategy_version, a.score_version,
		        s.driver_id::text, s.rank, s.score, s.factors
		   FROM dispatch_attempts a
		   LEFT JOIN dispatch_scores s ON s.attempt_id = a.id
		  WHERE a.job_id = $1
		  ORDER BY a.attempt, s.rank`, jobID)
	if err != nil {
		return nil, fmt.Errorf("explain dispatch: %w", err)
	}
	defer rows.Close()

	var out []map[string]any
	for rows.Next() {
		var attempt, radius, strategyVersion, scoreVersion int
		var outcome string
		var driverID *string
		var rank *int
		var score *float64
		var factors []byte
		if err := rows.Scan(&attempt, &radius, &outcome, &strategyVersion, &scoreVersion,
			&driverID, &rank, &score, &factors); err != nil {
			return nil, err
		}
		entry := map[string]any{
			"attempt": attempt, "radius_meters": radius, "outcome": outcome,
			"strategy_version": strategyVersion, "score_version": scoreVersion,
		}
		if driverID != nil {
			entry["driver_id"], entry["rank"], entry["score"] = *driverID, *rank, *score
			var decoded map[string]any
			if json.Unmarshal(factors, &decoded) == nil {
				entry["factors"] = decoded
			}
		}
		out = append(out, entry)
	}
	return out, rows.Err()
}

// --- event deduplication and outbox -------------------------------------------

// MarkProcessed records that a consumer handled an event, reporting whether
// this was the first time. NATS redelivers; consumers must be idempotent
// (document 46).
func (s *Store) MarkProcessed(ctx context.Context, consumer, eventID string) (first bool, err error) {
	tag, err := s.pool.Exec(ctx,
		`INSERT INTO processed_events (consumer, event_id) VALUES ($1, $2) ON CONFLICT DO NOTHING`,
		consumer, eventID)
	if err != nil {
		return false, fmt.Errorf("mark event processed: %w", err)
	}
	return tag.RowsAffected() == 1, nil
}

// Enqueue writes an event to the outbox. Called inside the caller's
// transaction so the event and the state change commit together (document 46).
func Enqueue(ctx context.Context, tx pgx.Tx, subject, eventID string, payload any) error {
	encoded, err := json.Marshal(payload)
	if err != nil {
		return fmt.Errorf("encode outbox payload: %w", err)
	}
	if _, err := tx.Exec(ctx,
		`INSERT INTO event_outbox (subject, event_id, payload) VALUES ($1, $2, $3)
		 ON CONFLICT (event_id) DO NOTHING`,
		subject, eventID, encoded); err != nil {
		return fmt.Errorf("enqueue event: %w", err)
	}
	return nil
}

// PendingEvents returns unpublished outbox rows.
func (s *Store) PendingEvents(ctx context.Context, limit int) ([]OutboxEvent, error) {
	rows, err := s.pool.Query(ctx,
		`SELECT id, subject, event_id, payload FROM event_outbox
		  WHERE published_at IS NULL ORDER BY id LIMIT $1`, limit)
	if err != nil {
		return nil, fmt.Errorf("load pending events: %w", err)
	}
	defer rows.Close()

	var out []OutboxEvent
	for rows.Next() {
		var e OutboxEvent
		if err := rows.Scan(&e.ID, &e.Subject, &e.EventID, &e.Payload); err != nil {
			return nil, err
		}
		out = append(out, e)
	}
	return out, rows.Err()
}

// OutboxEvent is a pending publication.
type OutboxEvent struct {
	ID      int64
	Subject string
	EventID string
	Payload []byte
}

// MarkPublished records that an outbox row reached the broker.
func (s *Store) MarkPublished(ctx context.Context, id int64) error {
	_, err := s.pool.Exec(ctx, `UPDATE event_outbox SET published_at = now() WHERE id = $1`, id)
	if err != nil {
		return fmt.Errorf("mark event published: %w", err)
	}
	return nil
}

func isUnique(err error) bool {
	var pgErr interface{ SQLState() string }
	return errors.As(err, &pgErr) && pgErr.SQLState() == "23505"
}
