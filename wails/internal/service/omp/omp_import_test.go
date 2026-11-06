package omp

import (
	"encoding/json"
	"testing"
)

func TestPrepareChannelImport(t *testing.T) {
	result, err := PrepareChannelImport(OmpChannelImportInput{
		ChannelName: "My Chan",
		ChannelType: "sub-2-api",
		BaseURL:     "https://api.example.com",
		Platform:    strPtr("openai"),
		APIKey:      "sk-test",
		TokenName:   "tok",
		ModelIDs:    []string{"claude-opus-5-5"},
	})
	if err != nil {
		t.Fatalf("PrepareChannelImport: %v", err)
	}
	if result.ProviderID != "my-chan-tok" {
		t.Errorf("ProviderID = %q", result.ProviderID)
	}
	provider := result.Provider
	if provider.BaseURL == nil || *provider.BaseURL != "https://api.example.com/v1" {
		t.Errorf("BaseURL = %v", provider.BaseURL)
	}
	if provider.API == nil || *provider.API != "openai-completions" {
		t.Errorf("API = %v", provider.API)
	}
	if provider.Auth == nil || *provider.Auth != "apiKey" {
		t.Errorf("Auth = %v, want apiKey", provider.Auth)
	}
	model := provider.Models[0]
	if model.Name == nil || *model.Name != "Claude Opus 5.5" {
		t.Errorf("Name = %v, want registry name", model.Name)
	}
	if model.ContextWindow == nil || *model.ContextWindow != 1000000 {
		t.Errorf("ContextWindow = %v", model.ContextWindow)
	}
	if model.MaxTokens == nil || *model.MaxTokens != 128000 {
		t.Errorf("MaxTokens = %v", model.MaxTokens)
	}
	if model.Input == nil || len(*model.Input) == 0 {
		t.Errorf("Input = %v, want registry modalities", model.Input)
	}
}

func TestPrepareChannelImportAntigravity(t *testing.T) {
	result, err := PrepareChannelImport(OmpChannelImportInput{
		ChannelName: "ag",
		ChannelType: "new-api",
		BaseURL:     "https://ag.example.com/",
		Platform:    strPtr("antigravity"),
		APIKey:      "sk",
		ModelIDs:    []string{"claude-opus-5-5"},
	})
	if err != nil {
		t.Fatalf("PrepareChannelImport: %v", err)
	}
	if *result.Provider.BaseURL != "https://ag.example.com" {
		t.Errorf("BaseURL = %q, want channel base URL", *result.Provider.BaseURL)
	}
	if *result.Provider.API != "anthropic-messages" {
		t.Errorf("API = %q", *result.Provider.API)
	}
}

// TestImportChannelProviderIsLossless: re-importing a provider updates
// credentials, protocol and models while keeping unrelated providers and
// unknown keys already on disk.
func TestImportChannelProviderIsLossless(t *testing.T) {
	home := sandbox(t)
	writeModelsFile(t, home, `providers:
  keep:
    baseUrl: https://keep.example.com/v1
    api: openai-completions
    futureProviderKey:
      a: 1
    models: []
  imported:
    baseUrl: https://old.example.com/v1
    api: openai-completions
    apiKey: sk-old
    futureProviderKey:
      a: 1
    models:
      - id: claude-opus-5-5
        futureModelKey: keep
`)

	result, err := ImportChannelProvider(OmpChannelImportInput{
		ChannelName: "Chan",
		ChannelType: "new-api",
		BaseURL:     "https://api.example.com",
		Platform:    strPtr("anthropic"),
		APIKey:      "sk-new",
		ProviderID:  "imported",
		ModelIDs:    []string{"claude-opus-5-5"},
	})
	if err != nil {
		t.Fatalf("ImportChannelProvider: %v", err)
	}
	if result.ProviderID != "imported" {
		t.Fatalf("ProviderID = %q", result.ProviderID)
	}

	config, err := readModelsYML()
	if err != nil {
		t.Fatalf("readModelsYML: %v", err)
	}
	if _, ok := config.Providers["keep"]; !ok {
		t.Error("unrelated provider was dropped by an import")
	}
	imported, ok := config.Providers["imported"].(map[string]any)
	if !ok {
		t.Fatalf("imported provider = %T", config.Providers["imported"])
	}
	if imported["apiKey"] != "sk-new" {
		t.Errorf("apiKey = %v, want sk-new", imported["apiKey"])
	}
	if imported["api"] != "anthropic-messages" {
		t.Errorf("api = %v", imported["api"])
	}
	if _, ok := imported["futureProviderKey"]; !ok {
		t.Error("unknown provider key was dropped")
	}
	models, _ := imported["models"].([]any)
	if len(models) != 1 {
		t.Fatalf("models = %v", imported["models"])
	}
	model, _ := models[0].(map[string]any)
	if model["futureModelKey"] != "keep" {
		t.Errorf("unknown model key was dropped: %v", model)
	}
}

func TestMergeOmpModelRolesKeepsUnknownRoles(t *testing.T) {
	existing := &OmpModelRoles{
		Default: strPtr("live/default"),
		Advisor: strPtr("live/advisor"),
		Extra:   map[string]json.RawMessage{"futureRole": json.RawMessage(`"live/future"`)},
	}
	merged := mergeOmpModelRoles(existing, OmpModelRoles{Default: strPtr("profile/default")})
	if merged.Default == nil || *merged.Default != "profile/default" {
		t.Errorf("Default = %v", merged.Default)
	}
	if merged.Advisor != nil {
		t.Errorf("Advisor = %v, want cleared", merged.Advisor)
	}
	if string(merged.Extra["futureRole"]) != `"live/future"` {
		t.Errorf("unknown role dropped: %v", merged.Extra)
	}
}
