-- Add unique constraint on mfa_totp.user_id so ON CONFLICT (user_id) works.
CREATE UNIQUE INDEX IF NOT EXISTS mfa_totp_user_id_key ON mfa_totp (user_id);
