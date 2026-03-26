-- Add per-member WireGuard keys for S2S tunnel interfaces.
-- Each gateway needs its own private key for each S2S tunnel it participates in.
ALTER TABLE s2s_tunnel_members ADD COLUMN IF NOT EXISTS private_key TEXT;
ALTER TABLE s2s_tunnel_members ADD COLUMN IF NOT EXISTS public_key TEXT;
ALTER TABLE s2s_tunnel_members ADD COLUMN IF NOT EXISTS listen_port INTEGER NOT NULL DEFAULT 0;

COMMENT ON COLUMN s2s_tunnel_members.private_key IS 'WireGuard private key for this gateway in this S2S tunnel';
COMMENT ON COLUMN s2s_tunnel_members.public_key IS 'WireGuard public key derived from private_key';
COMMENT ON COLUMN s2s_tunnel_members.listen_port IS 'WireGuard listen port for this S2S interface (0 = auto)';
