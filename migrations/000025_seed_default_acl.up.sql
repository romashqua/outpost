-- Grant the "everyone" group full access to all active networks.
-- This ensures out-of-the-box connectivity without manual ACL setup.
-- {0.0.0.0/0} is a wildcard meaning "all destinations in this network".
INSERT INTO network_acls (network_id, group_id, allowed_ips)
SELECT n.id, g.id, '{0.0.0.0/0}'::cidr[]
FROM networks n, groups g
WHERE g.name = 'everyone' AND g.is_system = true AND n.is_active = true
ON CONFLICT DO NOTHING;
