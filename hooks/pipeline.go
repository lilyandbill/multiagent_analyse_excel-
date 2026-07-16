package hooks

import (
	"context"
	"fmt"
)

// Pipeline executes a list of hooks in order for a given event and hook context.
// Execution stops early if any hook returns Continue=false.
// Pipeline is deterministic: hooks run in priority order, and identical
// inputs always produce the same execution order.
type Pipeline struct {
	registry *Registry
}

// NewPipeline creates a new hook execution pipeline backed by the given registry.
func NewPipeline(registry *Registry) *Pipeline {
	return &Pipeline{registry: registry}
}

// Run executes all hooks registered for the given event.
// Hooks are executed in priority order (lowest priority value first).
// If a hook returns Continue=false, execution stops immediately and
// the remaining hooks are skipped.
func (p *Pipeline) Run(ctx context.Context, event Event, hctx *HookContext) PipelineResult {
	hctx.Event = event

	hooks := p.registry.GetHooks(event)

	result := PipelineResult{
		Event:      event,
		TotalHooks: len(hooks),
		Results:    make([]HookResult, 0, len(hooks)),
	}

	for _, hook := range hooks {
		hr := hook.Execute(ctx, hctx)
		result.Results = append(result.Results, hr)

		if hr.Error != nil {
			result.Errors = append(result.Errors, PipelineError{
				HookName: hook.Name(),
				Error:    hr.Error,
			})
		}

		if !hr.Continue {
			result.Aborted = true
			result.AbortedBy = hook.Name()
			result.CompletedHooks = len(result.Results)
			return result
		}

		// Merge metadata from hook into context for downstream hooks.
		if hr.Metadata != nil {
			if hctx.Metadata == nil {
				hctx.Metadata = make(map[string]any)
			}
			for k, v := range hr.Metadata {
				hctx.Metadata[k] = v
			}
		}
	}

	result.CompletedHooks = len(result.Results)
	return result
}

// PipelineResult contains the aggregated results of executing a hook pipeline.
type PipelineResult struct {
	// Event is the lifecycle event that was processed.
	Event Event `json:"event"`

	// TotalHooks is the number of hooks registered for this event.
	TotalHooks int `json:"total_hooks"`

	// CompletedHooks is the number of hooks that were actually executed.
	CompletedHooks int `json:"completed_hooks"`

	// Aborted is true if execution was stopped by a hook returning Continue=false.
	Aborted bool `json:"aborted"`

	// AbortedBy is the name of the hook that caused the abort (empty if not aborted).
	AbortedBy string `json:"aborted_by,omitempty"`

	// Results contains the individual results from each executed hook.
	Results []HookResult `json:"results"`

	// Errors contains any errors encountered during hook execution.
	Errors []PipelineError `json:"errors,omitempty"`
}

// PipelineError records an error from a specific hook.
type PipelineError struct {
	HookName string `json:"hook_name"`
	Error    error  `json:"error"`
}

// AllPassed returns true if all hooks executed and none aborted or errored.
func (pr PipelineResult) AllPassed() bool {
	return !pr.Aborted && len(pr.Errors) == 0
}

// HasErrors returns true if any hook returned an error.
func (pr PipelineResult) HasErrors() bool {
	return len(pr.Errors) > 0
}

// Summary returns a human-readable summary of the pipeline execution.
func (pr PipelineResult) Summary() string {
	if pr.Aborted {
		return fmt.Sprintf("pipeline aborted by hook %q after %d/%d hooks",
			pr.AbortedBy, pr.CompletedHooks, pr.TotalHooks)
	}
	if len(pr.Errors) > 0 {
		return fmt.Sprintf("pipeline completed with %d errors (%d/%d hooks)",
			len(pr.Errors), pr.CompletedHooks, pr.TotalHooks)
	}
	return fmt.Sprintf("pipeline completed successfully (%d/%d hooks)",
		pr.CompletedHooks, pr.TotalHooks)
}

// RunPreHooks is a convenience function that runs all hooks for a "before_*" event.
// It returns true if execution should continue (no hook aborted).
func (p *Pipeline) RunPreHooks(ctx context.Context, event Event, hctx *HookContext) (bool, *PipelineResult) {
	result := p.Run(ctx, event, hctx)
	return !result.Aborted, &result
}

// RunPostHooks is a convenience function that runs all hooks for an "after_*" event.
// Post-hooks may abort, but typically they only observe and record.
func (p *Pipeline) RunPostHooks(ctx context.Context, event Event, hctx *HookContext) *PipelineResult {
	result := p.Run(ctx, event, hctx)
	return &result
}
