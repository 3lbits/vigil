-- +goose Up
ALTER TABLE frameworks
    ADD COLUMN not_relevant        BOOLEAN NOT NULL DEFAULT false,
    ADD COLUMN not_relevant_reason TEXT    NOT NULL DEFAULT '';

-- +goose Down
ALTER TABLE frameworks
    DROP COLUMN IF EXISTS not_relevant,
    DROP COLUMN IF EXISTS not_relevant_reason;
