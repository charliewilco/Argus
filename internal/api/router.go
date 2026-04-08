package api

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"time"

	"github.com/go-chi/chi/v5"
	"github.com/google/uuid"

	"github.com/charliewilco/argus/internal/connections"
	"github.com/charliewilco/argus/internal/dlq"
	"github.com/charliewilco/argus/internal/envelope"
	"github.com/charliewilco/argus/internal/oauth"
	"github.com/charliewilco/argus/internal/pipeline"
	"github.com/charliewilco/argus/internal/queue"
	"github.com/charliewilco/argus/internal/store"
	"github.com/charliewilco/argus/providers"
)

type oauthManager interface {
	BeginAuth(ctx context.Context, cfg *oauth.Config, request oauth.AuthorizationRequest) (*oauth.AuthorizationSession, error)
	Exchange(ctx context.Context, cfg *oauth.Config, code, stateID string) (*oauth.ExchangeResult, error)
}

type providerRegistry interface {
	Get(id string) (providers.Provider, error)
	Metadata() []providers.Metadata
}

type connectionService interface {
	ListConnections(ctx context.Context, tenantID, providerID string) ([]connections.Connection, error)
	DeleteConnection(ctx context.Context, tenantID, id string) error
}

type pipelineStore interface {
	SavePipeline(ctx context.Context, pipeline *pipeline.Pipeline) error
	ListPipelines(ctx context.Context, tenantID string) ([]*pipeline.Pipeline, error)
}

type eventStore interface {
	SaveEvent(ctx context.Context, event *envelope.Event) error
}

type triggerMatcher interface {
	Match(ctx context.Context, event envelope.Event) ([]pipeline.Pipeline, error)
}

type dlqStore interface {
	List(ctx context.Context) ([]dlq.FailedJob, error)
	Replay(ctx context.Context, id string) error
}

type RouterOptions struct {
	BaseURL     string
	TenantID    string
	Now         func() time.Time
	OAuth       oauthManager
	Providers   providerRegistry
	Connections connectionService
	Pipelines   pipelineStore
	Events      eventStore
	Matcher     triggerMatcher
	Queue       queue.Queue
	DLQ         dlqStore
}
type router struct {
	baseURL     string
	tenantID    string
	now         func() time.Time
	oauth       oauthManager
	providers   providerRegistry
	connections connectionService
	pipelines   pipelineStore
	events      eventStore
	matcher     triggerMatcher
	queue       queue.Queue
	dlq         dlqStore
}

type authorizeRequest struct {
	ConnectionID string `json:"connection_id"`
}

func NewRouter(opts RouterOptions) (http.Handler, error) {
	if opts.TenantID == "" {
		return nil, fmt.Errorf("api.NewRouter: tenant ID is required")
	}
	if opts.Now == nil {
		opts.Now = time.Now
	}
	if opts.OAuth == nil || opts.Providers == nil || opts.Connections == nil || opts.Pipelines == nil || opts.Events == nil || opts.Matcher == nil || opts.Queue == nil || opts.DLQ == nil {
		return nil, fmt.Errorf("api.NewRouter: all dependencies are required")
	}

	h := &router{
		baseURL:     opts.BaseURL,
		tenantID:    opts.TenantID,
		now:         opts.Now,
		oauth:       opts.OAuth,
		providers:   opts.Providers,
		connections: opts.Connections,
		pipelines:   opts.Pipelines,
		events:      opts.Events,
		matcher:     opts.Matcher,
		queue:       opts.Queue,
		dlq:         opts.DLQ,
	}

	r := chi.NewRouter()
	r.Get("/healthz", h.healthz)
	r.Post("/oauth/{provider}/authorize", h.authorize)
	r.Get("/oauth/{provider}/callback", h.callback)
	r.Post("/webhooks/{provider}/{connectionID}", h.webhook)
	r.Post("/webhooks/{provider}", h.webhook)
	r.Get("/connections", h.listConnections)
	r.Delete("/connections/{id}", h.deleteConnection)
	r.Get("/providers", h.listProviders)
	r.Get("/pipelines", h.listPipelines)
	r.Post("/pipelines", h.createPipeline)
	r.Get("/dlq", h.listDLQ)
	r.Post("/dlq/{id}/replay", h.replayDLQ)

	return r, nil
}

func (h *router) healthz(w http.ResponseWriter, _ *http.Request) {
	writeJSON(w, http.StatusOK, map[string]string{
		"base_url": h.baseURL,
		"service":  "argus",
		"status":   "ok",
	})
}

func (h *router) authorize(w http.ResponseWriter, r *http.Request) {
	providerID := chi.URLParam(r, "provider")
	provider, err := h.providers.Get(providerID)
	if err != nil {
		writeError(w, http.StatusNotFound, err)
		return
	}

	var request authorizeRequest
	if err := decodeJSONBody(r, &request); err != nil {
		writeError(w, http.StatusBadRequest, err)
		return
	}
	if request.ConnectionID == "" {
		request.ConnectionID = uuid.NewString()
	}

	cfg := provider.OAuthConfig()
	session, err := h.oauth.BeginAuth(r.Context(), cfg, oauth.AuthorizationRequest{
		TenantID:     h.tenantID,
		ConnectionID: request.ConnectionID,
		Provider:     providerID,
	})
	if err != nil {
		writeError(w, http.StatusInternalServerError, err)
		return
	}

	writeJSON(w, http.StatusOK, map[string]any{
		"auth_url":      session.AuthURL,
		"connection_id": request.ConnectionID,
		"expires_at":    session.ExpiresAt.UTC(),
		"state":         session.State,
	})
}

func (h *router) callback(w http.ResponseWriter, r *http.Request) {
	providerID := chi.URLParam(r, "provider")
	provider, err := h.providers.Get(providerID)
	if err != nil {
		writeError(w, http.StatusNotFound, err)
		return
	}

	code := r.URL.Query().Get("code")
	stateID := r.URL.Query().Get("state")
	if code == "" || stateID == "" {
		writeError(w, http.StatusBadRequest, fmt.Errorf("api.callback: code and state are required"))
		return
	}

	cfg := provider.OAuthConfig()
	result, err := h.oauth.Exchange(r.Context(), cfg, code, stateID)
	if err != nil {
		status := http.StatusInternalServerError
		if errors.Is(err, oauth.ErrExpiredState) {
			status = http.StatusGone
		}
		writeError(w, status, err)
		return
	}
	if result.Provider != providerID {
		writeError(w, http.StatusBadRequest, fmt.Errorf("api.callback: provider mismatch"))
		return
	}

	writeJSON(w, http.StatusOK, map[string]any{
		"connection_id": result.ConnectionID,
		"provider":      result.Provider,
		"tenant_id":     result.TenantID,
	})
}

func (h *router) webhook(w http.ResponseWriter, r *http.Request) {
	providerID := chi.URLParam(r, "provider")
	provider, err := h.providers.Get(providerID)
	if err != nil {
		writeError(w, http.StatusNotFound, err)
		return
	}

	body, err := io.ReadAll(r.Body)
	if err != nil {
		writeError(w, http.StatusBadRequest, fmt.Errorf("api.webhook: read request body: %w", err))
		return
	}

	webhookEvent, err := provider.ParseWebhookEvent(r.Header, body)
	if err != nil {
		writeError(w, http.StatusUnauthorized, err)
		return
	}

	event := envelope.Event{
		ID:           webhookEvent.ID,
		TenantID:     h.tenantID,
		ConnectionID: chi.URLParam(r, "connectionID"),
		Provider:     providerID,
		TriggerKey:   webhookEvent.TriggerKey,
		Raw:          webhookEvent.Raw,
		Normalized:   webhookEvent.Normalized,
		ReceivedAt:   h.now().UTC(),
	}
	if event.ID == "" {
		event.ID = uuid.NewString()
	}
	if event.Normalized == nil {
		event.Normalized = map[string]any{}
	}

	if err := h.events.SaveEvent(r.Context(), &event); err != nil {
		writeError(w, http.StatusInternalServerError, err)
		return
	}

	matches, err := h.matcher.Match(r.Context(), event)
	if err != nil {
		writeError(w, http.StatusInternalServerError, err)
		return
	}

	jobIDs := make([]string, 0, len(matches))
	for _, matched := range matches {
		job := &queue.Job{
			ID:      uuid.NewString(),
			EventID: event.ID,
			Payload: map[string]any{
				"event_id":    event.ID,
				"pipeline_id": matched.ID,
			},
		}

		if err := h.queue.Enqueue(r.Context(), job); err != nil {
			writeError(w, http.StatusInternalServerError, err)
			return
		}

		jobIDs = append(jobIDs, job.ID)
	}

	writeJSON(w, http.StatusAccepted, map[string]any{
		"event_id":            event.ID,
		"matched_pipelines":   len(matches),
		"queued_job_ids":      jobIDs,
		"webhook_trigger_key": event.TriggerKey,
	})
}

func (h *router) listConnections(w http.ResponseWriter, r *http.Request) {
	values, err := h.connections.ListConnections(r.Context(), h.tenantID, r.URL.Query().Get("provider"))
	if err != nil {
		writeError(w, http.StatusInternalServerError, err)
		return
	}

	writeJSON(w, http.StatusOK, values)
}

func (h *router) listProviders(w http.ResponseWriter, _ *http.Request) {
	writeJSON(w, http.StatusOK, h.providers.Metadata())
}

func (h *router) deleteConnection(w http.ResponseWriter, r *http.Request) {
	if err := h.connections.DeleteConnection(r.Context(), h.tenantID, chi.URLParam(r, "id")); err != nil {
		status := http.StatusInternalServerError
		if errors.Is(err, store.ErrNotFound) {
			status = http.StatusNotFound
		}
		writeError(w, status, err)
		return
	}

	w.WriteHeader(http.StatusNoContent)
}

func (h *router) listPipelines(w http.ResponseWriter, r *http.Request) {
	values, err := h.pipelines.ListPipelines(r.Context(), h.tenantID)
	if err != nil {
		writeError(w, http.StatusInternalServerError, err)
		return
	}

	writeJSON(w, http.StatusOK, values)
}

func (h *router) createPipeline(w http.ResponseWriter, r *http.Request) {
	var value pipeline.Pipeline
	if err := decodeJSONBody(r, &value); err != nil {
		writeError(w, http.StatusBadRequest, err)
		return
	}

	if value.ID == "" {
		value.ID = uuid.NewString()
	}
	value.TenantID = h.tenantID
	value.Normalize()
	if !value.HasExplicitEnabled() {
		value.SetEnabled(true)
	}

	if err := h.pipelines.SavePipeline(r.Context(), &value); err != nil {
		writeError(w, http.StatusInternalServerError, err)
		return
	}

	writeJSON(w, http.StatusCreated, value)
}

func (h *router) listDLQ(w http.ResponseWriter, r *http.Request) {
	jobs, err := h.dlq.List(r.Context())
	if err != nil {
		writeError(w, http.StatusInternalServerError, err)
		return
	}

	writeJSON(w, http.StatusOK, jobs)
}

func (h *router) replayDLQ(w http.ResponseWriter, r *http.Request) {
	if err := h.dlq.Replay(r.Context(), chi.URLParam(r, "id")); err != nil {
		status := http.StatusInternalServerError
		if errors.Is(err, store.ErrNotFound) {
			status = http.StatusNotFound
		}
		writeError(w, status, err)
		return
	}

	writeJSON(w, http.StatusAccepted, map[string]string{
		"status": "replayed",
	})
}

func decodeJSONBody(r *http.Request, target any) error {
	if r.Body == nil || r.ContentLength == 0 {
		return nil
	}

	decoder := json.NewDecoder(r.Body)
	decoder.UseNumber()

	if err := decoder.Decode(target); err != nil {
		return err
	}

	return nil
}

func writeError(w http.ResponseWriter, status int, err error) {
	writeJSON(w, status, map[string]string{
		"error": err.Error(),
	})
}

func writeJSON(w http.ResponseWriter, status int, payload any) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(status)
	_ = json.NewEncoder(w).Encode(payload)
}
