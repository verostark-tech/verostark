CREATE TABLE works (
    id           BIGSERIAL PRIMARY KEY,
    org_id       TEXT      NOT NULL,
    title        TEXT      NOT NULL,
    iswc         TEXT,
    internal_ref TEXT,
    created_at   TIMESTAMPTZ NOT NULL DEFAULT NOW()
);

CREATE INDEX works_org_iswc ON works (org_id, iswc);

CREATE TABLE writers (
    id              BIGSERIAL PRIMARY KEY,
    org_id          TEXT    NOT NULL,
    name            TEXT    NOT NULL,
    ipi_name_number TEXT,
    ipi_base_number TEXT,
    is_controlled   BOOLEAN NOT NULL DEFAULT FALSE,
    created_at      TIMESTAMPTZ NOT NULL DEFAULT NOW()
);

CREATE TABLE work_writers (
    id               BIGSERIAL PRIMARY KEY,
    org_id           TEXT           NOT NULL,
    work_id          BIGINT         NOT NULL REFERENCES works(id),
    writer_id        BIGINT         NOT NULL REFERENCES writers(id),
    manuscript_share NUMERIC(7, 4)  NOT NULL DEFAULT 0,
    controlled_share NUMERIC(7, 4)  NOT NULL DEFAULT 0,
    created_at       TIMESTAMPTZ    NOT NULL DEFAULT NOW()
);

CREATE TABLE statements (
    id         BIGSERIAL PRIMARY KEY,
    org_id     TEXT NOT NULL,
    filename   TEXT NOT NULL,
    period     TEXT NOT NULL,
    pro        TEXT NOT NULL DEFAULT 'STIM',
    status     TEXT NOT NULL DEFAULT 'pending',
    created_at TIMESTAMPTZ NOT NULL DEFAULT NOW()
);

CREATE TABLE statement_lines (
    id           BIGSERIAL PRIMARY KEY,
    org_id       TEXT           NOT NULL,
    statement_id BIGINT         NOT NULL REFERENCES statements(id),
    work_ref     TEXT,
    iswc         TEXT,
    territory    TEXT,
    right_type   TEXT,
    source       TEXT,
    net_amount   NUMERIC(15, 4) NOT NULL,
    currency     TEXT           NOT NULL DEFAULT 'SEK',
    period       TEXT,
    created_at   TIMESTAMPTZ    NOT NULL DEFAULT NOW()
);

CREATE INDEX statement_lines_stmt_org ON statement_lines (statement_id, org_id);
