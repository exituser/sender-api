-- Inbound delivery is only routable after the domain points MX at SES.
ALTER TABLE domains
    ADD COLUMN IF NOT EXISTS mx_status VARCHAR(50) NOT NULL DEFAULT 'pending';

ALTER TABLE domains
    ADD COLUMN IF NOT EXISTS mx_dns_record TEXT;
