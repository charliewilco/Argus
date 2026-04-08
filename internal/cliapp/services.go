package cliapp

import (
	"context"
	"fmt"
	"time"

	"github.com/google/uuid"
	"golang.org/x/oauth2"

	"github.com/charliewilco/argus/internal/connections"
	"github.com/charliewilco/argus/internal/dlq"
	"github.com/charliewilco/argus/internal/queue"
	"github.com/charliewilco/argus/providers"
)

type ConnectionsService interface {
	ListConnections(ctx context.Context, tenantID, providerID string) ([]connections.Connection, error)
}

type DLQService interface {
	List(ctx context.Context) ([]dlq.FailedJob, error)
	Replay(ctx context.Context, id string) error
}

type PipelineRunner interface {
	Run(ctx context.Context, pipelineID, eventID string) (string, error)
}

type Services struct {
	Connections ConnectionsService
	DLQ         DLQService
	Pipeline    PipelineRunner
}

func NewServices(connectionsService ConnectionsService, dlqService DLQService, pipelineRunner PipelineRunner) (*Services, error) {
	if connectionsService == nil {
		return nil, fmt.Errorf("cliapp.NewServices: connections service is required")
	}
	if dlqService == nil {
		return nil, fmt.Errorf("cliapp.NewServices: DLQ service is required")
	}
	if pipelineRunner == nil {
		return nil, fmt.Errorf("cliapp.NewServices: pipeline runner is required")
	}

	return &Services{Connections: connectionsService, DLQ: dlqService, Pipeline: pipelineRunner}, nil
}

type pipelineQueueRunner struct {
	queue queue.Queue
	now   func() time.Time
	newID func() string
}

func NewPipelineQueueRunner(jobQueue queue.Queue, now func() time.Time, newID func() string) (PipelineRunner, error) {
	if jobQueue == nil {
		return nil, fmt.Errorf("cliapp.NewPipelineQueueRunner: queue is required")
	}
	if now == nil {
		now = time.Now
	}
	if newID == nil {
		newID = uuid.NewString
	}

	return &pipelineQueueRunner{queue: jobQueue, now: now, newID: newID}, nil
}

func (r *pipelineQueueRunner) Run(ctx context.Context, pipelineID, eventID string) (string, error) {
	if pipelineID == "" {
		return "", fmt.Errorf("cliapp.PipelineRunner.Run: pipeline ID is required")
	}
	if eventID == "" {
		return "", fmt.Errorf("cliapp.PipelineRunner.Run: event ID is required")
	}

	jobID := r.newID()
	if err := r.queue.Enqueue(ctx, &queue.Job{
		ID:          jobID,
		EventID:     eventID,
		AvailableAt: r.now().UTC(),
		Payload: map[string]any{
			"pipeline_id": pipelineID,
			"event_id":    eventID,
		},
	}); err != nil {
		return "", fmt.Errorf("cliapp.PipelineRunner.Run: enqueue manual run job: %w", err)
	}

	return jobID, nil
}

type noopTokenReader struct{}

func (noopTokenReader) GetToken(context.Context, string, string, *oauth2.Config) (*oauth2.Token, error) {
	return nil, fmt.Errorf("cliapp.noopTokenReader.GetToken: token reads are unavailable in CLI list operations")
}

type noopProviderRegistry struct{}

func (noopProviderRegistry) Get(string) (providers.Provider, error) {
	return nil, fmt.Errorf("cliapp.noopProviderRegistry.Get: provider lookups are unavailable in CLI list operations")
}

func NewConnectionsDomainService(connectionStore interface {
	SaveConnection(ctx context.Context, connection *connections.Connection) error
	GetConnection(ctx context.Context, tenantID, connectionID string) (*connections.Connection, error)
	GetConnectionByID(ctx context.Context, connectionID string) (*connections.Connection, error)
	ListConnections(ctx context.Context, tenantID, providerID string) ([]*connections.Connection, error)
	DeleteConnection(ctx context.Context, tenantID, connectionID string) error
}, now func() time.Time) (ConnectionsService, error) {
	service, err := connections.NewService(connectionStore, noopTokenReader{}, noopProviderRegistry{}, now)
	if err != nil {
		return nil, fmt.Errorf("cliapp.NewConnectionsDomainService: %w", err)
	}

	return service, nil
}
