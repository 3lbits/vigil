-- +goose Up
CREATE TABLE risk_asset_links (
    risk_id    UUID NOT NULL REFERENCES risks(id) ON DELETE CASCADE,
    asset_id   UUID NOT NULL REFERENCES assets(id) ON DELETE CASCADE,
    created_at TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    PRIMARY KEY (risk_id, asset_id)
);

CREATE INDEX idx_risk_asset_links_asset ON risk_asset_links (asset_id);

-- +goose Down
DROP INDEX IF EXISTS idx_risk_asset_links_asset;
DROP TABLE IF EXISTS risk_asset_links;
