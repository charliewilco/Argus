package envelope

import "time"

type Action struct {
	ID           string
	TenantID     string
	PipelineID   string
	StepID       string
	ConnectionID string
	Provider     string
	ActionKey    string
	Input        map[string]any
	Output       map[string]any
	CreatedAt    time.Time
}
