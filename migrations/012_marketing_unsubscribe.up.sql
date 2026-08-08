ALTER TABLE emails
    ADD COLUMN IF NOT EXISTS category VARCHAR(20) NOT NULL DEFAULT 'transactional';

ALTER TABLE suppressions
    DROP CONSTRAINT IF EXISTS suppressions_reason_check;

ALTER TABLE suppressions
    ADD CONSTRAINT suppressions_reason_check
    CHECK (reason IN ('bounce', 'complaint', 'unsubscribe'));

CREATE INDEX IF NOT EXISTS idx_emails_category
    ON emails (team_id, category);
