-- Add missing UNIQUE constraints expected by handler code.

ALTER TABLE s2s_tunnels ADD CONSTRAINT s2s_tunnels_name_key UNIQUE (name);
ALTER TABLE gateways ADD CONSTRAINT gateways_network_name_key UNIQUE (network_id, name);
ALTER TABLE gateways ADD CONSTRAINT gateways_network_endpoint_key UNIQUE (network_id, endpoint);
