-- +goose Up

ALTER TABLE frameworks
    ADD COLUMN framework_type TEXT NOT NULL DEFAULT 'regulation'
        CHECK (framework_type IN ('regulation', 'standard', 'directive'));

-- +goose Down

ALTER TABLE frameworks DROP COLUMN framework_type;
