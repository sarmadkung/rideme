-- Location and tracking (documents 18, 48, 98, 102).
--
-- Redis holds current driver state; PostgreSQL remains the durable source of
-- truth (document 13). This schema is the durable half.

CREATE TABLE driver_locations (
    id          bigserial PRIMARY KEY,
    driver_id   uuid        NOT NULL REFERENCES drivers (id) ON DELETE CASCADE,
    vehicle_id  uuid        REFERENCES vehicles (id) ON DELETE SET NULL,
    job_id      uuid        REFERENCES jobs (id) ON DELETE SET NULL,
    location    geography(Point, 4326) NOT NULL,
    accuracy_m  real,
    heading_deg real,
    speed_mps   real,
    -- When the device recorded the fix, not when the server stored it. A
    -- buffered pipeline (document 48) means the two differ, and freshness is a
    -- question about the device clock.
    recorded_at timestamptz NOT NULL,
    created_at  timestamptz NOT NULL DEFAULT now(),
    CONSTRAINT driver_locations_heading_range CHECK (heading_deg IS NULL OR (heading_deg >= 0 AND heading_deg < 360)),
    CONSTRAINT driver_locations_speed_nonneg  CHECK (speed_mps IS NULL OR speed_mps >= 0),
    CONSTRAINT driver_locations_accuracy_nonneg CHECK (accuracy_m IS NULL OR accuracy_m >= 0)
);

CREATE INDEX driver_locations_driver_time_idx ON driver_locations (driver_id, recorded_at DESC);
CREATE INDEX driver_locations_job_idx ON driver_locations (job_id, recorded_at) WHERE job_id IS NOT NULL;
CREATE INDEX driver_locations_geo_idx ON driver_locations USING GIST (location);
-- Drives the retention sweep. BD-15 sets the period; the mechanism does not
-- need the value to exist.
CREATE INDEX driver_locations_retention_idx ON driver_locations (recorded_at);

-- A tracking session is the window during which a driver's position is
-- collected for a job. Document 102 requires location access be scoped: the
-- session is what scopes it, so "who may see this driver now" is a row rather
-- than a rule scattered across handlers.
CREATE TABLE tracking_sessions (
    id         uuid PRIMARY KEY DEFAULT gen_random_uuid(),
    job_id     uuid        NOT NULL REFERENCES jobs (id) ON DELETE CASCADE,
    driver_id  uuid        NOT NULL REFERENCES drivers (id) ON DELETE CASCADE,
    started_at timestamptz NOT NULL DEFAULT now(),
    ended_at   timestamptz
);

-- One live tracking session per job.
CREATE UNIQUE INDEX tracking_sessions_live_idx ON tracking_sessions (job_id) WHERE ended_at IS NULL;
CREATE INDEX tracking_sessions_driver_idx ON tracking_sessions (driver_id) WHERE ended_at IS NULL;

-- Document 102: "audit privileged access". A customer watching their own
-- driver is ordinary; an operator opening a driver's history is not, and the
-- difference must be visible afterwards.
CREATE TABLE location_access_log (
    id          bigserial PRIMARY KEY,
    actor_id    uuid        REFERENCES users (id) ON DELETE SET NULL,
    actor_role  text        NOT NULL,
    driver_id   uuid        REFERENCES drivers (id) ON DELETE SET NULL,
    job_id      uuid        REFERENCES jobs (id) ON DELETE SET NULL,
    scope       text        NOT NULL,
    granted     boolean     NOT NULL,
    created_at  timestamptz NOT NULL DEFAULT now()
);

CREATE INDEX location_access_log_actor_idx ON location_access_log (actor_id, created_at DESC);
