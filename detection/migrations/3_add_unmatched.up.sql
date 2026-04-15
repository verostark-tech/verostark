ALTER TABLE detection_runs ADD COLUMN unmatched_count INT NOT NULL DEFAULT 0;

CREATE TABLE detection_unmatched (
    id               BIGSERIAL PRIMARY KEY,
    org_id           TEXT           NOT NULL,
    detection_run_id BIGINT         NOT NULL REFERENCES detection_runs(id),
    statement_line_id BIGINT        NOT NULL,
    iswc             TEXT,
    work_ref         TEXT,
    right_type       TEXT,
    net_amount       NUMERIC(15, 4),
    period           TEXT,
    reason           TEXT           NOT NULL, -- no_iswc | no_catalogue_match | unknown_right_type
    created_at       TIMESTAMPTZ    NOT NULL DEFAULT NOW()
);

CREATE INDEX detection_unmatched_run ON detection_unmatched (detection_run_id, org_id);
