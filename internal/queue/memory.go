package queue

import (
	"context"
	"errors"
	"sync"
	"time"
)

type MemoryQueue struct {
	mu      sync.Mutex
	pending []*Job
	leased  map[string]*Job
	notify  chan struct{}
	now     func() time.Time
}

func NewMemoryQueue() *MemoryQueue {
	return &MemoryQueue{
		pending: make([]*Job, 0),
		leased:  make(map[string]*Job),
		notify:  make(chan struct{}, 1),
		now:     time.Now,
	}
}

func (q *MemoryQueue) Enqueue(_ context.Context, job *Job) error {
	if job == nil {
		return errors.New("queue.Enqueue: job is required")
	}
	if job.ID == "" {
		return errors.New("queue.Enqueue: job ID is required")
	}

	jobCopy := cloneJob(job)
	if jobCopy.AvailableAt.IsZero() {
		jobCopy.AvailableAt = q.now().UTC()
	}

	q.mu.Lock()
	defer q.mu.Unlock()

	q.pending = append(q.pending, jobCopy)
	q.signal()

	return nil
}

func (q *MemoryQueue) Dequeue(ctx context.Context) (*Job, error) {
	for {
		q.mu.Lock()
		index, waitFor := q.nextReadyIndexLocked()
		if index >= 0 {
			job := q.pending[index]
			q.pending = append(q.pending[:index], q.pending[index+1:]...)
			q.leased[job.ID] = job
			q.mu.Unlock()
			return cloneJob(job), nil
		}
		q.mu.Unlock()

		if err := q.wait(ctx, waitFor); err != nil {
			return nil, err
		}
	}
}

func (q *MemoryQueue) Ack(_ context.Context, jobID string) error {
	q.mu.Lock()
	defer q.mu.Unlock()

	if _, ok := q.leased[jobID]; !ok {
		return ErrNotFound
	}

	delete(q.leased, jobID)
	return nil
}

func (q *MemoryQueue) Nack(_ context.Context, jobID, reason string) error {
	q.mu.Lock()
	defer q.mu.Unlock()

	job, ok := q.leased[jobID]
	if !ok {
		return ErrNotFound
	}

	delete(q.leased, jobID)
	job.Attempt++
	job.LastError = reason
	job.AvailableAt = q.now().UTC()
	q.pending = append(q.pending, job)
	q.signal()

	return nil
}

func (q *MemoryQueue) nextReadyIndexLocked() (int, time.Duration) {
	now := q.now().UTC()
	nextWait := time.Duration(-1)

	for index, job := range q.pending {
		if !job.AvailableAt.After(now) {
			return index, 0
		}

		waitFor := job.AvailableAt.Sub(now)
		if nextWait < 0 || waitFor < nextWait {
			nextWait = waitFor
		}
	}

	return -1, nextWait
}

func (q *MemoryQueue) wait(ctx context.Context, waitFor time.Duration) error {
	if waitFor < 0 {
		select {
		case <-ctx.Done():
			return ctx.Err()
		case <-q.notify:
			return nil
		}
	}

	timer := time.NewTimer(waitFor)
	defer timer.Stop()

	select {
	case <-ctx.Done():
		return ctx.Err()
	case <-q.notify:
		return nil
	case <-timer.C:
		return nil
	}
}

func (q *MemoryQueue) signal() {
	select {
	case q.notify <- struct{}{}:
	default:
	}
}

func cloneJob(job *Job) *Job {
	if job == nil {
		return nil
	}

	clone := *job
	if job.Payload != nil {
		clone.Payload = make(map[string]any, len(job.Payload))
		for key, value := range job.Payload {
			clone.Payload[key] = value
		}
	}

	return &clone
}
