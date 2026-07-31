-- Compose initializes 001, 003, and 004 directly for a convenient local stack.
-- Keep golang-migrate's bookkeeping aligned with that exact schema version.
CREATE TABLE IF NOT EXISTS public.schema_migrations (
    version BIGINT NOT NULL PRIMARY KEY,
    dirty BOOLEAN NOT NULL
);

INSERT INTO public.schema_migrations (version, dirty)
VALUES (4, false)
ON CONFLICT (version) DO UPDATE SET dirty = EXCLUDED.dirty;
