-- Durable, fenced send attempts. PostgreSQL is authoritative for whether an
-- email may be submitted to the provider; Redis is only a wake-up mechanism.
ALTER TABLE emails
    ADD COLUMN IF NOT EXISTS send_attempt_id UUID,
    ADD COLUMN IF NOT EXISTS send_fence_token UUID,
    ADD COLUMN IF NOT EXISTS send_attempt_state VARCHAR(32) NOT NULL DEFAULT 'none',
    ADD COLUMN IF NOT EXISTS send_lease_until TIMESTAMPTZ,
    ADD COLUMN IF NOT EXISTS ambiguous_at TIMESTAMPTZ,
    ADD COLUMN IF NOT EXISTS queue_recovery_pending BOOLEAN NOT NULL DEFAULT FALSE;

ALTER TABLE emails DROP CONSTRAINT IF EXISTS emails_status_check;
ALTER TABLE emails
    ADD CONSTRAINT emails_status_check CHECK (status IN (
        'queued', 'sending', 'sent', 'delivered', 'opened',
        'clicked', 'bounced', 'complained', 'failed', 'cancelled', 'ambiguous'
    ));

ALTER TABLE emails DROP CONSTRAINT IF EXISTS emails_send_attempt_state_check;
ALTER TABLE emails
    ADD CONSTRAINT emails_send_attempt_state_check CHECK (send_attempt_state IN (
        'none', 'leased', 'send_started', 'accepted', 'ambiguous', 'failed_terminal'
    ));

CREATE INDEX IF NOT EXISTS idx_emails_send_attempt_due
    ON emails (send_attempt_state, send_lease_until)
    WHERE send_attempt_state IN ('leased', 'send_started');
CREATE INDEX IF NOT EXISTS idx_emails_queue_recovery_pending
    ON emails (created_at)
    WHERE queue_recovery_pending = TRUE;

-- Authenticated provider callbacks are acknowledged only after this durable
-- inbox accepts them. They can then be correlated and replayed idempotently.
CREATE TABLE IF NOT EXISTS provider_event_inbox (
    event_id UUID PRIMARY KEY,
    provider_message_id VARCHAR(255) NOT NULL,
    event_type VARCHAR(100) NOT NULL,
    payload JSONB,
    email_id UUID REFERENCES emails(id) ON DELETE SET NULL,
    send_attempt_id UUID,
    status VARCHAR(20) NOT NULL DEFAULT 'pending'
        CHECK (status IN ('pending', 'processing', 'processed', 'ignored')),
    attempts INTEGER NOT NULL DEFAULT 0 CHECK (attempts >= 0),
    next_attempt_at TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    lease_until TIMESTAMPTZ,
    last_error TEXT,
    created_at TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    processed_at TIMESTAMPTZ
);

CREATE INDEX IF NOT EXISTS idx_provider_event_inbox_due
    ON provider_event_inbox (status, next_attempt_at, created_at);
CREATE INDEX IF NOT EXISTS idx_provider_event_inbox_provider
    ON provider_event_inbox (provider_message_id)
    WHERE status IN ('pending', 'processing');
CREATE INDEX IF NOT EXISTS idx_provider_event_inbox_attempt
    ON provider_event_inbox (send_attempt_id)
    WHERE status IN ('pending', 'processing') AND send_attempt_id IS NOT NULL;

-- Transactional webhook outbox. Domain state and this replayable work item are
-- committed together; a separate dispatcher expands it into endpoint-specific
-- deliveries.
CREATE TABLE IF NOT EXISTS webhook_outbox (
    id UUID PRIMARY KEY,
    team_id UUID NOT NULL REFERENCES teams(id) ON DELETE CASCADE,
    event_id UUID NOT NULL UNIQUE,
    event VARCHAR(100) NOT NULL,
    payload JSONB,
    retention_class VARCHAR(20) NOT NULL DEFAULT 'outbound'
        CHECK (retention_class IN ('outbound', 'inbound')),
    status VARCHAR(20) NOT NULL DEFAULT 'pending'
        CHECK (status IN ('pending', 'processing', 'dispatched', 'failed')),
    attempts INTEGER NOT NULL DEFAULT 0 CHECK (attempts >= 0),
    next_attempt_at TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    lease_until TIMESTAMPTZ,
    last_error TEXT,
    created_at TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    dispatched_at TIMESTAMPTZ
);

CREATE INDEX IF NOT EXISTS idx_webhook_outbox_due
    ON webhook_outbox (status, next_attempt_at, created_at);

ALTER TABLE webhook_deliveries
    ADD COLUMN IF NOT EXISTS retention_class VARCHAR(20) NOT NULL DEFAULT 'outbound';
ALTER TABLE webhook_deliveries ALTER COLUMN payload DROP NOT NULL;
ALTER TABLE webhook_deliveries DROP CONSTRAINT IF EXISTS webhook_deliveries_retention_class_check;
ALTER TABLE webhook_deliveries
    ADD CONSTRAINT webhook_deliveries_retention_class_check
    CHECK (retention_class IN ('outbound', 'inbound'));

CREATE INDEX IF NOT EXISTS idx_webhook_deliveries_retention
    ON webhook_deliveries (retention_class, created_at);
CREATE INDEX IF NOT EXISTS idx_webhook_outbox_retention
    ON webhook_outbox (retention_class, created_at);

UPDATE webhook_deliveries
SET retention_class = 'inbound'
WHERE event LIKE 'inbound.%';

-- Canonical contact identity is lower-case throughout the product. Merge
-- case-only duplicates without discarding profile data: an unsubscribe is
-- authoritative, the preferred row wins conflicting values, and missing names
-- or property keys are filled from older duplicates.
WITH ranked AS (
    SELECT c.*,
           lower(btrim(email)) AS canonical_email,
           ROW_NUMBER() OVER (
               PARTITION BY team_id, lower(btrim(email))
               ORDER BY (email = lower(btrim(email))) DESC, created_at, id
           ) AS position
    FROM contacts c
), merged_base AS (
    SELECT team_id,
           canonical_email,
           (ARRAY_AGG(first_name ORDER BY position)
               FILTER (WHERE NULLIF(btrim(first_name), '') IS NOT NULL))[1] AS first_name,
           (ARRAY_AGG(last_name ORDER BY position)
               FILTER (WHERE NULLIF(btrim(last_name), '') IS NOT NULL))[1] AS last_name,
           BOOL_AND(COALESCE(subscribed, TRUE)) AS all_subscribed
    FROM ranked
    GROUP BY team_id, canonical_email
), property_values AS (
    SELECT ranked.team_id,
           ranked.canonical_email,
           property.key,
           property.value,
           ROW_NUMBER() OVER (
               PARTITION BY ranked.team_id, ranked.canonical_email, property.key
               ORDER BY ranked.position
           ) AS property_position
    FROM ranked
    CROSS JOIN LATERAL jsonb_each(COALESCE(ranked.properties, '{}'::jsonb)) AS property
), merged_properties AS (
    SELECT team_id,
           canonical_email,
           jsonb_object_agg(key, value) FILTER (WHERE property_position = 1) AS properties
    FROM property_values
    GROUP BY team_id, canonical_email
), keepers AS (
    UPDATE contacts contact
    SET first_name = COALESCE(merged_base.first_name, contact.first_name),
        last_name = COALESCE(merged_base.last_name, contact.last_name),
        subscribed = merged_base.all_subscribed,
        properties = COALESCE(merged_properties.properties, '{}'::jsonb),
        updated_at = NOW()
    FROM ranked
    JOIN merged_base
      ON merged_base.team_id IS NOT DISTINCT FROM ranked.team_id
     AND merged_base.canonical_email = ranked.canonical_email
    LEFT JOIN merged_properties
      ON merged_properties.team_id IS NOT DISTINCT FROM ranked.team_id
     AND merged_properties.canonical_email = ranked.canonical_email
    WHERE contact.id = ranked.id AND ranked.position = 1
    RETURNING contact.id
)
DELETE FROM contacts contact
USING ranked
WHERE contact.id = ranked.id AND ranked.position > 1;

UPDATE contacts SET email = lower(btrim(email));
ALTER TABLE contacts DROP CONSTRAINT IF EXISTS contacts_team_id_email_key;
ALTER TABLE contacts ADD CONSTRAINT contacts_team_id_email_key UNIQUE (team_id, email);
CREATE UNIQUE INDEX IF NOT EXISTS ux_contacts_team_canonical_email
    ON contacts (team_id, lower(email));

-- Suppression lookups use the same canonical identity. Preserve the strongest
-- signal when old rows collide: complaint, then bounce, then unsubscribe.
WITH ranked AS (
    SELECT id,
           ROW_NUMBER() OVER (
               PARTITION BY team_id, lower(btrim(email))
               ORDER BY CASE reason
                   WHEN 'complaint' THEN 3
                   WHEN 'bounce' THEN 2
                   ELSE 1
               END DESC, created_at, id
           ) AS position,
           FIRST_VALUE(reason) OVER (
               PARTITION BY team_id, lower(btrim(email))
               ORDER BY CASE reason
                   WHEN 'complaint' THEN 3
                   WHEN 'bounce' THEN 2
                   ELSE 1
               END DESC, created_at, id
           ) AS retained_reason
    FROM suppressions
)
UPDATE suppressions s
SET email = lower(btrim(s.email)),
    reason = ranked.retained_reason,
    updated_at = NOW()
FROM ranked
WHERE s.id = ranked.id AND ranked.position = 1;

WITH ranked AS (
    SELECT id,
           ROW_NUMBER() OVER (
               PARTITION BY team_id, lower(btrim(email))
               ORDER BY CASE reason
                   WHEN 'complaint' THEN 3
                   WHEN 'bounce' THEN 2
                   ELSE 1
               END DESC, created_at, id
           ) AS position
    FROM suppressions
)
DELETE FROM suppressions s
USING ranked
WHERE s.id = ranked.id AND ranked.position > 1;

UPDATE suppressions SET email = lower(btrim(email));
ALTER TABLE suppressions DROP CONSTRAINT IF EXISTS suppressions_team_id_email_key;
ALTER TABLE suppressions ADD CONSTRAINT suppressions_team_id_email_key UNIQUE (team_id, email);
CREATE UNIQUE INDEX IF NOT EXISTS ux_suppressions_team_canonical_email
    ON suppressions (team_id, lower(email));

-- New internal tables and the Stripe event inbox must not be exposed through
-- Supabase public roles even when default grants drift.
DO $$
DECLARE
    role_name TEXT;
    table_name TEXT;
BEGIN
    FOREACH role_name IN ARRAY ARRAY['anon', 'authenticated'] LOOP
        IF EXISTS (SELECT 1 FROM pg_roles WHERE rolname = role_name) THEN
            FOREACH table_name IN ARRAY ARRAY[
                'provider_event_inbox', 'webhook_outbox', 'webhook_deliveries',
                'stripe_events', 'email_batches', 'suppressions'
            ] LOOP
                IF to_regclass('public.' || table_name) IS NOT NULL THEN
                    EXECUTE format('REVOKE ALL PRIVILEGES ON TABLE public.%I FROM %I', table_name, role_name);
                END IF;
            END LOOP;
        END IF;
    END LOOP;
END
$$;
