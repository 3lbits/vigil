-- +goose Up

CREATE TABLE users (
    id          UUID        NOT NULL DEFAULT gen_random_uuid() PRIMARY KEY,
    provider    TEXT        NOT NULL,
    provider_id TEXT        NOT NULL,
    email       TEXT        NOT NULL DEFAULT '',
    name        TEXT        NOT NULL DEFAULT '',
    role        TEXT        NOT NULL DEFAULT 'viewer'
                            CHECK (role IN ('admin', 'editor', 'viewer')),
    created_at  TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    updated_at  TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    UNIQUE (provider, provider_id)
);

CREATE TABLE sessions (
    token        TEXT        NOT NULL PRIMARY KEY,
    user_id      UUID        NOT NULL REFERENCES users(id) ON DELETE CASCADE,
    created_at   TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    expires_at   TIMESTAMPTZ NOT NULL DEFAULT NOW() + INTERVAL '24 hours',
    last_seen_at TIMESTAMPTZ NOT NULL DEFAULT NOW()
);
CREATE INDEX sessions_user_id_idx    ON sessions (user_id);
CREATE INDEX sessions_expires_at_idx ON sessions (expires_at);

-- +goose Down

DROP TABLE IF EXISTS sessions;
DROP TABLE IF EXISTS users;
