-- +goose Up

ALTER TABLE requirements
    ADD COLUMN not_relevant        BOOLEAN NOT NULL DEFAULT false,
    ADD COLUMN not_relevant_reason TEXT    NOT NULL DEFAULT '';

-- +goose Down

ALTER TABLE requirements
    DROP COLUMN IF EXISTS not_relevant,
    DROP COLUMN IF EXISTS not_relevant_reason;
