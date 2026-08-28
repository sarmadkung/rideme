package tracking

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"time"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"
	"github.com/redis/go-redis/v9"
)

// Store keeps current state in Redis and history in PostgreSQL.
//
// The split follows document 18: "Redis stores current driver state, active
// job, geospatial availability and short-lived locks. PostgreSQL remains
// durable source of truth."
type Store struct {
	pool  *pgxpool.Pool
	redis *redis.Client
	// currentTTL expires a driver's live position so a crashed device stops
	// appearing available. A stale key is worse than a missing one: dispatch
	// would offer a job to a phone that is switched off.
	currentTTL time.Duration
}

func NewStore(pool *pgxpool.Pool, client *redis.Client) *Store {
	return &Store{pool: pool, redis: client, currentTTL: 10 * time.Minute}
}

const (
	currentKeyPrefix = "driver:current:"
	// availableGeoKey is the geospatial index dispatch queries. Only drivers
	// who are AVAILABLE belong in it; membership is the fast answer to "who
	// could take this job", before any of the expensive checks run.
	availableGeoKey = "drivers:available:geo"
)

func currentKey(driverID string) string { return currentKeyPrefix + driverID }

// PutCurrent writes the driver's live position and, when they are available,
// their entry in the geospatial index.
func (s *Store) PutCurrent(ctx context.Context, c Current, available bool) error {
	encoded, err := json.Marshal(c)
	if err != nil {
		return fmt.Errorf("encode current location: %w", err)
	}
	pipe := s.redis.TxPipeline()
	pipe.Set(ctx, currentKey(c.DriverID), encoded, s.currentTTL)
	if available {
		pipe.GeoAdd(ctx, availableGeoKey, &redis.GeoLocation{
			Name: c.DriverID, Latitude: c.Lat, Longitude: c.Lon,
		})
	} else {
		// A driver who is offline, on a trip, or mid-offer must leave the
		// candidate pool immediately, not when their key expires.
		pipe.ZRem(ctx, availableGeoKey, c.DriverID)
	}
	if _, err := pipe.Exec(ctx); err != nil {
		return fmt.Errorf("store current location: %w", err)
	}
	return nil
}

// Current returns a driver's live position.
func (s *Store) Current(ctx context.Context, driverID string) (Current, bool, error) {
	raw, err := s.redis.Get(ctx, currentKey(driverID)).Bytes()
	if errors.Is(err, redis.Nil) {
		return Current{}, false, nil
	}
	if err != nil {
		return Current{}, false, fmt.Errorf("load current location: %w", err)
	}
	var c Current
	if err := json.Unmarshal(raw, &c); err != nil {
		return Current{}, false, fmt.Errorf("decode current location: %w", err)
	}
	return c, true, nil
}

// RemoveFromPool takes a driver out of the availability index — going offline,
// being suspended, or accepting a job.
func (s *Store) RemoveFromPool(ctx context.Context, driverID string) error {
	if err := s.redis.ZRem(ctx, availableGeoKey, driverID).Err(); err != nil {
		return fmt.Errorf("remove from availability pool: %w", err)
	}
	return nil
}

// NearbyDriver is a candidate from the geospatial index.
type NearbyDriver struct {
	DriverID       string
	DistanceMeters float64
	Lat, Lon       float64
}

// Nearby returns available drivers within a radius, nearest first.
//
// This is dispatch's candidate discovery step (document 42): a bounded
// geospatial lookup rather than a scan of every driver. Everything expensive —
// eligibility, routing, scoring — runs only on what this returns.
func (s *Store) Nearby(ctx context.Context, lat, lon float64, radiusMeters float64, limit int) ([]NearbyDriver, error) {
	results, err := s.redis.GeoSearchLocation(ctx, availableGeoKey, &redis.GeoSearchLocationQuery{
		GeoSearchQuery: redis.GeoSearchQuery{
			Longitude:  lon,
			Latitude:   lat,
			Radius:     radiusMeters,
			RadiusUnit: "m",
			Sort:       "ASC",
			Count:      limit,
		},
		WithCoord: true,
		WithDist:  true,
	}).Result()
	if err != nil {
		return nil, fmt.Errorf("search nearby drivers: %w", err)
	}
	out := make([]NearbyDriver, 0, len(results))
	for _, r := range results {
		out = append(out, NearbyDriver{
			DriverID: r.Name, DistanceMeters: r.Dist,
			Lat: r.Latitude, Lon: r.Longitude,
		})
	}
	return out, nil
}

// --- durable history ---------------------------------------------------------

// AppendHistory writes fixes to PostgreSQL in one batch.
//
// Document 48: "Do not write every point directly into PostgreSQL
// synchronously. Use a buffered/asynchronous pipeline." The batch signature is
// that requirement made structural — there is no single-fix insert to reach
// for, so the ingestion path cannot accidentally become synchronous.
func (s *Store) AppendHistory(ctx context.Context, fixes []Fix) (int64, error) {
	if len(fixes) == 0 {
		return 0, nil
	}
	batch := &pgx.Batch{}
	for _, f := range fixes {
		var vehicleID, jobID any
		if f.VehicleID != "" {
			vehicleID = f.VehicleID
		}
		if f.JobID != "" {
			jobID = f.JobID
		}
		batch.Queue(
			`INSERT INTO driver_locations
			   (driver_id, vehicle_id, job_id, location, accuracy_m, heading_deg, speed_mps, recorded_at)
			 VALUES ($1, $2, $3, ST_SetSRID(ST_MakePoint($4, $5), 4326)::geography, $6, $7, $8, $9)`,
			f.DriverID, vehicleID, jobID, f.Lon, f.Lat, f.AccuracyM, f.HeadingDeg, f.SpeedMPS, f.RecordedAt)
	}
	results := s.pool.SendBatch(ctx, batch)
	defer func() { _ = results.Close() }()

	var written int64
	for range fixes {
		if _, err := results.Exec(); err != nil {
			return written, fmt.Errorf("append location history: %w", err)
		}
		written++
	}
	return written, nil
}

// LastFix returns the most recent stored position, for jump detection across
// process restarts.
func (s *Store) LastFix(ctx context.Context, driverID string) (*Previous, error) {
	var p Previous
	err := s.pool.QueryRow(ctx,
		`SELECT ST_Y(location::geometry), ST_X(location::geometry), recorded_at
		   FROM driver_locations WHERE driver_id = $1
		  ORDER BY recorded_at DESC LIMIT 1`, driverID).
		Scan(&p.Lat, &p.Lon, &p.RecordedAt)
	if errors.Is(err, pgx.ErrNoRows) {
		return nil, nil
	}
	if err != nil {
		return nil, fmt.Errorf("load last fix: %w", err)
	}
	return &p, nil
}

// JobTrack returns the positions recorded against one job, in order — the
// route a customer or an investigator sees.
func (s *Store) JobTrack(ctx context.Context, jobID string) ([]Current, error) {
	rows, err := s.pool.Query(ctx,
		`SELECT driver_id::text, ST_Y(location::geometry), ST_X(location::geometry),
		        heading_deg, speed_mps, recorded_at
		   FROM driver_locations WHERE job_id = $1 ORDER BY recorded_at`, jobID)
	if err != nil {
		return nil, fmt.Errorf("load job track: %w", err)
	}
	defer rows.Close()

	var out []Current
	for rows.Next() {
		var c Current
		if err := rows.Scan(&c.DriverID, &c.Lat, &c.Lon, &c.HeadingDeg, &c.SpeedMPS, &c.RecordedAt); err != nil {
			return nil, err
		}
		c.JobID = jobID
		out = append(out, c)
	}
	return out, rows.Err()
}

// PurgeHistoryBefore deletes location history older than a cutoff.
//
// BD-15 sets the period and is unresolved — a privacy and legal decision, not
// an engineering one. The mechanism exists and takes the cutoff as an argument
// precisely so no default is baked in: nothing here decides how long location
// is kept, and running the sweep requires someone to choose.
func (s *Store) PurgeHistoryBefore(ctx context.Context, cutoff time.Time) (int64, error) {
	tag, err := s.pool.Exec(ctx, `DELETE FROM driver_locations WHERE recorded_at < $1`, cutoff)
	if err != nil {
		return 0, fmt.Errorf("purge location history: %w", err)
	}
	return tag.RowsAffected(), nil
}

// --- tracking sessions -------------------------------------------------------

// StartSession opens tracking for a job, or returns the live one.
func (s *Store) StartSession(ctx context.Context, jobID, driverID string) (Session, error) {
	var session Session
	err := s.pool.QueryRow(ctx,
		`INSERT INTO tracking_sessions (job_id, driver_id) VALUES ($1, $2)
		 ON CONFLICT (job_id) WHERE ended_at IS NULL DO UPDATE SET driver_id = EXCLUDED.driver_id
		 RETURNING id::text, job_id::text, driver_id::text, started_at, ended_at`,
		jobID, driverID).Scan(&session.ID, &session.JobID, &session.DriverID, &session.StartedAt, &session.EndedAt)
	if err != nil {
		return Session{}, fmt.Errorf("start tracking session: %w", err)
	}
	return session, nil
}

// EndSession closes tracking for a job.
func (s *Store) EndSession(ctx context.Context, jobID string) error {
	_, err := s.pool.Exec(ctx,
		`UPDATE tracking_sessions SET ended_at = now() WHERE job_id = $1 AND ended_at IS NULL`, jobID)
	if err != nil {
		return fmt.Errorf("end tracking session: %w", err)
	}
	return nil
}

// LiveSession returns the open tracking session for a job.
func (s *Store) LiveSession(ctx context.Context, jobID string) (Session, bool, error) {
	var session Session
	err := s.pool.QueryRow(ctx,
		`SELECT id::text, job_id::text, driver_id::text, started_at, ended_at
		   FROM tracking_sessions WHERE job_id = $1 AND ended_at IS NULL`, jobID).
		Scan(&session.ID, &session.JobID, &session.DriverID, &session.StartedAt, &session.EndedAt)
	if errors.Is(err, pgx.ErrNoRows) {
		return Session{}, false, nil
	}
	if err != nil {
		return Session{}, false, fmt.Errorf("load tracking session: %w", err)
	}
	return session, true, nil
}

// --- access control ----------------------------------------------------------

// AuthorizeView decides whether an actor may see a driver's live location, and
// records the decision.
//
// Document 102 scopes access: a customer sees their own active job, a driver
// their assigned jobs, a merchant the relevant delivery, operations their
// authorized scope. Every answer is logged, because "audit privileged access"
// means the log must exist before anyone asks who looked.
func (s *Store) AuthorizeView(ctx context.Context, actorID, actorRole, driverID, jobID string, scope Scope) error {
	granted, err := s.permitted(ctx, actorID, actorRole, driverID, jobID, scope)
	logErr := s.logAccess(ctx, actorID, actorRole, driverID, jobID, scope, granted)
	if logErr != nil && err == nil {
		err = logErr
	}
	if err != nil {
		return err
	}
	if !granted {
		return ErrNotPermitted
	}
	return nil
}

func (s *Store) permitted(ctx context.Context, actorID, actorRole, driverID, jobID string, scope Scope) (bool, error) {
	switch scope {
	case ScopeSelf:
		var owns bool
		err := s.pool.QueryRow(ctx,
			`SELECT EXISTS (SELECT 1 FROM drivers WHERE id = $1 AND user_id = $2)`,
			driverID, actorID).Scan(&owns)
		return owns, err

	case ScopeOwnJob:
		// A customer may watch the driver on their own job, and only while it
		// is live — document 102: "only location needed for active service".
		var permitted bool
		err := s.pool.QueryRow(ctx,
			`SELECT EXISTS (
			   SELECT 1 FROM jobs j
			   JOIN tracking_sessions t ON t.job_id = j.id AND t.ended_at IS NULL
			    WHERE j.id = $1 AND j.requester_user_id = $2 AND t.driver_id = $3)`,
			jobID, actorID, driverID).Scan(&permitted)
		return permitted, err

	case ScopeAssignedJob:
		var permitted bool
		err := s.pool.QueryRow(ctx,
			`SELECT EXISTS (
			   SELECT 1 FROM drivers d
			   JOIN jobs j ON j.assigned_driver_id = d.id
			    WHERE d.user_id = $1 AND j.id = $2 AND d.id = $3)`,
			actorID, jobID, driverID).Scan(&permitted)
		return permitted, err

	case ScopeMerchant:
		var permitted bool
		err := s.pool.QueryRow(ctx,
			`SELECT EXISTS (SELECT 1 FROM jobs WHERE id = $1 AND merchant_id IS NOT NULL)`,
			jobID).Scan(&permitted)
		return permitted, err

	case ScopeOperations:
		// Role-gated at the handler; recorded here because an operator viewing
		// a driver is exactly the privileged access document 102 wants audited.
		return actorRole == "ADMIN" || actorRole == "SUPER_ADMIN" || actorRole == "SUPPORT", nil

	default:
		return false, nil
	}
}

func (s *Store) logAccess(ctx context.Context, actorID, actorRole, driverID, jobID string, scope Scope, granted bool) error {
	var actor, driver, job any
	if actorID != "" {
		actor = actorID
	}
	if driverID != "" {
		driver = driverID
	}
	if jobID != "" {
		job = jobID
	}
	_, err := s.pool.Exec(ctx,
		`INSERT INTO location_access_log (actor_id, actor_role, driver_id, job_id, scope, granted)
		 VALUES ($1, $2, $3, $4, $5, $6)`,
		actor, actorRole, driver, job, scope, granted)
	if err != nil {
		return fmt.Errorf("log location access: %w", err)
	}
	return nil
}
