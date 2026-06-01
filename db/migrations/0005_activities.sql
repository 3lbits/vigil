-- +goose Up

CREATE TABLE activities (
    id                  UUID        NOT NULL DEFAULT gen_random_uuid() PRIMARY KEY,
    measure_id          UUID        NOT NULL REFERENCES measures(id) ON DELETE CASCADE,
    title               TEXT        NOT NULL,
    description         TEXT        NOT NULL DEFAULT '',
    activity_type       TEXT        NOT NULL DEFAULT 'one_off'
                            CHECK (activity_type IN ('one_off', 'recurring')),
    recurrence          TEXT        NOT NULL DEFAULT 'none'
                            CHECK (recurrence IN ('none', 'monthly', 'quarterly', 'annual', 'ad_hoc')),
    status              TEXT        NOT NULL DEFAULT 'planned'
                            CHECK (status IN ('planned', 'in_progress', 'completed', 'overdue')),
    owner               TEXT        NOT NULL DEFAULT '',
    due_date            DATE,
    completed_at        TIMESTAMPTZ,
    completed_by        TEXT        NOT NULL DEFAULT '',
    notes               TEXT        NOT NULL DEFAULT '',
    evidence_url        TEXT        NOT NULL DEFAULT '',
    parent_activity_id  UUID        REFERENCES activities(id) ON DELETE SET NULL,
    created_at          TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    updated_at          TIMESTAMPTZ NOT NULL DEFAULT NOW()
);

CREATE INDEX activities_measure_id_idx ON activities (measure_id);
CREATE INDEX activities_status_idx     ON activities (status);
CREATE INDEX activities_due_date_idx   ON activities (due_date);

ALTER TABLE measures ADD COLUMN last_verified_at TIMESTAMPTZ;

-- +goose Down

ALTER TABLE measures DROP COLUMN last_verified_at;
DROP TABLE IF EXISTS activities;
