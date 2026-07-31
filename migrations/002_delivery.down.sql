ALTER TABLE domains DROP COLUMN IF EXISTS verification_dns_record;
ALTER TABLE domains DROP COLUMN IF EXISTS verification_status;
ALTER TABLE emails DROP COLUMN IF EXISTS attachments;
ALTER TABLE emails DROP COLUMN IF EXISTS reply_to;
