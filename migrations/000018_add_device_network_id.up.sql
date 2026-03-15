-- Add network_id to devices so each device is associated with a specific network.
-- Backfill existing devices with the first active network.

ALTER TABLE devices ADD COLUMN network_id UUID REFERENCES networks(id) ON DELETE SET NULL;

-- Backfill: assign all existing devices to the first active network.
UPDATE devices SET network_id = (
    SELECT id FROM networks WHERE is_active = true ORDER BY created_at LIMIT 1
) WHERE network_id IS NULL;

CREATE INDEX idx_devices_network_id ON devices(network_id);
