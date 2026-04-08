package pipeline

import "encoding/json"

type ErrorBehavior string
type StepType string

const (
	StepTypeAction    StepType = "action"
	StepTypeCondition StepType = "condition"
	StepTypeTransform StepType = "transform"
)

const (
	ErrorBehaviorContinue ErrorBehavior = "continue"
	ErrorBehaviorAbort    ErrorBehavior = "abort"
	ErrorBehaviorRetry    ErrorBehavior = "retry"
)

type Trigger struct {
	Key        string         `json:"key"`
	Conditions map[string]any `json:"conditions,omitempty"`
}

type Pipeline struct {
	ID           string  `json:"id"`
	TenantID     string  `json:"tenant_id"`
	Name         string  `json:"name"`
	TriggerKey   string  `json:"trigger_key"`
	Trigger      Trigger `json:"trigger"`
	ConnectionID string  `json:"connection_id"`
	Steps        []Step  `json:"steps"`
	Enabled      bool    `json:"enabled"`
	enabledSet   bool
}

type Step struct {
	ID      string         `json:"id"`
	Name    string         `json:"name"`
	Type    StepType       `json:"type"`
	Config  map[string]any `json:"config"`
	OnError ErrorBehavior  `json:"on_error,omitempty"`
}

func (p *Pipeline) SetEnabled(enabled bool) {
	p.Enabled = enabled
	p.enabledSet = true
}

func (p Pipeline) HasExplicitEnabled() bool {
	return p.enabledSet
}

func (p *Pipeline) Normalize() {
	if p.Trigger.Key == "" {
		p.Trigger.Key = p.TriggerKey
	}
	if p.TriggerKey == "" {
		p.TriggerKey = p.Trigger.Key
	}
	if p.Trigger.Conditions == nil {
		p.Trigger.Conditions = map[string]any{}
	}
	if p.Steps == nil {
		p.Steps = []Step{}
	}
}

func (p *Pipeline) UnmarshalJSON(data []byte) error {
	type decodedPipeline struct {
		ID           string  `json:"id"`
		TenantID     string  `json:"tenant_id"`
		Name         string  `json:"name"`
		TriggerKey   string  `json:"trigger_key"`
		Trigger      Trigger `json:"trigger"`
		ConnectionID string  `json:"connection_id"`
		Steps        []Step  `json:"steps"`
		Enabled      *bool   `json:"enabled"`
	}

	var decoded decodedPipeline
	if err := json.Unmarshal(data, &decoded); err != nil {
		return err
	}

	p.ID = decoded.ID
	p.TenantID = decoded.TenantID
	p.Name = decoded.Name
	p.TriggerKey = decoded.TriggerKey
	p.Trigger = decoded.Trigger
	p.ConnectionID = decoded.ConnectionID
	p.Steps = decoded.Steps
	if decoded.Enabled != nil {
		p.SetEnabled(*decoded.Enabled)
	} else {
		p.Enabled = false
		p.enabledSet = false
	}

	p.Normalize()

	return nil
}

func (s *Step) UnmarshalJSON(data []byte) error {
	type stepAlias Step
	type legacyStep struct {
		ID         string         `json:"id"`
		Name       string         `json:"name"`
		Type       StepType       `json:"type"`
		Config     map[string]any `json:"config"`
		Action     string         `json:"action"`
		Connection string         `json:"connection"`
		Input      map[string]any `json:"input"`
		Expression string         `json:"expression"`
		Condition  string         `json:"condition"`
		OnError    ErrorBehavior  `json:"on_error"`
	}

	var decoded legacyStep
	if err := json.Unmarshal(data, &decoded); err != nil {
		return err
	}

	s.ID = decoded.ID
	s.Name = decoded.Name
	s.Type = decoded.Type
	s.OnError = decoded.OnError

	if decoded.Config != nil {
		s.Config = normalizeStepConfig(decoded.Type, decoded.Config)
	}

	if s.Type == "" {
		switch {
		case decoded.Action != "":
			s.Type = StepTypeAction
			if s.Config == nil {
				s.Config = map[string]any{}
			}
			s.Config["action"] = decoded.Action
			for key, value := range decoded.Input {
				s.Config[key] = value
			}
			if decoded.Connection != "" {
				s.Config["connection_id"] = decoded.Connection
			}
		case decoded.Condition != "" || decoded.Expression != "":
			expression := decoded.Condition
			if expression == "" {
				expression = decoded.Expression
			}
			s.Type = StepTypeCondition
			s.Config = conditionConfigFromExpression(expression)
		}
	}

	if s.Config == nil {
		s.Config = map[string]any{}
	}
	if s.Type == StepTypeCondition && len(s.Config) == 0 {
		expression := decoded.Condition
		if expression == "" {
			expression = decoded.Expression
		}
		if expression != "" {
			s.Config = conditionConfigFromExpression(expression)
		}
	}
	if s.Type == StepTypeCondition {
		s.Config = normalizeConditionStepConfig(s.Config)
	}

	return nil
}

func normalizeStepConfig(stepType StepType, config map[string]any) map[string]any {
	if stepType != StepTypeCondition {
		return config
	}

	return normalizeConditionStepConfig(config)
}

func normalizeConditionStepConfig(config map[string]any) map[string]any {
	if config == nil {
		return map[string]any{}
	}

	normalized := make(map[string]any, len(config))
	for key, value := range config {
		normalized[key] = value
	}

	expression, _ := normalized["expression"].(string)
	delete(normalized, "expression")

	conditions := make(map[string]any)
	if rawConditions, ok := normalized["conditions"].(map[string]any); ok {
		for key, value := range rawConditions {
			if key == "expression" {
				if nestedExpression, ok := value.(string); ok && nestedExpression != "" {
					expression = nestedExpression
				}
				continue
			}
			conditions[key] = value
		}
	}

	if expression != "" {
		conditions[expression] = true
	}
	if len(conditions) > 0 {
		normalized["conditions"] = conditions
	} else {
		delete(normalized, "conditions")
	}

	return normalized
}

func conditionConfigFromExpression(expression string) map[string]any {
	if expression == "" {
		return map[string]any{}
	}

	return map[string]any{
		"conditions": map[string]any{
			expression: true,
		},
	}
}
