-- work_id on detection_flags was always NULL — no catalogue lookup in V0.1.
ALTER TABLE detection_flags DROP COLUMN IF EXISTS work_id;
