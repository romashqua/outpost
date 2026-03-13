-- Tenants
CREATE TABLE tenants (
    id           UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    name         TEXT NOT NULL,
    slug         TEXT UNIQUE NOT NULL,
    plan         TEXT NOT NULL DEFAULT 'free',
    max_users    INTEGER NOT NULL DEFAULT 50,
    max_devices  INTEGER NOT NULL DEFAULT 100,
    max_networks INTEGER NOT NULL DEFAULT 5,
    is_active    BOOLEAN NOT NULL DEFAULT true,
    created_at   TIMESTAMPTZ NOT NULL DEFAULT now(),
    updated_at   TIMESTAMPTZ NOT NULL DEFAULT now()
);

CREATE INDEX idx_tenants_slug ON tenants(slug);

-- Add tenant_id to existing tables (nullable for backward compatibility).
ALTER TABLE users ADD COLUMN tenant_id UUID REFERENCES tenants(id) ON DELETE SET NULL;
ALTER TABLE networks ADD COLUMN tenant_id UUID REFERENCES tenants(id) ON DELETE SET NULL;
ALTER TABLE gateways ADD COLUMN tenant_id UUID REFERENCES tenants(id) ON DELETE SET NULL;

CREATE INDEX idx_users_tenant_id ON users(tenant_id);
CREATE INDEX idx_networks_tenant_id ON networks(tenant_id);
CREATE INDEX idx_gateways_tenant_id ON gateways(tenant_id);

-- Device posture
CREATE TABLE device_posture (
    id              UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    device_id       UUID NOT NULL REFERENCES devices(id) ON DELETE CASCADE,
    os_type         TEXT NOT NULL,
    os_version      TEXT NOT NULL,
    disk_encrypted  BOOLEAN NOT NULL DEFAULT false,
    screen_lock     BOOLEAN NOT NULL DEFAULT false,
    antivirus       BOOLEAN NOT NULL DEFAULT false,
    firewall        BOOLEAN NOT NULL DEFAULT false,
    score           INTEGER NOT NULL DEFAULT 0,
    checked_at      TIMESTAMPTZ NOT NULL DEFAULT now()
);

CREATE INDEX idx_device_posture_device_id ON device_posture(device_id);
CREATE INDEX idx_device_posture_checked_at ON device_posture(checked_at DESC);

-- Posture policies
CREATE TABLE posture_policies (
    id                      UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    network_id              UUID NOT NULL REFERENCES networks(id) ON DELETE CASCADE,
    name                    TEXT NOT NULL,
    require_disk_encryption BOOLEAN NOT NULL DEFAULT false,
    require_screen_lock     BOOLEAN NOT NULL DEFAULT false,
    require_antivirus       BOOLEAN NOT NULL DEFAULT false,
    require_firewall        BOOLEAN NOT NULL DEFAULT false,
    min_os_versions         JSONB NOT NULL DEFAULT '{}',
    min_score               INTEGER NOT NULL DEFAULT 0
);

CREATE INDEX idx_posture_policies_network_id ON posture_policies(network_id);

-- Flow records (partitioned by recorded_at)
CREATE TABLE flow_records (
    gateway_id  UUID NOT NULL,
    device_id   UUID NOT NULL,
    user_id     UUID NOT NULL,
    src_ip      INET NOT NULL,
    dst_ip      INET NOT NULL,
    protocol    TEXT NOT NULL,
    dst_port    INTEGER NOT NULL,
    bytes_sent  BIGINT NOT NULL DEFAULT 0,
    bytes_recv  BIGINT NOT NULL DEFAULT 0,
    duration_ms BIGINT NOT NULL DEFAULT 0,
    recorded_at TIMESTAMPTZ NOT NULL DEFAULT now()
) PARTITION BY RANGE (recorded_at);

CREATE TABLE flow_records_default PARTITION OF flow_records DEFAULT;

CREATE INDEX idx_flow_records_recorded_at ON flow_records(recorded_at DESC);
CREATE INDEX idx_flow_records_user_id ON flow_records(user_id);
CREATE INDEX idx_flow_records_device_id ON flow_records(device_id);
CREATE INDEX idx_flow_records_gateway_id ON flow_records(gateway_id);

-- Key pairs
CREATE TABLE key_pairs (
    id          UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    device_id   UUID REFERENCES devices(id) ON DELETE CASCADE,
    gateway_id  UUID REFERENCES gateways(id) ON DELETE CASCADE,
    public_key  TEXT NOT NULL,
    created_at  TIMESTAMPTZ NOT NULL DEFAULT now(),
    expires_at  TIMESTAMPTZ NOT NULL,
    is_active   BOOLEAN NOT NULL DEFAULT true,
    rotation_id INTEGER NOT NULL DEFAULT 1
);

CREATE INDEX idx_key_pairs_device_id ON key_pairs(device_id);
CREATE INDEX idx_key_pairs_gateway_id ON key_pairs(gateway_id);
CREATE INDEX idx_key_pairs_expires_at ON key_pairs(expires_at) WHERE is_active = true;
