package worker

import (
	"context"
	"encoding/json"
	"errors"
	"testing"
	"time"

	"github.com/stretchr/testify/require"

	"github.com/charliewilco/argus/internal/envelope"
	"github.com/charliewilco/argus/internal/pipeline"
	"github.com/charliewilco/argus/internal/queue"
	"github.com/charliewilco/argus/internal/store"
)

type queueStub struct {
	acked    []string
	nacked   []string
	lastNack string
}

func (q *queueStub) Dequeue(_ context.Context) (*queue.Job, error) { return nil, errors.New("unused") }

func (q *queueStub) Ack(_ context.Context, jobID string) error {
	q.acked = append(q.acked, jobID)
	return nil
}

func (q *queueStub) Nack(_ context.Context, jobID, reason string) error {
	q.nacked = append(q.nacked, jobID)
	q.lastNack = reason
	return nil
}

type storeStub struct {
	pipeline *pipeline.Pipeline
	event    *envelope.Event
	err      error
}

func (s *storeStub) GetPipeline(_ context.Context, _ string) (*pipeline.Pipeline, error) {
	if s.err != nil {
		return nil, s.err
	}
	return s.pipeline, nil
}

func (s *storeStub) GetEvent(_ context.Context, _ string) (*envelope.Event, error) {
	if s.err != nil {
		return nil, s.err
	}
	return s.event, nil
}

type executorStub struct {
	calls int
	err   error
}

func (e *executorStub) Execute(_ context.Context, _ pipeline.Pipeline, _ envelope.Event) (pipeline.ExecutionResult, error) {
	e.calls++
	if e.err != nil {
		return pipeline.ExecutionResult{}, e.err
	}
	return pipeline.ExecutionResult{Status: pipeline.ExecutionStatusSucceeded}, nil
}

type dlqStub struct {
	jobID string
}

func (d *dlqStub) PushFailed(_ context.Context, id, _ string, _ []byte, _ string, _ int, _ time.Time) error {
	d.jobID = id
	return nil
}

func TestProcessJobExecutesAndAcks(t *testing.T) {
	t.Parallel()

	q := &queueStub{}
	executor := &executorStub{}
	worker, err := New(
		q,
		&storeStub{
			pipeline: &pipeline.Pipeline{ID: "pipe_1"},
			event:    &envelope.Event{ID: "evt_1"},
		},
		executor,
		&dlqStub{},
		nil,
		func() time.Time { return time.Date(2026, 4, 7, 12, 0, 0, 0, time.UTC) },
	)
	require.NoError(t, err)

	err = worker.processJob(context.Background(), &queue.Job{
		ID:      "job_1",
		EventID: "evt_1",
		Payload: map[string]any{
			"pipeline_id": "pipe_1",
			"event_id":    "evt_1",
		},
	})
	require.NoError(t, err)
	require.Equal(t, 1, executor.calls)
	require.Equal(t, []string{"job_1"}, q.acked)
}

func TestProcessJobSendsInvalidPayloadToDLQAndAcks(t *testing.T) {
	t.Parallel()

	q := &queueStub{}
	dlq := &dlqStub{}
	worker, err := New(
		q,
		&storeStub{},
		&executorStub{},
		dlq,
		nil,
		func() time.Time { return time.Date(2026, 4, 7, 12, 0, 0, 0, time.UTC) },
	)
	require.NoError(t, err)

	job := &queue.Job{
		ID:      "job_bad",
		EventID: "evt_1",
		Payload: map[string]any{"event_id": "evt_1"},
	}
	err = worker.processJob(context.Background(), job)
	require.Error(t, err)
	require.Equal(t, "job_bad", dlq.jobID)
	require.Equal(t, []string{"job_bad"}, q.acked)

	payload, marshalErr := json.Marshal(job)
	require.NoError(t, marshalErr)
	require.NotEmpty(t, payload)
}

func TestProcessJobNacksTransientStoreFailures(t *testing.T) {
	t.Parallel()

	q := &queueStub{}
	worker, err := New(
		q,
		&storeStub{err: errors.New("sqlite busy")},
		&executorStub{},
		&dlqStub{},
		nil,
		func() time.Time { return time.Date(2026, 4, 7, 12, 0, 0, 0, time.UTC) },
	)
	require.NoError(t, err)

	err = worker.processJob(context.Background(), &queue.Job{
		ID: "job_retry",
		Payload: map[string]any{
			"pipeline_id": "pipe_1",
			"event_id":    "evt_1",
		},
	})
	require.Error(t, err)
	require.Equal(t, []string{"job_retry"}, q.nacked)
	require.Empty(t, q.acked)
}

func TestProcessJobDLQsMissingEntities(t *testing.T) {
	t.Parallel()

	q := &queueStub{}
	dlq := &dlqStub{}
	worker, err := New(
		q,
		&storeStub{err: store.ErrNotFound},
		&executorStub{},
		dlq,
		nil,
		func() time.Time { return time.Date(2026, 4, 7, 12, 0, 0, 0, time.UTC) },
	)
	require.NoError(t, err)

	err = worker.processJob(context.Background(), &queue.Job{
		ID: "job_missing",
		Payload: map[string]any{
			"pipeline_id": "pipe_1",
			"event_id":    "evt_1",
		},
	})
	require.Error(t, err)
	require.Equal(t, "job_missing", dlq.jobID)
	require.Equal(t, []string{"job_missing"}, q.acked)
}

func TestProcessJobAcksHandledPipelineFailures(t *testing.T) {
	t.Parallel()

	q := &queueStub{}
	worker, err := New(
		q,
		&storeStub{
			pipeline: &pipeline.Pipeline{ID: "pipe_1"},
			event:    &envelope.Event{ID: "evt_1"},
		},
		&executorStub{
			err: &pipeline.ExecutionError{
				StepID: "step_1",
				Err:    errors.New("boom"),
			},
		},
		&dlqStub{},
		nil,
		func() time.Time { return time.Date(2026, 4, 7, 12, 0, 0, 0, time.UTC) },
	)
	require.NoError(t, err)

	err = worker.processJob(context.Background(), &queue.Job{
		ID:      "job_failed",
		EventID: "evt_1",
		Payload: map[string]any{
			"pipeline_id": "pipe_1",
			"event_id":    "evt_1",
		},
	})
	require.Error(t, err)
	require.Equal(t, []string{"job_failed"}, q.acked)
	require.Empty(t, q.nacked)
}

func TestProcessJobNacksUnhandledExecutionFailures(t *testing.T) {
	t.Parallel()

	q := &queueStub{}
	worker, err := New(
		q,
		&storeStub{
			pipeline: &pipeline.Pipeline{ID: "pipe_1"},
			event:    &envelope.Event{ID: "evt_1"},
		},
		&executorStub{err: errors.New("dlq unavailable")},
		&dlqStub{},
		nil,
		func() time.Time { return time.Date(2026, 4, 7, 12, 0, 0, 0, time.UTC) },
	)
	require.NoError(t, err)

	err = worker.processJob(context.Background(), &queue.Job{
		ID:      "job_retry_execution",
		EventID: "evt_1",
		Payload: map[string]any{
			"pipeline_id": "pipe_1",
			"event_id":    "evt_1",
		},
	})
	require.Error(t, err)
	require.Equal(t, []string{"job_retry_execution"}, q.nacked)
	require.Empty(t, q.acked)
}
