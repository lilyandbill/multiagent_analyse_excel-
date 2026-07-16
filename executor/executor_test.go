package executor

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"os"
	"strings"
	"testing"
	"time"
)

// ── Fake Executor Tests ──────────────────────────────────────────────────

func TestFakeExecutor_Deterministic(t *testing.T) {
	f := FakeExecutor{}

	r1, _ := f.Execute(context.Background(), Request{Task: "hello"})
	r2, _ := f.Execute(context.Background(), Request{Task: "hello"})

	if r1.Text != r2.Text {
		t.Errorf("fake executor should be deterministic: %q vs %q", r1.Text, r2.Text)
	}
	if r1.Model != "fake" {
		t.Errorf("model = %q, want fake", r1.Model)
	}
	if r1.InputTokens != 10 || r1.OutputTokens != 20 {
		t.Errorf("tokens = (%d, %d), want (10, 20)", r1.InputTokens, r1.OutputTokens)
	}
}

// ── OpenAI Executor Tests ────────────────────────────────────────────────

func TestOpenAIExecutor_Success(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		// Verify method and path.
		if r.Method != http.MethodPost || r.URL.Path != "/chat/completions" {
			t.Errorf("unexpected request: %s %s", r.Method, r.URL.Path)
		}
		// Verify auth header.
		auth := r.Header.Get("Authorization")
		if auth != "Bearer test-key" {
			t.Errorf("auth = %q, want Bearer test-key", auth)
		}
		// Verify content type.
		if ct := r.Header.Get("Content-Type"); ct != "application/json" {
			t.Errorf("content-type = %q", ct)
		}

		// Parse request body.
		var req chatRequest
		json.NewDecoder(r.Body).Decode(&req)
		if req.Model != "test-model" {
			t.Errorf("model = %q, want test-model", req.Model)
		}
		if len(req.Messages) == 0 || req.Messages[0].Content != "test task" {
			t.Errorf("messages = %v", req.Messages)
		}

		json.NewEncoder(w).Encode(chatResponse{
			ID:    "chat-1",
			Model: "test-model",
			Choices: []chatChoice{
				{Message: chatMessage{Role: "assistant", Content: "Hello, world!"}},
			},
			Usage: &chatUsageData{
				PromptTokens:     15,
				CompletionTokens: 8,
				TotalTokens:      23,
			},
		})
	}))
	defer srv.Close()

	e := NewOpenAIExecutor(OpenAIConfig{
		APIKey:  "test-key",
		BaseURL: srv.URL,
		Model:   "test-model",
		Timeout: 5 * time.Second,
	})

	result, err := e.Execute(context.Background(), Request{Task: "test task"})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if result.Text != "Hello, world!" {
		t.Errorf("text = %q, want Hello, world!", result.Text)
	}
	if result.Model != "test-model" {
		t.Errorf("model = %q", result.Model)
	}
	if result.InputTokens != 15 {
		t.Errorf("input tokens = %d, want 15", result.InputTokens)
	}
	if result.OutputTokens != 8 {
		t.Errorf("output tokens = %d, want 8", result.OutputTokens)
	}
}

func TestOpenAIExecutor_WithSystemMessage(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		var req chatRequest
		json.NewDecoder(r.Body).Decode(&req)
		if len(req.Messages) != 2 {
			t.Errorf("expected 2 messages, got %d", len(req.Messages))
		}
		if req.Messages[0].Role != "system" || req.Messages[0].Content != "be helpful" {
			t.Errorf("system message: %v", req.Messages[0])
		}
		json.NewEncoder(w).Encode(chatResponse{
			Choices: []chatChoice{{Message: chatMessage{Content: "ok"}}},
		})
	}))
	defer srv.Close()

	e := NewOpenAIExecutor(OpenAIConfig{
		APIKey: "test-key", BaseURL: srv.URL, Model: "m", Timeout: 5 * time.Second,
	})
	result, err := e.Execute(context.Background(), Request{
		Task:      "test",
		SystemMsg: "be helpful",
	})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if result.Text != "ok" {
		t.Errorf("text = %q", result.Text)
	}
}

func TestOpenAIExecutor_HTTPError(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusBadRequest)
		w.Write([]byte(`{"error": {"message": "bad request"}}`))
	}))
	defer srv.Close()

	e := NewOpenAIExecutor(OpenAIConfig{
		APIKey: "test-key", BaseURL: srv.URL, Model: "m", Timeout: 5 * time.Second,
	})
	_, err := e.Execute(context.Background(), Request{Task: "test"})
	if err == nil {
		t.Fatal("expected error")
	}
	if !strings.Contains(err.Error(), "400") {
		t.Errorf("error should mention status code: %v", err)
	}
}

func TestOpenAIExecutor_MalformedJSON(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Write([]byte("not json"))
	}))
	defer srv.Close()

	e := NewOpenAIExecutor(OpenAIConfig{
		APIKey: "test-key", BaseURL: srv.URL, Model: "m", Timeout: 5 * time.Second,
	})
	_, err := e.Execute(context.Background(), Request{Task: "test"})
	if err == nil {
		t.Fatal("expected error")
	}
	if !strings.Contains(err.Error(), "parse response") {
		t.Errorf("error should mention parsing: %v", err)
	}
}

func TestOpenAIExecutor_EmptyChoices(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		json.NewEncoder(w).Encode(chatResponse{Choices: []chatChoice{}})
	}))
	defer srv.Close()

	e := NewOpenAIExecutor(OpenAIConfig{
		APIKey: "test-key", BaseURL: srv.URL, Model: "m", Timeout: 5 * time.Second,
	})
	_, err := e.Execute(context.Background(), Request{Task: "test"})
	if err == nil {
		t.Fatal("expected error")
	}
	if !strings.Contains(err.Error(), "no choices") {
		t.Errorf("error should mention empty choices: %v", err)
	}
}

func TestOpenAIExecutor_APIError(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		json.NewEncoder(w).Encode(chatResponse{
			Error: &chatAPIError{Message: "rate limit exceeded", Code: "rate_limit"},
		})
	}))
	defer srv.Close()

	e := NewOpenAIExecutor(OpenAIConfig{
		APIKey: "test-key", BaseURL: srv.URL, Model: "m", Timeout: 5 * time.Second,
	})
	_, err := e.Execute(context.Background(), Request{Task: "test"})
	if err == nil {
		t.Fatal("expected error")
	}
	if !strings.Contains(err.Error(), "rate limit exceeded") {
		t.Errorf("error should contain API message: %v", err)
	}
}

func TestOpenAIExecutor_Timeout(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		time.Sleep(200 * time.Millisecond)
	}))
	defer srv.Close()

	e := NewOpenAIExecutor(OpenAIConfig{
		APIKey: "test-key", BaseURL: srv.URL, Model: "m", Timeout: 50 * time.Millisecond,
	})
	_, err := e.Execute(context.Background(), Request{Task: "test"})
	if err == nil {
		t.Fatal("expected timeout error")
	}
}

func TestOpenAIExecutor_ContextCancellation(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		time.Sleep(500 * time.Millisecond)
	}))
	defer srv.Close()

	e := NewOpenAIExecutor(OpenAIConfig{
		APIKey: "test-key", BaseURL: srv.URL, Model: "m", Timeout: 10 * time.Second,
	})

	ctx, cancel := context.WithCancel(context.Background())
	cancel() // immediate cancellation

	_, err := e.Execute(ctx, Request{Task: "test"})
	if err == nil {
		t.Fatal("expected context cancellation error")
	}
}

func TestOpenAIExecutor_APIKeyNotInError(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusInternalServerError)
	}))
	defer srv.Close()

	e := NewOpenAIExecutor(OpenAIConfig{
		APIKey: "sk-secret-key-12345", BaseURL: srv.URL, Model: "m", Timeout: 5 * time.Second,
	})
	_, err := e.Execute(context.Background(), Request{Task: "test"})
	if err == nil {
		t.Fatal("expected error")
	}
	if strings.Contains(err.Error(), "sk-secret-key-12345") {
		t.Errorf("API key leaked in error: %v", err)
	}
}

func TestOpenAIExecutor_RetryOn429(t *testing.T) {
	attempts := 0
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		attempts++
		if attempts < 3 {
			w.WriteHeader(http.StatusTooManyRequests)
			return
		}
		json.NewEncoder(w).Encode(chatResponse{
			Choices: []chatChoice{{Message: chatMessage{Content: "retry success"}}},
		})
	}))
	defer srv.Close()

	e := NewOpenAIExecutor(OpenAIConfig{
		APIKey: "test-key", BaseURL: srv.URL, Model: "m", Timeout: 5 * time.Second,
	})
	result, err := e.Execute(context.Background(), Request{Task: "test"})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if result.Text != "retry success" {
		t.Errorf("text = %q", result.Text)
	}
	if attempts != 3 {
		t.Errorf("attempts = %d, want 3", attempts)
	}
}

func TestOpenAIExecutor_RetryExhausted(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusServiceUnavailable)
	}))
	defer srv.Close()

	e := NewOpenAIExecutor(OpenAIConfig{
		APIKey: "test-key", BaseURL: srv.URL, Model: "m", Timeout: 5 * time.Second,
	})
	_, err := e.Execute(context.Background(), Request{Task: "test"})
	if err == nil {
		t.Fatal("expected error after retries exhausted")
	}
	if !strings.Contains(err.Error(), "retries") {
		t.Errorf("error should mention retries: %v", err)
	}
}

// ── Configuration Tests ──────────────────────────────────────────────────

func TestOpenAIConfigFromEnv_MissingKey(t *testing.T) {
	t.Setenv("OPENAI_API_KEY", "")
	t.Setenv("OPENAI_MODEL", "gpt-4")
	_, err := OpenAIConfigFromEnv()
	if err == nil || !strings.Contains(err.Error(), "OPENAI_API_KEY") {
		t.Errorf("expected API key error, got: %v", err)
	}
}

func TestOpenAIConfigFromEnv_MissingModel(t *testing.T) {
	t.Setenv("OPENAI_API_KEY", "sk-test")
	t.Setenv("OPENAI_MODEL", "")
	_, err := OpenAIConfigFromEnv()
	if err == nil || !strings.Contains(err.Error(), "OPENAI_MODEL") {
		t.Errorf("expected model error, got: %v", err)
	}
}

func TestOpenAIConfigFromEnv_Defaults(t *testing.T) {
	t.Setenv("OPENAI_API_KEY", "sk-test")
	t.Setenv("OPENAI_MODEL", "gpt-4")
	cfg, err := OpenAIConfigFromEnv()
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if cfg.BaseURL != "https://api.openai.com/v1" {
		t.Errorf("base URL = %q", cfg.BaseURL)
	}
	if cfg.Timeout != 60*time.Second {
		t.Errorf("timeout = %v", cfg.Timeout)
	}
}

func TestOpenAIConfigFromEnv_CustomBaseURL(t *testing.T) {
	t.Setenv("OPENAI_API_KEY", "sk-test")
	t.Setenv("OPENAI_MODEL", "gpt-4")
	t.Setenv("OPENAI_BASE_URL", "https://custom.api.com/v1/")
	cfg, err := OpenAIConfigFromEnv()
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if cfg.BaseURL != "https://custom.api.com/v1" {
		t.Errorf("base URL = %q (trailing slash should be trimmed)", cfg.BaseURL)
	}
}

func TestOpenAIExecutor_APIKeyNotInRequestBody(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		var buf [4096]byte
		n, _ := r.Body.Read(buf[:])
		body := string(buf[:n])
		if strings.Contains(body, "test-api-key-for-body-check") {
			t.Error("API key found in request body")
		}
		json.NewEncoder(w).Encode(chatResponse{
			Choices: []chatChoice{{Message: chatMessage{Content: "ok"}}},
		})
	}))
	defer srv.Close()

	e := NewOpenAIExecutor(OpenAIConfig{
		APIKey: "test-api-key-for-body-check", BaseURL: srv.URL, Model: "m", Timeout: 5 * time.Second,
	})
	_, err := e.Execute(context.Background(), Request{Task: "test"})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
}

func TestOpenAIExecutor_EmptyAssistantContent(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		json.NewEncoder(w).Encode(chatResponse{
			Choices: []chatChoice{{Message: chatMessage{Role: "assistant", Content: ""}}},
			Usage:   &chatUsageData{PromptTokens: 1, CompletionTokens: 0, TotalTokens: 1},
		})
	}))
	defer srv.Close()

	e := NewOpenAIExecutor(OpenAIConfig{
		APIKey: "test-key", BaseURL: srv.URL, Model: "m", Timeout: 5 * time.Second,
	})
	result, err := e.Execute(context.Background(), Request{Task: "test"})
	if err != nil {
		t.Fatalf("empty content is valid (not an error): %v", err)
	}
	if result.Text != "" {
		t.Errorf("text = %q, want empty string", result.Text)
	}
}

// ── Interface Compliance ─────────────────────────────────────────────────

func TestFakeExecutor_ImplementsInterface(t *testing.T) {
	var _ Executor = FakeExecutor{}
}

func TestOpenAIExecutor_ImplementsInterface(t *testing.T) {
	var _ Executor = &OpenAIExecutor{}
}

// ── .env Loading Tests ───────────────────────────────────────────────────

func TestLoadDotEnv_Basic(t *testing.T) {
	dir := t.TempDir()
	path := dir + "/.env"
	content := "OPENAI_API_KEY=sk-from-env-file\nOPENAI_MODEL=gpt-4\n"
	if err := os.WriteFile(path, []byte(content), 0644); err != nil {
		t.Fatal(err)
	}

	// Start clean.
	t.Setenv("OPENAI_API_KEY", "")
	t.Setenv("OPENAI_MODEL", "")

	if err := loadDotEnv(path); err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if os.Getenv("OPENAI_API_KEY") != "sk-from-env-file" {
		t.Errorf("API key = %q, want sk-from-env-file", os.Getenv("OPENAI_API_KEY"))
	}
	if os.Getenv("OPENAI_MODEL") != "gpt-4" {
		t.Errorf("model = %q, want gpt-4", os.Getenv("OPENAI_MODEL"))
	}
}

func TestLoadDotEnv_DoesNotOverride(t *testing.T) {
	dir := t.TempDir()
	path := dir + "/.env"
	os.WriteFile(path, []byte("OPENAI_MODEL=gpt-4\n"), 0644)

	// Env var is already set — .env must not override it.
	t.Setenv("OPENAI_MODEL", "existing-model")

	if err := loadDotEnv(path); err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if os.Getenv("OPENAI_MODEL") != "existing-model" {
		t.Errorf("env var should not be overridden, got %q", os.Getenv("OPENAI_MODEL"))
	}
}

func TestLoadDotEnv_Comments(t *testing.T) {
	dir := t.TempDir()
	path := dir + "/.env"
	content := "# This is a comment\nOPENAI_MODEL=gpt-4\n# Another comment\n"
	os.WriteFile(path, []byte(content), 0644)

	t.Setenv("OPENAI_MODEL", "")

	if err := loadDotEnv(path); err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if os.Getenv("OPENAI_MODEL") != "gpt-4" {
		t.Errorf("model = %q", os.Getenv("OPENAI_MODEL"))
	}
}

func TestLoadDotEnv_QuotedValues(t *testing.T) {
	dir := t.TempDir()
	path := dir + "/.env"
	content := "KEY1=\"double-quoted\"\nKEY2='single-quoted'\nKEY3=unquoted\n"
	os.WriteFile(path, []byte(content), 0644)

	t.Setenv("KEY1", "")
	t.Setenv("KEY2", "")
	t.Setenv("KEY3", "")

	loadDotEnv(path)

	if os.Getenv("KEY1") != "double-quoted" {
		t.Errorf("KEY1 = %q", os.Getenv("KEY1"))
	}
	if os.Getenv("KEY2") != "single-quoted" {
		t.Errorf("KEY2 = %q", os.Getenv("KEY2"))
	}
	if os.Getenv("KEY3") != "unquoted" {
		t.Errorf("KEY3 = %q", os.Getenv("KEY3"))
	}
}

func TestLoadDotEnv_ExportPrefix(t *testing.T) {
	dir := t.TempDir()
	path := dir + "/.env"
	os.WriteFile(path, []byte("export OPENAI_MODEL=gpt-4\n"), 0644)

	t.Setenv("OPENAI_MODEL", "")

	loadDotEnv(path)
	if os.Getenv("OPENAI_MODEL") != "gpt-4" {
		t.Errorf("model = %q", os.Getenv("OPENAI_MODEL"))
	}
}

func TestLoadDotEnv_FileNotFound(t *testing.T) {
	err := loadDotEnv("/nonexistent/path/.env")
	if err == nil {
		t.Error("expected error for missing file")
	}
}

func TestOpenAIConfigFromEnv_WithDotEnv(t *testing.T) {
	// Write a .env to the current temp directory and run from there.
	dir := t.TempDir()
	path := dir + "/.env"
	os.WriteFile(path, []byte("OPENAI_API_KEY=sk-dotenv-key\nOPENAI_MODEL=dotenv-model\n"), 0644)

	// Clear and load.
	t.Setenv("OPENAI_API_KEY", "")
	t.Setenv("OPENAI_MODEL", "")
	t.Setenv("OPENAI_BASE_URL", "")

	if err := loadDotEnv(path); err != nil {
		t.Fatalf("loadDotEnv: %v", err)
	}

	cfg, err := OpenAIConfigFromEnv()
	if err != nil {
		t.Fatalf("OpenAIConfigFromEnv: %v", err)
	}
	if cfg.APIKey != "sk-dotenv-key" {
		t.Errorf("API key = %q", cfg.APIKey)
	}
	if cfg.Model != "dotenv-model" {
		t.Errorf("model = %q", cfg.Model)
	}
	if cfg.BaseURL != "https://api.openai.com/v1" {
		t.Errorf("base URL = %q", cfg.BaseURL)
	}
}

func TestLoadDotEnv_BlankLines(t *testing.T) {
	dir := t.TempDir()
	path := dir + "/.env"
	content := "\n\nOPENAI_MODEL=gpt-4\n\n\n"
	os.WriteFile(path, []byte(content), 0644)

	t.Setenv("OPENAI_MODEL", "")
	loadDotEnv(path)
	if os.Getenv("OPENAI_MODEL") != "gpt-4" {
		t.Errorf("model = %q", os.Getenv("OPENAI_MODEL"))
	}
}
