ALTER TABLE statement_lines
    ADD COLUMN controlled_share NUMERIC(7, 4) NOT NULL DEFAULT 0,
    ADD COLUMN work_title       TEXT          NOT NULL DEFAULT '';
