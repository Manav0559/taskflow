-- The single-column index on job_id let Postgres find a job's runs but every read
-- path (ListJobRuns, LatestRunForJob, LatestRunsForJobs) still sorted them by
-- created_at afterward. A composite index lets it walk straight to the newest rows
-- for a job_id instead of scan-then-sort - this starts to matter once a job (e.g. a
-- tight cron schedule) accumulates a long run history. The leading column (job_id)
-- still serves any query that only filters on job_id, so the old single-column
-- index is now redundant and dropped rather than doubling write-time index upkeep.
DROP INDEX IF EXISTS idx_job_runs_job_id;
CREATE INDEX IF NOT EXISTS idx_job_runs_job_id_created_at ON job_runs (job_id, created_at DESC);
