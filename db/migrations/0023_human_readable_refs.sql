-- +goose Up
ALTER TABLE risk_assessments ADD COLUMN ref_num SERIAL;
ALTER TABLE risks             ADD COLUMN ref_num SERIAL;
ALTER TABLE measures          ADD COLUMN ref_num SERIAL;
ALTER TABLE activities        ADD COLUMN ref_num SERIAL;

-- +goose Down
ALTER TABLE risk_assessments DROP COLUMN IF EXISTS ref_num;
ALTER TABLE risks             DROP COLUMN IF EXISTS ref_num;
ALTER TABLE measures          DROP COLUMN IF EXISTS ref_num;
ALTER TABLE activities        DROP COLUMN IF EXISTS ref_num;
