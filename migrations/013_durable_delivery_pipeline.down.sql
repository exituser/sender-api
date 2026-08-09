-- Refuse a rollback that would silently discard provider evidence, replayable
-- webhook work, or fenced send-attempt state. Operators must drain/reconcile
-- these records explicitly before downgrading the schema.
DO $$
BEGIN
    IF EXISTS (SELECT 1 FROM provider_event_inbox)
       OR EXISTS (SELECT 1 FROM webhook_outbox)
       OR EXISTS (
           SELECT 1
           FROM emails
           WHERE send_attempt_id IS NOT NULL
              OR send_fence_token IS NOT NULL
              OR send_attempt_state <> 'none'
              OR send_lease_until IS NOT NULL
              OR ambiguous_at IS NOT NULL
              OR queue_recovery_pending
              OR status = 'ambiguous'
       ) THEN
        RAISE EXCEPTION USING
            MESSAGE = 'migration 013 rollback blocked: durable delivery state is not empty',
            HINT = 'reconcile provider events, webhook work, and send attempts before retrying the downgrade';
    END IF;
END
$$;

DROP INDEX IF EXISTS ux_contacts_team_canonical_email;
ALTER TABLE contacts DROP CONSTRAINT IF EXISTS contacts_team_id_email_key;
ALTER TABLE contacts ADD CONSTRAINT contacts_team_id_email_key UNIQUE (team_id, email);

DROP INDEX IF EXISTS ux_suppressions_team_canonical_email;
ALTER TABLE suppressions DROP CONSTRAINT IF EXISTS suppressions_team_id_email_key;
ALTER TABLE suppressions ADD CONSTRAINT suppressions_team_id_email_key UNIQUE (team_id, email);

DROP INDEX IF EXISTS idx_webhook_outbox_retention;
DROP INDEX IF EXISTS idx_webhook_deliveries_retention;
ALTER TABLE webhook_deliveries DROP CONSTRAINT IF EXISTS webhook_deliveries_retention_class_check;
ALTER TABLE webhook_deliveries DROP COLUMN IF EXISTS retention_class;
UPDATE webhook_deliveries SET payload = '{}'::jsonb WHERE payload IS NULL;
ALTER TABLE webhook_deliveries ALTER COLUMN payload SET NOT NULL;

DROP TABLE IF EXISTS webhook_outbox;
DROP TABLE IF EXISTS provider_event_inbox;

DROP INDEX IF EXISTS idx_emails_queue_recovery_pending;
DROP INDEX IF EXISTS idx_emails_send_attempt_due;
ALTER TABLE emails DROP CONSTRAINT IF EXISTS emails_send_attempt_state_check;
ALTER TABLE emails DROP CONSTRAINT IF EXISTS emails_status_check;
UPDATE emails SET status = 'failed' WHERE status = 'ambiguous';
ALTER TABLE emails
    ADD CONSTRAINT emails_status_check CHECK (status IN (
        'queued', 'sending', 'sent', 'delivered', 'opened',
        'clicked', 'bounced', 'complained', 'failed', 'cancelled'
    ));
ALTER TABLE emails
    DROP COLUMN IF EXISTS queue_recovery_pending,
    DROP COLUMN IF EXISTS ambiguous_at,
    DROP COLUMN IF EXISTS send_lease_until,
    DROP COLUMN IF EXISTS send_attempt_state,
    DROP COLUMN IF EXISTS send_fence_token,
    DROP COLUMN IF EXISTS send_attempt_id;
