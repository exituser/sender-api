CREATE TABLE IF NOT EXISTS email_batches (
    team_id UUID NOT NULL REFERENCES teams(id) ON DELETE CASCADE,
    idempotency_key VARCHAR(255) NOT NULL,
    request_hash CHAR(64) NOT NULL,
    created_at TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    PRIMARY KEY (team_id, idempotency_key)
);

CREATE INDEX IF NOT EXISTS idx_email_batches_created_at
    ON email_batches (created_at);

DO $$
DECLARE
    role_name TEXT;
BEGIN
    FOREACH role_name IN ARRAY ARRAY['anon', 'authenticated'] LOOP
        IF EXISTS (SELECT 1 FROM pg_roles WHERE rolname = role_name) THEN
            EXECUTE format('REVOKE ALL PRIVILEGES ON TABLE public.email_batches FROM %I', role_name);
        END IF;
    END LOOP;
END
$$;
