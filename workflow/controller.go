package workflow

import (
	"context"
	"fmt"
	"sync"
	"time"
)

// ExecutionMode defines how a task should be executed.
type ExecutionMode string

const (
	// ModeAuto executes without requiring user approval (low-cost, read-only, unambiguous).
	ModeAuto ExecutionMode = "auto"

	// ModePlanReview shows the plan and allows modification before executing.
	ModePlanReview ExecutionMode = "plan_review"

	// ModeApprovalRequired requires explicit user approval before executing
	// (destructive, ambiguous, expensive, or modeling tasks).
	ModeApprovalRequired ExecutionMode = "approval_required"
)

// Decision is the structured output from the Main Agent.
// It defines the next action the system should take.
type Decision struct {
	Action      string         `json:"action"`
	ToolName    string         `json:"tool_name,omitempty"`
	SkillName   string         `json:"skill_name,omitempty"`
	Arguments   map[string]any `json:"arguments,omitempty"`
	Reason      string         `json:"reason"`
	TargetState State          `json:"target_state,omitempty"`
}

// TaskContext holds all information about the current task execution.
type TaskContext struct {
	TaskID    string        `json:"task_id"`
	State     State         `json:"state"`
	Budget    ComputeBudget `json:"budget"`
	Usage     Usage         `json:"usage"`
	WorkDir   string        `json:"work_dir"`
	Mode      ExecutionMode `json:"mode"`
	StartedAt time.Time     `json:"started_at"`
	UpdatedAt time.Time     `json:"updated_at"`

	mu sync.RWMutex
}

// NewTaskContext creates a new task context with the given ID and work directory.
func NewTaskContext(taskID, workDir string) *TaskContext {
	now := time.Now()
	return &TaskContext{
		TaskID:    taskID,
		State:     StateReceived,
		Budget:    DefaultBudget(),
		Usage:     Usage{},
		WorkDir:   workDir,
		Mode:      ModeAuto,
		StartedAt: now,
		UpdatedAt: now,
	}
}

// CurrentState returns the current state in a thread-safe manner.
func (tc *TaskContext) CurrentState() State {
	tc.mu.RLock()
	defer tc.mu.RUnlock()
	return tc.State
}

// Transition attempts to move the task to the target state.
// Returns an error if the transition is invalid or if budget is exceeded.
func (tc *TaskContext) Transition(ctx context.Context, target State) error {
	tc.mu.Lock()
	defer tc.mu.Unlock()

	if !tc.State.CanTransition(target) {
		return fmt.Errorf("invalid state transition: %s -> %s", tc.State, target)
	}

	// Check budget before allowing transition.
	if err := tc.Budget.Check(tc.Usage); err != nil {
		return fmt.Errorf("budget check failed for transition %s -> %s: %w", tc.State, target, err)
	}

	tc.State = target
	tc.Usage.Iterations++
	tc.UpdatedAt = time.Now()
	tc.Usage.ExecutionTime = time.Since(tc.StartedAt)

	return nil
}

// SetMode sets the execution mode for the task.
func (tc *TaskContext) SetMode(mode ExecutionMode) {
	tc.mu.Lock()
	defer tc.mu.Unlock()
	tc.Mode = mode
}

// RecordLLMCall records an LLM API call in the usage tracker.
func (tc *TaskContext) RecordLLMCall(inputTokens, outputTokens int, estimatedCost float64) {
	tc.mu.Lock()
	defer tc.mu.Unlock()
	tc.Usage.LLMCalls++
	tc.Usage.InputTokens += inputTokens
	tc.Usage.OutputTokens += outputTokens
	tc.Usage.EstimatedCost += estimatedCost
}

// RecordToolCall records a deterministic tool call in the usage tracker.
func (tc *TaskContext) RecordToolCall(rowsScanned int64) {
	tc.mu.Lock()
	defer tc.mu.Unlock()
	tc.Usage.ToolCalls++
	tc.Usage.RowsScanned += rowsScanned
}

// RecordRAGChunks records the number of RAG chunks injected into context.
func (tc *TaskContext) RecordRAGChunks(count int) {
	tc.mu.Lock()
	defer tc.mu.Unlock()
	tc.Usage.RAGChunks += count
}

// RecordRetry increments the retry counter.
func (tc *TaskContext) RecordRetry() {
	tc.mu.Lock()
	defer tc.mu.Unlock()
	tc.Usage.Retries++
}

// CheckBudget verifies that current usage is within budget.
func (tc *TaskContext) CheckBudget() error {
	tc.mu.RLock()
	defer tc.mu.RUnlock()
	return tc.Budget.Check(tc.Usage)
}

// IsActive returns true if the task is still being processed.
func (tc *TaskContext) IsActive() bool {
	tc.mu.RLock()
	defer tc.mu.RUnlock()
	return tc.State.IsActive()
}

// Snapshot returns a copy of the current task context state.
func (tc *TaskContext) Snapshot() TaskContext {
	tc.mu.RLock()
	defer tc.mu.RUnlock()
	return TaskContext{
		TaskID:    tc.TaskID,
		State:     tc.State,
		Budget:    tc.Budget,
		Usage:     tc.Usage,
		WorkDir:   tc.WorkDir,
		Mode:      tc.Mode,
		StartedAt: tc.StartedAt,
		UpdatedAt: tc.UpdatedAt,
	}
}

// Controller defines the interface for the workflow state machine controller.
// It validates and applies state transitions, manages budget, and coordinates
// the execution pipeline.
type Controller interface {
	// Receive initializes a new task and transitions to RECEIVED state.
	Receive(ctx context.Context, taskID, workDir string) (*TaskContext, error)

	// GetTask returns the current task context.
	GetTask(taskID string) (*TaskContext, error)

	// Transition attempts to move a task to the target state.
	// The LLM may recommend a transition, but the Controller validates and applies it.
	Transition(ctx context.Context, taskID string, target State) error

	// Fail marks a task as FAILED with an error message.
	Fail(ctx context.Context, taskID string, err error) error

	// Complete marks a task as DONE.
	Complete(ctx context.Context, taskID string) error
}
