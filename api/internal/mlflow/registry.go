package mlflow

import (
	"encoding/json"
	"fmt"
	"net/http"
	"net/url"
	"os"
	"strings"
	"time"
	"unicode"
)

// RegistryClient loads prompts from the MLflow Prompt Registry via REST API.
type RegistryClient struct {
	baseURL    string
	token      string
	httpClient *http.Client
}

func NewRegistryClient() *RegistryClient {
	baseURL := os.Getenv("MLFLOW_TRACKING_URI")
	if baseURL == "" {
		baseURL = "http://mlflow:5000"
	}
	return &RegistryClient{
		baseURL:    baseURL,
		token:      os.Getenv("MLFLOW_TRACKING_TOKEN"),
		httpClient: &http.Client{Timeout: 10 * time.Second},
	}
}

type modelVersionTag struct {
	Key   string `json:"key"`
	Value string `json:"value"`
}

type modelVersionResponse struct {
	ModelVersion struct {
		Tags []modelVersionTag `json:"tags"`
	} `json:"model_version"`
}

// LoadPrompt fetches the template text for the given prompt URI.
//
// Supported URI formats:
//
//	prompts:/name/alias        (e.g. prompts:/assistant/production)
//	prompts:/name/version      (e.g. prompts:/assistant/1)
//	prompts:/name              (resolves to @latest)
func (p *RegistryClient) LoadPrompt(promptURI string) (string, error) {
	name, ref, err := parsePromptURI(promptURI)
	if err != nil {
		return "", err
	}

	var mv modelVersionResponse

	if isNumeric(ref) {
		apiURL := fmt.Sprintf("%s/api/2.0/mlflow/model-versions/get?name=%s&version=%s",
			p.baseURL, url.QueryEscape(name), url.QueryEscape(ref))
		if err := p.getJSON(apiURL, &mv); err != nil {
			return "", fmt.Errorf("load prompt version %s/%s: %w", name, ref, err)
		}
	} else {
		apiURL := fmt.Sprintf("%s/api/2.0/mlflow/registered-models/alias?name=%s&alias=%s",
			p.baseURL, url.QueryEscape(name), url.QueryEscape(ref))
		if err := p.getJSON(apiURL, &mv); err != nil {
			return "", fmt.Errorf("load prompt alias %s@%s: %w", name, ref, err)
		}
	}

	for _, tag := range mv.ModelVersion.Tags {
		if tag.Key == "mlflow.prompt.text" || tag.Key == "mlflow.prompt.template" {
			return tag.Value, nil
		}
	}
	return "", fmt.Errorf("prompt %q has no mlflow.prompt.text tag", promptURI)
}

// FormatPrompt fills in {{variable}} placeholders in the template.
func FormatPrompt(template string, vars map[string]string) string {
	result := template
	for k, v := range vars {
		result = strings.ReplaceAll(result, "{{"+k+"}}", v)
		result = strings.ReplaceAll(result, "{{ "+k+" }}", v)
	}
	return result
}

func parsePromptURI(uri string) (name, ref string, err error) {
	const prefix = "prompts:/"
	if !strings.HasPrefix(uri, prefix) {
		return "", "", fmt.Errorf("invalid prompt URI %q: must start with %q", uri, prefix)
	}
	rest := strings.TrimPrefix(uri, prefix)
	rest = strings.TrimPrefix(rest, "/")
	parts := strings.SplitN(rest, "/", 2)
	name = parts[0]
	if name == "" {
		return "", "", fmt.Errorf("invalid prompt URI %q: name is empty", uri)
	}
	ref = "latest"
	if len(parts) == 2 && parts[1] != "" {
		ref = parts[1]
	}
	return name, ref, nil
}

func isNumeric(s string) bool {
	if s == "" {
		return false
	}
	for _, r := range s {
		if !unicode.IsDigit(r) {
			return false
		}
	}
	return true
}

func (p *RegistryClient) getJSON(apiURL string, dest interface{}) error {
	req, err := http.NewRequest(http.MethodGet, apiURL, nil)
	if err != nil {
		return fmt.Errorf("build request: %w", err)
	}
	if p.token != "" {
		req.Header.Set("Authorization", "Bearer "+p.token)
	}
	resp, err := p.httpClient.Do(req)
	if err != nil {
		return fmt.Errorf("http get: %w", err)
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		return fmt.Errorf("unexpected status %d", resp.StatusCode)
	}
	if err := json.NewDecoder(resp.Body).Decode(dest); err != nil {
		return fmt.Errorf("decode response: %w", err)
	}
	return nil
}
