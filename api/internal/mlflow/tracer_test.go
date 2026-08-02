package mlflow

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"
)

// ---------------------------------------------------------------------------
// ensureExperiment
// ---------------------------------------------------------------------------

func newTestTracer(baseURL string) *Tracer {
	return &Tracer{
		baseURL:      baseURL,
		experimentID: "0",
		httpClient:   &http.Client{},
	}
}

func TestEnsureExperiment_ExistsReturnsID(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		resp := struct {
			Experiment struct {
				ExperimentID string `json:"experiment_id"`
			} `json:"experiment"`
		}{}
		resp.Experiment.ExperimentID = "7"
		w.WriteHeader(http.StatusOK)
		json.NewEncoder(w).Encode(resp)
	}))
	defer srv.Close()

	tr := newTestTracer(srv.URL)
	id, err := tr.ensureExperiment("go-llm-agent")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if id != "7" {
		t.Errorf("got id %q, want %q", id, "7")
	}
}

func TestEnsureExperiment_NotFoundCreatesNew(t *testing.T) {
	callCount := 0
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		callCount++
		if r.Method == http.MethodGet {
			// Simulate experiment not found
			w.WriteHeader(http.StatusNotFound)
			return
		}
		// POST: return newly created experiment ID
		resp := struct {
			ExperimentID string `json:"experiment_id"`
		}{ExperimentID: "42"}
		w.WriteHeader(http.StatusOK)
		json.NewEncoder(w).Encode(resp)
	}))
	defer srv.Close()

	tr := newTestTracer(srv.URL)
	id, err := tr.ensureExperiment("new-experiment")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if id != "42" {
		t.Errorf("got id %q, want %q", id, "42")
	}
	if callCount != 2 {
		t.Errorf("expected 2 calls (GET + POST), got %d", callCount)
	}
}

func TestEnsureExperiment_GetError(t *testing.T) {
	// Use a server that immediately closes the connection
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		hj, ok := w.(http.Hijacker)
		if !ok {
			w.WriteHeader(http.StatusInternalServerError)
			return
		}
		conn, _, _ := hj.Hijack()
		conn.Close()
	}))
	defer srv.Close()

	tr := newTestTracer(srv.URL)
	_, err := tr.ensureExperiment("any")
	if err == nil {
		t.Fatal("expected error from closed connection, got nil")
	}
}

// ---------------------------------------------------------------------------
// LogLLMTrace — smoke test: should not panic, logs silently on errors
// ---------------------------------------------------------------------------

func TestLogLLMTrace_NoopOnServerError(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusInternalServerError)
	}))
	defer srv.Close()

	tr := newTestTracer(srv.URL)
	// Must not panic
	tr.LogLLMTrace("user1", "hello", "world", "gpt-4o-mini", 10, 20, time.Now(), time.Millisecond*100)
}

func TestLogLLMTrace_Success(t *testing.T) {
	var received map[string]interface{}

	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		json.NewDecoder(r.Body).Decode(&received)
		w.WriteHeader(http.StatusOK)
	}))
	defer srv.Close()

	tr := newTestTracer(srv.URL)
	tr.experimentID = "5"
	tr.LogLLMTrace("alice", "prompt", "reply", "gpt-4o-mini", 15, 25, time.Now(), time.Second)

	if received == nil {
		t.Fatal("expected tracer to POST a payload, got nothing")
	}
	if received["experiment_id"] != "5" {
		t.Errorf("experiment_id: got %v, want %q", received["experiment_id"], "5")
	}
}
