CREATE TABLE IF NOT EXISTS dead_letter_jobs (
	id TEXT PRIMARY KEY,
	job_type TEXT NOT NULL,
	payload TEXT NOT NULL,
	reason TEXT NOT NULL,
	attempt_count INTEGER NOT NULL DEFAULT 0,
	failed_at TIMESTAMP NOT NULL,
	replayed_at TIMESTAMP
);

CREATE INDEX IF NOT EXISTS idx_dead_letter_jobs_failed_at
	ON dead_letter_jobs (failed_at DESC);

CREATE INDEX IF NOT EXISTS idx_dead_letter_jobs_replayed_at
	ON dead_letter_jobs (replayed_at);
