package app

import (
	"bytes"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/http/cookiejar"
	"strings"
	"time"
)

// ---------------------------------------------------------------------------
// Shared HTTP helpers
// ---------------------------------------------------------------------------

func newHTTPClient(timeout time.Duration) *http.Client {
	return &http.Client{Timeout: timeout}
}

func truncateBody(body string) string {
	if len(body) > 500 {
		return body[:500] + "..."
	}
	return body
}

type probeResult struct {
	success      bool
	modelID      string
	responseText *string
	error        *string
}

// probeModelEndpoint performs the lightweight `/v1/models` ping used by the
// connectivity tester (mirrors connectivity::probe_endpoint).
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

	client := newHTTPClient(timeout)
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
	message := fmt.Sprintf("HTTP %d: %s", response.StatusCode, truncateBody(string(body)))
	return probeResult{success: false, modelID: modelID, error: &message}
}

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
func (a *App) FetchDshModels(baseURL string, apiKey string, api *string) ([]DshModel, error) {
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

	client := newHTTPClient(30 * time.Second)
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
		return nil, fmt.Errorf("API error %d: %s", response.StatusCode, truncateBody(string(body)))
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

func errString(message string) error { return fmt.Errorf("%s", message) }

// ---------------------------------------------------------------------------
// FetchModelsByApiKey (channel-style model discovery)
// ---------------------------------------------------------------------------

// ModelInfo mirrors factory_settings::ModelInfo.
type ModelInfo struct {
	ID   string  `json:"id"`
	Name *string `json:"name"`
}

// modelEndpointUrls mirrors channel::model_endpoint_urls.
func modelEndpointUrls(baseURL string, platform *string) []string {
	base := strings.TrimRight(strings.TrimSpace(baseURL), "/")
	normalized := base
	if strings.HasSuffix(normalized, "/v1") || strings.HasSuffix(normalized, "/v1beta") {
		normalized = strings.TrimSuffix(strings.TrimSuffix(normalized, "/v1"), "/v1beta")
	}
	switch {
	case platform != nil && *platform == "gemini":
		return []string{normalized + "/v1beta/models"}
	case platform != nil && *platform == "deepseek":
		return []string{normalized + "/v1/models", normalized + "/models"}
	default:
		return []string{normalized + "/v1/models"}
	}
}

func parseOpenAIModels(data map[string]any) []ModelInfo {
	var models []ModelInfo
	items, _ := data["data"].([]any)
	for _, item := range items {
		entry, _ := item.(map[string]any)
		id, ok := entry["id"].(string)
		if !ok || id == "" {
			continue
		}
		info := ModelInfo{ID: id}
		if name, ok := entry["display_name"].(string); ok && name != "" {
			info.Name = &name
		} else if name, ok := entry["name"].(string); ok && name != "" {
			info.Name = &name
		}
		models = append(models, info)
	}
	return models
}

func parseGeminiModels(data map[string]any) []ModelInfo {
	var models []ModelInfo
	items, _ := data["models"].([]any)
	for _, item := range items {
		entry, _ := item.(map[string]any)
		raw, ok := entry["name"].(string)
		if !ok || raw == "" {
			continue
		}
		info := ModelInfo{ID: strings.TrimPrefix(raw, "models/")}
		if name, ok := entry["displayName"].(string); ok && name != "" {
			info.Name = &name
		}
		models = append(models, info)
	}
	return models
}

// FetchModelsByApiKey probes the platform-specific model endpoints and
// returns the parsed model list.
func (a *App) FetchModelsByApiKey(baseUrl string, apiKey string, platform *string) ([]ModelInfo, error) {
	key := strings.TrimSpace(apiKey)
	if key == "" {
		return nil, errString("API key is required to fetch models")
	}
	client := newHTTPClient(15 * time.Second)
	urls := modelEndpointUrls(baseUrl, platform)
	var lastError string
	for _, url := range urls {
		request, err := http.NewRequest(http.MethodGet, url, nil) // #nosec G107 -- provider URL from user config
		if err != nil {
			lastError = fmt.Sprintf("Failed to build request: %v", err)
			continue
		}
		request.Header.Set("Authorization", "Bearer "+key)
		response, err := client.Do(request)
		if err != nil {
			lastError = fmt.Sprintf("Failed to fetch models: %v", err)
			continue
		}
		body, readErr := io.ReadAll(io.LimitReader(response.Body, 8<<20))
		_ = response.Body.Close()
		if readErr != nil {
			lastError = fmt.Sprintf("Failed to read response: %v", readErr)
			continue
		}
		if response.StatusCode < 200 || response.StatusCode >= 300 {
			lastError = fmt.Sprintf("API error %d: %s", response.StatusCode, truncateBody(string(body)))
			continue
		}
		var data map[string]any
		if err := json.Unmarshal(body, &data); err != nil {
			lastError = fmt.Sprintf("Failed to parse response: %v", err)
			continue
		}
		var models []ModelInfo
		if platform != nil && *platform == "gemini" {
			models = parseGeminiModels(data)
		} else {
			models = parseOpenAIModels(data)
		}
		if len(models) > 0 {
			return models, nil
		}
		lastError = "Provider returned no models"
	}
	if lastError == "" {
		lastError = "Unable to fetch models"
	}
	return nil, errString(lastError)
}

// ---------------------------------------------------------------------------
// Channel type detection
// ---------------------------------------------------------------------------

// DetectChannelType probes well-known endpoints to classify a channel
// (mirrors channel::detect_channel_type).
func (a *App) DetectChannelType(baseUrl string) (string, error) {
	base := strings.TrimRight(strings.TrimSpace(baseUrl), "/")
	client := newHTTPClient(10 * time.Second)

	get := func(url string) (int, []byte) {
		request, err := http.NewRequest(http.MethodGet, url, nil) // #nosec G107 -- user-provided URL
		if err != nil {
			return 0, nil
		}
		response, err := client.Do(request)
		if err != nil {
			return 0, nil
		}
		defer func() { _ = response.Body.Close() }()
		body, _ := io.ReadAll(io.LimitReader(response.Body, 1<<20))
		return response.StatusCode, body
	}

	// 1. Ollama
	if status, body := get(base); status >= 200 && status < 300 {
		text := string(body)
		if strings.Contains(text, "Ollama") || strings.Contains(text, "ollama") {
			return "ollama", nil
		}
	}

	// 2. Sub2API
	if status, body := get(base + "/health"); status >= 200 && status < 300 {
		var data map[string]any
		if json.Unmarshal(body, &data) == nil {
			if value, _ := data["status"].(string); value == "ok" {
				return "sub-2-api", nil
			}
		}
	}

	// 3. New API
	if status, _ := get(base + "/api/status"); status >= 200 && status < 300 {
		return "new-api", nil
	}

	// 3.5. DeepSeek-style /models with owned_by: deepseek
	if status, body := get(base + "/models"); status >= 200 && status < 300 {
		var data map[string]any
		if json.Unmarshal(body, &data) == nil {
			if items, _ := data["data"].([]any); len(items) > 0 {
				for _, item := range items {
					entry, _ := item.(map[string]any)
					if _, hasID := entry["id"]; !hasID {
						continue
					}
					if ownedBy, _ := entry["owned_by"].(string); ownedBy == "deepseek" {
						return "deep-seek", nil
					}
				}
			}
		}
	}

	// 4. CLI Proxy API style /v1/models
	if status, body := get(base + "/v1/models"); status >= 200 && status < 300 {
		var data map[string]any
		if json.Unmarshal(body, &data) == nil {
			if items, _ := data["data"].([]any); len(items) > 0 {
				for _, item := range items {
					if entry, _ := item.(map[string]any); entry != nil {
						if _, hasID := entry["id"]; hasID {
							return "cli-proxy-api", nil
						}
					}
				}
			}
		}
	}

	return "", errString("Unable to auto-detect channel type")
}

// ---------------------------------------------------------------------------
// Channel token fetching (New API session flow)
// ---------------------------------------------------------------------------

// ChannelToken mirrors channel::ChannelToken.
type ChannelToken struct {
	ID             float64 `json:"id"`
	Name           string  `json:"name"`
	Key            string  `json:"key"`
	Status         int32   `json:"status"`
	RemainQuota    float64 `json:"remainQuota"`
	UsedQuota      float64 `json:"usedQuota"`
	UnlimitedQuota bool    `json:"unlimitedQuota"`
	Platform       *string `json:"platform"`
	GroupName      *string `json:"groupName"`
}

// FetchChannelTokens logs in to a New API compatible channel and lists the
// account's tokens. Other channel types return a single synthetic token that
// carries the password/API key (mirroring the Rust core).
func (a *App) FetchChannelTokens(channelType string, baseUrl string, username string, password string) ([]ChannelToken, error) {
	base := strings.TrimRight(strings.TrimSpace(baseUrl), "/")
	switch channelType {
	case "new-api":
		return fetchNewAPITokens(base, username, password)
	case "sub-2-api":
		return fetchSub2APITokens(base, username, password)
	default:
		return []ChannelToken{{
			ID:             0,
			Name:           "API Key",
			Key:            password,
			Status:         1,
			UnlimitedQuota: true,
		}}, nil
	}
}

// fetchSub2APITokens logs in to a Sub2API gateway (email/password), lists
// the account's API keys with pagination and merges per-key usage. Mirrors
// channel::fetch_sub2api_tokens in the Rust core, including error strings.
func fetchSub2APITokens(base, email, password string) ([]ChannelToken, error) {
	client := &http.Client{Timeout: 15 * time.Second}

	// Login (v1 API).
	loginBody, _ := json.Marshal(map[string]string{"email": email, "password": password})
	loginResponse, err := client.Post(base+"/api/v1/auth/login", "application/json", bytes.NewReader(loginBody)) // #nosec G107 -- user-provided URL
	if err != nil {
		return nil, fmt.Errorf("Failed to login: %w", err)
	}
	defer func() { _ = loginResponse.Body.Close() }()
	loginData, err := io.ReadAll(io.LimitReader(loginResponse.Body, 1<<20))
	if err != nil {
		return nil, fmt.Errorf("Failed to read login response body: %w", err)
	}
	if loginResponse.StatusCode < 200 || loginResponse.StatusCode >= 300 {
		return nil, errString(fmt.Sprintf("Login failed %d %s: %s", loginResponse.StatusCode, http.StatusText(loginResponse.StatusCode), string(loginData)))
	}
	var loginResult map[string]any
	if err := json.Unmarshal(loginData, &loginResult); err != nil {
		return nil, fmt.Errorf("Failed to parse login response: %w", err)
	}
	code, hasCode := loginResult["code"].(float64)
	if !hasCode || int64(code) != 0 {
		message, _ := loginResult["message"].(string)
		if message == "" {
			message = "Unknown error"
		}
		return nil, errString("Login failed: " + message)
	}
	data, _ := loginResult["data"].(map[string]any)
	accessToken, _ := data["access_token"].(string)
	if accessToken == "" {
		return nil, errString("Could not get access_token from login response")
	}
	authHeader := "Bearer " + accessToken

	// Group info is optional; failures degrade to (nil, nil) lookups.
	groupInfo := map[int64][2]string{}
	groupsRequest, _ := http.NewRequest(http.MethodGet, base+"/api/v1/groups/available", nil) // #nosec G107 -- user-provided URL
	groupsRequest.Header.Set("Authorization", authHeader)
	if groupsResponse, err := client.Do(groupsRequest); err == nil {
		if groupsResponse.StatusCode >= 200 && groupsResponse.StatusCode < 300 {
			if body, err := io.ReadAll(io.LimitReader(groupsResponse.Body, 4<<20)); err == nil {
				var groupsData map[string]any
				if json.Unmarshal(body, &groupsData) == nil {
					items, _ := groupsData["data"].([]any)
					for _, item := range items {
						group, _ := item.(map[string]any)
						id, okID := group["id"].(float64)
						platform, okPlatform := group["platform"].(string)
						name, okName := group["name"].(string)
						if okID && okPlatform && okName {
							groupInfo[int64(id)] = [2]string{platform, name}
						}
					}
				}
			}
		}
		_ = groupsResponse.Body.Close()
	}

	// Keys list (paginated).
	const pageSize = 100
	var allItems []any
	for page := 1; ; page++ {
		keysURL := fmt.Sprintf("%s/api/v1/keys?page=%d&page_size=%d", base, page, pageSize)
		keysRequest, _ := http.NewRequest(http.MethodGet, keysURL, nil) // #nosec G107 -- user-provided URL
		keysRequest.Header.Set("Authorization", authHeader)
		keysResponse, err := client.Do(keysRequest)
		if err != nil {
			return nil, fmt.Errorf("Failed to fetch keys: %w", err)
		}
		body, readErr := io.ReadAll(io.LimitReader(keysResponse.Body, 8<<20))
		_ = keysResponse.Body.Close()
		if readErr != nil {
			return nil, fmt.Errorf("Failed to read keys response body: %w", readErr)
		}
		if keysResponse.StatusCode < 200 || keysResponse.StatusCode >= 300 {
			return nil, errString(fmt.Sprintf("API error %d %s: %s", keysResponse.StatusCode, http.StatusText(keysResponse.StatusCode), string(body)))
		}
		var keysData map[string]any
		if err := json.Unmarshal(body, &keysData); err != nil {
			return nil, fmt.Errorf("Failed to parse keys response: %w", err)
		}
		payload, _ := keysData["data"].(map[string]any)
		items, _ := payload["items"].([]any)
		allItems = append(allItems, items...)
		if len(items) < pageSize {
			break
		}
	}

	// Usage is optional.
	usageMap := map[int64]map[string]any{}
	usageRequest, _ := http.NewRequest(http.MethodGet, base+"/api/v1/keys/usage", nil) // #nosec G107 -- user-provided URL
	usageRequest.Header.Set("Authorization", authHeader)
	if usageResponse, err := client.Do(usageRequest); err == nil {
		if usageResponse.StatusCode >= 200 && usageResponse.StatusCode < 300 {
			if body, err := io.ReadAll(io.LimitReader(usageResponse.Body, 8<<20)); err == nil {
				var usageData map[string]any
				if json.Unmarshal(body, &usageData) == nil {
					entries, _ := usageData["data"].([]any)
					for _, entry := range entries {
						usage, _ := entry.(map[string]any)
						if keyID, ok := usage["key_id"].(float64); ok {
							usageMap[int64(keyID)] = usage
						}
					}
				}
			}
		}
		_ = usageResponse.Body.Close()
	}

	tokens := []ChannelToken{}
	for _, item := range allItems {
		key, _ := item.(map[string]any)
		id, okID := key["id"].(float64)
		name, okName := key["name"].(string)
		keyValue, okKey := key["key"].(string)
		if !okID || !okName || !okKey {
			continue
		}
		status := int32(1)
		if rawStatus, ok := key["status"].(float64); ok {
			status = int32(rawStatus)
		}
		var usedQuota float64
		if usage, ok := usageMap[int64(id)]; ok {
			if cost, ok := usage["total_actual_cost"].(float64); ok {
				usedQuota = cost
			}
		}
		var platform, groupName *string
		if group, ok := key["group"].(map[string]any); ok {
			if p, ok := group["platform"].(string); ok {
				platform = &p
			}
			if n, ok := group["name"].(string); ok {
				groupName = &n
			}
		} else if groupID, ok := key["group_id"].(float64); ok {
			if info, ok := groupInfo[int64(groupID)]; ok {
				platform = &info[0]
				groupName = &info[1]
			}
		}
		tokens = append(tokens, ChannelToken{
			ID:             id,
			Name:           name,
			Key:            keyValue,
			Status:         status,
			UnlimitedQuota: true,
			Platform:       platform,
			GroupName:      groupName,
			UsedQuota:      usedQuota,
		})
	}
	return tokens, nil
}

func fetchNewAPITokens(base, username, password string) ([]ChannelToken, error) {
	jar, err := cookiejar.New(nil)
	if err != nil {
		return nil, err
	}
	client := &http.Client{Timeout: 15 * time.Second, Jar: jar}

	loginBody, _ := json.Marshal(map[string]string{"username": username, "password": password})
	loginResponse, err := client.Post(base+"/api/user/login", "application/json", bytes.NewReader(loginBody)) // #nosec G107 -- user-provided URL
	if err != nil {
		return nil, fmt.Errorf("Failed to login: %w", err)
	}
	defer func() { _ = loginResponse.Body.Close() }()
	loginData, _ := io.ReadAll(io.LimitReader(loginResponse.Body, 1<<20))
	var loginResult map[string]any
	if json.Unmarshal(loginData, &loginResult) != nil {
		return nil, errString("Failed to parse login response")
	}
	if success, _ := loginResult["success"].(bool); !success {
		message, _ := loginResult["message"].(string)
		if message == "" {
			message = "Login failed"
		}
		return nil, errString(message)
	}

	tokenResponse, err := client.Get(base + "/api/token/self") // #nosec G107 -- user-provided URL
	if err != nil {
		return nil, fmt.Errorf("Failed to fetch tokens: %w", err)
	}
	defer func() { _ = tokenResponse.Body.Close() }()
	tokenData, _ := io.ReadAll(io.LimitReader(tokenResponse.Body, 4<<20))
	var result map[string]any
	if err := json.Unmarshal(tokenData, &result); err != nil {
		return nil, fmt.Errorf("Failed to parse token response: %w", err)
	}
	if success, _ := result["success"].(bool); !success {
		return nil, errString("Token listing failed")
	}
	items, _ := result["data"].([]any)
	tokens := []ChannelToken{}
	for _, item := range items {
		entry, _ := item.(map[string]any)
		token := ChannelToken{}
		if value, ok := entry["id"].(float64); ok {
			token.ID = value
		}
		token.Name, _ = entry["name"].(string)
		token.Key, _ = entry["key"].(string)
		if value, ok := entry["status"].(float64); ok {
			token.Status = int32(value)
		}
		token.RemainQuota, _ = entry["remain_quota"].(float64)
		token.UsedQuota, _ = entry["used_quota"].(float64)
		token.UnlimitedQuota, _ = entry["unlimited_quota"].(bool)
		tokens = append(tokens, token)
	}
	if len(tokens) == 0 {
		return nil, errString("No tokens found")
	}
	return tokens, nil
}
