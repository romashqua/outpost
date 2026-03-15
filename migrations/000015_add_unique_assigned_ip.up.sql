-- Add unique constraint on assigned_ip to prevent IP collisions.
CREATE UNIQUE INDEX IF NOT EXISTS devices_assigned_ip_unique ON devices (assigned_ip);
