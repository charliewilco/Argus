package pipeline_test

import (
	"encoding/json"
	"testing"

	"github.com/stretchr/testify/require"

	"github.com/charliewilco/argus/internal/pipeline"
)

func TestStepUnmarshalNormalizesConditionCompatibilityShapes(t *testing.T) {
	t.Parallel()

	testCases := []struct {
		name    string
		payload string
	}{
		{
			name:    "top-level expression",
			payload: `{"id":"step_1","type":"condition","expression":"event.pull_request.merged"}`,
		},
		{
			name:    "config expression",
			payload: `{"id":"step_1","type":"condition","config":{"expression":"event.pull_request.merged"}}`,
		},
		{
			name:    "nested conditions expression",
			payload: `{"id":"step_1","type":"condition","config":{"conditions":{"expression":"event.pull_request.merged"}}}`,
		},
		{
			name:    "legacy condition",
			payload: `{"id":"step_1","condition":"event.pull_request.merged"}`,
		},
	}

	for _, testCase := range testCases {
		testCase := testCase
		t.Run(testCase.name, func(t *testing.T) {
			t.Parallel()

			var step pipeline.Step
			require.NoError(t, json.Unmarshal([]byte(testCase.payload), &step))

			require.Equal(t, pipeline.StepTypeCondition, step.Type)
			require.Equal(t, map[string]any{
				"conditions": map[string]any{
					"event.pull_request.merged": true,
				},
			}, step.Config)
		})
	}
}

func TestPipelineUnmarshalTracksExplicitEnabled(t *testing.T) {
	t.Parallel()

	var omitted pipeline.Pipeline
	require.NoError(t, json.Unmarshal([]byte(`{"id":"pipe_1"}`), &omitted))
	require.False(t, omitted.Enabled)
	require.False(t, omitted.HasExplicitEnabled())

	var disabled pipeline.Pipeline
	require.NoError(t, json.Unmarshal([]byte(`{"id":"pipe_2","enabled":false}`), &disabled))
	require.False(t, disabled.Enabled)
	require.True(t, disabled.HasExplicitEnabled())
}
