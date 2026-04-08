package triggers_test

import (
	"context"
	"testing"

	"github.com/stretchr/testify/require"

	"github.com/charliewilco/argus/internal/envelope"
	"github.com/charliewilco/argus/internal/pipeline"
	"github.com/charliewilco/argus/internal/triggers"
)

type triggerStore struct {
	pipelines []*pipeline.Pipeline
}

func (s *triggerStore) ListPipelines(_ context.Context, _ string) ([]*pipeline.Pipeline, error) {
	return s.pipelines, nil
}

func TestTriggerMatcherMatchesAndRejectsByConditions(t *testing.T) {
	t.Parallel()

	matcher, err := triggers.NewTriggerMatcher(&triggerStore{
		pipelines: []*pipeline.Pipeline{
			{
				ID:           "pipe_match",
				TenantID:     "tenant_1",
				ConnectionID: "conn_1",
				Enabled:      true,
				Trigger: pipeline.Trigger{
					Key: "github.pull_request",
					Conditions: map[string]any{
						"event.action":               "opened",
						"event.repository.full_name": "charliewilco/argus",
					},
				},
			},
			{
				ID:           "pipe_wrong_action",
				TenantID:     "tenant_1",
				ConnectionID: "conn_1",
				Enabled:      true,
				Trigger: pipeline.Trigger{
					Key: "github.pull_request",
					Conditions: map[string]any{
						"event.action": "closed",
					},
				},
			},
			{
				ID:       "pipe_wrong_tenant",
				TenantID: "tenant_2",
				Enabled:  true,
				Trigger: pipeline.Trigger{
					Key: "github.pull_request",
				},
			},
			{
				ID:      "pipe_disabled",
				Enabled: false,
				Trigger: pipeline.Trigger{
					Key: "github.pull_request",
				},
			},
		},
	})
	require.NoError(t, err)

	event := envelope.Event{
		ID:           "evt_1",
		TenantID:     "tenant_1",
		ConnectionID: "conn_1",
		Provider:     "github",
		TriggerKey:   "github.pull_request",
		Normalized: map[string]any{
			"action": "opened",
			"repository": map[string]any{
				"full_name": "charliewilco/argus",
			},
		},
	}

	matched, err := matcher.Match(context.Background(), event)
	require.NoError(t, err)
	require.Len(t, matched, 1)
	require.Equal(t, "pipe_match", matched[0].ID)
}

func TestTriggerMatcherRejectsConnectionBoundPipelineWithoutEventConnection(t *testing.T) {
	t.Parallel()

	matcher, err := triggers.NewTriggerMatcher(&triggerStore{
		pipelines: []*pipeline.Pipeline{
			{
				ID:           "pipe_bound",
				TenantID:     "tenant_1",
				ConnectionID: "conn_1",
				Enabled:      true,
				Trigger: pipeline.Trigger{
					Key: "github.pull_request",
				},
			},
		},
	})
	require.NoError(t, err)

	matched, err := matcher.Match(context.Background(), envelope.Event{
		ID:         "evt_1",
		TenantID:   "tenant_1",
		TriggerKey: "github.pull_request",
	})
	require.NoError(t, err)
	require.Empty(t, matched)
}
