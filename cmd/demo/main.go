// Demo agent harness: a minimal runnable CLI that demonstrates the
// workflow, hooks, skills, budget, and artifact store working together.
//
// Usage:
//
//	go run ./cmd/demo --task "analyze sample data"
package main

import (
	"context"
	"flag"
	"fmt"
	"os"
	"strings"

	"excel-agent/artifacts"
	"excel-agent/executor"
	"excel-agent/hooks"
	"excel-agent/skills"
	"excel-agent/workflow"
)

func main() {
	task := flag.String("task", "", "Task description (required)")
	execName := flag.String("executor", "fake", "Executor: fake or openai")
	flag.Parse()

	ctx := context.Background()

	result, err := runDemo(ctx, *execName, *task)
	if err != nil {
		fmt.Fprintf(os.Stderr, "Error: %v\n", err)
		os.Exit(1)
	}

	fmt.Println(formatSummary(result))
}

// demoResult holds the result of a demo execution for structured output.
type demoResult struct {
	Task         string
	SkillName    string
	HookOrder    []string
	StatePath    []workflow.State
	Iterations   int
	ToolCalls    int
	LLMCalls     int
	ArtifactID   string
	ArtifactPath string
	FinalState   workflow.State
	SkillResult  string
}

// runDemo orchestrates the full agent harness.
func runDemo(ctx context.Context, execName, task string) (*demoResult, error) {
	task = strings.TrimSpace(task)
	if task == "" {
		return nil, fmt.Errorf("task must not be empty")
	}

	// ── Select executor ──────────────────────────────────────────────
	var exec executor.Executor
	switch strings.ToLower(execName) {
	case "fake":
		exec = executor.FakeExecutor{}
	case "openai":
		cfg, err := executor.OpenAIConfigFromEnv()
		if err != nil {
			return nil, fmt.Errorf("openai config: %w", err)
		}
		exec = executor.NewOpenAIExecutor(cfg)
	default:
		return nil, fmt.Errorf("unknown executor: %q (use 'fake' or 'openai')", execName)
	}

	workDir, err := os.MkdirTemp("", "agent-demo-*")
	if err != nil {
		return nil, fmt.Errorf("create work dir: %w", err)
	}
	defer os.RemoveAll(workDir)

	// ── 1. Workflow ──────────────────────────────────────────────────
	tc := workflow.NewTaskContext("demo-001", workDir)

	// ── 2. Skill ─────────────────────────────────────────────────────
	skillRegistry := skills.NewRegistry()

	skill, err := skills.NewSkill(skills.Manifest{
		Name:        "demo_analysis",
		DisplayName: "Demo Analysis",
		Version:     skills.Version{Major: 1, Minor: 0, Patch: 0},
		Description: "A deterministic demo skill for integration testing.",
		InputContract: []skills.FieldDefinition{
			{Name: "task", Type: "string", Required: true, Description: "The task to analyze"},
		},
		Permissions:      []string{"read_file"},
		MaxEstimatedCost: 0.05,
	}, func(ctx context.Context, input map[string]any) (any, error) {
		taskVal, _ := input["task"].(string)
		result, err := exec.Execute(ctx, executor.Request{Task: taskVal})
		if err != nil {
			return nil, err
		}
		return result.Text, nil
	})
	if err != nil {
		return nil, fmt.Errorf("create skill: %w", err)
	}
	if err := skillRegistry.Register(skill); err != nil {
		return nil, fmt.Errorf("register skill: %w", err)
	}
	if err := skillRegistry.ActivateLatest("demo_analysis"); err != nil {
		return nil, fmt.Errorf("activate skill: %w", err)
	}

	// ── 3. Hooks ─────────────────────────────────────────────────────
	hookRegistry := hooks.NewRegistry()
	var hookOrder []string

	hookRegistry.Register(&simpleHook{
		name:     hooks.HookSchemaValidation,
		event:    hooks.EventBeforeTool,
		priority: hooks.PriorityHighest,
		fn: func(ctx context.Context, hctx *hooks.HookContext) hooks.HookResult {
			hookOrder = append(hookOrder, "schema_validation")
			return hooks.HookResult{Continue: true}
		},
	})
	hookRegistry.Register(&simpleHook{
		name:     hooks.HookPermission,
		event:    hooks.EventBeforeTool,
		priority: hooks.PriorityHigh,
		fn: func(ctx context.Context, hctx *hooks.HookContext) hooks.HookResult {
			hookOrder = append(hookOrder, "permission")
			return hooks.HookResult{Continue: true}
		},
	})
	hookRegistry.Register(&simpleHook{
		name:     hooks.HookResultSummary,
		event:    hooks.EventAfterTool,
		priority: hooks.PriorityNormal,
		fn: func(ctx context.Context, hctx *hooks.HookContext) hooks.HookResult {
			hookOrder = append(hookOrder, "result_summary")
			return hooks.HookResult{Continue: true}
		},
	})

	pipeline := hooks.NewPipeline(hookRegistry)

	// ── 4. Before-task hooks ─────────────────────────────────────────
	hookCtx := &hooks.HookContext{TaskID: tc.TaskID, State: string(tc.CurrentState())}
	cont, result := pipeline.RunPreHooks(ctx, hooks.EventBeforeTask, hookCtx)
	if !cont {
		return nil, fmt.Errorf("before_task hooks aborted: %s", result.Summary())
	}

	// ── 5. State transitions: RECEIVED → INGESTING → INSPECTING → PLANNING ──
	statePath := []workflow.State{
		workflow.StateIngesting,
		workflow.StateInspecting,
		workflow.StatePlanning,
	}
	for _, target := range statePath {
		if err := tc.Transition(ctx, target); err != nil {
			return nil, fmt.Errorf("transition %s -> %s: %w", tc.CurrentState(), target, err)
		}
	}

	// ── 6. Execute skill ─────────────────────────────────────────────
	hookCtx.State = string(tc.CurrentState())
	hookCtx.ToolName = "demo_analysis"
	cont, result = pipeline.RunPreHooks(ctx, hooks.EventBeforeTool, hookCtx)
	if !cont {
		return nil, fmt.Errorf("before_tool hooks aborted: %s", result.Summary())
	}

	activeSkill := skillRegistry.GetActive("demo_analysis")
	if activeSkill == nil {
		return nil, fmt.Errorf("skill 'demo_analysis' not active")
	}

	skillResult, err := activeSkill.Handler(ctx, map[string]any{"task": task})
	if err != nil {
		return nil, fmt.Errorf("skill execution: %w", err)
	}
	skillResultStr := fmt.Sprintf("%v", skillResult)

	tc.RecordToolCall(1)
	tc.RecordLLMCall(100, 50, 0.01)

	hookCtx.ToolResult = skillResult
	pipeline.RunPostHooks(ctx, hooks.EventAfterTool, hookCtx)

	// ── 7. Artifact ──────────────────────────────────────────────────
	store := artifacts.NewStore()
	artifactID := artifacts.NewArtifactID()
	artifactPath := "artifacts/demo_result.txt"
	if err := store.Put(&artifacts.Artifact{
		ID:          artifactID,
		TaskID:      tc.TaskID,
		Type:        artifacts.ArtifactTypeResult,
		Name:        "demo_result",
		Path:        artifactPath,
		ContentType: "text/plain",
		Size:        int64(len(skillResultStr)),
		Metadata:    map[string]any{"result": skillResultStr},
	}); err != nil {
		return nil, fmt.Errorf("save artifact: %w", err)
	}

	// ── 8. Final transitions: PLANNING → EXECUTING → VERIFYING → REPORTING → DONE ──
	executionPath := []workflow.State{
		workflow.StateExecuting,
		workflow.StateVerifying,
		workflow.StateReporting,
		workflow.StateDone,
	}
	for _, target := range executionPath {
		if err := tc.Transition(ctx, target); err != nil {
			return nil, fmt.Errorf("transition %s -> %s: %w", tc.CurrentState(), target, err)
		}
	}

	// ── 9. Snapshot ──────────────────────────────────────────────────
	if _, err := store.CreateSnapshot(tc.TaskID, string(tc.CurrentState()), "demo complete"); err != nil {
		return nil, fmt.Errorf("snapshot: %w", err)
	}

	usg := tc.Snapshot().Usage

	return &demoResult{
		Task:         task,
		SkillName:    "demo_analysis",
		HookOrder:    hookOrder,
		StatePath:    statePath,
		Iterations:   usg.Iterations,
		ToolCalls:    usg.ToolCalls,
		LLMCalls:     usg.LLMCalls,
		ArtifactID:   artifactID,
		ArtifactPath: artifactPath,
		FinalState:   tc.CurrentState(),
		SkillResult:  skillResultStr,
	}, nil
}

// formatSummary formats the execution result for console output.
func formatSummary(r *demoResult) string {
	states := make([]string, len(r.StatePath)+1)
	states[0] = "RECEIVED"
	for i, s := range r.StatePath {
		states[i+1] = string(s)
	}

	return fmt.Sprintf(`Task: %s
Skill: %s
Hooks: %s
State: %s -> EXECUTING -> VERIFYING -> REPORTING -> DONE
Budget: %d LLM calls / %d tool calls / %d iterations used
Artifact: %s (%s)
Status: %s
Result: %s`,
		r.Task,
		r.SkillName,
		strings.Join(r.HookOrder, " -> "),
		strings.Join(states, " -> "),
		r.LLMCalls, r.ToolCalls, r.Iterations,
		r.ArtifactPath, r.ArtifactID,
		r.FinalState,
		r.SkillResult,
	)
}

// simpleHook adapts a function into the hooks.Hook interface.
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
