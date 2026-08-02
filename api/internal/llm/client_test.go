package llm

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"

	openai "github.com/sashabaranov/go-openai"
)

// ---------------------------------------------------------------------------
// NewClient
// ---------------------------------------------------------------------------

func TestNewClient_DefaultModel(t *testing.T) {
	t.Setenv("OPENAI_MODEL", "")
	c := NewClient()
	if c.model != "gpt-4o-mini" {
		t.Errorf("default model: got %q, want %q", c.model, "gpt-4o-mini")
	}
}

func TestNewClient_CustomModel(t *testing.T) {
	t.Setenv("OPENAI_MODEL", "gpt-4o")
	c := NewClient()
	if c.model != "gpt-4o" {
		t.Errorf("custom model: got %q, want %q", c.model, "gpt-4o")
	}
}

// ---------------------------------------------------------------------------
// Chat
// ---------------------------------------------------------------------------

// buildOpenAIResponse constructs a minimal OpenAI-compatible JSON response.
func buildOpenAIResponse(content, model string, promptTokens, completionTokens int) []byte {
	resp := openai.ChatCompletionResponse{
		Model: model,
		Choices: []openai.ChatCompletionChoice{
			{
				Message: openai.ChatCompletionMessage{
					Role:    openai.ChatMessageRoleAssistant,
					Content: content,
				},
			},
		},
		Usage: openai.Usage{
			PromptTokens:     promptTokens,
			CompletionTokens: completionTokens,
		},
	}
	b, _ := json.Marshal(resp)
	return b
}

// newClientWithBaseURL creates a Client pointed at a custom base URL (test server).
func newClientWithBaseURL(baseURL string) *Client {
	cfg := openai.DefaultConfig("test-key")
	cfg.BaseURL = baseURL + "/v1"
	return &Client{
		client: openai.NewClientWithConfig(cfg),
		model:  "gpt-4o-mini",
	}
}

func TestChat_Success(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		w.Write(buildOpenAIResponse("Hello!", "gpt-4o-mini", 10, 5))
	}))
	defer srv.Close()

	c := newClientWithBaseURL(srv.URL)
	resp, err := c.Chat(context.Background(), "You are helpful.", "Hi")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if resp.Content != "Hello!" {
		t.Errorf("content: got %q, want %q", resp.Content, "Hello!")
	}
	if resp.InputTokens != 10 {
		t.Errorf("input tokens: got %d, want 10", resp.InputTokens)
	}
	if resp.OutputTokens != 5 {
		t.Errorf("output tokens: got %d, want 5", resp.OutputTokens)
	}
}

func TestChat_NoChoices(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		resp := openai.ChatCompletionResponse{Model: "gpt-4o-mini", Choices: nil}
		json.NewEncoder(w).Encode(resp)
	}))
	defer srv.Close()

	c := newClientWithBaseURL(srv.URL)
	_, err := c.Chat(context.Background(), "sys", "user")
	if err == nil {
		t.Fatal("expected error for empty choices, got nil")
	}
}

func TestChat_HTTPError(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusInternalServerError)
	}))
	defer srv.Close()

	c := newClientWithBaseURL(srv.URL)
	_, err := c.Chat(context.Background(), "sys", "user")
	if err == nil {
		t.Fatal("expected error for HTTP 500, got nil")
	}
}

func TestChat_NilClient(t *testing.T) {
	c := &Client{client: nil, model: "gpt-4o-mini"}
	_, err := c.Chat(context.Background(), "sys", "user")
	if err == nil {
		t.Fatal("expected error for nil client, got nil")
	}
}

func TestChat_ContextCanceled(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		// Block until client disconnects
		<-r.Context().Done()
	}))
	defer srv.Close()

	c := newClientWithBaseURL(srv.URL)
	ctx, cancel := context.WithCancel(context.Background())
	cancel() // cancel immediately

	_, err := c.Chat(ctx, "sys", "user")
	if err == nil {
		t.Fatal("expected error for canceled context, got nil")
	}
}
