-- Delivery and cargo (documents 79-91).
--
-- Parcel and cargo are Job types, not new entities. What they add is evidence
-- that a delivery happened, a way to handle one that did not, and the physical
-- constraints a cargo vehicle must satisfy.

-- --- proof of delivery (document 83) -----------------------------------------

-- Proof is per stop, not per job: a multi-stop delivery produces one proof per
-- drop, and a job-level record could not say which parcel reached whom.
CREATE TABLE delivery_proofs (
    id          uuid PRIMARY KEY DEFAULT gen_random_uuid(),
    job_id      uuid        NOT NULL REFERENCES jobs (id) ON DELETE CASCADE,
    stop_id     uuid        REFERENCES job_stops (id) ON DELETE CASCADE,
    method      text        NOT NULL,
    -- Object-storage reference, never the binary. Document 83: "Store secure
    -- media references, not unnecessary duplicate binaries in the job
    -- database."
    media_key   text,
    -- Where the driver was when they captured it. Document 83 warns against
    -- relying on GPS alone, so this corroborates the proof rather than being it.
    location    geography(Point, 4326),
    recipient_name text,
    verified    boolean     NOT NULL DEFAULT false,
    captured_by uuid        REFERENCES users (id) ON DELETE SET NULL,
    metadata    jsonb       NOT NULL DEFAULT '{}'::jsonb,
    created_at  timestamptz NOT NULL DEFAULT now(),
    CONSTRAINT proofs_method_valid CHECK (method IN (
        'RECIPIENT_OTP', 'SIGNATURE', 'PHOTO', 'RECIPIENT_CONFIRMATION',
        'CODE_SCAN', 'MERCHANT_CONFIRMATION'))
);

CREATE INDEX delivery_proofs_job_idx ON delivery_proofs (job_id);

-- Recipient OTPs. Short-lived and hashed for the same reason login OTPs are
-- (documents 83, 123): a stored code is a code someone can read.
CREATE TABLE delivery_otps (
    stop_id     uuid PRIMARY KEY REFERENCES job_stops (id) ON DELETE CASCADE,
    job_id      uuid        NOT NULL REFERENCES jobs (id) ON DELETE CASCADE,
    code_hash   bytea       NOT NULL,
    attempts    integer     NOT NULL DEFAULT 0,
    max_attempts integer    NOT NULL DEFAULT 5,
    expires_at  timestamptz NOT NULL,
    consumed_at timestamptz,
    created_at  timestamptz NOT NULL DEFAULT now()
);

-- --- delivery failure and return (document 84) -------------------------------

CREATE TABLE delivery_attempts (
    id           uuid PRIMARY KEY DEFAULT gen_random_uuid(),
    job_id       uuid        NOT NULL REFERENCES jobs (id) ON DELETE CASCADE,
    stop_id      uuid        REFERENCES job_stops (id) ON DELETE CASCADE,
    attempt      integer     NOT NULL,
    outcome      text        NOT NULL,
    failure_reason text,
    -- What happens next. Document 84 requires a deterministic next action
    -- rather than a parcel left in an undefined state.
    next_action  text,
    notes        text,
    created_at   timestamptz NOT NULL DEFAULT now(),
    CONSTRAINT delivery_attempts_outcome_valid CHECK (outcome IN ('DELIVERED', 'FAILED')),
    CONSTRAINT delivery_attempts_reason_valid CHECK (failure_reason IS NULL OR failure_reason IN (
        'RECIPIENT_UNAVAILABLE', 'WRONG_ADDRESS', 'RECIPIENT_REJECTED',
        'DAMAGED_PACKAGE', 'ACCESS_BLOCKED', 'MERCHANT_ISSUE')),
    CONSTRAINT delivery_attempts_action_valid CHECK (next_action IS NULL OR next_action IN (
        'RETRY', 'RESCHEDULE', 'RETURN', 'ESCALATE')),
    UNIQUE (stop_id, attempt)
);

CREATE INDEX delivery_attempts_job_idx ON delivery_attempts (job_id, created_at);

-- Document 84: a return "may create a Return Stop rather than creating an
-- unrelated manual job". Marking the stop keeps the parcel's history on one
-- job instead of splitting it across two.
ALTER TABLE job_stops
    ADD COLUMN is_return boolean NOT NULL DEFAULT false,
    ADD COLUMN returns_stop_id uuid REFERENCES job_stops (id) ON DELETE SET NULL;

-- --- cargo (documents 80, 87) ------------------------------------------------

-- Cargo attributes, per job. Document 41 is explicit: "Do not use weight
-- alone." Dimensions are separate columns rather than the vehicles table's
-- free-form jsonb, because these are compared numerically on every dispatch.
CREATE TABLE cargo_details (
    job_id                uuid PRIMARY KEY REFERENCES jobs (id) ON DELETE CASCADE,
    total_weight_kg       numeric(10, 2),
    length_cm             numeric(10, 2),
    width_cm              numeric(10, 2),
    height_cm             numeric(10, 2),
    volume_m3             numeric(10, 3),
    item_count            integer,
    fragile               boolean NOT NULL DEFAULT false,
    temperature_sensitive boolean NOT NULL DEFAULT false,
    special_handling      text,
    -- Document 87: "Represent helper as an explicit job requirement. Do not
    -- infer it only from vehicle type."
    loading_assistance    text NOT NULL DEFAULT 'DRIVER_ONLY',
    CONSTRAINT cargo_assistance_valid CHECK (loading_assistance IN (
        'DRIVER_ONLY', 'DRIVER_PLUS_HELPER', 'CUSTOMER_LOADING', 'MERCHANT_LOADING')),
    CONSTRAINT cargo_positive CHECK (
        (total_weight_kg IS NULL OR total_weight_kg > 0) AND
        (length_cm IS NULL OR length_cm > 0) AND
        (width_cm  IS NULL OR width_cm  > 0) AND
        (height_cm IS NULL OR height_cm > 0) AND
        (item_count IS NULL OR item_count > 0))
);

-- Vehicle cargo dimensions, so document 80's hard constraints are comparable
-- columns rather than parsed JSON.
ALTER TABLE vehicles
    ADD COLUMN max_volume_m3   numeric(10, 3),
    ADD COLUMN cargo_length_cm numeric(10, 2),
    ADD COLUMN cargo_width_cm  numeric(10, 2),
    ADD COLUMN cargo_height_cm numeric(10, 2),
    ADD COLUMN passenger_seats integer,
    ADD COLUMN equipment       text[] NOT NULL DEFAULT '{}';

-- Document 87's timestamps. These are recorded whether or not they are priced:
-- BD-13 leaves the rates open, and an unpriced event is still an event.
CREATE TABLE stop_timings (
    stop_id             uuid PRIMARY KEY REFERENCES job_stops (id) ON DELETE CASCADE,
    job_id              uuid        NOT NULL REFERENCES jobs (id) ON DELETE CASCADE,
    arrived_at          timestamptz,
    waiting_started_at  timestamptz,
    loading_started_at  timestamptz,
    loaded_at           timestamptz,
    unloading_started_at timestamptz,
    unloaded_at         timestamptz,
    -- Grace before waiting becomes billable (document 87). Configurable, and
    -- the fee it implies is BD-13 and unset.
    grace_seconds       integer     NOT NULL DEFAULT 0,
    chargeable_waiting_seconds integer NOT NULL DEFAULT 0,
    loading_seconds     integer     NOT NULL DEFAULT 0,
    updated_at          timestamptz NOT NULL DEFAULT now(),
    CONSTRAINT stop_timings_nonneg CHECK (
        grace_seconds >= 0 AND chargeable_waiting_seconds >= 0 AND loading_seconds >= 0)
);

CREATE INDEX stop_timings_job_idx ON stop_timings (job_id);

-- Restricted goods. Document 88 makes this a legal list, and BD-13 leaves it
-- to the owner: the table ships empty and the check passes vacuously until a
-- list exists. Shipping a guessed list would be worse than shipping none.
CREATE TABLE restricted_goods (
    code       text PRIMARY KEY,
    label      text        NOT NULL,
    market     text        NOT NULL DEFAULT 'PK',
    created_at timestamptz NOT NULL DEFAULT now()
);
