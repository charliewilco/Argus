package pipeline

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"time"

	"github.com/charliewilco/argus/internal/envelope"
	"github.com/charliewilco/argus/internal/queue"
	"github.com/charliewilco/argus/providers"
)

const FailedJobTypePipelineExecution = "pipeline_execution"

var ErrExecutionFailed = errors.New("pipeline: execution failed")

type ActionDispatcher interface {
	Dispatch(ctx context.Context, stepConfig map[string]any, connectionID string, event *envelope.Event) (providers.ActionResult, error)
}

type DeadLetterQueue interface {
	PushFailed(ctx context.Context, id, jobType string, payload []byte, reason string, attemptCount int, failedAt time.Time) error
}

type ExecutionStatus string

const (
	ExecutionStatusSucceeded ExecutionStatus = "succeeded"
	ExecutionStatusFailed    ExecutionStatus = "failed"
)

type StepStatus string

const (
	StepStatusSucceeded StepStatus = "succeeded"
	StepStatusFailed    StepStatus = "failed"
)

type StepResult struct {
	StepID     string         `json:"step_id"`
	Type       StepType       `json:"type"`
	Status     StepStatus     `json:"status"`
	Output     map[string]any `json:"output,omitempty"`
	Error      string         `json:"error,omitempty"`
	FinishedAt time.Time      `json:"finished_at"`
}

type ExecutionResult struct {
	PipelineID   string          `json:"pipeline_id"`
	EventID      string          `json:"event_id"`
	Status       ExecutionStatus `json:"status"`
	StepOutcomes []StepResult    `json:"step_outcomes"`
}

type ExecutionError struct {
	StepID string
	Err    error
}

func (e *ExecutionError) Error() string {
	if e == nil {
		return ErrExecutionFailed.Error()
	}

	return fmt.Sprintf("pipeline.Execute: step %q: %v", e.StepID, e.Err)
}

func (e *ExecutionError) Unwrap() error {
	if e == nil {
		return ErrExecutionFailed
	}

	return errors.Join(ErrExecutionFailed, e.Err)
}

type Executor struct {
	dispatcher ActionDispatcher
	dlq        DeadLetterQueue
	now        func() time.Time
}

func NewExecutor(dispatcher ActionDispatcher, dlq DeadLetterQueue, now func() time.Time) (*Executor, error) {
	if dispatcher == nil {
		return nil, fmt.Errorf("pipeline.NewExecutor: dispatcher is required")
	}
	if dlq == nil {
		return nil, fmt.Errorf("pipeline.NewExecutor: DLQ is required")
	}
	if now == nil {
		now = time.Now
	}

	return &Executor{
		dispatcher: dispatcher,
		dlq:        dlq,
		now:        now,
	}, nil
}

func (e *Executor) Execute(ctx context.Context, value Pipeline, event envelope.Event) (ExecutionResult, error) {
	value.Normalize()

	result := ExecutionResult{
		PipelineID:   value.ID,
		EventID:      event.ID,
		Status:       ExecutionStatusSucceeded,
		StepOutcomes: make([]StepResult, 0, len(value.Steps)),
	}

	stepOutputs := make(map[string]map[string]any, len(value.Steps))
	for _, step := range value.Steps {
		stepResult, err := e.executeStep(ctx, value, step, &event, stepOutputs)
		result.StepOutcomes = append(result.StepOutcomes, stepResult)
		if err != nil {
			result.Status = ExecutionStatusFailed

			if dlqErr := e.pushFailedJob(ctx, value, event, err); dlqErr != nil {
				return result, fmt.Errorf("pipeline.Execute: push failed job: %w", dlqErr)
			}

			return result, &ExecutionError{
				StepID: step.ID,
				Err:    err,
			}
		}
		if stepResult.Output != nil {
			stepOutputs[step.ID] = stepResult.Output
		}
	}

	return result, nil
}

func (e *Executor) executeStep(ctx context.Context, value Pipeline, step Step, event *envelope.Event, stepOutputs map[string]map[string]any) (StepResult, error) {
	stepConfig := step.Config
	if stepConfig == nil {
		stepConfig = map[string]any{}
	}

	context := executionContext(value, event, stepOutputs)
	resolvedConfig, _ := ResolveTemplates(stepConfig, context).(map[string]any)
	if resolvedConfig == nil {
		resolvedConfig = map[string]any{}
	}

	stepResult := StepResult{
		StepID:     step.ID,
		Type:       step.Type,
		Status:     StepStatusSucceeded,
		FinishedAt: e.now().UTC(),
	}

	switch step.Type {
	case StepTypeCondition:
		conditions, _ := resolvedConfig["conditions"].(map[string]any)
		if len(conditions) == 0 {
			stepResult.Status = StepStatusFailed
			stepResult.Error = "condition step requires conditions"
			return stepResult, fmt.Errorf("condition step requires conditions")
		}
		if !MatchConditions(context, conditions) {
			stepResult.Status = StepStatusFailed
			stepResult.Error = "condition did not match"
			return stepResult, fmt.Errorf("condition did not match")
		}
		stepResult.Output = map[string]any{"matched": true}
		return stepResult, nil
	case StepTypeTransform:
		output, _ := resolvedConfig["output"].(map[string]any)
		if output == nil {
			output = resolvedConfig
		}
		stepResult.Output, _ = ResolveTemplates(output, context).(map[string]any)
		if stepResult.Output == nil {
			stepResult.Output = map[string]any{}
		}
		return stepResult, nil
	case StepTypeAction:
		connectionID, _ := resolvedConfig["connection_id"].(string)
		if connectionID == "" {
			connectionID = value.ConnectionID
		}

		actionResult, err := e.dispatcher.Dispatch(ctx, resolvedConfig, connectionID, event)
		if err != nil {
			stepResult.Status = StepStatusFailed
			stepResult.Error = err.Error()
			return stepResult, err
		}
		stepResult.Output = map[string]any{
			"provider": actionResult.Provider,
			"action":   actionResult.Action,
			"status":   actionResult.Status,
			"output":   actionResult.Output,
		}
		return stepResult, nil
	default:
		stepResult.Status = StepStatusFailed
		stepResult.Error = "unsupported step type"
		return stepResult, fmt.Errorf("unsupported step type %q", step.Type)
	}
}

func (e *Executor) pushFailedJob(ctx context.Context, value Pipeline, event envelope.Event, reason error) error {
	job := queue.Job{
		ID:          fmt.Sprintf("%s:%s", value.ID, event.ID),
		EventID:     event.ID,
		Attempt:     1,
		AvailableAt: e.now().UTC(),
		LastError:   reason.Error(),
		Payload: map[string]any{
			"pipeline_id": value.ID,
			"tenant_id":   value.TenantID,
			"event_id":    event.ID,
		},
	}

	payload, err := json.Marshal(job)
	if err != nil {
		return fmt.Errorf("pipeline.pushFailedJob: marshal job: %w", err)
	}

	if err := e.dlq.PushFailed(
		ctx,
		job.ID,
		FailedJobTypePipelineExecution,
		payload,
		reason.Error(),
		job.Attempt,
		e.now().UTC(),
	); err != nil {
		return fmt.Errorf("pipeline.pushFailedJob: %w", err)
	}

	return nil
}

func executionContext(value Pipeline, event *envelope.Event, stepOutputs map[string]map[string]any) map[string]any {
	steps := make(map[string]any, len(stepOutputs))
	for stepID, output := range stepOutputs {
		steps[stepID] = map[string]any{"output": output}
	}

	eventData := map[string]any{
		"id":            event.ID,
		"tenant_id":     event.TenantID,
		"connection_id": event.ConnectionID,
		"provider":      event.Provider,
		"trigger_key":   event.TriggerKey,
	}
	for key, entry := range event.Normalized {
		eventData[key] = entry
	}

	return map[string]any{
		"event": eventData,
		"pipeline": map[string]any{
			"id":            value.ID,
			"tenant_id":     value.TenantID,
			"connection_id": value.ConnectionID,
		},
		"steps": steps,
	}
}
