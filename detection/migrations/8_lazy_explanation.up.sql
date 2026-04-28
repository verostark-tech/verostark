ALTER TABLE detection_flags
    ADD COLUMN explanation_status TEXT             NOT NULL DEFAULT 'pending',
    ADD COLUMN right_type         TEXT             NOT NULL DEFAULT 'mechanical',
    ADD COLUMN period             TEXT             NOT NULL DEFAULT '',
    ADD COLUMN gross_amount       DOUBLE PRECISION NOT NULL DEFAULT 0,
    ADD COLUMN controlled_share   DOUBLE PRECISION NOT NULL DEFAULT 0,
    ADD COLUMN updated_at         TIMESTAMPTZ      NOT NULL DEFAULT NOW();
