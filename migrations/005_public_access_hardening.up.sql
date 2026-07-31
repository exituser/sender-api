-- The Go API is the only public data access boundary. Do not expose the
-- application tables through Supabase PostgREST roles by accident.
DO $$
DECLARE
    role_name TEXT;
    table_name TEXT;
BEGIN
    FOREACH role_name IN ARRAY ARRAY['anon', 'authenticated'] LOOP
        IF EXISTS (SELECT 1 FROM pg_roles WHERE rolname = role_name) THEN
            FOREACH table_name IN ARRAY ARRAY[
                'teams', 'team_members', 'api_keys', 'domains', 'emails',
                'email_events', 'contacts', 'inbound_emails', 'webhooks',
                'webhook_deliveries'
            ] LOOP
                EXECUTE format('REVOKE ALL PRIVILEGES ON TABLE public.%I FROM %I', table_name, role_name);
            END LOOP;
        END IF;
    END LOOP;

    IF EXISTS (SELECT 1 FROM pg_roles WHERE rolname = 'supabase_admin') THEN
        REVOKE ALL PRIVILEGES ON ALL TABLES IN SCHEMA public FROM anon, authenticated;
        ALTER DEFAULT PRIVILEGES FOR ROLE supabase_admin IN SCHEMA public
            REVOKE ALL ON TABLES FROM anon, authenticated;
    END IF;
END
$$;
