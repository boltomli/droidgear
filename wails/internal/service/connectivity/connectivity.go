package connectivity

// Model connectivity testing and diagnostics, ported from
// droidgear-core::connectivity (src-tauri/crates/droidgear-core/src/
// connectivity.rs) plus the thin command layer in
// src-tauri/src/commands/connectivity.rs.
//
// Two test modes exist: "ping" (HTTP probe on {base}/v1/models, free) and
// "inference" (real chat request, costs tokens). Timeouts mirror the Rust
// tester: the configured timeout (default 10s) for pings, at least 30s for
// inference.

import (
	"bytes"
	"droidgear-wails/internal/service/factory"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net"
	"net/http"
	"strings"
	"time"

	"droidgear-wails/internal/core"
)

// ---------------------------------------------------------------------------
// Types (mirroring droidgear-core::connectivity)
// ---------------------------------------------------------------------------

// ConnectionDiagnostics mirrors connectivity::ConnectionDiagnostics.
type ConnectionDiagnostics struct {
	Success      bool    `json:"success"`
	Provider     string  `json:"provider"`
	ModelID      string  `json:"modelId"`
	LatencyMS    uint32  `json:"latencyMs"`
	Error        *string `json:"error,omitempty"`
	Timestamp    string  `json:"timestamp"`
	TestMode     string  `json:"testMode"`
	ResponseText *string `json:"responseText,omitempty"`
	PromptUsed   *string `json:"promptUsed,omitempty"`
}

// ModelTestResult mirrors connectivity::ModelTestResult.
type ModelTestResult struct {
	ModelID     string                `json:"modelId"`
	ModelName   string                `json:"modelName"`
	Diagnostics ConnectionDiagnostics `json:"diagnostics"`
	IsAvailable bool                  `json:"isAvailable"`
}

// ConnectivitySummary mirrors connectivity::ConnectivitySummary.
type ConnectivitySummary struct {
	TotalModels       uint32  `json:"totalModels"`
	AvailableModels   uint32  `json:"availableModels"`
	UnavailableModels uint32  `json:"unavailableModels"`
	AvgLatencyMS      float64 `json:"avgLatencyMs"`
	LastUpdated       string  `json:"lastUpdated"`
}

// TestMode values (bindings.ts: type TestMode = "ping" | "inference").
const (
	connectivityModePing      = "ping"
	connectivityModeInference = "inference"
)

// connectivityDefaultInferencePrompt mirrors connectivity::
// DEFAULT_INFERENCE_PROMPT.
const connectivityDefaultInferencePrompt = "Hi"

// ---------------------------------------------------------------------------
// Model tester (mirroring connectivity::ModelTester)
// ---------------------------------------------------------------------------

// connectivityTestConfig mirrors connectivity::TestConfig. The retry fields
// are carried for fidelity with the Rust struct and its defaults test; the
// Rust tester never retries either.
type connectivityTestConfig struct {
	TimeoutSeconds uint32
	RetryOnFailure bool
	MaxRetries     uint32
}

func connectivityDefaultTestConfig() connectivityTestConfig {
	return connectivityTestConfig{
		TimeoutSeconds: 10,
		RetryOnFailure: true,
		MaxRetries:     2,
	}
}

// connectivityTester mirrors connectivity::ModelTester.
type connectivityTester struct {
	config connectivityTestConfig
}

// connectivityNewTester mirrors ModelTester::new (default TestConfig).
func connectivityNewTester() connectivityTester {
	return connectivityTester{config: connectivityDefaultTestConfig()}
}

func (t connectivityTester) pingTimeout() time.Duration {
	return time.Duration(t.config.TimeoutSeconds) * time.Second
}

// inferenceTimeout mirrors `config.timeout_seconds.max(30)`.
func (t connectivityTester) inferenceTimeout() time.Duration {
	seconds := t.config.TimeoutSeconds
	if seconds < 30 {
		seconds = 30
	}
	return time.Duration(seconds) * time.Second
}

// testModel mirrors ModelTester::test_model (ping a BYOK custom model).
func (t connectivityTester) testModel(provider, baseURL, apiKey, modelID string) ConnectionDiagnostics {
	start := time.Now()
	providerStr := connectivityProviderString(provider)
	if apiKey == "" {
		return connectivityPingDiagnostics(providerStr, modelID, start, errString("No API key configured"))
	}
	probeErr := connectivityProbeEndpoint(provider, baseURL, apiKey, t.pingTimeout())
	return connectivityPingDiagnostics(providerStr, modelID, start, probeErr)
}

// testModelDirect mirrors ModelTester::test_model_direct: the diagnostics
// echo the provider string exactly as passed (no normalisation).
func (t connectivityTester) testModelDirect(provider, baseURL, apiKey, modelID string) ConnectionDiagnostics {
	start := time.Now()
	if apiKey == "" {
		return connectivityPingDiagnostics(provider, modelID, start, errString("No API key configured"))
	}
	probeErr := connectivityProbeEndpoint(provider, baseURL, apiKey, t.pingTimeout())
	return connectivityPingDiagnostics(provider, modelID, start, probeErr)
}

// testModelInference mirrors ModelTester::test_model_inference (a real
// model call validating key, model and quota).
func (t connectivityTester) testModelInference(provider, baseURL, apiKey, modelID, prompt string) ConnectionDiagnostics {
	start := time.Now()
	providerStr := connectivityProviderString(provider)
	if apiKey == "" {
		return connectivityInferenceDiagnostics(providerStr, modelID, prompt, start, "", errString("No API key configured"))
	}
	text, inferErr := connectivityRunInference(provider, baseURL, apiKey, modelID, prompt, t.inferenceTimeout())
	return connectivityInferenceDiagnostics(providerStr, modelID, prompt, start, text, inferErr)
}

// ---------------------------------------------------------------------------
// HTTP probe / inference (mirrors probe_endpoint / run_inference)
// ---------------------------------------------------------------------------

// connectivityProbeEndpoint mirrors connectivity::ModelTester::probe_endpoint:
// GET {base}/v1/models with the provider's auth headers.
func connectivityProbeEndpoint(provider, baseURL, apiKey string, timeout time.Duration) error {
	base := strings.TrimRight(baseURL, "/")
	url := base + "/v1/models"
	request, err := http.NewRequest(http.MethodGet, url, nil) // #nosec G107 -- provider URL from user config
	if err != nil {
		return fmt.Errorf("Network error: %v", err)
	}
	if connectivityProviderString(provider) == "anthropic" {
		request.Header.Set("x-api-key", apiKey)
		request.Header.Set("anthropic-version", "2023-06-01")
	} else {
		request.Header.Set("Authorization", "Bearer "+apiKey)
	}

	client := core.NewHTTPClient(timeout)
	response, err := client.Do(request)
	if err != nil {
		if connectivityIsTimeout(err) {
			return errString("Request timed out")
		}
		return fmt.Errorf("Network error: %v", err)
	}
	defer func() { _ = response.Body.Close() }()

	if response.StatusCode >= 200 && response.StatusCode < 300 {
		return nil
	}
	body, readErr := io.ReadAll(io.LimitReader(response.Body, 8<<10))
	if readErr != nil {
		body = []byte("Unable to read response body")
	}
	return errString(fmt.Sprintf("HTTP %d %s: %s",
		response.StatusCode, connectivityStatusText(response.StatusCode), core.TruncateBody(string(body))))
}

// connectivityRunInference mirrors connectivity::ModelTester::run_inference:
// POST a 32-token chat request and return the response text.
func connectivityRunInference(provider, baseURL, apiKey, modelID, prompt string, timeout time.Duration) (string, error) {
	base := strings.TrimRight(baseURL, "/")
	payload := connectivityInferencePayload{
		Model:     modelID,
		MaxTokens: 32,
		Messages:  []connectivityChatMessage{{Role: "user", Content: prompt}},
	}
	body, _ := json.Marshal(payload) // cannot fail: all fields are plain strings/ints

	url := base + "/v1/chat/completions"
	if connectivityProviderString(provider) == "anthropic" {
		url = base + "/v1/messages"
	}
	request, err := http.NewRequest(http.MethodPost, url, bytes.NewReader(body)) // #nosec G107 -- provider URL from user config
	if err != nil {
		return "", fmt.Errorf("Inference network error: %v", err)
	}
	request.Header.Set("Content-Type", "application/json")
	if connectivityProviderString(provider) == "anthropic" {
		request.Header.Set("x-api-key", apiKey)
		request.Header.Set("anthropic-version", "2023-06-01")
	} else {
		request.Header.Set("Authorization", "Bearer "+apiKey)
	}

	client := core.NewHTTPClient(timeout)
	response, err := client.Do(request)
	if err != nil {
		if connectivityIsTimeout(err) {
			return "", errString("Inference request timed out")
		}
		return "", fmt.Errorf("Inference network error: %v", err)
	}
	defer func() { _ = response.Body.Close() }()

	responseBody, err := io.ReadAll(io.LimitReader(response.Body, 8<<20))
	if err != nil {
		return "", fmt.Errorf("Failed to read inference response body: %v", err)
	}
	if response.StatusCode < 200 || response.StatusCode >= 300 {
		return "", errString(fmt.Sprintf("Inference HTTP %d %s: %s",
			response.StatusCode, connectivityStatusText(response.StatusCode), core.TruncateBody(string(responseBody))))
	}

	var data any
	if err := json.Unmarshal(responseBody, &data); err != nil {
		return "", fmt.Errorf("Failed to parse inference response: %v", err)
	}
	return connectivityTruncateResponseText(connectivityInferenceResponseText(provider, data)), nil
}

// connectivityChatMessage mirrors the chat message shape sent by run_inference.
type connectivityChatMessage struct {
	Role    string `json:"role"`
	Content string `json:"content"`
}

// connectivityInferencePayload mirrors the JSON body sent by run_inference
// (field order matches the Rust serde_json::json! object).
type connectivityInferencePayload struct {
	Model     string                    `json:"model"`
	MaxTokens int                       `json:"max_tokens"`
	Messages  []connectivityChatMessage `json:"messages"`
}

// connectivityIsTimeout reports whether the client deadline fired (mirrors
// tokio::time::timeout around req.send() in the Rust tester).
func connectivityIsTimeout(err error) bool {
	var netErr net.Error
	return errors.As(err, &netErr) && netErr.Timeout()
}

// connectivityStatusText renders the HTTP reason phrase the way Rust's
// http::StatusCode Display does ("404 Not Found").
func connectivityStatusText(code int) string {
	if text := http.StatusText(code); text != "" {
		return text
	}
	return "<unknown status code>"
}

// connectivityInferenceResponseText extracts the response text using the
// provider's wire format; missing/mistyped fields yield "" (mirrors the
// serde_json Option chains).
func connectivityInferenceResponseText(provider string, data any) string {
	if connectivityProviderString(provider) == "anthropic" {
		return connectivityAnthropicResponseText(data)
	}
	return connectivityOpenAIResponseText(data)
}

// connectivityAnthropicResponseText extracts content[0].text from the
// /v1/messages response shape.
func connectivityAnthropicResponseText(data any) string {
	object, ok := data.(map[string]any)
	if !ok {
		return ""
	}
	content, ok := object["content"].([]any)
	if !ok || len(content) == 0 {
		return ""
	}
	block, ok := content[0].(map[string]any)
	if !ok {
		return ""
	}
	text, _ := block["text"].(string)
	return text
}

// connectivityOpenAIResponseText extracts choices[0].message.content from
// the /v1/chat/completions response shape.
func connectivityOpenAIResponseText(data any) string {
	object, ok := data.(map[string]any)
	if !ok {
		return ""
	}
	choices, ok := object["choices"].([]any)
	if !ok || len(choices) == 0 {
		return ""
	}
	choice, ok := choices[0].(map[string]any)
	if !ok {
		return ""
	}
	message, ok := choice["message"].(map[string]any)
	if !ok {
		return ""
	}
	text, _ := message["content"].(string)
	return text
}

// connectivityTruncateResponseText mirrors Rust's 200-byte truncation with
// an ellipsis (Go slices bytes without the Rust char-boundary panic).
func connectivityTruncateResponseText(text string) string {
	if len(text) > 200 {
		return text[:200] + "…"
	}
	return text
}

// ---------------------------------------------------------------------------
// Diagnostics builders / helpers
// ---------------------------------------------------------------------------

// connectivityTimestamp formats a UTC timestamp like chrono's to_rfc3339
// (fractional seconds, +00:00 offset).
func connectivityTimestamp() string {
	return time.Now().UTC().Format("2006-01-02T15:04:05.999999999+00:00")
}

// connectivityProviderString mirrors connectivity::provider_to_string. The
// Go factory.CustomModel carries the kebab-case wire value ("anthropic", "openai",
// "generic-chat-completion-api"); diagnostics report the Rust short names.
func connectivityProviderString(provider string) string {
	switch provider {
	case "anthropic":
		return "anthropic"
	case "openai":
		return "openai"
	default:
		return "generic"
	}
}

func connectivityPingDiagnostics(provider, modelID string, start time.Time, probeErr error) ConnectionDiagnostics {
	diagnostics := ConnectionDiagnostics{
		Success:   probeErr == nil,
		Provider:  provider,
		ModelID:   modelID,
		LatencyMS: uint32(time.Since(start).Milliseconds()),
		Timestamp: connectivityTimestamp(),
		TestMode:  connectivityModePing,
	}
	if probeErr != nil {
		message := probeErr.Error()
		diagnostics.Error = &message
	}
	return diagnostics
}

func connectivityInferenceDiagnostics(provider, modelID, prompt string, start time.Time, responseText string, inferErr error) ConnectionDiagnostics {
	diagnostics := ConnectionDiagnostics{
		Success:    inferErr == nil,
		Provider:   provider,
		ModelID:    modelID,
		LatencyMS:  uint32(time.Since(start).Milliseconds()),
		Timestamp:  connectivityTimestamp(),
		TestMode:   connectivityModeInference,
		PromptUsed: &prompt,
	}
	if inferErr != nil {
		message := inferErr.Error()
		diagnostics.Error = &message
	} else {
		diagnostics.ResponseText = &responseText
	}
	return diagnostics
}

// connectivityModeIsInference validates the TestMode argument the way the
// Tauri layer deserializes it (unknown variants are rejected) before any
// model loading happens.
func connectivityModeIsInference(mode string) (bool, error) {
	switch mode {
	case connectivityModePing:
		return false, nil
	case connectivityModeInference:
		return true, nil
	default:
		return false, fmt.Errorf("unknown variant `%s`, expected `ping` or `inference`", mode)
	}
}

// connectivityPromptOrDefault mirrors `prompt.unwrap_or(DEFAULT_INFERENCE_PROMPT)`.
func connectivityPromptOrDefault(prompt *string) string {
	if prompt != nil {
		return *prompt
	}
	return connectivityDefaultInferencePrompt
}

// connectivityCustomModelDisplay mirrors the display-name fallback used for
// ModelTestResult.model_name.
func connectivityCustomModelDisplay(model factory.CustomModel) string {
	if model.DisplayName != nil {
		return *model.DisplayName
	}
	return model.Model
}

// ---------------------------------------------------------------------------
// Commands (mirroring src-tauri/src/commands/connectivity.rs)
// ---------------------------------------------------------------------------

// TestModelConnection mirrors `test_model_connection` (ping mode).
func TestModelConnection(modelID string) (ModelTestResult, error) {
	return connectivityTestSpecificModel(modelID, connectivityModePing, nil)
}

// TestModelConnectionWithMode mirrors `test_model_connection_with_mode`.
func TestModelConnectionWithMode(modelID string, mode string, prompt *string) (ModelTestResult, error) {
	return connectivityTestSpecificModel(modelID, mode, prompt)
}

// TestAllModelConnectionsCommand mirrors `test_all_model_connections_command`
// (ping mode over every BYOK custom model).
func TestAllModelConnectionsCommand() ([]ModelTestResult, error) {
	return connectivityTestAllModels(connectivityModePing, nil)
}

// TestAllModelConnectionsWithMode mirrors `test_all_model_connections_with_mode`.
func TestAllModelConnectionsWithMode(mode string, prompt *string) ([]ModelTestResult, error) {
	return connectivityTestAllModels(mode, prompt)
}

// TestProviderConnection mirrors `test_provider_connection`: a direct ping
// with caller-supplied credentials that never errors — failures surface in
// diagnostics.error (Rust returns Ok(test_model_direct(...))).
func TestProviderConnection(provider string, baseUrl string, apiKey string, modelID string) (ConnectionDiagnostics, error) {
	tester := connectivityNewTester()
	return tester.testModelDirect(provider, baseUrl, apiKey, modelID), nil
}

// connectivityTestSpecificModel mirrors test_specific_model_connection[_with_mode].
// The model is looked up by its explicit `id` field only, and a missing id
// yields `Model {model_id} not found`.
func connectivityTestSpecificModel(modelID, mode string, prompt *string) (ModelTestResult, error) {
	inference, err := connectivityModeIsInference(mode)
	if err != nil {
		return ModelTestResult{}, err
	}
	models, _ := factory.LoadCustomModels()
	var model *factory.CustomModel
	for index := range models {
		if models[index].ID != nil && *models[index].ID == modelID {
			model = &models[index]
			break
		}
	}
	if model == nil {
		return ModelTestResult{}, errString(fmt.Sprintf("Model %s not found", modelID))
	}

	tester := connectivityNewTester()
	display := connectivityCustomModelDisplay(*model)
	promptStr := connectivityPromptOrDefault(prompt)

	var diagnostics ConnectionDiagnostics
	if inference {
		diagnostics = tester.testModelInference(model.Provider, model.BaseURL, model.APIKey, model.Model, promptStr)
	} else {
		diagnostics = tester.testModel(model.Provider, model.BaseURL, model.APIKey, model.Model)
	}
	return ModelTestResult{
		ModelID:     modelID,
		ModelName:   display,
		Diagnostics: diagnostics,
		IsAvailable: diagnostics.Success,
	}, nil
}

// connectivityTestAllModels mirrors test_all_model_connections[_with_mode]:
// every configured model is tested sequentially; load failures yield an
// empty list (the Rust loader never errors).
func connectivityTestAllModels(mode string, prompt *string) ([]ModelTestResult, error) {
	inference, err := connectivityModeIsInference(mode)
	if err != nil {
		return []ModelTestResult{}, err
	}
	models, _ := factory.LoadCustomModels()
	tester := connectivityNewTester()
	promptStr := connectivityPromptOrDefault(prompt)

	results := make([]ModelTestResult, 0, len(models))
	for _, model := range models {
		var diagnostics ConnectionDiagnostics
		if inference {
			diagnostics = tester.testModelInference(model.Provider, model.BaseURL, model.APIKey, model.Model, promptStr)
		} else {
			diagnostics = tester.testModel(model.Provider, model.BaseURL, model.APIKey, model.Model)
		}
		modelID := model.Model
		if model.ID != nil {
			modelID = *model.ID
		}
		results = append(results, ModelTestResult{
			ModelID:     modelID,
			ModelName:   connectivityCustomModelDisplay(model),
			Diagnostics: diagnostics,
			IsAvailable: diagnostics.Success,
		})
	}
	return results, nil
}

// connectivitySummary mirrors connectivity::get_connectivity_summary: the
// average latency divides the success-latency sum by the available count.
func connectivitySummary(results []ModelTestResult) ConnectivitySummary {
	available := 0
	var latencySum uint64
	for _, result := range results {
		if result.IsAvailable {
			available++
		}
		if result.Diagnostics.Success {
			latencySum += uint64(result.Diagnostics.LatencyMS)
		}
	}
	average := 0.0
	if available > 0 {
		average = float64(latencySum) / float64(available)
	}
	return ConnectivitySummary{
		TotalModels:       uint32(len(results)),
		AvailableModels:   uint32(available),
		UnavailableModels: uint32(len(results) - available),
		AvgLatencyMS:      average,
		LastUpdated:       connectivityTimestamp(),
	}
}
