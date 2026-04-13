CREATE TABLE organisations (
    id   TEXT PRIMARY KEY,  -- Clerk org ID (e.g. org_xxx)
    name TEXT NOT NULL,
    pro  TEXT NOT NULL DEFAULT 'STIM',
    created_at TIMESTAMPTZ NOT NULL DEFAULT NOW()
);
