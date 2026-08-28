ALTER TABLE assignments DROP COLUMN IF EXISTS score;
ALTER TABLE assignments DROP COLUMN IF EXISTS attempt_id;
DROP TABLE IF EXISTS dispatch_config;
DROP TABLE IF EXISTS event_outbox;
DROP TABLE IF EXISTS processed_events;
DROP TABLE IF EXISTS dispatch_scores;
DROP TABLE IF EXISTS dispatch_attempts;
DROP TABLE IF EXISTS driver_reservations;
