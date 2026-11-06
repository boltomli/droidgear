package pi

import (
	"encoding/json"
	"os"
	"testing"

	"droidgear-wails/internal/testutil"
)

func TestPrepareChannelImport(t *testing.T) {
	result, err := PrepareChannelImport(PiChannelImportInput{
		ChannelName: "My Chan",
		ChannelType: "new-api",
		BaseURL:     "https://api.example.com/",
		Platform:    ptr("openai"),
		APIKey:      "sk-test",
		TokenName:   "tok 1",
		ModelIDs:    []string{"claude-opus-5-5"},
	})
	if err != nil {
		t.Fatalf("PrepareChannelImport: %v", err)
	}
	if result.ProviderID != "my-chan-tok-1" {
		t.Errorf("ProviderID = %q", result.ProviderID)
	}
	if result.BaseURL != "https://api.example.com/v1" {
		t.Errorf("BaseURL = %q", result.BaseURL)
	}
	if result.API != "openai-completions" {
		t.Errorf("API = %q", result.API)
	}
	model := result.Models[0]
	if model.Name == nil || *model.Name != "Claude Opus 5.5" {
		t.Errorf("Name = %v, want registry name", model.Name)
	}
	if !model.Reasoning || model.ContextWindow != 1000000 || model.MaxTokens != 128000 {
		t.Errorf("specs = %+v", model)
	}
	if model.Compat == nil || model.Compat.ForceAdaptiveThinking == nil || !*model.Compat.ForceAdaptiveThinking {
		t.Errorf("compat = %+v, want forceAdaptiveThinking", model.Compat)
	}
}

func TestPrepareChannelImportRequiresKeyAndModels(t *testing.T) {
	if _, err := PrepareChannelImport(PiChannelImportInput{ModelIDs: []string{"m"}}); err == nil {
		t.Error("expected an error without an API key")
	}
	if _, err := PrepareChannelImport(PiChannelImportInput{APIKey: "sk"}); err == nil {
		t.Error("expected an error without models")
	}
}

// TestImportChannelProviderIsLossless: a channel import updates credentials,
// protocol and models, but keeps other providers and every field DroidGear
// does not model (upstream drift guard).
func TestImportChannelProviderIsLossless(t *testing.T) {
	testutil.IsolatedHome(t)
	path, err := piConfigPath()
	if err != nil {
		t.Fatalf("piConfigPath: %v", err)
	}
	live := `{
	  "$schema": "https://pi.dev/schemas/models.schema.json",
	  "providers": {
	    "keep": {"baseUrl": "https://keep.example.com/v1", "api": "openai-completions", "futureProviderKey": {"a": 1}, "models": []},
	    "imported": {
	      "baseUrl": "https://old.example.com/v1",
	      "api": "openai-completions",
	      "apiKey": "sk-old",
	      "futureProviderKey": {"a": 1},
	      "models": [{"id": "m1", "futureModelKey": "keep"}]
	    }
	  }
	}`
	if err := os.MkdirAll(path[:len(path)-len("/models.json")], 0o755); err != nil {
		t.Fatalf("mkdir: %v", err)
	}
	if err := os.WriteFile(path, []byte(live), 0o644); err != nil {
		t.Fatalf("write: %v", err)
	}

	result, err := ImportChannelProvider(PiChannelImportInput{
		ChannelName: "Chan",
		ChannelType: "new-api",
		BaseURL:     "https://api.example.com",
		Platform:    ptr("anthropic"),
		APIKey:      "sk-new",
		ProviderID:  "imported",
		ModelIDs:    []string{"m1"},
	})
	if err != nil {
		t.Fatalf("ImportChannelProvider: %v", err)
	}
	if result.ProviderID != "imported" {
		t.Fatalf("ProviderID = %q", result.ProviderID)
	}

	data, err := os.ReadFile(path) // #nosec G304 -- isolated test path
	if err != nil {
		t.Fatalf("read: %v", err)
	}
	var root map[string]any
	if err := json.Unmarshal(data, &root); err != nil {
		t.Fatalf("unmarshal: %v", err)
	}
	if root["$schema"] == nil {
		t.Error("$schema root key was dropped")
	}
	providers := root["providers"].(map[string]any)
	if _, ok := providers["keep"]; !ok {
		t.Error("unrelated provider was dropped by an import")
	}
	imported := providers["imported"].(map[string]any)
	if imported["apiKey"] != "sk-new" {
		t.Errorf("apiKey = %v, want sk-new", imported["apiKey"])
	}
	if imported["futureProviderKey"] == nil {
		t.Error("unknown provider key was dropped")
	}
	models := imported["models"].([]any)
	model := models[0].(map[string]any)
	if model["futureModelKey"] != "keep" {
		t.Errorf("unknown model key was dropped: %v", model)
	}
}
