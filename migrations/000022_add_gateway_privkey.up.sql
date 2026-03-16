-- Add wireguard_privkey column to gateways table.
-- This allows core to send the private key to the gateway via gRPC GetConfig,
-- so the gateway can automatically configure the WireGuard interface.
ALTER TABLE gateways ADD COLUMN IF NOT EXISTS wireguard_privkey TEXT NOT NULL DEFAULT '';

-- Set the dev gateway private key (matches pubkey from seed migration 000020).
UPDATE gateways
SET wireguard_privkey = 'gL2wor2tp29I8OLrRWCOz9uDkmhuvehu69pI2DILYUA='
WHERE wireguard_pubkey = '8LwwVQeBoIpq9R6wMIxYhvpesJmZD7UIRdtyouzo+hg=';
