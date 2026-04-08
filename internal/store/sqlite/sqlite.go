package sqlite

import (
	"context"
	"database/sql"
	"encoding/json"
	"errors"
	"fmt"
	"strings"
	"time"

	"github.com/golang-migrate/migrate/v4"
	sqlitemigrate "github.com/golang-migrate/migrate/v4/database/sqlite"
	"github.com/golang-migrate/migrate/v4/source/iofs"
	_ "modernc.org/sqlite"

	"github.com/charliewilco/argus/internal/connections"
	"github.com/charliewilco/argus/internal/envelope"
	"github.com/charliewilco/argus/internal/pipeline"
	"github.com/charliewilco/argus/internal/store"
	"github.com/charliewilco/argus/migrations"
)

type Store struct {
	db *sql.DB
}

func Open(ctx context.Context, databaseURL string) (*Store, error) {
	driverName, dsn, err := parseDatabaseURL(databaseURL)
	if err != nil {
		return nil, err
	}

	migrationDB, err := sql.Open(driverName, dsn)
	if err != nil {
		return nil, fmt.Errorf("sqlite.Open: open migration database: %w", err)
	}
	if err := applyMigrations(migrationDB); err != nil {
		_ = migrationDB.Close()
		return nil, fmt.Errorf("sqlite.Open: apply migrations: %w", err)
	}
	if err := migrationDB.Close(); err != nil {
		return nil, fmt.Errorf("sqlite.Open: close migration database: %w", err)
	}

	db, err := sql.Open(driverName, dsn)
	if err != nil {
		return nil, fmt.Errorf("sqlite.Open: open database: %w", err)
	}

	if _, err := db.ExecContext(ctx, "PRAGMA foreign_keys = ON;"); err != nil {
		_ = db.Close()
		return nil, fmt.Errorf("sqlite.Open: enable foreign keys: %w", err)
	}

	return &Store{db: db}, nil
}

func (s *Store) Close() error {
	if s == nil || s.db == nil {
		return nil
	}

	return s.db.Close()
}

func (s *Store) SaveEvent(ctx context.Context, event *envelope.Event) error {
	normalized, err := marshalJSON(event.Normalized)
	if err != nil {
		return fmt.Errorf("sqlite.SaveEvent: marshal normalized: %w", err)
	}

	_, err = s.db.ExecContext(
		ctx,
		`
		INSERT INTO events (
			id, tenant_id, connection_id, provider, trigger_key, raw, normalized, received_at
		) VALUES (?, ?, ?, ?, ?, ?, ?, ?)
		ON CONFLICT(id) DO UPDATE SET
			tenant_id = excluded.tenant_id,
			connection_id = excluded.connection_id,
			provider = excluded.provider,
			trigger_key = excluded.trigger_key,
			raw = excluded.raw,
			normalized = excluded.normalized,
			received_at = excluded.received_at
		`,
		event.ID,
		event.TenantID,
		event.ConnectionID,
		event.Provider,
		event.TriggerKey,
		event.Raw,
		normalized,
		event.ReceivedAt.UTC(),
	)
	if err != nil {
		return fmt.Errorf("sqlite.SaveEvent: %w", err)
	}

	return nil
}

func (s *Store) GetEvent(ctx context.Context, id string) (*envelope.Event, error) {
	row := s.db.QueryRowContext(
		ctx,
		`
		SELECT id, tenant_id, connection_id, provider, trigger_key, raw, normalized, received_at
		FROM events
		WHERE id = ?
		`,
		id,
	)

	event, err := scanEvent(row)
	if err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			return nil, store.ErrNotFound
		}

		return nil, fmt.Errorf("sqlite.GetEvent: %w", err)
	}

	return event, nil
}

func (s *Store) ListEvents(ctx context.Context, tenantID string, filter store.EventFilter) ([]*envelope.Event, error) {
	query := strings.Builder{}
	query.WriteString(`
		SELECT id, tenant_id, connection_id, provider, trigger_key, raw, normalized, received_at
		FROM events
		WHERE tenant_id = ?
	`)

	args := []any{tenantID}

	if filter.ConnectionID != "" {
		query.WriteString(" AND connection_id = ?")
		args = append(args, filter.ConnectionID)
	}
	if filter.Provider != "" {
		query.WriteString(" AND provider = ?")
		args = append(args, filter.Provider)
	}
	if filter.TriggerKey != "" {
		query.WriteString(" AND trigger_key = ?")
		args = append(args, filter.TriggerKey)
	}
	if filter.Since != nil {
		query.WriteString(" AND received_at >= ?")
		args = append(args, filter.Since.UTC())
	}
	if filter.Until != nil {
		query.WriteString(" AND received_at <= ?")
		args = append(args, filter.Until.UTC())
	}

	query.WriteString(" ORDER BY received_at DESC")
	if filter.Limit > 0 {
		query.WriteString(" LIMIT ?")
		args = append(args, filter.Limit)
	}

	rows, err := s.db.QueryContext(ctx, query.String(), args...)
	if err != nil {
		return nil, fmt.Errorf("sqlite.ListEvents: %w", err)
	}
	defer rows.Close()

	events := make([]*envelope.Event, 0)
	for rows.Next() {
		event, scanErr := scanEvent(rows)
		if scanErr != nil {
			return nil, fmt.Errorf("sqlite.ListEvents: scan event: %w", scanErr)
		}
		events = append(events, event)
	}

	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("sqlite.ListEvents: rows: %w", err)
	}

	return events, nil
}

func (s *Store) SaveConnection(ctx context.Context, connection *connections.Connection) error {
	configJSON, err := marshalJSON(connection.Config)
	if err != nil {
		return fmt.Errorf("sqlite.SaveConnection: marshal config: %w", err)
	}

	createdAt := connection.CreatedAt.UTC()
	if createdAt.IsZero() {
		createdAt = time.Now().UTC()
	}

	_, err = s.db.ExecContext(
		ctx,
		`
		INSERT INTO connections (
			tenant_id, connection_id, provider, encrypted_token, config_json, created_at
		) VALUES (?, ?, ?, ?, ?, ?)
		ON CONFLICT(tenant_id, connection_id) DO UPDATE SET
			provider = excluded.provider,
			config_json = excluded.config_json,
			created_at = excluded.created_at
		`,
		connection.TenantID,
		connection.ConnectionID,
		connection.Provider,
		[]byte{},
		configJSON,
		createdAt,
	)
	if err != nil {
		return fmt.Errorf("sqlite.SaveConnection: %w", err)
	}

	return nil
}

func (s *Store) EnsureConnection(ctx context.Context, tenantID, connectionID, provider string, createdAt time.Time) error {
	if tenantID == "" || connectionID == "" || provider == "" {
		return fmt.Errorf("sqlite.EnsureConnection: tenant ID, connection ID, and provider are required")
	}

	ts := createdAt.UTC()
	if ts.IsZero() {
		ts = time.Now().UTC()
	}

	_, err := s.db.ExecContext(
		ctx,
		`
		INSERT INTO connections (
			tenant_id, connection_id, provider, encrypted_token, config_json, created_at
		) VALUES (?, ?, ?, ?, ?, ?)
		ON CONFLICT(tenant_id, connection_id) DO UPDATE SET
			provider = excluded.provider
		`,
		tenantID,
		connectionID,
		provider,
		[]byte{},
		`{}`,
		ts,
	)
	if err != nil {
		return fmt.Errorf("sqlite.EnsureConnection: %w", err)
	}

	return nil
}

func (s *Store) GetConnection(ctx context.Context, tenantID, connectionID string) (*connections.Connection, error) {
	row := s.db.QueryRowContext(
		ctx,
		`
		SELECT tenant_id, connection_id, provider, encrypted_token, config_json, created_at
		FROM connections
		WHERE tenant_id = ? AND connection_id = ?
		`,
		tenantID,
		connectionID,
	)

	connection, err := scanConnection(row)
	if err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			return nil, store.ErrNotFound
		}

		return nil, fmt.Errorf("sqlite.GetConnection: %w", err)
	}

	return connection, nil
}

func (s *Store) GetConnectionByID(ctx context.Context, connectionID string) (*connections.Connection, error) {
	row := s.db.QueryRowContext(
		ctx,
		`
		SELECT tenant_id, connection_id, provider, encrypted_token, config_json, created_at
		FROM connections
		WHERE connection_id = ?
		`,
		connectionID,
	)

	connection, err := scanConnection(row)
	if err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			return nil, store.ErrNotFound
		}

		return nil, fmt.Errorf("sqlite.GetConnectionByID: %w", err)
	}

	return connection, nil
}

func (s *Store) ListConnections(ctx context.Context, tenantID, providerID string) ([]*connections.Connection, error) {
	query := strings.Builder{}
	query.WriteString(`
		SELECT tenant_id, connection_id, provider, encrypted_token, config_json, created_at
		FROM connections
		WHERE 1 = 1
	`)

	args := make([]any, 0, 2)
	if tenantID != "" {
		query.WriteString(" AND tenant_id = ?")
		args = append(args, tenantID)
	}
	if providerID != "" {
		query.WriteString(" AND provider = ?")
		args = append(args, providerID)
	}
	query.WriteString(" ORDER BY created_at DESC, connection_id ASC")

	rows, err := s.db.QueryContext(ctx, query.String(), args...)
	if err != nil {
		return nil, fmt.Errorf("sqlite.ListConnections: %w", err)
	}
	defer rows.Close()

	connectionsList := make([]*connections.Connection, 0)
	for rows.Next() {
		connection, scanErr := scanConnection(rows)
		if scanErr != nil {
			return nil, fmt.Errorf("sqlite.ListConnections: scan connection: %w", scanErr)
		}
		connectionsList = append(connectionsList, connection)
	}

	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("sqlite.ListConnections: rows: %w", err)
	}

	return connectionsList, nil
}

func (s *Store) DeleteConnection(ctx context.Context, tenantID, connectionID string) error {
	result, err := s.db.ExecContext(
		ctx,
		`DELETE FROM connections WHERE tenant_id = ? AND connection_id = ?`,
		tenantID,
		connectionID,
	)
	if err != nil {
		return fmt.Errorf("sqlite.DeleteConnection: %w", err)
	}

	affected, err := result.RowsAffected()
	if err != nil {
		return fmt.Errorf("sqlite.DeleteConnection: rows affected: %w", err)
	}
	if affected == 0 {
		return store.ErrNotFound
	}

	return nil
}

func (s *Store) SaveConnectionSecret(ctx context.Context, secret store.ConnectionSecret) error {
	result, err := s.db.ExecContext(
		ctx,
		`
		UPDATE connections
		SET encrypted_token = ?, created_at = created_at
		WHERE tenant_id = ? AND connection_id = ?
		`,
		secret.Ciphertext,
		secret.TenantID,
		secret.ConnectionID,
	)
	if err != nil {
		return fmt.Errorf("sqlite.SaveConnectionSecret: %w", err)
	}

	rowsAffected, err := result.RowsAffected()
	if err != nil {
		return fmt.Errorf("sqlite.SaveConnectionSecret: rows affected: %w", err)
	}
	if rowsAffected == 0 {
		return store.ErrNotFound
	}

	return nil
}

func (s *Store) GetConnectionSecret(ctx context.Context, tenantID, connectionID string) (*store.ConnectionSecret, error) {
	row := s.db.QueryRowContext(
		ctx,
		`
		SELECT tenant_id, connection_id, encrypted_token, created_at
		FROM connections
		WHERE tenant_id = ? AND connection_id = ?
		`,
		tenantID,
		connectionID,
	)

	var secret store.ConnectionSecret
	if err := row.Scan(&secret.TenantID, &secret.ConnectionID, &secret.Ciphertext, &secret.UpdatedAt); err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			return nil, store.ErrNotFound
		}

		return nil, fmt.Errorf("sqlite.GetConnectionSecret: %w", err)
	}

	return &secret, nil
}

func (s *Store) SaveOAuthState(ctx context.Context, state store.OAuthState) error {
	createdAt := state.CreatedAt.UTC()
	if createdAt.IsZero() {
		createdAt = time.Now().UTC()
	}

	_, err := s.db.ExecContext(
		ctx,
		`
		INSERT INTO oauth_states (
			state_key, tenant_id, connection_id, provider, code_verifier, expires_at, created_at
		) VALUES (?, ?, ?, ?, ?, ?, ?)
		ON CONFLICT(state_key) DO UPDATE SET
			tenant_id = excluded.tenant_id,
			connection_id = excluded.connection_id,
			provider = excluded.provider,
			code_verifier = excluded.code_verifier,
			expires_at = excluded.expires_at,
			created_at = excluded.created_at
		`,
		state.ID,
		state.TenantID,
		state.ConnectionID,
		state.Provider,
		state.CodeVerifier,
		state.ExpiresAt.UTC(),
		createdAt,
	)
	if err != nil {
		return fmt.Errorf("sqlite.SaveOAuthState: %w", err)
	}

	return nil
}

func (s *Store) GetOAuthState(ctx context.Context, key string) (*store.OAuthState, error) {
	row := s.db.QueryRowContext(
		ctx,
		`
		SELECT state_key, tenant_id, connection_id, provider, code_verifier, expires_at, created_at
		FROM oauth_states
		WHERE state_key = ?
		`,
		key,
	)

	var state store.OAuthState
	if err := row.Scan(
		&state.ID,
		&state.TenantID,
		&state.ConnectionID,
		&state.Provider,
		&state.CodeVerifier,
		&state.ExpiresAt,
		&state.CreatedAt,
	); err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			return nil, store.ErrNotFound
		}

		return nil, fmt.Errorf("sqlite.GetOAuthState: %w", err)
	}

	state.ExpiresAt = state.ExpiresAt.UTC()
	state.CreatedAt = state.CreatedAt.UTC()

	return &state, nil
}

func (s *Store) DeleteOAuthState(ctx context.Context, key string) error {
	result, err := s.db.ExecContext(ctx, `DELETE FROM oauth_states WHERE state_key = ?`, key)
	if err != nil {
		return fmt.Errorf("sqlite.DeleteOAuthState: %w", err)
	}

	affected, err := result.RowsAffected()
	if err != nil {
		return fmt.Errorf("sqlite.DeleteOAuthState: rows affected: %w", err)
	}
	if affected == 0 {
		return store.ErrNotFound
	}

	return nil
}

func (s *Store) SavePipeline(ctx context.Context, value *pipeline.Pipeline) error {
	if value == nil {
		return fmt.Errorf("sqlite.SavePipeline: pipeline is required")
	}

	value.Normalize()
	if !value.HasExplicitEnabled() && !value.Enabled {
		enabled, err := s.resolvePipelineEnabled(ctx, value.ID)
		if err != nil {
			return err
		}
		value.Enabled = enabled
	}

	triggerJSON, err := marshalJSON(value.Trigger)
	if err != nil {
		return fmt.Errorf("sqlite.SavePipeline: marshal trigger: %w", err)
	}

	if !value.HasExplicitEnabled() && !value.Enabled {
		enabled, err := s.resolvePipelineEnabled(ctx, value.ID)
		if err != nil {
			return err
		}
		value.Enabled = enabled
	}

	stepsJSON, err := marshalJSON(value.Steps)
	if err != nil {
		return fmt.Errorf("sqlite.SavePipeline: marshal steps: %w", err)
	}

	_, err = s.db.ExecContext(
		ctx,
		`
		INSERT INTO pipelines (
			id, tenant_id, name, trigger_key, trigger_json, connection_id, steps_json, enabled
		) VALUES (?, ?, ?, ?, ?, ?, ?, ?)
		ON CONFLICT(id) DO UPDATE SET
			tenant_id = excluded.tenant_id,
			name = excluded.name,
			trigger_key = excluded.trigger_key,
			trigger_json = excluded.trigger_json,
			connection_id = excluded.connection_id,
			steps_json = excluded.steps_json,
			enabled = excluded.enabled
		`,
		value.ID,
		value.TenantID,
		value.Name,
		value.TriggerKey,
		triggerJSON,
		value.ConnectionID,
		stepsJSON,
		value.Enabled,
	)
	if err != nil {
		return fmt.Errorf("sqlite.SavePipeline: %w", err)
	}

	return nil
}

func (s *Store) resolvePipelineEnabled(ctx context.Context, id string) (bool, error) {
	if id == "" {
		return true, nil
	}

	row := s.db.QueryRowContext(ctx, `SELECT enabled FROM pipelines WHERE id = ?`, id)

	var enabled bool
	if err := row.Scan(&enabled); err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			return true, nil
		}
		return false, fmt.Errorf("sqlite.SavePipeline: load existing enabled state: %w", err)
	}

	return enabled, nil
}

func (s *Store) GetPipeline(ctx context.Context, id string) (*pipeline.Pipeline, error) {
	row := s.db.QueryRowContext(
		ctx,
		`
		SELECT id, tenant_id, name, trigger_key, trigger_json, connection_id, steps_json, enabled
		FROM pipelines
		WHERE id = ?
		`,
		id,
	)

	value, err := scanPipeline(row)
	if err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			return nil, store.ErrNotFound
		}

		return nil, fmt.Errorf("sqlite.GetPipeline: %w", err)
	}

	return value, nil
}

func (s *Store) ListPipelines(ctx context.Context, tenantID string) ([]*pipeline.Pipeline, error) {
	query := strings.Builder{}
	query.WriteString(`
		SELECT id, tenant_id, name, trigger_key, trigger_json, connection_id, steps_json, enabled
		FROM pipelines
	`)

	args := make([]any, 0, 1)
	if tenantID != "" {
		query.WriteString(" WHERE tenant_id = ?")
		args = append(args, tenantID)
	}
	query.WriteString(" ORDER BY id ASC")

	rows, err := s.db.QueryContext(ctx, query.String(), args...)
	if err != nil {
		return nil, fmt.Errorf("sqlite.ListPipelines: %w", err)
	}
	defer rows.Close()

	pipelines := make([]*pipeline.Pipeline, 0)
	for rows.Next() {
		value, scanErr := scanPipeline(rows)
		if scanErr != nil {
			return nil, fmt.Errorf("sqlite.ListPipelines: scan pipeline: %w", scanErr)
		}
		pipelines = append(pipelines, value)
	}

	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("sqlite.ListPipelines: rows: %w", err)
	}

	return pipelines, nil
}

func (s *Store) PushFailedJob(ctx context.Context, job store.FailedJob) error {
	_, err := s.db.ExecContext(
		ctx,
		`
		INSERT INTO dead_letter_jobs (
			id, job_type, payload, reason, attempt_count, failed_at, replayed_at
		) VALUES (?, ?, ?, ?, ?, ?, ?)
		ON CONFLICT(id) DO UPDATE SET
			job_type = excluded.job_type,
			payload = excluded.payload,
			reason = excluded.reason,
			attempt_count = excluded.attempt_count,
			failed_at = excluded.failed_at,
			replayed_at = excluded.replayed_at
		`,
		job.ID,
		job.JobType,
		string(job.Payload),
		job.Reason,
		job.AttemptCount,
		job.FailedAt.UTC(),
		job.ReplayedAt,
	)
	if err != nil {
		return fmt.Errorf("sqlite.PushFailedJob: %w", err)
	}

	return nil
}

func (s *Store) GetFailedJob(ctx context.Context, id string) (*store.FailedJob, error) {
	row := s.db.QueryRowContext(
		ctx,
		`
		SELECT id, job_type, payload, reason, attempt_count, failed_at, replayed_at
		FROM dead_letter_jobs
		WHERE id = ?
		`,
		id,
	)

	job, err := scanFailedJob(row)
	if err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			return nil, store.ErrNotFound
		}

		return nil, fmt.Errorf("sqlite.GetFailedJob: %w", err)
	}

	return job, nil
}

func (s *Store) ListFailedJobs(ctx context.Context) ([]*store.FailedJob, error) {
	rows, err := s.db.QueryContext(
		ctx,
		`
		SELECT id, job_type, payload, reason, attempt_count, failed_at, replayed_at
		FROM dead_letter_jobs
		ORDER BY failed_at DESC, id ASC
		`,
	)
	if err != nil {
		return nil, fmt.Errorf("sqlite.ListFailedJobs: %w", err)
	}
	defer rows.Close()

	jobs := make([]*store.FailedJob, 0)
	for rows.Next() {
		job, scanErr := scanFailedJob(rows)
		if scanErr != nil {
			return nil, fmt.Errorf("sqlite.ListFailedJobs: scan failed job: %w", scanErr)
		}
		jobs = append(jobs, job)
	}

	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("sqlite.ListFailedJobs: rows: %w", err)
	}

	return jobs, nil
}

func (s *Store) MarkFailedJobReplayed(ctx context.Context, id string, replayedAt time.Time) error {
	result, err := s.db.ExecContext(
		ctx,
		`UPDATE dead_letter_jobs SET replayed_at = ? WHERE id = ?`,
		replayedAt.UTC(),
		id,
	)
	if err != nil {
		return fmt.Errorf("sqlite.MarkFailedJobReplayed: %w", err)
	}

	affected, err := result.RowsAffected()
	if err != nil {
		return fmt.Errorf("sqlite.MarkFailedJobReplayed: rows affected: %w", err)
	}
	if affected == 0 {
		return store.ErrNotFound
	}

	return nil
}

func (s *Store) DeleteFailedJob(ctx context.Context, id string) error {
	result, err := s.db.ExecContext(ctx, `DELETE FROM dead_letter_jobs WHERE id = ?`, id)
	if err != nil {
		return fmt.Errorf("sqlite.DeleteFailedJob: %w", err)
	}

	affected, err := result.RowsAffected()
	if err != nil {
		return fmt.Errorf("sqlite.DeleteFailedJob: rows affected: %w", err)
	}
	if affected == 0 {
		return store.ErrNotFound
	}

	return nil
}

type rowScanner interface {
	Scan(dest ...any) error
}

func scanEvent(scanner rowScanner) (*envelope.Event, error) {
	var raw struct {
		ID           string
		TenantID     string
		ConnectionID string
		Provider     string
		TriggerKey   string
		Raw          []byte
		Normalized   string
		ReceivedAt   time.Time
	}

	if err := scanner.Scan(
		&raw.ID,
		&raw.TenantID,
		&raw.ConnectionID,
		&raw.Provider,
		&raw.TriggerKey,
		&raw.Raw,
		&raw.Normalized,
		&raw.ReceivedAt,
	); err != nil {
		return nil, err
	}

	event := &envelope.Event{
		ID:           raw.ID,
		TenantID:     raw.TenantID,
		ConnectionID: raw.ConnectionID,
		Provider:     raw.Provider,
		TriggerKey:   raw.TriggerKey,
		Raw:          raw.Raw,
		ReceivedAt:   raw.ReceivedAt.UTC(),
	}

	if err := unmarshalJSON(raw.Normalized, &event.Normalized); err != nil {
		return nil, err
	}
	if event.Normalized == nil {
		event.Normalized = map[string]any{}
	}

	return event, nil
}

func scanConnection(scanner rowScanner) (*connections.Connection, error) {
	var raw struct {
		TenantID     string
		ConnectionID string
		Provider     string
		ConfigJSON   string
		CreatedAt    time.Time
	}

	if err := scanner.Scan(
		&raw.TenantID,
		&raw.ConnectionID,
		&raw.Provider,
		new([]byte),
		&raw.ConfigJSON,
		&raw.CreatedAt,
	); err != nil {
		return nil, err
	}

	connection := &connections.Connection{
		TenantID:     raw.TenantID,
		ConnectionID: raw.ConnectionID,
		Provider:     raw.Provider,
		CreatedAt:    raw.CreatedAt.UTC(),
	}

	if err := unmarshalJSON(raw.ConfigJSON, &connection.Config); err != nil {
		return nil, err
	}
	if connection.Config == nil {
		connection.Config = map[string]any{}
	}

	return connection, nil
}

func scanPipeline(scanner rowScanner) (*pipeline.Pipeline, error) {
	var raw struct {
		ID           string
		TenantID     string
		Name         string
		TriggerKey   string
		TriggerJSON  string
		ConnectionID string
		StepsJSON    string
		Enabled      bool
	}

	if err := scanner.Scan(
		&raw.ID,
		&raw.TenantID,
		&raw.Name,
		&raw.TriggerKey,
		&raw.TriggerJSON,
		&raw.ConnectionID,
		&raw.StepsJSON,
		&raw.Enabled,
	); err != nil {
		return nil, err
	}

	value := &pipeline.Pipeline{
		ID:           raw.ID,
		TenantID:     raw.TenantID,
		Name:         raw.Name,
		TriggerKey:   raw.TriggerKey,
		ConnectionID: raw.ConnectionID,
	}
	value.SetEnabled(raw.Enabled)

	if err := unmarshalJSON(raw.TriggerJSON, &value.Trigger); err != nil {
		return nil, err
	}
	if err := unmarshalJSON(raw.StepsJSON, &value.Steps); err != nil {
		return nil, err
	}
	value.Normalize()

	return value, nil
}

func scanFailedJob(scanner rowScanner) (*store.FailedJob, error) {
	var job store.FailedJob
	var payload string

	if err := scanner.Scan(
		&job.ID,
		&job.JobType,
		&payload,
		&job.Reason,
		&job.AttemptCount,
		&job.FailedAt,
		&job.ReplayedAt,
	); err != nil {
		return nil, err
	}

	job.Payload = []byte(payload)
	job.FailedAt = job.FailedAt.UTC()
	if job.ReplayedAt != nil {
		replayedAt := job.ReplayedAt.UTC()
		job.ReplayedAt = &replayedAt
	}

	return &job, nil
}

func marshalJSON(value any) (string, error) {
	if value == nil {
		return "null", nil
	}

	data, err := json.Marshal(value)
	if err != nil {
		return "", err
	}

	return string(data), nil
}

func unmarshalJSON[T any](value string, target *T) error {
	if value == "" || value == "null" {
		return nil
	}

	return json.Unmarshal([]byte(value), target)
}

func parseDatabaseURL(databaseURL string) (driverName string, dsn string, err error) {
	const prefix = "sqlite:"
	if !strings.HasPrefix(databaseURL, prefix) {
		return "", "", fmt.Errorf("sqlite.parseDatabaseURL: unsupported database url %q", databaseURL)
	}

	dsn = strings.TrimPrefix(databaseURL, prefix)
	if dsn == "" {
		return "", "", fmt.Errorf("sqlite.parseDatabaseURL: empty sqlite dsn")
	}

	return "sqlite", dsn, nil
}

func applyMigrations(db *sql.DB) error {
	driver, err := sqlitemigrate.WithInstance(db, &sqlitemigrate.Config{})
	if err != nil {
		return fmt.Errorf("sqlite.applyMigrations: create driver: %w", err)
	}

	source, err := iofs.New(migrations.Files, ".")
	if err != nil {
		return fmt.Errorf("sqlite.applyMigrations: create source: %w", err)
	}

	migrator, err := migrate.NewWithInstance("iofs", source, "sqlite", driver)
	if err != nil {
		return fmt.Errorf("sqlite.applyMigrations: create migrator: %w", err)
	}
	defer func() {
		_, _ = migrator.Close()
	}()

	if err := migrator.Up(); err != nil && !errors.Is(err, migrate.ErrNoChange) {
		return fmt.Errorf("sqlite.applyMigrations: run up migrations: %w", err)
	}

	return nil
}
