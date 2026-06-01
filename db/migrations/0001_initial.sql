-- +goose Up

CREATE TABLE frameworks (
    id          UUID        NOT NULL DEFAULT gen_random_uuid() PRIMARY KEY,
    name        TEXT        NOT NULL,
    short_name  TEXT        NOT NULL DEFAULT '',
    version     TEXT        NOT NULL DEFAULT '',
    description TEXT        NOT NULL DEFAULT '',
    created_at  TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    updated_at  TIMESTAMPTZ NOT NULL DEFAULT NOW()
);

CREATE TABLE requirements (
    id           UUID        NOT NULL DEFAULT gen_random_uuid() PRIMARY KEY,
    framework_id UUID        NOT NULL REFERENCES frameworks(id) ON DELETE CASCADE,
    ref          TEXT        NOT NULL,
    title        TEXT        NOT NULL,
    description  TEXT        NOT NULL DEFAULT '',
    sort_order   INT         NOT NULL DEFAULT 0,
    created_at   TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    updated_at   TIMESTAMPTZ NOT NULL DEFAULT NOW()
);

CREATE TABLE measures (
    id          UUID        NOT NULL DEFAULT gen_random_uuid() PRIMARY KEY,
    name        TEXT        NOT NULL,
    description TEXT        NOT NULL DEFAULT '',
    category    TEXT        NOT NULL DEFAULT '',
    owner       TEXT        NOT NULL DEFAULT '',
    status      TEXT        NOT NULL DEFAULT 'planned'
                            CHECK (status IN ('planned', 'in_progress', 'implemented', 'deprecated')),
    created_at  TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    updated_at  TIMESTAMPTZ NOT NULL DEFAULT NOW()
);

-- A measure can satisfy requirements across multiple frameworks.
CREATE TABLE measure_requirements (
    measure_id     UUID        NOT NULL REFERENCES measures(id) ON DELETE CASCADE,
    requirement_id UUID        NOT NULL REFERENCES requirements(id) ON DELETE CASCADE,
    note           TEXT        NOT NULL DEFAULT '',
    created_at     TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    PRIMARY KEY (measure_id, requirement_id)
);

-- +goose Down

DROP TABLE IF EXISTS measure_requirements;
DROP TABLE IF EXISTS measures;
DROP TABLE IF EXISTS requirements;
DROP TABLE IF EXISTS frameworks;
