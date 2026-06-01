-- +goose Up
CREATE TABLE assets (
    id          UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    name        TEXT NOT NULL,
    description TEXT NOT NULL DEFAULT '',
    asset_type  TEXT NOT NULL DEFAULT '',
    owner       TEXT NOT NULL DEFAULT '',
    status      TEXT NOT NULL DEFAULT 'active'
                CHECK (status IN ('active', 'planned', 'retired')),
    criticality TEXT NOT NULL DEFAULT 'medium'
                CHECK (criticality IN ('low', 'medium', 'high')),
    created_at  TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    updated_at  TIMESTAMPTZ NOT NULL DEFAULT NOW()
);

CREATE TABLE risk_assessment_assets (
    assessment_id UUID NOT NULL REFERENCES risk_assessments(id) ON DELETE CASCADE,
    asset_id      UUID NOT NULL REFERENCES assets(id) ON DELETE CASCADE,
    created_at    TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    PRIMARY KEY (assessment_id, asset_id)
);

CREATE INDEX idx_assets_name ON assets (name);
CREATE INDEX idx_assets_owner ON assets (owner);
CREATE INDEX idx_assets_status ON assets (status);
CREATE INDEX idx_risk_assessment_assets_assessment ON risk_assessment_assets (assessment_id);

-- +goose Down
DROP INDEX IF EXISTS idx_risk_assessment_assets_assessment;
DROP INDEX IF EXISTS idx_assets_status;
DROP INDEX IF EXISTS idx_assets_owner;
DROP INDEX IF EXISTS idx_assets_name;
DROP TABLE IF EXISTS risk_assessment_assets;
DROP TABLE IF EXISTS assets;
