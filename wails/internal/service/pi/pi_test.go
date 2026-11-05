package pi

import (
	"encoding/json"
	"fmt"
	"os"
	"strings"
	"testing"

	"droidgear-wails/internal/testutil"
)

func ptr[T any](v T) *T { return &v }

// collectNulls walks decoded JSON and returns the dotted paths whose value is
// null. Values directly under a `thinkingLevelMap` object are exempt: pi's
// schema allows null there and documents it as "this thinking level is
// unsupported".
func collectNulls(v any, path string, out *[]string) {
	switch t := v.(type) {
	case map[string]any:
		for key, child := range t {
			childPath := path + "." + key
			if child == nil {
				if strings.HasSuffix(path, ".thinkingLevelMap") {
					continue
				}
				*out = append(*out, childPath)
				continue
			}
			collectNulls(child, childPath, out)
		}
	case []any:
		for i, child := range t {
			collectNulls(child, fmt.Sprintf("%s[%d]", path, i), out)
		}
	}
}

// brokenProfile mirrors the shape the Pi UI submits for a provider whose
// optional fields are all unset — exactly the config that used to be written
// to models.json as `"oauth": null`, `"headers": null`, `"cost": null`, ...
// and then rejected by pi's schema.
func brokenProfile() PiProfile {
	return PiProfile{
		ID:        "p-nulls",
		Name:      "Nulls",
		CreatedAt: "2024-01-01T00:00:00Z",
		UpdatedAt: "2024-01-01T00:00:00Z",
		Providers: map[string]PiProviderConfig{
			"wududu": {
				BaseURL: ptr("https://sub.wududu.com/v1"),
				API:     ptr("openai-completions"),
				APIKey:  ptr("sk-test"),
				Models: []PiModel{
					{
						ID:            "deepseek-v4.1-flash",
						Name:          ptr("DeepSeek V4.1 Flash"),
						Reasoning:     true,
						Input:         []string{"text", "image"},
						ContextWindow: 1000000,
						MaxTokens:     393216,
						Compat:        &PiCompatConfig{},
					},
				},
			},
			"wududu-anthropic": {
				BaseURL: ptr("https://sub.wududu.com"),
				API:     ptr("anthropic-messages"),
				APIKey:  ptr("sk-test"),
				Models: []PiModel{
					{
						ID:        "claude-opus-5-5",
						Name:      ptr("Claude Opus 5.5"),
						Reasoning: true,
						// nil value = "minimal unsupported"; must survive as
						// an explicit null, which pi's schema allows.
						ThinkingLevelMap: map[string]*string{"max": ptr("max"), "minimal": nil},
						Input:            []string{"text", "image"},
						ContextWindow:    1000000,
						MaxTokens:        128000,
						Compat:           &PiCompatConfig{ForceAdaptiveThinking: ptr(true)},
					},
				},
			},
		},
	}
}

// TestApplyProfileWritesSchemaCleanModelsJSON is the regression test for
// "Invalid models.json schema": applying a profile used to emit null for every
// unset optional field, which pi's JSON schema rejects (oauth must be string,
// headers must be object, models[].api must be string, ...).
func TestApplyProfileWritesSchemaCleanModelsJSON(t *testing.T) {
	testutil.IsolatedHome(t)

	profile := brokenProfile()
	if err := SaveProfile(profile); err != nil {
		t.Fatalf("SaveProfile: %v", err)
	}
	if err := ApplyProfile(profile.ID); err != nil {
		t.Fatalf("ApplyProfile: %v", err)
	}

	path, err := piConfigPath()
	if err != nil {
		t.Fatalf("piConfigPath: %v", err)
	}
	data, err := os.ReadFile(path) // #nosec G304 -- isolated test path
	if err != nil {
		t.Fatalf("read models.json: %v", err)
	}

	var decoded any
	if err := json.Unmarshal(data, &decoded); err != nil {
		t.Fatalf("models.json is not valid JSON: %v", err)
	}

	var nulls []string
	collectNulls(decoded, "", &nulls)
	if len(nulls) > 0 {
		t.Errorf("models.json contains null values pi's schema rejects: %s\n%s", strings.Join(nulls, ", "), data)
	}

	root, ok := decoded.(map[string]any)
	if !ok {
		t.Fatalf("models.json root is not an object: %T", decoded)
	}
	providers, ok := root["providers"].(map[string]any)
	if !ok {
		t.Fatalf("models.json providers is not an object: %T", root["providers"])
	}
	if len(providers) != 2 {
		t.Fatalf("expected 2 providers, got %d", len(providers))
	}
	for id, raw := range providers {
		provider, ok := raw.(map[string]any)
		if !ok {
			t.Fatalf("provider %s is not an object: %T", id, raw)
		}
		// models must stay a present array: it is required by pi's schema
		// shape and non-optional in the generated TS bindings.
		models, ok := provider["models"].([]any)
		if !ok {
			t.Errorf("provider %s: models = %T, want array", id, provider["models"])
			continue
		}
		if len(models) == 0 {
			t.Errorf("provider %s: models is empty", id)
		}
		for i, rawModel := range models {
			model, ok := rawModel.(map[string]any)
			if !ok {
				t.Fatalf("provider %s model %d is not an object: %T", id, i, rawModel)
			}
			if _, ok := model["id"].(string); !ok {
				t.Errorf("provider %s model %d: id missing", id, i)
			}
		}
	}

	// The thinking level marked unsupported must keep its explicit null.
	anthropic := providers["wududu-anthropic"].(map[string]any)
	model := anthropic["models"].([]any)[0].(map[string]any)
	levelMap := model["thinkingLevelMap"].(map[string]any)
	if levelMap["minimal"] != nil {
		t.Errorf("thinkingLevelMap.minimal = %v, want null (level unsupported)", levelMap["minimal"])
	}
	if levelMap["max"] != "max" {
		t.Errorf("thinkingLevelMap.max = %v, want \"max\"", levelMap["max"])
	}
}

// TestProviderConfigMarshalKeepsModelsArray covers the provider-level edge
// case directly: a provider without models must serialize as `"models": []`,
// never `"models": null`.
func TestProviderConfigMarshalKeepsModelsArray(t *testing.T) {
	provider := PiProviderConfig{BaseURL: ptr("https://example.com/v1"), API: ptr("openai-completions")}
	data, err := json.Marshal(provider)
	if err != nil {
		t.Fatalf("marshal: %v", err)
	}
	var decoded map[string]any
	if err := json.Unmarshal(data, &decoded); err != nil {
		t.Fatalf("unmarshal: %v", err)
	}
	models, ok := decoded["models"].([]any)
	if !ok {
		t.Fatalf("models = %T, want array", decoded["models"])
	}
	if len(models) != 0 {
		t.Fatalf("expected empty models array, got %v", models)
	}
}

// TestApplyProfilePreservesUnknownProviderFields guards the Extra round-trip:
// keys DroidGear does not model (e.g. newer pi fields) must survive an
// apply cycle instead of being dropped.
func TestApplyProfilePreservesUnknownProviderFields(t *testing.T) {
	testutil.IsolatedHome(t)

	profile := brokenProfile()
	profile.Providers["wududu"] = PiProviderConfig{
		BaseURL: ptr("https://sub.wududu.com/v1"),
		API:     ptr("openai-completions"),
		APIKey:  ptr("sk-test"),
		Extra:   map[string]json.RawMessage{"promptCache": json.RawMessage(`{"short": 60}`)},
		Models:  []PiModel{{ID: "m1", Reasoning: true}},
	}
	if err := SaveProfile(profile); err != nil {
		t.Fatalf("SaveProfile: %v", err)
	}
	if err := ApplyProfile(profile.ID); err != nil {
		t.Fatalf("ApplyProfile: %v", err)
	}

	path, err := piConfigPath()
	if err != nil {
		t.Fatalf("piConfigPath: %v", err)
	}
	data, err := os.ReadFile(path) // #nosec G304 -- isolated test path
	if err != nil {
		t.Fatalf("read models.json: %v", err)
	}
	var decoded map[string]any
	if err := json.Unmarshal(data, &decoded); err != nil {
		t.Fatalf("unmarshal: %v", err)
	}
	provider := decoded["providers"].(map[string]any)["wududu"].(map[string]any)
	cache, ok := provider["promptCache"].(map[string]any)
	if !ok {
		t.Fatalf("promptCache dropped by apply: %v", provider)
	}
	if cache["short"] != float64(60) {
		t.Fatalf("promptCache.short = %v, want 60", cache["short"])
	}
}
