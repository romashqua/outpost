-- Smart routing rules for selective proxy/bypass

-- Upstream proxy servers for routing (must be created before smart_route_entries).
CREATE TABLE proxy_servers (
    id          UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    name        TEXT NOT NULL,
    type        TEXT NOT NULL CHECK (type IN ('socks5', 'http', 'shadowsocks', 'vless')),
    address     TEXT NOT NULL,
    port        INTEGER NOT NULL,
    username    TEXT,
    password    TEXT,
    extra_config JSONB,
    is_active   BOOLEAN NOT NULL DEFAULT true,
    created_at  TIMESTAMPTZ NOT NULL DEFAULT now(),
    updated_at  TIMESTAMPTZ NOT NULL DEFAULT now()
);

-- Smart route groups.
CREATE TABLE smart_routes (
    id          UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    name        TEXT NOT NULL,
    description TEXT,
    is_active   BOOLEAN NOT NULL DEFAULT true,
    created_at  TIMESTAMPTZ NOT NULL DEFAULT now(),
    updated_at  TIMESTAMPTZ NOT NULL DEFAULT now()
);

-- Individual routing entries within a smart route group.
CREATE TABLE smart_route_entries (
    id             UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    smart_route_id UUID NOT NULL REFERENCES smart_routes(id) ON DELETE CASCADE,
    entry_type     TEXT NOT NULL CHECK (entry_type IN ('domain', 'cidr', 'domain_suffix')),
    value          TEXT NOT NULL,
    action         TEXT NOT NULL CHECK (action IN ('proxy', 'direct', 'block')),
    proxy_id       UUID REFERENCES proxy_servers(id),
    priority       INTEGER NOT NULL DEFAULT 100,
    created_at     TIMESTAMPTZ NOT NULL DEFAULT now(),
    UNIQUE(smart_route_id, entry_type, value)
);

-- Link smart routes to networks.
CREATE TABLE network_smart_routes (
    network_id     UUID NOT NULL REFERENCES networks(id) ON DELETE CASCADE,
    smart_route_id UUID NOT NULL REFERENCES smart_routes(id) ON DELETE CASCADE,
    PRIMARY KEY (network_id, smart_route_id)
);

CREATE INDEX idx_smart_route_entries_route ON smart_route_entries(smart_route_id);
CREATE INDEX idx_smart_route_entries_type ON smart_route_entries(entry_type, value);
