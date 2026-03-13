-- Add description column to s2s_tunnels
ALTER TABLE s2s_tunnels ADD COLUMN IF NOT EXISTS description TEXT NOT NULL DEFAULT '';
