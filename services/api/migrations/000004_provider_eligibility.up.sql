-- Providers, vehicles and service eligibility (documents 16, 29, 30, 41, 108).

-- --- taxonomy as configuration -----------------------------------------------
--
-- Document 30: "Vehicle taxonomy must be configuration-friendly because local
-- names/categories can evolve", and the capability examples there are
-- "configuration examples, not permanent hard-coded rules."
--
-- Migration 000003 encoded both as CHECK constraints. That was wrong against
-- this document: adding a vehicle category a new market uses would have needed
-- a schema migration and a deploy. They become reference tables here.

CREATE TABLE vehicle_types (
    code        text PRIMARY KEY,
    label       text        NOT NULL,
    -- Ordering for client pickers; local categories are not alphabetical.
    sort_order  integer     NOT NULL DEFAULT 0,
    active      boolean     NOT NULL DEFAULT true,
    created_at  timestamptz NOT NULL DEFAULT now()
);

CREATE TABLE capabilities (
    code       text PRIMARY KEY,
    label      text        NOT NULL,
    active     boolean     NOT NULL DEFAULT true,
    created_at timestamptz NOT NULL DEFAULT now()
);

-- The initial taxonomy document 30 lists. Seeded, not constrained: a market can
-- add to this table without a migration.
INSERT INTO vehicle_types (code, label, sort_order) VALUES
    ('MOTORCYCLE',      'Motorcycle',      10),
    ('RICKSHAW',        'Rickshaw',        20),
    ('CAR',             'Car',             30),
    ('LOADER_RICKSHAW', 'Loader Rickshaw', 40),
    ('SUZUKI_PICKUP',   'Suzuki Pickup',   50),
    ('SHEHZORE',        'Shehzore',        60),
    ('MAZDA',           'Mazda',           70),
    ('TRUCK',           'Truck',           80);

INSERT INTO capabilities (code, label) VALUES
    ('PASSENGER',         'Passenger'),
    ('PARCEL',            'Parcel'),
    ('GROCERY',           'Grocery'),
    ('BUSINESS_DELIVERY', 'Business Delivery'),
    ('SMALL_CARGO',       'Small Cargo'),
    ('HOUSE_MOVING',      'House Moving'),
    ('HEAVY_CARGO',       'Heavy Cargo'),
    ('INTERCITY',         'Intercity');

-- Migration 000003's rows used a narrower vocabulary; map them before the
-- constraints change so no existing row is orphaned.
UPDATE vehicles SET type = 'LOADER_RICKSHAW' WHERE type = 'LOADER';
UPDATE vehicles SET type = 'SUZUKI_PICKUP'   WHERE type IN ('PICKUP', 'VAN');

ALTER TABLE vehicles DROP CONSTRAINT vehicles_type_valid;
ALTER TABLE vehicles ADD CONSTRAINT vehicles_type_fk
    FOREIGN KEY (type) REFERENCES vehicle_types (code);

ALTER TABLE vehicle_capabilities DROP CONSTRAINT vehicle_capability_valid;
ALTER TABLE vehicle_capabilities ADD CONSTRAINT vehicle_capability_fk
    FOREIGN KEY (capability) REFERENCES capabilities (code);

-- Document 30 requires the backend to compute effective capabilities: "Do not
-- trust capabilities submitted by the client." This column records where a
-- capability came from, so a client-asserted one can never be mistaken for a
-- verified one.
ALTER TABLE vehicle_capabilities
    ADD COLUMN source text NOT NULL DEFAULT 'DERIVED',
    ADD CONSTRAINT vehicle_capability_source_valid CHECK (source IN ('DERIVED', 'ADMIN'));

-- What each vehicle type is capable of by default (document 30's examples).
CREATE TABLE vehicle_type_capabilities (
    vehicle_type text NOT NULL REFERENCES vehicle_types (code) ON DELETE CASCADE,
    capability   text NOT NULL REFERENCES capabilities (code) ON DELETE CASCADE,
    PRIMARY KEY (vehicle_type, capability)
);

INSERT INTO vehicle_type_capabilities (vehicle_type, capability) VALUES
    ('MOTORCYCLE',      'PASSENGER'),
    ('MOTORCYCLE',      'PARCEL'),
    ('MOTORCYCLE',      'GROCERY'),
    ('RICKSHAW',        'PASSENGER'),
    ('RICKSHAW',        'PARCEL'),
    ('RICKSHAW',        'GROCERY'),
    ('CAR',             'PASSENGER'),
    ('CAR',             'PARCEL'),
    ('CAR',             'GROCERY'),
    ('LOADER_RICKSHAW', 'PARCEL'),
    ('LOADER_RICKSHAW', 'SMALL_CARGO'),
    ('LOADER_RICKSHAW', 'BUSINESS_DELIVERY'),
    ('SUZUKI_PICKUP',   'PARCEL'),
    ('SUZUKI_PICKUP',   'BUSINESS_DELIVERY'),
    ('SUZUKI_PICKUP',   'SMALL_CARGO'),
    ('SUZUKI_PICKUP',   'HOUSE_MOVING'),
    ('SHEHZORE',        'SMALL_CARGO'),
    ('SHEHZORE',        'HOUSE_MOVING'),
    ('SHEHZORE',        'BUSINESS_DELIVERY'),
    ('MAZDA',           'HEAVY_CARGO'),
    ('MAZDA',           'HOUSE_MOVING'),
    ('MAZDA',           'INTERCITY'),
    ('TRUCK',           'HEAVY_CARGO'),
    ('TRUCK',           'INTERCITY');

-- --- verification states -----------------------------------------------------
--
-- Documents 29 and 30 give fuller state lists than 16's summary. They are the
-- dedicated implementation documents, so they win (conflict C-7).

ALTER TABLE drivers DROP CONSTRAINT drivers_verification_valid;
ALTER TABLE drivers ALTER COLUMN verification_status SET DEFAULT 'NOT_STARTED';
UPDATE drivers SET verification_status = 'NOT_STARTED' WHERE verification_status = 'PENDING_VERIFICATION';
ALTER TABLE drivers ADD CONSTRAINT drivers_verification_valid CHECK (
    verification_status IN ('NOT_STARTED', 'IN_PROGRESS', 'SUBMITTED', 'UNDER_REVIEW',
                            'APPROVED', 'REJECTED', 'SUSPENDED')
);

ALTER TABLE vehicles DROP CONSTRAINT vehicles_verification_valid;
ALTER TABLE vehicles ALTER COLUMN verification_status SET DEFAULT 'PENDING';
UPDATE vehicles SET verification_status = 'PENDING' WHERE verification_status = 'PENDING_VERIFICATION';
ALTER TABLE vehicles ADD CONSTRAINT vehicles_verification_valid CHECK (
    verification_status IN ('PENDING', 'UNDER_REVIEW', 'VERIFIED', 'REJECTED', 'SUSPENDED', 'EXPIRED')
);

-- Document 30's vehicle record includes colour; 13 omitted it.
ALTER TABLE vehicles ADD COLUMN color text;

-- Document 30: "a driver selects one active vehicle before going online."
ALTER TABLE drivers ADD COLUMN active_vehicle_id uuid REFERENCES vehicles (id) ON DELETE SET NULL;

-- --- documents ---------------------------------------------------------------
--
-- One table for driver and vehicle documents: the model in document 29 is the
-- same either way, and the expiry job that matters most should not have to
-- read two tables to find what is lapsing.

CREATE TABLE documents (
    id               uuid PRIMARY KEY DEFAULT gen_random_uuid(),
    owner_type       text        NOT NULL,
    driver_id        uuid        REFERENCES drivers (id) ON DELETE CASCADE,
    vehicle_id       uuid        REFERENCES vehicles (id) ON DELETE CASCADE,
    type             text        NOT NULL,
    number           text,
    -- The object-storage key. Files upload directly via a signed URL and never
    -- pass through the API (document 29).
    file_key         text,
    issued_at        date,
    expires_at       date,
    status           text        NOT NULL DEFAULT 'PENDING',
    rejection_reason text,
    created_at       timestamptz NOT NULL DEFAULT now(),
    updated_at       timestamptz NOT NULL DEFAULT now(),
    CONSTRAINT documents_status_valid CHECK (status IN ('PENDING', 'VERIFIED', 'REJECTED', 'EXPIRED')),
    CONSTRAINT documents_owner_valid CHECK (owner_type IN ('DRIVER', 'VEHICLE')),
    -- Exactly one owner. A document belonging to both or neither has no
    -- meaning, and the expiry gate would not know whom to stop.
    CONSTRAINT documents_owner_exclusive CHECK (
        (owner_type = 'DRIVER'  AND driver_id  IS NOT NULL AND vehicle_id IS NULL) OR
        (owner_type = 'VEHICLE' AND vehicle_id IS NOT NULL AND driver_id  IS NULL)
    )
);

CREATE INDEX documents_driver_idx  ON documents (driver_id)  WHERE driver_id  IS NOT NULL;
CREATE INDEX documents_vehicle_idx ON documents (vehicle_id) WHERE vehicle_id IS NOT NULL;
-- Drives the expiry sweep document 29 requires.
CREATE INDEX documents_expiry_idx  ON documents (expires_at)
    WHERE status = 'VERIFIED' AND expires_at IS NOT NULL;

-- Which documents are mandatory, per market and vehicle type.
--
-- BD-14 is a PRODUCT_DECISION and a regulatory one: which licence or permit a
-- vehicle type needs is set by law, not by the platform. The *model* is
-- type-agnostic and can be built now; the list is data, and this table is
-- deliberately seeded empty. Document 29: "Do not hard-code country-specific
-- document requirements into the domain model."
CREATE TABLE document_requirements (
    id           uuid PRIMARY KEY DEFAULT gen_random_uuid(),
    market       text NOT NULL DEFAULT 'PK',
    owner_type   text NOT NULL,
    -- NULL means the requirement applies to every vehicle type.
    vehicle_type text REFERENCES vehicle_types (code) ON DELETE CASCADE,
    type         text NOT NULL,
    mandatory    boolean NOT NULL DEFAULT true,
    CONSTRAINT document_requirements_owner_valid CHECK (owner_type IN ('DRIVER', 'VEHICLE')),
    UNIQUE (market, owner_type, vehicle_type, type)
);

-- --- verification review audit -----------------------------------------------
--
-- Document 29: "Every review action is audited."
CREATE TABLE verification_reviews (
    id          bigserial PRIMARY KEY,
    subject_type text       NOT NULL,
    subject_id  uuid        NOT NULL,
    reviewer_id uuid        REFERENCES users (id) ON DELETE SET NULL,
    from_status text,
    to_status   text        NOT NULL,
    reason      text,
    created_at  timestamptz NOT NULL DEFAULT now(),
    CONSTRAINT verification_reviews_subject_valid CHECK (subject_type IN ('DRIVER', 'VEHICLE', 'DOCUMENT'))
);

CREATE INDEX verification_reviews_subject_idx ON verification_reviews (subject_type, subject_id, created_at DESC);
