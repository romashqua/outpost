-- Missing indexes for frequently joined FK columns.
-- Without these, queries joining user_groups and network_acls do full table scans.
CREATE INDEX IF NOT EXISTS idx_user_groups_user_id ON user_groups(user_id);
CREATE INDEX IF NOT EXISTS idx_user_groups_group_id ON user_groups(group_id);
CREATE INDEX IF NOT EXISTS idx_network_acls_network_id ON network_acls(network_id);
CREATE INDEX IF NOT EXISTS idx_network_acls_group_id ON network_acls(group_id);
CREATE INDEX IF NOT EXISTS idx_s2s_routes_via_gateway ON s2s_routes(via_gateway);

-- Missing unique constraint on WebAuthn credential IDs.
CREATE UNIQUE INDEX IF NOT EXISTS mfa_webauthn_credential_id_key ON mfa_webauthn(credential_id);
