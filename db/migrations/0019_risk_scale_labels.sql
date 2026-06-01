-- +goose Up
CREATE TABLE risk_scale_labels (
    scale       TEXT NOT NULL,
    level       INT  NOT NULL CHECK (level BETWEEN 1 AND 5),
    label       TEXT NOT NULL DEFAULT '',
    description TEXT NOT NULL DEFAULT '',
    PRIMARY KEY (scale, level)
);

-- +goose Down
DROP TABLE IF EXISTS risk_scale_labels;
