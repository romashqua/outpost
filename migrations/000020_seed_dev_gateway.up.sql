-- Seed default dev gateway for docker-compose development environment.
-- Token: "outpost-dev-gateway-token" (set via GATEWAY_TOKEN env var in docker-compose.yml)
-- SHA-256 hash: b04d9ee79e90454a83948a3b0537c16916e5ad7adc313377a0865b77484eba5d
-- IMPORTANT: Replace the token in production!
INSERT INTO gateways (network_id, name, wireguard_pubkey, endpoint, token_hash, is_active)
SELECT
    n.id,
    'default-gateway',
    'tTCbiejJGtoHyiga5NwaqXsPMXgl1xR7ikij6Ekd+Hk=',
    'gateway:51820',
    'b04d9ee79e90454a83948a3b0537c16916e5ad7adc313377a0865b77484eba5d',
    true
FROM networks n
WHERE n.name = 'default'
ON CONFLICT (wireguard_pubkey) DO NOTHING;
