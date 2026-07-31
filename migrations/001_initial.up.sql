-- ============================================
-- TEAMS
-- ============================================
CREATE TABLE teams (
    id UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    name VARCHAR(255) NOT NULL,
    slug VARCHAR(255) UNIQUE NOT NULL,
    plan VARCHAR(50) DEFAULT 'free' CHECK (plan IN ('free', 'pro', 'scale')),
    stripe_customer_id VARCHAR(255),
    stripe_subscription_id VARCHAR(255),
    created_at TIMESTAMPTZ DEFAULT NOW(),
    updated_at TIMESTAMPTZ DEFAULT NOW()
);

-- ============================================
-- TEAM MEMBERS
-- ============================================
CREATE TABLE team_members (
    id UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    team_id UUID REFERENCES teams(id) ON DELETE CASCADE,
    user_id UUID NOT NULL,
    role VARCHAR(50) DEFAULT 'member' CHECK (role IN ('owner', 'admin', 'member')),
    created_at TIMESTAMPTZ DEFAULT NOW(),
    UNIQUE(team_id, user_id)
);

-- ============================================
-- API KEYS
-- ============================================
CREATE TABLE api_keys (
    id UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    team_id UUID REFERENCES teams(id) ON DELETE CASCADE,
    name VARCHAR(255) NOT NULL,
    key_hash VARCHAR(255) NOT NULL,
    key_prefix VARCHAR(10) NOT NULL,
    permissions JSONB DEFAULT '["send"]',
    created_at TIMESTAMPTZ DEFAULT NOW(),
    last_used_at TIMESTAMPTZ
);

-- ============================================
-- DOMAINS
-- ============================================
CREATE TABLE domains (
    id UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    team_id UUID REFERENCES teams(id) ON DELETE CASCADE,
    name VARCHAR(255) NOT NULL,
    status VARCHAR(50) DEFAULT 'pending' CHECK (status IN ('pending', 'verified', 'failed')),
    verification_token VARCHAR(255),
    verification_status VARCHAR(50) DEFAULT 'pending',
    spf_status VARCHAR(50) DEFAULT 'pending',
    dkim_status VARCHAR(50) DEFAULT 'pending',
    dmarc_status VARCHAR(50) DEFAULT 'pending',
    dkim_dns_record TEXT,
    spf_dns_record TEXT,
    dmarc_dns_record TEXT,
    verification_dns_record TEXT,
    created_at TIMESTAMPTZ DEFAULT NOW(),
    UNIQUE(team_id, name)
);

-- ============================================
-- EMAILS
-- ============================================
CREATE TABLE emails (
    id UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    team_id UUID REFERENCES teams(id) ON DELETE CASCADE,
    api_key_id UUID REFERENCES api_keys(id),
    from_addr VARCHAR(255) NOT NULL,
    to_addr JSONB NOT NULL,
    cc JSONB,
    bcc JSONB,
    subject TEXT NOT NULL,
    html TEXT,
    text TEXT,
    reply_to JSONB DEFAULT '[]',
    attachments JSONB DEFAULT '[]',
    status VARCHAR(50) DEFAULT 'queued' CHECK (status IN (
        'queued', 'sending', 'sent', 'delivered', 'opened',
        'clicked', 'bounced', 'complained', 'failed', 'cancelled'
    )),
    tags JSONB DEFAULT '[]',
    metadata JSONB DEFAULT '{}',
    headers JSONB DEFAULT '{}',
    scheduled_at TIMESTAMPTZ,
    sent_at TIMESTAMPTZ,
    created_at TIMESTAMPTZ DEFAULT NOW()
);

CREATE INDEX idx_emails_team_id ON emails(team_id);
CREATE INDEX idx_emails_status ON emails(status);
CREATE INDEX idx_emails_created_at ON emails(created_at DESC);

-- ============================================
-- EMAIL EVENTS
-- ============================================
CREATE TABLE email_events (
    id UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    email_id UUID REFERENCES emails(id) ON DELETE CASCADE,
    event VARCHAR(50) NOT NULL,
    timestamp TIMESTAMPTZ DEFAULT NOW(),
    data JSONB DEFAULT '{}',
    ip_address INET,
    user_agent TEXT
);

CREATE INDEX idx_email_events_email_id ON email_events(email_id);
CREATE INDEX idx_email_events_event ON email_events(event);

-- ============================================
-- CONTACTS
-- ============================================
CREATE TABLE contacts (
    id UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    team_id UUID REFERENCES teams(id) ON DELETE CASCADE,
    email VARCHAR(255) NOT NULL,
    first_name VARCHAR(255),
    last_name VARCHAR(255),
    subscribed BOOLEAN DEFAULT true,
    properties JSONB DEFAULT '{}',
    created_at TIMESTAMPTZ DEFAULT NOW(),
    updated_at TIMESTAMPTZ DEFAULT NOW(),
    UNIQUE(team_id, email)
);

CREATE INDEX idx_contacts_team_id ON contacts(team_id);

-- ============================================
-- INBOUND EMAILS
-- ============================================
CREATE TABLE inbound_emails (
    id UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    team_id UUID REFERENCES teams(id) ON DELETE CASCADE,
    message_id VARCHAR(255),
    from_addr VARCHAR(255) NOT NULL,
    to_addr JSONB NOT NULL,
    subject TEXT,
    text TEXT,
    html TEXT,
    attachments JSONB DEFAULT '[]',
    raw_s3_key VARCHAR(1024),
    headers JSONB DEFAULT '{}',
    created_at TIMESTAMPTZ DEFAULT NOW()
);

CREATE INDEX idx_inbound_emails_team_id ON inbound_emails(team_id);

-- ============================================
-- WEBHOOKS
-- ============================================
CREATE TABLE webhooks (
    id UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    team_id UUID REFERENCES teams(id) ON DELETE CASCADE,
    url TEXT NOT NULL,
    events TEXT[] NOT NULL,
    secret VARCHAR(255) NOT NULL,
    active BOOLEAN DEFAULT true,
    created_at TIMESTAMPTZ DEFAULT NOW()
);

-- RLS disabled: auth is handled by Go API via JWT middleware, not Supabase RLS.
