-- Add UNIQUE constraints on smart_routes.name and proxy_servers.name.
-- The handler code already checks for PG error 23505 (unique violation)
-- but the constraints were missing from the original migration.

ALTER TABLE smart_routes ADD CONSTRAINT smart_routes_name_key UNIQUE (name);
ALTER TABLE proxy_servers ADD CONSTRAINT proxy_servers_name_key UNIQUE (name);
