-- Pricing configuration and booking (documents 05, 34, 35, 36).
--
-- CAP-1's boundary. Document 34: "Rates are configuration, not hard-coded
-- business logic", configurable "by city, zone, job type, vehicle type and time
-- window". No rate value appears in Go anywhere.

CREATE TABLE pricing_tariffs (
    id                       uuid PRIMARY KEY DEFAULT gen_random_uuid(),
    -- The scope this tariff applies to. NULL means "any".
    job_type                 text        NOT NULL,
    vehicle_type             text        REFERENCES vehicle_types (code) ON DELETE CASCADE,
    city                     text,
    -- Document 34 requires a pricing_version on every quote so historical
    -- prices are never recomputed from current configuration.
    version                  integer     NOT NULL,
    currency                 text        NOT NULL DEFAULT 'PKR',

    -- Every amount is integer minor units (ADR-008). Every rate is integer
    -- basis points or minor-units-per-unit; there is no float in this table.
    minimum_fare_minor       bigint      NOT NULL DEFAULT 0,
    base_minor               bigint      NOT NULL DEFAULT 0,
    per_km_minor             bigint      NOT NULL DEFAULT 0,
    per_minute_minor         bigint      NOT NULL DEFAULT 0,
    waiting_per_minute_minor bigint      NOT NULL DEFAULT 0,
    loading_per_minute_minor bigint      NOT NULL DEFAULT 0,
    per_kg_minor             bigint      NOT NULL DEFAULT 0,
    service_fee_minor        bigint      NOT NULL DEFAULT 0,
    service_fee_bps          integer     NOT NULL DEFAULT 0,
    tax_bps                  integer     NOT NULL DEFAULT 0,

    -- Bounded demand adjustment (document 34: "Do not introduce uncontrolled
    -- surge"). BD-02 is unresolved, so both bounds default to 10000 bps — a
    -- multiplier of exactly 1.0, which makes the term present and inert.
    demand_min_bps           integer     NOT NULL DEFAULT 10000,
    demand_max_bps           integer     NOT NULL DEFAULT 10000,

    active_from              timestamptz NOT NULL DEFAULT now(),
    active_to                timestamptz,
    created_at               timestamptz NOT NULL DEFAULT now(),

    CONSTRAINT tariffs_currency_valid CHECK (currency = 'PKR'),
    CONSTRAINT tariffs_job_type_valid CHECK (job_type IN ('RIDE', 'PARCEL', 'GROCERY', 'CARGO', 'FREIGHT')),
    CONSTRAINT tariffs_amounts_nonneg CHECK (
        minimum_fare_minor >= 0 AND base_minor >= 0 AND per_km_minor >= 0 AND
        per_minute_minor >= 0 AND waiting_per_minute_minor >= 0 AND
        loading_per_minute_minor >= 0 AND per_kg_minor >= 0 AND service_fee_minor >= 0
    ),
    CONSTRAINT tariffs_bps_sane CHECK (
        service_fee_bps BETWEEN 0 AND 10000 AND tax_bps BETWEEN 0 AND 10000
    ),
    -- The cap that makes surge bounded rather than a promise to bound it.
    CONSTRAINT tariffs_demand_bounded CHECK (
        demand_min_bps >= 10000 AND demand_max_bps >= demand_min_bps AND demand_max_bps <= 30000
    ),
    CONSTRAINT tariffs_window_ordered CHECK (active_to IS NULL OR active_to > active_from),
    UNIQUE (job_type, vehicle_type, city, version)
);

CREATE INDEX pricing_tariffs_lookup_idx ON pricing_tariffs (job_type, vehicle_type, city, version DESC);

-- Document 34's full quote breakdown.
ALTER TABLE pricing_quotes
    ADD COLUMN vehicle_type        text REFERENCES vehicle_types (code) ON DELETE SET NULL,
    ADD COLUMN tariff_id           uuid REFERENCES pricing_tariffs (id) ON DELETE SET NULL,
    ADD COLUMN pricing_version     integer,
    ADD COLUMN requested_by        uuid REFERENCES users (id) ON DELETE CASCADE,
    ADD COLUMN base_minor          bigint NOT NULL DEFAULT 0,
    ADD COLUMN distance_minor      bigint NOT NULL DEFAULT 0,
    ADD COLUMN time_minor          bigint NOT NULL DEFAULT 0,
    ADD COLUMN service_fee_minor   bigint NOT NULL DEFAULT 0,
    ADD COLUMN waiting_minor       bigint NOT NULL DEFAULT 0,
    ADD COLUMN loading_minor       bigint NOT NULL DEFAULT 0,
    ADD COLUMN demand_minor        bigint NOT NULL DEFAULT 0,
    ADD COLUMN discount_minor      bigint NOT NULL DEFAULT 0,
    ADD COLUMN tax_minor           bigint NOT NULL DEFAULT 0,
    ADD COLUMN distance_meters     bigint,
    ADD COLUMN duration_seconds    bigint,
    -- Whether the route behind this quote was live or estimated. A quote built
    -- on a straight-line guess should not be presented like a measured one.
    ADD COLUMN route_confidence    text;

-- Price lock (document 34): once a job is confirmed against a quote, the
-- snapshot is immutable. Recomputing a historical price from current
-- configuration is how a customer gets charged a rate that did not exist when
-- they booked.
CREATE TABLE job_price_locks (
    job_id          uuid PRIMARY KEY REFERENCES jobs (id) ON DELETE CASCADE,
    quote_id        uuid        NOT NULL REFERENCES pricing_quotes (id) ON DELETE RESTRICT,
    pricing_version integer     NOT NULL,
    total_minor     bigint      NOT NULL,
    currency        text        NOT NULL,
    -- The complete quote as it stood at confirmation.
    snapshot        jsonb       NOT NULL,
    locked_at       timestamptz NOT NULL DEFAULT now(),
    CONSTRAINT price_locks_total_nonneg CHECK (total_minor >= 0)
);

-- Idempotency (documents 14, 35, 185: "client/request id + operation scope").
--
-- Job and payment creation carry an Idempotency-Key. A retried create must
-- return the original result, not make a second job — a customer whose network
-- dropped mid-request should not find two rides on the way.
CREATE TABLE idempotency_keys (
    key         text        NOT NULL,
    scope       text        NOT NULL,
    user_id     uuid        NOT NULL REFERENCES users (id) ON DELETE CASCADE,
    -- Fingerprint of the request body. A key reused with different content is
    -- a client bug, and returning the first result would silently discard the
    -- second request.
    request_hash bytea      NOT NULL,
    resource_id uuid,
    response    jsonb,
    created_at  timestamptz NOT NULL DEFAULT now(),
    PRIMARY KEY (scope, key, user_id)
);

CREATE INDEX idempotency_keys_created_idx ON idempotency_keys (created_at);

-- Cancellation records. BD-01 is unresolved: the tier structure from document
-- 005 is representable here, the amounts are not set anywhere.
CREATE TABLE job_cancellations (
    job_id            uuid PRIMARY KEY REFERENCES jobs (id) ON DELETE CASCADE,
    cancelled_by      text        NOT NULL,
    actor_id          uuid        REFERENCES users (id) ON DELETE SET NULL,
    reason            text,
    -- Which document 005 tier applied. The tier is determined by state; the
    -- fee it implies is configuration that does not exist yet.
    tier              text        NOT NULL,
    fee_minor         bigint,
    compensation_minor bigint,
    currency          text        NOT NULL DEFAULT 'PKR',
    created_at        timestamptz NOT NULL DEFAULT now(),
    CONSTRAINT cancellations_actor_valid CHECK (
        cancelled_by IN ('CUSTOMER', 'DRIVER', 'MERCHANT', 'ADMIN', 'SUPPORT', 'SYSTEM')),
    CONSTRAINT cancellations_tier_valid CHECK (
        tier IN ('BEFORE_ASSIGNMENT', 'AFTER_ASSIGNMENT', 'AFTER_ARRIVAL', 'AFTER_START')),
    CONSTRAINT cancellations_amounts_nonneg CHECK (
        (fee_minor IS NULL OR fee_minor >= 0) AND
        (compensation_minor IS NULL OR compensation_minor >= 0))
);
