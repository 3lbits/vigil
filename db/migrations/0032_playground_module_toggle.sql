-- +goose Up
ALTER TABLE app_settings
ADD COLUMN playground_enabled BOOLEAN NOT NULL DEFAULT TRUE;

-- +goose Down
ALTER TABLE app_settings
DROP COLUMN IF EXISTS playground_enabled;
