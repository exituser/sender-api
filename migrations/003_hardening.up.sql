-- Idempotent outbound requests.
ALTER TABLE emails ADD COLUMN IF NOT EXISTS idempotency_key VARCHAR(255);
ALTER TABLE emails ADD COLUMN IF NOT EXISTS idempotency_hash CHAR(64);
ALTER TABLE emails ADD COLUMN IF NOT EXISTS provider_message_id VARCHAR(255);
ALTER TABLE emails ADD COLUMN IF NOT EXISTS sending_at TIMESTAMPTZ;

CREATE UNIQUE INDEX IF NOT EXISTS ux_emails_team_idempotency_key
    ON emails (team_id, idempotency_key)
    WHERE idempotency_key IS NOT NULL;

CREATE UNIQUE INDEX IF NOT EXISTS ux_emails_provider_message_id
    ON emails (provider_message_id)
    WHERE provider_message_id IS NOT NULL;

-- SES/SNS retries must not create duplicate inbound messages for one team.
CREATE UNIQUE INDEX IF NOT EXISTS ux_inbound_emails_team_message_id
    ON inbound_emails (team_id, message_id)
    WHERE message_id IS NOT NULL;

-- A verified sender domain belongs to one team in a shared SES account.
CREATE UNIQUE INDEX IF NOT EXISTS ux_domains_normalized_name
    ON domains (lower(trim(trailing '.' FROM name)));

CREATE UNIQUE INDEX IF NOT EXISTS ux_api_keys_key_hash
    ON api_keys (key_hash);

-- Query-specific tenant indexes avoid repeated full scans and sorts.
CREATE INDEX IF NOT EXISTS idx_emails_team_created_at
    ON emails (team_id, created_at DESC);
CREATE INDEX IF NOT EXISTS idx_contacts_team_created_at
    ON contacts (team_id, created_at DESC);
CREATE INDEX IF NOT EXISTS idx_inbound_emails_team_created_at
    ON inbound_emails (team_id, created_at DESC);
CREATE INDEX IF NOT EXISTS idx_email_events_email_timestamp
    ON email_events (email_id, timestamp DESC);

-- Make tenant ownership mandatory for newly hardened installations. If an
-- existing database contains bad legacy rows, the migration fails safely and
-- requires an explicit operator cleanup instead of silently changing data.
ALTER TABLE team_members ALTER COLUMN team_id SET NOT NULL;
ALTER TABLE api_keys ALTER COLUMN team_id SET NOT NULL;
ALTER TABLE domains ALTER COLUMN team_id SET NOT NULL;
ALTER TABLE emails ALTER COLUMN team_id SET NOT NULL;
ALTER TABLE inbound_emails ALTER COLUMN team_id SET NOT NULL;
ALTER TABLE webhooks ALTER COLUMN team_id SET NOT NULL;

CREATE TABLE IF NOT EXISTS webhook_deliveries (
    id UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    webhook_id UUID NOT NULL REFERENCES webhooks(id) ON DELETE CASCADE,
    event_id UUID NOT NULL,
    event VARCHAR(100) NOT NULL,
    payload JSONB NOT NULL,
    status VARCHAR(20) NOT NULL DEFAULT 'pending'
        CHECK (status IN ('pending', 'sending', 'delivered', 'failed')),
    attempts INTEGER NOT NULL DEFAULT 0 CHECK (attempts >= 0),
    next_attempt_at TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    lease_until TIMESTAMPTZ,
    last_error TEXT,
    created_at TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    delivered_at TIMESTAMPTZ,
    UNIQUE(webhook_id, event_id)
);

CREATE INDEX IF NOT EXISTS idx_webhook_deliveries_due
    ON webhook_deliveries (status, next_attempt_at);

-- The Supabase image creates application tables as supabase_admin. Keep the
-- optional postgres role limited to the public schema objects it
-- needs instead of changing table ownership. The Supabase image creates this
-- role after init scripts on a fresh volume, so the grant is conditional.
DO $$
BEGIN
    IF EXISTS (SELECT 1 FROM pg_roles WHERE rolname = 'postgres') THEN
        EXECUTE 'GRANT USAGE ON SCHEMA public TO postgres';
        EXECUTE 'GRANT SELECT, INSERT, UPDATE, DELETE ON ALL TABLES IN SCHEMA public TO postgres';
        EXECUTE 'GRANT USAGE, SELECT ON ALL SEQUENCES IN SCHEMA public TO postgres';
        EXECUTE 'ALTER DEFAULT PRIVILEGES IN SCHEMA public GRANT SELECT, INSERT, UPDATE, DELETE ON TABLES TO postgres';
        EXECUTE 'ALTER DEFAULT PRIVILEGES IN SCHEMA public GRANT USAGE, SELECT ON SEQUENCES TO postgres';
    END IF;
END
$$;
