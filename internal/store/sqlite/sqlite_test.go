package sqlite_test

import (
	"context"
	"path/filepath"
	"testing"
	"time"

	"github.com/stretchr/testify/require"

	"github.com/charliewilco/argus/internal/connections"
	"github.com/charliewilco/argus/internal/envelope"
	"github.com/charliewilco/argus/internal/pipeline"
	"github.com/charliewilco/argus/internal/store"
	sqlitestore "github.com/charliewilco/argus/internal/store/sqlite"
)

func TestStoreSaveAndGetEvent(t *testing.T) {
	t.Parallel()

	ctx := context.Background()
	sqliteStore := newStore(t)

	event := &envelope.Event{
		ID:           "evt_123",
		TenantID:     "tenant_1",
		ConnectionID: "conn_1",
		Provider:     "github",
		TriggerKey:   "issue.created",
		Raw:          []byte(`{"action":"opened"}`),
		Normalized: map[string]any{
			"title": "Bug report",
		},
		ReceivedAt: time.Date(2026, 4, 7, 12, 0, 0, 0, time.UTC),
	}

	require.NoError(t, sqliteStore.SaveEvent(ctx, event))

	got, err := sqliteStore.GetEvent(ctx, event.ID)
	require.NoError(t, err)
	require.Equal(t, event.ID, got.ID)
	require.Equal(t, event.TenantID, got.TenantID)
	require.Equal(t, event.ConnectionID, got.ConnectionID)
	require.Equal(t, event.Provider, got.Provider)
	require.Equal(t, event.TriggerKey, got.TriggerKey)
	require.Equal(t, event.Raw, got.Raw)
	require.Equal(t, event.Normalized["title"], got.Normalized["title"])
	require.True(t, event.ReceivedAt.Equal(got.ReceivedAt))
}

func TestStoreListEventsRespectsTenantAndLimit(t *testing.T) {
	t.Parallel()

	ctx := context.Background()
	sqliteStore := newStore(t)

	baseTime := time.Date(2026, 4, 7, 12, 0, 0, 0, time.UTC)
	events := []*envelope.Event{
		{
			ID:           "evt_1",
			TenantID:     "tenant_1",
			ConnectionID: "conn_1",
			Provider:     "github",
			TriggerKey:   "issue.created",
			Raw:          []byte(`{}`),
			Normalized:   map[string]any{"sequence": float64(1)},
			ReceivedAt:   baseTime,
		},
		{
			ID:           "evt_2",
			TenantID:     "tenant_1",
			ConnectionID: "conn_1",
			Provider:     "github",
			TriggerKey:   "issue.created",
			Raw:          []byte(`{}`),
			Normalized:   map[string]any{"sequence": float64(2)},
			ReceivedAt:   baseTime.Add(time.Minute),
		},
		{
			ID:           "evt_3",
			TenantID:     "tenant_2",
			ConnectionID: "conn_2",
			Provider:     "github",
			TriggerKey:   "issue.created",
			Raw:          []byte(`{}`),
			Normalized:   map[string]any{"sequence": float64(3)},
			ReceivedAt:   baseTime.Add(2 * time.Minute),
		},
	}

	for _, event := range events {
		require.NoError(t, sqliteStore.SaveEvent(ctx, event))
	}

	got, err := sqliteStore.ListEvents(ctx, "tenant_1", store.EventFilter{Limit: 1})
	require.NoError(t, err)
	require.Len(t, got, 1)
	require.Equal(t, "evt_2", got[0].ID)
}

func TestStoreSaveAndGetConnectionAndPipeline(t *testing.T) {
	t.Parallel()

	ctx := context.Background()
	sqliteStore := newStore(t)

	connection := &connections.Connection{
		TenantID:     "tenant_1",
		ConnectionID: "conn_1",
		Provider:     "github",
		Config: map[string]any{
			"repository": "charliewilco/argus",
		},
		CreatedAt: time.Date(2026, 4, 7, 12, 0, 0, 0, time.UTC),
	}

	require.NoError(t, sqliteStore.SaveConnection(ctx, connection))

	gotConnection, err := sqliteStore.GetConnection(ctx, connection.TenantID, connection.ConnectionID)
	require.NoError(t, err)
	require.Equal(t, connection.Provider, gotConnection.Provider)
	require.Equal(t, connection.Config["repository"], gotConnection.Config["repository"])
	require.Nil(t, gotConnection.Token)

	value := &pipeline.Pipeline{
		ID:           "pipe_1",
		TenantID:     "tenant_1",
		Name:         "New issue to Slack",
		TriggerKey:   "github.issue.created",
		ConnectionID: "conn_1",
		Enabled:      true,
		Steps: []pipeline.Step{
			{
				ID:         "step_1",
				Name:       "Notify Slack",
				Action:     "slack.send_message",
				Connection: "conn_slack",
				Input: map[string]any{
					"text": "{{event.normalized.title}}",
				},
				OnError: pipeline.ErrorBehaviorRetry,
			},
		},
	}

	require.NoError(t, sqliteStore.SavePipeline(ctx, value))

	gotPipeline, err := sqliteStore.GetPipeline(ctx, value.ID)
	require.NoError(t, err)
	require.Equal(t, value.Name, gotPipeline.Name)
	require.Len(t, gotPipeline.Steps, 1)
	require.Equal(t, value.Steps[0].Action, gotPipeline.Steps[0].Action)

	pipelines, err := sqliteStore.ListPipelines(ctx, "tenant_1")
	require.NoError(t, err)
	require.Len(t, pipelines, 1)
}

func TestStoreSaveGetAndDeleteOAuthStateAndSecret(t *testing.T) {
	t.Parallel()

	ctx := context.Background()
	sqliteStore := newStore(t)

	connection := &connections.Connection{
		TenantID:     "tenant_1",
		ConnectionID: "conn_1",
		Provider:     "github",
		Config:       map[string]any{},
		CreatedAt:    time.Date(2026, 4, 7, 12, 0, 0, 0, time.UTC),
	}
	require.NoError(t, sqliteStore.SaveConnection(ctx, connection))

	secret := store.ConnectionSecret{
		TenantID:     "tenant_1",
		ConnectionID: "conn_1",
		Ciphertext:   []byte("ciphertext"),
		UpdatedAt:    time.Date(2026, 4, 7, 12, 1, 0, 0, time.UTC),
	}
	require.NoError(t, sqliteStore.SaveConnectionSecret(ctx, secret))

	gotSecret, err := sqliteStore.GetConnectionSecret(ctx, secret.TenantID, secret.ConnectionID)
	require.NoError(t, err)
	require.Equal(t, secret.Ciphertext, gotSecret.Ciphertext)

	state := store.OAuthState{
		ID:           "state_123",
		TenantID:     "tenant_1",
		ConnectionID: "conn_1",
		Provider:     "github",
		CodeVerifier: "verifier",
		CreatedAt:    time.Date(2026, 4, 7, 12, 0, 0, 0, time.UTC),
		ExpiresAt:    time.Date(2026, 4, 7, 12, 10, 0, 0, time.UTC),
	}

	require.NoError(t, sqliteStore.SaveOAuthState(ctx, state))

	gotState, err := sqliteStore.GetOAuthState(ctx, state.ID)
	require.NoError(t, err)
	require.Equal(t, state.Provider, gotState.Provider)
	require.Equal(t, state.CodeVerifier, gotState.CodeVerifier)

	require.NoError(t, sqliteStore.DeleteOAuthState(ctx, state.ID))

	_, err = sqliteStore.GetOAuthState(ctx, state.ID)
	require.ErrorIs(t, err, store.ErrNotFound)
}

func newStore(t *testing.T) *sqlitestore.Store {
	t.Helper()

	ctx := context.Background()
	path := filepath.Join(t.TempDir(), "argus-test.db")
	sqliteStore, err := sqlitestore.Open(ctx, "sqlite:"+path)
	require.NoError(t, err)
	t.Cleanup(func() {
		require.NoError(t, sqliteStore.Close())
	})

	return sqliteStore
}
