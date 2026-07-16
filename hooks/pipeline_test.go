package hooks

import (
	"context"
	"errors"
	"testing"
)

func TestPipeline_Run_AllPass(t *testing.T) {
	r := NewRegistry()
	r.Register(&testHook{name: "h1", event: EventBeforeTool, priority: PriorityHigh,
		execute: func(ctx context.Context, hctx *HookContext) HookResult {
			return HookResult{Continue: true}
		},
	})
	r.Register(&testHook{name: "h2", event: EventBeforeTool, priority: PriorityLow,
		execute: func(ctx context.Context, hctx *HookContext) HookResult {
			return HookResult{Continue: true}
		},
	})

	p := NewPipeline(r)
	result := p.Run(context.Background(), EventBeforeTool, &HookContext{TaskID: "test"})

	if result.TotalHooks != 2 {
		t.Errorf("TotalHooks = %d, want 2", result.TotalHooks)
	}
	if result.CompletedHooks != 2 {
		t.Errorf("CompletedHooks = %d, want 2", result.CompletedHooks)
	}
	if result.Aborted {
		t.Error("should not be aborted")
	}
	if !result.AllPassed() {
		t.Error("AllPassed should be true")
	}
}

func TestPipeline_Run_AbortOnFirstHook(t *testing.T) {
	r := NewRegistry()
	r.Register(&testHook{name: "h1", event: EventBeforeTool, priority: PriorityHigh,
		execute: func(ctx context.Context, hctx *HookContext) HookResult {
			return HookResult{Continue: false, Message: "aborted by h1"}
		},
	})
	r.Register(&testHook{name: "h2", event: EventBeforeTool, priority: PriorityLow,
		execute: func(ctx context.Context, hctx *HookContext) HookResult {
			return HookResult{Continue: true}
		},
	})

	p := NewPipeline(r)
	result := p.Run(context.Background(), EventBeforeTool, &HookContext{TaskID: "test"})

	if !result.Aborted {
		t.Error("should be aborted")
	}
	if result.AbortedBy != "h1" {
		t.Errorf("AbortedBy = %q, want %q", result.AbortedBy, "h1")
	}
	if result.CompletedHooks != 1 {
		t.Errorf("CompletedHooks = %d, want 1", result.CompletedHooks)
	}
	if result.AllPassed() {
		t.Error("AllPassed should be false when aborted")
	}
}

func TestPipeline_Run_ErrorDoesNotAbort(t *testing.T) {
	r := NewRegistry()
	r.Register(&testHook{name: "h1", event: EventBeforeTool, priority: PriorityHigh,
		execute: func(ctx context.Context, hctx *HookContext) HookResult {
			return HookResult{Continue: true, Error: errors.New("non-fatal error")}
		},
	})
	r.Register(&testHook{name: "h2", event: EventBeforeTool, priority: PriorityLow,
		execute: func(ctx context.Context, hctx *HookContext) HookResult {
			return HookResult{Continue: true}
		},
	})

	p := NewPipeline(r)
	result := p.Run(context.Background(), EventBeforeTool, &HookContext{TaskID: "test"})

	if result.Aborted {
		t.Error("should not abort on non-fatal errors")
	}
	if !result.HasErrors() {
		t.Error("HasErrors should be true")
	}
	if result.CompletedHooks != 2 {
		t.Errorf("CompletedHooks = %d, want 2", result.CompletedHooks)
	}
	if result.AllPassed() {
		t.Error("AllPassed should be false when there are errors")
	}
}

func TestPipeline_Run_MetadataPropagation(t *testing.T) {
	r := NewRegistry()
	r.Register(&testHook{name: "h1", event: EventBeforeTool, priority: PriorityHigh,
		execute: func(ctx context.Context, hctx *HookContext) HookResult {
			return HookResult{Continue: true, Metadata: map[string]any{"key1": "val1"}}
		},
	})
	r.Register(&testHook{name: "h2", event: EventBeforeTool, priority: PriorityLow,
		execute: func(ctx context.Context, hctx *HookContext) HookResult {
			// Verify metadata from h1 is available
			if v, ok := hctx.Metadata["key1"]; !ok || v != "val1" {
				t.Errorf("metadata not propagated, got %v", hctx.Metadata)
			}
			return HookResult{Continue: true, Metadata: map[string]any{"key2": "val2"}}
		},
	})

	p := NewPipeline(r)
	hctx := &HookContext{TaskID: "test"}
	result := p.Run(context.Background(), EventBeforeTool, hctx)

	if result.Aborted {
		t.Error("should not be aborted")
	}
	if hctx.Metadata["key1"] != "val1" {
		t.Errorf("key1 = %v, want val1", hctx.Metadata["key1"])
	}
	if hctx.Metadata["key2"] != "val2" {
		t.Errorf("key2 = %v, want val2", hctx.Metadata["key2"])
	}
}

func TestPipeline_Run_EmptyRegistry(t *testing.T) {
	r := NewRegistry()
	p := NewPipeline(r)
	result := p.Run(context.Background(), EventBeforeTool, &HookContext{TaskID: "test"})

	if result.TotalHooks != 0 {
		t.Errorf("TotalHooks = %d, want 0", result.TotalHooks)
	}
	if result.Aborted {
		t.Error("should not abort with empty registry")
	}
	if !result.AllPassed() {
		t.Error("AllPassed should be true for empty registry")
	}
}

func TestPipeline_RunPreHooks(t *testing.T) {
	r := NewRegistry()
	r.Register(&testHook{name: "h1", event: EventBeforeTool, priority: PriorityHigh,
		execute: func(ctx context.Context, hctx *HookContext) HookResult {
			return HookResult{Continue: true}
		},
	})

	p := NewPipeline(r)
	shouldContinue, result := p.RunPreHooks(context.Background(), EventBeforeTool, &HookContext{TaskID: "test"})

	if !shouldContinue {
		t.Error("RunPreHooks should return true when no hook aborts")
	}
	if result.TotalHooks != 1 {
		t.Errorf("TotalHooks = %d, want 1", result.TotalHooks)
	}
}

func TestPipeline_RunPreHooks_Abort(t *testing.T) {
	r := NewRegistry()
	r.Register(&testHook{name: "h1", event: EventBeforeTool, priority: PriorityHigh,
		execute: func(ctx context.Context, hctx *HookContext) HookResult {
			return HookResult{Continue: false}
		},
	})

	p := NewPipeline(r)
	shouldContinue, result := p.RunPreHooks(context.Background(), EventBeforeTool, &HookContext{TaskID: "test"})

	if shouldContinue {
		t.Error("RunPreHooks should return false when a hook aborts")
	}
	if !result.Aborted {
		t.Error("result should show aborted")
	}
}

func TestPipeline_RunPostHooks(t *testing.T) {
	r := NewRegistry()
	r.Register(&testHook{name: "h1", event: EventAfterTool, priority: PriorityNormal,
		execute: func(ctx context.Context, hctx *HookContext) HookResult {
			return HookResult{Continue: true}
		},
	})

	p := NewPipeline(r)
	result := p.RunPostHooks(context.Background(), EventAfterTool, &HookContext{TaskID: "test"})

	if result.TotalHooks != 1 {
		t.Errorf("TotalHooks = %d, want 1", result.TotalHooks)
	}
	if result.Aborted {
		t.Error("should not abort")
	}
}

func TestPipelineResult_Summary(t *testing.T) {
	r := NewRegistry()
	r.Register(&testHook{name: "h1", event: EventBeforeTool, priority: PriorityHigh,
		execute: func(ctx context.Context, hctx *HookContext) HookResult {
			return HookResult{Continue: false, Message: "stop"}
		},
	})

	p := NewPipeline(r)
	result := p.Run(context.Background(), EventBeforeTool, &HookContext{TaskID: "test"})

	summary := result.Summary()
	if summary == "" {
		t.Error("summary should not be empty")
	}
	t.Logf("aborted summary: %s", summary)
}
