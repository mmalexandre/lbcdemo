package mlflow

import (
	"bytes"
	"encoding/json"
	"fmt"
	"io"
	"log"
	"net/http"
	"net/url"
	"os"
	"time"
)

const defaultExperimentName = "Magician"

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

	expName := os.Getenv("MLFLOW_EXPERIMENT_NAME")
	if expName == "" {
		expName = defaultExperimentName
	}

	expID, err := t.ensureExperiment(expName)
	if err != nil {
		log.Printf("mlflow: could not ensure experiment, using default: %v", err)
		expID = "0"
	}
	t.experimentID = expID
	log.Printf("mlflow: using experiment name=%q id=%s at %s", expName, expID, baseURL)
	return t
}

// NewTracerWithURL creates a Tracer using the given MLflow base URL.
// Intended for integration tests that point at a local stub server.
func NewTracerWithURL(baseURL string) *Tracer {
	t := &Tracer{
		baseURL:    baseURL,
		httpClient: &http.Client{Timeout: 10 * time.Second},
	}
	expName := os.Getenv("MLFLOW_EXPERIMENT_NAME")
	if expName == "" {
		expName = defaultExperimentName
	}

	expID, err := t.ensureExperiment(expName)
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
	promptName, promptVersion string,
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

	tags := []traceKV{
		{Key: "mlflow.user", Value: user},
		{Key: "model", Value: model},
		{Key: "input_tokens", Value: fmt.Sprintf("%d", inputTokens)},
		{Key: "output_tokens", Value: fmt.Sprintf("%d", outputTokens)},
	}
	if promptName != "" {
		tags = append(tags, traceKV{Key: "mlflow.prompt.name", Value: promptName})
	}
	if promptVersion != "" {
		tags = append(tags, traceKV{Key: "mlflow.prompt.version", Value: promptVersion})
	}
	if linkedPrompts := linkedPromptsTagValue(promptName, promptVersion); linkedPrompts != "" {
		tags = append(tags, traceKV{Key: "mlflow.linkedPrompts", Value: linkedPrompts})
	}

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
		"tags": tags,
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

	respBody, _ := io.ReadAll(resp.Body)

	if resp.StatusCode != http.StatusOK {
		log.Printf("mlflow: trace endpoint returned status %d", resp.StatusCode)
		return
	}

	if promptName == "" || promptVersion == "" {
		return
	}

	traceID := extractTraceID(respBody)
	if traceID == "" {
		log.Printf("mlflow: trace created but no trace_id/request_id returned; skipping prompt linking")
		return
	}

	if err := t.linkPromptToTrace(traceID, promptName, promptVersion); err != nil {
		log.Printf("mlflow: link prompt to trace failed: %v", err)
	}
}

func linkedPromptsTagValue(promptName, promptVersion string) string {
	if promptName == "" || promptVersion == "" {
		return ""
	}
	body, err := json.Marshal([]map[string]string{{
		"name":    promptName,
		"version": promptVersion,
	}})
	if err != nil {
		return ""
	}
	return string(body)
}

func extractTraceID(respBody []byte) string {
	var parsed struct {
		RequestID string `json:"request_id"`
		TraceID   string `json:"trace_id"`
		TraceInfo struct {
			RequestID string `json:"request_id"`
			TraceID   string `json:"trace_id"`
		} `json:"trace_info"`
		Trace struct {
			TraceInfo struct {
				RequestID string `json:"request_id"`
				TraceID   string `json:"trace_id"`
			} `json:"trace_info"`
		} `json:"trace"`
	}

	if err := json.Unmarshal(respBody, &parsed); err != nil {
		return ""
	}

	candidates := []string{
		parsed.TraceInfo.TraceID,
		parsed.TraceInfo.RequestID,
		parsed.Trace.TraceInfo.TraceID,
		parsed.Trace.TraceInfo.RequestID,
		parsed.TraceID,
		parsed.RequestID,
	}
	for _, id := range candidates {
		if id != "" {
			return id
		}
	}
	return ""
}

func (t *Tracer) linkPromptToTrace(traceID, promptName, promptVersion string) error {
	body, err := json.Marshal(map[string]interface{}{
		"trace_id": traceID,
		"prompt_versions": []map[string]string{{
			"name":    promptName,
			"version": promptVersion,
		}},
	})
	if err != nil {
		return fmt.Errorf("marshal link payload: %w", err)
	}

	resp, err := t.httpClient.Post(
		fmt.Sprintf("%s/api/2.0/mlflow/traces/link-prompts", t.baseURL),
		"application/json",
		bytes.NewReader(body),
	)
	if err != nil {
		return fmt.Errorf("post link prompt: %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		respBody, _ := io.ReadAll(resp.Body)
		return fmt.Errorf("status %d: %s", resp.StatusCode, string(respBody))
	}

	return nil
}
