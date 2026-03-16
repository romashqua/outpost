-- Remove default ACL entries for the "everyone" group.
DELETE FROM network_acls
WHERE group_id = (SELECT id FROM groups WHERE name = 'everyone' AND is_system = true);
