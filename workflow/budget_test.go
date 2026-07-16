package workflow

import (
	"testing"
	"time"
)

func TestDefaultBudget_HasReasonableDefaults(t *testing.T) {
	b := DefaultBudget()

	checks := []struct {
		field string
		got   any
		want  any
	}{
		{"MaxLLMCalls", b.MaxLLMCalls, 3},
		{"MaxToolCalls", b.MaxToolCalls, 10},
		{"MaxIterations", b.MaxIterations, 8},
		{"MaxTables", b.MaxTables, 3},
		{"MaxRAGChunks", b.MaxRAGChunks, 4},
		{"MaxContextChars", b.MaxContextChars, 12000},
		{"MaxRetries", b.MaxRetries, 1},
	}
	for _, c := range checks {
		if c.got != c.want {
			t.Errorf("%s = %v, want %v", c.field, c.got, c.want)
		}
	}
	if b.MaxExecutionTime != 120*time.Second {
		t.Errorf("MaxExecutionTime = %v, want 120s", b.MaxExecutionTime)
	}
}

func TestBudgetCheck_WithinLimits(t *testing.T) {
	b := DefaultBudget()
	u := Usage{
		LLMCalls:      1,
		InputTokens:   1000,
		OutputTokens:  500,
		ToolCalls:     5,
		Iterations:    3,
		TablesInScope: 1,
		RowsScanned:   1000,
		RAGChunks:     2,
		ContextChars:  5000,
		ExecutionTime: 30 * time.Second,
		EstimatedCost: 0.01,
	}
	if err := b.Check(u); err != nil {
		t.Errorf("expected no error, got: %v", err)
	}
}

func TestBudgetCheck_ExceedLLMCalls(t *testing.T) {
	b := DefaultBudget()
	u := Usage{LLMCalls: 10}
	err := b.Check(u)
	if err == nil {
		t.Fatal("expected error for exceeded LLM calls")
	}
	exceeded, ok := err.(*ExceededError)
	if !ok {
		t.Fatalf("expected *ExceededError, got %T", err)
	}
	if exceeded.Field != "max_llm_calls" {
		t.Errorf("field = %q, want %q", exceeded.Field, "max_llm_calls")
	}
}

func TestBudgetCheck_ExceedToolCalls(t *testing.T) {
	b := DefaultBudget()
	err := b.Check(Usage{ToolCalls: 15})
	if err == nil {
		t.Fatal("expected error")
	}
	if err.(*ExceededError).Field != "max_tool_calls" {
		t.Errorf("field = %q, want %q", err.(*ExceededError).Field, "max_tool_calls")
	}
}

func TestBudgetCheck_ExceedIterations(t *testing.T) {
	b := DefaultBudget()
	err := b.Check(Usage{Iterations: 100})
	if err == nil {
		t.Fatal("expected error for exceeded iterations")
	}
}

func TestBudgetCheck_ExceedExecutionTime(t *testing.T) {
	b := DefaultBudget()
	err := b.Check(Usage{ExecutionTime: 5 * time.Minute})
	if err == nil {
		t.Fatal("expected error for exceeded execution time")
	}
}

func TestBudgetCheck_ExceedEstimatedCost(t *testing.T) {
	b := DefaultBudget()
	err := b.Check(Usage{EstimatedCost: 10.0})
	if err == nil {
		t.Fatal("expected error for exceeded cost")
	}
}

func TestBudgetCheck_ZeroLimitSkipsCheck(t *testing.T) {
	b := ComputeBudget{} // all zero
	u := Usage{
		LLMCalls:      100,
		ToolCalls:     100,
		Iterations:    100,
		ExecutionTime: 10 * time.Hour,
		EstimatedCost: 100.0,
	}
	if err := b.Check(u); err != nil {
		t.Errorf("zero limits should not enforce, got error: %v", err)
	}
}

func TestExceededError_Error(t *testing.T) {
	e := &ExceededError{
		Field:   "max_llm_calls",
		Limit:   3,
		Actual:  10,
		Message: "LLM call limit exceeded",
	}
	if e.Error() == "" {
		t.Error("ExceededError.Error() should not be empty")
	}
}
