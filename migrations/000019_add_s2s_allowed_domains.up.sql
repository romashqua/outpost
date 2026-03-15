CREATE TABLE IF NOT EXISTS s2s_allowed_domains (
    id UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    tunnel_id UUID NOT NULL REFERENCES s2s_tunnels(id) ON DELETE CASCADE,
    domain TEXT NOT NULL,
    created_at TIMESTAMPTZ NOT NULL DEFAULT now(),
    UNIQUE(tunnel_id, domain)
);
CREATE INDEX IF NOT EXISTS idx_s2s_allowed_domains_tunnel ON s2s_allowed_domains(tunnel_id);
