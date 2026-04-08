package pipeline

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
