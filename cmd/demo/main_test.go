package main

import (
	"context"
	"strings"
	"testing"

	"excel-agent/workflow"
)

func TestRunDemo_Success(t *testing.T) {
	ctx := context.Background()
	result, err := runDemo(ctx, "analyze sample data")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if result == nil {
		t.Fatal("expected result, got nil")
	}
	if result.Task != "analyze sample data" {
		t.Errorf("task = %q, want %q", result.Task, "analyze sample data")
	}
	if result.SkillName != "demo_analysis" {
		t.Errorf("skill = %q, want %q", result.SkillName, "demo_analysis")
	}
	if result.FinalState != workflow.StateDone {
		t.Errorf("final state = %s, want DONE", result.FinalState)
	}
	if !strings.Contains(result.SkillResult, "deterministic result for: analyze sample data") {
		t.Errorf("skill result = %q, want %q", result.SkillResult, "deterministic result for: analyze sample data")
	}
	if result.Iterations != 7 {
		t.Errorf("iterations = %d, want 7", result.Iterations)
	}
	if result.ToolCalls != 1 {
		t.Errorf("tool calls = %d, want 1", result.ToolCalls)
	}
	if result.LLMCalls != 1 {
		t.Errorf("LLM calls = %d, want 1", result.LLMCalls)
	}
	if result.ArtifactID == "" {
		t.Error("artifact ID should not be empty")
	}
	if result.ArtifactPath != "artifacts/demo_result.txt" {
		t.Errorf("artifact path = %q, want %q", result.ArtifactPath, "artifacts/demo_result.txt")
	}
	if len(result.HookOrder) != 3 {
		t.Errorf("hook count = %d, want 3. Order: %v", len(result.HookOrder), result.HookOrder)
	}
	// Verify hook priority order.
	expected := []string{"schema_validation", "permission", "result_summary"}
	for i, exp := range expected {
		if result.HookOrder[i] != exp {
			t.Errorf("hook[%d] = %q, want %q", i, result.HookOrder[i], exp)
		}
	}
}

func TestRunDemo_EmptyTask(t *testing.T) {
	ctx := context.Background()
	_, err := runDemo(ctx, "")
	if err == nil {
		t.Fatal("expected error for empty task")
	}
	if !strings.Contains(err.Error(), "task must not be empty") {
		t.Errorf("error = %q, want message about empty task", err.Error())
	}
}

func TestRunDemo_WhitespaceOnlyTask(t *testing.T) {
	ctx := context.Background()
	_, err := runDemo(ctx, "   ")
	if err == nil {
		t.Fatal("expected error for whitespace-only task")
	}
}

func TestRunDemo_Deterministic(t *testing.T) {
	// Two runs with the same input produce identical results.
	ctx := context.Background()

	r1, err := runDemo(ctx, "deterministic test")
	if err != nil {
		t.Fatalf("run 1: %v", err)
	}
	r2, err := runDemo(ctx, "deterministic test")
	if err != nil {
		t.Fatalf("run 2: %v", err)
	}

	if r1.FinalState != r2.FinalState {
		t.Errorf("final state differs: %s vs %s", r1.FinalState, r2.FinalState)
	}
	if r1.Iterations != r2.Iterations {
		t.Errorf("iterations differ: %d vs %d", r1.Iterations, r2.Iterations)
	}
	if r1.SkillResult != r2.SkillResult {
		t.Errorf("skill result differs: %q vs %q", r1.SkillResult, r2.SkillResult)
	}
	if r1.HookOrder[0] != r2.HookOrder[0] || r1.HookOrder[1] != r2.HookOrder[1] || r1.HookOrder[2] != r2.HookOrder[2] {
		t.Errorf("hook order differs: %v vs %v", r1.HookOrder, r2.HookOrder)
	}
}

func TestFormatSummary(t *testing.T) {
	r := &demoResult{
		Task:         "test task",
		SkillName:    "test_skill",
		HookOrder:    []string{"a", "b"},
		StatePath:    []workflow.State{workflow.StateIngesting},
		Iterations:   2,
		ToolCalls:    1,
		LLMCalls:     1,
		ArtifactID:   "abc-123",
		ArtifactPath: "artifacts/out.txt",
		FinalState:   workflow.StateDone,
		SkillResult:  "result value",
	}

	out := formatSummary(r)

	checks := []string{
		"Task: test task",
		"Skill: test_skill",
		"Hooks: a -> b",
		"State: RECEIVED",
		"DONE",
		"Budget: 1 LLM calls / 1 tool calls / 2 iterations used",
		"Artifact: artifacts/out.txt (abc-123)",
		"Status: DONE",
		"Result: result value",
	}
	for _, expected := range checks {
		if !strings.Contains(out, expected) {
			t.Errorf("output missing %q", expected)
		}
	}

	t.Logf("\n%s", out)
}
