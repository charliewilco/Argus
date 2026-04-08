CREATE TABLE IF NOT EXISTS events (
	id TEXT PRIMARY KEY,
	tenant_id TEXT NOT NULL,
	connection_id TEXT NOT NULL,
	provider TEXT NOT NULL,
	trigger_key TEXT NOT NULL,
	raw BLOB NOT NULL,
	normalized TEXT NOT NULL DEFAULT '{}',
	received_at TIMESTAMP NOT NULL
);

CREATE INDEX IF NOT EXISTS idx_events_tenant_received_at
	ON events (tenant_id, received_at DESC);

CREATE INDEX IF NOT EXISTS idx_events_lookup
	ON events (tenant_id, connection_id, provider, trigger_key);

CREATE TABLE IF NOT EXISTS connections (
	tenant_id TEXT NOT NULL,
	connection_id TEXT NOT NULL,
	provider TEXT NOT NULL,
	encrypted_token BLOB NOT NULL DEFAULT X'',
	config_json TEXT NOT NULL DEFAULT '{}',
	created_at TIMESTAMP NOT NULL,
	PRIMARY KEY (tenant_id, connection_id)
);

CREATE INDEX IF NOT EXISTS idx_connections_provider
	ON connections (tenant_id, provider);

CREATE TABLE IF NOT EXISTS oauth_states (
	state_key TEXT PRIMARY KEY,
	tenant_id TEXT NOT NULL,
	connection_id TEXT NOT NULL,
	provider TEXT NOT NULL,
	code_verifier TEXT NOT NULL,
	expires_at TIMESTAMP NOT NULL,
	created_at TIMESTAMP NOT NULL
);

CREATE INDEX IF NOT EXISTS idx_oauth_states_expiry
	ON oauth_states (expires_at);

CREATE TABLE IF NOT EXISTS pipelines (
	id TEXT PRIMARY KEY,
	tenant_id TEXT NOT NULL,
	name TEXT NOT NULL,
	trigger_key TEXT NOT NULL,
	connection_id TEXT NOT NULL,
	steps_json TEXT NOT NULL DEFAULT '[]',
	enabled BOOLEAN NOT NULL DEFAULT 1
);

CREATE INDEX IF NOT EXISTS idx_pipelines_tenant
	ON pipelines (tenant_id, trigger_key);
