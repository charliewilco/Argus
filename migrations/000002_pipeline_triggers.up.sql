ALTER TABLE pipelines ADD COLUMN trigger_json TEXT NOT NULL DEFAULT '{}';

CREATE UNIQUE INDEX IF NOT EXISTS idx_connections_connection_id
	ON connections (connection_id);
