-- +goose Up
ALTER TABLE activities ADD COLUMN priority TEXT NOT NULL DEFAULT 'medium';
ALTER TABLE activities ADD COLUMN kind     TEXT NOT NULL DEFAULT 'review';

-- +goose Down
ALTER TABLE activities DROP COLUMN priority;
ALTER TABLE activities DROP COLUMN kind;
