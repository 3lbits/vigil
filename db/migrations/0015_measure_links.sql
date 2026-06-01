-- +goose Up

CREATE TABLE measure_links (
    id          UUID        NOT NULL DEFAULT gen_random_uuid() PRIMARY KEY,
    measure_id  UUID        NOT NULL REFERENCES measures(id) ON DELETE CASCADE,
    url         TEXT        NOT NULL,
    label       TEXT        NOT NULL DEFAULT '',
    created_at  TIMESTAMPTZ NOT NULL DEFAULT NOW()
);

CREATE INDEX measure_links_measure_id_idx ON measure_links (measure_id);

-- +goose Down

DROP TABLE IF EXISTS measure_links;
