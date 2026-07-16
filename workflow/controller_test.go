package workflow

import (
	"context"
	"testing"
)

func TestNewTaskContext(t *testing.T) {
	tc := NewTaskContext("task-1", "/tmp/work")

	if tc.TaskID != "task-1" {
		t.Errorf("TaskID = %q, want %q", tc.TaskID, "task-1")
	}
	if tc.State != StateReceived {
		t.Errorf("State = %q, want %q", tc.State, StateReceived)
	}
	if tc.WorkDir != "/tmp/work" {
		t.Errorf("WorkDir = %q, want %q", tc.WorkDir, "/tmp/work")
	}
	if tc.Mode != ModeAuto {
		t.Errorf("Mode = %q, want %q", tc.Mode, ModeAuto)
	}
	if !tc.IsActive() {
		t.Error("new task should be active")
	}
}

func TestTaskContext_Transition_HappyPath(t *testing.T) {
	tc := NewTaskContext("task-1", "/tmp/work")

	if err := tc.Transition(context.Background(), StateIngesting); err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if tc.CurrentState() != StateIngesting {
		t.Errorf("expected INGESTING, got %s", tc.CurrentState())
	}
}

func TestTaskContext_Transition_Invalid(t *testing.T) {
	tc := NewTaskContext("task-1", "/tmp/work")

	err := tc.Transition(context.Background(), StateDone)
	if err == nil {
		t.Fatal("expected error for invalid transition")
	}
	if tc.CurrentState() != StateReceived {
		t.Errorf("state should remain RECEIVED after failed transition, got %s", tc.CurrentState())
	}
}

func TestTaskContext_Transition_ExceedBudget(t *testing.T) {
	tc := NewTaskContext("task-1", "/tmp/work")

	// Exhaust budget by exceeding max iterations
	tc.mu.Lock()
	tc.Usage.Iterations = tc.Budget.MaxIterations + 1
	tc.mu.Unlock()

	err := tc.Transition(context.Background(), StateIngesting)
	if err == nil {
		t.Fatal("expected budget exceeded error")
	}
}

func TestTaskContext_RecordLLMCall(t *testing.T) {
	tc := NewTaskContext("task-1", "/tmp/work")

	tc.RecordLLMCall(1000, 500, 0.01)
	tc.RecordLLMCall(2000, 800, 0.02)

	snap := tc.Snapshot()
	if snap.Usage.LLMCalls != 2 {
		t.Errorf("LLMCalls = %d, want 2", snap.Usage.LLMCalls)
	}
	if snap.Usage.InputTokens != 3000 {
		t.Errorf("InputTokens = %d, want 3000", snap.Usage.InputTokens)
	}
	if snap.Usage.OutputTokens != 1300 {
		t.Errorf("OutputTokens = %d, want 1300", snap.Usage.OutputTokens)
	}
	if snap.Usage.EstimatedCost != 0.03 {
		t.Errorf("EstimatedCost = %f, want 0.03", snap.Usage.EstimatedCost)
	}
}

func TestTaskContext_RecordToolCall(t *testing.T) {
	tc := NewTaskContext("task-1", "/tmp/work")

	tc.RecordToolCall(5000)
	tc.RecordToolCall(3000)

	snap := tc.Snapshot()
	if snap.Usage.ToolCalls != 2 {
		t.Errorf("ToolCalls = %d, want 2", snap.Usage.ToolCalls)
	}
	if snap.Usage.RowsScanned != 8000 {
		t.Errorf("RowsScanned = %d, want 8000", snap.Usage.RowsScanned)
	}
}

func TestTaskContext_SetMode(t *testing.T) {
	tc := NewTaskContext("task-1", "/tmp/work")
	tc.SetMode(ModeApprovalRequired)
	if tc.Snapshot().Mode != ModeApprovalRequired {
		t.Errorf("Mode = %q, want %q", tc.Snapshot().Mode, ModeApprovalRequired)
	}
}

func TestTaskContext_Snapshot_IsIndependent(t *testing.T) {
	tc := NewTaskContext("task-1", "/tmp/work")
	tc.RecordLLMCall(100, 50, 0.01)

	snap := tc.Snapshot()
	tc.RecordLLMCall(200, 100, 0.02)

	if snap.Usage.LLMCalls != 1 {
		t.Errorf("snapshot LLMCalls should be 1, got %d", snap.Usage.LLMCalls)
	}
	if tc.Snapshot().Usage.LLMCalls != 2 {
		t.Errorf("current LLMCalls should be 2, got %d", tc.Snapshot().Usage.LLMCalls)
	}
}

func TestTaskContext_IsActive(t *testing.T) {
	tc := NewTaskContext("task-1", "/tmp/work")
	if !tc.IsActive() {
		t.Error("RECEIVED should be active")
	}

	tc.mu.Lock()
	tc.State = StateDone
	tc.mu.Unlock()

	if tc.IsActive() {
		t.Error("DONE should not be active")
	}
}

func TestTaskContext_CheckBudget(t *testing.T) {
	tc := NewTaskContext("task-1", "/tmp/work")

	if err := tc.CheckBudget(); err != nil {
		t.Errorf("unexpected budget error: %v", err)
	}

	tc.mu.Lock()
	tc.Usage.ToolCalls = 100
	tc.mu.Unlock()

	if err := tc.CheckBudget(); err == nil {
		t.Error("expected budget error")
	}
}

func TestExecutionMode_Values(t *testing.T) {
	if ModeAuto == ModePlanReview || ModeAuto == ModeApprovalRequired || ModePlanReview == ModeApprovalRequired {
		t.Error("execution modes should be distinct")
	}
}

func TestTaskContext_Transition_IncrementsIterations(t *testing.T) {
	tc := NewTaskContext("task-1", "/tmp/work")

	for i := 0; i < 3; i++ {
		var target State
		switch tc.CurrentState() {
		case StateReceived:
			target = StateIngesting
		case StateIngesting:
			target = StateInspecting
		case StateInspecting:
			target = StatePlanning
		}
		if err := tc.Transition(context.Background(), target); err != nil {
			t.Fatalf("unexpected error at iteration %d: %v", i, err)
		}
	}

	snap := tc.Snapshot()
	if snap.Usage.Iterations != 3 {
		t.Errorf("Iterations = %d, want 3", snap.Usage.Iterations)
	}
}
