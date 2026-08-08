-- Compose initializes the complete schema for a convenient local stack.
-- Keep migration bookkeeping aligned with the latest mounted migration.
CREATE TABLE IF NOT EXISTS public.schema_migrations (
    version BIGINT NOT NULL PRIMARY KEY,
    dirty BOOLEAN NOT NULL
);

INSERT INTO public.schema_migrations (version, dirty)
VALUES (12, false)
ON CONFLICT (version) DO UPDATE SET dirty = EXCLUDED.dirty;
