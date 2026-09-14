-- BD-05 revised by the owner on 2026-09-14: the platform commission falls from
-- a flat 20% to a flat 10%.
--
-- Version 2 rather than an UPDATE of version 1. `commission_rates` carries a
-- version and an active window precisely so a rate change is a new row: a job
-- settled last week was settled at 20%, and rewriting the row that says so
-- would make that earning inexplicable to the driver who received it and to
-- whoever answers their question about it.
--
-- CommissionRateFor orders by version descending within the active window, so
-- closing version 1 and opening version 2 at the same instant leaves exactly
-- one answer at every point in time.

UPDATE commission_rates
   SET active_to = now()
 WHERE version = 1
   AND subject_type = 'DRIVER'
   AND active_to IS NULL;

INSERT INTO commission_rates (job_type, subject_type, rate_bps, flat_minor, version) VALUES
    ('RIDE',    'DRIVER', 1000, 0, 2),
    ('PARCEL',  'DRIVER', 1000, 0, 2),
    ('GROCERY', 'DRIVER', 1000, 0, 2),
    ('CARGO',   'DRIVER', 1000, 0, 2),
    ('FREIGHT', 'DRIVER', 1000, 0, 2);

-- No MERCHANT row is added. The owner decided on 2026-09-14 that the platform
-- takes no cut from a shop's goods, and the refusal BD-05 kept is what records
-- that: CommissionRateFor returns ErrNoCommission for MERCHANT, so nothing can
-- quietly start charging shops without a row and a decision behind it.
