-- NAT traversal: relay servers and device NAT status tracking.

-- STUN/TURN relay servers.
CREATE TABLE relay_servers (
    id          UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    name        TEXT NOT NULL,
    address     TEXT NOT NULL,
    region      TEXT NOT NULL DEFAULT '',
    protocol    TEXT NOT NULL CHECK (protocol IN ('stun', 'turn')),
    is_active   BOOLEAN NOT NULL DEFAULT true,
    created_at  TIMESTAMPTZ NOT NULL DEFAULT now(),
    updated_at  TIMESTAMPTZ NOT NULL DEFAULT now()
);

CREATE INDEX idx_relay_servers_active ON relay_servers (is_active) WHERE is_active = true;

-- Per-device NAT status (latest detection result).
CREATE TABLE nat_status (
    id              UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    device_id       UUID NOT NULL REFERENCES devices(id) ON DELETE CASCADE,
    nat_type        TEXT NOT NULL CHECK (nat_type IN ('full_cone', 'restricted_cone', 'port_restricted', 'symmetric', 'open', 'unknown')),
    external_ip     TEXT NOT NULL DEFAULT '',
    external_port   INTEGER NOT NULL DEFAULT 0,
    relay_server_id UUID REFERENCES relay_servers(id) ON DELETE SET NULL,
    last_checked    TIMESTAMPTZ NOT NULL DEFAULT now(),
    created_at      TIMESTAMPTZ NOT NULL DEFAULT now(),
    UNIQUE(device_id)
);

CREATE INDEX idx_nat_status_device ON nat_status (device_id);
CREATE INDEX idx_nat_status_type ON nat_status (nat_type);
