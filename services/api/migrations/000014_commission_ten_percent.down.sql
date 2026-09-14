-- Reopen version 1 and withdraw version 2, restoring the 20% rate.
DELETE FROM commission_rates WHERE version = 2 AND rate_bps = 1000 AND subject_type = 'DRIVER';

UPDATE commission_rates
   SET active_to = NULL
 WHERE version = 1
   AND subject_type = 'DRIVER';
