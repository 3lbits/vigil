-- +goose Up
CREATE TABLE app_settings (
    id                   INT PRIMARY KEY DEFAULT 1 CHECK (id = 1),
    compliance_enabled   BOOLEAN NOT NULL DEFAULT TRUE,
    risk_enabled         BOOLEAN NOT NULL DEFAULT TRUE,
    activities_enabled   BOOLEAN NOT NULL DEFAULT TRUE
);
INSERT INTO app_settings (id) VALUES (1);

-- +goose Down
DROP TABLE IF EXISTS app_settings;
