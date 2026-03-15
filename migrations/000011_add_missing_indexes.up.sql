-- Add missing indexes on foreign key columns for query performance.

CREATE INDEX IF NOT EXISTS idx_devices_user_id ON devices(user_id);
CREATE INDEX IF NOT EXISTS idx_gateways_network_id ON gateways(network_id);
CREATE INDEX IF NOT EXISTS idx_s2s_tunnel_members_tunnel ON s2s_tunnel_members(tunnel_id);
CREATE INDEX IF NOT EXISTS idx_s2s_routes_tunnel ON s2s_routes(tunnel_id);
CREATE INDEX IF NOT EXISTS idx_peer_stats_recorded_at ON peer_stats(recorded_at DESC);

-- Fix FK cascade rules: allow gateway deletion to clean up S2S references.
ALTER TABLE s2s_routes DROP CONSTRAINT IF EXISTS s2s_routes_via_gateway_fkey;
ALTER TABLE s2s_routes ADD CONSTRAINT s2s_routes_via_gateway_fkey
    FOREIGN KEY (via_gateway) REFERENCES gateways(id) ON DELETE CASCADE;

ALTER TABLE s2s_tunnels DROP CONSTRAINT IF EXISTS s2s_tunnels_hub_gateway_id_fkey;
ALTER TABLE s2s_tunnels ADD CONSTRAINT s2s_tunnels_hub_gateway_id_fkey
    FOREIGN KEY (hub_gateway_id) REFERENCES gateways(id) ON DELETE SET NULL;

-- Fix proxy_id FK: SET NULL when proxy is deleted instead of blocking.
ALTER TABLE smart_route_entries DROP CONSTRAINT IF EXISTS smart_route_entries_proxy_id_fkey;
ALTER TABLE smart_route_entries ADD CONSTRAINT smart_route_entries_proxy_id_fkey
    FOREIGN KEY (proxy_id) REFERENCES proxy_servers(id) ON DELETE SET NULL;
