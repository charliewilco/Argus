package app_test

import (
	"context"
	"path/filepath"
	"sync/atomic"
	"testing"
	"time"

	"github.com/stretchr/testify/require"

	"github.com/charliewilco/argus/config"
	"github.com/charliewilco/argus/internal/app"
	"github.com/charliewilco/argus/internal/queue"
)

func TestNewWithConfigBuildsRuntime(t *testing.T) {
	t.Parallel()

	databasePath := filepath.Join(t.TempDir(), "argus.db")
	cfg := config.Config{
		BaseURL:     "http://localhost:8080",
		DatabaseURL: "sqlite:" + databasePath,
		SecretKey:   "test-secret-key",
		TenantID:    "tenant-a",
	}

	var queueFactoryCalls atomic.Int32
	runtime, err := app.NewWithConfig(context.Background(), cfg, app.Options{
		QueueFactory: func(_ config.Config) (queue.Queue, error) {
			queueFactoryCalls.Add(1)
			return queue.NewMemoryQueue(), nil
		},
	})
	require.NoError(t, err)
	t.Cleanup(func() {
		require.NoError(t, runtime.Close())
	})

	require.NotNil(t, runtime.Store)
	require.NotNil(t, runtime.Queue)
	require.NotNil(t, runtime.Providers)
	require.NotNil(t, runtime.OAuth)
	require.NotNil(t, runtime.Connections)
	require.NotNil(t, runtime.DLQ)
	require.NotNil(t, runtime.Matcher)
	require.NotNil(t, runtime.Dispatcher)
	require.NotNil(t, runtime.Executor)
	require.Equal(t, int32(1), queueFactoryCalls.Load())
}

func TestRuntimeCloseCancelsContext(t *testing.T) {
	t.Parallel()

	databasePath := filepath.Join(t.TempDir(), "argus.db")
	cfg := config.Config{
		BaseURL:     "http://localhost:8080",
		DatabaseURL: "sqlite:" + databasePath,
		SecretKey:   "test-secret-key",
		TenantID:    "tenant-a",
	}

	runtime, err := app.NewWithConfig(context.Background(), cfg, app.Options{})
	require.NoError(t, err)

	require.NoError(t, runtime.Close())
	require.NoError(t, runtime.Close())

	select {
	case <-runtime.Context.Done():
	case <-time.After(time.Second):
		t.Fatal("expected runtime context to be canceled")
	}
}
