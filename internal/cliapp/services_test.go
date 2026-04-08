package cliapp

import (
	"context"
	"testing"
	"time"

	"github.com/stretchr/testify/require"

	"github.com/charliewilco/argus/internal/queue"
)

func TestPipelineQueueRunnerRun(t *testing.T) {
	t.Parallel()

	q := queue.NewMemoryQueue()
	now := time.Date(2026, 1, 2, 3, 4, 5, 0, time.UTC)
	runner, err := NewPipelineQueueRunner(q, func() time.Time { return now }, func() string { return "job_1" })
	require.NoError(t, err)

	jobID, err := runner.Run(context.Background(), "pipe_1", "evt_1")
	require.NoError(t, err)
	require.Equal(t, "job_1", jobID)

	job, err := q.Dequeue(context.Background())
	require.NoError(t, err)
	require.Equal(t, "job_1", job.ID)
	require.Equal(t, now, job.AvailableAt)
	require.Equal(t, map[string]any{"pipeline_id": "pipe_1", "event_id": "evt_1"}, job.Payload)
}

func TestPipelineQueueRunnerRunValidatesInput(t *testing.T) {
	t.Parallel()

	runner, err := NewPipelineQueueRunner(queue.NewMemoryQueue(), time.Now, func() string { return "job_1" })
	require.NoError(t, err)

	_, err = runner.Run(context.Background(), "", "evt_1")
	require.Error(t, err)

	_, err = runner.Run(context.Background(), "pipe_1", "")
	require.Error(t, err)
}
