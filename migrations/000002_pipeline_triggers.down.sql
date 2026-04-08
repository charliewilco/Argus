DROP INDEX IF EXISTS idx_connections_connection_id;

ALTER TABLE pipelines DROP COLUMN trigger_json;
