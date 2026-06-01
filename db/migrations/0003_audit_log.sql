-- +goose Up

CREATE TABLE audit_log (
    id          UUID        NOT NULL DEFAULT gen_random_uuid() PRIMARY KEY,
    event_time  TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    event       TEXT        NOT NULL,
    user_id     UUID        REFERENCES users(id) ON DELETE SET NULL,
    source_ip   TEXT        NOT NULL DEFAULT '',
    user_agent  TEXT        NOT NULL DEFAULT '',
    request_id  TEXT        NOT NULL DEFAULT '',
    trace_id    TEXT        NOT NULL DEFAULT '',
    attrs       JSONB       NOT NULL DEFAULT '{}'
);
CREATE INDEX audit_log_event_time_idx ON audit_log (event_time DESC);
CREATE INDEX audit_log_user_id_idx    ON audit_log (user_id);

-- +goose Down

DROP TABLE IF EXISTS audit_log;
