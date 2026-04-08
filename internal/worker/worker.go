package worker

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"time"

	"github.com/charliewilco/argus/internal/envelope"
	"github.com/charliewilco/argus/internal/pipeline"
	"github.com/charliewilco/argus/internal/queue"
	"github.com/charliewilco/argus/internal/store"
)

const FailedJobTypeExecutionWorker = "execution_worker"

type Logger interface {
	Printf(format string, args ...any)
}

type jobQueue interface {
	Dequeue(ctx context.Context) (*queue.Job, error)
	Ack(ctx context.Context, jobID string) error
	Nack(ctx context.Context, jobID, reason string) error
}

type executionStore interface {
	GetPipeline(ctx context.Context, id string) (*pipeline.Pipeline, error)
	GetEvent(ctx context.Context, id string) (*envelope.Event, error)
}

type executor interface {
	Execute(ctx context.Context, value pipeline.Pipeline, event envelope.Event) (pipeline.ExecutionResult, error)
}

type failureSink interface {
	PushFailed(ctx context.Context, id, jobType string, payload []byte, reason string, attemptCount int, failedAt time.Time) error
}

type Worker struct {
	queue    jobQueue
	store    executionStore
	executor executor
	dlq      failureSink
	logger   Logger
	now      func() time.Time
}

func New(queue jobQueue, store executionStore, executor executor, dlq failureSink, logger Logger, now func() time.Time) (*Worker, error) {
	if queue == nil {
		return nil, fmt.Errorf("worker.New: queue is required")
	}
	if store == nil {
		return nil, fmt.Errorf("worker.New: store is required")
	}
	if executor == nil {
		return nil, fmt.Errorf("worker.New: executor is required")
	}
	if dlq == nil {
		return nil, fmt.Errorf("worker.New: DLQ is required")
	}
	if now == nil {
		now = time.Now
	}

	return &Worker{
		queue:    queue,
		store:    store,
		executor: executor,
		dlq:      dlq,
		logger:   logger,
		now:      now,
	}, nil
}

func (w *Worker) Run(ctx context.Context) error {
	for {
		job, err := w.queue.Dequeue(ctx)
		if err != nil {
			if errors.Is(err, context.Canceled) || errors.Is(err, context.DeadlineExceeded) {
				return nil
			}
			return fmt.Errorf("worker.Run: dequeue job: %w", err)
		}

		if err := w.processJob(ctx, job); err != nil {
			w.logf("worker process failed job_id=%s err=%v", job.ID, err)
		}
	}
}

func (w *Worker) processJob(ctx context.Context, job *queue.Job) error {
	if job == nil {
		return fmt.Errorf("worker.processJob: job is required")
	}

	pipelineID, eventID, err := jobPayloadIDs(job)
	if err != nil {
		return w.failJob(ctx, job, err)
	}

	value, err := w.store.GetPipeline(ctx, pipelineID)
	if err != nil {
		if errors.Is(err, store.ErrNotFound) {
			return w.failJob(ctx, job, fmt.Errorf("worker.processJob: load pipeline %q: %w", pipelineID, err))
		}
		if nackErr := w.queue.Nack(ctx, job.ID, err.Error()); nackErr != nil {
			return fmt.Errorf("worker.processJob: nack job after pipeline load failure: %w", nackErr)
		}
		return fmt.Errorf("worker.processJob: load pipeline %q: %w", pipelineID, err)
	}

	event, err := w.store.GetEvent(ctx, eventID)
	if err != nil {
		if errors.Is(err, store.ErrNotFound) {
			return w.failJob(ctx, job, fmt.Errorf("worker.processJob: load event %q: %w", eventID, err))
		}
		if nackErr := w.queue.Nack(ctx, job.ID, err.Error()); nackErr != nil {
			return fmt.Errorf("worker.processJob: nack job after event load failure: %w", nackErr)
		}
		return fmt.Errorf("worker.processJob: load event %q: %w", eventID, err)
	}

	if _, err := w.executor.Execute(ctx, *value, *event); err != nil {
		if errors.Is(err, pipeline.ErrExecutionFailed) {
			// Executor already sent handled step failures to the DLQ.
			if ackErr := w.queue.Ack(ctx, job.ID); ackErr != nil {
				return fmt.Errorf("worker.processJob: ack handled execution failure: %w", ackErr)
			}
			return fmt.Errorf("worker.processJob: execute pipeline %q: %w", pipelineID, err)
		}

		if nackErr := w.queue.Nack(ctx, job.ID, err.Error()); nackErr != nil {
			return fmt.Errorf("worker.processJob: nack failed execution: %w", nackErr)
		}
		return fmt.Errorf("worker.processJob: execute pipeline %q: %w", pipelineID, err)
	}

	if err := w.queue.Ack(ctx, job.ID); err != nil {
		return fmt.Errorf("worker.processJob: ack job: %w", err)
	}

	return nil
}

func (w *Worker) failJob(ctx context.Context, job *queue.Job, reason error) error {
	payload, err := json.Marshal(job)
	if err != nil {
		if nackErr := w.queue.Nack(ctx, job.ID, reason.Error()); nackErr != nil {
			return fmt.Errorf("worker.failJob: marshal payload: %v; nack: %w", err, nackErr)
		}
		return fmt.Errorf("worker.failJob: marshal payload: %w", err)
	}

	if err := w.dlq.PushFailed(
		ctx,
		job.ID,
		FailedJobTypeExecutionWorker,
		payload,
		reason.Error(),
		job.Attempt,
		w.now().UTC(),
	); err != nil {
		if nackErr := w.queue.Nack(ctx, job.ID, reason.Error()); nackErr != nil {
			return fmt.Errorf("worker.failJob: push DLQ: %v; nack: %w", err, nackErr)
		}
		return fmt.Errorf("worker.failJob: push DLQ: %w", err)
	}

	if err := w.queue.Ack(ctx, job.ID); err != nil {
		return fmt.Errorf("worker.failJob: ack job: %w", err)
	}

	return reason
}

func jobPayloadIDs(job *queue.Job) (string, string, error) {
	if job == nil {
		return "", "", fmt.Errorf("worker.jobPayloadIDs: job is required")
	}

	pipelineID, _ := job.Payload["pipeline_id"].(string)
	eventID, _ := job.Payload["event_id"].(string)
	if pipelineID == "" || eventID == "" {
		return "", "", fmt.Errorf("worker.jobPayloadIDs: pipeline_id and event_id are required")
	}

	return pipelineID, eventID, nil
}

func (w *Worker) logf(format string, args ...any) {
	if w.logger == nil {
		return
	}

	w.logger.Printf(format, args...)
}
