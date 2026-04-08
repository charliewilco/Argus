package pipeline

import (
	"encoding/json"
	"fmt"
)

type ErrorBehavior string

const (
	ErrorBehaviorContinue ErrorBehavior = "continue"
	ErrorBehaviorAbort    ErrorBehavior = "abort"
	ErrorBehaviorRetry    ErrorBehavior = "retry"
)

type Pipeline struct {
	ID           string
	TenantID     string
	Name         string
	TriggerKey   string
	ConnectionID string
	Steps        []Step
	Enabled      bool

	enabledSet bool
}

type Step struct {
	ID         string
	Name       string
	Action     string
	Connection string
	Input      map[string]any
	Condition  string
	OnError    ErrorBehavior
}

type pipelineJSON struct {
	ID              string `json:"id,omitempty"`
	TenantID        string `json:"tenantId,omitempty"`
	TenantIDAlt     string `json:"tenant_id,omitempty"`
	Name            string `json:"name,omitempty"`
	TriggerKey      string `json:"triggerKey,omitempty"`
	TriggerKeyAlt   string `json:"trigger_key,omitempty"`
	ConnectionID    string `json:"connectionId,omitempty"`
	ConnectionIDAlt string `json:"connection_id,omitempty"`
	Steps           []Step `json:"steps,omitempty"`
	Enabled         *bool  `json:"enabled,omitempty"`
}

type stepJSON struct {
	ID         string          `json:"id,omitempty"`
	Name       string          `json:"name,omitempty"`
	Action     string          `json:"action,omitempty"`
	Connection string          `json:"connection,omitempty"`
	Input      map[string]any  `json:"input,omitempty"`
	Condition  json.RawMessage `json:"condition,omitempty"`
	Expression string          `json:"expression,omitempty"`
	Conditions json.RawMessage `json:"conditions,omitempty"`
	OnError    ErrorBehavior   `json:"onError,omitempty"`
	OnErrorAlt ErrorBehavior   `json:"on_error,omitempty"`
}

type stepConditions struct {
	Expression string `json:"expression,omitempty"`
}

func (p *Pipeline) SetEnabled(enabled bool) {
	if p == nil {
		return
	}

	p.Enabled = enabled
	p.enabledSet = true
}

func (p Pipeline) HasExplicitEnabled() bool {
	return p.enabledSet
}

func (p Pipeline) MarshalJSON() ([]byte, error) {
	payload := pipelineJSON{
		ID:           p.ID,
		TenantID:     p.TenantID,
		Name:         p.Name,
		TriggerKey:   p.TriggerKey,
		ConnectionID: p.ConnectionID,
		Steps:        p.Steps,
	}

	if p.Enabled || p.enabledSet {
		enabled := p.Enabled
		payload.Enabled = &enabled
	}

	return json.Marshal(payload)
}

func (p *Pipeline) UnmarshalJSON(data []byte) error {
	var payload pipelineJSON
	if err := json.Unmarshal(data, &payload); err != nil {
		return err
	}

	p.ID = payload.ID
	p.TenantID = firstNonEmpty(payload.TenantID, payload.TenantIDAlt)
	p.Name = payload.Name
	p.TriggerKey = firstNonEmpty(payload.TriggerKey, payload.TriggerKeyAlt)
	p.ConnectionID = firstNonEmpty(payload.ConnectionID, payload.ConnectionIDAlt)
	p.Steps = payload.Steps
	if p.Steps == nil {
		p.Steps = []Step{}
	}

	p.Enabled = true
	p.enabledSet = payload.Enabled != nil
	if payload.Enabled != nil {
		p.Enabled = *payload.Enabled
	}

	return nil
}

func (s Step) MarshalJSON() ([]byte, error) {
	type stepMarshalJSON struct {
		ID         string          `json:"id,omitempty"`
		Name       string          `json:"name,omitempty"`
		Action     string          `json:"action,omitempty"`
		Connection string          `json:"connection,omitempty"`
		Input      map[string]any  `json:"input,omitempty"`
		Conditions *stepConditions `json:"conditions,omitempty"`
		OnError    ErrorBehavior   `json:"onError,omitempty"`
	}

	payload := stepMarshalJSON{
		ID:         s.ID,
		Name:       s.Name,
		Action:     s.Action,
		Connection: s.Connection,
		Input:      s.Input,
		OnError:    s.OnError,
	}

	if s.Condition != "" {
		payload.Conditions = &stepConditions{
			Expression: s.Condition,
		}
	}

	return json.Marshal(payload)
}

func (s *Step) UnmarshalJSON(data []byte) error {
	var payload stepJSON
	if err := json.Unmarshal(data, &payload); err != nil {
		return err
	}

	*s = Step{
		ID:         payload.ID,
		Name:       payload.Name,
		Action:     payload.Action,
		Connection: payload.Connection,
		Input:      payload.Input,
		OnError:    payload.OnError,
	}
	if s.OnError == "" {
		s.OnError = payload.OnErrorAlt
	}

	switch {
	case payload.Expression != "":
		s.Condition = payload.Expression
	default:
		condition, ok, err := decodeConditionExpression(payload.Conditions)
		if err != nil {
			return fmt.Errorf("pipeline.Step.UnmarshalJSON: decode conditions: %w", err)
		}
		if !ok {
			condition, ok, err = decodeConditionExpression(payload.Condition)
			if err != nil {
				return fmt.Errorf("pipeline.Step.UnmarshalJSON: decode condition: %w", err)
			}
		}
		if ok {
			s.Condition = condition
		}
	}

	return nil
}

func decodeConditionExpression(data json.RawMessage) (string, bool, error) {
	if len(data) == 0 || string(data) == "null" {
		return "", false, nil
	}

	var expression string
	if err := json.Unmarshal(data, &expression); err == nil {
		if expression == "" {
			return "", false, nil
		}

		return expression, true, nil
	}

	var wrapped stepConditions
	if err := json.Unmarshal(data, &wrapped); err == nil && wrapped.Expression != "" {
		return wrapped.Expression, true, nil
	}

	var conditions map[string]any
	if err := json.Unmarshal(data, &conditions); err == nil {
		for key, value := range conditions {
			matched, ok := value.(bool)
			if ok && matched {
				return key, true, nil
			}
		}

		return "", false, nil
	}

	return "", false, fmt.Errorf("unsupported condition payload %s", string(data))
}

func firstNonEmpty(values ...string) string {
	for _, value := range values {
		if value != "" {
			return value
		}
	}

	return ""
}
