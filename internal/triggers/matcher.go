package triggers

import (
	"context"
	"fmt"

	"github.com/charliewilco/argus/internal/envelope"
	"github.com/charliewilco/argus/internal/pipeline"
)

type pipelineStore interface {
	ListPipelines(ctx context.Context, tenantID string) ([]*pipeline.Pipeline, error)
}

type TriggerMatcher struct {
	store pipelineStore
}

func NewTriggerMatcher(store pipelineStore) (*TriggerMatcher, error) {
	if store == nil {
		return nil, fmt.Errorf("triggers.NewTriggerMatcher: store is required")
	}

	return &TriggerMatcher{store: store}, nil
}

func (m *TriggerMatcher) Match(ctx context.Context, event envelope.Event) ([]pipeline.Pipeline, error) {
	pipelines, err := m.store.ListPipelines(ctx, "")
	if err != nil {
		return nil, fmt.Errorf("triggers.Match: list pipelines: %w", err)
	}

	context := map[string]any{
		"event": map[string]any{
			"id":            event.ID,
			"tenant_id":     event.TenantID,
			"connection_id": event.ConnectionID,
			"provider":      event.Provider,
			"trigger_key":   event.TriggerKey,
		},
	}
	for key, value := range event.Normalized {
		context["event"].(map[string]any)[key] = value
	}

	matched := make([]pipeline.Pipeline, 0)
	for _, item := range pipelines {
		if item == nil || !item.Enabled {
			continue
		}

		item.Normalize()
		if item.TenantID != "" && event.TenantID != "" && item.TenantID != event.TenantID {
			continue
		}
		if item.ConnectionID != "" && item.ConnectionID != event.ConnectionID {
			continue
		}
		if item.Trigger.Key != "" && item.Trigger.Key != event.TriggerKey {
			continue
		}
		if len(item.Trigger.Conditions) > 0 && !pipeline.MatchConditions(context, item.Trigger.Conditions) {
			continue
		}

		matched = append(matched, *item)
	}

	return matched, nil
}
