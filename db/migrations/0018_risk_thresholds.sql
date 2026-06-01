-- +goose Up
ALTER TABLE risk_global_settings
    ADD COLUMN low_max  INT NOT NULL DEFAULT 5,
    ADD COLUMN high_min INT NOT NULL DEFAULT 12;

-- +goose Down
ALTER TABLE risk_global_settings
    DROP COLUMN IF EXISTS low_max,
    DROP COLUMN IF EXISTS high_min;
