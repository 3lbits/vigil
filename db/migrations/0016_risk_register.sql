-- +goose Up
CREATE TABLE risk_assessments (
    id                       UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    name                     TEXT NOT NULL,
    scope                    TEXT NOT NULL DEFAULT '',
    analysis_object          TEXT NOT NULL DEFAULT '',
    security_objectives      TEXT NOT NULL DEFAULT '',
    business_objectives      TEXT NOT NULL DEFAULT '',
    methodology              TEXT NOT NULL DEFAULT '',
    risk_owner               TEXT NOT NULL DEFAULT '',
    risk_acceptance_criteria TEXT NOT NULL DEFAULT '',
    type                     TEXT NOT NULL DEFAULT 'security',
    org_level                TEXT NOT NULL DEFAULT 'org',
    status                   TEXT NOT NULL DEFAULT 'draft',
    current_step             INT NOT NULL DEFAULT 1,
    last_reviewed_at         TIMESTAMPTZ,
    created_by               UUID REFERENCES users(id),
    created_at               TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    updated_at               TIMESTAMPTZ NOT NULL DEFAULT NOW()
);

CREATE TABLE risks (
    id                   UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    assessment_id        UUID NOT NULL REFERENCES risk_assessments(id) ON DELETE CASCADE,
    name                 TEXT NOT NULL,
    assets               TEXT NOT NULL DEFAULT '',
    threats              TEXT NOT NULL DEFAULT '',
    undesired_event      TEXT NOT NULL DEFAULT '',
    vulnerabilities      TEXT NOT NULL DEFAULT '',
    likelihood_inherent  INT CHECK (likelihood_inherent BETWEEN 1 AND 5),
    consequence_inherent INT CHECK (consequence_inherent BETWEEN 1 AND 5),
    likelihood_current   INT CHECK (likelihood_current BETWEEN 1 AND 5),
    consequence_current  INT CHECK (consequence_current BETWEEN 1 AND 5),
    likelihood_target    INT CHECK (likelihood_target BETWEEN 1 AND 5),
    consequence_target   INT CHECK (consequence_target BETWEEN 1 AND 5),
    risk_decision        TEXT NOT NULL DEFAULT '',
    treatment_notes      TEXT NOT NULL DEFAULT '',
    owner                TEXT NOT NULL DEFAULT '',
    status               TEXT NOT NULL DEFAULT 'identified',
    created_at           TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    updated_at           TIMESTAMPTZ NOT NULL DEFAULT NOW()
);

CREATE TABLE risk_measure_links (
    risk_id    UUID NOT NULL REFERENCES risks(id) ON DELETE CASCADE,
    measure_id UUID NOT NULL REFERENCES measures(id) ON DELETE CASCADE,
    note       TEXT NOT NULL DEFAULT '',
    PRIMARY KEY (risk_id, measure_id)
);

-- +goose Down
DROP TABLE IF EXISTS risk_measure_links;
DROP TABLE IF EXISTS risks;
DROP TABLE IF EXISTS risk_assessments;
