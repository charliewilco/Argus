package dlq

import (
	"context"
	"encoding/json"
	"fmt"
	"time"

	"github.com/charliewilco/argus/internal/queue"
	"github.com/charliewilco/argus/internal/store"
)

type FailedJob struct {
	ID           string     `json:"id"`
	JobType      string     `json:"job_type"`
	Payload      []byte     `json:"payload"`
	Reason       string     `json:"reason"`
	AttemptCount int        `json:"attempt_count"`
	FailedAt     time.Time  `json:"failed_at"`
	ReplayedAt   *time.Time `json:"replayed_at,omitempty"`
}

type failedJobStore interface {
	PushFailedJob(ctx context.Context, job store.FailedJob) error
	GetFailedJob(ctx context.Context, id string) (*store.FailedJob, error)
	ListFailedJobs(ctx context.Context) ([]*store.FailedJob, error)
	MarkFailedJobReplayed(ctx context.Context, id string, replayedAt time.Time) error
	DeleteFailedJob(ctx context.Context, id string) error
}

type Store struct {
	store failedJobStore
	queue queue.Queue
	now   func() time.Time
}

func NewStore(store failedJobStore, queue queue.Queue, now func() time.Time) (*Store, error) {
	if store == nil {
		return nil, fmt.Errorf("dlq.NewStore: store is required")
	}
	if queue == nil {
		return nil, fmt.Errorf("dlq.NewStore: queue is required")
	}
	if now == nil {
		now = time.Now
	}

	return &Store{
		store: store,
		queue: queue,
		now:   now,
	}, nil
}

func (s *Store) Push(ctx context.Context, job FailedJob) error {
	if job.ID == "" {
		return fmt.Errorf("dlq.Push: job ID is required")
	}
	if job.FailedAt.IsZero() {
		job.FailedAt = s.now().UTC()
	}
	if !json.Valid(job.Payload) {
		return fmt.Errorf("dlq.Push: payload must be valid JSON")
	}

	if err := s.store.PushFailedJob(ctx, toStoreJob(job)); err != nil {
		return fmt.Errorf("dlq.Push: %w", err)
	}

	return nil
}

func (s *Store) PushFailed(ctx context.Context, id, jobType string, payload []byte, reason string, attemptCount int, failedAt time.Time) error {
	return s.Push(ctx, FailedJob{
		ID:           id,
		JobType:      jobType,
		Payload:      payload,
		Reason:       reason,
		AttemptCount: attemptCount,
		FailedAt:     failedAt,
	})
}

func (s *Store) List(ctx context.Context) ([]FailedJob, error) {
	jobs, err := s.store.ListFailedJobs(ctx)
	if err != nil {
		return nil, fmt.Errorf("dlq.List: %w", err)
	}

	result := make([]FailedJob, 0, len(jobs))
	for _, job := range jobs {
		if job == nil {
			continue
		}
		result = append(result, fromStoreJob(*job))
	}

	return result, nil
}

func (s *Store) Replay(ctx context.Context, id string) error {
	job, err := s.store.GetFailedJob(ctx, id)
	if err != nil {
		return fmt.Errorf("dlq.Replay: load failed job: %w", err)
	}

	queuedJob, err := replayQueueJob(*job)
	if err != nil {
		return fmt.Errorf("dlq.Replay: build queue job: %w", err)
	}

	if err := s.queue.Enqueue(ctx, queuedJob); err != nil {
		return fmt.Errorf("dlq.Replay: enqueue job: %w", err)
	}

	if err := s.store.MarkFailedJobReplayed(ctx, id, s.now().UTC()); err != nil {
		return fmt.Errorf("dlq.Replay: mark replayed: %w", err)
	}

	return nil
}

func (s *Store) Delete(ctx context.Context, id string) error {
	if err := s.store.DeleteFailedJob(ctx, id); err != nil {
		return fmt.Errorf("dlq.Delete: %w", err)
	}

	return nil
}

func replayQueueJob(job store.FailedJob) (*queue.Job, error) {
	var queuedJob queue.Job
	if err := json.Unmarshal(job.Payload, &queuedJob); err != nil {
		return nil, err
	}

	return &queuedJob, nil
}

func toStoreJob(job FailedJob) store.FailedJob {
	return store.FailedJob{
		ID:           job.ID,
		JobType:      job.JobType,
		Payload:      job.Payload,
		Reason:       job.Reason,
		AttemptCount: job.AttemptCount,
		FailedAt:     job.FailedAt.UTC(),
		ReplayedAt:   job.ReplayedAt,
	}
}

func fromStoreJob(job store.FailedJob) FailedJob {
	return FailedJob{
		ID:           job.ID,
		JobType:      job.JobType,
		Payload:      job.Payload,
		Reason:       job.Reason,
		AttemptCount: job.AttemptCount,
		FailedAt:     job.FailedAt.UTC(),
		ReplayedAt:   job.ReplayedAt,
	}
}
