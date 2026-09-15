DROP INDEX IF EXISTS ledger_entries_cash_in_transit_idx;
DROP TABLE IF EXISTS driver_settlements;
ALTER TABLE drivers DROP CONSTRAINT IF EXISTS drivers_credit_cap_positive;
ALTER TABLE drivers DROP COLUMN IF EXISTS credit_cap_minor;
DROP TABLE IF EXISTS driver_credit_limits;
