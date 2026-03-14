-- Device trust scores (computed from posture + MFA + freshness).
CREATE TABLE IF NOT EXISTS device_trust_scores (
    id          UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    device_id   UUID NOT NULL REFERENCES devices(id) ON DELETE CASCADE,
    score       INT NOT NULL DEFAULT 0,
    level       TEXT NOT NULL DEFAULT 'critical',
    violations  TEXT[] NOT NULL DEFAULT '{}',
    evaluated_at TIMESTAMPTZ NOT NULL DEFAULT now()
);

CREATE INDEX idx_device_trust_scores_device ON device_trust_scores (device_id, evaluated_at DESC);

-- Trust score configuration (singleton — one row).
CREATE TABLE IF NOT EXISTS trust_score_config (
    id                         INT PRIMARY KEY DEFAULT 1 CHECK (id = 1),
    weight_disk_encryption     INT NOT NULL DEFAULT 25,
    weight_screen_lock         INT NOT NULL DEFAULT 10,
    weight_antivirus           INT NOT NULL DEFAULT 20,
    weight_firewall            INT NOT NULL DEFAULT 15,
    weight_os_version          INT NOT NULL DEFAULT 15,
    weight_mfa                 INT NOT NULL DEFAULT 15,
    threshold_high             INT NOT NULL DEFAULT 80,
    threshold_medium           INT NOT NULL DEFAULT 50,
    threshold_low              INT NOT NULL DEFAULT 20,
    auto_restrict_below_medium BOOLEAN NOT NULL DEFAULT false,
    auto_block_below_low       BOOLEAN NOT NULL DEFAULT false,
    updated_at                 TIMESTAMPTZ NOT NULL DEFAULT now()
);

-- Insert default config.
INSERT INTO trust_score_config (id) VALUES (1) ON CONFLICT DO NOTHING;

-- ZTNA policies for conditional access.
CREATE TABLE IF NOT EXISTS ztna_policies (
    id          UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    name        TEXT NOT NULL UNIQUE,
    description TEXT,
    is_active   BOOLEAN NOT NULL DEFAULT true,
    -- Conditions (JSONB for flexibility).
    conditions  JSONB NOT NULL DEFAULT '{}',
    -- Action: allow, restrict, deny.
    action      TEXT NOT NULL DEFAULT 'allow',
    -- Networks this policy applies to (empty = all).
    network_ids UUID[] NOT NULL DEFAULT '{}',
    priority    INT NOT NULL DEFAULT 100,
    created_at  TIMESTAMPTZ NOT NULL DEFAULT now(),
    updated_at  TIMESTAMPTZ NOT NULL DEFAULT now()
);

-- DNS rules for split DNS.
CREATE TABLE IF NOT EXISTS dns_rules (
    id          UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    network_id  UUID NOT NULL REFERENCES networks(id) ON DELETE CASCADE,
    domain      TEXT NOT NULL,
    dns_server  TEXT NOT NULL,
    is_active   BOOLEAN NOT NULL DEFAULT true,
    created_at  TIMESTAMPTZ NOT NULL DEFAULT now()
);

CREATE INDEX idx_dns_rules_network ON dns_rules (network_id);
