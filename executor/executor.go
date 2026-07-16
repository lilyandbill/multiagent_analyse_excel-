// Package executor provides LLM execution capabilities for the agent harness.
// It supports both a deterministic fake executor for testing and an
// OpenAI-compatible executor for real LLM calls.
package executor

import (
	"bufio"
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"os"
	"strings"
	"time"
)

// ── Types ────────────────────────────────────────────────────────────────

// Request is the input to an executor.
type Request struct {
	Task      string `json:"task"`
	SystemMsg string `json:"system_msg,omitempty"`
	MaxTokens int    `json:"max_tokens,omitempty"`
}

// Result is the structured output from an executor.
type Result struct {
	Text         string `json:"text"`
	Model        string `json:"model,omitempty"`
	InputTokens  int    `json:"input_tokens"`
	OutputTokens int    `json:"output_tokens"`
}

// Usage tracks token consumption from a provider response.
type Usage struct {
	InputTokens  int `json:"prompt_tokens"`
	OutputTokens int `json:"completion_tokens"`
}

// Executor defines the interface for executing tasks.
type Executor interface {
	Execute(ctx context.Context, req Request) (Result, error)
}

// ── Fake Executor ────────────────────────────────────────────────────────

// FakeExecutor returns deterministic results for testing.
type FakeExecutor struct{}

// Execute returns a deterministic echo of the task.
func (f FakeExecutor) Execute(_ context.Context, req Request) (Result, error) {
	return Result{
		Text:         fmt.Sprintf("deterministic result for: %s", req.Task),
		Model:        "fake",
		InputTokens:  10,
		OutputTokens: 20,
	}, nil
}

// ── OpenAI Executor ──────────────────────────────────────────────────────

// OpenAIConfig holds configuration for the OpenAI-compatible executor.
type OpenAIConfig struct {
	APIKey  string
	BaseURL string
	Model   string
	Timeout time.Duration
}

// OpenAIConfigFromEnv reads configuration from a .env file (if present) and
// then from environment variables. Environment variables take priority over
// .env values. This allows .env for local development and env vars for CI/production.
//
// Required: OPENAI_API_KEY, OPENAI_MODEL
// Optional: OPENAI_BASE_URL (default: https://api.openai.com/v1)
func OpenAIConfigFromEnv() (OpenAIConfig, error) {
	// Load .env from current directory (best-effort, ignore errors).
	_ = loadDotEnv(".env")

	apiKey := os.Getenv("OPENAI_API_KEY")
	if apiKey == "" {
		return OpenAIConfig{}, fmt.Errorf("OPENAI_API_KEY is required (set in .env or environment)")
	}
	model := os.Getenv("OPENAI_MODEL")
	if model == "" {
		return OpenAIConfig{}, fmt.Errorf("OPENAI_MODEL is required (set in .env or environment)")
	}
	baseURL := os.Getenv("OPENAI_BASE_URL")
	if baseURL == "" {
		baseURL = "https://api.openai.com/v1"
	}
	baseURL = strings.TrimRight(baseURL, "/")

	return OpenAIConfig{
		APIKey:  apiKey,
		BaseURL: baseURL,
		Model:   model,
		Timeout: 60 * time.Second,
	}, nil
}

// loadDotEnv reads a KEY=VALUE .env file and sets matching environment
// variables. It never overwrites an already-set environment variable.
// Lines starting with # and blank lines are ignored.
// Values may be optionally quoted with single or double quotes.
func loadDotEnv(path string) error {
	f, err := os.Open(path)
	if err != nil {
		return err
	}
	defer f.Close()

	scanner := bufio.NewScanner(f)
	for scanner.Scan() {
		line := strings.TrimSpace(scanner.Text())

		// Skip blank lines and comments.
		if line == "" || strings.HasPrefix(line, "#") {
			continue
		}

		// Parse KEY=VALUE.
		eqIdx := strings.IndexByte(line, '=')
		if eqIdx < 0 {
			continue
		}
		key := strings.TrimSpace(line[:eqIdx])
		value := strings.TrimSpace(line[eqIdx+1:])

		// Strip surrounding quotes.
		value = stripQuotes(value)

		// Skip export prefix.
		key = strings.TrimPrefix(key, "export ")
		key = strings.TrimSpace(key)

		if key == "" {
			continue
		}

		// Don't override non-empty existing env vars.
		if v, exists := os.LookupEnv(key); exists && v != "" {
			continue
		}
		os.Setenv(key, value)
	}
	return scanner.Err()
}

// stripQuotes removes matching single or double quotes from a value.
func stripQuotes(s string) string {
	if len(s) >= 2 {
		if (s[0] == '"' && s[len(s)-1] == '"') ||
			(s[0] == '\'' && s[len(s)-1] == '\'') {
			return s[1 : len(s)-1]
		}
	}
	return s
}

// chatRequest is the OpenAI chat completion request body.
type chatRequest struct {
	Model    string        `json:"model"`
	Messages []chatMessage `json:"messages"`
}

// chatMessage is a single message in the chat completion request.
type chatMessage struct {
	Role    string `json:"role"`
	Content string `json:"content"`
}

// chatResponse is the OpenAI chat completion response body.
type chatResponse struct {
	ID      string         `json:"id"`
	Model   string         `json:"model"`
	Choices []chatChoice   `json:"choices"`
	Usage   *chatUsageData `json:"usage,omitempty"`
	Error   *chatAPIError  `json:"error,omitempty"`
}

type chatChoice struct {
	Message chatMessage `json:"message"`
}

type chatUsageData struct {
	PromptTokens     int `json:"prompt_tokens"`
	CompletionTokens int `json:"completion_tokens"`
	TotalTokens      int `json:"total_tokens"`
}

type chatAPIError struct {
	Message string `json:"message"`
	Code    string `json:"code,omitempty"`
}

// OpenAIExecutor sends requests to an OpenAI-compatible API.
type OpenAIExecutor struct {
	config OpenAIConfig
	client *http.Client
}

// NewOpenAIExecutor creates a new OpenAI executor with the given configuration.
func NewOpenAIExecutor(config OpenAIConfig) *OpenAIExecutor {
	return &OpenAIExecutor{
		config: config,
		client: &http.Client{
			Timeout: config.Timeout,
		},
	}
}

// Execute sends a chat completion request and returns the result.
func (e *OpenAIExecutor) Execute(ctx context.Context, req Request) (Result, error) {
	chatReq := chatRequest{
		Model: e.config.Model,
		Messages: []chatMessage{
			{Role: "user", Content: req.Task},
		},
	}
	if req.SystemMsg != "" {
		chatReq.Messages = append([]chatMessage{
			{Role: "system", Content: req.SystemMsg},
		}, chatReq.Messages...)
	}

	body, err := json.Marshal(chatReq)
	if err != nil {
		return Result{}, fmt.Errorf("marshal request: %w", err)
	}

	url := e.config.BaseURL + "/chat/completions"

	httpReq, err := http.NewRequestWithContext(ctx, http.MethodPost, url, bytes.NewReader(body))
	if err != nil {
		return Result{}, fmt.Errorf("create request: %w", err)
	}
	httpReq.Header.Set("Content-Type", "application/json")
	httpReq.Header.Set("Authorization", "Bearer "+e.config.APIKey)

	resp, err := e.doWithRetry(ctx, httpReq, body)
	if err != nil {
		return Result{}, err
	}
	defer resp.Body.Close()

	respBody, err := io.ReadAll(io.LimitReader(resp.Body, 1<<20)) // 1 MB limit
	if err != nil {
		return Result{}, fmt.Errorf("read response: %w", err)
	}

	var chatResp chatResponse
	if err := json.Unmarshal(respBody, &chatResp); err != nil {
		return Result{}, fmt.Errorf("parse response: %w", err)
	}

	// Check for API-level error.
	if chatResp.Error != nil {
		return Result{}, fmt.Errorf("API error: %s", chatResp.Error.Message)
	}

	if len(chatResp.Choices) == 0 {
		return Result{}, fmt.Errorf("empty response: no choices returned")
	}

	inputTokens := 0
	outputTokens := 0
	if chatResp.Usage != nil {
		inputTokens = chatResp.Usage.PromptTokens
		outputTokens = chatResp.Usage.CompletionTokens
	}

	return Result{
		Text:         chatResp.Choices[0].Message.Content,
		Model:        chatResp.Model,
		InputTokens:  inputTokens,
		OutputTokens: outputTokens,
	}, nil
}

// doWithRetry sends the HTTP request with a small bounded retry policy
// for transient failures (429, 5xx). Auth errors and 4xx are not retried.
func (e *OpenAIExecutor) doWithRetry(ctx context.Context, req *http.Request, body []byte) (*http.Response, error) {
	const maxRetries = 2
	const retryDelay = 500 * time.Millisecond

	var lastErr error

	for attempt := 0; attempt <= maxRetries; attempt++ {
		if attempt > 0 {
			select {
			case <-ctx.Done():
				return nil, ctx.Err()
			case <-time.After(retryDelay * time.Duration(attempt)):
			}
			// Reset body for retry.
			req.Body = io.NopCloser(bytes.NewReader(body))
		}

		resp, err := e.client.Do(req)
		if err != nil {
			lastErr = err
			if errors.Is(err, context.DeadlineExceeded) || errors.Is(err, context.Canceled) {
				return nil, err // don't retry context errors
			}
			continue
		}

		// Don't retry client errors (4xx except 429).
		if resp.StatusCode == http.StatusTooManyRequests ||
			resp.StatusCode >= 500 {
			resp.Body.Close()
			lastErr = fmt.Errorf("HTTP %d", resp.StatusCode)
			continue
		}

		if resp.StatusCode >= 400 {
			defer resp.Body.Close()
			limited := io.LimitReader(resp.Body, 4096)
			errBody, _ := io.ReadAll(limited)
			msg := strings.TrimSpace(string(errBody))
			if len(msg) > 200 {
				msg = msg[:200] + "..."
			}
			return nil, fmt.Errorf("HTTP %d: %s", resp.StatusCode, msg)
		}

		return resp, nil
	}

	return nil, fmt.Errorf("request failed after %d retries: %w", maxRetries+1, lastErr)
}
