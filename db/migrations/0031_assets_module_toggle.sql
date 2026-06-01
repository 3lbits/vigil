-- +goose Up
ALTER TABLE app_settings
ADD COLUMN assets_enabled BOOLEAN NOT NULL DEFAULT TRUE;

-- +goose Down
ALTER TABLE app_settings
DROP COLUMN IF EXISTS assets_enabled;
