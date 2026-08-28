package delivery

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"time"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"
	"github.com/sarmadkung/rideme/services/api/pkg/authn"
)

type Store struct {
	pool   *pgxpool.Pool
	secret string
}

func NewStore(pool *pgxpool.Pool, secret string) *Store {
	return &Store{pool: pool, secret: secret}
}

// --- proof -------------------------------------------------------------------

// RecordProof stores delivery evidence.
func (s *Store) RecordProof(ctx context.Context, p Proof) (Proof, error) {
	if err := ValidateProof(p); err != nil {
		return Proof{}, err
	}
	metadata := p.Metadata
	if metadata == nil {
		metadata = map[string]any{}
	}
	encoded, err := json.Marshal(metadata)
	if err != nil {
		return Proof{}, fmt.Errorf("encode proof metadata: %w", err)
	}
	var stopID, capturedBy any
	if p.StopID != "" {
		stopID = p.StopID
	}
	if p.CapturedBy != "" {
		capturedBy = p.CapturedBy
	}

	query := `INSERT INTO delivery_proofs
		    (job_id, stop_id, method, media_key, recipient_name, verified, captured_by, metadata, location)
		  VALUES ($1,$2,$3,NULLIF($4,''),NULLIF($5,''),$6,$7,$8,`
	args := []any{p.JobID, stopID, p.Method, p.MediaKey, p.RecipientName, p.Verified, capturedBy, encoded}
	if p.HasLocation {
		query += `ST_SetSRID(ST_MakePoint($9, $10), 4326)::geography)`
		args = append(args, p.Lon, p.Lat)
	} else {
		query += `NULL)`
	}
	query += ` RETURNING id::text, created_at`

	if err := s.pool.QueryRow(ctx, query, args...).Scan(&p.ID, &p.CreatedAt); err != nil {
		return Proof{}, fmt.Errorf("record proof: %w", err)
	}
	return p, nil
}

// ProofsFor returns the evidence recorded against a job.
func (s *Store) ProofsFor(ctx context.Context, jobID string) ([]Proof, error) {
	rows, err := s.pool.Query(ctx,
		`SELECT id::text, job_id::text, COALESCE(stop_id::text, ''), method,
		        COALESCE(media_key, ''), COALESCE(recipient_name, ''), verified, created_at
		   FROM delivery_proofs WHERE job_id = $1 ORDER BY created_at`, jobID)
	if err != nil {
		return nil, fmt.Errorf("load proofs: %w", err)
	}
	defer rows.Close()

	var out []Proof
	for rows.Next() {
		var p Proof
		if err := rows.Scan(&p.ID, &p.JobID, &p.StopID, &p.Method,
			&p.MediaKey, &p.RecipientName, &p.Verified, &p.CreatedAt); err != nil {
			return nil, err
		}
		out = append(out, p)
	}
	return out, rows.Err()
}

// --- recipient OTP -----------------------------------------------------------

// OTPTTL is how long a recipient code stands. Long enough for a driver to
// reach the door and read it out, short enough that a code overheard earlier
// is useless.
const OTPTTL = 30 * time.Minute

// IssueRecipientOTP creates a delivery code for a stop and returns the plain
// code so it can be sent to the recipient.
//
// Only the hash is stored, for the same reason login OTPs are hashed: a stored
// code is a code someone can read, and this one authorises a handover.
func (s *Store) IssueRecipientOTP(ctx context.Context, jobID, stopID string) (string, error) {
	code, err := authn.NewOTP()
	if err != nil {
		return "", fmt.Errorf("generate recipient code: %w", err)
	}
	_, err = s.pool.Exec(ctx,
		`INSERT INTO delivery_otps (stop_id, job_id, code_hash, expires_at)
		 VALUES ($1, $2, $3, $4)
		 ON CONFLICT (stop_id) DO UPDATE
		   SET code_hash = EXCLUDED.code_hash, expires_at = EXCLUDED.expires_at,
		       attempts = 0, consumed_at = NULL`,
		stopID, jobID, authn.HashOTP(s.secret, stopID, code), time.Now().UTC().Add(OTPTTL))
	if err != nil {
		return "", fmt.Errorf("issue recipient code: %w", err)
	}
	return code, nil
}

// VerifyRecipientOTP checks a code and consumes it.
//
// Consumption is conditional, so two drivers submitting the same code — or one
// driver retrying — produce one handover, not two.
func (s *Store) VerifyRecipientOTP(ctx context.Context, stopID, code string) error {
	tx, err := s.pool.Begin(ctx)
	if err != nil {
		return fmt.Errorf("begin: %w", err)
	}
	defer func() { _ = tx.Rollback(ctx) }()

	var storedHash []byte
	var attempts, maxAttempts int
	var expiresAt time.Time
	err = tx.QueryRow(ctx,
		`SELECT code_hash, attempts, max_attempts, expires_at FROM delivery_otps
		  WHERE stop_id = $1 AND consumed_at IS NULL FOR UPDATE`, stopID).
		Scan(&storedHash, &attempts, &maxAttempts, &expiresAt)
	if errors.Is(err, pgx.ErrNoRows) {
		return ErrOTPIncorrect
	}
	if err != nil {
		return fmt.Errorf("load recipient code: %w", err)
	}
	if attempts >= maxAttempts || !time.Now().UTC().Before(expiresAt) {
		return ErrOTPIncorrect
	}

	if !authn.EqualOTP(storedHash, authn.HashOTP(s.secret, stopID, code)) {
		if _, uerr := tx.Exec(ctx,
			`UPDATE delivery_otps SET attempts = attempts + 1 WHERE stop_id = $1`, stopID); uerr != nil {
			return fmt.Errorf("record failed attempt: %w", uerr)
		}
		if cerr := tx.Commit(ctx); cerr != nil {
			return fmt.Errorf("commit: %w", cerr)
		}
		return ErrOTPIncorrect
	}

	tag, err := tx.Exec(ctx,
		`UPDATE delivery_otps SET consumed_at = now() WHERE stop_id = $1 AND consumed_at IS NULL`, stopID)
	if err != nil {
		return fmt.Errorf("consume recipient code: %w", err)
	}
	if tag.RowsAffected() != 1 {
		return ErrOTPIncorrect
	}
	return tx.Commit(ctx)
}

// --- delivery attempts -------------------------------------------------------

// RecordAttempt stores a delivery attempt and the action it produced.
func (s *Store) RecordAttempt(ctx context.Context, a Attempt) (Attempt, error) {
	var stopID, reason, action, notes any
	if a.StopID != "" {
		stopID = a.StopID
	}
	if a.Reason != "" {
		reason = string(a.Reason)
	}
	if a.NextAction != "" {
		action = string(a.NextAction)
	}
	if a.Notes != "" {
		notes = a.Notes
	}
	outcome := "FAILED"
	if a.Delivered {
		outcome = "DELIVERED"
	}
	err := s.pool.QueryRow(ctx,
		`INSERT INTO delivery_attempts (job_id, stop_id, attempt, outcome, failure_reason, next_action, notes)
		 VALUES ($1,$2,$3,$4,$5,$6,$7) RETURNING id::text, created_at`,
		a.JobID, stopID, a.Attempt, outcome, reason, action, notes).
		Scan(&a.ID, &a.CreatedAt)
	if err != nil {
		return Attempt{}, fmt.Errorf("record delivery attempt: %w", err)
	}
	return a, nil
}

// AttemptCount returns how many attempts a stop has had.
func (s *Store) AttemptCount(ctx context.Context, stopID string) (int, error) {
	var count int
	err := s.pool.QueryRow(ctx,
		`SELECT count(*) FROM delivery_attempts WHERE stop_id = $1`, stopID).Scan(&count)
	if err != nil {
		return 0, fmt.Errorf("count delivery attempts: %w", err)
	}
	return count, nil
}

// AddReturnStop appends a return leg to the job.
//
// Document 84: a return "may create a Return Stop rather than creating an
// unrelated manual job". Keeping it on the same job keeps the parcel's history
// in one place instead of splitting it across two.
//
// BD-10 leaves the financial treatment of a return unresolved. This creates the
// stop and prices nothing.
func (s *Store) AddReturnStop(ctx context.Context, jobID, failedStopID string) (string, error) {
	var stopID string
	err := s.pool.QueryRow(ctx,
		`INSERT INTO job_stops (job_id, sequence, type, location, address, is_return, returns_stop_id)
		 SELECT $1,
		        (SELECT COALESCE(max(sequence), -1) + 1 FROM job_stops WHERE job_id = $1),
		        'DROPOFF',
		        origin.location, origin.address, true, $2
		   FROM job_stops origin
		  WHERE origin.job_id = $1 AND origin.type = 'PICKUP'
		  ORDER BY origin.sequence
		  LIMIT 1
		 RETURNING id::text`,
		jobID, failedStopID).Scan(&stopID)
	if errors.Is(err, pgx.ErrNoRows) {
		return "", errors.New("delivery: the job has no pickup to return to")
	}
	if err != nil {
		return "", fmt.Errorf("add return stop: %w", err)
	}
	return stopID, nil
}

// --- cargo -------------------------------------------------------------------

// SaveCargo stores the physical description of a load.
func (s *Store) SaveCargo(ctx context.Context, c Cargo) error {
	assistance := c.LoadingAssistance
	if assistance == "" {
		assistance = AssistDriverOnly
	}
	var handling any
	if c.SpecialHandling != "" {
		handling = c.SpecialHandling
	}
	_, err := s.pool.Exec(ctx,
		`INSERT INTO cargo_details
		   (job_id, total_weight_kg, length_cm, width_cm, height_cm, volume_m3, item_count,
		    fragile, temperature_sensitive, special_handling, loading_assistance)
		 VALUES ($1,$2,$3,$4,$5,$6,$7,$8,$9,$10,$11)
		 ON CONFLICT (job_id) DO UPDATE SET
		   total_weight_kg = EXCLUDED.total_weight_kg, length_cm = EXCLUDED.length_cm,
		   width_cm = EXCLUDED.width_cm, height_cm = EXCLUDED.height_cm,
		   volume_m3 = EXCLUDED.volume_m3, item_count = EXCLUDED.item_count,
		   fragile = EXCLUDED.fragile, temperature_sensitive = EXCLUDED.temperature_sensitive,
		   special_handling = EXCLUDED.special_handling,
		   loading_assistance = EXCLUDED.loading_assistance`,
		c.JobID, c.TotalWeightKG, c.LengthCM, c.WidthCM, c.HeightCM, c.VolumeM3,
		c.ItemCount, c.Fragile, c.TemperatureSensitive, handling, assistance)
	if err != nil {
		return fmt.Errorf("save cargo details: %w", err)
	}
	return nil
}

// CargoFor loads a job's cargo description.
func (s *Store) CargoFor(ctx context.Context, jobID string) (Cargo, bool, error) {
	var c Cargo
	var handling *string
	err := s.pool.QueryRow(ctx,
		`SELECT job_id::text, total_weight_kg, length_cm, width_cm, height_cm, volume_m3,
		        item_count, fragile, temperature_sensitive, special_handling, loading_assistance
		   FROM cargo_details WHERE job_id = $1`, jobID).
		Scan(&c.JobID, &c.TotalWeightKG, &c.LengthCM, &c.WidthCM, &c.HeightCM, &c.VolumeM3,
			&c.ItemCount, &c.Fragile, &c.TemperatureSensitive, &handling, &c.LoadingAssistance)
	if errors.Is(err, pgx.ErrNoRows) {
		return Cargo{}, false, nil
	}
	if err != nil {
		return Cargo{}, false, fmt.Errorf("load cargo details: %w", err)
	}
	if handling != nil {
		c.SpecialHandling = *handling
	}
	return c, true, nil
}

// VehicleCapacityFor loads what a vehicle can physically carry.
func (s *Store) VehicleCapacityFor(ctx context.Context, vehicleID string) (VehicleCapacity, error) {
	var v VehicleCapacity
	err := s.pool.QueryRow(ctx,
		`SELECT capacity_kg, max_volume_m3, cargo_length_cm, cargo_width_cm, cargo_height_cm, equipment
		   FROM vehicles WHERE id = $1`, vehicleID).
		Scan(&v.MaxWeightKG, &v.MaxVolumeM3, &v.CargoLengthCM, &v.CargoWidthCM, &v.CargoHeightCM, &v.Equipment)
	if errors.Is(err, pgx.ErrNoRows) {
		return VehicleCapacity{}, errors.New("delivery: vehicle not found")
	}
	if err != nil {
		return VehicleCapacity{}, fmt.Errorf("load vehicle capacity: %w", err)
	}
	return v, nil
}

// --- stop timings ------------------------------------------------------------

// MarkArrived records arrival and starts the waiting clock (document 87).
func (s *Store) MarkArrived(ctx context.Context, jobID, stopID string, graceSeconds int) error {
	_, err := s.pool.Exec(ctx,
		`INSERT INTO stop_timings (stop_id, job_id, arrived_at, waiting_started_at, grace_seconds)
		 VALUES ($1, $2, now(), now(), $3)
		 ON CONFLICT (stop_id) DO UPDATE
		   SET arrived_at = COALESCE(stop_timings.arrived_at, now()),
		       waiting_started_at = COALESCE(stop_timings.waiting_started_at, now()),
		       updated_at = now()`,
		stopID, jobID, graceSeconds)
	if err != nil {
		return fmt.Errorf("mark arrived: %w", err)
	}
	return nil
}

// MarkLoadingStarted ends waiting and starts loading, computing the chargeable
// waiting seconds in the same statement so the two cannot disagree.
func (s *Store) MarkLoadingStarted(ctx context.Context, stopID string) error {
	_, err := s.pool.Exec(ctx,
		`UPDATE stop_timings
		    SET loading_started_at = COALESCE(loading_started_at, now()),
		        chargeable_waiting_seconds = GREATEST(0,
		          EXTRACT(EPOCH FROM (now() - COALESCE(waiting_started_at, now())))::integer - grace_seconds),
		        updated_at = now()
		  WHERE stop_id = $1`, stopID)
	if err != nil {
		return fmt.Errorf("mark loading started: %w", err)
	}
	return nil
}

// MarkLoaded ends loading and records its duration.
func (s *Store) MarkLoaded(ctx context.Context, stopID string) error {
	_, err := s.pool.Exec(ctx,
		`UPDATE stop_timings
		    SET loaded_at = COALESCE(loaded_at, now()),
		        loading_seconds = GREATEST(0,
		          EXTRACT(EPOCH FROM (now() - COALESCE(loading_started_at, now())))::integer),
		        updated_at = now()
		  WHERE stop_id = $1`, stopID)
	if err != nil {
		return fmt.Errorf("mark loaded: %w", err)
	}
	return nil
}

// TimingFor loads a stop's recorded times.
func (s *Store) TimingFor(ctx context.Context, stopID string) (Timing, bool, error) {
	var t Timing
	err := s.pool.QueryRow(ctx,
		`SELECT stop_id::text, job_id::text, arrived_at, waiting_started_at, loading_started_at,
		        loaded_at, unloading_started_at, unloaded_at, grace_seconds,
		        chargeable_waiting_seconds, loading_seconds
		   FROM stop_timings WHERE stop_id = $1`, stopID).
		Scan(&t.StopID, &t.JobID, &t.ArrivedAt, &t.WaitingStartedAt, &t.LoadingStartedAt,
			&t.LoadedAt, &t.UnloadingStartedAt, &t.UnloadedAt, &t.GraceSeconds,
			&t.ChargeableWaitingSecs, &t.LoadingSeconds)
	if errors.Is(err, pgx.ErrNoRows) {
		return Timing{}, false, nil
	}
	if err != nil {
		return Timing{}, false, fmt.Errorf("load stop timing: %w", err)
	}
	return t, true, nil
}

// --- restricted goods --------------------------------------------------------

// CheckRestricted reports which declared goods are prohibited.
//
// The table ships empty (BD-13, document 88: the list is legal and the owner's
// to supply), so this passes vacuously until a list exists. That is the honest
// behaviour: a guessed list would block legitimate loads and miss real ones.
func (s *Store) CheckRestricted(ctx context.Context, market string, declared []string) ([]string, error) {
	if len(declared) == 0 {
		return nil, nil
	}
	if market == "" {
		market = "PK"
	}
	rows, err := s.pool.Query(ctx,
		`SELECT code FROM restricted_goods WHERE market = $1 AND code = ANY($2)`, market, declared)
	if err != nil {
		return nil, fmt.Errorf("check restricted goods: %w", err)
	}
	defer rows.Close()

	var blocked []string
	for rows.Next() {
		var code string
		if err := rows.Scan(&code); err != nil {
			return nil, err
		}
		blocked = append(blocked, code)
	}
	return blocked, rows.Err()
}
