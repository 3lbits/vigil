-- +goose Up
ALTER TABLE app_settings
ALTER COLUMN avvik_enabled SET DEFAULT FALSE;

UPDATE app_settings
SET avvik_enabled = FALSE
WHERE id = 1;

-- +goose Down
ALTER TABLE app_settings
ALTER COLUMN avvik_enabled SET DEFAULT TRUE;

UPDATE app_settings
SET avvik_enabled = TRUE
WHERE id = 1;
