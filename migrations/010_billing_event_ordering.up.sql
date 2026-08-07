ALTER TABLE teams
    ADD COLUMN IF NOT EXISTS billing_event_created BIGINT NOT NULL DEFAULT 0,
    ADD COLUMN IF NOT EXISTS billing_event_id VARCHAR(255) NOT NULL DEFAULT '';

CREATE TABLE IF NOT EXISTS stripe_events (
    event_id VARCHAR(255) PRIMARY KEY,
    event_type VARCHAR(100) NOT NULL,
    event_created BIGINT NOT NULL DEFAULT 0,
    received_at TIMESTAMPTZ NOT NULL DEFAULT NOW()
);

CREATE INDEX IF NOT EXISTS idx_stripe_events_created
    ON stripe_events (event_created);
