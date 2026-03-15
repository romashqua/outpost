-- Store device private key for consistent config downloads.
-- Encrypted at rest in production via PG column-level encryption (pgcrypto) or application-level.
ALTER TABLE devices ADD COLUMN IF NOT EXISTS wireguard_privkey TEXT;
