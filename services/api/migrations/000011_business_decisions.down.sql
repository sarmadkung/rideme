-- Reverses 000011. The decided values are removed and the mechanisms return to
-- refusing, which is the state they shipped in.

DROP INDEX IF EXISTS order_item_issues_settled_idx;

DROP INDEX IF EXISTS jobs_searching_since_idx;
ALTER TABLE jobs DROP CONSTRAINT IF EXISTS jobs_failure_reason_valid;
ALTER TABLE jobs DROP COLUMN IF EXISTS failure_reason;

UPDATE dispatch_config SET max_attempts = 4, updated_at = now();

ALTER TABLE orders DROP CONSTRAINT IF EXISTS orders_cancelled_by_valid;
ALTER TABLE orders DROP COLUMN IF EXISTS cancelled_by;

-- Back to no default. Existing merchants keep the timeout they were given:
-- clearing it would strand orders mid-flight, and 000011's own comment is that
-- an unset timeout fails loudly — reintroducing that failure on a rollback
-- would take down a working merchant rather than restore a safe state.
ALTER TABLE merchant_config ALTER COLUMN accept_timeout_seconds DROP DEFAULT;

ALTER TABLE pricing_tariffs ALTER COLUMN demand_max_bps SET DEFAULT 10000;
UPDATE pricing_tariffs SET demand_max_bps = 10000 WHERE demand_max_bps = 15000;

DELETE FROM commission_rates WHERE version = 1 AND rate_bps = 2000 AND subject_type = 'DRIVER';

DROP TABLE IF EXISTS platform_settings;
