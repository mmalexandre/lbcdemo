//go:build integration

package main

// Integration tests for the HTTP API. They require a running Postgres instance
// and optionally a running MLflow server. Set the following environment
// variables before running:
//
//	DATABASE_URL   e.g. postgres://postgres:postgres@localhost:5432/appdb?sslmode=disable
//	SESSION_SECRET any non-empty string (defaults to "test-secret" when unset)
//
// Run with:
//
//	go test -tags integration ./cmd/...
//
// The tests create the users table if it does not already exist, seed a test
// user, and clean up afterwards.

import (
	"bytes"
	"database/sql"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"os"
	"testing"
	"time"

	"api/internal/llm"
	"api/internal/mlflow"

	_ "github.com/lib/pq"
	"golang.org/x/crypto/bcrypt"
)

// ---------------------------------------------------------------------------
// Test helpers
// ---------------------------------------------------------------------------

// newTestDB opens a postgres connection using DATABASE_URL; skips if unset.
func newTestDB(t *testing.T) *sql.DB {
	t.Helper()
	dsn := os.Getenv("DATABASE_URL")
	if dsn == "" {
		t.Skip("DATABASE_URL not set – skipping integration test")
	}
	db, err := sql.Open("postgres", dsn)
	if err != nil {
		t.Fatalf("open db: %v", err)
	}
	// Wait up to 5 s for the DB to be ready (useful in CI where DB may be
	// starting at the same time as the test runner).
	deadline := time.Now().Add(5 * time.Second)
	for {
		if err := db.Ping(); err == nil {
			break
		}
		if time.Now().After(deadline) {
			t.Fatalf("db not reachable after 5 s")
		}
		time.Sleep(200 * time.Millisecond)
	}
	return db
}

// seedTestUser inserts a user with the given credentials (hashed password) and
// returns a cleanup function that removes the user afterwards.
func seedTestUser(t *testing.T, db *sql.DB, username, password string) func() {
	t.Helper()
	hash, err := bcrypt.GenerateFromPassword([]byte(password), bcrypt.MinCost)
	if err != nil {
		t.Fatalf("bcrypt: %v", err)
	}
	_, err = db.Exec(
		"INSERT INTO users (username, password_hash) VALUES ($1, $2) ON CONFLICT (username) DO UPDATE SET password_hash = $2",
		username, string(hash),
	)
	if err != nil {
		t.Fatalf("seed user: %v", err)
	}
	return func() {
		db.Exec("DELETE FROM users WHERE username = $1", username)
	}
}

// newNoopLLMClient returns an LLM client whose Chat method always returns a
// fixed response without making any real network calls.
type noopLLMClient struct{}

func (n *noopLLMClient) Chat(_ interface{}, _, _ string) (*llm.Response, error) {
	return &llm.Response{
		Content:      "integration test reply",
		Model:        "noop",
		InputTokens:  1,
		OutputTokens: 1,
	}, nil
}

// noopTracer is an mlflow.Tracer stand-in that discards all trace calls.
type noopTracer struct{}

func (n *noopTracer) LogLLMTrace(_, _, _, _ string, _, _ int, _ time.Time, _ time.Duration) {}

// noopRegistry is an mlflow.RegistryClient stand-in that always returns a
// fixed system prompt.
type noopRegistry struct{}

func (n *noopRegistry) LoadPrompt(_ string) (string, error) {
	return "You are a test assistant.", nil
}

// ---------------------------------------------------------------------------
// Router setup for integration tests
//
// We use the real newRouter but substitute no-op LLM/tracer/registry stubs so
// that the tests do not require OpenAI credentials or a live MLflow server.
// ---------------------------------------------------------------------------

// Since newRouter accepts concrete types we build slim fakes that satisfy the
// same interfaces used inside the handlers.  The actual handler code accesses
// llmClient.Chat, mlflowTracer.LogLLMTrace and promptRegistry.LoadPrompt, so
// we swap in lightweight httptest-backed stubs for the LLM and MLflow servers.

// fakeLLMServer returns a test HTTP server that mimics the OpenAI chat
// completion endpoint. The returned URL should be used as the base URL for
// the llm.Client created inside the test.
func fakeLLMServer(t *testing.T, replyContent string) *httptest.Server {
	t.Helper()
	return httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		resp := map[string]interface{}{
			"id":      "chatcmpl-test",
			"object":  "chat.completion",
			"model":   "gpt-4o-mini",
			"choices": []map[string]interface{}{{"index": 0, "message": map[string]string{"role": "assistant", "content": replyContent}, "finish_reason": "stop"}},
			"usage":   map[string]int{"prompt_tokens": 10, "completion_tokens": 5, "total_tokens": 15},
		}
		json.NewEncoder(w).Encode(resp)
	}))
}

// fakeMLflowServer returns a minimal MLflow-compatible server used for the
// tracer and registry during integration tests.
func fakeMLflowServer(t *testing.T) *httptest.Server {
	t.Helper()
	return httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		switch {
		case r.Method == http.MethodGet && r.URL.Path == "/api/2.0/mlflow/experiments/get-by-name":
			json.NewEncoder(w).Encode(map[string]interface{}{
				"experiment": map[string]string{"experiment_id": "1"},
			})
		case r.URL.Path == "/api/2.0/mlflow/traces":
			w.WriteHeader(http.StatusOK)
		case r.Method == http.MethodGet && r.URL.Path == "/api/2.0/mlflow/registered-models/alias":
			json.NewEncoder(w).Encode(map[string]interface{}{
				"model_version": map[string]interface{}{
					"tags": []map[string]string{
						{"key": "mlflow.prompt.text", "value": "You are a test assistant."},
					},
				},
			})
		default:
			w.WriteHeader(http.StatusNotFound)
		}
	}))
}

// buildTestRouter constructs the Gin router wired to the provided DB and stub
// external services.
func buildTestRouter(t *testing.T, db *sql.DB) http.Handler {
	t.Helper()

	llmSrv := fakeLLMServer(t, "integration test reply")
	t.Cleanup(llmSrv.Close)

	mlflowSrv := fakeMLflowServer(t)
	t.Cleanup(mlflowSrv.Close)

	t.Setenv("OPENAI_API_KEY", "test-key")
	t.Setenv("MLFLOW_TRACKING_URI", mlflowSrv.URL)

	llmClient := llm.NewClientWithBaseURL(llmSrv.URL+"/v1", "test-key")
	tracer := mlflow.NewTracerWithURL(mlflowSrv.URL)
	registry := mlflow.NewRegistryClientWithURL(mlflowSrv.URL)

	return newRouter(db, llmClient, tracer, registry, "test-secret", "http://localhost:5173")
}

// ---------------------------------------------------------------------------
// Helpers
// ---------------------------------------------------------------------------

func doJSON(t *testing.T, handler http.Handler, method, path string, body interface{}, cookies []*http.Cookie) *httptest.ResponseRecorder {
	t.Helper()
	var bodyBytes []byte
	if body != nil {
		var err error
		bodyBytes, err = json.Marshal(body)
		if err != nil {
			t.Fatalf("marshal body: %v", err)
		}
	}
	req := httptest.NewRequest(method, path, bytes.NewReader(bodyBytes))
	req.Header.Set("Content-Type", "application/json")
	for _, c := range cookies {
		req.AddCookie(c)
	}
	w := httptest.NewRecorder()
	handler.ServeHTTP(w, req)
	return w
}

func getCookies(w *httptest.ResponseRecorder) []*http.Cookie {
	resp := w.Result()
	return resp.Cookies()
}

// ---------------------------------------------------------------------------
// Tests
// ---------------------------------------------------------------------------

func TestIntegration_LoginSuccess(t *testing.T) {
	db := newTestDB(t)
	defer db.Close()
	cleanup := seedTestUser(t, db, "integ_user", "pass123")
	defer cleanup()

	router := buildTestRouter(t, db)

	w := doJSON(t, router, http.MethodPost, "/api/login",
		map[string]string{"username": "integ_user", "password": "pass123"}, nil)

	if w.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d: %s", w.Code, w.Body.String())
	}
	var resp map[string]string
	json.NewDecoder(w.Body).Decode(&resp)
	if resp["username"] != "integ_user" {
		t.Errorf("username: got %q, want %q", resp["username"], "integ_user")
	}
}

func TestIntegration_LoginWrongPassword(t *testing.T) {
	db := newTestDB(t)
	defer db.Close()
	cleanup := seedTestUser(t, db, "integ_user2", "correct")
	defer cleanup()

	router := buildTestRouter(t, db)

	w := doJSON(t, router, http.MethodPost, "/api/login",
		map[string]string{"username": "integ_user2", "password": "wrong"}, nil)

	if w.Code != http.StatusUnauthorized {
		t.Fatalf("expected 401, got %d", w.Code)
	}
}

func TestIntegration_LoginUnknownUser(t *testing.T) {
	db := newTestDB(t)
	defer db.Close()

	router := buildTestRouter(t, db)

	w := doJSON(t, router, http.MethodPost, "/api/login",
		map[string]string{"username": "nobody", "password": "x"}, nil)

	if w.Code != http.StatusUnauthorized {
		t.Fatalf("expected 401, got %d", w.Code)
	}
}

func TestIntegration_ProtectedEndpointRequiresAuth(t *testing.T) {
	db := newTestDB(t)
	defer db.Close()

	router := buildTestRouter(t, db)

	w := doJSON(t, router, http.MethodPost, "/api/prompt",
		map[string]string{"prompt": "hello"}, nil)

	if w.Code != http.StatusUnauthorized {
		t.Fatalf("expected 401, got %d", w.Code)
	}
}

func TestIntegration_MeEndpoint(t *testing.T) {
	db := newTestDB(t)
	defer db.Close()
	cleanup := seedTestUser(t, db, "integ_me", "pass")
	defer cleanup()

	router := buildTestRouter(t, db)

	// Login first
	loginResp := doJSON(t, router, http.MethodPost, "/api/login",
		map[string]string{"username": "integ_me", "password": "pass"}, nil)
	if loginResp.Code != http.StatusOK {
		t.Fatalf("login failed: %d %s", loginResp.Code, loginResp.Body.String())
	}
	cookies := getCookies(loginResp)

	// /me with session cookie
	w := doJSON(t, router, http.MethodGet, "/api/me", nil, cookies)
	if w.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d: %s", w.Code, w.Body.String())
	}
	var resp map[string]string
	json.NewDecoder(w.Body).Decode(&resp)
	if resp["username"] != "integ_me" {
		t.Errorf("username: got %q, want %q", resp["username"], "integ_me")
	}
}

func TestIntegration_MeEndpointWithoutAuth(t *testing.T) {
	db := newTestDB(t)
	defer db.Close()

	router := buildTestRouter(t, db)

	w := doJSON(t, router, http.MethodGet, "/api/me", nil, nil)
	if w.Code != http.StatusUnauthorized {
		t.Fatalf("expected 401, got %d", w.Code)
	}
}

func TestIntegration_LogoutClearsSession(t *testing.T) {
	db := newTestDB(t)
	defer db.Close()
	cleanup := seedTestUser(t, db, "integ_logout", "pass")
	defer cleanup()

	router := buildTestRouter(t, db)

	// Login
	loginResp := doJSON(t, router, http.MethodPost, "/api/login",
		map[string]string{"username": "integ_logout", "password": "pass"}, nil)
	if loginResp.Code != http.StatusOK {
		t.Fatalf("login failed: %d", loginResp.Code)
	}
	cookies := getCookies(loginResp)

	// Logout
	doJSON(t, router, http.MethodPost, "/api/logout", nil, cookies)

	// /me should now be unauthorized (session is cleared, so old cookie is invalid)
	w := doJSON(t, router, http.MethodGet, "/api/me", nil, cookies)
	if w.Code != http.StatusUnauthorized {
		t.Fatalf("expected 401 after logout, got %d", w.Code)
	}
}

func TestIntegration_PromptEndpointReturnsReply(t *testing.T) {
	db := newTestDB(t)
	defer db.Close()
	cleanup := seedTestUser(t, db, "integ_prompt", "pass")
	defer cleanup()

	router := buildTestRouter(t, db)

	// Login
	loginResp := doJSON(t, router, http.MethodPost, "/api/login",
		map[string]string{"username": "integ_prompt", "password": "pass"}, nil)
	if loginResp.Code != http.StatusOK {
		t.Fatalf("login failed: %d", loginResp.Code)
	}
	cookies := getCookies(loginResp)

	// Send prompt
	w := doJSON(t, router, http.MethodPost, "/api/prompt",
		map[string]string{"prompt": "Hello, world!"}, cookies)
	if w.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d: %s", w.Code, w.Body.String())
	}
	var resp PromptResponse
	json.NewDecoder(w.Body).Decode(&resp)
	if resp.Reply == "" {
		t.Error("expected non-empty reply")
	}
}
