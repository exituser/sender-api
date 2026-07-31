-- Compose initializes 001 and 003 directly for a convenient local stack.
-- Keep golang-migrate's bookkeeping aligned with that exact schema version.
CREATE TABLE IF NOT EXISTS schema_migrations (
    version BIGINT NOT NULL PRIMARY KEY,
    dirty BOOLEAN NOT NULL
);

INSERT INTO schema_migrations (version, dirty)
VALUES (3, false)
ON CONFLICT (version) DO UPDATE SET dirty = EXCLUDED.dirty;
