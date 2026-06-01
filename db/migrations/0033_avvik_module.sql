-- +goose Up
ALTER TABLE app_settings
ADD COLUMN avvik_enabled BOOLEAN NOT NULL DEFAULT TRUE;

ALTER TABLE organizations
ADD COLUMN key TEXT;

CREATE UNIQUE INDEX ux_organizations_key ON organizations (key) WHERE key IS NOT NULL;

CREATE TABLE avvik (
    id                             UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    title                          TEXT NOT NULL,
    description                    TEXT NOT NULL DEFAULT '',
    discovered_at                  TIMESTAMPTZ NOT NULL,
    reported_at                    TIMESTAMPTZ,
    reporter_name                  TEXT NOT NULL DEFAULT '',
    reporter_email                 TEXT NOT NULL DEFAULT '',
    assigned_to                    UUID REFERENCES users(id) ON DELETE SET NULL,
    org_unit_id                    UUID REFERENCES organizations(id) ON DELETE SET NULL,
    risk_level                     TEXT NOT NULL DEFAULT 'medium' CHECK (risk_level IN ('low', 'medium', 'high')),
    status                         TEXT NOT NULL DEFAULT 'new' CHECK (status IN ('new', 'triaging', 'investigating', 'mitigating', 'closed')),
    personal_data                  BOOLEAN NOT NULL DEFAULT FALSE,
    ksi                            BOOLEAN NOT NULL DEFAULT FALSE,
    ksi_information_owner          TEXT NOT NULL DEFAULT '',
    market_sensitive               BOOLEAN NOT NULL DEFAULT FALSE,
    market_assessment_note         TEXT NOT NULL DEFAULT '',
    gdpr_deadline_at               TIMESTAMPTZ,
    realised_risk_id               UUID REFERENCES risks(id) ON DELETE SET NULL,
    root_cause                     TEXT NOT NULL DEFAULT '',
    lessons_learned                TEXT NOT NULL DEFAULT '',
    log_qa_done                    BOOLEAN NOT NULL DEFAULT FALSE,
    followups_delegated            BOOLEAN NOT NULL DEFAULT FALSE,
    reporter_informed              BOOLEAN NOT NULL DEFAULT FALSE,
    org_informed                   BOOLEAN NOT NULL DEFAULT FALSE,
    mgmt_informed                  BOOLEAN NOT NULL DEFAULT FALSE,
    decisions_anchored             BOOLEAN NOT NULL DEFAULT FALSE,
    implementation_deadline_set    BOOLEAN NOT NULL DEFAULT FALSE,
    closure_summary                TEXT NOT NULL DEFAULT '',
    closed_at                      TIMESTAMPTZ,
    external_reference             TEXT NOT NULL DEFAULT '',
    import_source                  TEXT,
    imported_at                    TIMESTAMPTZ,
    created_at                     TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    updated_at                     TIMESTAMPTZ NOT NULL DEFAULT NOW()
);

CREATE TABLE avvik_events (
    id            UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    avvik_id      UUID NOT NULL REFERENCES avvik(id) ON DELETE CASCADE,
    actor_id      UUID REFERENCES users(id) ON DELETE SET NULL,
    actor_label   TEXT NOT NULL DEFAULT '',
    event_type    TEXT NOT NULL CHECK (event_type IN ('created', 'status_changed', 'risk_reassessed', 'triaged', 'note_added', 'notification_sent', 'evidence_added', 'measure_linked', 'activity_linked', 'closed', 'imported')),
    payload       JSONB NOT NULL DEFAULT '{}'::jsonb,
    occurred_at   TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    import_source TEXT
);

CREATE TABLE avvik_attachments (
    id          UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    avvik_id    UUID NOT NULL REFERENCES avvik(id) ON DELETE CASCADE,
    filename    TEXT NOT NULL,
    storage_key TEXT NOT NULL,
    uploaded_by UUID REFERENCES users(id) ON DELETE SET NULL,
    uploaded_at TIMESTAMPTZ NOT NULL DEFAULT NOW()
);

CREATE TABLE avvik_measures (
    avvik_id      UUID NOT NULL REFERENCES avvik(id) ON DELETE CASCADE,
    measure_id    UUID NOT NULL REFERENCES measures(id) ON DELETE CASCADE,
    relationship  TEXT NOT NULL DEFAULT 'corrective' CHECK (relationship IN ('corrective', 'preventive', 'compensating')),
    created_at    TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    PRIMARY KEY (avvik_id, measure_id)
);

CREATE TABLE avvik_activities (
    avvik_id      UUID NOT NULL REFERENCES avvik(id) ON DELETE CASCADE,
    activity_id   UUID NOT NULL REFERENCES activities(id) ON DELETE CASCADE,
    created_at    TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    PRIMARY KEY (avvik_id, activity_id)
);

CREATE TABLE avvik_notifications (
    id         UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    avvik_id   UUID NOT NULL REFERENCES avvik(id) ON DELETE CASCADE,
    audience   TEXT NOT NULL CHECK (audience IN ('reporter', 'management', 'organisation', 'exposed_employees', 'datatilsynet', 'nve', 'finanstilsynet', 'bors')),
    sent_at    TIMESTAMPTZ NOT NULL,
    sent_by    UUID REFERENCES users(id) ON DELETE SET NULL,
    notes      TEXT NOT NULL DEFAULT ''
);

CREATE UNIQUE INDEX ux_avvik_import_idempotency
ON avvik (import_source, external_reference)
WHERE import_source IS NOT NULL AND external_reference <> '';

CREATE INDEX idx_avvik_status ON avvik(status);
CREATE INDEX idx_avvik_risk_level ON avvik(risk_level);
CREATE INDEX idx_avvik_personal_data ON avvik(personal_data);
CREATE INDEX idx_avvik_org_unit_id ON avvik(org_unit_id);
CREATE INDEX idx_avvik_assigned_to ON avvik(assigned_to);
CREATE INDEX idx_avvik_gdpr_deadline ON avvik(gdpr_deadline_at);
CREATE INDEX idx_avvik_events_avvik_occurred ON avvik_events(avvik_id, occurred_at DESC);

-- +goose Down
DROP INDEX IF EXISTS idx_avvik_events_avvik_occurred;
DROP INDEX IF EXISTS idx_avvik_gdpr_deadline;
DROP INDEX IF EXISTS idx_avvik_assigned_to;
DROP INDEX IF EXISTS idx_avvik_org_unit_id;
DROP INDEX IF EXISTS idx_avvik_personal_data;
DROP INDEX IF EXISTS idx_avvik_risk_level;
DROP INDEX IF EXISTS idx_avvik_status;
DROP INDEX IF EXISTS ux_avvik_import_idempotency;
DROP TABLE IF EXISTS avvik_notifications;
DROP TABLE IF EXISTS avvik_activities;
DROP TABLE IF EXISTS avvik_measures;
DROP TABLE IF EXISTS avvik_attachments;
DROP TABLE IF EXISTS avvik_events;
DROP TABLE IF EXISTS avvik;
DROP INDEX IF EXISTS ux_organizations_key;
ALTER TABLE organizations DROP COLUMN IF EXISTS key;
ALTER TABLE app_settings DROP COLUMN IF EXISTS avvik_enabled;
