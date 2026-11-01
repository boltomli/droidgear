package app

import (
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"strings"
	"time"
)

// ModelInfo is declared in httpfetch.go (same shape as the Tauri type).

// FetchModels lists the models a provider API exposes (mirrors
// factory_settings::fetch_models). Anthropic endpoints are probed with
// x-api-key first and fall back to a Bearer token; OpenAI and generic
// chat-completion endpoints use Bearer only.
//
// `provider` is the kebab-case Provider enum from the frontend
// ("anthropic" | "openai" | "generic-chat-completion-api").
func (a *App) FetchModels(provider string, baseUrl string, apiKey string) ([]ModelInfo, error) {
	client := &http.Client{Timeout: 30 * time.Second}
	switch provider {
	case "anthropic":
		return fetchModelsAnthropic(client, baseUrl, apiKey)
	case "openai", "generic-chat-completion-api":
		return fetchModelsOpenAI(client, baseUrl, apiKey)
	default:
		// Rust matches on the deserialized enum, so an unknown variant never
		// reaches fetch_models; serde rejects it at the command boundary with
		// an "unknown variant" error. Mirror that shape.
		return nil, errString(fmt.Sprintf(
			"unknown variant `%s`, expected one of `anthropic`, `openai`, `generic-chat-completion-api`",
			provider,
		))
	}
}

func fetchModelsAnthropic(client *http.Client, baseUrl, apiKey string) ([]ModelInfo, error) {
	base := strings.TrimRight(strings.TrimSuffix(strings.TrimRight(baseUrl, "/"), "/v1"), "/")
	url := base + "/v1/models" // #nosec G107 -- user-provided URL

	request, _ := http.NewRequest(http.MethodGet, url, nil) // #nosec G107 -- user-provided URL
	request.Header.Set("x-api-key", apiKey)
	request.Header.Set("anthropic-version", "2023-06-01")
	response, err := client.Do(request)
	if err != nil {
		return nil, fmt.Errorf("Request failed: %w", err)
	}
	if response.StatusCode < 200 || response.StatusCode >= 300 {
		// Retry once with Bearer auth (mirrors the Rust fallback).
		_ = response.Body.Close()
		bearer, _ := http.NewRequest(http.MethodGet, url, nil) // #nosec G107 -- user-provided URL
		bearer.Header.Set("Authorization", "Bearer "+apiKey)
		response, err = client.Do(bearer)
		if err != nil {
			return nil, fmt.Errorf("Request failed: %w", err)
		}
	}
	defer func() { _ = response.Body.Close() }()

	if response.StatusCode < 200 || response.StatusCode >= 300 {
		body, _ := io.ReadAll(io.LimitReader(response.Body, 1<<20))
		return nil, errString(fmt.Sprintf("API error %d %s: %s", response.StatusCode, http.StatusText(response.StatusCode), string(body)))
	}

	body, err := io.ReadAll(io.LimitReader(response.Body, 8<<20))
	if err != nil {
		return nil, fmt.Errorf("Failed to read response body: %w", err)
	}
	var data struct {
		Data []struct {
			ID          string  `json:"id"`
			DisplayName *string `json:"display_name"`
		} `json:"data"`
	}
	if err := json.Unmarshal(body, &data); err != nil {
		return nil, fmt.Errorf("Failed to parse response: %w", err)
	}
	models := make([]ModelInfo, 0, len(data.Data))
	for _, m := range data.Data {
		if m.ID == "" {
			continue
		}
		models = append(models, ModelInfo{ID: m.ID, Name: m.DisplayName})
	}
	return models, nil
}

func fetchModelsOpenAI(client *http.Client, baseUrl, apiKey string) ([]ModelInfo, error) {
	base := strings.TrimRight(strings.TrimSuffix(strings.TrimRight(baseUrl, "/"), "/v1"), "/")
	url := base + "/v1/models" // #nosec G107 -- user-provided URL

	request, _ := http.NewRequest(http.MethodGet, url, nil) // #nosec G107 -- user-provided URL
	request.Header.Set("Authorization", "Bearer "+apiKey)
	response, err := client.Do(request)
	if err != nil {
		return nil, fmt.Errorf("Request failed: %w", err)
	}
	defer func() { _ = response.Body.Close() }()

	if response.StatusCode < 200 || response.StatusCode >= 300 {
		body, _ := io.ReadAll(io.LimitReader(response.Body, 1<<20))
		return nil, errString(fmt.Sprintf("API error %d %s: %s", response.StatusCode, http.StatusText(response.StatusCode), string(body)))
	}

	body, err := io.ReadAll(io.LimitReader(response.Body, 8<<20))
	if err != nil {
		return nil, fmt.Errorf("Failed to read response body: %w", err)
	}
	var data struct {
		Data []struct {
			ID string `json:"id"`
		} `json:"data"`
	}
	if err := json.Unmarshal(body, &data); err != nil {
		return nil, fmt.Errorf("Failed to parse response: %w", err)
	}
	models := make([]ModelInfo, 0, len(data.Data))
	for _, m := range data.Data {
		if m.ID == "" {
			continue
		}
		models = append(models, ModelInfo{ID: m.ID})
	}
	return models, nil
}
