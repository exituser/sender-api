-- Keep provider verification separate from the application's ownership TXT
-- check. A domain is sendable only after both checks pass.
ALTER TABLE domains
    ADD COLUMN IF NOT EXISTS ses_verification_status VARCHAR(50) NOT NULL DEFAULT 'pending';

-- SES Easy DKIM returns three CNAME records. The legacy singular column is
-- retained for compatibility and stores the first host; this JSONB column is
-- the canonical complete set shown to operators.
ALTER TABLE domains
    ADD COLUMN IF NOT EXISTS dkim_dns_records JSONB NOT NULL DEFAULT '[]'::jsonb;

CREATE INDEX IF NOT EXISTS idx_domains_ses_verification_status
    ON domains (ses_verification_status);
