CREATE TABLE IF NOT EXISTS signal_images (
    id           BIGSERIAL PRIMARY KEY,
    signal_id    BIGINT      NOT NULL REFERENCES signals(id) ON DELETE CASCADE,
    mime_type    TEXT        NOT NULL DEFAULT 'image/png',
    image_bytes  BYTEA       NOT NULL,
    width        INT         NOT NULL,
    height       INT         NOT NULL,
    render_status TEXT       NOT NULL,
    error_text   TEXT        NOT NULL DEFAULT '',
    retry_count  SMALLINT    NOT NULL DEFAULT 0,
    next_retry_at TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    created_at   TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    expires_at   TIMESTAMPTZ NOT NULL
);

CREATE UNIQUE INDEX IF NOT EXISTS idx_signal_images_signal_unique
    ON signal_images (signal_id);

CREATE INDEX IF NOT EXISTS idx_signal_images_expiry
    ON signal_images (expires_at);

CREATE INDEX IF NOT EXISTS idx_signal_images_retry
    ON signal_images (render_status, next_retry_at, retry_count);
