-- Provision the runtime login after migrations create the full schema.
-- Password is supplied at runtime with psql -v; no secret belongs in Git.
SELECT format($command$
DO $guard$
BEGIN
    IF %1$L = current_user OR %1$L = ANY (ARRAY[
        'postgres', 'supabase_admin', 'supabase_auth_admin',
        'supabase_storage_admin', 'authenticator', 'service_role',
        'anon', 'authenticated', 'dashboard_user', 'pgbouncer'
    ]) THEN
        RAISE EXCEPTION 'APP_DB_USER must be a dedicated non-administrative role';
    END IF;
    IF EXISTS (
        SELECT 1
        FROM pg_roles role
        WHERE role.rolname = %1$L
          AND (
            role.rolsuper
            OR role.rolcreatedb
            OR role.rolcreaterole
            OR role.rolreplication
            OR role.rolbypassrls
            OR
            EXISTS (SELECT 1 FROM pg_database db WHERE db.datdba = role.oid)
            OR EXISTS (SELECT 1 FROM pg_namespace namespace WHERE namespace.nspowner = role.oid)
            OR EXISTS (SELECT 1 FROM pg_class relation WHERE relation.relowner = role.oid)
          )
    ) THEN
        RAISE EXCEPTION 'APP_DB_USER is privileged or owns database objects; use a dedicated unprivileged runtime role';
    END IF;
END
$guard$;
$command$, :'app_role')
\gexec

SELECT format('CREATE ROLE %I LOGIN PASSWORD %L', :'app_role', :'app_password')
WHERE NOT EXISTS (SELECT 1 FROM pg_roles WHERE rolname = :'app_role')
\gexec

SELECT format('REVOKE %I FROM %I', parent.rolname, member.rolname)
FROM pg_auth_members membership
JOIN pg_roles parent ON parent.oid = membership.roleid
JOIN pg_roles member ON member.oid = membership.member
WHERE member.rolname = :'app_role'
\gexec

SELECT format(
    'ALTER ROLE %I WITH LOGIN NOSUPERUSER NOCREATEDB NOCREATEROLE NOINHERIT NOREPLICATION NOBYPASSRLS PASSWORD %L',
    :'app_role', :'app_password'
)
\gexec

SELECT format('REVOKE ALL ON DATABASE %I FROM PUBLIC', current_database())
\gexec
SELECT format('REVOKE ALL ON DATABASE %I FROM %I', current_database(), :'app_role')
\gexec
SELECT format('GRANT CONNECT ON DATABASE %I TO %I', current_database(), :'app_role')
\gexec
REVOKE CREATE ON SCHEMA public FROM PUBLIC;
REVOKE ALL ON SCHEMA public FROM :"app_role";
GRANT USAGE ON SCHEMA public TO :"app_role";
REVOKE ALL ON ALL TABLES IN SCHEMA public FROM :"app_role";
GRANT SELECT, INSERT, UPDATE, DELETE ON ALL TABLES IN SCHEMA public TO :"app_role";
REVOKE ALL ON ALL SEQUENCES IN SCHEMA public FROM :"app_role";
GRANT USAGE, SELECT, UPDATE ON ALL SEQUENCES IN SCHEMA public TO :"app_role";
ALTER DEFAULT PRIVILEGES IN SCHEMA public
    REVOKE ALL ON TABLES FROM :"app_role";
ALTER DEFAULT PRIVILEGES IN SCHEMA public
    GRANT SELECT, INSERT, UPDATE, DELETE ON TABLES TO :"app_role";
ALTER DEFAULT PRIVILEGES IN SCHEMA public
    REVOKE ALL ON SEQUENCES FROM :"app_role";
ALTER DEFAULT PRIVILEGES IN SCHEMA public
    GRANT USAGE, SELECT, UPDATE ON SEQUENCES TO :"app_role";
