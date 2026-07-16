// Package hooks provides the lifecycle hook system for the single-agent V1 architecture.
//
// Hooks are deterministic code that run at specific points in the task lifecycle.
// They must NOT invoke LLM calls unless explicitly allowed by configuration.
// Hooks use explicit priority ordering to guarantee deterministic execution order.
package hooks

import (
	"context"
	"fmt"
	"sort"
	"sync"
)

// Priority defines the execution order for hooks. Lower numbers run first.
type Priority int

const (
	// PriorityHighest runs first (e.g., validation that must happen before anything else).
	PriorityHighest Priority = 0

	// PriorityHigh runs early.
	PriorityHigh Priority = 25

	// PriorityNormal is the default priority.
	PriorityNormal Priority = 50

	// PriorityLow runs late.
	PriorityLow Priority = 75

	// PriorityLowest runs last (e.g., cleanup hooks).
	PriorityLowest Priority = 100
)

// Event represents a lifecycle event where hooks can be registered.
type Event string

const (
	// EventBeforeTask fires before a task starts processing.
	EventBeforeTask Event = "before_task"

	// EventBeforePlan fires before a plan is generated.
	EventBeforePlan Event = "before_plan"

	// EventAfterPlan fires after a plan is generated.
	EventAfterPlan Event = "after_plan"

	// EventBeforeTool fires before a tool is invoked.
	EventBeforeTool Event = "before_tool"

	// EventAfterTool fires after a tool completes.
	EventAfterTool Event = "after_tool"

	// EventOnToolError fires when a tool returns an error.
	EventOnToolError Event = "on_tool_error"

	// EventBeforeVerify fires before verification runs.
	EventBeforeVerify Event = "before_verify"

	// EventAfterVerify fires after verification completes.
	EventAfterVerify Event = "after_verify"

	// EventBeforeReport fires before report generation.
	EventBeforeReport Event = "before_report"

	// EventAfterReport fires after report generation.
	EventAfterReport Event = "after_report"

	// EventAfterTask fires after a task completes (success or failure).
	EventAfterTask Event = "after_task"

	// EventOnFeedback fires when user feedback is received.
	EventOnFeedback Event = "on_feedback"
)

// HookContext provides the execution context for a hook.
type HookContext struct {
	TaskID     string         `json:"task_id"`
	Event      Event          `json:"event"`
	State      string         `json:"state"`
	ToolName   string         `json:"tool_name,omitempty"`
	ToolArgs   map[string]any `json:"tool_args,omitempty"`
	ToolResult any            `json:"tool_result,omitempty"`
	ToolError  error          `json:"tool_error,omitempty"`
	Metadata   map[string]any `json:"metadata,omitempty"`
}

// HookResult is returned by a hook to indicate whether to continue or abort.
type HookResult struct {
	// Continue is true if execution should proceed after the hook.
	Continue bool `json:"continue"`

	// Message is an optional human-readable message from the hook.
	Message string `json:"message,omitempty"`

	// Error is set when the hook encounters an error that should stop execution.
	Error error `json:"error,omitempty"`

	// Metadata can carry data between hooks or to downstream systems.
	Metadata map[string]any `json:"metadata,omitempty"`
}

// Hook is a single lifecycle hook. Each hook has a name, priority, event binding,
// and an Execute function. Hooks must be idempotent and fast.
type Hook interface {
	// Name returns a unique identifier for this hook.
	Name() string

	// Event returns the lifecycle event this hook is bound to.
	Event() Event

	// Priority returns the execution priority (lower = earlier).
	Priority() Priority

	// Execute runs the hook logic. It should not panic.
	Execute(ctx context.Context, hctx *HookContext) HookResult
}

// Registry manages the collection of registered hooks.
type Registry struct {
	mu    sync.RWMutex
	hooks map[Event][]Hook
}

// NewRegistry creates an empty hook registry.
func NewRegistry() *Registry {
	return &Registry{
		hooks: make(map[Event][]Hook),
	}
}

// Register adds a hook to the registry. Hooks are sorted by priority after insertion.
// Returns an error if a hook with the same name and event is already registered.
func (r *Registry) Register(h Hook) error {
	r.mu.Lock()
	defer r.mu.Unlock()

	event := h.Event()
	for _, existing := range r.hooks[event] {
		if existing.Name() == h.Name() {
			return fmt.Errorf("hook %q already registered for event %q", h.Name(), event)
		}
	}

	r.hooks[event] = append(r.hooks[event], h)
	r.sortEvent(event)
	return nil
}

// Unregister removes a hook by name from a specific event.
func (r *Registry) Unregister(name string, event Event) {
	r.mu.Lock()
	defer r.mu.Unlock()

	hooks := r.hooks[event]
	filtered := make([]Hook, 0, len(hooks))
	for _, h := range hooks {
		if h.Name() != name {
			filtered = append(filtered, h)
		}
	}
	r.hooks[event] = filtered
}

// GetHooks returns all hooks registered for an event, sorted by priority.
// Returns a copy to prevent external mutation.
func (r *Registry) GetHooks(event Event) []Hook {
	r.mu.RLock()
	defer r.mu.RUnlock()

	hooks := r.hooks[event]
	result := make([]Hook, len(hooks))
	copy(result, hooks)
	return result
}

// Count returns the total number of registered hooks across all events.
func (r *Registry) Count() int {
	r.mu.RLock()
	defer r.mu.RUnlock()

	count := 0
	for _, hooks := range r.hooks {
		count += len(hooks)
	}
	return count
}

// sortEvent sorts hooks for a specific event by priority. Must be called with lock held.
func (r *Registry) sortEvent(event Event) {
	sort.SliceStable(r.hooks[event], func(i, j int) bool {
		return r.hooks[event][i].Priority() < r.hooks[event][j].Priority()
	})
}

// Predefined hook names for built-in V1 hooks.
const (
	HookSchemaValidation    = "schema_validation"
	HookJoinCardinality     = "join_cardinality"
	HookPermission          = "permission"
	HookCostEstimation      = "cost_estimation"
	HookCacheLookup         = "cache_lookup"
	HookSnapshotBeforeWrite = "snapshot_before_write"
	HookResultSummary       = "result_summary"
	HookArtifactPersistence = "artifact_persistence"
	HookCacheWrite          = "cache_write"
	HookResultValidation    = "result_validation"
	HookExecutionTrace      = "execution_trace"
	HookSnapshotAfterWrite  = "snapshot_after_write"
)
