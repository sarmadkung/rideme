DROP TABLE IF EXISTS restricted_goods;
DROP TABLE IF EXISTS stop_timings;

ALTER TABLE vehicles
    DROP COLUMN IF EXISTS equipment,
    DROP COLUMN IF EXISTS passenger_seats,
    DROP COLUMN IF EXISTS cargo_height_cm,
    DROP COLUMN IF EXISTS cargo_width_cm,
    DROP COLUMN IF EXISTS cargo_length_cm,
    DROP COLUMN IF EXISTS max_volume_m3;

DROP TABLE IF EXISTS cargo_details;

ALTER TABLE job_stops
    DROP COLUMN IF EXISTS returns_stop_id,
    DROP COLUMN IF EXISTS is_return;

DROP TABLE IF EXISTS delivery_attempts;
DROP TABLE IF EXISTS delivery_otps;
DROP TABLE IF EXISTS delivery_proofs;
