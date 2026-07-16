package hooks

import (
	"context"
	"errors"
	"testing"
)

// testHook is a simple hook implementation for testing.
type testHook struct {
	name     string
	event    Event
	priority Priority
	execute  func(ctx context.Context, hctx *HookContext) HookResult
}

func (h *testHook) Name() string       { return h.name }
func (h *testHook) Event() Event       { return h.event }
func (h *testHook) Priority() Priority { return h.priority }
func (h *testHook) Execute(ctx context.Context, hctx *HookContext) HookResult {
	if h.execute != nil {
		return h.execute(ctx, hctx)
	}
	return HookResult{Continue: true}
}

func TestRegistry_Register(t *testing.T) {
	r := NewRegistry()

	h := &testHook{name: "test_hook", event: EventBeforeTool, priority: PriorityNormal}
	if err := r.Register(h); err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if r.Count() != 1 {
		t.Errorf("count = %d, want 1", r.Count())
	}
}

func TestRegistry_Register_Duplicate(t *testing.T) {
	r := NewRegistry()

	r.Register(&testHook{name: "test_hook", event: EventBeforeTool, priority: PriorityNormal})
	err := r.Register(&testHook{name: "test_hook", event: EventBeforeTool, priority: PriorityLow})
	if err == nil {
		t.Fatal("expected duplicate registration error")
	}
}

func TestRegistry_Register_SameNameDifferentEvent(t *testing.T) {
	r := NewRegistry()

	r.Register(&testHook{name: "test_hook", event: EventBeforeTool, priority: PriorityNormal})
	err := r.Register(&testHook{name: "test_hook", event: EventAfterTool, priority: PriorityNormal})
	if err != nil {
		t.Fatalf("same name on different events should be allowed: %v", err)
	}
	if r.Count() != 2 {
		t.Errorf("count = %d, want 2", r.Count())
	}
}

func TestRegistry_Unregister(t *testing.T) {
	r := NewRegistry()

	r.Register(&testHook{name: "test_hook", event: EventBeforeTool, priority: PriorityNormal})
	r.Unregister("test_hook", EventBeforeTool)
	if r.Count() != 0 {
		t.Errorf("count = %d, want 0 after unregister", r.Count())
	}
}

func TestRegistry_GetHooks_SortedByPriority(t *testing.T) {
	r := NewRegistry()

	// Register in reverse priority order.
	r.Register(&testHook{name: "low", event: EventBeforeTool, priority: PriorityLow})
	r.Register(&testHook{name: "high", event: EventBeforeTool, priority: PriorityHigh})
	r.Register(&testHook{name: "normal", event: EventBeforeTool, priority: PriorityNormal})

	hooks := r.GetHooks(EventBeforeTool)
	if len(hooks) != 3 {
		t.Fatalf("expected 3 hooks, got %d", len(hooks))
	}
	if hooks[0].Name() != "high" {
		t.Errorf("hooks[0] = %q, want %q", hooks[0].Name(), "high")
	}
	if hooks[1].Name() != "normal" {
		t.Errorf("hooks[1] = %q, want %q", hooks[1].Name(), "normal")
	}
	if hooks[2].Name() != "low" {
		t.Errorf("hooks[2] = %q, want %q", hooks[2].Name(), "low")
	}
}

func TestRegistry_GetHooks_ReturnsCopy(t *testing.T) {
	r := NewRegistry()
	r.Register(&testHook{name: "h1", event: EventBeforeTool, priority: PriorityNormal})

	hooks := r.GetHooks(EventBeforeTool)
	hooks[0] = nil // mutate the returned slice

	if r.Count() != 1 {
		t.Errorf("count = %d, want 1 (registry should be unaffected)", r.Count())
	}
}

func TestHook_Execution(t *testing.T) {
	called := false
	h := &testHook{
		name:     "custom",
		event:    EventBeforeTool,
		priority: PriorityNormal,
		execute: func(ctx context.Context, hctx *HookContext) HookResult {
			called = true
			return HookResult{Continue: true, Message: "ok"}
		},
	}

	result := h.Execute(context.Background(), &HookContext{TaskID: "test"})
	if !called {
		t.Error("hook execute was not called")
	}
	if !result.Continue {
		t.Error("expected Continue=true")
	}
	if result.Message != "ok" {
		t.Errorf("message = %q, want %q", result.Message, "ok")
	}
}

func TestHook_Execution_Abort(t *testing.T) {
	h := &testHook{
		name:     "abort_hook",
		event:    EventBeforeTool,
		priority: PriorityHighest,
		execute: func(ctx context.Context, hctx *HookContext) HookResult {
			return HookResult{
				Continue: false,
				Error:    errors.New("permission denied"),
				Message:  "access denied for this operation",
			}
		},
	}

	result := h.Execute(context.Background(), &HookContext{TaskID: "test"})
	if result.Continue {
		t.Error("expected Continue=false for abort hook")
	}
	if result.Error == nil {
		t.Error("expected error to be set")
	}
}

func TestPredefinedHookNames(t *testing.T) {
	names := []string{
		HookSchemaValidation, HookJoinCardinality, HookPermission,
		HookCostEstimation, HookCacheLookup, HookSnapshotBeforeWrite,
		HookResultSummary, HookArtifactPersistence, HookCacheWrite,
		HookResultValidation, HookExecutionTrace, HookSnapshotAfterWrite,
	}

	seen := make(map[string]bool)
	for _, name := range names {
		if name == "" {
			t.Error("hook name should not be empty")
		}
		if seen[name] {
			t.Errorf("duplicate hook name: %q", name)
		}
		seen[name] = true
	}
	if len(seen) != 12 {
		t.Errorf("expected 12 unique hook names, got %d", len(seen))
	}
}

func TestRegistry_Register_EmptyName(t *testing.T) {
	r := NewRegistry()
	h := &testHook{name: "", event: EventBeforeTool, priority: PriorityNormal}
	// Empty name is technically allowed at the interface level; callers should validate.
	err := r.Register(h)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
}
