-- +goose Up

-- Remove tasks table (feature removed)
DROP TABLE IF EXISTS tasks;

-- Make activities.measure_id optional
ALTER TABLE activities DROP CONSTRAINT activities_measure_id_fkey;
ALTER TABLE activities ALTER COLUMN measure_id DROP NOT NULL;
ALTER TABLE activities ADD CONSTRAINT activities_measure_id_fkey
    FOREIGN KEY (measure_id) REFERENCES measures(id) ON DELETE SET NULL;

-- +goose Down

ALTER TABLE activities DROP CONSTRAINT activities_measure_id_fkey;
ALTER TABLE activities ALTER COLUMN measure_id SET NOT NULL;
ALTER TABLE activities ADD CONSTRAINT activities_measure_id_fkey
    FOREIGN KEY (measure_id) REFERENCES measures(id) ON DELETE CASCADE;
