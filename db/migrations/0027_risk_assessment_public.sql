-- +goose Up
ALTER TABLE risk_assessments ADD COLUMN is_public boolean NOT NULL DEFAULT false;

-- +goose Down
ALTER TABLE risk_assessments DROP COLUMN is_public;
