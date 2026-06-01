-- +goose Up
ALTER TABLE users
    ADD COLUMN org_id UUID REFERENCES organizations(id) ON DELETE SET NULL;

-- +goose Down
ALTER TABLE users DROP COLUMN org_id;
