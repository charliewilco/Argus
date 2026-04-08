package pipeline_test

import (
	"encoding/json"
	"testing"

	"github.com/stretchr/testify/require"

	"github.com/charliewilco/argus/internal/pipeline"
)

func TestPipelineUnmarshalDefaultsEnabledToTrue(t *testing.T) {
	t.Parallel()

	var value pipeline.Pipeline
	require.NoError(t, json.Unmarshal([]byte(`{
		"id": "pipe_1",
		"tenantId": "tenant_1",
		"name": "Example",
		"triggerKey": "github.issue.created",
		"connectionId": "conn_1"
	}`), &value))

	require.True(t, value.Enabled)
	require.False(t, value.HasExplicitEnabled())
}

func TestPipelineUnmarshalRespectsExplicitEnabledFalse(t *testing.T) {
	t.Parallel()

	var value pipeline.Pipeline
	require.NoError(t, json.Unmarshal([]byte(`{
		"id": "pipe_1",
		"enabled": false
	}`), &value))

	require.False(t, value.Enabled)
	require.True(t, value.HasExplicitEnabled())
}

func TestStepUnmarshalSupportsConditionAliases(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name    string
		payload string
	}{
		{
			name:    "legacy condition string",
			payload: `{"id":"step_1","condition":"event.action == \"opened\""}`,
		},
		{
			name:    "condition expression object",
			payload: `{"id":"step_1","condition":{"expression":"event.action == \"opened\""}}`,
		},
		{
			name:    "conditions expression object",
			payload: `{"id":"step_1","conditions":{"expression":"event.action == \"opened\""}}`,
		},
		{
			name:    "conditions map",
			payload: `{"id":"step_1","conditions":{"event.action == \"opened\"":true}}`,
		},
	}

	for _, tt := range tests {
		tt := tt
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			var step pipeline.Step
			require.NoError(t, json.Unmarshal([]byte(tt.payload), &step))
			require.Equal(t, `event.action == "opened"`, step.Condition)
		})
	}
}

func TestStepMarshalUsesConditionsExpressionShape(t *testing.T) {
	t.Parallel()

	data, err := json.Marshal(pipeline.Step{
		ID:        "step_1",
		Condition: `event.action == "opened"`,
	})
	require.NoError(t, err)
	require.JSONEq(t, `{
		"id": "step_1",
		"conditions": {
			"expression": "event.action == \"opened\""
		}
	}`, string(data))
}
