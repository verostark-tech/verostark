CREATE TABLE detection_runs (
    id           BIGSERIAL PRIMARY KEY,
    org_id       TEXT    NOT NULL,
    statement_id BIGINT  NOT NULL,
    status       TEXT    NOT NULL DEFAULT 'pending',
    flag_count   INT     NOT NULL DEFAULT 0,
    started_at   TIMESTAMPTZ,
    completed_at TIMESTAMPTZ,
    created_at   TIMESTAMPTZ NOT NULL DEFAULT NOW()
);

CREATE TABLE detection_flags (
    id                BIGSERIAL PRIMARY KEY,
    org_id            TEXT           NOT NULL,
    detection_run_id  BIGINT         NOT NULL REFERENCES detection_runs(id),
    statement_line_id BIGINT,
    work_id           BIGINT,
    expected_amount   NUMERIC(15, 4),
    received_amount   NUMERIC(15, 4),
    deviation_amount  NUMERIC(15, 4),
    deviation_pct     NUMERIC(7,  4),
    severity          TEXT           NOT NULL,
    pattern_type      TEXT           NOT NULL,
    explanation       TEXT           NOT NULL,
    recommendation    TEXT           NOT NULL,
    status            TEXT           NOT NULL DEFAULT 'open',
    created_at        TIMESTAMPTZ    NOT NULL DEFAULT NOW()
);

CREATE INDEX detection_flags_run_severity ON detection_flags (detection_run_id, severity);
CREATE INDEX detection_flags_org_status   ON detection_flags (org_id, status);
