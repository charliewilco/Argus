package manualrun

import (
	"context"
	"errors"
	"testing"
	"time"

	"github.com/stretchr/testify/require"

	"github.com/charliewilco/argus/internal/envelope"
	"github.com/charliewilco/argus/internal/pipeline"
	"github.com/charliewilco/argus/internal/queue"
	"github.com/charliewilco/argus/internal/store"
)

type storeStub struct {
	pipeline *pipeline.Pipeline
	event    *envelope.Event
	err      error
	saved    *envelope.Event
}

func (s *storeStub) GetPipeline(_ context.Context, _ string) (*pipeline.Pipeline, error) {
	if s.err != nil {
		return nil, s.err
	}
	if s.pipeline == nil {
		return nil, store.ErrNotFound
	}
	return s.pipeline, nil
}

func (s *storeStub) GetEvent(_ context.Context, _ string) (*envelope.Event, error) {
	if s.err != nil {
		return nil, s.err
	}
	if s.event == nil {
		return nil, store.ErrNotFound
	}
	return s.event, nil
}

func (s *storeStub) SaveEvent(_ context.Context, event *envelope.Event) error {
	if s.err != nil {
		return s.err
	}
	copy := *event
	s.saved = &copy
	return nil
}

type queueStub struct {
	job *queue.Job
	err error
}

func (q *queueStub) Enqueue(_ context.Context, job *queue.Job) error {
	if q.err != nil {
		return q.err
	}
	q.job = job
	return nil
}

func TestRejectsInvalidIdentifiers(t *testing.T) {
	t.Parallel()

	service, err := NewService(&storeStub{}, &queueStub{}, nil)
	require.NoError(t, err)

	_, err = service.Enqueue(context.Background(), Input{})
	require.ErrorContains(t, err, "tenant ID is required")

	_, err = service.Enqueue(context.Background(), Input{TenantID: "tenant_1"})
	require.ErrorContains(t, err, "pipeline ID is required")

	_, err = service.Enqueue(context.Background(), Input{TenantID: "tenant_1", PipelineID: "pipe_1"})
	require.ErrorContains(t, err, "event ID or synthetic event is required")

	_, err = service.Enqueue(context.Background(), Input{
		TenantID:   "tenant_1",
		PipelineID: "pipe_1",
		EventID:    "evt_1",
		Synthetic:  &SyntheticEventInput{},
	})
	require.ErrorContains(t, err, "mutually exclusive")
}

func TestErrorsOnMissingPipelineOrEvent(t *testing.T) {
	t.Parallel()

	serviceMissingPipeline, err := NewService(&storeStub{}, &queueStub{}, nil)
	require.NoError(t, err)

	_, err = serviceMissingPipeline.Enqueue(context.Background(), Input{
		TenantID:   "tenant_1",
		PipelineID: "pipe_missing",
		EventID:    "evt_1",
	})
	require.ErrorIs(t, err, store.ErrNotFound)
	require.ErrorContains(t, err, "pipeline \"pipe_missing\"")

	serviceMissingEvent, err := NewService(&storeStub{
		pipeline: &pipeline.Pipeline{ID: "pipe_1", TenantID: "tenant_1"},
	}, &queueStub{}, nil)
	require.NoError(t, err)

	_, err = serviceMissingEvent.Enqueue(context.Background(), Input{
		TenantID:   "tenant_1",
		PipelineID: "pipe_1",
		EventID:    "evt_missing",
	})
	require.ErrorIs(t, err, store.ErrNotFound)
	require.ErrorContains(t, err, "event \"evt_missing\"")
}

func TestEnqueuesExistingEventForTenant(t *testing.T) {
	t.Parallel()

	store := &storeStub{
		pipeline: &pipeline.Pipeline{ID: "pipe_1", TenantID: "tenant_1", TriggerKey: "github.push"},
		event:    &envelope.Event{ID: "evt_1", TenantID: "tenant_1", TriggerKey: "github.push"},
	}
	queue := &queueStub{}
	service, err := NewService(store, queue, nil)
	require.NoError(t, err)

	job, err := service.Enqueue(context.Background(), Input{
		TenantID:   "tenant_1",
		PipelineID: "pipe_1",
		EventID:    "evt_1",
	})
	require.NoError(t, err)
	require.NotNil(t, job)
	require.Equal(t, "evt_1", job.EventID)
	require.Equal(t, "pipe_1", job.Payload["pipeline_id"])
	require.Equal(t, "evt_1", job.Payload["event_id"])
	require.NotNil(t, queue.job)
	require.Nil(t, store.saved)
}

func TestRejectsTenantMismatchAndCreatesSyntheticEvent(t *testing.T) {
	t.Parallel()

	service, err := NewService(&storeStub{
		pipeline: &pipeline.Pipeline{ID: "pipe_1", TenantID: "tenant_2"},
		event:    &envelope.Event{ID: "evt_1", TenantID: "tenant_2"},
	}, &queueStub{}, nil)
	require.NoError(t, err)

	_, err = service.Enqueue(context.Background(), Input{
		TenantID:   "tenant_1",
		PipelineID: "pipe_1",
		EventID:    "evt_1",
	})
	require.ErrorContains(t, err, "does not belong to tenant")

	now := time.Date(2026, 4, 8, 12, 0, 0, 0, time.UTC)
	syntheticStore := &storeStub{
		pipeline: &pipeline.Pipeline{ID: "pipe_1", TenantID: "tenant_1", TriggerKey: "github.push"},
	}
	syntheticQueue := &queueStub{}
	syntheticService, err := NewService(syntheticStore, syntheticQueue, func() time.Time { return now })
	require.NoError(t, err)

	job, err := syntheticService.Enqueue(context.Background(), Input{
		TenantID:   "tenant_1",
		PipelineID: "pipe_1",
		Synthetic: &SyntheticEventInput{
			ConnectionID: "conn_1",
			Normalized: map[string]any{
				"hello": "world",
			},
		},
	})
	require.NoError(t, err)
	require.NotNil(t, syntheticStore.saved)
	require.Equal(t, "tenant_1", syntheticStore.saved.TenantID)
	require.Equal(t, "github.push", syntheticStore.saved.TriggerKey)
	require.Equal(t, "github", syntheticStore.saved.Provider)
	require.Equal(t, now, syntheticStore.saved.ReceivedAt)
	require.Equal(t, syntheticStore.saved.ID, job.EventID)
	require.Equal(t, syntheticStore.saved.ID, job.Payload["event_id"])
	require.Equal(t, "pipe_1", job.Payload["pipeline_id"])
}

func TestPropagatesOperationalFailures(t *testing.T) {
	t.Parallel()

	service, err := NewService(&storeStub{err: errors.New("db down")}, &queueStub{}, nil)
	require.NoError(t, err)

	_, err = service.Enqueue(context.Background(), Input{
		TenantID:   "tenant_1",
		PipelineID: "pipe_1",
		EventID:    "evt_1",
	})
	require.ErrorContains(t, err, "get pipeline")

	service, err = NewService(&storeStub{
		pipeline: &pipeline.Pipeline{ID: "pipe_1", TenantID: "tenant_1"},
		event:    &envelope.Event{ID: "evt_1", TenantID: "tenant_1"},
	}, &queueStub{err: errors.New("queue unavailable")}, nil)
	require.NoError(t, err)

	_, err = service.Enqueue(context.Background(), Input{
		TenantID:   "tenant_1",
		PipelineID: "pipe_1",
		EventID:    "evt_1",
	})
	require.ErrorContains(t, err, "enqueue job")
}
