package api_test

import (
	"context"
	"encoding/json"
	"errors"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/stretchr/testify/require"

	"github.com/charliewilco/argus/internal/api"
	"github.com/charliewilco/argus/internal/connections"
	"github.com/charliewilco/argus/internal/dlq"
	"github.com/charliewilco/argus/internal/envelope"
	"github.com/charliewilco/argus/internal/oauth"
	"github.com/charliewilco/argus/internal/pipeline"
	"github.com/charliewilco/argus/internal/queue"
	"github.com/charliewilco/argus/internal/store"
	"github.com/charliewilco/argus/internal/triggers"
	"github.com/charliewilco/argus/providers"
)

type oauthStub struct{}

func (oauthStub) BeginAuth(context.Context, *oauth.Config, oauth.AuthorizationRequest) (*oauth.AuthorizationSession, error) {
	return nil, errors.New("unexpected call")
}

func (oauthStub) Exchange(context.Context, *oauth.Config, string, string) (*oauth.ExchangeResult, error) {
	return nil, errors.New("unexpected call")
}

type providerRegistryStub struct {
	provider providers.Provider
}

func (s providerRegistryStub) Get(_ string) (providers.Provider, error) {
	return s.provider, nil
}

type providerStub struct {
	event envelope.Event
}

func (providerStub) ID() string { return "github" }

func (providerStub) OAuthConfig() oauth.Config { return oauth.Config{} }

func (p providerStub) ParseWebhookEvent(_ *http.Request) (envelope.Event, error) {
	return p.event, nil
}

func (providerStub) ExecuteAction(context.Context, *oauth.Token, providers.ActionRequest) (providers.ActionResult, error) {
	return providers.ActionResult{}, errors.New("unexpected call")
}

type pipelineStoreStub struct {
	pipelines []*pipeline.Pipeline
}

func (s pipelineStoreStub) SavePipeline(context.Context, *pipeline.Pipeline) error { return nil }

func (s pipelineStoreStub) ListPipelines(context.Context, string) ([]*pipeline.Pipeline, error) {
	return s.pipelines, nil
}

type eventStoreStub struct {
	event *envelope.Event
}

func (s *eventStoreStub) SaveEvent(_ context.Context, event *envelope.Event) error {
	copy := *event
	s.event = &copy
	return nil
}

type connectionServiceStub struct {
	listTenantID       string
	listProviderID     string
	deleteTenantID     string
	deleteConnectionID string
	connections []connections.Connection
	deletedIDs  []string
}

func (s *connectionServiceStub) ListConnections(_ context.Context, tenantID, providerID string) ([]connections.Connection, error) {
	s.listTenantID = tenantID
	s.listProviderID = providerID
	filtered := make([]connections.Connection, 0, len(s.connections))
	for _, connection := range s.connections {
		if connection.TenantID != tenantID {
			continue
		}
		if providerID != "" && connection.Provider != providerID {
			continue
		}
		filtered = append(filtered, connection)
	}
	return filtered, nil
}

func (s *connectionServiceStub) DeleteConnection(_ context.Context, tenantID, id string) error {
	s.deleteTenantID = tenantID
	s.deleteConnectionID = id
	s.deletedIDs = append(s.deletedIDs, id)
	for _, connection := range s.connections {
		if connection.TenantID == tenantID && connection.ConnectionID == id {
			return nil
		}
	}
	return store.ErrNotFound
}

type queueRecorder struct {
	enqueued []*queue.Job
}

func (q *queueRecorder) Enqueue(_ context.Context, job *queue.Job) error {
	q.enqueued = append(q.enqueued, job)
	return nil
}

func (q *queueRecorder) Dequeue(context.Context) (*queue.Job, error) {
	return nil, errors.New("unexpected call")
}

func (q *queueRecorder) Ack(context.Context, string) error { return nil }

func (q *queueRecorder) Nack(context.Context, string, string) error { return nil }

type dlqStub struct{}

func (dlqStub) List(context.Context) ([]dlq.FailedJob, error) { return nil, nil }

func (dlqStub) Replay(context.Context, string) error { return nil }

type noOpMatcherStub struct{}

func (noOpMatcherStub) Match(context.Context, envelope.Event) ([]pipeline.Pipeline, error) { return nil, nil }

func TestWebhookSkipsPipelinesBoundToOtherConnections(t *testing.T) {
	t.Parallel()

	events := &eventStoreStub{}
	queues := &queueRecorder{}
	matcher, err := triggers.NewTriggerMatcher(pipelineStoreStub{
		pipelines: []*pipeline.Pipeline{
			{
				ID:           "pipe_1",
				TenantID:     "tenant_1",
				ConnectionID: "conn_other",
				Enabled:      true,
				Trigger: pipeline.Trigger{
					Key: "github.push",
				},
			},
		},
	})
	require.NoError(t, err)

	handler, err := api.NewRouter(api.RouterOptions{
		BaseURL:     "http://localhost:8080",
		TenantID:    "tenant_1",
		OAuth:       oauthStub{},
		Providers:   providerRegistryStub{provider: providerStub{event: envelope.Event{ID: "evt_1", TriggerKey: "github.push"}}},
		Connections: &connectionServiceStub{},
		Pipelines:   pipelineStoreStub{},
		Events:      events,
		Matcher:     matcher,
		Queue:       queues,
		DLQ:         dlqStub{},
	})
	require.NoError(t, err)

	request := httptest.NewRequest(http.MethodPost, "/webhooks/github/conn_1", strings.NewReader(`{"ref":"refs/heads/main"}`))
	recorder := httptest.NewRecorder()

	handler.ServeHTTP(recorder, request)

	require.Equal(t, http.StatusAccepted, recorder.Code)
	require.Empty(t, queues.enqueued, "webhooks for a different connection should not enqueue jobs")
	require.NotNil(t, events.event)
	require.Equal(t, "tenant_1", events.event.TenantID)
	require.Equal(t, "github", events.event.Provider)
	require.Equal(t, "conn_1", events.event.ConnectionID)

	var payload map[string]any
	require.NoError(t, json.Unmarshal(recorder.Body.Bytes(), &payload))
	require.Equal(t, float64(0), payload["matched_pipelines"])
}

func TestListConnectionsIsTenantScoped(t *testing.T) {
	t.Parallel()

	connectionService := &connectionServiceStub{
		connections: []connections.Connection{
			{TenantID: "tenant_1", ConnectionID: "conn_1", Provider: "github"},
			{TenantID: "tenant_2", ConnectionID: "conn_2", Provider: "github"},
		},
	}

	handler, err := api.NewRouter(api.RouterOptions{
		BaseURL:  "http://localhost:8080",
		TenantID: "tenant_1",
		OAuth:    oauthStub{},
		Providers: providerRegistryStub{
			provider: providerStub{},
		},
		Connections: connectionService,
		Pipelines: pipelineStoreStub{},
		Events:    &eventStoreStub{},
		Matcher:   noOpMatcherStub{},
		Queue:     &queueRecorder{},
		DLQ:       dlqStub{},
	})
	require.NoError(t, err)

	request := httptest.NewRequest(http.MethodGet, "/connections?provider=github", nil)
	recorder := httptest.NewRecorder()

	handler.ServeHTTP(recorder, request)

	require.Equal(t, http.StatusOK, recorder.Code)
	require.Equal(t, "tenant_1", connectionService.listTenantID)
	require.Equal(t, "github", connectionService.listProviderID)

	var values []connections.Connection
	require.NoError(t, json.Unmarshal(recorder.Body.Bytes(), &values))
	require.Len(t, values, 1)
	require.Equal(t, "tenant_1", values[0].TenantID)
}

func TestDeleteConnectionIsTenantScoped(t *testing.T) {
	t.Parallel()

	connectionService := &connectionServiceStub{
		connections: []connections.Connection{
			{TenantID: "tenant_1", ConnectionID: "conn_1", Provider: "github"},
		},
	}

	handler, err := api.NewRouter(api.RouterOptions{
		BaseURL:     "http://localhost:8080",
		TenantID:    "tenant_1",
		OAuth:       oauthStub{},
		Providers:   providerRegistryStub{provider: providerStub{}},
		Connections: connectionService,
		Pipelines:   pipelineStoreStub{},
		Events:      &eventStoreStub{},
		Matcher:     noOpMatcherStub{},
		Queue:       &queueRecorder{},
		DLQ:         dlqStub{},
	})
	require.NoError(t, err)

	request := httptest.NewRequest(http.MethodDelete, "/connections/conn_2", nil)
	recorder := httptest.NewRecorder()

	handler.ServeHTTP(recorder, request)

	require.Equal(t, http.StatusNotFound, recorder.Code)
	require.Equal(t, "tenant_1", connectionService.deleteTenantID)
	require.Equal(t, "conn_2", connectionService.deleteConnectionID)
}
