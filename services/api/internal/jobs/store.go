package jobs

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"time"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"
	"github.com/sarmadkung/rideme/services/api/pkg/money"
)

// Store is the persistence boundary for the job core.
type Store struct{ pool *pgxpool.Pool }

func NewStore(pool *pgxpool.Pool) *Store { return &Store{pool: pool} }

const jobColumns = `id, type, requester_user_id, COALESCE(merchant_id::text, ''), status,
	scheduled_at, COALESCE(pricing_quote_id::text, ''), COALESCE(assigned_driver_id::text, ''),
	COALESCE(assigned_vehicle_id::text, ''), terminated_at, created_at, updated_at`

func scanJob(row pgx.Row) (Job, error) {
	var j Job
	err := row.Scan(&j.ID, &j.Type, &j.RequesterUserID, &j.MerchantID, &j.Status,
		&j.ScheduledAt, &j.QuoteID, &j.AssignedDriverID, &j.AssignedVehicleID,
		&j.TerminatedAt, &j.CreatedAt, &j.UpdatedAt)
	return j, err
}

// Create stores a job and its stops and requirements in one transaction.
//
// All three or none: a job whose stops failed to insert is a job with no
// destination, which dispatch would happily try to serve.
func (s *Store) Create(ctx context.Context, job Job, actor Actor) (Job, error) {
	if err := job.ValidateForCreation(); err != nil {
		return Job{}, err
	}
	tx, err := s.pool.Begin(ctx)
	if err != nil {
		return Job{}, fmt.Errorf("begin: %w", err)
	}
	defer func() { _ = tx.Rollback(ctx) }()

	status := job.Status
	if status == "" {
		status = Machine.Initial()
	}
	var merchant, quote any
	if job.MerchantID != "" {
		merchant = job.MerchantID
	}
	if job.QuoteID != "" {
		quote = job.QuoteID
	}

	created, err := scanJob(tx.QueryRow(ctx,
		`INSERT INTO jobs (type, requester_user_id, merchant_id, status, scheduled_at, pricing_quote_id)
		 VALUES ($1, $2, $3, $4, $5, $6) RETURNING `+jobColumns,
		job.Type, job.RequesterUserID, merchant, status, job.ScheduledAt, quote))
	if err != nil {
		return Job{}, fmt.Errorf("insert job: %w", err)
	}

	for _, stop := range job.Stops {
		var id string
		err := tx.QueryRow(ctx,
			`INSERT INTO job_stops (job_id, sequence, type, location, address, contact_name, contact_phone)
			 VALUES ($1, $2, $3, ST_SetSRID(ST_MakePoint($4, $5), 4326)::geography,
			         NULLIF($6, ''), NULLIF($7, ''), NULLIF($8, ''))
			 RETURNING id`,
			created.ID, stop.Sequence, stop.Type,
			stop.Location.Longitude, stop.Location.Latitude, // PostGIS takes x=lon, y=lat
			stop.Address, stop.ContactName, stop.ContactPhone).Scan(&id)
		if err != nil {
			return Job{}, fmt.Errorf("insert stop %d: %w", stop.Sequence, err)
		}
		stop.ID = id
		stop.JobID = created.ID
		created.Stops = append(created.Stops, stop)
	}

	for _, req := range job.Requirements {
		if _, err := tx.Exec(ctx,
			`INSERT INTO job_requirements (job_id, requirement, value) VALUES ($1, $2, NULLIF($3, ''))`,
			created.ID, req.Name, req.Value); err != nil {
			return Job{}, fmt.Errorf("insert requirement %q: %w", req.Name, err)
		}
		created.Requirements = append(created.Requirements, req)
	}

	if err := appendHistory(ctx, tx, created.ID, "", created.Status, actor, nil); err != nil {
		return Job{}, err
	}
	if err := tx.Commit(ctx); err != nil {
		return Job{}, fmt.Errorf("commit: %w", err)
	}
	return created, nil
}

// ByID loads a job with its stops and requirements.
func (s *Store) ByID(ctx context.Context, id string) (Job, error) {
	job, err := scanJob(s.pool.QueryRow(ctx, `SELECT `+jobColumns+` FROM jobs WHERE id = $1`, id))
	if errors.Is(err, pgx.ErrNoRows) {
		return Job{}, ErrNotFound
	}
	if err != nil {
		return Job{}, fmt.Errorf("load job: %w", err)
	}
	if job.Stops, err = s.stopsOf(ctx, id); err != nil {
		return Job{}, err
	}
	if job.Requirements, err = s.requirementsOf(ctx, id); err != nil {
		return Job{}, err
	}
	return job, nil
}

func (s *Store) stopsOf(ctx context.Context, jobID string) ([]Stop, error) {
	rows, err := s.pool.Query(ctx,
		`SELECT id, job_id, sequence, type, ST_Y(location::geometry), ST_X(location::geometry),
		        COALESCE(address, ''), COALESCE(contact_name, ''), COALESCE(contact_phone, ''),
		        arrived_at, completed_at
		   FROM job_stops WHERE job_id = $1 ORDER BY sequence`, jobID)
	if err != nil {
		return nil, fmt.Errorf("load stops: %w", err)
	}
	defer rows.Close()

	var out []Stop
	for rows.Next() {
		var s Stop
		if err := rows.Scan(&s.ID, &s.JobID, &s.Sequence, &s.Type,
			&s.Location.Latitude, &s.Location.Longitude,
			&s.Address, &s.ContactName, &s.ContactPhone, &s.ArrivedAt, &s.CompletedAt); err != nil {
			return nil, fmt.Errorf("scan stop: %w", err)
		}
		out = append(out, s)
	}
	return out, rows.Err()
}

func (s *Store) requirementsOf(ctx context.Context, jobID string) ([]Requirement, error) {
	rows, err := s.pool.Query(ctx,
		`SELECT requirement, COALESCE(value, '') FROM job_requirements WHERE job_id = $1 ORDER BY requirement`, jobID)
	if err != nil {
		return nil, fmt.Errorf("load requirements: %w", err)
	}
	defer rows.Close()

	var out []Requirement
	for rows.Next() {
		var r Requirement
		if err := rows.Scan(&r.Name, &r.Value); err != nil {
			return nil, fmt.Errorf("scan requirement: %w", err)
		}
		out = append(out, r)
	}
	return out, rows.Err()
}

// ListForRequester returns a customer's jobs, newest first, cursor-paginated
// (ADR-009). The cursor is the created_at of the last row seen.
func (s *Store) ListForRequester(ctx context.Context, userID string, before *time.Time, limit int) ([]Job, error) {
	rows, err := s.pool.Query(ctx,
		`SELECT `+jobColumns+` FROM jobs
		  WHERE requester_user_id = $1 AND ($2::timestamptz IS NULL OR created_at < $2)
		  ORDER BY created_at DESC LIMIT $3`,
		userID, before, limit)
	if err != nil {
		return nil, fmt.Errorf("list jobs: %w", err)
	}
	defer rows.Close()

	var out []Job
	for rows.Next() {
		job, err := scanJob(rows)
		if err != nil {
			return nil, fmt.Errorf("scan job: %w", err)
		}
		out = append(out, job)
	}
	return out, rows.Err()
}

// Transition moves a job to a new status, refusing anything the machine does
// not permit and anything that races another writer.
//
// The `status = $2` predicate is compare-and-set. Two concurrent transitions
// read the same job and both believe the move is legal; without the predicate
// both would write, and the second would silently overwrite the first —
// a driver's ACCEPTED erased by a customer's CANCELLED, or the reverse. With
// it, exactly one wins and the loser is told.
func (s *Store) Transition(ctx context.Context, jobID string, from, to Status, actor Actor, metadata map[string]any) (Job, error) {
	if err := Machine.Validate(from, to); err != nil {
		return Job{}, err
	}
	tx, err := s.pool.Begin(ctx)
	if err != nil {
		return Job{}, fmt.Errorf("begin: %w", err)
	}
	defer func() { _ = tx.Rollback(ctx) }()

	var terminated any
	if Machine.Terminal(to) {
		terminated = time.Now().UTC()
	}

	job, err := scanJob(tx.QueryRow(ctx,
		`UPDATE jobs
		    SET status = $3, updated_at = now(),
		        terminated_at = COALESCE(terminated_at, $4)
		  WHERE id = $1 AND status = $2
		  RETURNING `+jobColumns,
		jobID, from, to, terminated))
	if errors.Is(err, pgx.ErrNoRows) {
		// Either the job is gone or its status moved. Distinguishing the two
		// tells the caller whether to retry or to stop.
		var current Status
		if qerr := s.pool.QueryRow(ctx, `SELECT status FROM jobs WHERE id = $1`, jobID).Scan(&current); qerr != nil {
			return Job{}, ErrNotFound
		}
		return Job{}, fmt.Errorf("%w: expected %s, found %s", ErrStaleTransition, from, current)
	}
	if err != nil {
		return Job{}, fmt.Errorf("transition job: %w", err)
	}

	if err := appendHistory(ctx, tx, jobID, from, to, actor, metadata); err != nil {
		return Job{}, err
	}
	if err := tx.Commit(ctx); err != nil {
		return Job{}, fmt.Errorf("commit: %w", err)
	}
	return job, nil
}

// History returns every recorded transition for a job, oldest first.
func (s *Store) History(ctx context.Context, jobID string) ([]StatusChange, error) {
	rows, err := s.pool.Query(ctx,
		`SELECT id, job_id, COALESCE(from_status, ''), to_status, actor_type,
		        COALESCE(actor_id::text, ''), metadata, created_at
		   FROM job_status_history WHERE job_id = $1 ORDER BY id`, jobID)
	if err != nil {
		return nil, fmt.Errorf("load history: %w", err)
	}
	defer rows.Close()

	var out []StatusChange
	for rows.Next() {
		var change StatusChange
		var raw []byte
		if err := rows.Scan(&change.ID, &change.JobID, &change.From, &change.To,
			&change.Actor.Type, &change.Actor.ID, &raw, &change.CreatedAt); err != nil {
			return nil, fmt.Errorf("scan history: %w", err)
		}
		if len(raw) > 0 {
			_ = json.Unmarshal(raw, &change.Metadata)
		}
		out = append(out, change)
	}
	return out, rows.Err()
}

func appendHistory(ctx context.Context, tx pgx.Tx, jobID string, from, to Status, actor Actor, metadata map[string]any) error {
	if metadata == nil {
		metadata = map[string]any{}
	}
	encoded, err := json.Marshal(metadata)
	if err != nil {
		return fmt.Errorf("encode history metadata: %w", err)
	}
	actorType := actor.Type
	if actorType == "" {
		actorType = ActorSystem
	}
	var actorID, fromStatus any
	if actor.ID != "" {
		actorID = actor.ID
	}
	if from != "" {
		fromStatus = from
	}
	if _, err := tx.Exec(ctx,
		`INSERT INTO job_status_history (job_id, from_status, to_status, actor_type, actor_id, metadata)
		 VALUES ($1, $2, $3, $4, $5, $6)`,
		jobID, fromStatus, to, actorType, actorID, encoded); err != nil {
		return fmt.Errorf("append history: %w", err)
	}
	return nil
}

// --- quotes ------------------------------------------------------------------

// CreateQuote stores a price offered for a job. No pricing is computed here;
// the amount is supplied by whatever produced it (CAP-1, Phase 7).
func (s *Store) CreateQuote(ctx context.Context, q Quote) (Quote, error) {
	if err := q.Amount.Validate(); err != nil {
		return Quote{}, err
	}
	breakdown := q.Breakdown
	if breakdown == nil {
		breakdown = map[string]any{}
	}
	encoded, err := json.Marshal(breakdown)
	if err != nil {
		return Quote{}, fmt.Errorf("encode breakdown: %w", err)
	}
	var low, high any
	if q.Low != nil && q.High != nil {
		low, high = q.Low.Minor, q.High.Minor
	}
	err = s.pool.QueryRow(ctx,
		`INSERT INTO pricing_quotes (job_type, amount_minor, currency, low_minor, high_minor, breakdown, expires_at)
		 VALUES ($1, $2, $3, $4, $5, $6, $7) RETURNING id, created_at`,
		q.JobType, q.Amount.Minor, q.Amount.Currency, low, high, encoded, q.ExpiresAt).
		Scan(&q.ID, &q.CreatedAt)
	if err != nil {
		return Quote{}, fmt.Errorf("insert quote: %w", err)
	}
	return q, nil
}

// QuoteByID loads a quote.
func (s *Store) QuoteByID(ctx context.Context, id string) (Quote, error) {
	var q Quote
	var minor int64
	var currency money.Currency
	var low, high *int64
	var raw []byte
	err := s.pool.QueryRow(ctx,
		`SELECT id, job_type, amount_minor, currency, low_minor, high_minor, breakdown, expires_at, created_at
		   FROM pricing_quotes WHERE id = $1`, id).
		Scan(&q.ID, &q.JobType, &minor, &currency, &low, &high, &raw, &q.ExpiresAt, &q.CreatedAt)
	if errors.Is(err, pgx.ErrNoRows) {
		return Quote{}, ErrNotFound
	}
	if err != nil {
		return Quote{}, fmt.Errorf("load quote: %w", err)
	}
	if q.Amount, err = money.New(minor, currency); err != nil {
		return Quote{}, err
	}
	if low != nil && high != nil {
		lowAmount, err := money.New(*low, currency)
		if err != nil {
			return Quote{}, err
		}
		highAmount, err := money.New(*high, currency)
		if err != nil {
			return Quote{}, err
		}
		q.Low, q.High = &lowAmount, &highAmount
	}
	if len(raw) > 0 {
		_ = json.Unmarshal(raw, &q.Breakdown)
	}
	return q, nil
}

// AttachQuote links a quote to a job.
func (s *Store) AttachQuote(ctx context.Context, jobID, quoteID string) error {
	_, err := s.pool.Exec(ctx,
		`UPDATE jobs SET pricing_quote_id = $2, updated_at = now() WHERE id = $1`, jobID, quoteID)
	if err != nil {
		return fmt.Errorf("attach quote: %w", err)
	}
	return nil
}

// --- assignments -------------------------------------------------------------

const assignmentColumns = `id, job_id, driver_id, COALESCE(vehicle_id::text, ''), status,
	offered_at, responded_at, accepted_at, completed_at, expires_at`

func scanAssignment(row pgx.Row) (Assignment, error) {
	var a Assignment
	err := row.Scan(&a.ID, &a.JobID, &a.DriverID, &a.VehicleID, &a.Status,
		&a.OfferedAt, &a.RespondedAt, &a.AcceptedAt, &a.CompletedAt, &a.ExpiresAt)
	return a, err
}

// Offer creates an assignment, or reports that the job is already claimed.
//
// The uniqueness is enforced by a partial unique index, not by a prior SELECT.
// Under concurrency a check-then-insert has a window between the two in which
// another dispatcher inserts; the index has no such window. This is the
// guarantee Phase 8 is built on: two drivers can never hold one job.
func (s *Store) Offer(ctx context.Context, a Assignment) (Assignment, error) {
	var vehicle any
	if a.VehicleID != "" {
		vehicle = a.VehicleID
	}
	created, err := scanAssignment(s.pool.QueryRow(ctx,
		`INSERT INTO assignments (job_id, driver_id, vehicle_id, status, expires_at)
		 VALUES ($1, $2, $3, 'OFFERED', $4) RETURNING `+assignmentColumns,
		a.JobID, a.DriverID, vehicle, a.ExpiresAt))
	if err != nil {
		if isUniqueViolation(err) {
			return Assignment{}, ErrAlreadyClaimed
		}
		return Assignment{}, fmt.Errorf("offer assignment: %w", err)
	}
	return created, nil
}

// RespondToAssignment moves an assignment, compare-and-set on its current
// status so two responses to one offer cannot both take effect.
func (s *Store) RespondToAssignment(ctx context.Context, id string, from, to AssignmentStatus) (Assignment, error) {
	if err := AssignmentMachine.Validate(from, to); err != nil {
		return Assignment{}, err
	}
	var accepted, completed any
	now := time.Now().UTC()
	if to == AssignmentAccepted {
		accepted = now
	}
	if to == AssignmentCompleted {
		completed = now
	}
	a, err := scanAssignment(s.pool.QueryRow(ctx,
		`UPDATE assignments
		    SET status = $3, responded_at = COALESCE(responded_at, now()),
		        accepted_at = COALESCE(accepted_at, $4), completed_at = COALESCE(completed_at, $5)
		  WHERE id = $1 AND status = $2
		  RETURNING `+assignmentColumns,
		id, from, to, accepted, completed))
	if errors.Is(err, pgx.ErrNoRows) {
		return Assignment{}, ErrStaleTransition
	}
	if err != nil {
		return Assignment{}, fmt.Errorf("respond to assignment: %w", err)
	}
	return a, nil
}

// LiveAssignment returns the assignment currently holding a job, if any.
func (s *Store) LiveAssignment(ctx context.Context, jobID string) (Assignment, error) {
	a, err := scanAssignment(s.pool.QueryRow(ctx,
		`SELECT `+assignmentColumns+` FROM assignments
		  WHERE job_id = $1 AND status IN ('OFFERED', 'ACCEPTED')`, jobID))
	if errors.Is(err, pgx.ErrNoRows) {
		return Assignment{}, ErrNotFound
	}
	if err != nil {
		return Assignment{}, fmt.Errorf("load assignment: %w", err)
	}
	return a, nil
}

func isUniqueViolation(err error) bool {
	var pgErr interface{ SQLState() string }
	return errors.As(err, &pgErr) && pgErr.SQLState() == "23505"
}
