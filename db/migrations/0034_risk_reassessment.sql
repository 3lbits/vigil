-- +goose Up

ALTER TABLE risks
    ADD COLUMN review_needed BOOLEAN NOT NULL DEFAULT FALSE,
    ADD COLUMN review_due TIMESTAMPTZ,
    ADD COLUMN assessed_at TIMESTAMPTZ,
    ADD COLUMN assessed_by UUID REFERENCES users(id),
    ADD COLUMN assessment_rationale TEXT NOT NULL DEFAULT '';

CREATE INDEX idx_risks_review_needed ON risks (review_needed);
CREATE INDEX idx_risks_review_due ON risks (review_due);

CREATE TABLE risk_reassessment_events (
    id             UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    risk_id        UUID NOT NULL REFERENCES risks(id) ON DELETE CASCADE,
    measure_id     UUID NOT NULL REFERENCES measures(id) ON DELETE CASCADE,
    trigger_status TEXT NOT NULL CHECK (trigger_status IN ('implemented', 'deprecated')),
    triggered_at   TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    triggered_by   UUID REFERENCES users(id) ON DELETE SET NULL,
    note           TEXT NOT NULL DEFAULT ''
);

CREATE INDEX idx_risk_reassess_events_risk_time ON risk_reassessment_events (risk_id, triggered_at DESC);
CREATE INDEX idx_risk_reassess_events_measure_time ON risk_reassessment_events (measure_id, triggered_at DESC);

-- +goose Down

DROP INDEX IF EXISTS idx_risk_reassess_events_measure_time;
DROP INDEX IF EXISTS idx_risk_reassess_events_risk_time;
DROP TABLE IF EXISTS risk_reassessment_events;

DROP INDEX IF EXISTS idx_risks_review_due;
DROP INDEX IF EXISTS idx_risks_review_needed;

ALTER TABLE risks
    DROP COLUMN IF EXISTS assessment_rationale,
    DROP COLUMN IF EXISTS assessed_by,
    DROP COLUMN IF EXISTS assessed_at,
    DROP COLUMN IF EXISTS review_due,
    DROP COLUMN IF EXISTS review_needed;
