-- Seed default admin user (password: admin)
-- IMPORTANT: Change the password after first login!
INSERT INTO users (username, email, password_hash, first_name, last_name, is_active, is_admin)
VALUES (
    'admin',
    'admin@outpost.local',
    crypt('admin', gen_salt('bf')),
    'Admin',
    'User',
    true,
    true
) ON CONFLICT (username) DO NOTHING;

-- Assign admin to 'admins' and 'everyone' groups
INSERT INTO user_groups (user_id, group_id)
SELECT u.id, g.id
FROM users u, groups g
WHERE u.username = 'admin' AND g.name = 'admins'
ON CONFLICT DO NOTHING;

INSERT INTO user_groups (user_id, group_id)
SELECT u.id, g.id
FROM users u, groups g
WHERE u.username = 'admin' AND g.name = 'everyone'
ON CONFLICT DO NOTHING;

-- Assign admin role
INSERT INTO user_roles (user_id, role_id)
SELECT u.id, r.id
FROM users u, roles r
WHERE u.username = 'admin' AND r.name = 'admin'
ON CONFLICT DO NOTHING;

-- Create default network
INSERT INTO networks (name, address, dns, port)
VALUES ('default', '10.10.0.0/16', ARRAY['1.1.1.1', '8.8.8.8'], 51820)
ON CONFLICT (name) DO NOTHING;
