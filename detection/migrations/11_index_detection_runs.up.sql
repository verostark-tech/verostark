-- Speed up the ListFlags subquery:
--   SELECT MAX(id) FROM detection_runs WHERE org_id=$1 GROUP BY statement_id
-- Without this index Postgres full-scans detection_runs on every dashboard load.
CREATE INDEX detection_runs_org_stmt ON detection_runs (org_id, statement_id);
