-- Add health tracking columns to gateways for HA failover.
ALTER TABLE gateways ADD COLUMN IF NOT EXISTS health_status TEXT NOT NULL DEFAULT 'unknown';
ALTER TABLE gateways ADD COLUMN IF NOT EXISTS consecutive_failures INT NOT NULL DEFAULT 0;

COMMENT ON COLUMN gateways.health_status IS 'Gateway health: healthy, degraded, unhealthy, unknown';
COMMENT ON COLUMN gateways.consecutive_failures IS 'Consecutive heartbeat failures (reset on successful heartbeat)';
