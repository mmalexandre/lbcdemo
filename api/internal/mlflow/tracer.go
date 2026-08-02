package mlflow

import (
	"bytes"
	"encoding/json"
	"fmt"
	"log"
	"net/http"
	"net/url"
	"os"
	"time"
)

// Tracer logs LLM call traces to the MLflow tracking server via REST API.
type Tracer struct {
	baseURL      string
	experimentID string
	httpClient   *http.Client
}

type traceKV struct {
	Key   string `json:"key"`
	Value string `json:"value"`
}

func NewTracer() *Tracer {
	baseURL := os.Getenv("MLFLOW_TRACKING_URI")
	if baseURL == "" {
		baseURL = "http://mlflow:5000"
	}
	t := &Tracer{
		baseURL:    baseURL,
		httpClient: &http.Client{Timeout: 10 * time.Second},
	}

	expID, err := t.ensureExperiment("go-llm-agent")
	if err != nil {
		log.Printf("mlflow: could not ensure experiment, using default: %v", err)
		expID = "0"
	}
	t.experimentID = expID
	log.Printf("mlflow: using experiment id=%s at %s", expID, baseURL)
	return t
}

// NewTracerWithURL creates a Tracer using the given MLflow base URL.
// Intended for integration tests that point at a local stub server.
func NewTracerWithURL(baseURL string) *Tracer {
	t := &Tracer{
		baseURL:    baseURL,
		httpClient: &http.Client{Timeout: 10 * time.Second},
	}
	expID, err := t.ensureExperiment("go-llm-agent")
	if err != nil {
		expID = "0"
	}
	t.experimentID = expID
	return t
}

// ensureExperiment returns the experiment ID for the given name, creating it if needed.
func (t *Tracer) ensureExperiment(name string) (string, error) {
	getURL := fmt.Sprintf("%s/api/2.0/mlflow/experiments/get-by-name?experiment_name=%s",
		t.baseURL, url.QueryEscape(name))

	resp, err := t.httpClient.Get(getURL)
	if err != nil {
		return "", fmt.Errorf("get experiment: %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode == http.StatusOK {
		var result struct {
			Experiment struct {
				ExperimentID string `json:"experiment_id"`
			} `json:"experiment"`
		}
		if err := json.NewDecoder(resp.Body).Decode(&result); err != nil {
			return "", fmt.Errorf("decode experiment: %w", err)
		}
		return result.Experiment.ExperimentID, nil
	}

	// Experiment not found — create it.
	body, _ := json.Marshal(map[string]string{"name": name})
	createResp, err := t.httpClient.Post(
		fmt.Sprintf("%s/api/2.0/mlflow/experiments/create", t.baseURL),
		"application/json",
		bytes.NewReader(body),
	)
	if err != nil {
		return "", fmt.Errorf("create experiment: %w", err)
	}
	defer createResp.Body.Close()

	var created struct {
		ExperimentID string `json:"experiment_id"`
	}
	if err := json.NewDecoder(createResp.Body).Decode(&created); err != nil {
		return "", fmt.Errorf("decode create experiment: %w", err)
	}
	return created.ExperimentID, nil
}

// LogLLMTrace records a completed LLM call as a trace in MLflow.
// It is safe to call concurrently and fails silently (logging errors).
func (t *Tracer) LogLLMTrace(
	user, prompt, reply, model string,
	inputTokens, outputTokens int,
	startTime time.Time,
	duration time.Duration,
) {
	inputs, _ := json.Marshal(map[string]interface{}{
		"messages": []map[string]string{
			{"role": "user", "content": prompt},
		},
	})
	outputs, _ := json.Marshal(map[string]interface{}{
		"choices": []map[string]interface{}{
			{"message": map[string]string{
				"role":    "assistant",
				"content": reply,
			}},
		},
	})

	payload := map[string]interface{}{
		"experiment_id":     t.experimentID,
		"timestamp_ms":      startTime.UnixMilli(),
		"execution_time_ms": duration.Milliseconds(),
		"status":            "OK",
		"request_metadata": []traceKV{
			{Key: "mlflow.traceInputs", Value: string(inputs)},
			{Key: "mlflow.traceOutputs", Value: string(outputs)},
			{Key: "mlflow.traceName", Value: "chat_completion"},
		},
		"tags": []traceKV{
			{Key: "mlflow.user", Value: user},
			{Key: "model", Value: model},
			{Key: "input_tokens", Value: fmt.Sprintf("%d", inputTokens)},
			{Key: "output_tokens", Value: fmt.Sprintf("%d", outputTokens)},
		},
	}

	body, err := json.Marshal(payload)
	if err != nil {
		log.Printf("mlflow: marshal trace: %v", err)
		return
	}

	resp, err := t.httpClient.Post(
		fmt.Sprintf("%s/api/2.0/mlflow/traces", t.baseURL),
		"application/json",
		bytes.NewReader(body),
	)
	if err != nil {
		log.Printf("mlflow: post trace: %v", err)
		return
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		log.Printf("mlflow: trace endpoint returned status %d", resp.StatusCode)
	}
}
