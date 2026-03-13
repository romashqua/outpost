-- Outpost VPN initial schema

-- Extensions
CREATE EXTENSION IF NOT EXISTS "pgcrypto";

-- Users
CREATE TABLE users (
    id            UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    username      TEXT UNIQUE NOT NULL,
    email         TEXT UNIQUE NOT NULL,
    password_hash TEXT,
    first_name    TEXT NOT NULL DEFAULT '',
    last_name     TEXT NOT NULL DEFAULT '',
    phone         TEXT,
    is_active     BOOLEAN NOT NULL DEFAULT true,
    is_admin      BOOLEAN NOT NULL DEFAULT false,
    ldap_dn       TEXT,
    scim_external_id TEXT,
    mfa_enabled   BOOLEAN NOT NULL DEFAULT false,
    enrolled_at   TIMESTAMPTZ,
    created_at    TIMESTAMPTZ NOT NULL DEFAULT now(),
    updated_at    TIMESTAMPTZ NOT NULL DEFAULT now()
);

-- Groups
CREATE TABLE groups (
    id          UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    name        TEXT UNIQUE NOT NULL,
    description TEXT NOT NULL DEFAULT '',
    is_system   BOOLEAN NOT NULL DEFAULT false,
    created_at  TIMESTAMPTZ NOT NULL DEFAULT now()
);

CREATE TABLE user_groups (
    user_id  UUID NOT NULL REFERENCES users(id) ON DELETE CASCADE,
    group_id UUID NOT NULL REFERENCES groups(id) ON DELETE CASCADE,
    PRIMARY KEY (user_id, group_id)
);

-- RBAC
CREATE TABLE roles (
    id          UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    name        TEXT UNIQUE NOT NULL,
    description TEXT NOT NULL DEFAULT '',
    permissions JSONB NOT NULL DEFAULT '[]',
    is_system   BOOLEAN NOT NULL DEFAULT false,
    created_at  TIMESTAMPTZ NOT NULL DEFAULT now()
);

CREATE TABLE user_roles (
    user_id UUID NOT NULL REFERENCES users(id) ON DELETE CASCADE,
    role_id UUID NOT NULL REFERENCES roles(id) ON DELETE CASCADE,
    PRIMARY KEY (user_id, role_id)
);

-- MFA credentials
CREATE TABLE mfa_totp (
    id         UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    user_id    UUID NOT NULL REFERENCES users(id) ON DELETE CASCADE,
    secret     BYTEA NOT NULL,
    verified   BOOLEAN NOT NULL DEFAULT false,
    created_at TIMESTAMPTZ NOT NULL DEFAULT now()
);

CREATE TABLE mfa_webauthn (
    id            UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    user_id       UUID NOT NULL REFERENCES users(id) ON DELETE CASCADE,
    credential_id BYTEA NOT NULL,
    public_key    BYTEA NOT NULL,
    sign_count    BIGINT NOT NULL DEFAULT 0,
    name          TEXT NOT NULL,
    created_at    TIMESTAMPTZ NOT NULL DEFAULT now()
);

CREATE TABLE mfa_backup_codes (
    id        UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    user_id   UUID NOT NULL REFERENCES users(id) ON DELETE CASCADE,
    code_hash TEXT NOT NULL,
    used      BOOLEAN NOT NULL DEFAULT false
);

-- Devices (WireGuard peers)
CREATE TABLE devices (
    id               UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    user_id          UUID NOT NULL REFERENCES users(id) ON DELETE CASCADE,
    name             TEXT NOT NULL,
    wireguard_pubkey TEXT UNIQUE NOT NULL,
    assigned_ip      INET NOT NULL,
    is_approved      BOOLEAN NOT NULL DEFAULT false,
    last_handshake   TIMESTAMPTZ,
    created_at       TIMESTAMPTZ NOT NULL DEFAULT now(),
    updated_at       TIMESTAMPTZ NOT NULL DEFAULT now()
);

-- Networks
CREATE TABLE networks (
    id         UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    name       TEXT UNIQUE NOT NULL,
    address    CIDR NOT NULL,
    dns        TEXT[] NOT NULL DEFAULT '{}',
    port       INTEGER NOT NULL DEFAULT 51820,
    keepalive  INTEGER NOT NULL DEFAULT 25,
    is_active  BOOLEAN NOT NULL DEFAULT true,
    created_at TIMESTAMPTZ NOT NULL DEFAULT now(),
    updated_at TIMESTAMPTZ NOT NULL DEFAULT now()
);

-- Gateways
CREATE TABLE gateways (
    id               UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    network_id       UUID NOT NULL REFERENCES networks(id) ON DELETE CASCADE,
    name             TEXT NOT NULL,
    public_ip        INET,
    wireguard_pubkey TEXT UNIQUE NOT NULL,
    endpoint         TEXT NOT NULL,
    is_active        BOOLEAN NOT NULL DEFAULT true,
    priority         INTEGER NOT NULL DEFAULT 0,
    token_hash       TEXT NOT NULL,
    last_seen        TIMESTAMPTZ,
    created_at       TIMESTAMPTZ NOT NULL DEFAULT now(),
    updated_at       TIMESTAMPTZ NOT NULL DEFAULT now()
);

-- Network ACLs
CREATE TABLE network_acls (
    id          UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    network_id  UUID NOT NULL REFERENCES networks(id) ON DELETE CASCADE,
    group_id    UUID NOT NULL REFERENCES groups(id) ON DELETE CASCADE,
    allowed_ips CIDR[] NOT NULL DEFAULT '{0.0.0.0/0}',
    UNIQUE(network_id, group_id)
);

-- Site-to-site tunnels
CREATE TABLE s2s_tunnels (
    id             UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    name           TEXT NOT NULL,
    topology       TEXT NOT NULL CHECK (topology IN ('mesh', 'hub_spoke')),
    hub_gateway_id UUID REFERENCES gateways(id),
    is_active      BOOLEAN NOT NULL DEFAULT true,
    created_at     TIMESTAMPTZ NOT NULL DEFAULT now(),
    updated_at     TIMESTAMPTZ NOT NULL DEFAULT now()
);

CREATE TABLE s2s_tunnel_members (
    tunnel_id     UUID NOT NULL REFERENCES s2s_tunnels(id) ON DELETE CASCADE,
    gateway_id    UUID NOT NULL REFERENCES gateways(id) ON DELETE CASCADE,
    local_subnets CIDR[] NOT NULL,
    PRIMARY KEY (tunnel_id, gateway_id)
);

CREATE TABLE s2s_routes (
    id          UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    tunnel_id   UUID NOT NULL REFERENCES s2s_tunnels(id) ON DELETE CASCADE,
    destination CIDR NOT NULL,
    via_gateway UUID NOT NULL REFERENCES gateways(id),
    metric      INTEGER NOT NULL DEFAULT 100,
    is_active   BOOLEAN NOT NULL DEFAULT true,
    created_at  TIMESTAMPTZ NOT NULL DEFAULT now()
);

-- OIDC provider
CREATE TABLE oidc_clients (
    id            TEXT PRIMARY KEY,
    secret_hash   TEXT NOT NULL,
    name          TEXT NOT NULL,
    redirect_uris TEXT[] NOT NULL,
    scopes        TEXT[] NOT NULL DEFAULT '{openid,profile,email}',
    created_at    TIMESTAMPTZ NOT NULL DEFAULT now()
);

CREATE TABLE oidc_auth_codes (
    code         TEXT PRIMARY KEY,
    client_id    TEXT NOT NULL REFERENCES oidc_clients(id) ON DELETE CASCADE,
    user_id      UUID NOT NULL REFERENCES users(id) ON DELETE CASCADE,
    scopes       TEXT[] NOT NULL,
    nonce        TEXT,
    redirect_uri TEXT NOT NULL,
    code_challenge TEXT,
    code_challenge_method TEXT,
    expires_at   TIMESTAMPTZ NOT NULL,
    created_at   TIMESTAMPTZ NOT NULL DEFAULT now()
);

-- Sessions
CREATE TABLE sessions (
    id         TEXT PRIMARY KEY,
    user_id    UUID NOT NULL REFERENCES users(id) ON DELETE CASCADE,
    ip_address INET,
    user_agent TEXT,
    expires_at TIMESTAMPTZ NOT NULL,
    created_at TIMESTAMPTZ NOT NULL DEFAULT now()
);

CREATE INDEX idx_sessions_user_id ON sessions(user_id);
CREATE INDEX idx_sessions_expires_at ON sessions(expires_at);

-- Enrollment tokens
CREATE TABLE enrollment_tokens (
    id         UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    user_id    UUID NOT NULL REFERENCES users(id) ON DELETE CASCADE,
    token_hash TEXT UNIQUE NOT NULL,
    expires_at TIMESTAMPTZ NOT NULL,
    used       BOOLEAN NOT NULL DEFAULT false,
    created_at TIMESTAMPTZ NOT NULL DEFAULT now()
);

-- Audit log
CREATE TABLE audit_log (
    id         BIGSERIAL PRIMARY KEY,
    timestamp  TIMESTAMPTZ NOT NULL DEFAULT now(),
    user_id    UUID REFERENCES users(id) ON DELETE SET NULL,
    action     TEXT NOT NULL,
    resource   TEXT NOT NULL,
    details    JSONB,
    ip_address INET,
    user_agent TEXT
);

CREATE INDEX idx_audit_log_timestamp ON audit_log(timestamp DESC);
CREATE INDEX idx_audit_log_user_id ON audit_log(user_id);
CREATE INDEX idx_audit_log_action ON audit_log(action);

-- Peer stats (partitioned)
CREATE TABLE peer_stats (
    gateway_id     UUID NOT NULL,
    device_id      UUID NOT NULL,
    rx_bytes       BIGINT NOT NULL,
    tx_bytes       BIGINT NOT NULL,
    last_handshake TIMESTAMPTZ,
    endpoint       TEXT,
    recorded_at    TIMESTAMPTZ NOT NULL DEFAULT now()
) PARTITION BY RANGE (recorded_at);

-- Create partition for current month
CREATE TABLE peer_stats_default PARTITION OF peer_stats DEFAULT;

-- Settings
CREATE TABLE settings (
    key        TEXT PRIMARY KEY,
    value      JSONB NOT NULL,
    updated_at TIMESTAMPTZ NOT NULL DEFAULT now()
);

-- Seed default admin role
INSERT INTO roles (name, description, permissions, is_system) VALUES
    ('admin', 'Full system administrator', '["*"]', true),
    ('user', 'Regular user with self-service access', '["self:read","self:write","device:read","device:write"]', true),
    ('viewer', 'Read-only access to all resources', '["*:read"]', true);

-- Seed default groups
INSERT INTO groups (name, description, is_system) VALUES
    ('everyone', 'All users', true),
    ('admins', 'System administrators', true);

-- Seed default settings
INSERT INTO settings (key, value) VALUES
    ('general.instance_name', '"Outpost VPN"'),
    ('general.locale', '"ru"'),
    ('wireguard.default_keepalive', '25'),
    ('wireguard.default_dns', '["1.1.1.1","8.8.8.8"]'),
    ('enrollment.enabled', 'true'),
    ('mfa.required', 'false'),
    ('mfa.methods', '["totp","webauthn","email"]');
