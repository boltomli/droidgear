package dsh

import (
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"strings"
	"time"

	"droidgear-wails/internal/core"
)

// Model list discovery for Dsh providers (GET {baseURL}/models with
// registry enrichment) — moved from internal/app/httpfetch.go so the type it
// returns (DshModel) and its enricher stay in one package.

// ---------------------------------------------------------------------------
// Model list parsers
// ---------------------------------------------------------------------------

func parseOpenAIModelIDs(data map[string]any) []string {
	var ids []string
	items, _ := data["data"].([]any)
	for _, item := range items {
		entry, _ := item.(map[string]any)
		if id, ok := entry["id"].(string); ok && id != "" {
			ids = append(ids, id)
		}
	}
	return ids
}

func parseGeminiModelIDs(data map[string]any) []string {
	var ids []string
	items, _ := data["models"].([]any)
	for _, item := range items {
		entry, _ := item.(map[string]any)
		name, ok := entry["name"].(string)
		if !ok || name == "" {
			continue
		}
		ids = append(ids, strings.TrimPrefix(name, "models/"))
	}
	return ids
}

// ---------------------------------------------------------------------------
// FetchDshModels (GET {baseURL}/models with registry enrichment)
// ---------------------------------------------------------------------------

// FetchDshModels fetches the model list from a provider's models endpoint
// and enriches each id with registry metadata (name, contextWindow,
// maxTokens, reasoningEfforts, input).
func FetchModels(baseURL string, apiKey string, api *string) ([]DshModel, error) {
	trimmedBase := strings.TrimRight(strings.TrimSpace(baseURL), "/")
	if trimmedBase == "" {
		return nil, errString("Base URL is required to fetch models")
	}
	key := strings.TrimSpace(apiKey)
	if key == "" {
		return nil, errString("API key is required to fetch models")
	}

	protocol := "openai-completions"
	if api != nil && strings.TrimSpace(*api) != "" {
		protocol = strings.TrimSpace(*api)
	}

	url := trimmedBase + "/models"
	headersBearer := true
	switch protocol {
	case "anthropic-messages":
		headersBearer = false
	case "google-generative-ai":
		url = trimmedBase + "/models?key=" + key
	}

	request, err := http.NewRequest(http.MethodGet, url, nil) // #nosec G107 -- provider URL from user config
	if err != nil {
		return nil, fmt.Errorf("Failed to build request: %w", err)
	}
	if headersBearer {
		request.Header.Set("Authorization", "Bearer "+key)
	} else if protocol == "anthropic-messages" {
		request.Header.Set("x-api-key", key)
		request.Header.Set("anthropic-version", "2023-06-01")
	}

	client := core.NewHTTPClient(30 * time.Second)
	response, err := client.Do(request)
	if err != nil {
		return nil, fmt.Errorf("Failed to fetch models from %s: %w", url, err)
	}
	defer func() { _ = response.Body.Close() }()

	body, err := io.ReadAll(io.LimitReader(response.Body, 8<<20))
	if err != nil {
		return nil, fmt.Errorf("Failed to read response body: %w", err)
	}
	if response.StatusCode < 200 || response.StatusCode >= 300 {
		return nil, fmt.Errorf("API error %d: %s", response.StatusCode, core.TruncateBody(string(body)))
	}

	var data map[string]any
	if err := json.Unmarshal(body, &data); err != nil {
		return nil, fmt.Errorf("Failed to parse models response: %w", err)
	}

	var ids []string
	if protocol == "google-generative-ai" {
		ids = parseGeminiModelIDs(data)
	} else {
		ids = parseOpenAIModelIDs(data)
	}
	if len(ids) == 0 {
		return nil, errString("Provider returned no models")
	}

	models := make([]DshModel, 0, len(ids))
	for _, id := range ids {
		model := DshModel{ID: id}
		enrichDshModel(&model)
		models = append(models, model)
	}
	return models, nil
}
