package workflow

import (
	"fmt"
	"time"
)

// ComputeBudget defines the resource limits for a single task execution.
// When any limit is exceeded, execution stops and returns a structured error.
// All limits are enforced by the Workflow Controller, not by LLM prompting.
type ComputeBudget struct {
	// MaxLLMCalls limits the total number of LLM API calls per task.
	MaxLLMCalls int `json:"max_llm_calls"`

	// MaxInputTokens limits the total input tokens consumed across all LLM calls.
	MaxInputTokens int `json:"max_input_tokens"`

	// MaxOutputTokens limits the total output tokens produced across all LLM calls.
	MaxOutputTokens int `json:"max_output_tokens"`

	// MaxToolCalls limits the total number of deterministic tool calls per task.
	MaxToolCalls int `json:"max_tool_calls"`

	// MaxRetries limits the number of retries for a failed step.
	MaxRetries int `json:"max_retries"`

	// MaxIterations limits the number of workflow state transitions.
	MaxIterations int `json:"max_iterations"`

	// MaxTables limits the number of tables (sheets/files) that can be in scope.
	MaxTables int `json:"max_tables"`

	// MaxRowsScanned limits the number of data rows that can be read.
	MaxRowsScanned int64 `json:"max_rows_scanned"`

	// MaxRAGChunks limits the number of RAG chunks injected into the LLM context.
	MaxRAGChunks int `json:"max_rag_chunks"`

	// MaxContextChars limits the total characters in the LLM context window.
	MaxContextChars int `json:"max_context_chars"`

	// MaxExecutionTime limits the wall-clock duration of a task.
	MaxExecutionTime time.Duration `json:"max_execution_time"`

	// MaxEstimatedCost limits the estimated API cost in USD.
	MaxEstimatedCost float64 `json:"max_estimated_cost"`
}

// DefaultBudget returns the V1 standard mode budget with conservative defaults.
func DefaultBudget() ComputeBudget {
	return ComputeBudget{
		MaxLLMCalls:      3,
		MaxInputTokens:   32000,
		MaxOutputTokens:  16000,
		MaxToolCalls:     10,
		MaxRetries:       1,
		MaxIterations:    8,
		MaxTables:        3,
		MaxRowsScanned:   100_000,
		MaxRAGChunks:     4,
		MaxContextChars:  12000,
		MaxExecutionTime: 120 * time.Second,
		MaxEstimatedCost: 0.50,
	}
}

// Usage tracks actual resource consumption during task execution.
type Usage struct {
	LLMCalls      int           `json:"llm_calls"`
	InputTokens   int           `json:"input_tokens"`
	OutputTokens  int           `json:"output_tokens"`
	ToolCalls     int           `json:"tool_calls"`
	Retries       int           `json:"retries"`
	Iterations    int           `json:"iterations"`
	TablesInScope int           `json:"tables_in_scope"`
	RowsScanned   int64         `json:"rows_scanned"`
	RAGChunks     int           `json:"rag_chunks"`
	ContextChars  int           `json:"context_chars"`
	ExecutionTime time.Duration `json:"execution_time"`
	EstimatedCost float64       `json:"estimated_cost"`
	CacheHits     int           `json:"cache_hits"`
	RepeatedCalls int           `json:"repeated_calls"`
}

// ExceededError is returned when a budget limit is exceeded.
type ExceededError struct {
	Field   string `json:"field"`
	Limit   any    `json:"limit"`
	Actual  any    `json:"actual"`
	Message string `json:"message"`
}

func (e *ExceededError) Error() string {
	return fmt.Sprintf("budget exceeded: %s (limit=%v, actual=%v): %s",
		e.Field, e.Limit, e.Actual, e.Message)
}

// Check checks the current usage against the budget and returns an error
// if any limit is exceeded. Returns nil if usage is within budget.
func (b ComputeBudget) Check(u Usage) error {
	if b.MaxLLMCalls > 0 && u.LLMCalls > b.MaxLLMCalls {
		return &ExceededError{
			Field:   "max_llm_calls",
			Limit:   b.MaxLLMCalls,
			Actual:  u.LLMCalls,
			Message: "LLM call limit exceeded",
		}
	}
	if b.MaxInputTokens > 0 && u.InputTokens > b.MaxInputTokens {
		return &ExceededError{
			Field:   "max_input_tokens",
			Limit:   b.MaxInputTokens,
			Actual:  u.InputTokens,
			Message: "input token limit exceeded",
		}
	}
	if b.MaxOutputTokens > 0 && u.OutputTokens > b.MaxOutputTokens {
		return &ExceededError{
			Field:   "max_output_tokens",
			Limit:   b.MaxOutputTokens,
			Actual:  u.OutputTokens,
			Message: "output token limit exceeded",
		}
	}
	if b.MaxToolCalls > 0 && u.ToolCalls > b.MaxToolCalls {
		return &ExceededError{
			Field:   "max_tool_calls",
			Limit:   b.MaxToolCalls,
			Actual:  u.ToolCalls,
			Message: "tool call limit exceeded",
		}
	}
	if b.MaxRetries > 0 && u.Retries > b.MaxRetries {
		return &ExceededError{
			Field:   "max_retries",
			Limit:   b.MaxRetries,
			Actual:  u.Retries,
			Message: "retry limit exceeded",
		}
	}
	if b.MaxIterations > 0 && u.Iterations > b.MaxIterations {
		return &ExceededError{
			Field:   "max_iterations",
			Limit:   b.MaxIterations,
			Actual:  u.Iterations,
			Message: "iteration limit exceeded",
		}
	}
	if b.MaxTables > 0 && u.TablesInScope > b.MaxTables {
		return &ExceededError{
			Field:   "max_tables",
			Limit:   b.MaxTables,
			Actual:  u.TablesInScope,
			Message: "table count limit exceeded",
		}
	}
	if b.MaxRowsScanned > 0 && u.RowsScanned > b.MaxRowsScanned {
		return &ExceededError{
			Field:   "max_rows_scanned",
			Limit:   b.MaxRowsScanned,
			Actual:  u.RowsScanned,
			Message: "rows scanned limit exceeded",
		}
	}
	if b.MaxRAGChunks > 0 && u.RAGChunks > b.MaxRAGChunks {
		return &ExceededError{
			Field:   "max_rag_chunks",
			Limit:   b.MaxRAGChunks,
			Actual:  u.RAGChunks,
			Message: "RAG chunk limit exceeded",
		}
	}
	if b.MaxContextChars > 0 && u.ContextChars > b.MaxContextChars {
		return &ExceededError{
			Field:   "max_context_chars",
			Limit:   b.MaxContextChars,
			Actual:  u.ContextChars,
			Message: "context character limit exceeded",
		}
	}
	if b.MaxExecutionTime > 0 && u.ExecutionTime > b.MaxExecutionTime {
		return &ExceededError{
			Field:   "max_execution_time",
			Limit:   b.MaxExecutionTime,
			Actual:  u.ExecutionTime,
			Message: "execution time limit exceeded",
		}
	}
	if b.MaxEstimatedCost > 0 && u.EstimatedCost > b.MaxEstimatedCost {
		return &ExceededError{
			Field:   "max_estimated_cost",
			Limit:   b.MaxEstimatedCost,
			Actual:  u.EstimatedCost,
			Message: "estimated cost limit exceeded",
		}
	}
	return nil
}
