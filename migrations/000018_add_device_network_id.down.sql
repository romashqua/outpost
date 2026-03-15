DROP INDEX IF EXISTS idx_devices_network_id;
ALTER TABLE devices DROP COLUMN IF EXISTS network_id;
