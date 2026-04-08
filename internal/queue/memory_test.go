package queue_test

import (
	"context"
	"testing"
	"time"

	"github.com/stretchr/testify/require"

	"github.com/charliewilco/argus/internal/queue"
)

func TestMemoryQueueRoundTrip(t *testing.T) {
	t.Parallel()

	q := queue.NewMemoryQueue()
	ctx := context.Background()

	require.NoError(t, q.Enqueue(ctx, &queue.Job{
		ID:      "job_1",
		EventID: "evt_1",
		Payload: map[string]any{"key": "value"},
	}))

	job, err := q.Dequeue(ctx)
	require.NoError(t, err)
	require.Equal(t, "job_1", job.ID)
	require.Equal(t, "value", job.Payload["key"])

	require.NoError(t, q.Ack(ctx, job.ID))
}

func TestMemoryQueueDequeueWaitsForJob(t *testing.T) {
	t.Parallel()

	q := queue.NewMemoryQueue()
	ctx, cancel := context.WithTimeout(context.Background(), time.Second)
	defer cancel()

	type result struct {
		job *queue.Job
		err error
	}

	done := make(chan result, 1)
	go func() {
		job, err := q.Dequeue(ctx)
		done <- result{job: job, err: err}
	}()

	time.Sleep(25 * time.Millisecond)
	require.NoError(t, q.Enqueue(ctx, &queue.Job{ID: "job_wait"}))

	select {
	case result := <-done:
		require.NoError(t, result.err)
		require.Equal(t, "job_wait", result.job.ID)
	case <-ctx.Done():
		t.Fatal("timed out waiting for dequeued job")
	}
}

func TestMemoryQueueNackRequeuesJob(t *testing.T) {
	t.Parallel()

	q := queue.NewMemoryQueue()
	ctx := context.Background()

	require.NoError(t, q.Enqueue(ctx, &queue.Job{ID: "job_retry"}))

	job, err := q.Dequeue(ctx)
	require.NoError(t, err)
	require.Equal(t, 0, job.Attempt)

	require.NoError(t, q.Nack(ctx, job.ID, "temporary failure"))

	retried, err := q.Dequeue(ctx)
	require.NoError(t, err)
	require.Equal(t, 1, retried.Attempt)
	require.Equal(t, "temporary failure", retried.LastError)
}
