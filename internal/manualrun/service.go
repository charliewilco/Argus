package manualrun

import (
	"context"
	"fmt"
	"strings"
	"time"

	"github.com/google/uuid"

	"github.com/charliewilco/argus/internal/envelope"
	"github.com/charliewilco/argus/internal/pipeline"
	"github.com/charliewilco/argus/internal/queue"
	"github.com/charliewilco/argus/internal/store"
)

type Store interface {
	GetPipeline(ctx context.Context, id string) (*pipeline.Pipeline, error)
	GetEvent(ctx context.Context, id string) (*envelope.Event, error)
	SaveEvent(ctx context.Context, event *envelope.Event) error
}

type Queue interface {
	Enqueue(ctx context.Context, job *queue.Job) error
}

type Input struct {
	TenantID   string
	PipelineID string
	EventID    string
	Synthetic  *SyntheticEventInput
}

type SyntheticEventInput struct {
	ConnectionID string
	Provider     string
	TriggerKey   string
	Raw          []byte
	Normalized   map[string]any
}

type Service struct {
	store Store
	queue Queue
	now   func() time.Time
}

func NewService(store Store, queue Queue, now func() time.Time) (*Service, error) {
	if store == nil {
		return nil, fmt.Errorf("manualrun.NewService: store is required")
	}
	if queue == nil {
		return nil, fmt.Errorf("manualrun.NewService: queue is required")
	}
	if now == nil {
		now = time.Now
	}

	return &Service{store: store, queue: queue, now: now}, nil
}

func (s *Service) Enqueue(ctx context.Context, input Input) (*queue.Job, error) {
	if input.TenantID == "" {
		return nil, fmt.Errorf("manualrun.Service.Enqueue: tenant ID is required")
	}
	if input.PipelineID == "" {
		return nil, fmt.Errorf("manualrun.Service.Enqueue: pipeline ID is required")
	}
	if input.EventID != "" && input.Synthetic != nil {
		return nil, fmt.Errorf("manualrun.Service.Enqueue: event ID and synthetic event are mutually exclusive")
	}
	if input.EventID == "" && input.Synthetic == nil {
		return nil, fmt.Errorf("manualrun.Service.Enqueue: event ID or synthetic event is required")
	}

	value, err := s.store.GetPipeline(ctx, input.PipelineID)
	if err != nil {
		if err == store.ErrNotFound {
			return nil, fmt.Errorf("manualrun.Service.Enqueue: pipeline %q: %w", input.PipelineID, err)
		}
		return nil, fmt.Errorf("manualrun.Service.Enqueue: get pipeline %q: %w", input.PipelineID, err)
	}
	if value.TenantID != input.TenantID {
		return nil, fmt.Errorf("manualrun.Service.Enqueue: pipeline %q does not belong to tenant %q", input.PipelineID, input.TenantID)
	}

	eventID := input.EventID
	if input.Synthetic != nil {
		eventID, err = s.createSyntheticEvent(ctx, input.TenantID, value, input.Synthetic)
		if err != nil {
			return nil, err
		}
	} else {
		event, getErr := s.store.GetEvent(ctx, input.EventID)
		if getErr != nil {
			if getErr == store.ErrNotFound {
				return nil, fmt.Errorf("manualrun.Service.Enqueue: event %q: %w", input.EventID, getErr)
			}
			return nil, fmt.Errorf("manualrun.Service.Enqueue: get event %q: %w", input.EventID, getErr)
		}
		if event.TenantID != input.TenantID {
			return nil, fmt.Errorf("manualrun.Service.Enqueue: event %q does not belong to tenant %q", input.EventID, input.TenantID)
		}
	}

	job := &queue.Job{
		ID:      uuid.NewString(),
		EventID: eventID,
		Payload: map[string]any{
			"pipeline_id": input.PipelineID,
			"event_id":    eventID,
		},
	}
	if err := s.queue.Enqueue(ctx, job); err != nil {
		return nil, fmt.Errorf("manualrun.Service.Enqueue: enqueue job: %w", err)
	}

	return job, nil
}

func (s *Service) createSyntheticEvent(ctx context.Context, tenantID string, value *pipeline.Pipeline, input *SyntheticEventInput) (string, error) {
	normalized := input.Normalized
	if normalized == nil {
		normalized = map[string]any{}
	}

	event := envelope.Event{
		ID:           uuid.NewString(),
		TenantID:     tenantID,
		ConnectionID: input.ConnectionID,
		Provider:     input.Provider,
		TriggerKey:   input.TriggerKey,
		Raw:          input.Raw,
		Normalized:   normalized,
		ReceivedAt:   s.now().UTC(),
	}
	if event.TriggerKey == "" {
		event.TriggerKey = value.TriggerKey
	}
	if event.Provider == "" {
		event.Provider = providerFromTrigger(event.TriggerKey)
	}
	if err := s.store.SaveEvent(ctx, &event); err != nil {
		return "", fmt.Errorf("manualrun.Service.Enqueue: save synthetic event: %w", err)
	}

	return event.ID, nil
}

func providerFromTrigger(triggerKey string) string {
	if triggerKey == "" {
		return ""
	}
	parts := strings.SplitN(triggerKey, ".", 2)
	return parts[0]
}
