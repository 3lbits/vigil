-- +goose Up

ALTER TABLE users DROP CONSTRAINT IF EXISTS users_role_check;
ALTER TABLE users ADD CONSTRAINT users_role_check
    CHECK (role IN ('admin', 'editor', 'viewer', 'contributor'));

CREATE TABLE resource_participants (
    id            UUID        NOT NULL DEFAULT gen_random_uuid() PRIMARY KEY,
    resource_type TEXT        NOT NULL,
    resource_id   UUID        NOT NULL,
    user_id       UUID        NOT NULL REFERENCES users(id) ON DELETE CASCADE,
    role          TEXT        NOT NULL DEFAULT 'participant'
                              CHECK (role IN ('owner', 'participant')),
    created_at    TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    UNIQUE (resource_type, resource_id, user_id)
);
CREATE INDEX rp_lookup_idx ON resource_participants (resource_type, resource_id, user_id);

-- +goose Down

DROP TABLE IF EXISTS resource_participants;
ALTER TABLE users DROP CONSTRAINT IF EXISTS users_role_check;
ALTER TABLE users ADD CONSTRAINT users_role_check
    CHECK (role IN ('admin', 'editor', 'viewer'));
