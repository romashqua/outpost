ALTER TABLE s2s_tunnel_members DROP COLUMN IF EXISTS private_key;
ALTER TABLE s2s_tunnel_members DROP COLUMN IF EXISTS public_key;
ALTER TABLE s2s_tunnel_members DROP COLUMN IF EXISTS listen_port;
