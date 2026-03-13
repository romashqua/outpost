-- Drop key pairs
DROP TABLE IF EXISTS key_pairs;

-- Drop flow records
DROP TABLE IF EXISTS flow_records_default;
DROP TABLE IF EXISTS flow_records;

-- Drop posture policies
DROP TABLE IF EXISTS posture_policies;

-- Drop device posture
DROP TABLE IF EXISTS device_posture;

-- Remove tenant_id columns from existing tables
ALTER TABLE gateways DROP COLUMN IF EXISTS tenant_id;
ALTER TABLE networks DROP COLUMN IF EXISTS tenant_id;
ALTER TABLE users DROP COLUMN IF EXISTS tenant_id;

-- Drop tenants
DROP TABLE IF EXISTS tenants;
