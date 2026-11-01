package app

// Tests for the OpenClaw port (droidgear-core::openclaw). The round-trip and
// preservation cases mirror the Rust test-suite in
// src-tauri/crates/droidgear-core/src/openclaw.rs; every test runs against an
// isolated home directory and never touches the real profile.

import (
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

// ---------------------------------------------------------------------------
// Helpers
// ---------------------------------------------------------------------------

func openclawTestReadFile(t *testing.T, path string) string {
	t.Helper()
	data, err := os.ReadFile(path) // #nosec G304 -- test fixture
	if err != nil {
		t.Fatalf("read %s: %v", path, err)
	}
	return string(data)
}

// openclawTestWriteConfig drops a fixture openclaw.json into the isolated
// ~/.openclaw directory.
func openclawTestWriteConfig(t *testing.T, content string) string {
	t.Helper()
	dir, err := openclawConfigDir()
	if err != nil {
		t.Fatalf("config dir: %v", err)
	}
	path := filepath.Join(dir, "openclaw.json")
	if err := os.WriteFile(path, []byte(content), 0o644); err != nil { // #nosec G306 -- test fixture
		t.Fatalf("write config: %v", err)
	}
	return path
}

// openclawTestConfigMap parses the written config into plain Go maps for
// value assertions (key order is checked separately via the ordered tree).
func openclawTestConfigMap(t *testing.T) map[string]any {
	t.Helper()
	path, err := openclawConfigPath()
	if err != nil {
		t.Fatalf("config path: %v", err)
	}
	var out map[string]any
	if err := json.Unmarshal([]byte(openclawTestReadFile(t, path)), &out); err != nil {
		t.Fatalf("parse config: %v", err)
	}
	return out
}

// openclawTestConfigDoc parses the written config into the order-preserving
// tree so key order can be asserted.
func openclawTestConfigDoc(t *testing.T) *openclawObject {
	t.Helper()
	path, err := openclawConfigPath()
	if err != nil {
		t.Fatalf("config path: %v", err)
	}
	document, err := openclawParseDocument([]byte(openclawTestReadFile(t, path)))
	if err != nil {
		t.Fatalf("parse config: %v", err)
	}
	object, ok := document.(*openclawObject)
	if !ok {
		t.Fatalf("config root is not an object")
	}
	return object
}

func openclawTestChild(t *testing.T, root map[string]any, keys ...string) any {
	t.Helper()
	var current any = root
	for _, key := range keys {
		object, ok := current.(map[string]any)
		if !ok {
			t.Fatalf("key %q: parent is %T, not an object", key, current)
		}
		current, ok = object[key]
		if !ok {
			t.Fatalf("missing key %q in config", key)
		}
	}
	return current
}

func openclawTestObject(t *testing.T, value any) map[string]any {
	t.Helper()
	object, ok := value.(map[string]any)
	if !ok {
		t.Fatalf("expected object, got %T (%v)", value, value)
	}
	return object
}

func openclawTestArray(t *testing.T, value any) []any {
	t.Helper()
	array, ok := value.([]any)
	if !ok {
		t.Fatalf("expected array, got %T (%v)", value, value)
	}
	return array
}

func openclawTestString(t *testing.T, value any) string {
	t.Helper()
	text, ok := value.(string)
	if !ok {
		t.Fatalf("expected string, got %T (%v)", value, value)
	}
	return text
}

func openclawTestBool(t *testing.T, value any) bool {
	t.Helper()
	flag, ok := value.(bool)
	if !ok {
		t.Fatalf("expected bool, got %T (%v)", value, value)
	}
	return flag
}

// openclawTestRoundTrip mirrors Rust's run_load_save_roundtrip: seed an
// existing config, create the default profile from it, apply it, return the
// rewritten config.
func openclawTestRoundTrip(t *testing.T, existing string) map[string]any {
	t.Helper()
	isolatedHome(t)
	a := &App{}
	openclawTestWriteConfig(t, existing)
	profile, err := a.CreateDefaultOpenclawProfile()
	if err != nil {
		t.Fatalf("CreateDefaultOpenclawProfile: %v", err)
	}
	if err := a.ApplyOpenclawProfile(profile); err != nil {
		t.Fatalf("ApplyOpenclawProfile: %v", err)
	}
	return openclawTestConfigMap(t)
}

func openclawTestProfile() OpenClawProfile {
	return OpenClawProfile{
		ID:        "test-id",
		Name:      "Test",
		CreatedAt: "2025-01-01T00:00:00Z",
		UpdatedAt: "2025-01-01T00:00:00Z",
		Providers: map[string]OpenClawProviderConfig{},
	}
}

// ---------------------------------------------------------------------------
// Build → parse round-trips (mirrors the Rust roundtrip tests)
// ---------------------------------------------------------------------------

func TestOpenclawRoundtripEmptyProfile(t *testing.T) {
	isolatedHome(t)
	config := newOpenclawObject()
	profile := openclawTestProfile()
	openclawApplyProfileInPlace(config, &profile)

	if len(config.keys) != 0 {
		t.Fatalf("empty profile materialized keys: %v", config.keys)
	}
	defaultModel, failovers, providers := openclawParseConfig(config)
	if defaultModel != nil {
		t.Fatalf("expected no default model, got %q", *defaultModel)
	}
	if failovers != nil {
		t.Fatalf("expected no failovers, got %v", *failovers)
	}
	if len(providers) != 0 {
		t.Fatalf("expected no providers, got %v", providers)
	}
}

func TestOpenclawRoundtripDefaultModelAndFailovers(t *testing.T) {
	isolatedHome(t)

	profile := openclawTestProfile()
	profile.DefaultModel = strPtr("anthropic/claude-opus-4-6")
	fallbacks := []string{"minimax/MiniMax-M2.7", "openai/gpt-5.4-mini"}
	profile.FailoverModels = &fallbacks

	config := newOpenclawObject()
	openclawApplyProfileInPlace(config, &profile)

	defaultModel, failovers, _ := openclawParseConfig(config)
	if defaultModel == nil || *defaultModel != "anthropic/claude-opus-4-6" {
		t.Fatalf("default model lost: %v", defaultModel)
	}
	if failovers == nil || len(*failovers) != 2 || (*failovers)[0] != "minimax/MiniMax-M2.7" {
		t.Fatalf("failovers lost: %v", failovers)
	}

	// An explicitly empty list clears the key (Rust: Some(vec![]) → remove).
	profile.FailoverModels = &[]string{}
	cleared := newOpenclawObject()
	openclawApplyProfileInPlace(cleared, &profile)
	agents := openclawTestObject(t, mustObject(t, cleared)["agents"])
	defaults := openclawTestObject(t, agents["defaults"])
	model := openclawTestObject(t, defaults["model"])
	if _, present := model["fallbacks"]; present {
		t.Fatalf("empty failovers should not emit fallbacks: %v", model)
	}
	// Parsed back: failovers are absent → nil.
	_, parsedFailovers, _ := openclawParseConfig(cleared)
	if parsedFailovers != nil {
		t.Fatalf("empty failovers should parse back as nil, got %v", *parsedFailovers)
	}
}

// mustObject narrows an *openclawObject for the map-based assertion helpers.
func mustObject(t *testing.T, value any) map[string]any {
	t.Helper()
	object, ok := value.(*openclawObject)
	if !ok {
		t.Fatalf("expected openclawObject, got %T", value)
	}
	out := map[string]any{}
	for _, key := range object.keys {
		out[key] = openclawTestToPlain(object.values[key])
	}
	return out
}

func openclawTestToPlain(value any) any {
	switch typed := value.(type) {
	case *openclawObject:
		out := map[string]any{}
		for _, key := range typed.keys {
			out[key] = openclawTestToPlain(typed.values[key])
		}
		return out
	case []any:
		out := make([]any, 0, len(typed))
		for _, item := range typed {
			out = append(out, openclawTestToPlain(item))
		}
		return out
	default:
		return typed
	}
}

func TestOpenclawRoundtripProvidersWithModels(t *testing.T) {
	isolatedHome(t)
	profile := openclawTestProfile()
	profile.DefaultModel = strPtr("custom/my-model")
	profile.Providers = map[string]OpenClawProviderConfig{
		"custom": {
			BaseURL: strPtr("http://localhost:4000/v1"),
			APIKey:  strPtr("sk-test-key"),
			API:     strPtr("openai-completions"),
			Models: []OpenClawModel{
				{
					ID:            "my-model",
					Name:          strPtr("My Model"),
					Input:         []string{"text"},
					ContextWindow: uint32Ptr(128000),
					MaxTokens:     uint32Ptr(32000),
				},
				{
					ID:            "vision-model",
					Name:          strPtr("Vision Model"),
					Reasoning:     true,
					Input:         []string{"text", "image"},
					ContextWindow: uint32Ptr(200000),
					MaxTokens:     uint32Ptr(4096),
				},
			},
		},
	}

	config := newOpenclawObject()
	openclawApplyProfileInPlace(config, &profile)

	defaultModel, _, providers := openclawParseConfig(config)
	if defaultModel == nil || *defaultModel != "custom/my-model" {
		t.Fatalf("default model lost: %v", defaultModel)
	}
	custom, ok := providers["custom"]
	if !ok {
		t.Fatalf("custom provider missing: %v", providers)
	}
	if custom.BaseURL == nil || *custom.BaseURL != "http://localhost:4000/v1" {
		t.Fatalf("baseUrl lost: %v", custom.BaseURL)
	}
	if custom.APIKey == nil || *custom.APIKey != "sk-test-key" {
		t.Fatalf("apiKey lost: %v", custom.APIKey)
	}
	if custom.API == nil || *custom.API != "openai-completions" {
		t.Fatalf("api lost: %v", custom.API)
	}
	if len(custom.Models) != 2 {
		t.Fatalf("expected 2 models, got %d", len(custom.Models))
	}
	second := custom.Models[1]
	if !second.Reasoning || second.ContextWindow == nil || *second.ContextWindow != 200000 {
		t.Fatalf("second model wrong: %+v", second)
	}
	if len(second.Input) != 2 || second.Input[1] != "image" {
		t.Fatalf("input lost: %v", second.Input)
	}

	// models.refs are written into agents.defaults.models.
	refs := openclawTestObject(t, openclawTestChild(t, mustObject(t, config), "agents", "defaults", "models"))
	if _, ok := refs["custom/my-model"]; !ok {
		t.Fatalf("ref custom/my-model missing: %v", refs)
	}
}

func uint32Ptr(value uint32) *uint32 {
	return &value
}

// ---------------------------------------------------------------------------
// Parsing of the documented config format
// ---------------------------------------------------------------------------

func TestOpenclawParseDocumentedConfig(t *testing.T) {
	document, err := openclawParseDocument([]byte(`{
		"agents": {"defaults": {"model": {"primary": "anthropic/claude-opus-4-6", "fallbacks": ["minimax/MiniMax-M2.7"]}}},
		"models": {"mode": "merge", "providers": {
			"custom-proxy": {
				"baseUrl": "http://localhost:4000/v1",
				"apiKey": "LITELLM_KEY",
				"api": "openai-completions",
				"models": [{"id": "llama-3.1-8b", "name": "Llama 3.1 8B", "reasoning": false, "input": ["text"], "contextWindow": 128000, "maxTokens": 32000}]
			},
			"bare": {},
			"no-models": {"baseUrl": "http://localhost:9999/v1"}
		}}
	}`))
	if err != nil {
		t.Fatalf("parse: %v", err)
	}
	defaultModel, failovers, providers := openclawParseConfig(document)
	if defaultModel == nil || *defaultModel != "anthropic/claude-opus-4-6" {
		t.Fatalf("default model wrong: %v", defaultModel)
	}
	if failovers == nil || len(*failovers) != 1 || (*failovers)[0] != "minimax/MiniMax-M2.7" {
		t.Fatalf("failovers wrong: %v", failovers)
	}
	proxy, ok := providers["custom-proxy"]
	if !ok || len(proxy.Models) != 1 {
		t.Fatalf("custom-proxy wrong: %+v", providers)
	}
	if proxy.Models[0].ContextWindow == nil || *proxy.Models[0].ContextWindow != 128000 {
		t.Fatalf("contextWindow wrong: %+v", proxy.Models[0])
	}
	if _, ok := providers["bare"]; !ok {
		t.Fatalf("bare provider missing")
	}
	if len(providers["no-models"].Models) != 0 {
		t.Fatalf("provider without models array should have none")
	}

	// A plain-string `model` is not the object form → not parsed.
	stringModel, err := openclawParseDocument([]byte(`{"agents": {"defaults": {"model": "anthropic/claude-sonnet-4-6"}}}`))
	if err != nil {
		t.Fatalf("parse: %v", err)
	}
	if dm, _, _ := openclawParseConfig(stringModel); dm != nil {
		t.Fatalf("plain string model should not be parsed, got %q", *dm)
	}

	// Empty fallbacks array → nil.
	emptyFallbacks, err := openclawParseDocument([]byte(`{"agents": {"defaults": {"model": {"primary": "m", "fallbacks": []}}}}`))
	if err != nil {
		t.Fatalf("parse: %v", err)
	}
	if _, fo, _ := openclawParseConfig(emptyFallbacks); fo != nil {
		t.Fatalf("empty fallbacks should parse as nil, got %v", *fo)
	}
}

// ---------------------------------------------------------------------------
// create default → apply → read (mirrors Rust default-profile tests)
// ---------------------------------------------------------------------------

func TestOpenclawDefaultProfileFreshInstall(t *testing.T) {
	isolatedHome(t)
	a := &App{}

	profile, err := a.CreateDefaultOpenclawProfile()
	if err != nil {
		t.Fatalf("CreateDefaultOpenclawProfile: %v", err)
	}
	if profile.DefaultModel == nil || *profile.DefaultModel != "anthropic/claude-sonnet-4-20250514" {
		t.Fatalf("fresh default model wrong: %v", profile.DefaultModel)
	}
	if profile.FailoverModels != nil {
		t.Fatalf("fresh profile should have no failovers: %v", *profile.FailoverModels)
	}
	if len(profile.Providers) != 0 {
		t.Fatalf("fresh profile should have no providers: %v", profile.Providers)
	}
	if profile.Name != "Default" || profile.Description == nil || *profile.Description != "Default OpenClaw profile" {
		t.Fatalf("fresh profile metadata wrong: %+v", profile)
	}

	if err := a.ApplyOpenclawProfile(profile); err != nil {
		t.Fatalf("ApplyOpenclawProfile: %v", err)
	}
	current, err := a.ReadOpenclawCurrentConfig()
	if err != nil {
		t.Fatalf("ReadOpenclawCurrentConfig: %v", err)
	}
	if current.DefaultModel == nil || *current.DefaultModel != "anthropic/claude-sonnet-4-20250514" {
		t.Fatalf("current default model wrong: %v", current.DefaultModel)
	}
	if current.Providers == nil || len(current.Providers) != 0 {
		t.Fatalf("current providers should be an empty map: %v", current.Providers)
	}

	config := openclawTestConfigMap(t)
	model := openclawTestObject(t, openclawTestChild(t, config, "agents", "defaults", "model"))
	if model["primary"] != "anthropic/claude-sonnet-4-20250514" {
		t.Fatalf("primary wrong: %v", model["primary"])
	}
	if _, present := model["fallbacks"]; present {
		t.Fatalf("no failovers → no fallbacks key: %v", model)
	}
}

func TestOpenclawDefaultProfileParsesExistingConfig(t *testing.T) {
	isolatedHome(t)
	a := &App{}
	openclawTestWriteConfig(t, `{
		"agents": {"defaults": {"model": {"primary": "openai/gpt-5.5", "fallbacks": ["anthropic/claude-opus-4-6"]}}},
		"models": {"mode": "merge", "providers": {
			"openai": {"baseUrl": "https://api.openai.com/v1", "api": "openai-completions",
				"models": [{"id": "gpt-5.5", "name": "GPT 5.5", "input": ["text"], "contextWindow": 1000000, "maxTokens": 32000}]},
			"anthropic": {"baseUrl": "https://api.anthropic.com/v1", "api": "anthropic-messages",
				"models": [{"id": "claude-opus-4-6", "name": "Claude Opus 4", "reasoning": true, "input": ["text", "image"], "contextWindow": 200000, "maxTokens": 4096}]}
		}}
	}`)

	profile, err := a.CreateDefaultOpenclawProfile()
	if err != nil {
		t.Fatalf("CreateDefaultOpenclawProfile: %v", err)
	}
	if profile.DefaultModel == nil || *profile.DefaultModel != "openai/gpt-5.5" {
		t.Fatalf("default model not read from config: %v", profile.DefaultModel)
	}
	if profile.FailoverModels == nil || len(*profile.FailoverModels) != 1 || (*profile.FailoverModels)[0] != "anthropic/claude-opus-4-6" {
		t.Fatalf("failovers not read from config: %v", profile.FailoverModels)
	}
	openai, ok := profile.Providers["openai"]
	if !ok || openai.BaseURL == nil || *openai.BaseURL != "https://api.openai.com/v1" {
		t.Fatalf("openai provider not parsed: %+v", profile.Providers)
	}
	if len(openai.Models) != 1 || openai.Models[0].ContextWindow == nil || *openai.Models[0].ContextWindow != 1000000 {
		t.Fatalf("openai models not parsed: %+v", openai.Models)
	}
	anthropic, ok := profile.Providers["anthropic"]
	if !ok || len(anthropic.Models) != 1 || !anthropic.Models[0].Reasoning {
		t.Fatalf("anthropic provider not parsed: %+v", profile.Providers)
	}
	if got := anthropic.Models[0].Input; len(got) != 2 || got[1] != "image" {
		t.Fatalf("anthropic input not parsed: %v", got)
	}
}

// ---------------------------------------------------------------------------
// Load → save preservation (mirrors the Rust preservation tests)
// ---------------------------------------------------------------------------

const openclawPreserveFixture = `{
	"agents": {
		"defaults": {
			"model": {"primary": "wududu/m1", "fallbacks": ["wududu/m1"], "strategy": "weighted", "weights": {"wududu/m1": 1.0}, "futureKey": {"x": 42}},
			"workspace": "/home/u/.openclaw/workspace",
			"models": {
				"wududu/m1": {"alias": "fast", "params": {"thinking": "low"}, "streaming": true},
				"openai/gpt-5.5": {"alias": "smart"}
			},
			"blockStreamingChunk": {"minChars": 200, "maxChars": 600, "smoothingMode": "adaptive", "customTuning": {"weight": 0.5}}
		}
	},
	"channels": {
		"telegram": {"blockStreaming": true, "chunkMode": "message", "botToken": "xxx:yyy", "chatIds": ["-1001", "-1002"], "proxy": {"url": "socks5://localhost:1080"}},
		"feishu": {"webhookUrl": "https://example/hook?a=1&b=2"}
	},
	"models": {
		"mode": "merge",
		"providers": {
			"wududu": {
				"timeoutSeconds": 600,
				"baseUrl": "https://w.example/path?a=1&b=2",
				"headers": {"X-A": "1"},
				"apiKey": "k",
				"api": "openai-completions",
				"auth": "api-key",
				"params": {"temperature": 0.7},
				"models": [
					{"id": "m1", "name": "M1", "reasoning": true, "input": ["text"], "contextWindow": 200000, "contextTokens": 150000, "maxTokens": 8192, "params": {"thinking_budget": 8000}, "headers": {"X-Trace": "1"}, "compat": {"supportsTools": true}}
				]
			}
		}
	},
	"gateway": {"port": 18789, "mode": "local"},
	"bindings": [{"agentId": "main"}],
	"plugins": {"entries": {"feishu": {"enabled": true}}},
	"wizard": {"lastRunCommand": "onboard"},
	"tools": {"profile": "coding"},
	"session": {"dmScope": "per-channel-peer"},
	"meta": {"lastTouchedVersion": "2026.5.18"}
}`

func TestOpenclawApplyPreservesUnknownKeys(t *testing.T) {
	config := openclawTestRoundTrip(t, openclawPreserveFixture)

	// Unrelated top-level keys survive.
	for _, key := range []string{"gateway", "channels", "bindings", "plugins", "wizard", "tools", "session", "meta"} {
		if _, ok := config[key]; !ok {
			t.Fatalf("top-level key %q lost", key)
		}
	}
	if got := openclawTestChild(t, config, "agents", "defaults", "workspace"); got != "/home/u/.openclaw/workspace" {
		t.Fatalf("agents.defaults.workspace lost: %v", got)
	}

	// Provider-level fields DroidGear does not model survive.
	provider := openclawTestObject(t, openclawTestChild(t, config, "models", "providers", "wududu"))
	if provider["auth"] != "api-key" {
		t.Fatalf("provider.auth lost: %v", provider["auth"])
	}
	if provider["timeoutSeconds"] != float64(600) {
		t.Fatalf("provider.timeoutSeconds lost: %v", provider["timeoutSeconds"])
	}
	headers := openclawTestObject(t, provider["headers"])
	if headers["X-A"] != "1" {
		t.Fatalf("provider.headers lost: %v", headers)
	}
	params := openclawTestObject(t, provider["params"])
	if params["temperature"] != 0.7 {
		t.Fatalf("provider.params lost: %v", params["temperature"])
	}

	// Model-level unknown fields survive the surgical models[] rewrite.
	model := openclawTestObject(t, openclawTestArray(t, provider["models"])[0])
	if model["contextTokens"] != float64(150000) {
		t.Fatalf("model.contextTokens lost: %v", model["contextTokens"])
	}
	modelParams := openclawTestObject(t, model["params"])
	if modelParams["thinking_budget"] != float64(8000) {
		t.Fatalf("model.params lost: %v", modelParams)
	}
	compat := openclawTestObject(t, model["compat"])
	if compat["supportsTools"] != true {
		t.Fatalf("model.compat lost: %v", compat)
	}

	// agents.defaults.models entries: owned ref contents + unowned ref.
	refs := openclawTestObject(t, openclawTestChild(t, config, "agents", "defaults", "models"))
	ownedRef := openclawTestObject(t, refs["wududu/m1"])
	if ownedRef["alias"] != "fast" || ownedRef["streaming"] != true {
		t.Fatalf("owned ref contents lost: %v", ownedRef)
	}
	paramsRef := openclawTestObject(t, ownedRef["params"])
	if paramsRef["thinking"] != "low" {
		t.Fatalf("owned ref params lost: %v", paramsRef)
	}
	unownedRef := openclawTestObject(t, refs["openai/gpt-5.5"])
	if unownedRef["alias"] != "smart" {
		t.Fatalf("unowned ref lost: %v", unownedRef)
	}

	// Unknown siblings inside partially owned blocks survive.
	modelBlock := openclawTestObject(t, openclawTestChild(t, config, "agents", "defaults", "model"))
	if modelBlock["strategy"] != "weighted" {
		t.Fatalf("agents.defaults.model.strategy lost: %v", modelBlock["strategy"])
	}
	future := openclawTestObject(t, modelBlock["futureKey"])
	if future["x"] != float64(42) {
		t.Fatalf("agents.defaults.model.futureKey lost: %v", future)
	}
	chunk := openclawTestObject(t, openclawTestChild(t, config, "agents", "defaults", "blockStreamingChunk"))
	if chunk["smoothingMode"] != "adaptive" {
		t.Fatalf("blockStreamingChunk.smoothingMode lost: %v", chunk["smoothingMode"])
	}
	telegram := openclawTestObject(t, openclawTestChild(t, config, "channels", "telegram"))
	if telegram["botToken"] != "xxx:yyy" {
		t.Fatalf("telegram.botToken lost: %v", telegram["botToken"])
	}
	if got := openclawTestChild(t, config, "channels", "feishu", "webhookUrl"); got != "https://example/hook?a=1&b=2" {
		t.Fatalf("sibling channel lost: %v", got)
	}

	// No HTML escaping: '&' must be written byte-identically to serde_json.
	raw := openclawTestReadFile(t, mustConfigPath(t))
	if strings.Contains(raw, `\u0026`) {
		t.Fatalf("config HTML-escaped '&': %s", raw)
	}
}

// TestOpenclawApplyUpdatesOwnedKeysOnly checks the write side of the
// preservation contract: owned keys are rewritten, everything else is not.
func TestOpenclawApplyUpdatesOwnedKeysOnly(t *testing.T) {
	isolatedHome(t)
	a := &App{}
	openclawTestWriteConfig(t, openclawPreserveFixture)

	profile, err := a.CreateDefaultOpenclawProfile()
	if err != nil {
		t.Fatalf("CreateDefaultOpenclawProfile: %v", err)
	}
	// Change the owned values, then apply.
	profile.DefaultModel = strPtr("wududu/m2")
	profile.Providers["wududu"] = OpenClawProviderConfig{
		BaseURL: strPtr("https://changed.example"),
		APIKey:  strPtr("k2"),
		API:     strPtr("openai-completions"),
		Models:  []OpenClawModel{{ID: "m2", Name: strPtr("M2"), Input: []string{"text"}}},
	}
	if err := a.ApplyOpenclawProfile(profile); err != nil {
		t.Fatalf("ApplyOpenclawProfile: %v", err)
	}

	config := openclawTestConfigMap(t)
	if got := openclawTestChild(t, config, "agents", "defaults", "model", "primary"); got != "wududu/m2" {
		t.Fatalf("primary not updated: %v", got)
	}
	provider := openclawTestObject(t, openclawTestChild(t, config, "models", "providers", "wududu"))
	if provider["baseUrl"] != "https://changed.example" || provider["apiKey"] != "k2" {
		t.Fatalf("provider not updated: %v", provider)
	}
	if provider["timeoutSeconds"] != float64(600) {
		t.Fatalf("unowned provider key lost: %v", provider["timeoutSeconds"])
	}
	models := openclawTestArray(t, provider["models"])
	if len(models) != 1 || openclawTestObject(t, models[0])["id"] != "m2" {
		t.Fatalf("models[] not reconciled: %v", models)
	}
	// The dropped model's ref is pruned; the unowned ref stays.
	refs := openclawTestObject(t, openclawTestChild(t, config, "agents", "defaults", "models"))
	if _, ok := refs["wududu/m1"]; ok {
		t.Fatalf("stale owned ref not pruned: %v", refs)
	}
	if _, ok := refs["wududu/m2"]; !ok {
		t.Fatalf("new ref missing: %v", refs)
	}
	if _, ok := refs["openai/gpt-5.5"]; !ok {
		t.Fatalf("unowned ref pruned: %v", refs)
	}
}

// TestOpenclawApplyRemovesDroppedModels mirrors
// test_load_save_removes_models_for_models_dropped_from_profile: the provider
// stays, its models[] goes away.
func TestOpenclawApplyRemovesDroppedModels(t *testing.T) {
	isolatedHome(t)
	a := &App{}
	openclawTestWriteConfig(t, `{
		"agents": {"defaults": {"model": {"primary": "wududu/keep"}}},
		"models": {"providers": {"wududu": {"baseUrl": "https://w", "apiKey": "k", "api": "openai-completions",
			"models": [{"id": "keep", "name": "Keep", "reasoning": false, "input": ["text"], "contextWindow": 100000, "maxTokens": 4096}]}}}
	}`)

	profile, err := a.CreateDefaultOpenclawProfile()
	if err != nil {
		t.Fatalf("CreateDefaultOpenclawProfile: %v", err)
	}
	for id, provider := range profile.Providers {
		provider.Models = nil
		profile.Providers[id] = provider
	}
	if err := a.ApplyOpenclawProfile(profile); err != nil {
		t.Fatalf("ApplyOpenclawProfile: %v", err)
	}

	config := openclawTestConfigMap(t)
	provider := openclawTestObject(t, openclawTestChild(t, config, "models", "providers", "wududu"))
	if _, present := provider["models"]; present {
		t.Fatalf("models[] should be removed: %v", provider)
	}
	if provider["baseUrl"] != "https://w" {
		t.Fatalf("provider lost: %v", provider)
	}
}

// TestOpenclawApplyDoesNotInjectUnownedKeys mirrors
// test_load_save_does_not_inject_unowned_keys: an empty profile must not
// materialize `models` or `agents`.
func TestOpenclawApplyDoesNotInjectUnownedKeys(t *testing.T) {
	config := openclawTestRoundTrip(t, `{"meta": {"v": 1}, "gateway": {"port": 18789}}`)
	if _, ok := config["models"]; ok {
		t.Fatalf("empty profile materialized `models`: %v", config)
	}
	if _, ok := config["agents"]; ok {
		t.Fatalf("empty profile materialized `agents`: %v", config)
	}
	if _, ok := config["meta"]; !ok {
		t.Fatalf("meta lost: %v", config)
	}
	if _, ok := config["gateway"]; !ok {
		t.Fatalf("gateway lost: %v", config)
	}
}

// TestOpenclawApplyPreservesKeyOrder mirrors the two Rust key-order tests:
// serde_json's preserve_order keeps the document stable across a rewrite.
func TestOpenclawApplyPreservesKeyOrder(t *testing.T) {
	isolatedHome(t)
	a := &App{}
	openclawTestWriteConfig(t, `{
		"meta": {"v": 1},
		"agents": {"defaults": {"model": {"primary": "wududu/m1"}}},
		"gateway": {"port": 18789},
		"models": {"providers": {"wududu": {
			"timeoutSeconds": 600,
			"baseUrl": "https://w",
			"headers": {"X-A": "1"},
			"apiKey": "k",
			"api": "openai-completions",
			"models": [{"id": "m1", "name": "M1", "reasoning": false, "input": ["text"], "contextWindow": 100000, "maxTokens": 4096}]
		}}},
		"plugins": {"entries": {}}
	}`)

	profile, err := a.CreateDefaultOpenclawProfile()
	if err != nil {
		t.Fatalf("CreateDefaultOpenclawProfile: %v", err)
	}
	if err := a.ApplyOpenclawProfile(profile); err != nil {
		t.Fatalf("ApplyOpenclawProfile: %v", err)
	}

	document := openclawTestConfigDoc(t)
	wantTop := []string{"meta", "agents", "gateway", "models", "plugins"}
	if strings.Join(document.keys, ",") != strings.Join(wantTop, ",") {
		t.Fatalf("top-level key order changed: got %v want %v", document.keys, wantTop)
	}

	provider, ok := document.get("models").(*openclawObject)
	if !ok {
		t.Fatalf("models is not an object")
	}
	providers, ok := provider.get("providers").(*openclawObject)
	if !ok {
		t.Fatalf("models.providers is not an object")
	}
	wududu, ok := providers.get("wududu").(*openclawObject)
	if !ok {
		t.Fatalf("provider wududu missing")
	}
	wantProvider := []string{"timeoutSeconds", "baseUrl", "headers", "apiKey", "api", "models"}
	if strings.Join(wududu.keys, ",") != strings.Join(wantProvider, ",") {
		t.Fatalf("provider key order changed: got %v want %v", wududu.keys, wantProvider)
	}
}

// ---------------------------------------------------------------------------
// Status / current config
// ---------------------------------------------------------------------------

func TestOpenclawConfigStatusAndCurrentConfig(t *testing.T) {
	isolatedHome(t)
	a := &App{}

	status, err := a.GetOpenclawConfigStatus()
	if err != nil {
		t.Fatalf("GetOpenclawConfigStatus: %v", err)
	}
	if status.ConfigExists {
		t.Fatalf("config should not exist yet: %s", status.ConfigPath)
	}
	if !strings.HasSuffix(status.ConfigPath, "openclaw.json") {
		t.Fatalf("unexpected config path: %s", status.ConfigPath)
	}

	current, err := a.ReadOpenclawCurrentConfig()
	if err != nil {
		t.Fatalf("ReadOpenclawCurrentConfig: %v", err)
	}
	if current.DefaultModel != nil || current.Providers == nil || len(current.Providers) != 0 {
		t.Fatalf("missing config should yield empty current config: %+v", current)
	}

	openclawTestWriteConfig(t, `{"agents": {"defaults": {"model": {"primary": "openai/gpt-5.5"}}}}`)
	status, err = a.GetOpenclawConfigStatus()
	if err != nil {
		t.Fatalf("GetOpenclawConfigStatus: %v", err)
	}
	if !status.ConfigExists {
		t.Fatalf("config should exist: %s", status.ConfigPath)
	}
	current, err = a.ReadOpenclawCurrentConfig()
	if err != nil {
		t.Fatalf("ReadOpenclawCurrentConfig: %v", err)
	}
	if current.DefaultModel == nil || *current.DefaultModel != "openai/gpt-5.5" {
		t.Fatalf("default model not read: %v", current.DefaultModel)
	}

	// Invalid JSON is reported with the Rust wording.
	openclawTestWriteConfig(t, "{not json")
	if _, err := a.ReadOpenclawCurrentConfig(); err == nil || !strings.HasPrefix(err.Error(), "Invalid config JSON: ") {
		t.Fatalf("expected Invalid config JSON error, got %v", err)
	}
}

func mustConfigPath(t *testing.T) string {
	t.Helper()
	path, err := openclawConfigPath()
	if err != nil {
		t.Fatalf("config path: %v", err)
	}
	return path
}

// ---------------------------------------------------------------------------
// Profile CRUD
// ---------------------------------------------------------------------------

func TestOpenclawProfileCRUD(t *testing.T) {
	isolatedHome(t)
	a := &App{}

	// Create with an empty id → server-side uuid + timestamps.
	created := OpenClawProfile{Name: "Alpha", CreatedAt: nowRFC3339(), UpdatedAt: nowRFC3339()}
	if err := a.SaveOpenclawProfile(created); err != nil {
		t.Fatalf("SaveOpenclawProfile: %v", err)
	}
	profiles, err := a.ListOpenclawProfiles()
	if err != nil {
		t.Fatalf("ListOpenclawProfiles: %v", err)
	}
	if len(profiles) != 1 {
		t.Fatalf("expected 1 profile, got %d", len(profiles))
	}
	stored := profiles[0]
	if stored.ID == "" || stored.ID == " " {
		t.Fatalf("id not generated: %q", stored.ID)
	}
	if stored.Name != "Alpha" || stored.CreatedAt == "" || stored.UpdatedAt == "" {
		t.Fatalf("profile not stamped: %+v", stored)
	}
	if stored.Providers == nil {
		t.Fatalf("providers should serialize as {}")
	}

	// Read back through Get.
	fetched, err := a.GetOpenclawProfile(stored.ID)
	if err != nil {
		t.Fatalf("GetOpenclawProfile: %v", err)
	}
	if fetched.Name != "Alpha" {
		t.Fatalf("fetched profile wrong: %+v", fetched)
	}

	// Update keeps createdAt, refreshes updatedAt.
	updated := fetched
	updated.Name = "Alpha renamed"
	if err := a.SaveOpenclawProfile(updated); err != nil {
		t.Fatalf("SaveOpenclawProfile (update): %v", err)
	}
	after, err := a.GetOpenclawProfile(stored.ID)
	if err != nil {
		t.Fatalf("GetOpenclawProfile: %v", err)
	}
	if after.CreatedAt != stored.CreatedAt {
		t.Fatalf("createdAt changed on update: %s → %s", stored.CreatedAt, after.CreatedAt)
	}
	if after.Name != "Alpha renamed" {
		t.Fatalf("rename lost: %+v", after)
	}

	// Sorting by lowercase name.
	second := OpenClawProfile{Name: "beta"}
	if err := a.SaveOpenclawProfile(second); err != nil {
		t.Fatalf("SaveOpenclawProfile (beta): %v", err)
	}
	profiles, err = a.ListOpenclawProfiles()
	if err != nil {
		t.Fatalf("ListOpenclawProfiles: %v", err)
	}
	if len(profiles) != 2 || profiles[0].Name != "Alpha renamed" || profiles[1].Name != "beta" {
		t.Fatalf("profiles not sorted by lowercase name: %+v", profiles)
	}

	// Duplicate gets a fresh id and the requested name.
	duplicate, err := a.DuplicateOpenclawProfile(stored.ID, "Copy")
	if err != nil {
		t.Fatalf("DuplicateOpenclawProfile: %v", err)
	}
	if duplicate.ID == stored.ID || duplicate.Name != "Copy" {
		t.Fatalf("duplicate wrong: %+v", duplicate)
	}

	// Active pointer: apply → set, delete → cleared.
	if err := a.ApplyOpenclawProfile(after); err != nil {
		t.Fatalf("ApplyOpenclawProfile: %v", err)
	}
	active, err := a.GetActiveOpenclawProfileId()
	if err != nil {
		t.Fatalf("GetActiveOpenclawProfileId: %v", err)
	}
	if active == nil || *active != stored.ID {
		t.Fatalf("active id wrong: %v", active)
	}
	if err := a.DeleteOpenclawProfile(stored.ID); err != nil {
		t.Fatalf("DeleteOpenclawProfile: %v", err)
	}
	if _, err := a.GetOpenclawProfile(stored.ID); err == nil {
		t.Fatalf("deleted profile still readable")
	}
	active, err = a.GetActiveOpenclawProfileId()
	if err != nil {
		t.Fatalf("GetActiveOpenclawProfileId: %v", err)
	}
	if active != nil {
		t.Fatalf("active id should be cleared after delete: %v", *active)
	}
	// Deleting an unknown id is a no-op, not an error.
	if err := a.DeleteOpenclawProfile(stored.ID); err != nil {
		t.Fatalf("DeleteOpenclawProfile (missing): %v", err)
	}
}

func TestOpenclawInvalidProfileID(t *testing.T) {
	isolatedHome(t)
	a := &App{}

	if _, err := a.GetOpenclawProfile("../evil"); err == nil || err.Error() != "Invalid profile id" {
		t.Fatalf("expected Invalid profile id, got %v", err)
	}
	if err := a.SaveOpenclawProfile(OpenClawProfile{ID: "bad id", Name: "x"}); err == nil || err.Error() != "Invalid profile id" {
		t.Fatalf("expected Invalid profile id, got %v", err)
	}
	if err := a.DeleteOpenclawProfile(""); err == nil || err.Error() != "Invalid profile id" {
		t.Fatalf("expected Invalid profile id, got %v", err)
	}
}

// TestOpenclawCorruptProfileIsSkipped pins serde's required-field behaviour:
// a profile file without `name` fails to load (and is skipped by List)
// instead of coming back with an empty name.
func TestOpenclawCorruptProfileIsSkipped(t *testing.T) {
	isolatedHome(t)
	a := &App{}

	if err := a.SaveOpenclawProfile(OpenClawProfile{Name: "Good"}); err != nil {
		t.Fatalf("SaveOpenclawProfile: %v", err)
	}
	dir, err := openclawProfilesDir()
	if err != nil {
		t.Fatalf("profiles dir: %v", err)
	}
	if err := os.WriteFile(filepath.Join(dir, "broken.json"), []byte(`{"id": "broken"}`), 0o644); err != nil { // #nosec G306 -- test fixture
		t.Fatalf("write broken profile: %v", err)
	}

	profiles, err := a.ListOpenclawProfiles()
	if err != nil {
		t.Fatalf("ListOpenclawProfiles: %v", err)
	}
	if len(profiles) != 1 || profiles[0].Name != "Good" {
		t.Fatalf("corrupt profile not skipped: %+v", profiles)
	}
	if _, err := a.GetOpenclawProfile("broken"); err == nil || !strings.Contains(err.Error(), "missing field `name`") {
		t.Fatalf("expected missing field `name`, got %v", err)
	}
}

// ---------------------------------------------------------------------------
// Subagents
// ---------------------------------------------------------------------------

const openclawSubagentsFixture = `{
	"gateway": {"port": 18789},
	"agents": {
		"list": [
			{"id": "main", "subagents": {"allowAgents": ["coder"], "maxConcurrent": 3}, "extraField": true},
			{"id": "coder", "model": {"primary": "openai/gpt-5.5"}, "tools": {"profile": "coding"}},
			{"name": "entry without id"}
		]
	}
}`

func TestOpenclawReadSubagents(t *testing.T) {
	isolatedHome(t)
	a := &App{}
	openclawTestWriteConfig(t, openclawSubagentsFixture)

	subagents, err := a.ReadOpenclawSubagents()
	if err != nil {
		t.Fatalf("ReadOpenclawSubagents: %v", err)
	}
	// The id-less entry is skipped, like serde's failed from_value.
	if len(subagents) != 2 {
		t.Fatalf("expected 2 subagents, got %d: %+v", len(subagents), subagents)
	}
	main := subagents[0]
	if main.ID != "main" || main.Subagents == nil || main.Subagents.MaxConcurrent == nil || *main.Subagents.MaxConcurrent != 3 {
		t.Fatalf("main wrong: %+v", main)
	}
	if main.Subagents.AllowAgents == nil || len(*main.Subagents.AllowAgents) != 1 || (*main.Subagents.AllowAgents)[0] != "coder" {
		t.Fatalf("main.allowAgents wrong: %+v", main.Subagents)
	}
	coder := subagents[1]
	if coder.ID != "coder" || coder.Model == nil || *coder.Model.Primary != "openai/gpt-5.5" {
		t.Fatalf("coder wrong: %+v", coder)
	}
	if coder.Tools == nil || *coder.Tools.Profile != "coding" {
		t.Fatalf("coder.tools wrong: %+v", coder)
	}

	// Missing config → empty list (never null).
	if err := os.Remove(mustConfigPath(t)); err != nil {
		t.Fatalf("remove config: %v", err)
	}
	empty, err := a.ReadOpenclawSubagents()
	if err != nil {
		t.Fatalf("ReadOpenclawSubagents: %v", err)
	}
	if empty == nil || len(empty) != 0 {
		t.Fatalf("expected empty list, got %v", empty)
	}
}

func TestOpenclawSaveSubagentsMergesAndSynthesizesMain(t *testing.T) {
	isolatedHome(t)
	a := &App{}
	openclawTestWriteConfig(t, openclawSubagentsFixture)

	name := "Coder"
	if err := a.SaveOpenclawSubagents([]OpenClawSubAgent{
		{ID: "coder", Name: &name},
		{ID: "reviewer"},
	}); err != nil {
		t.Fatalf("SaveOpenclawSubagents: %v", err)
	}

	config := openclawTestConfigMap(t)
	if config["gateway"] == nil {
		t.Fatalf("gateway lost: %v", config)
	}
	list := openclawTestArray(t, openclawTestChild(t, config, "agents", "list"))
	if len(list) != 3 {
		t.Fatalf("expected 3 entries, got %d: %v", len(list), list)
	}

	// Synthesised main: allowAgents from the incoming ids, existing extras
	// (maxConcurrent, unknown fields) preserved by the deep merge.
	savedMain := openclawTestObject(t, list[0])
	if savedMain["id"] != "main" || savedMain["extraField"] != true {
		t.Fatalf("main not merged with existing entry: %v", savedMain)
	}
	savedAllow := openclawTestArray(t, openclawTestChild(t, savedMain, "subagents", "allowAgents"))
	if len(savedAllow) != 2 || savedAllow[0] != "coder" || savedAllow[1] != "reviewer" {
		t.Fatalf("allowAgents wrong: %v", savedAllow)
	}
	if got := openclawTestChild(t, savedMain, "subagents", "maxConcurrent"); got != float64(3) {
		t.Fatalf("maxConcurrent lost: %v", got)
	}

	// Existing coder merged with the incoming update.
	savedCoder := openclawTestObject(t, list[1])
	if savedCoder["id"] != "coder" || savedCoder["name"] != "Coder" {
		t.Fatalf("coder not updated: %v", savedCoder)
	}
	tools := openclawTestObject(t, savedCoder["tools"])
	if tools["profile"] != "coding" {
		t.Fatalf("coder.tools lost: %v", tools)
	}
	model := openclawTestObject(t, savedCoder["model"])
	if model["primary"] != "openai/gpt-5.5" {
		t.Fatalf("coder.model lost: %v", model)
	}

	// New agent appended.
	if got := openclawTestObject(t, list[2])["id"]; got != "reviewer" {
		t.Fatalf("reviewer missing: %v", list)
	}
}

func TestOpenclawSaveSubagentsDropsRemovedAgents(t *testing.T) {
	isolatedHome(t)
	a := &App{}
	openclawTestWriteConfig(t, openclawSubagentsFixture)

	if err := a.SaveOpenclawSubagents([]OpenClawSubAgent{{ID: "reviewer"}}); err != nil {
		t.Fatalf("SaveOpenclawSubagents: %v", err)
	}

	config := openclawTestConfigMap(t)
	list := openclawTestArray(t, openclawTestChild(t, config, "agents", "list"))
	if len(list) != 2 {
		t.Fatalf("expected 2 entries, got %d: %v", len(list), list)
	}
	savedMain := openclawTestObject(t, list[0])
	allow := openclawTestArray(t, openclawTestChild(t, savedMain, "subagents", "allowAgents"))
	if len(allow) != 1 || allow[0] != "reviewer" {
		t.Fatalf("allowAgents should track the incoming list: %v", allow)
	}
	for _, entry := range list {
		if openclawTestObject(t, entry)["id"] == "coder" {
			t.Fatalf("removed agent still present: %v", list)
		}
	}
}

func TestOpenclawSaveSubagentsKeepsExplicitMainAndEmptyAllowList(t *testing.T) {
	isolatedHome(t)
	a := &App{}
	openclawTestWriteConfig(t, openclawSubagentsFixture)

	empty := []string{}
	if err := a.SaveOpenclawSubagents([]OpenClawSubAgent{
		{ID: "main", Subagents: &OpenClawSubAgentSubagentsConfig{AllowAgents: &empty}},
		{ID: "helper"},
	}); err != nil {
		t.Fatalf("SaveOpenclawSubagents: %v", err)
	}

	config := openclawTestConfigMap(t)
	list := openclawTestArray(t, openclawTestChild(t, config, "agents", "list"))
	if len(list) != 2 {
		t.Fatalf("explicit main must not be duplicated: %v", list)
	}
	savedMain := openclawTestObject(t, list[0])
	allow := openclawTestArray(t, openclawTestChild(t, savedMain, "subagents", "allowAgents"))
	if len(allow) != 0 {
		t.Fatalf("cleared allowAgents should be an empty array: %v", allow)
	}
	// maxConcurrent was not part of the incoming overlay → preserved.
	if got := openclawTestChild(t, savedMain, "subagents", "maxConcurrent"); got != float64(3) {
		t.Fatalf("maxConcurrent should survive an overlay without it: %v", got)
	}
}

// TestOpenclawSaveSubagentsCreatesConfigFromScratch covers the no-config path:
// saving subagents materializes agents.list without touching anything else.
func TestOpenclawSaveSubagentsCreatesConfigFromScratch(t *testing.T) {
	isolatedHome(t)
	a := &App{}

	if err := a.SaveOpenclawSubagents([]OpenClawSubAgent{{ID: "helper"}}); err != nil {
		t.Fatalf("SaveOpenclawSubagents: %v", err)
	}
	config := openclawTestConfigMap(t)
	list := openclawTestArray(t, openclawTestChild(t, config, "agents", "list"))
	if len(list) != 2 {
		t.Fatalf("expected main + helper, got %v", list)
	}
	if got := openclawTestObject(t, list[0])["id"]; got != "main" {
		t.Fatalf("main should come first: %v", list)
	}
	// The document is pretty-printed like serde_json's to_string_pretty.
	raw := openclawTestReadFile(t, mustConfigPath(t))
	if !strings.Contains(raw, "\n  \"agents\": {") {
		t.Fatalf("config not pretty-printed: %s", raw)
	}
}
