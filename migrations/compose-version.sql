-- Compose initializes 001, 003, 004, and 005 directly for a convenient local stack.
-- Keep golang-migrate's bookkeeping aligned with that exact schema version.
CREATE TABLE IF NOT EXISTS public.schema_migrations (
    version BIGINT NOT NULL PRIMARY KEY,
    dirty BOOLEAN NOT NULL
);

INSERT INTO public.schema_migrations (version, dirty)
VALUES (5, false)
ON CONFLICT (version) DO UPDATE SET dirty = EXCLUDED.dirty;
