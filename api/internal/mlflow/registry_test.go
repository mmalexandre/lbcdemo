package mlflow

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"
)

// ---------------------------------------------------------------------------
// parsePromptURI
// ---------------------------------------------------------------------------

func TestParsePromptURI_NameAndAlias(t *testing.T) {
	name, ref, err := parsePromptURI("prompts:/assistant/production")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if name != "assistant" {
		t.Errorf("name: got %q, want %q", name, "assistant")
	}
	if ref != "production" {
		t.Errorf("ref: got %q, want %q", ref, "production")
	}
}

func TestParsePromptURI_NameAndVersion(t *testing.T) {
	name, ref, err := parsePromptURI("prompts:/assistant/42")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if name != "assistant" {
		t.Errorf("name: got %q, want %q", name, "assistant")
	}
	if ref != "42" {
		t.Errorf("ref: got %q, want %q", ref, "42")
	}
}

func TestParsePromptURI_NameOnly_DefaultsToLatest(t *testing.T) {
	_, ref, err := parsePromptURI("prompts:/mymodel")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if ref != "latest" {
		t.Errorf("ref: got %q, want %q", ref, "latest")
	}
}

func TestParsePromptURI_DoubleSlash(t *testing.T) {
	name, ref, err := parsePromptURI("prompts://assistant/production")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if name != "assistant" {
		t.Errorf("name: got %q, want %q", name, "assistant")
	}
	if ref != "production" {
		t.Errorf("ref: got %q, want %q", ref, "production")
	}
}

func TestParsePromptURI_InvalidPrefix(t *testing.T) {
	_, _, err := parsePromptURI("http://example.com/model/1")
	if err == nil {
		t.Fatal("expected error for invalid prefix, got nil")
	}
}

func TestParsePromptURI_EmptyName(t *testing.T) {
	_, _, err := parsePromptURI("prompts:/")
	if err == nil {
		t.Fatal("expected error for empty name, got nil")
	}
}

// ---------------------------------------------------------------------------
// isNumeric
// ---------------------------------------------------------------------------

func TestIsNumeric(t *testing.T) {
	cases := []struct {
		input string
		want  bool
	}{
		{"42", true},
		{"0", true},
		{"1234567890", true},
		{"latest", false},
		{"production", false},
		{"", false},
		{"1a", false},
	}
	for _, tc := range cases {
		got := isNumeric(tc.input)
		if got != tc.want {
			t.Errorf("isNumeric(%q) = %v, want %v", tc.input, got, tc.want)
		}
	}
}

// ---------------------------------------------------------------------------
// FormatPrompt
// ---------------------------------------------------------------------------

func TestFormatPrompt_ReplacesPlaceholders(t *testing.T) {
	tmpl := "Hello, {{name}}! Today is {{ day }}."
	vars := map[string]string{"name": "Alice", "day": "Monday"}
	got := FormatPrompt(tmpl, vars)
	want := "Hello, Alice! Today is Monday."
	if got != want {
		t.Errorf("FormatPrompt = %q, want %q", got, want)
	}
}

func TestFormatPrompt_UnknownVarsLeft(t *testing.T) {
	tmpl := "Hello, {{name}}!"
	got := FormatPrompt(tmpl, map[string]string{})
	if got != tmpl {
		t.Errorf("expected template unchanged, got %q", got)
	}
}

func TestFormatPrompt_EmptyTemplate(t *testing.T) {
	got := FormatPrompt("", map[string]string{"x": "y"})
	if got != "" {
		t.Errorf("expected empty string, got %q", got)
	}
}

// ---------------------------------------------------------------------------
// RegistryClient.LoadPrompt via HTTP test server
// ---------------------------------------------------------------------------

func newTestRegistryClient(baseURL string) *RegistryClient {
	return &RegistryClient{
		baseURL:    baseURL,
		token:      "",
		httpClient: &http.Client{},
	}
}

func TestLoadPrompt_ByAlias(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		resp := modelVersionResponse{}
		resp.ModelVersion.Tags = []modelVersionTag{
			{Key: "mlflow.prompt.text", Value: "You are a helpful assistant."},
		}
		json.NewEncoder(w).Encode(resp)
	}))
	defer srv.Close()

	c := newTestRegistryClient(srv.URL)
	text, err := c.LoadPrompt("prompts:/assistant/production")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if text != "You are a helpful assistant." {
		t.Errorf("got %q", text)
	}
}

func TestLoadPrompt_ByVersion(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		resp := modelVersionResponse{}
		resp.ModelVersion.Tags = []modelVersionTag{
			{Key: "mlflow.prompt.template", Value: "Hello {{name}}"},
		}
		json.NewEncoder(w).Encode(resp)
	}))
	defer srv.Close()

	c := newTestRegistryClient(srv.URL)
	text, err := c.LoadPrompt("prompts:/assistant/3")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if text != "Hello {{name}}" {
		t.Errorf("got %q", text)
	}
}

func TestLoadPrompt_MissingTag(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		resp := modelVersionResponse{}
		resp.ModelVersion.Tags = []modelVersionTag{
			{Key: "some.other.tag", Value: "irrelevant"},
		}
		json.NewEncoder(w).Encode(resp)
	}))
	defer srv.Close()

	c := newTestRegistryClient(srv.URL)
	_, err := c.LoadPrompt("prompts:/assistant/production")
	if err == nil {
		t.Fatal("expected error for missing prompt tag, got nil")
	}
}

func TestLoadPrompt_HTTPError(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusInternalServerError)
	}))
	defer srv.Close()

	c := newTestRegistryClient(srv.URL)
	_, err := c.LoadPrompt("prompts:/assistant/production")
	if err == nil {
		t.Fatal("expected error for HTTP 500, got nil")
	}
}

func TestLoadPrompt_InvalidURI(t *testing.T) {
	c := newTestRegistryClient("http://localhost")
	_, err := c.LoadPrompt("not-a-prompt-uri")
	if err == nil {
		t.Fatal("expected error for invalid URI, got nil")
	}
}
