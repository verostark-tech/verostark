CREATE TABLE detection_progress (
    statement_id    BIGINT PRIMARY KEY,
    org_id          TEXT NOT NULL,
    phase           TEXT NOT NULL DEFAULT 'reading',
    works_total     INT  NOT NULL DEFAULT 0,
    works_checked   INT  NOT NULL DEFAULT 0,
    flag_count      INT  NOT NULL DEFAULT 0,
    unmatched_count INT  NOT NULL DEFAULT 0,
    error           TEXT,
    updated_at      TIMESTAMPTZ NOT NULL DEFAULT NOW()
);
