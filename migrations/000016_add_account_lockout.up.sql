ALTER TABLE users
    ADD COLUMN failed_login_attempts int NOT NULL DEFAULT 0,
    ADD COLUMN locked_until timestamptz;
