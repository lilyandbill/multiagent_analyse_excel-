// Package integration contains end-to-end integration tests that verify the
// workflow, hooks, skills, and artifacts modules work together correctly.
package integration

import (
	"context"
	"fmt"
	"strings"
	"testing"

	"excel-agent/artifacts"
	"excel-agent/hooks"
	"excel-agent/skills"
	"excel-agent/workflow"
)

// simpleHook adapts a function into the hooks.Hook interface for testing.
type simpleHook struct {
	name     string
	event    hooks.Event
	priority hooks.Priority
	fn       func(ctx context.Context, hctx *hooks.HookContext) hooks.HookResult
}

func (h *simpleHook) Name() string             { return h.name }
func (h *simpleHook) Event() hooks.Event       { return h.event }
func (h *simpleHook) Priority() hooks.Priority { return h.priority }
func (h *simpleHook) Execute(ctx context.Context, hctx *hooks.HookContext) hooks.HookResult {
	if h.fn != nil {
		return h.fn(ctx, hctx)
	}
	return hooks.HookResult{Continue: true}
}

// TestIntegration_HappyPath exercises the full end-to-end flow:
//  1. Create a task context with a budget.
//  2. Register and activate a skill.
//  3. Configure a hook pipeline with deterministic hooks.
//  4. Execute the skill and record the result as an artifact.
//  5. Advance the workflow through all valid states to DONE.
//  6. Verify budget consumption, state order, hook execution, and artifact persistence.
func TestIntegration_HappyPath(t *testing.T) {
	ctx := context.Background()
	workDir := t.TempDir()

	// ── 1. Create task context ──────────────────────────────────────────
	tc := workflow.NewTaskContext("task-001", workDir)
	if tc.CurrentState() != workflow.StateReceived {
		t.Fatalf("initial state = %s, want RECEIVED", tc.CurrentState())
	}

	// ── 2. Skill registry: register and activate ────────────────────────
	skillRegistry := skills.NewRegistry()

	executed := false
	skill, err := skills.NewSkill(skills.Manifest{
		Name:        skills.SkillFTYieldAnalysis,
		DisplayName: "FT Yield Analysis",
		Version:     skills.Version{Major: 1, Minor: 0, Patch: 0},
		Description: "Calculates and verifies yield for Final Test data.",
		InputContract: []skills.FieldDefinition{
			{Name: "table_id", Type: "string", Required: true, Description: "Target data table"},
		},
		Permissions:      []string{"read_file"},
		MaxEstimatedCost: 0.10,
	}, func(ctx context.Context, input map[string]any) (any, error) {
		executed = true
		tableID, ok := input["table_id"].(string)
		if !ok {
			return nil, fmt.Errorf("table_id is required")
		}
		return map[string]any{
			"yield":    0.952,
			"total":    1000,
			"pass":     952,
			"fail":     48,
			"table_id": tableID,
		}, nil
	})
	if err != nil {
		t.Fatalf("NewSkill: %v", err)
	}
	if err := skillRegistry.Register(skill); err != nil {
		t.Fatalf("Register: %v", err)
	}
	if err := skillRegistry.ActivateLatest(skills.SkillFTYieldAnalysis); err != nil {
		t.Fatalf("ActivateLatest: %v", err)
	}

	// ── 3. Hook pipeline ────────────────────────────────────────────────
	hookRegistry := hooks.NewRegistry()

	// Track hook execution order.
	var hookOrder []string

	// Schema validation hook (pre-execute, high priority).
	hookRegistry.Register(&simpleHook{
		name:     hooks.HookSchemaValidation,
		event:    hooks.EventBeforeTool,
		priority: hooks.PriorityHigh,
		fn: func(ctx context.Context, hctx *hooks.HookContext) hooks.HookResult {
			hookOrder = append(hookOrder, "schema_validation")
			return hooks.HookResult{Continue: true, Metadata: map[string]any{"schema_ok": true}}
		},
	})

	// Permission hook (pre-execute, highest priority).
	hookRegistry.Register(&simpleHook{
		name:     hooks.HookPermission,
		event:    hooks.EventBeforeTool,
		priority: hooks.PriorityHighest,
		fn: func(ctx context.Context, hctx *hooks.HookContext) hooks.HookResult {
			hookOrder = append(hookOrder, "permission")
			return hooks.HookResult{Continue: true}
		},
	})

	// Result summary hook (post-execute, normal priority).
	hookRegistry.Register(&simpleHook{
		name:     hooks.HookResultSummary,
		event:    hooks.EventAfterTool,
		priority: hooks.PriorityNormal,
		fn: func(ctx context.Context, hctx *hooks.HookContext) hooks.HookResult {
			hookOrder = append(hookOrder, "result_summary")
			return hooks.HookResult{Continue: true}
		},
	})

	// Pre-verify hook.
	hookRegistry.Register(&simpleHook{
		name:     "pre_verify_check",
		event:    hooks.EventBeforeVerify,
		priority: hooks.PriorityNormal,
		fn: func(ctx context.Context, hctx *hooks.HookContext) hooks.HookResult {
			hookOrder = append(hookOrder, "pre_verify")
			return hooks.HookResult{Continue: true}
		},
	})

	pipeline := hooks.NewPipeline(hookRegistry)

	// ── 4. Artifact store ───────────────────────────────────────────────
	store := artifacts.NewStore()

	// ── 5. Run before_task hooks ────────────────────────────────────────
	hookCtx := &hooks.HookContext{TaskID: tc.TaskID, State: string(tc.CurrentState())}
	cont, result := pipeline.RunPreHooks(ctx, hooks.EventBeforeTask, hookCtx)
	if !cont {
		t.Fatalf("before_task hooks aborted: %s", result.Summary())
	}

	// ── 6. Transition: RECEIVED → INGESTING → INSPECTING → PLANNING ────
	statePath := []workflow.State{
		workflow.StateIngesting,
		workflow.StateInspecting,
		workflow.StatePlanning,
	}
	for _, target := range statePath {
		if err := tc.Transition(ctx, target); err != nil {
			t.Fatalf("transition %s -> %s: %v", tc.CurrentState(), target, err)
		}
	}

	// ── 7. Run before_tool hooks, then execute skill ───────────────────
	hookCtx.State = string(tc.CurrentState())
	hookCtx.ToolName = skills.SkillFTYieldAnalysis
	cont, result = pipeline.RunPreHooks(ctx, hooks.EventBeforeTool, hookCtx)
	if !cont {
		t.Fatalf("before_tool hooks aborted: %s", result.Summary())
	}

	// Verify hook execution order: Permission (0) before SchemaValidation (25).
	if hookOrder[0] != "permission" {
		t.Errorf("hook[0] = %q, want permission", hookOrder[0])
	}
	if hookOrder[1] != "schema_validation" {
		t.Errorf("hook[1] = %q, want schema_validation", hookOrder[1])
	}

	// Activate the skill and execute.
	activeSkill := skillRegistry.GetActive(skills.SkillFTYieldAnalysis)
	if activeSkill == nil {
		t.Fatal("active skill is nil")
	}

	skillResult, err := activeSkill.Handler(ctx, map[string]any{"table_id": "ft_result"})
	if err != nil {
		t.Fatalf("skill execution: %v", err)
	}
	if !executed {
		t.Fatal("skill handler was not called")
	}

	// Record a tool call in usage.
	tc.RecordToolCall(1000)
	tc.RecordLLMCall(500, 200, 0.01)

	// Run after_tool hooks.
	hookCtx.ToolResult = skillResult
	pipeline.RunPostHooks(ctx, hooks.EventAfterTool, hookCtx)

	// ── 8. Save artifact ────────────────────────────────────────────────
	artifactID := artifacts.NewArtifactID()
	if err := store.Put(&artifacts.Artifact{
		ID:          artifactID,
		TaskID:      tc.TaskID,
		Type:        artifacts.ArtifactTypeResult,
		Name:        "yield_result",
		Path:        "artifacts/yield_result.json",
		ContentType: "application/json",
		Metadata:    map[string]any{"yield": 0.952},
	}); err != nil {
		t.Fatalf("Put artifact: %v", err)
	}

	// ── 9. Transition: PLANNING → EXECUTING → VERIFYING → REPORTING → DONE
	executionPath := []workflow.State{
		workflow.StateExecuting,
		workflow.StateVerifying,
		workflow.StateReporting,
		workflow.StateDone,
	}
	for _, target := range executionPath {
		// Run before_verify hook when appropriate.
		if target == workflow.StateVerifying {
			hookCtx.State = string(tc.CurrentState())
			cont, _ := pipeline.RunPreHooks(ctx, hooks.EventBeforeVerify, hookCtx)
			if !cont {
				t.Fatal("before_verify hooks aborted")
			}
		}
		if err := tc.Transition(ctx, target); err != nil {
			t.Fatalf("transition %s -> %s: %v", tc.CurrentState(), target, err)
		}
	}

	// ── 10. Create final snapshot ───────────────────────────────────────
	snap, err := store.CreateSnapshot(tc.TaskID, string(tc.CurrentState()), "task completed")
	if err != nil {
		t.Fatalf("CreateSnapshot: %v", err)
	}

	// ── 11. Assertions ──────────────────────────────────────────────────

	// State: completed.
	if tc.CurrentState() != workflow.StateDone {
		t.Errorf("final state = %s, want DONE", tc.CurrentState())
	}
	if tc.IsActive() {
		t.Error("task should not be active in DONE state")
	}

	// Skill: registered and retrievable.
	retrieved := skillRegistry.GetVersion(skills.SkillFTYieldAnalysis, skills.Version{Major: 1, Minor: 0, Patch: 0})
	if retrieved == nil {
		t.Fatal("skill not retrievable by version")
	}
	if retrieved.Manifest.Name != skills.SkillFTYieldAnalysis {
		t.Errorf("skill name = %q, want %q", retrieved.Manifest.Name, skills.SkillFTYieldAnalysis)
	}

	// Hooks: executed in expected order.
	if len(hookOrder) != 4 {
		t.Errorf("hook execution count = %d, want 4. Order: %v", len(hookOrder), hookOrder)
	}
	expectedHookOrder := []string{"permission", "schema_validation", "result_summary", "pre_verify"}
	for i, expected := range expectedHookOrder {
		if i < len(hookOrder) && hookOrder[i] != expected {
			t.Errorf("hook[%d] = %q, want %q", i, hookOrder[i], expected)
		}
	}

	// Budget: iterations consumed exactly as expected.
	// 1 RECEIVED->INGESTING + 1 INGESTING->INSPECTING + 1 INSPECTING->PLANNING
	// + 1 PLANNING->EXECUTING + 1 EXECUTING->VERIFYING + 1 VERIFYING->REPORTING
	// + 1 REPORTING->DONE = 7 transitions.
	snap2 := tc.Snapshot()
	if snap2.Usage.Iterations != 7 {
		t.Errorf("iterations = %d, want 7", snap2.Usage.Iterations)
	}
	if snap2.Usage.ToolCalls != 1 {
		t.Errorf("tool calls = %d, want 1", snap2.Usage.ToolCalls)
	}
	if snap2.Usage.LLMCalls != 1 {
		t.Errorf("LLM calls = %d, want 1", snap2.Usage.LLMCalls)
	}

	// Artifact: persisted and retrievable.
	loaded := store.Get(artifactID)
	if loaded == nil {
		t.Fatal("artifact not found in store")
	}
	if loaded.Name != "yield_result" {
		t.Errorf("artifact name = %q, want %q", loaded.Name, "yield_result")
	}
	if loaded.TaskID != tc.TaskID {
		t.Errorf("artifact TaskID = %q, want %q", loaded.TaskID, tc.TaskID)
	}

	// Artifacts by task.
	ids := store.ListByTask(tc.TaskID)
	if len(ids) != 1 || ids[0] != artifactID {
		t.Errorf("ListByTask = %v, want [%s]", ids, artifactID)
	}

	// Snapshot: created and retrievable.
	snapLoaded := store.GetSnapshot(snap.ID)
	if snapLoaded == nil {
		t.Fatal("snapshot not found")
	}
	if snapLoaded.State != string(workflow.StateDone) {
		t.Errorf("snapshot state = %q, want DONE", snapLoaded.State)
	}

	// Snapshot contains the artifact.
	snapArtifacts := store.GetArtifactsForSnapshot(snap.ID)
	if len(snapArtifacts) != 1 {
		t.Errorf("snapshot artifacts = %d, want 1", len(snapArtifacts))
	}

	// No retries occurred.
	if snap2.Usage.Retries != 0 {
		t.Errorf("retries = %d, want 0", snap2.Usage.Retries)
	}

	// Budget not exceeded.
	if err := tc.CheckBudget(); err != nil {
		t.Errorf("budget should not be exceeded: %v", err)
	}

	t.Logf("hook order: %v", hookOrder)
	t.Logf("final state: %s, iterations: %d", tc.CurrentState(), snap2.Usage.Iterations)
}

// TestIntegration_BudgetExhaustion verifies that execution stops when
// the iteration budget is exhausted.
func TestIntegration_BudgetExhaustion(t *testing.T) {
	ctx := context.Background()
	workDir := t.TempDir()

	tc := workflow.NewTaskContext("task-002", workDir)

	// Set a tight budget: only 1 iteration allowed.
	// The check is u.Iterations > b.MaxIterations (strict greater-than),
	// so MaxIterations=1 allows 2 successful transitions (0→1, 1→2)
	// and fails on the 3rd (2 > 1).
	tc.Budget.MaxIterations = 1

	// First transition: RECEIVED → INGESTING (iteration 0→1, passes: 1 > 1 is false)
	if err := tc.Transition(ctx, workflow.StateIngesting); err != nil {
		t.Fatalf("first transition should succeed: %v", err)
	}

	// Second transition: INGESTING → INSPECTING (iteration 1→2, passes: 2 > 1 is false)
	if err := tc.Transition(ctx, workflow.StateInspecting); err != nil {
		t.Fatalf("second transition should succeed: %v", err)
	}

	// Third transition: should fail due to budget exhaustion (iteration 2→3, check: 3 > 1 is true)
	err := tc.Transition(ctx, workflow.StatePlanning)
	if err == nil {
		t.Fatal("expected budget exhaustion error, got nil")
	}
	if !strings.Contains(err.Error(), "budget") {
		t.Errorf("error should mention budget, got: %v", err)
	}

	// State should remain at INSPECTING (the failed transition didn't take effect).
	if tc.CurrentState() != workflow.StateInspecting {
		t.Errorf("state should be INSPECTING after failed transition, got %s", tc.CurrentState())
	}

	snap := tc.Snapshot()
	if snap.Usage.Iterations != 2 {
		t.Errorf("iterations = %d, want 2", snap.Usage.Iterations)
	}

	t.Logf("budget exhaustion correctly stopped at state %s after %d iterations",
		tc.CurrentState(), snap.Usage.Iterations)
}

// TestIntegration_InvalidStateTransition verifies that invalid transitions are rejected.
func TestIntegration_InvalidStateTransition(t *testing.T) {
	ctx := context.Background()
	tc := workflow.NewTaskContext("task-003", t.TempDir())

	// Invalid: RECEIVED → DONE.
	err := tc.Transition(ctx, workflow.StateDone)
	if err == nil {
		t.Fatal("expected error for invalid transition RECEIVED -> DONE")
	}
	if !strings.Contains(err.Error(), "invalid state transition") {
		t.Errorf("error should mention invalid transition, got: %v", err)
	}

	// State should still be RECEIVED.
	if tc.CurrentState() != workflow.StateReceived {
		t.Errorf("state = %s, want RECEIVED", tc.CurrentState())
	}
}

// TestIntegration_MissingSkill verifies that looking up a missing skill returns nil.
func TestIntegration_MissingSkill(t *testing.T) {
	registry := skills.NewRegistry()

	if s := registry.GetActive("nonexistent"); s != nil {
		t.Error("GetActive for missing skill should return nil")
	}
	if s := registry.GetVersion("nonexistent", skills.Version{Major: 1, Minor: 0, Patch: 0}); s != nil {
		t.Error("GetVersion for missing skill should return nil")
	}
	if err := registry.ActivateLatest("nonexistent"); err == nil {
		t.Error("ActivateLatest for missing skill should return error")
	}
	if err := registry.Activate("nonexistent", skills.Version{Major: 1, Minor: 0, Patch: 0}); err == nil {
		t.Error("Activate for missing skill should return error")
	}
}

// TestIntegration_DeterministicExecution verifies the integration test produces
// the same results on repeated runs.
func TestIntegration_DeterministicExecution(t *testing.T) {
	// Run the happy path 3 times; assertions must be identical each time.
	for run := 0; run < 3; run++ {
		t.Run(fmt.Sprintf("run_%d", run), func(t *testing.T) {
			ctx := context.Background()

			tc := workflow.NewTaskContext("task-det", t.TempDir())

			// Register skill.
			r := skills.NewRegistry()
			skill, _ := skills.NewSkill(skills.Manifest{
				Name:        "det_test",
				Version:     skills.Version{Major: 1, Minor: 0, Patch: 0},
				Description: "Deterministic test skill.",
			}, func(ctx context.Context, input map[string]any) (any, error) {
				return "deterministic", nil
			})
			r.Register(skill)
			r.ActivateLatest("det_test")

			// Execute deterministic handler.
			result, err := r.GetActive("det_test").Handler(ctx, nil)
			if err != nil {
				t.Fatalf("handler: %v", err)
			}
			if result != "deterministic" {
				t.Errorf("result = %v, want deterministic", result)
			}

			// Transitions always follow the same order.
			for _, target := range []workflow.State{
				workflow.StateIngesting,
				workflow.StateInspecting,
				workflow.StatePlanning,
				workflow.StateExecuting,
				workflow.StateVerifying,
				workflow.StateReporting,
				workflow.StateDone,
			} {
				if err := tc.Transition(ctx, target); err != nil {
					t.Fatalf("transition: %v", err)
				}
			}

			if tc.CurrentState() != workflow.StateDone {
				t.Errorf("state = %s, want DONE", tc.CurrentState())
			}
		})
	}
}
