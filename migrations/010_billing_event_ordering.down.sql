DROP INDEX IF EXISTS idx_stripe_events_created;
DROP TABLE IF EXISTS stripe_events;

ALTER TABLE teams
    DROP COLUMN IF EXISTS billing_event_id,
    DROP COLUMN IF EXISTS billing_event_created;
