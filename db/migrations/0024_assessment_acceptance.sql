-- +goose Up
ALTER TABLE risk_assessments
    ADD COLUMN acceptance_note TEXT NOT NULL DEFAULT '';

-- +goose Down
ALTER TABLE risk_assessments DROP COLUMN acceptance_note;
