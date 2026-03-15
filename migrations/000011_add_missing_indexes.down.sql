-- Revert FK changes.
ALTER TABLE smart_route_entries DROP CONSTRAINT IF EXISTS smart_route_entries_proxy_id_fkey;
ALTER TABLE smart_route_entries ADD CONSTRAINT smart_route_entries_proxy_id_fkey
    FOREIGN KEY (proxy_id) REFERENCES proxy_servers(id);

ALTER TABLE s2s_tunnels DROP CONSTRAINT IF EXISTS s2s_tunnels_hub_gateway_id_fkey;
ALTER TABLE s2s_tunnels ADD CONSTRAINT s2s_tunnels_hub_gateway_id_fkey
    FOREIGN KEY (hub_gateway_id) REFERENCES gateways(id);

ALTER TABLE s2s_routes DROP CONSTRAINT IF EXISTS s2s_routes_via_gateway_fkey;
ALTER TABLE s2s_routes ADD CONSTRAINT s2s_routes_via_gateway_fkey
    FOREIGN KEY (via_gateway) REFERENCES gateways(id);

-- Drop indexes.
DROP INDEX IF EXISTS idx_peer_stats_recorded_at;
DROP INDEX IF EXISTS idx_s2s_routes_tunnel;
DROP INDEX IF EXISTS idx_s2s_tunnel_members_tunnel;
DROP INDEX IF EXISTS idx_gateways_network_id;
DROP INDEX IF EXISTS idx_devices_user_id;
