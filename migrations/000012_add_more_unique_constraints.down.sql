ALTER TABLE gateways DROP CONSTRAINT IF EXISTS gateways_network_endpoint_key;
ALTER TABLE gateways DROP CONSTRAINT IF EXISTS gateways_network_name_key;
ALTER TABLE s2s_tunnels DROP CONSTRAINT IF EXISTS s2s_tunnels_name_key;
