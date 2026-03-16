-- Gateway can serve multiple networks (junction table).
CREATE TABLE gateway_networks (
    gateway_id UUID NOT NULL REFERENCES gateways(id) ON DELETE CASCADE,
    network_id UUID NOT NULL REFERENCES networks(id) ON DELETE CASCADE,
    created_at TIMESTAMPTZ NOT NULL DEFAULT now(),
    PRIMARY KEY (gateway_id, network_id)
);

CREATE INDEX idx_gateway_networks_network ON gateway_networks(network_id);

-- Migrate existing data: copy gateways.network_id into junction table.
INSERT INTO gateway_networks (gateway_id, network_id)
SELECT id, network_id FROM gateways
WHERE network_id IS NOT NULL
ON CONFLICT DO NOTHING;

-- Make network_id nullable (kept for backward compatibility, junction table is authoritative).
ALTER TABLE gateways ALTER COLUMN network_id DROP NOT NULL;
