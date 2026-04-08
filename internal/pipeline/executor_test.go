package pipeline_test

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
	"github.com/charliewilco/argus/providers"
)

type dispatcherStub struct {
	result     providers.ActionResult
	err        error
	lastConfig map[string]any
}

func (d *dispatcherStub) Dispatch(_ context.Context, stepConfig map[string]any, _, _ string, _ *envelope.Event) (providers.ActionResult, error) {
	d.lastConfig = stepConfig
	if d.err != nil {
		return providers.ActionResult{}, d.err
	}
	return d.result, nil
}

func TestExecutorRunsLegacyConditionStep(t *testing.T) {
	t.Parallel()

	executor, err := pipeline.NewExecutor(&dispatcherStub{}, &dlqStub{}, func() time.Time {
		return time.Date(2026, 4, 7, 12, 0, 0, 0, time.UTC)
	})
	require.NoError(t, err)

	var value pipeline.Pipeline
	require.NoError(t, json.Unmarshal([]byte(`{
		"id":"pipe_legacy",
		"tenant_id":"tenant_1",
		"steps":[
			{"id":"step_1","condition":"event.pull_request.merged"}
		]
	}`), &value))

	result, err := executor.Execute(context.Background(), value, envelope.Event{
		ID: "evt_1",
		Normalized: map[string]any{
			"pull_request": map[string]any{
				"merged": true,
			},
		},
	})
	require.NoError(t, err)
	require.Equal(t, pipeline.ExecutionStatusSucceeded, result.Status)
	require.Len(t, result.StepOutcomes, 1)
	require.Equal(t, pipeline.StepStatusSucceeded, result.StepOutcomes[0].Status)
	require.Equal(t, true, result.StepOutcomes[0].Output["matched"])
}

type dlqStub struct {
	jobs []failedJobRecord
}

type failedJobRecord struct {
	ID      string
	JobType string
	Payload []byte
}

func (d *dlqStub) PushFailed(_ context.Context, id, jobType string, payload []byte, _ string, _ int, _ time.Time) error {
	d.jobs = append(d.jobs, failedJobRecord{
		ID:      id,
		JobType: jobType,
		Payload: payload,
	})
	return nil
}

func TestExecutorRunsAllStepsOnSuccess(t *testing.T) {
	t.Parallel()

	dispatcher := &dispatcherStub{
		result: providers.ActionResult{
			Provider: "github",
			Action:   "github.noop",
			Status:   "ok",
			Output: map[string]any{
				"delivered": true,
			},
		},
	}

	executor, err := pipeline.NewExecutor(dispatcher, &dlqStub{}, func() time.Time {
		return time.Date(2026, 4, 7, 12, 0, 0, 0, time.UTC)
	})
	require.NoError(t, err)

	result, err := executor.Execute(context.Background(), pipeline.Pipeline{
		ID:           "pipe_1",
		TenantID:     "tenant_1",
		ConnectionID: "conn_1",
		Steps: []pipeline.Step{
			{
				ID:   "transform_1",
				Type: pipeline.StepTypeTransform,
				Config: map[string]any{
					"output": map[string]any{
						"message": "{{event.repository.full_name}}",
					},
				},
			},
			{
				ID:   "action_1",
				Type: pipeline.StepTypeAction,
				Config: map[string]any{
					"action":        "github.noop",
					"connection_id": "conn_1",
					"message":       "{{steps.transform_1.output.message}}",
				},
			},
		},
	}, envelope.Event{
		ID: "evt_1",
		Normalized: map[string]any{
			"repository": map[string]any{
				"full_name": "charliewilco/argus",
			},
		},
	})
	require.NoError(t, err)
	require.Equal(t, pipeline.ExecutionStatusSucceeded, result.Status)
	require.Len(t, result.StepOutcomes, 2)
	require.Equal(t, pipeline.StepStatusSucceeded, result.StepOutcomes[0].Status)
	require.Equal(t, "charliewilco/argus", result.StepOutcomes[0].Output["message"])
	require.Equal(t, pipeline.StepStatusSucceeded, result.StepOutcomes[1].Status)
	require.Equal(t, "charliewilco/argus", dispatcher.lastConfig["message"])
}

func TestExecutorStopsOnFailureAndPushesToDLQ(t *testing.T) {
	t.Parallel()

	dlqStore := &dlqStub{}
	executor, err := pipeline.NewExecutor(&dispatcherStub{
		err: errors.New("boom"),
	}, dlqStore, func() time.Time {
		return time.Date(2026, 4, 7, 12, 0, 0, 0, time.UTC)
	})
	require.NoError(t, err)

	result, err := executor.Execute(context.Background(), pipeline.Pipeline{
		ID:           "pipe_1",
		TenantID:     "tenant_1",
		ConnectionID: "conn_1",
		Steps: []pipeline.Step{
			{
				ID:   "action_1",
				Type: pipeline.StepTypeAction,
				Config: map[string]any{
					"action": "github.noop",
				},
			},
			{
				ID:   "action_2",
				Type: pipeline.StepTypeAction,
				Config: map[string]any{
					"action": "github.noop",
				},
			},
		},
	}, envelope.Event{ID: "evt_1"})
	require.Error(t, err)
	require.Equal(t, pipeline.ExecutionStatusFailed, result.Status)
	require.Len(t, result.StepOutcomes, 1)
	require.Equal(t, pipeline.StepStatusFailed, result.StepOutcomes[0].Status)
	require.Len(t, dlqStore.jobs, 1)
	require.Equal(t, pipeline.FailedJobTypePipelineExecution, dlqStore.jobs[0].JobType)

	var queuedJob queue.Job
	require.NoError(t, json.Unmarshal(dlqStore.jobs[0].Payload, &queuedJob))
	require.Equal(t, "pipe_1:evt_1", queuedJob.ID)
}
