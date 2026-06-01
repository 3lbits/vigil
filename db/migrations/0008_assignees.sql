-- +goose Up

ALTER TABLE tasks       ADD COLUMN assignee_id UUID REFERENCES users(id) ON DELETE SET NULL;
ALTER TABLE measures    ADD COLUMN assignee_id UUID REFERENCES users(id) ON DELETE SET NULL;
ALTER TABLE activities  ADD COLUMN assignee_id UUID REFERENCES users(id) ON DELETE SET NULL;

-- +goose Down

ALTER TABLE activities  DROP COLUMN assignee_id;
ALTER TABLE measures    DROP COLUMN assignee_id;
ALTER TABLE tasks       DROP COLUMN assignee_id;
