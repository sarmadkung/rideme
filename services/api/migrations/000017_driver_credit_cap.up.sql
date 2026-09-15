-- BD-09, resolved in part by the owner on 2026-09-15.
--
-- With cash, the driver collects and the platform is owed. The ledger already
-- records that position — CASH_IN_TRANSIT debited against the driver — and
-- until now nothing acted on it, so the balance grew without bound.
--
-- The owner's decision: a driver who owes more than their cap cannot work
-- until they pay. The cap is per vehicle type, because a bike rider and a
-- truck driver do not carry the same float, and overridable per driver,
-- because who is reliable is learned rather than configured.
--
-- The liability itself — who bears the loss when a driver disappears owing
-- money — is still unallocated. That is a contract term, not a schema.

-- --- the caps ----------------------------------------------------------------

-- Versioned with an active window, exactly like commission_rates. A cap that
-- changed must not rewrite the cap a driver was actually blocked under, or a
-- support conversation about "why was I stopped on Tuesday" has no answer.
CREATE TABLE driver_credit_limits (
    id           uuid PRIMARY KEY DEFAULT gen_random_uuid(),
    vehicle_type text        NOT NULL,
    cap_minor    bigint      NOT NULL,
    -- Where the driver is warned before they are stopped. Being cut off with
    -- no notice, mid-shift, is how a platform loses a driver permanently.
    warn_minor   bigint      NOT NULL,
    currency     text        NOT NULL DEFAULT 'PKR',
    version      integer     NOT NULL DEFAULT 1,
    active_from  timestamptz NOT NULL DEFAULT now(),
    active_to    timestamptz,
    CONSTRAINT driver_credit_limits_vehicle_valid CHECK (vehicle_type IN (
        'MOTORCYCLE', 'RICKSHAW', 'CAR', 'PICKUP', 'MAZDA', 'SHEHZORE', 'TRUCK')),
    CONSTRAINT driver_credit_limits_currency_valid CHECK (currency = 'PKR'),
    CONSTRAINT driver_credit_limits_cap_positive CHECK (cap_minor > 0),
    -- A warning after the stop is not a warning.
    CONSTRAINT driver_credit_limits_warn_below_cap CHECK (warn_minor > 0 AND warn_minor <= cap_minor),
    UNIQUE (vehicle_type, version)
);

-- A driver whose cap differs from their vehicle's default. Null everywhere
-- until someone raises or lowers one, so the common case costs nothing.
ALTER TABLE drivers ADD COLUMN credit_cap_minor bigint;
ALTER TABLE drivers ADD CONSTRAINT drivers_credit_cap_positive
    CHECK (credit_cap_minor IS NULL OR credit_cap_minor > 0);

-- --- repayments --------------------------------------------------------------

-- What a driver has handed back. Separate from the ledger transaction it
-- produces, because "how did this arrive" is an operational question the
-- ledger deliberately does not model: the ledger knows the money moved, this
-- knows an agent counted notes in a hand or a wallet transfer cleared.
CREATE TABLE driver_settlements (
    id             uuid PRIMARY KEY DEFAULT gen_random_uuid(),
    driver_id      uuid        NOT NULL REFERENCES drivers (id) ON DELETE RESTRICT,
    amount_minor   bigint      NOT NULL,
    currency       text        NOT NULL DEFAULT 'PKR',
    method         text        NOT NULL,
    -- The operator who took it, for cash handed to a person. Null for a
    -- transfer that cleared on its own.
    recorded_by    uuid        REFERENCES users (id) ON DELETE SET NULL,
    reference      text,
    -- The ledger transaction this produced, so the two records can never be
    -- reconciled against each other and found to disagree.
    transaction_id uuid        REFERENCES ledger_transactions (id) ON DELETE RESTRICT,
    idempotency_key text UNIQUE,
    note           text,
    created_at     timestamptz NOT NULL DEFAULT now(),
    CONSTRAINT driver_settlements_amount_positive CHECK (amount_minor > 0),
    CONSTRAINT driver_settlements_currency_valid CHECK (currency = 'PKR'),
    -- CASH is an agent taking notes. The digital methods are a rail this
    -- platform does not have yet; they are listed so adding one is a caller
    -- rather than a migration.
    CONSTRAINT driver_settlements_method_valid CHECK (method IN (
        'CASH', 'BANK_TRANSFER', 'WALLET', 'ADJUSTMENT'))
);

CREATE INDEX driver_settlements_driver_idx ON driver_settlements (driver_id, created_at DESC);

-- The balance query runs on every go-online and every dispatch round, so the
-- entries behind it are worth an index of their own.
CREATE INDEX ledger_entries_cash_in_transit_idx
    ON ledger_entries (subject_id, created_at)
    WHERE account = 'CASH_IN_TRANSIT';

-- The table ships EMPTY, and that means something different here than it does
-- for commission_rates.
--
-- An unset commission refuses to settle, because paying a guessed rate is
-- worse than not paying. An unset cap must NOT refuse to let drivers work —
-- failing closed here would take every driver off the road at once over a
-- missing configuration row. So no cap means no limit, loudly logged and
-- surfaced to operations, and the owner sets the numbers.
