//go:build integration

package mlflow

// Integration tests for the MLflow RegistryClient and Tracer.  They exercise
// the full request/response cycle across both components using an in-process
// httptest server that mimics the MLflow REST API.
//
// Run with:
//
//	go test -tags integration ./internal/mlflow/...

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync/atomic"
	"testing"
	"time"
)

// ---------------------------------------------------------------------------
// Shared fake MLflow server
// ---------------------------------------------------------------------------

type fakeMLflow struct {
	// Counts how many traces were received.
	traceCount atomic.Int32

	// promptTemplate is returned for any alias/version lookup.
	promptTemplate string

	// experimentID is returned for experiment get/create calls.
	experimentID string
}

func (f *fakeMLflow) handler() http.Handler {
	mux := http.NewServeMux()

	// Experiment: get-by-name
	mux.HandleFunc("/api/2.0/mlflow/experiments/get-by-name", func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		json.NewEncoder(w).Encode(map[string]interface{}{
			"experiment": map[string]string{"experiment_id": f.experimentID},
		})
	})

	// Experiment: create
	mux.HandleFunc("/api/2.0/mlflow/experiments/create", func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		json.NewEncoder(w).Encode(map[string]string{"experiment_id": f.experimentID})
	})

	// Model version: get by version number
	mux.HandleFunc("/api/2.0/mlflow/model-versions/get", func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		json.NewEncoder(w).Encode(map[string]interface{}{
			"model_version": map[string]interface{}{
				"tags": []map[string]string{
					{"key": "mlflow.prompt.text", "value": f.promptTemplate},
				},
			},
		})
	})

	// Registered model: get by alias
	mux.HandleFunc("/api/2.0/mlflow/registered-models/alias", func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		json.NewEncoder(w).Encode(map[string]interface{}{
			"model_version": map[string]interface{}{
				"tags": []map[string]string{
					{"key": "mlflow.prompt.text", "value": f.promptTemplate},
				},
			},
		})
	})

	// Traces
	mux.HandleFunc("/api/2.0/mlflow/traces", func(w http.ResponseWriter, r *http.Request) {
		f.traceCount.Add(1)
		w.WriteHeader(http.StatusOK)
	})

	return mux
}

func newFakeMLflow(promptTemplate, experimentID string) (*fakeMLflow, *httptest.Server) {
	f := &fakeMLflow{
		promptTemplate: promptTemplate,
		experimentID:   experimentID,
	}
	srv := httptest.NewServer(f.handler())
	return f, srv
}

// ---------------------------------------------------------------------------
// RegistryClient integration tests
// ---------------------------------------------------------------------------

func TestRegistryClient_LoadPromptByAlias(t *testing.T) {
	_, srv := newFakeMLflow("Hello, {{name}}!", "1")
	defer srv.Close()

	rc := NewRegistryClientWithURL(srv.URL)
	template, err := rc.LoadPrompt("prompts:/mymodel/production")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if template != "Hello, {{name}}!" {
		t.Errorf("template: got %q, want %q", template, "Hello, {{name}}!")
	}
}

func TestRegistryClient_LoadPromptByVersion(t *testing.T) {
	_, srv := newFakeMLflow("Version prompt.", "1")
	defer srv.Close()

	rc := NewRegistryClientWithURL(srv.URL)
	template, err := rc.LoadPrompt("prompts:/mymodel/3")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if template != "Version prompt." {
		t.Errorf("template: got %q, want %q", template, "Version prompt.")
	}
}

func TestRegistryClient_LoadAndFormatPrompt(t *testing.T) {
	_, srv := newFakeMLflow("Dear {{user}}, your score is {{score}}.", "1")
	defer srv.Close()

	rc := NewRegistryClientWithURL(srv.URL)
	template, err := rc.LoadPrompt("prompts:/notifications/welcome")
	if err != nil {
		t.Fatalf("load prompt: %v", err)
	}

	result := FormatPrompt(template, map[string]string{
		"user":  "Alice",
		"score": "42",
	})
	want := "Dear Alice, your score is 42."
	if result != want {
		t.Errorf("FormatPrompt: got %q, want %q", result, want)
	}
}

func TestRegistryClient_MissingPromptTag(t *testing.T) {
	// Server returns a model version with no mlflow.prompt.text tag.
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		json.NewEncoder(w).Encode(map[string]interface{}{
			"model_version": map[string]interface{}{
				"tags": []map[string]string{},
			},
		})
	}))
	defer srv.Close()

	rc := NewRegistryClientWithURL(srv.URL)
	_, err := rc.LoadPrompt("prompts:/mymodel/production")
	if err == nil {
		t.Fatal("expected error for missing tag, got nil")
	}
	if !strings.Contains(err.Error(), "no mlflow.prompt.text tag") {
		t.Errorf("unexpected error message: %v", err)
	}
}

// ---------------------------------------------------------------------------
// Tracer integration tests
// ---------------------------------------------------------------------------

func TestTracer_LogLLMTrace_ReceivedByServer(t *testing.T) {
	fake, srv := newFakeMLflow("", "5")
	defer srv.Close()

	tr := NewTracerWithURL(srv.URL)

	tr.LogLLMTrace(
		"alice",
		"What is the capital of France?",
		"Paris.",
		"gpt-4o-mini",
		10, 5,
		time.Now(),
		100*time.Millisecond,
	)

	// LogLLMTrace is synchronous in its HTTP call; give a tiny moment for
	// goroutine-dispatched variants if any, then check.
	time.Sleep(50 * time.Millisecond)

	if got := fake.traceCount.Load(); got != 1 {
		t.Errorf("trace count: got %d, want 1", got)
	}
}

func TestTracer_LogLLMTrace_MultipleTracesAreAllReceived(t *testing.T) {
	fake, srv := newFakeMLflow("", "3")
	defer srv.Close()

	tr := NewTracerWithURL(srv.URL)

	const n = 5
	for i := 0; i < n; i++ {
		tr.LogLLMTrace("user", "prompt", "reply", "model", 1, 1, time.Now(), time.Millisecond)
	}
	time.Sleep(100 * time.Millisecond)

	if got := fake.traceCount.Load(); got != n {
		t.Errorf("trace count: got %d, want %d", got, n)
	}
}

func TestTracer_EnsureExperiment_UsesExistingID(t *testing.T) {
	_, srv := newFakeMLflow("", "99")
	defer srv.Close()

	tr := NewTracerWithURL(srv.URL)
	if tr.experimentID != "99" {
		t.Errorf("experimentID: got %q, want %q", tr.experimentID, "99")
	}
}

// ---------------------------------------------------------------------------
// End-to-end: Registry + Tracer together
// ---------------------------------------------------------------------------

func TestIntegration_RegistryAndTracerWorkflow(t *testing.T) {
	fake, srv := newFakeMLflow("You are {{role}}. Answer concisely.", "7")
	defer srv.Close()

	rc := NewRegistryClientWithURL(srv.URL)
	tr := NewTracerWithURL(srv.URL)

	// Load and format the system prompt.
	template, err := rc.LoadPrompt("prompts:/assistant/production")
	if err != nil {
		t.Fatalf("LoadPrompt: %v", err)
	}
	systemPrompt := FormatPrompt(template, map[string]string{"role": "a helpful assistant"})
	if !strings.Contains(systemPrompt, "a helpful assistant") {
		t.Errorf("formatted prompt: got %q", systemPrompt)
	}

	// Simulate logging an LLM trace for this interaction.
	tr.LogLLMTrace("bob", "Hello", "Hi there!", "gpt-4o-mini", 5, 3, time.Now(), 50*time.Millisecond)
	time.Sleep(50 * time.Millisecond)

	if got := fake.traceCount.Load(); got != 1 {
		t.Errorf("trace count: got %d, want 1", got)
	}
}
