CREATE TABLE IF NOT EXISTS ssh_users (
    id              BIGSERIAL       PRIMARY KEY,
    username        TEXT            NOT NULL UNIQUE,
    display_name    TEXT            NOT NULL DEFAULT '',
    public_key      TEXT            NOT NULL,
    key_type        TEXT            NOT NULL DEFAULT 'ssh-ed25519',
    fingerprint     TEXT            NOT NULL UNIQUE,
    is_active       BOOLEAN         NOT NULL DEFAULT TRUE,
    last_login_at   TIMESTAMPTZ,
    created_at      TIMESTAMPTZ     NOT NULL DEFAULT NOW(),
    updated_at      TIMESTAMPTZ     NOT NULL DEFAULT NOW()
);

CREATE INDEX IF NOT EXISTS idx_ssh_users_fingerprint
    ON ssh_users (fingerprint) WHERE is_active = TRUE;

CREATE INDEX IF NOT EXISTS idx_ssh_users_username
    ON ssh_users (username) WHERE is_active = TRUE;
