DROP INDEX IF EXISTS idx_domains_ses_verification_status;
ALTER TABLE domains DROP COLUMN IF EXISTS dkim_dns_records;
ALTER TABLE domains DROP COLUMN IF EXISTS ses_verification_status;
