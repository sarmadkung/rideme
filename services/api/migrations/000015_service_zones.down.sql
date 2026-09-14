DROP INDEX IF EXISTS pricing_tariffs_lookup_idx;
CREATE INDEX pricing_tariffs_lookup_idx ON pricing_tariffs (job_type, vehicle_type, city, version DESC);

ALTER TABLE pricing_tariffs DROP CONSTRAINT IF EXISTS tariffs_scope_unique;
ALTER TABLE pricing_tariffs ADD CONSTRAINT pricing_tariffs_job_type_vehicle_type_city_version_key
    UNIQUE (job_type, vehicle_type, city, version);

ALTER TABLE pricing_tariffs DROP COLUMN IF EXISTS zone_id;

DROP TABLE IF EXISTS service_zones;
