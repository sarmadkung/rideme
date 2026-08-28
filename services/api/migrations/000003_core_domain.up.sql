-- The core entity model (documents 04, 13, 15, 16, 26).
--
-- The central decision, from document 04: **all operational work is a Job.**
-- A ride, a parcel, a grocery order and a cargo haul are one table with a type,
-- not four parallel booking entities. Forking them is the most expensive
-- mistake available in this codebase — dispatch, pricing, tracking, payment and
-- the operator console would each have to learn four shapes instead of one.

-- --- providers and vehicles --------------------------------------------------

-- A driver is a role a user holds, not a separate account (document 13:
-- drivers.user_id).
CREATE TABLE drivers (
    id                  uuid PRIMARY KEY DEFAULT gen_random_uuid(),
    user_id             uuid        NOT NULL UNIQUE REFERENCES users (id) ON DELETE CASCADE,
    verification_status text        NOT NULL DEFAULT 'PENDING_VERIFICATION',
    -- Operational state, document 16.
    status              text        NOT NULL DEFAULT 'OFFLINE',
    -- Reliability figures document 13 names. Stored as they are computed;
    -- rating feeds the dispatch score's driver_reliability term (document 05).
    rating              numeric(3, 2),
    completion_rate     numeric(5, 4),
    cancellation_rate   numeric(5, 4),
    acceptance_rate     numeric(5, 4),
    created_at          timestamptz NOT NULL DEFAULT now(),
    updated_at          timestamptz NOT NULL DEFAULT now(),
    CONSTRAINT drivers_verification_valid CHECK (
        verification_status IN ('PENDING_VERIFICATION', 'VERIFIED', 'SUSPENDED', 'EXPIRED')
    ),
    CONSTRAINT drivers_status_valid CHECK (
        status IN ('OFFLINE', 'AVAILABLE', 'OFFERED', 'ACCEPTED', 'ON_TRIP', 'PAUSED', 'SUSPENDED', 'BLOCKED')
    ),
    CONSTRAINT drivers_rating_range CHECK (rating IS NULL OR (rating >= 0 AND rating <= 5)),
    CONSTRAINT drivers_rates_range CHECK (
        (completion_rate   IS NULL OR (completion_rate   >= 0 AND completion_rate   <= 1)) AND
        (cancellation_rate IS NULL OR (cancellation_rate >= 0 AND cancellation_rate <= 1)) AND
        (acceptance_rate   IS NULL OR (acceptance_rate   >= 0 AND acceptance_rate   <= 1))
    )
);

CREATE INDEX drivers_status_idx ON drivers (status) WHERE status = 'AVAILABLE';

CREATE TABLE vehicles (
    id                  uuid PRIMARY KEY DEFAULT gen_random_uuid(),
    owner_user_id       uuid        NOT NULL REFERENCES users (id) ON DELETE CASCADE,
    type                text        NOT NULL,
    make                text,
    model               text,
    year                integer,
    plate_number        text        NOT NULL UNIQUE,
    capacity_kg         numeric(10, 2),
    -- Free-form because document 13 says "dimensions" without a shape, and
    -- guessing one would be inventing schema.
    dimensions          jsonb,
    verification_status text        NOT NULL DEFAULT 'PENDING_VERIFICATION',
    created_at          timestamptz NOT NULL DEFAULT now(),
    updated_at          timestamptz NOT NULL DEFAULT now(),
    CONSTRAINT vehicles_type_valid CHECK (
        type IN ('MOTORCYCLE', 'RICKSHAW', 'CAR', 'LOADER', 'VAN', 'PICKUP', 'TRUCK')
    ),
    CONSTRAINT vehicles_verification_valid CHECK (
        verification_status IN ('PENDING_VERIFICATION', 'VERIFIED', 'SUSPENDED', 'EXPIRED')
    ),
    CONSTRAINT vehicles_year_sane CHECK (year IS NULL OR (year >= 1900 AND year <= 2200)),
    CONSTRAINT vehicles_capacity_positive CHECK (capacity_kg IS NULL OR capacity_kg > 0)
);

-- Document 04: "A vehicle should have capabilities rather than being hard-coded
-- to one service." A motorcycle carries passengers, parcels and groceries; that
-- is three rows, not three boolean columns and not three vehicle types.
CREATE TABLE vehicle_capabilities (
    vehicle_id uuid NOT NULL REFERENCES vehicles (id) ON DELETE CASCADE,
    capability text NOT NULL,
    PRIMARY KEY (vehicle_id, capability),
    CONSTRAINT vehicle_capability_valid CHECK (
        capability IN ('PASSENGER', 'PARCEL', 'GROCERY', 'SMALL_CARGO', 'HEAVY_CARGO')
    )
);

CREATE TABLE driver_vehicles (
    driver_id  uuid        NOT NULL REFERENCES drivers (id) ON DELETE CASCADE,
    vehicle_id uuid        NOT NULL REFERENCES vehicles (id) ON DELETE CASCADE,
    is_primary boolean     NOT NULL DEFAULT false,
    status     text        NOT NULL DEFAULT 'ACTIVE',
    created_at timestamptz NOT NULL DEFAULT now(),
    PRIMARY KEY (driver_id, vehicle_id),
    CONSTRAINT driver_vehicles_status_valid CHECK (status IN ('ACTIVE', 'INACTIVE'))
);

-- One primary vehicle per driver: "primary" that can be held twice means
-- nothing to the code that picks a default.
CREATE UNIQUE INDEX driver_vehicles_one_primary_idx
    ON driver_vehicles (driver_id) WHERE is_primary;

-- --- quotes ------------------------------------------------------------------

-- The quote a job was booked against.
--
-- Money is an integer count of minor units with its currency (ADR-008). There
-- is no numeric or float column for an amount anywhere in this schema, and no
-- pricing logic in this phase — CAP-1's boundary is created by the ride slice.
CREATE TABLE pricing_quotes (
    id             uuid PRIMARY KEY DEFAULT gen_random_uuid(),
    job_type       text        NOT NULL,
    amount_minor   bigint      NOT NULL,
    currency       text        NOT NULL DEFAULT 'PKR',
    -- A range where uncertainty is material (document 05: "show a range rather
    -- than false precision"). Both null when the quote is exact.
    low_minor      bigint,
    high_minor     bigint,
    -- The inputs and rule outputs the amount came from, so a quote can be
    -- explained after the fact. Shape is set by the pricing capability.
    breakdown      jsonb       NOT NULL DEFAULT '{}'::jsonb,
    expires_at     timestamptz,
    created_at     timestamptz NOT NULL DEFAULT now(),
    CONSTRAINT quotes_currency_valid CHECK (currency = 'PKR'),
    CONSTRAINT quotes_amount_nonneg CHECK (amount_minor >= 0),
    CONSTRAINT quotes_range_ordered CHECK (
        (low_minor IS NULL AND high_minor IS NULL) OR
        (low_minor IS NOT NULL AND high_minor IS NOT NULL AND low_minor <= high_minor)
    )
);

-- --- jobs --------------------------------------------------------------------

CREATE TABLE jobs (
    id                  uuid PRIMARY KEY DEFAULT gen_random_uuid(),
    type                text        NOT NULL,
    requester_user_id   uuid        NOT NULL REFERENCES users (id) ON DELETE RESTRICT,
    merchant_id         uuid,
    status              text        NOT NULL DEFAULT 'DRAFT',
    scheduled_at        timestamptz,
    pricing_quote_id    uuid        REFERENCES pricing_quotes (id) ON DELETE SET NULL,
    assigned_driver_id  uuid        REFERENCES drivers (id) ON DELETE SET NULL,
    assigned_vehicle_id uuid        REFERENCES vehicles (id) ON DELETE SET NULL,
    -- Set once, when the job first reaches a terminal state.
    terminated_at       timestamptz,
    created_at          timestamptz NOT NULL DEFAULT now(),
    updated_at          timestamptz NOT NULL DEFAULT now(),
    CONSTRAINT jobs_type_valid CHECK (type IN ('RIDE', 'PARCEL', 'GROCERY', 'CARGO', 'FREIGHT')),
    CONSTRAINT jobs_status_valid CHECK (status IN (
        'DRAFT', 'QUOTED', 'REQUESTED', 'SEARCHING', 'ASSIGNED', 'ACCEPTED',
        'ARRIVING', 'AT_PICKUP', 'IN_PROGRESS', 'AT_DROPOFF', 'COMPLETED',
        'CANCELLED', 'FAILED', 'EXPIRED', 'DISPUTED'
    ))
);

-- Document 26's index examples.
CREATE INDEX jobs_status_created_idx ON jobs (status, created_at DESC);
CREATE INDEX jobs_driver_status_idx  ON jobs (assigned_driver_id, status)
    WHERE assigned_driver_id IS NOT NULL;
CREATE INDEX jobs_requester_idx      ON jobs (requester_user_id, created_at DESC);

-- Stops are ordered and typed: a ride has PICKUP then DROPOFF, a multi-stop
-- delivery has more. Modelling pickup and destination as columns on jobs would
-- have made multi-stop (document 82) a schema migration instead of extra rows.
CREATE TABLE job_stops (
    id           uuid PRIMARY KEY DEFAULT gen_random_uuid(),
    job_id       uuid        NOT NULL REFERENCES jobs (id) ON DELETE CASCADE,
    sequence     integer     NOT NULL,
    type         text        NOT NULL,
    -- geography(Point,4326), not two numerics: distance and containment are
    -- what dispatch asks of this column, and PostGIS answers them on a sphere
    -- with an index (documents 12, 26).
    location     geography(Point, 4326) NOT NULL,
    address      text,
    contact_name  text,
    contact_phone text,
    arrived_at   timestamptz,
    completed_at timestamptz,
    CONSTRAINT job_stops_type_valid CHECK (type IN ('PICKUP', 'DROPOFF', 'WAYPOINT')),
    CONSTRAINT job_stops_sequence_positive CHECK (sequence >= 0),
    UNIQUE (job_id, sequence)
);

CREATE INDEX job_stops_location_idx ON job_stops USING GIST (location);

-- Requirements a candidate vehicle or driver must satisfy (document 04:
-- JobRequirement). Kept as rows so eligibility is a join, not a parsed blob.
CREATE TABLE job_requirements (
    job_id      uuid NOT NULL REFERENCES jobs (id) ON DELETE CASCADE,
    requirement text NOT NULL,
    value       text,
    PRIMARY KEY (job_id, requirement)
);

-- --- assignments -------------------------------------------------------------

CREATE TABLE assignments (
    id           uuid PRIMARY KEY DEFAULT gen_random_uuid(),
    job_id       uuid        NOT NULL REFERENCES jobs (id) ON DELETE CASCADE,
    driver_id    uuid        NOT NULL REFERENCES drivers (id) ON DELETE RESTRICT,
    vehicle_id   uuid        REFERENCES vehicles (id) ON DELETE SET NULL,
    status       text        NOT NULL DEFAULT 'OFFERED',
    offered_at   timestamptz NOT NULL DEFAULT now(),
    responded_at timestamptz,
    accepted_at  timestamptz,
    completed_at timestamptz,
    expires_at   timestamptz,
    CONSTRAINT assignments_status_valid CHECK (
        status IN ('OFFERED', 'ACCEPTED', 'REJECTED', 'EXPIRED', 'CANCELLED', 'COMPLETED')
    )
);

-- The invariant that stops two drivers holding one job.
--
-- Dispatch will enforce this in application code as well, but the guarantee has
-- to live here: a partial unique index is checked by the database under
-- concurrency, where an application check between a SELECT and an INSERT is
-- not. Phase 8 depends on this row existing.
CREATE UNIQUE INDEX assignments_one_live_per_job_idx
    ON assignments (job_id) WHERE status IN ('OFFERED', 'ACCEPTED');

CREATE INDEX assignments_driver_idx ON assignments (driver_id, status);

-- --- history -----------------------------------------------------------------

-- Every job status transition, as document 15 requires: "Every transition emits
-- JobStatusChanged containing job ID, previous/new status, actor, timestamp and
-- metadata." This table is that record; the NATS event (Phase 6) is its
-- broadcast, not its storage.
CREATE TABLE job_status_history (
    id         bigserial PRIMARY KEY,
    job_id     uuid        NOT NULL REFERENCES jobs (id) ON DELETE CASCADE,
    from_status text,
    to_status  text        NOT NULL,
    actor_type text        NOT NULL,
    actor_id   uuid,
    metadata   jsonb       NOT NULL DEFAULT '{}'::jsonb,
    created_at timestamptz NOT NULL DEFAULT now(),
    CONSTRAINT job_history_actor_valid CHECK (actor_type IN ('CUSTOMER', 'DRIVER', 'MERCHANT', 'ADMIN', 'SUPPORT', 'SYSTEM'))
);

CREATE INDEX job_status_history_job_idx ON job_status_history (job_id, created_at);
