-- Business decisions BD-01, BD-02, BD-04, BD-05, BD-11 and BD-12.
--
-- Every mechanism these settle was already built; each refused to act rather
-- than invent a number. This migration supplies the numbers the owner chose on
-- 2026-08-28 and nothing else. The refusal paths stay exactly where they were:
-- a value that is absent for a market or a merchant still fails loudly. What
-- changes is that the platform-wide defaults now exist.
--
-- Values remain configuration. Nothing here becomes a Go constant, so changing
-- a rate is a row edit and an audit trail, not a deploy.

-- --------------------------------------------------------------------------
-- Platform-wide settings.
--
-- The values that belong to the platform rather than to a market, a merchant
-- or a tariff. Integer-only: money is minor units and rates are basis points,
-- so a single bigint column holds every one of them without a float appearing
-- anywhere (ADR-008).
CREATE TABLE platform_settings (
    key         text PRIMARY KEY,
    value       bigint      NOT NULL,
    -- Which business decision this value answers, so a number in production
    -- can always be traced back to the decision that chose it.
    decision    text,
    unit        text        NOT NULL,
    description text        NOT NULL,
    updated_at  timestamptz NOT NULL DEFAULT now(),
    CONSTRAINT platform_settings_unit_valid CHECK (
        unit IN ('MINOR_UNITS', 'BASIS_POINTS', 'SECONDS', 'COUNT', 'BOOLEAN'))
);

INSERT INTO platform_settings (key, value, decision, unit, description) VALUES
    -- BD-01. Cancelling is free for two minutes after a driver accepts; after
    -- that it costs PKR 100. The clock starts at acceptance, not at booking,
    -- so a customer whose request never found a driver is never charged.
    ('cancellation.grace_seconds',      120,   'BD-01', 'SECONDS',
     'Free-cancellation window, measured from driver acceptance'),
    ('cancellation.fee_minor',          10000, 'BD-01', 'MINOR_UNITS',
     'Cancellation fee in PKR minor units once the grace window has passed'),

    -- BD-02. Surge is demand-triggered and capped at 1.5x. The cap is also
    -- written into every tariff below; this row is the platform ceiling that
    -- no market may configure past.
    ('surge.max_bps',                   15000, 'BD-02', 'BASIS_POINTS',
     'Hard ceiling on the demand multiplier: 15000 bps is 1.5x'),
    ('surge.min_supply_for_surge',      1,     'BD-02', 'COUNT',
     'Available drivers below which demand is not computed at all'),

    -- BD-04. Three dispatch rounds over roughly ninety seconds, then the job
    -- ends as EXPIRED with a NO_SUPPLY reason. Nothing is charged.
    ('dispatch.search_deadline_seconds', 90,   'BD-04', 'SECONDS',
     'Total time a job may spend SEARCHING before it expires as NO_SUPPLY'),

    -- BD-12. Ten minutes for a merchant to answer, then the order cancels
    -- itself. This is the fallback for a merchant with no explicit override.
    ('merchant.accept_timeout_seconds', 600,   'BD-12', 'SECONDS',
     'Default merchant acceptance window when a merchant has no override'),

    -- BD-11. The customer pays what the substitute actually costs, up or
    -- down. A value of 1 means the difference passes through to the customer;
    -- 0 would mean the platform absorbs it.
    ('substitution.customer_pays_difference', 1, 'BD-11', 'BOOLEAN',
     'Whether a substitution price difference is charged to the customer');

-- --------------------------------------------------------------------------
-- BD-05: commission is a flat 20% of the gross earning on every service.
--
-- The table was built empty on purpose and CommissionRateFor still returns
-- ErrNoCommission for any combination without a row — a new service added
-- tomorrow will refuse to pay until someone sets its rate, which is the
-- behaviour worth keeping.
INSERT INTO commission_rates (job_type, subject_type, rate_bps, flat_minor, version) VALUES
    ('RIDE',    'DRIVER', 2000, 0, 1),
    ('PARCEL',  'DRIVER', 2000, 0, 1),
    ('GROCERY', 'DRIVER', 2000, 0, 1),
    ('CARGO',   'DRIVER', 2000, 0, 1),
    ('FREIGHT', 'DRIVER', 2000, 0, 1);

-- --------------------------------------------------------------------------
-- BD-02: every tariff created from now on carries the 1.5x ceiling.
--
-- The column default was 10000 — the term present and inert. Existing rows are
-- raised to the decided cap; the CHECK constraint already bounds the column at
-- 30000, so the platform ceiling is enforced in the application against
-- platform_settings rather than loosened here.
ALTER TABLE pricing_tariffs ALTER COLUMN demand_max_bps SET DEFAULT 15000;
UPDATE pricing_tariffs SET demand_max_bps = 15000 WHERE demand_max_bps = 10000;

-- --------------------------------------------------------------------------
-- BD-12: ten minutes, for merchants that do not set their own.
ALTER TABLE merchant_config ALTER COLUMN accept_timeout_seconds SET DEFAULT 600;
UPDATE merchant_config SET accept_timeout_seconds = 600 WHERE accept_timeout_seconds IS NULL;

-- Why an order cancelled itself, so an expiry is distinguishable from a
-- customer changing their mind or a merchant refusing.
ALTER TABLE orders ADD COLUMN cancelled_by text;
ALTER TABLE orders ADD CONSTRAINT orders_cancelled_by_valid CHECK (
    cancelled_by IS NULL OR cancelled_by IN ('CUSTOMER', 'MERCHANT', 'ADMIN', 'SUPPORT', 'SYSTEM'));

-- The sweeper's query is already indexed: 000009 created
-- orders_accept_deadline_idx for exactly this, before there was a duration to
-- put in accept_deadline. Nothing to add here.

-- --------------------------------------------------------------------------
-- BD-04: three rounds, not four.
--
-- The rings stay as they are — the fourth ring is simply never reached, and
-- leaving it configured means raising max_attempts is a one-row change.
UPDATE dispatch_config SET max_attempts = 3, updated_at = now();

-- Ending a search needs a reason a customer can be shown. The job's terminal
-- state is EXPIRED (document 015); this records why it expired.
ALTER TABLE jobs ADD COLUMN failure_reason text;
ALTER TABLE jobs ADD CONSTRAINT jobs_failure_reason_valid CHECK (
    failure_reason IS NULL OR failure_reason IN (
        'NO_SUPPLY', 'PAYMENT_FAILED', 'CUSTOMER_UNREACHABLE',
        'PROVIDER_UNREACHABLE', 'MERCHANT_REJECTED', 'SYSTEM_ERROR'));

-- The sweeper's query: jobs that have been searching too long.
CREATE INDEX jobs_searching_since_idx ON jobs (updated_at) WHERE status = 'SEARCHING';

-- --------------------------------------------------------------------------
-- BD-11: the customer pays the substitute's actual price.
--
-- No schema change. Document 74 forbids mutating the original order line, so
-- the substitute price is not written onto it — the price already lives in
-- order_item_issues.substitute_unit_price_minor, and the order total reads it
-- from there once the substitution is settled. What the customer ordered and
-- what they were charged stay in separate rows, which is the point.
--
-- The index makes that per-line lookup cheap enough to run on every total.
CREATE INDEX order_item_issues_settled_idx
    ON order_item_issues (order_item_id, created_at DESC)
 WHERE action = 'SUBSTITUTE'
   AND substitute_unit_price_minor IS NOT NULL
   AND resolution IN ('CUSTOMER_ACCEPTED', 'AUTO_APPLIED');
