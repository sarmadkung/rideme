-- Service zones (document 97): a geographic boundary pricing and dispatch can
-- key off, replacing a plain city-name match with an actual point-in-zone
-- test. v1 keeps the geometry simple — a circle (center + radius) — which
-- reuses the same geography(Point,4326)/ST_DWithin pattern already used for
-- driver_locations and job_stops, rather than introducing polygon storage
-- this schema has never needed before.

CREATE TABLE service_zones (
    id            uuid PRIMARY KEY DEFAULT gen_random_uuid(),
    name          text        NOT NULL,
    -- Display/reporting only; the center+radius below is what pricing and
    -- dispatch actually query against.
    city          text,
    center        geography(Point, 4326) NOT NULL,
    radius_meters integer     NOT NULL,
    status        text        NOT NULL DEFAULT 'ACTIVE',
    created_at    timestamptz NOT NULL DEFAULT now(),
    updated_at    timestamptz NOT NULL DEFAULT now(),
    CONSTRAINT service_zones_status_valid CHECK (status IN ('ACTIVE', 'INACTIVE')),
    -- 200km caps a zone at "small country region", not "everywhere" — a zone
    -- that large is a city string wearing a costume.
    CONSTRAINT service_zones_radius_sane CHECK (radius_meters > 0 AND radius_meters <= 200000)
);

CREATE INDEX service_zones_center_gix ON service_zones USING GIST (center);

-- Pricing gains a zone dimension alongside the existing city string (document
-- 34: "by city, zone"). NULL means "any", same convention as vehicle_type and
-- city already use.
ALTER TABLE pricing_tariffs ADD COLUMN zone_id uuid REFERENCES service_zones (id) ON DELETE CASCADE;

ALTER TABLE pricing_tariffs DROP CONSTRAINT pricing_tariffs_job_type_vehicle_type_city_version_key;
ALTER TABLE pricing_tariffs ADD CONSTRAINT tariffs_scope_unique
    UNIQUE (job_type, vehicle_type, city, zone_id, version);

DROP INDEX pricing_tariffs_lookup_idx;
CREATE INDEX pricing_tariffs_lookup_idx ON pricing_tariffs (job_type, vehicle_type, city, zone_id, version DESC);
