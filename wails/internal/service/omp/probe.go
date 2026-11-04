package omp

import (
	"fmt"
	"io"
	"net/http"
	"strings"
	"time"

	"droidgear-wails/internal/core"
)

// probeResult carries the outcome of a single `/v1/models` ping.
type probeResult struct {
	success      bool
	modelID      string
	responseText *string
	error        *string
}

// probeModelEndpoint performs the lightweight `/v1/models` ping used by the
// OMP connection test (mirrors connectivity::probe_endpoint). It moved here
// with omp because omp was its only caller — leaving it in internal/app
// would have made the service depend on the GUI layer (design doc §2 R1).
func probeModelEndpoint(provider, baseURL, apiKey, modelID string, timeout time.Duration) probeResult {
	if strings.TrimSpace(apiKey) == "" {
		message := "No API key configured"
		return probeResult{success: false, modelID: modelID, error: &message}
	}

	base := strings.TrimRight(strings.TrimSpace(baseURL), "/")
	url := base + "/v1/models"
	request, err := http.NewRequest(http.MethodGet, url, nil) // #nosec G107 -- provider URL from user config
	if err != nil {
		message := fmt.Sprintf("Failed to build request: %v", err)
		return probeResult{success: false, modelID: modelID, error: &message}
	}
	if strings.Contains(provider, "anthropic") {
		request.Header.Set("x-api-key", apiKey)
		request.Header.Set("anthropic-version", "2023-06-01")
	} else {
		request.Header.Set("Authorization", "Bearer "+apiKey)
	}

	client := core.NewHTTPClient(timeout)
	response, err := client.Do(request)
	if err != nil {
		message := fmt.Sprintf("Network error: %v", err)
		return probeResult{success: false, modelID: modelID, error: &message}
	}
	defer func() { _ = response.Body.Close() }()

	if response.StatusCode >= 200 && response.StatusCode < 300 {
		return probeResult{success: true, modelID: modelID}
	}
	body, _ := io.ReadAll(io.LimitReader(response.Body, 4096))
	message := fmt.Sprintf("HTTP %d: %s", response.StatusCode, core.TruncateBody(string(body)))
	return probeResult{success: false, modelID: modelID, error: &message}
}
