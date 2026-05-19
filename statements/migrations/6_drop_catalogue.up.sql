-- works, writers, work_writers were scaffolded for CWR ingestion (Phase 2).
-- They are never written to or read from in V0.1. Dropping them.
DROP TABLE IF EXISTS work_writers;
DROP TABLE IF EXISTS writers;
DROP TABLE IF EXISTS works;
