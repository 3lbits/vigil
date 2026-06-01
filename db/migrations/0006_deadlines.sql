-- +goose Up

CREATE TABLE deadlines (
    id                  UUID        NOT NULL DEFAULT gen_random_uuid() PRIMARY KEY,

    -- Classification
    deadline_type       TEXT        NOT NULL
                            CHECK (deadline_type IN ('regulatory', 'internal')),
    trigger_type        TEXT        NOT NULL DEFAULT 'calendar'
                            CHECK (trigger_type IN ('calendar', 'event_triggered')),
    is_recurring        BOOLEAN     NOT NULL DEFAULT false,
    recurrence          TEXT        NOT NULL DEFAULT 'none'
                            CHECK (recurrence IN ('none', 'annual', 'quarterly', 'monthly')),

    -- Identity
    title               TEXT        NOT NULL,
    description         TEXT        NOT NULL DEFAULT '',
    source_ref          TEXT        NOT NULL DEFAULT '',
    owner               TEXT        NOT NULL DEFAULT '',

    -- Calendar-type due date
    due_date            DATE,

    -- Event-triggered SLA fields
    timeframe_hours     INT,
    triggered_at        TIMESTAMPTZ,

    -- Notification lead time (days before due_date to start alerting)
    lead_time_days      INT         NOT NULL DEFAULT 7,

    -- Status lifecycle
    status              TEXT        NOT NULL DEFAULT 'upcoming'
                            CHECK (status IN ('upcoming', 'due_soon', 'overdue', 'resolved')),
    resolved_at         TIMESTAMPTZ,
    resolved_by         TEXT        NOT NULL DEFAULT '',
    resolution_notes    TEXT        NOT NULL DEFAULT '',

    -- Relationships (all nullable)
    requirement_id      UUID        REFERENCES requirements(id) ON DELETE SET NULL,
    measure_id          UUID        REFERENCES measures(id) ON DELETE SET NULL,
    activity_id         UUID        REFERENCES activities(id) ON DELETE SET NULL,

    created_at          TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    updated_at          TIMESTAMPTZ NOT NULL DEFAULT NOW()
);

CREATE INDEX deadlines_type_idx     ON deadlines (deadline_type);
CREATE INDEX deadlines_status_idx   ON deadlines (status);
CREATE INDEX deadlines_due_date_idx ON deadlines (due_date ASC NULLS LAST);

-- +goose Down

DROP TABLE IF EXISTS deadlines;
