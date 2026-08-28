DROP TABLE IF EXISTS verification_reviews;
DROP TABLE IF EXISTS document_requirements;
DROP TABLE IF EXISTS documents;

ALTER TABLE drivers DROP COLUMN IF EXISTS active_vehicle_id;
ALTER TABLE vehicles DROP COLUMN IF EXISTS color;

ALTER TABLE vehicles DROP CONSTRAINT IF EXISTS vehicles_verification_valid;
UPDATE vehicles SET verification_status = 'PENDING_VERIFICATION'
 WHERE verification_status IN ('PENDING', 'UNDER_REVIEW', 'REJECTED');
ALTER TABLE vehicles ALTER COLUMN verification_status SET DEFAULT 'PENDING_VERIFICATION';
ALTER TABLE vehicles ADD CONSTRAINT vehicles_verification_valid CHECK (
    verification_status IN ('PENDING_VERIFICATION', 'VERIFIED', 'SUSPENDED', 'EXPIRED'));

ALTER TABLE drivers DROP CONSTRAINT IF EXISTS drivers_verification_valid;
UPDATE drivers SET verification_status = 'PENDING_VERIFICATION'
 WHERE verification_status IN ('NOT_STARTED', 'IN_PROGRESS', 'SUBMITTED', 'UNDER_REVIEW', 'REJECTED');
UPDATE drivers SET verification_status = 'VERIFIED' WHERE verification_status = 'APPROVED';
ALTER TABLE drivers ALTER COLUMN verification_status SET DEFAULT 'PENDING_VERIFICATION';
ALTER TABLE drivers ADD CONSTRAINT drivers_verification_valid CHECK (
    verification_status IN ('PENDING_VERIFICATION', 'VERIFIED', 'SUSPENDED', 'EXPIRED'));

DROP TABLE IF EXISTS vehicle_type_capabilities;

ALTER TABLE vehicle_capabilities DROP CONSTRAINT IF EXISTS vehicle_capability_source_valid;
ALTER TABLE vehicle_capabilities DROP COLUMN IF EXISTS source;
ALTER TABLE vehicle_capabilities DROP CONSTRAINT IF EXISTS vehicle_capability_fk;
DELETE FROM vehicle_capabilities WHERE capability NOT IN ('PASSENGER','PARCEL','GROCERY','SMALL_CARGO','HEAVY_CARGO');
ALTER TABLE vehicle_capabilities ADD CONSTRAINT vehicle_capability_valid CHECK (
    capability IN ('PASSENGER', 'PARCEL', 'GROCERY', 'SMALL_CARGO', 'HEAVY_CARGO'));

ALTER TABLE vehicles DROP CONSTRAINT IF EXISTS vehicles_type_fk;
UPDATE vehicles SET type = 'LOADER' WHERE type = 'LOADER_RICKSHAW';
UPDATE vehicles SET type = 'PICKUP' WHERE type IN ('SUZUKI_PICKUP', 'SHEHZORE', 'MAZDA');
ALTER TABLE vehicles ADD CONSTRAINT vehicles_type_valid CHECK (
    type IN ('MOTORCYCLE', 'RICKSHAW', 'CAR', 'LOADER', 'VAN', 'PICKUP', 'TRUCK'));

DROP TABLE IF EXISTS capabilities;
DROP TABLE IF EXISTS vehicle_types;
