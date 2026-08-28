-- Payments and the financial ledger (documents 19, 51-64).
--
-- Every table here is append-only in spirit and, where it matters, in
-- constraint. Document 53: "Never mutate historical financial entries.
-- Corrections use reversal/adjustment transactions." A ledger you can edit is
-- not a ledger; it is a spreadsheet with extra steps.

-- --- payment intents (document 52) -------------------------------------------

CREATE TABLE payment_intents (
    id                 uuid PRIMARY KEY DEFAULT gen_random_uuid(),
    job_id             uuid        REFERENCES jobs (id) ON DELETE RESTRICT,
    order_id           uuid        REFERENCES orders (id) ON DELETE RESTRICT,
    customer_user_id   uuid        NOT NULL REFERENCES users (id) ON DELETE RESTRICT,
    amount_minor       bigint      NOT NULL,
    currency           text        NOT NULL DEFAULT 'PKR',
    status             text        NOT NULL DEFAULT 'REQUIRES_PAYMENT',
    method             text        NOT NULL,
    provider           text        NOT NULL DEFAULT 'none',
    -- The provider's identifier. Unique so a provider reference cannot be
    -- attached to two intents.
    provider_reference text,
    idempotency_key    text,
    captured_minor     bigint      NOT NULL DEFAULT 0,
    refunded_minor     bigint      NOT NULL DEFAULT 0,
    expires_at         timestamptz,
    created_at         timestamptz NOT NULL DEFAULT now(),
    updated_at         timestamptz NOT NULL DEFAULT now(),
    CONSTRAINT payment_intents_currency_valid CHECK (currency = 'PKR'),
    CONSTRAINT payment_intents_amount_positive CHECK (amount_minor > 0),
    CONSTRAINT payment_intents_method_valid CHECK (method IN ('CASH', 'CARD', 'WALLET', 'BANK')),
    CONSTRAINT payment_intents_status_valid CHECK (status IN (
        'REQUIRES_PAYMENT', 'PROCESSING', 'AUTHORIZED', 'CAPTURED',
        'PARTIALLY_REFUNDED', 'REFUNDED', 'FAILED', 'CANCELLED', 'EXPIRED')),
    -- Captures and refunds can never exceed what was intended, whatever a
    -- webhook claims. A provider bug must not become a platform liability.
    CONSTRAINT payment_intents_captured_bounded CHECK (captured_minor BETWEEN 0 AND amount_minor),
    CONSTRAINT payment_intents_refund_bounded CHECK (refunded_minor BETWEEN 0 AND captured_minor),
    -- An intent pays for exactly one thing.
    CONSTRAINT payment_intents_one_subject CHECK (num_nonnulls(job_id, order_id) = 1)
);

CREATE UNIQUE INDEX payment_intents_provider_ref_idx
    ON payment_intents (provider, provider_reference) WHERE provider_reference IS NOT NULL;
CREATE UNIQUE INDEX payment_intents_idempotency_idx
    ON payment_intents (customer_user_id, idempotency_key) WHERE idempotency_key IS NOT NULL;
-- At most one live intent per job: two would let a customer be charged twice
-- for one ride.
CREATE UNIQUE INDEX payment_intents_one_live_per_job_idx
    ON payment_intents (job_id)
    WHERE job_id IS NOT NULL AND status IN ('REQUIRES_PAYMENT', 'PROCESSING', 'AUTHORIZED', 'CAPTURED');
CREATE UNIQUE INDEX payment_intents_one_live_per_order_idx
    ON payment_intents (order_id)
    WHERE order_id IS NOT NULL AND status IN ('REQUIRES_PAYMENT', 'PROCESSING', 'AUTHORIZED', 'CAPTURED');

-- --- provider webhooks (document 58) -----------------------------------------

-- Deduplicated by provider event id. Document 52: "Webhook processing must be
-- idempotent. Store provider event IDs." A provider replaying a capture must
-- not capture twice.
CREATE TABLE payment_webhook_events (
    provider     text        NOT NULL,
    event_id     text        NOT NULL,
    intent_id    uuid        REFERENCES payment_intents (id) ON DELETE SET NULL,
    event_type   text        NOT NULL,
    payload      jsonb       NOT NULL,
    signature_ok boolean     NOT NULL,
    processed_at timestamptz,
    received_at  timestamptz NOT NULL DEFAULT now(),
    PRIMARY KEY (provider, event_id)
);

CREATE INDEX payment_webhook_events_unprocessed_idx
    ON payment_webhook_events (received_at) WHERE processed_at IS NULL;

-- --- double-entry ledger (document 53) ---------------------------------------

CREATE TABLE ledger_accounts (
    code       text PRIMARY KEY,
    label      text        NOT NULL,
    -- Which side increases this account. Getting this wrong inverts every
    -- balance the account appears in, so it is data rather than convention.
    normal_side text       NOT NULL,
    created_at timestamptz NOT NULL DEFAULT now(),
    CONSTRAINT ledger_accounts_side_valid CHECK (normal_side IN ('DEBIT', 'CREDIT'))
);

INSERT INTO ledger_accounts (code, label, normal_side) VALUES
    ('CUSTOMER_RECEIVABLE', 'Customer Receivable', 'DEBIT'),
    ('CUSTOMER_WALLET',     'Customer Wallet',     'CREDIT'),
    ('PLATFORM_CLEARING',   'Platform Clearing',   'DEBIT'),
    ('PLATFORM_REVENUE',    'Platform Revenue',    'CREDIT'),
    ('DRIVER_EXPENSE',      'Driver Expense',      'DEBIT'),
    ('DRIVER_PAYABLE',      'Driver Payable',      'CREDIT'),
    ('MERCHANT_PAYABLE',    'Merchant Payable',    'CREDIT'),
    ('TAX_PAYABLE',         'Tax Payable',         'CREDIT'),
    ('REFUND_LIABILITY',    'Refund Liability',    'CREDIT'),
    ('CASH_IN_TRANSIT',     'Cash In Transit',     'DEBIT');

CREATE TABLE ledger_transactions (
    id          uuid PRIMARY KEY DEFAULT gen_random_uuid(),
    kind        text        NOT NULL,
    -- What this transaction is about, for reconciliation and support.
    job_id      uuid        REFERENCES jobs (id) ON DELETE SET NULL,
    order_id    uuid        REFERENCES orders (id) ON DELETE SET NULL,
    intent_id   uuid        REFERENCES payment_intents (id) ON DELETE SET NULL,
    -- A correction references what it reverses (document 53: corrections use
    -- new transactions, never edits).
    reverses_id uuid        REFERENCES ledger_transactions (id) ON DELETE RESTRICT,
    idempotency_key text UNIQUE,
    description text,
    created_at  timestamptz NOT NULL DEFAULT now(),
    CONSTRAINT ledger_transactions_kind_valid CHECK (kind IN (
        'PAYMENT', 'CAPTURE', 'REFUND', 'EARNING', 'COMMISSION', 'PAYOUT',
        'SETTLEMENT', 'ADJUSTMENT', 'REVERSAL', 'COD_COLLECTION'))
);

CREATE INDEX ledger_transactions_job_idx ON ledger_transactions (job_id) WHERE job_id IS NOT NULL;
CREATE INDEX ledger_transactions_intent_idx ON ledger_transactions (intent_id) WHERE intent_id IS NOT NULL;
CREATE INDEX ledger_transactions_created_idx ON ledger_transactions (created_at);

CREATE TABLE ledger_entries (
    id             bigserial PRIMARY KEY,
    transaction_id uuid        NOT NULL REFERENCES ledger_transactions (id) ON DELETE RESTRICT,
    account        text        NOT NULL REFERENCES ledger_accounts (code) ON DELETE RESTRICT,
    -- Signed: a debit is positive, a credit is negative, and every
    -- transaction's entries sum to zero. One signed column rather than two
    -- nullable ones means "balances" is `sum() = 0` — a query anyone can run
    -- and nobody can get subtly wrong.
    amount_minor   bigint      NOT NULL,
    currency       text        NOT NULL DEFAULT 'PKR',
    -- Who the entry is for, so a driver's balance is a filter rather than a
    -- separate table that can drift.
    subject_type   text,
    subject_id     uuid,
    created_at     timestamptz NOT NULL DEFAULT now(),
    CONSTRAINT ledger_entries_currency_valid CHECK (currency = 'PKR'),
    CONSTRAINT ledger_entries_nonzero CHECK (amount_minor <> 0),
    CONSTRAINT ledger_entries_subject_valid CHECK (subject_type IS NULL OR subject_type IN (
        'CUSTOMER', 'DRIVER', 'MERCHANT', 'PLATFORM'))
);

CREATE INDEX ledger_entries_transaction_idx ON ledger_entries (transaction_id);
CREATE INDEX ledger_entries_account_idx ON ledger_entries (account, created_at);
CREATE INDEX ledger_entries_subject_idx ON ledger_entries (subject_type, subject_id, created_at)
    WHERE subject_id IS NOT NULL;

-- Immutability, enforced rather than documented. Document 53: "Entries cannot
-- be edited or deleted through ordinary APIs." A trigger makes that true for
-- every path, including a migration or an operator with psql.
CREATE OR REPLACE FUNCTION ledger_entries_immutable() RETURNS trigger AS $$
BEGIN
    RAISE EXCEPTION 'ledger entries are immutable; use a reversal transaction';
END;
$$ LANGUAGE plpgsql;

CREATE TRIGGER ledger_entries_no_update
    BEFORE UPDATE OR DELETE ON ledger_entries
    FOR EACH ROW EXECUTE FUNCTION ledger_entries_immutable();

-- --- settlement and payouts (documents 55, 56) -------------------------------

CREATE TABLE payouts (
    id            uuid PRIMARY KEY DEFAULT gen_random_uuid(),
    subject_type  text        NOT NULL,
    subject_id    uuid        NOT NULL,
    amount_minor  bigint      NOT NULL,
    currency      text        NOT NULL DEFAULT 'PKR',
    status        text        NOT NULL DEFAULT 'PENDING',
    -- The period this payout settles, so the same period cannot be paid twice.
    period_start  timestamptz NOT NULL,
    period_end    timestamptz NOT NULL,
    reference     text,
    failure_reason text,
    created_at    timestamptz NOT NULL DEFAULT now(),
    updated_at    timestamptz NOT NULL DEFAULT now(),
    CONSTRAINT payouts_subject_valid CHECK (subject_type IN ('DRIVER', 'MERCHANT')),
    CONSTRAINT payouts_amount_positive CHECK (amount_minor > 0),
    CONSTRAINT payouts_currency_valid CHECK (currency = 'PKR'),
    CONSTRAINT payouts_status_valid CHECK (status IN ('PENDING', 'PROCESSING', 'PAID', 'FAILED', 'CANCELLED')),
    CONSTRAINT payouts_period_ordered CHECK (period_end > period_start)
);

-- One live payout per subject per period. Two payout requests for the same
-- period is one of the races document 59 names explicitly.
CREATE UNIQUE INDEX payouts_one_per_period_idx
    ON payouts (subject_type, subject_id, period_start, period_end)
    WHERE status IN ('PENDING', 'PROCESSING', 'PAID');

-- --- reconciliation (document 58) --------------------------------------------

-- Mismatches become cases, never silent fixes. Document 58: "Create
-- reconciliation cases rather than silently fixing mismatches", and BD-08's
-- recommended default is zero tolerance — any discrepancy is investigated.
CREATE TABLE reconciliation_cases (
    id             uuid PRIMARY KEY DEFAULT gen_random_uuid(),
    provider       text        NOT NULL,
    kind           text        NOT NULL,
    intent_id      uuid        REFERENCES payment_intents (id) ON DELETE SET NULL,
    provider_reference text,
    expected_minor bigint,
    actual_minor   bigint,
    currency       text        NOT NULL DEFAULT 'PKR',
    status         text        NOT NULL DEFAULT 'OPEN',
    detail         jsonb       NOT NULL DEFAULT '{}'::jsonb,
    resolved_by    uuid        REFERENCES users (id) ON DELETE SET NULL,
    resolution     text,
    created_at     timestamptz NOT NULL DEFAULT now(),
    resolved_at    timestamptz,
    CONSTRAINT reconciliation_kind_valid CHECK (kind IN (
        'MISSING_INTERNAL', 'MISSING_PROVIDER', 'DUPLICATE_CAPTURE',
        'AMOUNT_MISMATCH', 'UNEXPECTED_REFUND', 'SETTLEMENT_DIFFERENCE')),
    CONSTRAINT reconciliation_status_valid CHECK (status IN ('OPEN', 'INVESTIGATING', 'RESOLVED', 'WRITTEN_OFF'))
);

CREATE INDEX reconciliation_cases_open_idx ON reconciliation_cases (created_at) WHERE status = 'OPEN';

-- Commission configuration. BD-05 is a PRODUCT_DECISION: the rates are the
-- owner's. The table ships empty and earnings cannot be computed without a
-- row, which is the register's instruction — "Build commission as
-- configuration from day one. Values are the owner's."
CREATE TABLE commission_rates (
    id           uuid PRIMARY KEY DEFAULT gen_random_uuid(),
    job_type     text        NOT NULL,
    subject_type text        NOT NULL,
    -- Basis points, integer. A percentage is never a float here.
    rate_bps     integer     NOT NULL,
    flat_minor   bigint      NOT NULL DEFAULT 0,
    version      integer     NOT NULL DEFAULT 1,
    active_from  timestamptz NOT NULL DEFAULT now(),
    active_to    timestamptz,
    CONSTRAINT commission_job_type_valid CHECK (job_type IN ('RIDE', 'PARCEL', 'GROCERY', 'CARGO', 'FREIGHT')),
    CONSTRAINT commission_subject_valid CHECK (subject_type IN ('DRIVER', 'MERCHANT')),
    CONSTRAINT commission_rate_bounded CHECK (rate_bps BETWEEN 0 AND 10000),
    CONSTRAINT commission_flat_nonneg CHECK (flat_minor >= 0),
    UNIQUE (job_type, subject_type, version)
);
