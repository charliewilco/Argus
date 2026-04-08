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
	ID           string
	TenantID     string
	Name         string
	TriggerKey   string
	Trigger      Trigger
	ConnectionID string
	Steps        []Step
	Enabled      bool
}

type Step struct {
	ID      string
	Name    string
	Type    StepType
	Config  map[string]any
	OnError ErrorBehavior
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
		s.Config = decoded.Config
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
		case decoded.Condition != "":
			s.Type = StepTypeCondition
			s.Config = map[string]any{
				"expression": decoded.Condition,
			}
		}
	}

	if s.Config == nil {
		s.Config = map[string]any{}
	}

	return nil
}
