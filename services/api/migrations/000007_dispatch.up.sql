-- Dispatch (documents 38, 39, 40, 42, 43, 44, 45, 46, 49).
--
-- Document 46's invariants, enforced by the database rather than by
-- application code, because application checks have a window between the read
-- and the write and constraints do not:
--
--   * at most one active assignment per job   — migration 000003
--   * at most one active reservation per driver — here

-- A reservation holds a driver for a job while an offer is outstanding
-- (document 43). It is separate from the assignment because a driver is held
-- *before* they accept, and something must prevent a second dispatcher
-- offering them a different job in that window.
CREATE TABLE driver_reservations (
    id           uuid PRIMARY KEY DEFAULT gen_random_uuid(),
    job_id       uuid        NOT NULL REFERENCES jobs (id) ON DELETE CASCADE,
    driver_id    uuid        NOT NULL REFERENCES drivers (id) ON DELETE CASCADE,
    assignment_id uuid       REFERENCES assignments (id) ON DELETE SET NULL,
    state        text        NOT NULL DEFAULT 'RESERVED',
    expires_at   timestamptz NOT NULL,
    released_at  timestamptz,
    created_at   timestamptz NOT NULL DEFAULT now(),
    CONSTRAINT reservations_state_valid CHECK (
        state IN ('RESERVED', 'CONSUMED', 'RELEASED', 'EXPIRED'))
);

-- Document 46: "At most one active job/reservation may consume a driver's
-- dispatch capacity." Without this, two dispatchers racing on two jobs both
-- offer the same driver, and whichever they accept the other job is stranded
-- holding a reservation that will never be consumed.
CREATE UNIQUE INDEX driver_reservations_one_active_idx
    ON driver_reservations (driver_id) WHERE state = 'RESERVED';

CREATE INDEX driver_reservations_job_idx ON driver_reservations (job_id);
-- Drives the expiry sweep.
CREATE INDEX driver_reservations_expiry_idx
    ON driver_reservations (expires_at) WHERE state = 'RESERVED';

-- One dispatch attempt: a ring of candidates, scored, with one offer made.
-- Document 44's retry strategy is a sequence of these with an expanding radius.
CREATE TABLE dispatch_attempts (
    id                uuid PRIMARY KEY DEFAULT gen_random_uuid(),
    job_id            uuid        NOT NULL REFERENCES jobs (id) ON DELETE CASCADE,
    attempt           integer     NOT NULL,
    radius_meters     integer     NOT NULL,
    geo_candidates    integer     NOT NULL DEFAULT 0,
    eligible_candidates integer   NOT NULL DEFAULT 0,
    offered_driver_id uuid        REFERENCES drivers (id) ON DELETE SET NULL,
    outcome           text        NOT NULL,
    -- Document 40 requires every assignment be explainable retrospectively.
    strategy_version  integer     NOT NULL DEFAULT 1,
    score_version     integer     NOT NULL DEFAULT 1,
    created_at        timestamptz NOT NULL DEFAULT now(),
    CONSTRAINT dispatch_attempts_outcome_valid CHECK (
        outcome IN ('OFFERED', 'NO_CANDIDATES', 'NO_ELIGIBLE', 'RESERVATION_LOST', 'ERROR')),
    UNIQUE (job_id, attempt)
);

CREATE INDEX dispatch_attempts_job_idx ON dispatch_attempts (job_id, attempt);

-- Why each candidate scored as it did (document 40: "explain the major factors
-- behind an assignment for support/debugging").
--
-- Without this a dispatch complaint is unanswerable: the inputs — positions,
-- availability, freshness — are volatile and gone by the time anyone asks.
CREATE TABLE dispatch_scores (
    attempt_id uuid    NOT NULL REFERENCES dispatch_attempts (id) ON DELETE CASCADE,
    driver_id  uuid    NOT NULL REFERENCES drivers (id) ON DELETE CASCADE,
    rank       integer NOT NULL,
    score      numeric(10, 6) NOT NULL,
    -- Each weighted term and its input, so a score can be recomputed by hand.
    factors    jsonb   NOT NULL DEFAULT '{}'::jsonb,
    PRIMARY KEY (attempt_id, driver_id)
);

-- Durable deduplication for NATS consumers (document 46: "NATS delivery may be
-- repeated. Consumers must be idempotent").
CREATE TABLE processed_events (
    event_id     text        NOT NULL,
    consumer     text        NOT NULL,
    processed_at timestamptz NOT NULL DEFAULT now(),
    PRIMARY KEY (consumer, event_id)
);

CREATE INDEX processed_events_time_idx ON processed_events (processed_at);

-- The outbox document 46 calls for where atomic state change plus event
-- publication is required. A row is written in the same transaction as the
-- state change; publishing happens after the commit, so an event can never
-- describe a state that was rolled back.
CREATE TABLE event_outbox (
    id           bigserial PRIMARY KEY,
    subject      text        NOT NULL,
    event_id     text        NOT NULL UNIQUE,
    payload      jsonb       NOT NULL,
    created_at   timestamptz NOT NULL DEFAULT now(),
    published_at timestamptz
);

CREATE INDEX event_outbox_unpublished_idx ON event_outbox (id) WHERE published_at IS NULL;

-- Dispatch tuning. BD-03 is a TECHNICAL_DEFAULT: document 005 states the
-- weights "should be configurable and learned from real outcomes later", so
-- they are rows rather than constants and start with ETA dominant.
CREATE TABLE dispatch_config (
    job_type            text PRIMARY KEY,
    -- Document 39's rings, in metres.
    radius_rings        integer[]   NOT NULL DEFAULT '{2000,5000,10000,20000}',
    max_attempts        integer     NOT NULL DEFAULT 4,
    geo_candidate_limit integer     NOT NULL DEFAULT 100,
    score_limit         integer     NOT NULL DEFAULT 10,
    offer_ttl_seconds   integer     NOT NULL DEFAULT 20,
    -- Document 39's freshness thresholds.
    max_location_age_seconds integer NOT NULL DEFAULT 45,
    -- Weights in basis points so the whole score stays integer-friendly.
    -- ETA dominant, the rest low but non-zero so every term is exercised —
    -- exactly what the business decision register recommends for BD-03.
    weight_eta_bps         integer  NOT NULL DEFAULT 6000,
    weight_distance_bps    integer  NOT NULL DEFAULT 1500,
    weight_reliability_bps integer  NOT NULL DEFAULT 1000,
    weight_acceptance_bps  integer  NOT NULL DEFAULT 500,
    weight_idle_bps        integer  NOT NULL DEFAULT 500,
    weight_capability_bps  integer  NOT NULL DEFAULT 500,
    strategy_version    integer     NOT NULL DEFAULT 1,
    score_version       integer     NOT NULL DEFAULT 1,
    updated_at          timestamptz NOT NULL DEFAULT now(),
    CONSTRAINT dispatch_config_job_type_valid CHECK (
        job_type IN ('RIDE', 'PARCEL', 'GROCERY', 'CARGO', 'FREIGHT')),
    CONSTRAINT dispatch_config_bounded CHECK (
        max_attempts BETWEEN 1 AND 10 AND
        geo_candidate_limit BETWEEN 1 AND 1000 AND
        score_limit BETWEEN 1 AND 100 AND
        offer_ttl_seconds BETWEEN 5 AND 300)
);

INSERT INTO dispatch_config (job_type) VALUES
    ('RIDE'), ('PARCEL'), ('GROCERY'), ('CARGO'), ('FREIGHT');

-- Assignments gain their dispatch provenance.
ALTER TABLE assignments
    ADD COLUMN attempt_id uuid REFERENCES dispatch_attempts (id) ON DELETE SET NULL,
    ADD COLUMN score numeric(10, 6);
