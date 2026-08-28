DROP TABLE IF EXISTS job_cancellations;
DROP TABLE IF EXISTS idempotency_keys;
DROP TABLE IF EXISTS job_price_locks;

ALTER TABLE pricing_quotes
    DROP COLUMN IF EXISTS route_confidence,
    DROP COLUMN IF EXISTS duration_seconds,
    DROP COLUMN IF EXISTS distance_meters,
    DROP COLUMN IF EXISTS tax_minor,
    DROP COLUMN IF EXISTS discount_minor,
    DROP COLUMN IF EXISTS demand_minor,
    DROP COLUMN IF EXISTS loading_minor,
    DROP COLUMN IF EXISTS waiting_minor,
    DROP COLUMN IF EXISTS service_fee_minor,
    DROP COLUMN IF EXISTS time_minor,
    DROP COLUMN IF EXISTS distance_minor,
    DROP COLUMN IF EXISTS base_minor,
    DROP COLUMN IF EXISTS requested_by,
    DROP COLUMN IF EXISTS pricing_version,
    DROP COLUMN IF EXISTS tariff_id,
    DROP COLUMN IF EXISTS vehicle_type;

DROP TABLE IF EXISTS pricing_tariffs;
