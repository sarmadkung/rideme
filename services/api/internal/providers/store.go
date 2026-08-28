package providers

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"time"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"
	"github.com/sarmadkung/rideme/services/api/internal/eligibility"
)

type Store struct{ pool *pgxpool.Pool }

func NewStore(pool *pgxpool.Pool) *Store { return &Store{pool: pool} }

// --- drivers -----------------------------------------------------------------

const driverColumns = `id, user_id, verification_status, status,
	COALESCE(active_vehicle_id::text, ''), rating, completion_rate,
	cancellation_rate, acceptance_rate, created_at, updated_at`

func scanDriver(row pgx.Row) (Driver, error) {
	var d Driver
	err := row.Scan(&d.ID, &d.UserID, &d.VerificationStatus, &d.Status, &d.ActiveVehicleID,
		&d.Rating, &d.CompletionRate, &d.CancellationRate, &d.AcceptanceRate,
		&d.CreatedAt, &d.UpdatedAt)
	return d, err
}

// EnsureDriver creates the driver record for a user, or returns the existing
// one. Idempotent so "become a driver" can be retried — document 29 requires
// onboarding to be resumable.
func (s *Store) EnsureDriver(ctx context.Context, userID string) (Driver, error) {
	driver, err := scanDriver(s.pool.QueryRow(ctx,
		`INSERT INTO drivers (user_id) VALUES ($1)
		 ON CONFLICT (user_id) DO UPDATE SET updated_at = drivers.updated_at
		 RETURNING `+driverColumns, userID))
	if err != nil {
		return Driver{}, fmt.Errorf("ensure driver: %w", err)
	}
	return driver, nil
}

func (s *Store) DriverByID(ctx context.Context, id string) (Driver, error) {
	driver, err := scanDriver(s.pool.QueryRow(ctx,
		`SELECT `+driverColumns+` FROM drivers WHERE id = $1`, id))
	if errors.Is(err, pgx.ErrNoRows) {
		return Driver{}, ErrDriverNotFound
	}
	if err != nil {
		return Driver{}, fmt.Errorf("load driver: %w", err)
	}
	return driver, nil
}

func (s *Store) DriverByUserID(ctx context.Context, userID string) (Driver, error) {
	driver, err := scanDriver(s.pool.QueryRow(ctx,
		`SELECT `+driverColumns+` FROM drivers WHERE user_id = $1`, userID))
	if errors.Is(err, pgx.ErrNoRows) {
		return Driver{}, ErrDriverNotFound
	}
	if err != nil {
		return Driver{}, fmt.Errorf("load driver: %w", err)
	}
	return driver, nil
}

// TransitionVerification moves a driver's verification state, compare-and-set
// and recording the review (document 29: "Every review action is audited").
func (s *Store) TransitionVerification(ctx context.Context, driverID string, from, to VerificationStatus, reviewerID, reason string) (Driver, error) {
	if err := VerificationMachine.Validate(from, to); err != nil {
		return Driver{}, err
	}
	return s.transitionDriverColumn(ctx, "verification_status", driverID, string(from), string(to),
		func(tx pgx.Tx) error {
			return appendReview(ctx, tx, "DRIVER", driverID, reviewerID, string(from), string(to), reason)
		})
}

// TransitionAvailability moves a driver's operational state.
func (s *Store) TransitionAvailability(ctx context.Context, driverID string, from, to AvailabilityStatus) (Driver, error) {
	if err := AvailabilityMachine.Validate(from, to); err != nil {
		return Driver{}, err
	}
	return s.transitionDriverColumn(ctx, "status", driverID, string(from), string(to), nil)
}

func (s *Store) transitionDriverColumn(ctx context.Context, column, driverID, from, to string, after func(pgx.Tx) error) (Driver, error) {
	tx, err := s.pool.Begin(ctx)
	if err != nil {
		return Driver{}, fmt.Errorf("begin: %w", err)
	}
	defer func() { _ = tx.Rollback(ctx) }()

	// The column name is one of two package constants, never caller input.
	query := `UPDATE drivers SET ` + column + ` = $3, updated_at = now()
	           WHERE id = $1 AND ` + column + ` = $2 RETURNING ` + driverColumns
	driver, err := scanDriver(tx.QueryRow(ctx, query, driverID, from, to))
	if errors.Is(err, pgx.ErrNoRows) {
		return Driver{}, fmt.Errorf("%w: expected %s %s", ErrStale, column, from)
	}
	if err != nil {
		return Driver{}, fmt.Errorf("transition driver: %w", err)
	}
	if after != nil {
		if err := after(tx); err != nil {
			return Driver{}, err
		}
	}
	if err := tx.Commit(ctx); err != nil {
		return Driver{}, fmt.Errorf("commit: %w", err)
	}
	return driver, nil
}

// SetActiveVehicle selects the vehicle a driver works with (document 30).
//
// The vehicle must be verified and owned by the driver. Both are checked in
// the statement rather than beforehand, so a vehicle suspended between the
// check and the write cannot slip through.
func (s *Store) SetActiveVehicle(ctx context.Context, driverID, vehicleID string) (Driver, error) {
	driver, err := scanDriver(s.pool.QueryRow(ctx,
		`UPDATE drivers d SET active_vehicle_id = $2, updated_at = now()
		  WHERE d.id = $1
		    AND EXISTS (
		      SELECT 1 FROM vehicles v
		       WHERE v.id = $2
		         AND v.owner_user_id = d.user_id
		         AND v.verification_status = 'VERIFIED')
		  RETURNING `+driverColumns, driverID, vehicleID))
	if errors.Is(err, pgx.ErrNoRows) {
		return Driver{}, ErrVehicleNotReady
	}
	if err != nil {
		return Driver{}, fmt.Errorf("set active vehicle: %w", err)
	}
	return driver, nil
}

// --- vehicles ----------------------------------------------------------------

const vehicleColumns = `id, owner_user_id, type, COALESCE(make, ''), COALESCE(model, ''),
	year, COALESCE(color, ''), plate_number, capacity_kg, dimensions,
	verification_status, created_at, updated_at`

func scanVehicle(row pgx.Row) (Vehicle, error) {
	var v Vehicle
	var dims []byte
	err := row.Scan(&v.ID, &v.OwnerUserID, &v.Type, &v.Make, &v.Model, &v.Year, &v.Color,
		&v.PlateNumber, &v.CapacityKG, &dims, &v.VerificationStatus, &v.CreatedAt, &v.UpdatedAt)
	if err == nil && len(dims) > 0 {
		_ = json.Unmarshal(dims, &v.Dimensions)
	}
	return v, err
}

// RegisterVehicle stores a vehicle and derives its capabilities from its type.
//
// Capabilities are derived here, never accepted from the caller — document 30
// is explicit that the backend determines them. The registration payload has
// no capability field at all, which is the strongest form of that rule.
func (s *Store) RegisterVehicle(ctx context.Context, v Vehicle) (Vehicle, error) {
	tx, err := s.pool.Begin(ctx)
	if err != nil {
		return Vehicle{}, fmt.Errorf("begin: %w", err)
	}
	defer func() { _ = tx.Rollback(ctx) }()

	var dims any
	if v.Dimensions != nil {
		encoded, err := json.Marshal(v.Dimensions)
		if err != nil {
			return Vehicle{}, fmt.Errorf("encode dimensions: %w", err)
		}
		dims = encoded
	}

	created, err := scanVehicle(tx.QueryRow(ctx,
		`INSERT INTO vehicles (owner_user_id, type, make, model, year, color, plate_number, capacity_kg, dimensions)
		 VALUES ($1, $2, NULLIF($3, ''), NULLIF($4, ''), $5, NULLIF($6, ''), $7, $8, $9)
		 RETURNING `+vehicleColumns,
		v.OwnerUserID, v.Type, v.Make, v.Model, v.Year, v.Color, v.PlateNumber, v.CapacityKG, dims))
	if err != nil {
		return Vehicle{}, fmt.Errorf("register vehicle: %w", err)
	}

	if _, err := tx.Exec(ctx,
		`INSERT INTO vehicle_capabilities (vehicle_id, capability, source)
		 SELECT $1, capability, 'DERIVED' FROM vehicle_type_capabilities WHERE vehicle_type = $2
		 ON CONFLICT DO NOTHING`,
		created.ID, created.Type); err != nil {
		return Vehicle{}, fmt.Errorf("derive capabilities: %w", err)
	}

	if err := tx.Commit(ctx); err != nil {
		return Vehicle{}, fmt.Errorf("commit: %w", err)
	}
	created.Capabilities, err = s.CapabilitiesOf(ctx, created.ID)
	if err != nil {
		return Vehicle{}, err
	}
	return created, nil
}

func (s *Store) VehicleByID(ctx context.Context, id string) (Vehicle, error) {
	vehicle, err := scanVehicle(s.pool.QueryRow(ctx,
		`SELECT `+vehicleColumns+` FROM vehicles WHERE id = $1`, id))
	if errors.Is(err, pgx.ErrNoRows) {
		return Vehicle{}, ErrVehicleNotFound
	}
	if err != nil {
		return Vehicle{}, fmt.Errorf("load vehicle: %w", err)
	}
	if vehicle.Capabilities, err = s.CapabilitiesOf(ctx, id); err != nil {
		return Vehicle{}, err
	}
	return vehicle, nil
}

// VehiclesOfOwner lists a user's vehicles.
func (s *Store) VehiclesOfOwner(ctx context.Context, userID string) ([]Vehicle, error) {
	rows, err := s.pool.Query(ctx,
		`SELECT `+vehicleColumns+` FROM vehicles WHERE owner_user_id = $1 ORDER BY created_at`, userID)
	if err != nil {
		return nil, fmt.Errorf("list vehicles: %w", err)
	}
	defer rows.Close()

	var out []Vehicle
	for rows.Next() {
		vehicle, err := scanVehicle(rows)
		if err != nil {
			return nil, err
		}
		out = append(out, vehicle)
	}
	if err := rows.Err(); err != nil {
		return nil, err
	}
	for i := range out {
		if out[i].Capabilities, err = s.CapabilitiesOf(ctx, out[i].ID); err != nil {
			return nil, err
		}
	}
	return out, nil
}

// CapabilitiesOf returns a vehicle's effective capabilities.
func (s *Store) CapabilitiesOf(ctx context.Context, vehicleID string) ([]Capability, error) {
	rows, err := s.pool.Query(ctx,
		`SELECT capability, source FROM vehicle_capabilities WHERE vehicle_id = $1 ORDER BY capability`, vehicleID)
	if err != nil {
		return nil, fmt.Errorf("load capabilities: %w", err)
	}
	defer rows.Close()

	var out []Capability
	for rows.Next() {
		var c Capability
		if err := rows.Scan(&c.Code, &c.Source); err != nil {
			return nil, err
		}
		out = append(out, c)
	}
	return out, rows.Err()
}

// GrantCapability adds a capability an admin has decided applies beyond what
// the vehicle type implies — an equipped vehicle, a market exception.
func (s *Store) GrantCapability(ctx context.Context, vehicleID, capability string) error {
	_, err := s.pool.Exec(ctx,
		`INSERT INTO vehicle_capabilities (vehicle_id, capability, source)
		 VALUES ($1, $2, 'ADMIN') ON CONFLICT (vehicle_id, capability) DO UPDATE SET source = 'ADMIN'`,
		vehicleID, capability)
	if err != nil {
		return fmt.Errorf("grant capability: %w", err)
	}
	return nil
}

// TransitionVehicle moves a vehicle's verification state and records the review.
func (s *Store) TransitionVehicle(ctx context.Context, vehicleID string, from, to VehicleStatus, reviewerID, reason string) (Vehicle, error) {
	if err := VehicleMachine.Validate(from, to); err != nil {
		return Vehicle{}, err
	}
	tx, err := s.pool.Begin(ctx)
	if err != nil {
		return Vehicle{}, fmt.Errorf("begin: %w", err)
	}
	defer func() { _ = tx.Rollback(ctx) }()

	vehicle, err := scanVehicle(tx.QueryRow(ctx,
		`UPDATE vehicles SET verification_status = $3, updated_at = now()
		  WHERE id = $1 AND verification_status = $2 RETURNING `+vehicleColumns,
		vehicleID, from, to))
	if errors.Is(err, pgx.ErrNoRows) {
		return Vehicle{}, fmt.Errorf("%w: expected %s", ErrStale, from)
	}
	if err != nil {
		return Vehicle{}, fmt.Errorf("transition vehicle: %w", err)
	}

	// A vehicle that stops being verified must stop being anyone's active
	// vehicle, or a driver keeps working on a suspended vehicle until they
	// next change it.
	if to != VehicleVerified {
		if _, err := tx.Exec(ctx,
			`UPDATE drivers SET active_vehicle_id = NULL, updated_at = now() WHERE active_vehicle_id = $1`,
			vehicleID); err != nil {
			return Vehicle{}, fmt.Errorf("clear active vehicle: %w", err)
		}
	}
	if err := appendReview(ctx, tx, "VEHICLE", vehicleID, reviewerID, string(from), string(to), reason); err != nil {
		return Vehicle{}, err
	}
	if err := tx.Commit(ctx); err != nil {
		return Vehicle{}, fmt.Errorf("commit: %w", err)
	}
	return vehicle, nil
}

// --- documents ---------------------------------------------------------------

const documentColumns = `id, owner_type, COALESCE(driver_id::text, ''), COALESCE(vehicle_id::text, ''),
	type, COALESCE(number, ''), COALESCE(file_key, ''), issued_at, expires_at,
	status, COALESCE(rejection_reason, ''), created_at, updated_at`

func scanDocument(row pgx.Row) (Document, error) {
	var d Document
	err := row.Scan(&d.ID, &d.OwnerType, &d.DriverID, &d.VehicleID, &d.Type, &d.Number,
		&d.FileKey, &d.IssuedAt, &d.ExpiresAt, &d.Status, &d.RejectionReason,
		&d.CreatedAt, &d.UpdatedAt)
	return d, err
}

// AddDocument records a submitted credential.
func (s *Store) AddDocument(ctx context.Context, d Document) (Document, error) {
	var driverID, vehicleID any
	if d.DriverID != "" {
		driverID = d.DriverID
	}
	if d.VehicleID != "" {
		vehicleID = d.VehicleID
	}
	created, err := scanDocument(s.pool.QueryRow(ctx,
		`INSERT INTO documents (owner_type, driver_id, vehicle_id, type, number, file_key, issued_at, expires_at)
		 VALUES ($1, $2, $3, $4, NULLIF($5, ''), NULLIF($6, ''), $7, $8)
		 RETURNING `+documentColumns,
		d.OwnerType, driverID, vehicleID, d.Type, d.Number, d.FileKey, d.IssuedAt, d.ExpiresAt))
	if err != nil {
		return Document{}, fmt.Errorf("add document: %w", err)
	}
	return created, nil
}

// ReviewDocument approves or rejects a document, recording the decision.
func (s *Store) ReviewDocument(ctx context.Context, id string, to DocumentStatus, reviewerID, reason string) (Document, error) {
	tx, err := s.pool.Begin(ctx)
	if err != nil {
		return Document{}, fmt.Errorf("begin: %w", err)
	}
	defer func() { _ = tx.Rollback(ctx) }()

	var from DocumentStatus
	if err := tx.QueryRow(ctx, `SELECT status FROM documents WHERE id = $1`, id).Scan(&from); err != nil {
		return Document{}, fmt.Errorf("load document: %w", err)
	}
	doc, err := scanDocument(tx.QueryRow(ctx,
		`UPDATE documents SET status = $2, rejection_reason = NULLIF($3, ''), updated_at = now()
		  WHERE id = $1 RETURNING `+documentColumns, id, to, reason))
	if err != nil {
		return Document{}, fmt.Errorf("review document: %w", err)
	}
	if err := appendReview(ctx, tx, "DOCUMENT", id, reviewerID, string(from), string(to), reason); err != nil {
		return Document{}, err
	}
	if err := tx.Commit(ctx); err != nil {
		return Document{}, fmt.Errorf("commit: %w", err)
	}
	return doc, nil
}

// ExpireLapsedDocuments marks verified documents whose expiry has passed.
//
// Document 29 requires a scheduled process for this. The sweep is here; what
// runs it is a background worker, which arrives with the worker framework.
func (s *Store) ExpireLapsedDocuments(ctx context.Context, asOf time.Time) (int64, error) {
	tag, err := s.pool.Exec(ctx,
		`UPDATE documents SET status = 'EXPIRED', updated_at = now()
		  WHERE status = 'VERIFIED' AND expires_at IS NOT NULL AND expires_at <= $1`, asOf)
	if err != nil {
		return 0, fmt.Errorf("expire documents: %w", err)
	}
	return tag.RowsAffected(), nil
}

// SetRequirement configures a mandatory document for a market and vehicle type.
func (s *Store) SetRequirement(ctx context.Context, r Requirement) error {
	var vehicleType any
	if r.VehicleType != "" {
		vehicleType = r.VehicleType
	}
	market := r.Market
	if market == "" {
		market = "PK"
	}
	_, err := s.pool.Exec(ctx,
		`INSERT INTO document_requirements (market, owner_type, vehicle_type, type, mandatory)
		 VALUES ($1, $2, $3, $4, $5)
		 ON CONFLICT (market, owner_type, vehicle_type, type) DO UPDATE SET mandatory = EXCLUDED.mandatory`,
		market, r.OwnerType, vehicleType, r.Type, r.Mandatory)
	if err != nil {
		return fmt.Errorf("set requirement: %w", err)
	}
	return nil
}

// --- eligibility input -------------------------------------------------------

// Candidate assembles everything the eligibility rules need for a driver, in
// one round trip per concern.
//
// It returns eligibility's own types rather than this package's, so the rules
// depend on no domain package and dispatch can call them with data from
// anywhere.
func (s *Store) Candidate(ctx context.Context, driverID string, market string) (eligibility.Driver, eligibility.Vehicle, error) {
	driver, err := s.DriverByID(ctx, driverID)
	if err != nil {
		return eligibility.Driver{}, eligibility.Vehicle{}, err
	}

	out := eligibility.Driver{
		ID:                 driver.ID,
		VerificationStatus: string(driver.VerificationStatus),
		Status:             string(driver.Status),
	}
	if out.Documents, err = s.mandatoryDocuments(ctx, "DRIVER", driver.ID, "", market); err != nil {
		return eligibility.Driver{}, eligibility.Vehicle{}, err
	}

	if driver.ActiveVehicleID == "" {
		return out, eligibility.Vehicle{}, nil
	}
	vehicle, err := s.VehicleByID(ctx, driver.ActiveVehicleID)
	if err != nil {
		return eligibility.Driver{}, eligibility.Vehicle{}, err
	}

	capabilities := make([]string, len(vehicle.Capabilities))
	for i, c := range vehicle.Capabilities {
		capabilities[i] = c.Code
	}
	ev := eligibility.Vehicle{
		ID:                 vehicle.ID,
		Type:               vehicle.Type,
		VerificationStatus: string(vehicle.VerificationStatus),
		Capabilities:       capabilities,
		CapacityKG:         vehicle.CapacityKG,
	}
	if seats, ok := vehicle.Dimensions["passenger_seats"].(float64); ok {
		count := int(seats)
		ev.PassengerSeats = &count
	}
	if ev.Documents, err = s.mandatoryDocuments(ctx, "VEHICLE", "", vehicle.ID, market, vehicle.Type); err != nil {
		return eligibility.Driver{}, eligibility.Vehicle{}, err
	}
	return out, ev, nil
}

// mandatoryDocuments returns one entry per *configured requirement*, not per
// submitted document.
//
// The direction matters. Listing what was submitted answers "are these valid?";
// listing what is required answers "is anything missing?" — and a driver who
// never uploaded a licence has no row to be invalid. A LEFT JOIN from the
// requirements makes the absent document a PENDING one, which the rules refuse.
func (s *Store) mandatoryDocuments(ctx context.Context, ownerType, driverID, vehicleID, market string, vehicleType ...string) ([]eligibility.Document, error) {
	if market == "" {
		market = "PK"
	}
	var vType any
	if len(vehicleType) > 0 && vehicleType[0] != "" {
		vType = vehicleType[0]
	}
	var owner any
	if ownerType == "DRIVER" {
		owner = driverID
	} else {
		owner = vehicleID
	}

	rows, err := s.pool.Query(ctx,
		`SELECT r.type,
		        COALESCE(d.status, 'PENDING'),
		        d.expires_at
		   FROM document_requirements r
		   LEFT JOIN documents d
		     ON d.type = r.type
		    AND d.owner_type = r.owner_type
		    AND (CASE WHEN r.owner_type = 'DRIVER' THEN d.driver_id ELSE d.vehicle_id END) = $1::uuid
		  WHERE r.market = $2
		    AND r.owner_type = $3
		    AND r.mandatory
		    AND (r.vehicle_type IS NULL OR r.vehicle_type = $4)`,
		owner, market, ownerType, vType)
	if err != nil {
		return nil, fmt.Errorf("load required documents: %w", err)
	}
	defer rows.Close()

	var out []eligibility.Document
	for rows.Next() {
		var doc eligibility.Document
		var expires *time.Time
		if err := rows.Scan(&doc.Type, &doc.Status, &expires); err != nil {
			return nil, err
		}
		doc.ExpiresAt = expires
		doc.Mandatory = true
		out = append(out, doc)
	}
	return out, rows.Err()
}

func appendReview(ctx context.Context, tx pgx.Tx, subjectType, subjectID, reviewerID, from, to, reason string) error {
	var reviewer, fromStatus, reasonText any
	if reviewerID != "" {
		reviewer = reviewerID
	}
	if from != "" {
		fromStatus = from
	}
	if reason != "" {
		reasonText = reason
	}
	if _, err := tx.Exec(ctx,
		`INSERT INTO verification_reviews (subject_type, subject_id, reviewer_id, from_status, to_status, reason)
		 VALUES ($1, $2, $3, $4, $5, $6)`,
		subjectType, subjectID, reviewer, fromStatus, to, reasonText); err != nil {
		return fmt.Errorf("append review: %w", err)
	}
	return nil
}
