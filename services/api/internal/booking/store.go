package booking

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"time"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"
	"github.com/sarmadkung/rideme/services/api/internal/pricing"
	"github.com/sarmadkung/rideme/services/api/pkg/money"
)

// Store persists tariffs, quotes, price locks, idempotency keys and
// cancellations.
type Store struct{ pool *pgxpool.Pool }

func NewStore(pool *pgxpool.Pool) *Store { return &Store{pool: pool} }

// Tariff finds the configuration for a service, most specific first.
//
// Specificity order matters: a city with its own rates must win over the
// national default, and a vehicle-specific rate over a service-wide one.
// Without the ordering a new city tariff would be ignored until someone
// noticed the fares had not changed.
func (s *Store) Tariff(ctx context.Context, jobType, vehicleType, city string) (pricing.Tariff, error) {
	var t pricing.Tariff
	var vType, tCity *string
	err := s.pool.QueryRow(ctx,
		`SELECT id::text, job_type, vehicle_type, city, version, currency,
		        minimum_fare_minor, base_minor, per_km_minor, per_minute_minor,
		        waiting_per_minute_minor, loading_per_minute_minor, per_kg_minor,
		        service_fee_minor, service_fee_bps, tax_bps, demand_min_bps, demand_max_bps
		   FROM pricing_tariffs
		  WHERE job_type = $1
		    AND (vehicle_type = $2 OR vehicle_type IS NULL)
		    AND (city = $3 OR city IS NULL)
		    AND active_from <= now()
		    AND (active_to IS NULL OR active_to > now())
		  ORDER BY (vehicle_type IS NOT NULL) DESC, (city IS NOT NULL) DESC, version DESC
		  LIMIT 1`,
		jobType, vehicleType, city).
		Scan(&t.ID, &t.JobType, &vType, &tCity, &t.Version, &t.Currency,
			&t.MinimumFareMinor, &t.BaseMinor, &t.PerKMMinor, &t.PerMinuteMinor,
			&t.WaitingPerMinuteMinor, &t.LoadingPerMinuteMinor, &t.PerKGMinor,
			&t.ServiceFeeMinor, &t.ServiceFeeBPS, &t.TaxBPS, &t.DemandMinBPS, &t.DemandMaxBPS)
	if errors.Is(err, pgx.ErrNoRows) {
		return pricing.Tariff{}, pricing.ErrNoTariff
	}
	if err != nil {
		return pricing.Tariff{}, fmt.Errorf("load tariff: %w", err)
	}
	if vType != nil {
		t.VehicleType = *vType
	}
	if tCity != nil {
		t.City = *tCity
	}
	return t, nil
}

// SaveTariff stores pricing configuration.
func (s *Store) SaveTariff(ctx context.Context, t pricing.Tariff) (string, error) {
	var vehicleType, city any
	if t.VehicleType != "" {
		vehicleType = t.VehicleType
	}
	if t.City != "" {
		city = t.City
	}
	currency := t.Currency
	if currency == "" {
		currency = money.PKR
	}
	var id string
	err := s.pool.QueryRow(ctx,
		`INSERT INTO pricing_tariffs
		   (job_type, vehicle_type, city, version, currency, minimum_fare_minor, base_minor,
		    per_km_minor, per_minute_minor, waiting_per_minute_minor, loading_per_minute_minor,
		    per_kg_minor, service_fee_minor, service_fee_bps, tax_bps, demand_min_bps, demand_max_bps)
		 VALUES ($1,$2,$3,$4,$5,$6,$7,$8,$9,$10,$11,$12,$13,$14,$15,
		         COALESCE(NULLIF($16, 0), 10000), COALESCE(NULLIF($17, 0), 10000))
		 RETURNING id::text`,
		t.JobType, vehicleType, city, t.Version, currency, t.MinimumFareMinor, t.BaseMinor,
		t.PerKMMinor, t.PerMinuteMinor, t.WaitingPerMinuteMinor, t.LoadingPerMinuteMinor,
		t.PerKGMinor, t.ServiceFeeMinor, t.ServiceFeeBPS, t.TaxBPS, t.DemandMinBPS, t.DemandMaxBPS).
		Scan(&id)
	if err != nil {
		return "", fmt.Errorf("save tariff: %w", err)
	}
	return id, nil
}

// StoredQuote is a quote as persisted.
type StoredQuote struct {
	ID          string
	RequestedBy string
	Total       money.Amount
	Version     int
	ExpiresAt   *time.Time
	Used        bool
	Snapshot    []byte
}

// SaveQuote persists a priced quote with its full breakdown.
func (s *Store) SaveQuote(ctx context.Context, requestedBy string, q pricing.Quote) (string, error) {
	snapshot, err := json.Marshal(q)
	if err != nil {
		return "", fmt.Errorf("encode quote: %w", err)
	}
	component := func(name pricing.Component) int64 {
		if amount, ok := q.Component(name); ok {
			return amount.Minor
		}
		return 0
	}
	var tariffID any
	if q.TariffID != "" {
		tariffID = q.TariffID
	}
	var id string
	err = s.pool.QueryRow(ctx,
		`INSERT INTO pricing_quotes
		   (job_type, vehicle_type, tariff_id, pricing_version, requested_by,
		    amount_minor, currency, base_minor, distance_minor, time_minor,
		    service_fee_minor, waiting_minor, loading_minor, demand_minor,
		    discount_minor, tax_minor, distance_meters, duration_seconds,
		    route_confidence, breakdown, expires_at)
		 VALUES ($1,$2,$3,$4,$5,$6,$7,$8,$9,$10,$11,$12,$13,$14,$15,$16,$17,$18,$19,$20,$21)
		 RETURNING id::text`,
		q.JobType, q.VehicleType, tariffID, q.PricingVersion, requestedBy,
		q.Total.Minor, q.Currency,
		component(pricing.ComponentBase), component(pricing.ComponentDistance),
		component(pricing.ComponentTime), component(pricing.ComponentServiceFee),
		component(pricing.ComponentWaiting), component(pricing.ComponentLoading),
		component(pricing.ComponentDemand), -component(pricing.ComponentDiscount),
		component(pricing.ComponentTax),
		q.DistanceMeters, q.DurationSeconds, string(q.RouteConfidence), snapshot, q.ExpiresAt).
		Scan(&id)
	if err != nil {
		return "", fmt.Errorf("save quote: %w", err)
	}
	return id, nil
}

// QuoteByID loads a stored quote and reports whether it has been used.
func (s *Store) QuoteByID(ctx context.Context, id string) (StoredQuote, error) {
	var q StoredQuote
	var minor int64
	var currency money.Currency
	var requestedBy *string
	var version *int
	err := s.pool.QueryRow(ctx,
		`SELECT q.id::text, q.requested_by::text, q.amount_minor, q.currency,
		        q.pricing_version, q.expires_at, q.breakdown,
		        EXISTS (SELECT 1 FROM job_price_locks l WHERE l.quote_id = q.id)
		   FROM pricing_quotes q WHERE q.id = $1`, id).
		Scan(&q.ID, &requestedBy, &minor, &currency, &version, &q.ExpiresAt, &q.Snapshot, &q.Used)
	if errors.Is(err, pgx.ErrNoRows) {
		return StoredQuote{}, pgx.ErrNoRows
	}
	if err != nil {
		return StoredQuote{}, fmt.Errorf("load quote: %w", err)
	}
	if requestedBy != nil {
		q.RequestedBy = *requestedBy
	}
	if version != nil {
		q.Version = *version
	}
	if q.Total, err = money.New(minor, currency); err != nil {
		return StoredQuote{}, err
	}
	return q, nil
}

// LockPrice stores the immutable pricing snapshot for a confirmed job
// (document 034).
func (s *Store) LockPrice(ctx context.Context, jobID string, quote StoredQuote) error {
	_, err := s.pool.Exec(ctx,
		`INSERT INTO job_price_locks (job_id, quote_id, pricing_version, total_minor, currency, snapshot)
		 VALUES ($1, $2, $3, $4, $5, $6)`,
		jobID, quote.ID, quote.Version, quote.Total.Minor, quote.Total.Currency, quote.Snapshot)
	if err != nil {
		return fmt.Errorf("lock price: %w", err)
	}
	return nil
}

// LockedPrice returns the price a job was confirmed at.
func (s *Store) LockedPrice(ctx context.Context, jobID string) (money.Amount, int, error) {
	var minor int64
	var currency money.Currency
	var version int
	err := s.pool.QueryRow(ctx,
		`SELECT total_minor, currency, pricing_version FROM job_price_locks WHERE job_id = $1`, jobID).
		Scan(&minor, &currency, &version)
	if errors.Is(err, pgx.ErrNoRows) {
		return money.Amount{}, 0, pgx.ErrNoRows
	}
	if err != nil {
		return money.Amount{}, 0, fmt.Errorf("load locked price: %w", err)
	}
	amount, err := money.New(minor, currency)
	return amount, version, err
}

// --- idempotency -------------------------------------------------------------

// LookupIdempotent returns the resource a key already produced.
//
// A key seen with a *different* request body returns ErrKeyReused rather than
// the original result: replaying the first response would silently discard the
// second request, which is worse than refusing it.
func (s *Store) LookupIdempotent(ctx context.Context, scope, key, userID string, fingerprint []byte) (string, bool, error) {
	var resourceID *string
	var stored []byte
	err := s.pool.QueryRow(ctx,
		`SELECT resource_id::text, request_hash FROM idempotency_keys
		  WHERE scope = $1 AND key = $2 AND user_id = $3`, scope, key, userID).
		Scan(&resourceID, &stored)
	if errors.Is(err, pgx.ErrNoRows) {
		return "", false, nil
	}
	if err != nil {
		return "", false, fmt.Errorf("look up idempotency key: %w", err)
	}
	if !bytes.Equal(stored, fingerprint) {
		return "", false, ErrKeyReused
	}
	if resourceID == nil {
		return "", false, nil
	}
	return *resourceID, true, nil
}

// RecordIdempotent stores the outcome of an operation against its key.
func (s *Store) RecordIdempotent(ctx context.Context, scope, key, userID string, fingerprint []byte, resourceID string) error {
	_, err := s.pool.Exec(ctx,
		`INSERT INTO idempotency_keys (scope, key, user_id, request_hash, resource_id)
		 VALUES ($1, $2, $3, $4, $5)
		 ON CONFLICT (scope, key, user_id) DO UPDATE SET resource_id = EXCLUDED.resource_id
		  WHERE idempotency_keys.resource_id IS NULL`,
		scope, key, userID, fingerprint, resourceID)
	if err != nil {
		return fmt.Errorf("record idempotency key: %w", err)
	}
	return nil
}

// ClaimIdempotencyKey reserves a key before the work runs, so two concurrent
// requests with the same key cannot both proceed.
//
// Checking then inserting has a window between the two; inserting first and
// letting the primary key arbitrate does not.
func (s *Store) ClaimIdempotencyKey(ctx context.Context, scope, key, userID string, fingerprint []byte) (claimed bool, err error) {
	tag, err := s.pool.Exec(ctx,
		`INSERT INTO idempotency_keys (scope, key, user_id, request_hash)
		 VALUES ($1, $2, $3, $4) ON CONFLICT DO NOTHING`,
		scope, key, userID, fingerprint)
	if err != nil {
		return false, fmt.Errorf("claim idempotency key: %w", err)
	}
	return tag.RowsAffected() == 1, nil
}

// --- cancellations -----------------------------------------------------------

// RecordCancellation stores which tier applied. The fee is deliberately null:
// BD-01 is unresolved and no amount is invented here.
func (s *Store) RecordCancellation(ctx context.Context, jobID, actorType, actorID, reason, tier string) error {
	var actor, reasonText any
	if actorID != "" {
		actor = actorID
	}
	if reason != "" {
		reasonText = reason
	}
	_, err := s.pool.Exec(ctx,
		`INSERT INTO job_cancellations (job_id, cancelled_by, actor_id, reason, tier)
		 VALUES ($1, $2, $3, $4, $5) ON CONFLICT (job_id) DO NOTHING`,
		jobID, actorType, actor, reasonText, tier)
	if err != nil {
		return fmt.Errorf("record cancellation: %w", err)
	}
	return nil
}

// Cancellation returns the recorded cancellation for a job.
func (s *Store) Cancellation(ctx context.Context, jobID string) (tier, reason string, found bool, err error) {
	var reasonText *string
	err = s.pool.QueryRow(ctx,
		`SELECT tier, reason FROM job_cancellations WHERE job_id = $1`, jobID).Scan(&tier, &reasonText)
	if errors.Is(err, pgx.ErrNoRows) {
		return "", "", false, nil
	}
	if err != nil {
		return "", "", false, fmt.Errorf("load cancellation: %w", err)
	}
	if reasonText != nil {
		reason = *reasonText
	}
	return tier, reason, true, nil
}
