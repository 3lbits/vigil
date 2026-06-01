-- +goose Up

ALTER TABLE activities DROP CONSTRAINT IF EXISTS activities_recurrence_check;
ALTER TABLE activities ADD CONSTRAINT activities_recurrence_check
    CHECK (recurrence IN ('none', 'monthly', 'tertially', 'annual', 'ad_hoc'));

-- +goose Down

ALTER TABLE activities DROP CONSTRAINT IF EXISTS activities_recurrence_check;
ALTER TABLE activities ADD CONSTRAINT activities_recurrence_check
    CHECK (recurrence IN ('none', 'monthly', 'quarterly', 'annual', 'ad_hoc'));