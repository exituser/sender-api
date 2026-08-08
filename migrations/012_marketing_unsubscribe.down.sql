DROP INDEX IF EXISTS idx_emails_category;
ALTER TABLE suppressions DROP CONSTRAINT IF EXISTS suppressions_reason_check;
ALTER TABLE suppressions
    ADD CONSTRAINT suppressions_reason_check
    CHECK (reason IN ('bounce', 'complaint'));
ALTER TABLE emails DROP COLUMN IF EXISTS category;
