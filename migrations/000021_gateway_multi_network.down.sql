-- Restore network_id from junction table (pick first network).
UPDATE gateways g
SET network_id = (
    SELECT network_id FROM gateway_networks gn
    WHERE gn.gateway_id = g.id
    LIMIT 1
)
WHERE g.network_id IS NULL;

ALTER TABLE gateways ALTER COLUMN network_id SET NOT NULL;

DROP TABLE IF EXISTS gateway_networks;
