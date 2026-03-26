-- S2S tunnel peer health tracking.
CREATE TABLE s2s_tunnel_health (
    tunnel_id          UUID NOT NULL REFERENCES s2s_tunnels(id) ON DELETE CASCADE,
    gateway_id         UUID NOT NULL REFERENCES gateways(id) ON DELETE CASCADE,
    remote_gateway_id  UUID NOT NULL REFERENCES gateways(id) ON DELETE CASCADE,
    is_healthy         BOOLEAN NOT NULL DEFAULT true,
    latency_ms         INTEGER NOT NULL DEFAULT 0,
    consecutive_failures INTEGER NOT NULL DEFAULT 0,
    last_check_at      TIMESTAMPTZ NOT NULL DEFAULT now(),
    last_healthy_at    TIMESTAMPTZ,
    PRIMARY KEY (tunnel_id, gateway_id, remote_gateway_id)
);

CREATE INDEX idx_s2s_tunnel_health_tunnel ON s2s_tunnel_health(tunnel_id);
CREATE INDEX idx_s2s_tunnel_health_gateway ON s2s_tunnel_health(gateway_id);
