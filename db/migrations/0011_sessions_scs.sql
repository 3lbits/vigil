-- +goose Up
DROP TABLE sessions;

CREATE TABLE sessions (
    token   TEXT        NOT NULL PRIMARY KEY,
    data    BYTEA       NOT NULL,
    expiry  TIMESTAMPTZ NOT NULL,
    user_id UUID        REFERENCES users(id) ON DELETE CASCADE
);

CREATE INDEX sessions_expiry_idx  ON sessions (expiry);
CREATE INDEX sessions_user_id_idx ON sessions (user_id);

-- +goose Down
DROP TABLE sessions;

CREATE TABLE sessions (
    token        TEXT        NOT NULL PRIMARY KEY,
    user_id      UUID        NOT NULL REFERENCES users(id) ON DELETE CASCADE,
    created_at   TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    expires_at   TIMESTAMPTZ NOT NULL DEFAULT NOW() + INTERVAL '24 hours',
    last_seen_at TIMESTAMPTZ NOT NULL DEFAULT NOW()
);
