ALTER TABLE detection_flags
    ADD COLUMN work_title TEXT NOT NULL DEFAULT '',
    ADD COLUMN iswc       TEXT NOT NULL DEFAULT '';
