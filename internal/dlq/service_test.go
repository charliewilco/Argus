package dlq_test

import (
	"context"
	"encoding/json"
	"path/filepath"
	"testing"
	"time"

	"github.com/stretchr/testify/require"

	"github.com/charliewilco/argus/internal/dlq"
	"github.com/charliewilco/argus/internal/queue"
	sqlitestore "github.com/charliewilco/argus/internal/store/sqlite"
)

func TestReplayReenqueuesJobAndMarksReplayedAt(t *testing.T) {
	t.Parallel()

	ctx := context.Background()
	store := newStore(t)
	mainQueue := queue.NewMemoryQueue()
	now := func() time.Time {
		return time.Date(2026, 4, 7, 12, 0, 0, 0, time.UTC)
	}

	service, err := dlq.NewStore(store, mainQueue, now)
	require.NoError(t, err)

	payload, err := json.Marshal(queue.Job{
		ID:      "job_1",
		EventID: "evt_1",
		Payload: map[string]any{
			"pipeline_id": "pipe_1",
		},
	})
	require.NoError(t, err)

	require.NoError(t, service.Push(ctx, dlq.FailedJob{
		ID:           "failed_1",
		JobType:      "pipeline_execution",
		Payload:      payload,
		Reason:       "boom",
		AttemptCount: 1,
		FailedAt:     now(),
	}))

	require.NoError(t, service.Replay(ctx, "failed_1"))

	job, err := mainQueue.Dequeue(ctx)
	require.NoError(t, err)
	require.Equal(t, "job_1", job.ID)

	failedJob, err := store.GetFailedJob(ctx, "failed_1")
	require.NoError(t, err)
	require.NotNil(t, failedJob.ReplayedAt)
}

func newStore(t *testing.T) *sqlitestore.Store {
	t.Helper()

	ctx := context.Background()
	path := filepath.Join(t.TempDir(), "argus-dlq.db")
	store, err := sqlitestore.Open(ctx, "sqlite:"+path)
	require.NoError(t, err)
	t.Cleanup(func() {
		require.NoError(t, store.Close())
	})

	return store
}
