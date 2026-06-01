-- +goose Up
ALTER TABLE risks ADD COLUMN decision_notes TEXT NOT NULL DEFAULT '';

-- +goose Down
ALTER TABLE risks DROP COLUMN decision_notes;
