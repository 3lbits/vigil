-- +goose Up

-- Global risk settings (singleton row)
CREATE TABLE risk_global_settings (
    id                  INT PRIMARY KEY DEFAULT 1 CHECK (id = 1),
    acceptance_criteria TEXT NOT NULL DEFAULT ''
);
INSERT INTO risk_global_settings (id) VALUES (1);

-- Organization tree — top → department → team (max 2 hops from root)
CREATE TABLE organizations (
    id         UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    name       TEXT NOT NULL,
    parent_id  UUID REFERENCES organizations(id),
    created_at TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    updated_at TIMESTAMPTZ NOT NULL DEFAULT NOW()
);

-- Migrate existing risk_assessments data before dropping columns
ALTER TABLE risk_assessments
    DROP COLUMN methodology,
    DROP COLUMN risk_acceptance_criteria,
    DROP COLUMN org_level,
    DROP COLUMN risk_owner,
    ADD COLUMN risk_owner_id UUID REFERENCES users(id),
    ADD COLUMN org_id        UUID REFERENCES organizations(id);

-- Migrate existing risks data before dropping columns
ALTER TABLE risks
    DROP COLUMN assets,
    DROP COLUMN threats,
    DROP COLUMN undesired_event,
    DROP COLUMN vulnerabilities,
    DROP COLUMN likelihood_inherent,
    DROP COLUMN consequence_inherent,
    DROP COLUMN owner,
    DROP COLUMN treatment_notes,
    ADD COLUMN description           TEXT NOT NULL DEFAULT '',
    ADD COLUMN likelihood_reasoning  TEXT NOT NULL DEFAULT '',
    ADD COLUMN consequence_reasoning TEXT NOT NULL DEFAULT '',
    ADD COLUMN owner_id              UUID REFERENCES users(id);

-- Activity links for treatment step
CREATE TABLE risk_activity_links (
    risk_id     UUID NOT NULL REFERENCES risks(id) ON DELETE CASCADE,
    activity_id UUID NOT NULL REFERENCES activities(id) ON DELETE CASCADE,
    note        TEXT NOT NULL DEFAULT '',
    PRIMARY KEY (risk_id, activity_id)
);

-- +goose Down
DROP TABLE IF EXISTS risk_activity_links;

ALTER TABLE risks
    DROP COLUMN IF EXISTS description,
    DROP COLUMN IF EXISTS likelihood_reasoning,
    DROP COLUMN IF EXISTS consequence_reasoning,
    DROP COLUMN IF EXISTS owner_id,
    ADD COLUMN assets               TEXT NOT NULL DEFAULT '',
    ADD COLUMN threats              TEXT NOT NULL DEFAULT '',
    ADD COLUMN undesired_event      TEXT NOT NULL DEFAULT '',
    ADD COLUMN vulnerabilities      TEXT NOT NULL DEFAULT '',
    ADD COLUMN likelihood_inherent  INT CHECK (likelihood_inherent BETWEEN 1 AND 5),
    ADD COLUMN consequence_inherent INT CHECK (consequence_inherent BETWEEN 1 AND 5),
    ADD COLUMN owner                TEXT NOT NULL DEFAULT '',
    ADD COLUMN treatment_notes      TEXT NOT NULL DEFAULT '';

ALTER TABLE risk_assessments
    DROP COLUMN IF EXISTS risk_owner_id,
    DROP COLUMN IF EXISTS org_id,
    ADD COLUMN methodology              TEXT NOT NULL DEFAULT '',
    ADD COLUMN risk_acceptance_criteria TEXT NOT NULL DEFAULT '',
    ADD COLUMN org_level                TEXT NOT NULL DEFAULT 'org',
    ADD COLUMN risk_owner               TEXT NOT NULL DEFAULT '';

DROP TABLE IF EXISTS organizations;
DROP TABLE IF EXISTS risk_global_settings;
