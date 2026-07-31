ALTER TABLE teams
    ADD COLUMN IF NOT EXISTS billing_status VARCHAR(50) NOT NULL DEFAULT 'inactive',
    ADD COLUMN IF NOT EXISTS current_period_end TIMESTAMPTZ,
    ADD COLUMN IF NOT EXISTS cancel_at_period_end BOOLEAN NOT NULL DEFAULT false;

CREATE INDEX IF NOT EXISTS idx_teams_stripe_customer_id
    ON teams (stripe_customer_id)
    WHERE stripe_customer_id IS NOT NULL;

CREATE INDEX IF NOT EXISTS idx_teams_stripe_subscription_id
    ON teams (stripe_subscription_id)
    WHERE stripe_subscription_id IS NOT NULL;

-- These tables are created after the original public-access hardening
-- migration. Revoke access here as well so a later migration cannot
-- accidentally expose billing or suppression data through PostgREST.
DO $$
DECLARE
    role_name TEXT;
BEGIN
    FOREACH role_name IN ARRAY ARRAY['anon', 'authenticated'] LOOP
        IF EXISTS (SELECT 1 FROM pg_roles WHERE rolname = role_name) THEN
            EXECUTE format('REVOKE ALL PRIVILEGES ON TABLE public.suppressions FROM %I', role_name);
            EXECUTE format('REVOKE ALL PRIVILEGES ON TABLE public.team_invitations FROM %I', role_name);
        END IF;
    END LOOP;
END
$$;
