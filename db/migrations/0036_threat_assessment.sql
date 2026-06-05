-- +goose Up
ALTER TABLE risk_assessments
    ADD COLUMN threat_assessment_enabled BOOLEAN NOT NULL DEFAULT FALSE,
    ADD COLUMN threat_app_description    TEXT    NOT NULL DEFAULT '',
    ADD COLUMN threat_information_flow   TEXT    NOT NULL DEFAULT '';

-- +goose Down
ALTER TABLE risk_assessments
    DROP COLUMN threat_assessment_enabled,
    DROP COLUMN threat_app_description,
    DROP COLUMN threat_information_flow;
