package omp

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

// sandbox points every home-ish root at a temp directory so the OMP paths
// resolve under it (mirrors the other service tests).
func sandbox(t *testing.T) string {
	t.Helper()
	home := t.TempDir()
	for _, key := range []string{"HOME", "USERPROFILE", "APPDATA", "LOCALAPPDATA"} {
		t.Setenv(key, home)
	}
	return home
}

func writeModelsFile(t *testing.T, home string, contents string) string {
	t.Helper()
	path := filepath.Join(home, ".omp", "agent", "models.yml")
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		t.Fatalf("mkdir: %v", err)
	}
	if err := os.WriteFile(path, []byte(contents), 0o644); err != nil { // #nosec G306 -- test fixture
		t.Fatalf("write models.yml: %v", err)
	}
	return path
}

func readModelsFile(t *testing.T, path string) string {
	t.Helper()
	data, err := os.ReadFile(path) // #nosec G304 -- test fixture path
	if err != nil {
		t.Fatalf("read models.yml: %v", err)
	}
	return string(data)
}

func strPtr(value string) *string { return &value }
func boolPtr(value bool) *bool    { return &value }
func u32Ptr(value uint32) *uint32 { return &value }

// channelImportedProvider mirrors what the GUI channel import produces: the
// optional provider/model fields are nil and were previously serialised as
// explicit YAML nulls, which OMP's schema rejects.
func channelImportedProvider() OmpCustomProvider {
	return OmpCustomProvider{
		BaseURL: strPtr("https://llm-gateway.example.test/v1"),
		API:     strPtr("openai-completions"),
		APIKey:  strPtr("sk-test"),
		Auth:    strPtr("apiKey"),
		Models: []OmpCustomModel{
			{
				ID:            "claude-opus-5--int",
				Reasoning:     boolPtr(false),
				ContextWindow: u32Ptr(128000),
				MaxTokens:     u32Ptr(16384),
			},
			{
				ID:            "claude-sonnet-5-5",
				Name:          strPtr("Claude Sonnet 5.5"),
				API:           nil,
				Reasoning:     boolPtr(true),
				Input:         &[]string{"text", "image"},
				ContextWindow: u32Ptr(1000000),
				MaxTokens:     u32Ptr(128000),
			},
		},
	}
}

func TestSaveCustomProviderOmitsNulls(t *testing.T) {
	home := sandbox(t)
	if err := SaveCustomProvider("litellm", channelImportedProvider()); err != nil {
		t.Fatalf("SaveCustomProvider: %v", err)
	}
	path := filepath.Join(home, ".omp", "agent", "models.yml")
	written := readModelsFile(t, path)

	// OMP's omptype schema rejects every explicit null, so none may survive.
	for _, line := range strings.Split(written, "\n") {
		if strings.HasSuffix(strings.TrimSpace(line), ": null") {
			t.Errorf("models.yml carries an explicit null: %q", line)
		}
	}
	if !strings.Contains(written, "reasoning: false") {
		t.Errorf("explicit false must survive pruning, got:\n%s", written)
	}
	if !strings.Contains(written, "name: Claude Sonnet 5.5") {
		t.Errorf("model name lost, got:\n%s", written)
	}
	// Integer windows must not be rewritten in exponent notation.
	if !strings.Contains(written, "contextWindow: 1000000") {
		t.Errorf("contextWindow not rendered as an integer, got:\n%s", written)
	}

	list, err := ReadCustomProviders()
	if err != nil {
		t.Fatalf("ReadCustomProviders: %v", err)
	}
	if len(list.Providers) != 1 {
		t.Fatalf("expected one provider, got %d", len(list.Providers))
	}
	provider := list.Providers[0].Config
	if provider.APIKey == nil || *provider.APIKey != "sk-test" {
		t.Errorf("apiKey round-trip failed: %+v", provider.APIKey)
	}
	if len(provider.Models) != 2 {
		t.Fatalf("expected two models, got %d", len(provider.Models))
	}
	first := provider.Models[0]
	if first.Name != nil || first.API != nil || first.Input != nil {
		t.Errorf("absent model fields must stay nil, got %+v", first)
	}
	if first.ContextWindow == nil || *first.ContextWindow != 128000 {
		t.Errorf("contextWindow round-trip failed: %+v", first.ContextWindow)
	}
}

func TestSaveCustomProviderRepairsLegacyNulls(t *testing.T) {
	home := sandbox(t)
	legacy := `providers:
    litellm:
        api: openai-completions
        apiKey: sk-test
        authHeader: null
        compat: null
        headers: null
        modelOverrides: null
        models:
            - id: claude-opus-5--int
              api: null
              input: null
              cost: null
        remoteCompaction: null
        transport: null
`
	path := writeModelsFile(t, home, legacy)

	// Saving any provider rewrites the whole file, so the legacy null keys
	// must disappear even when this provider is untouched by the save.
	if err := SaveCustomProvider("other", OmpCustomProvider{
		BaseURL: strPtr("https://other.example.test/v1"),
		APIKey:  strPtr("sk-other"),
		Models:  []OmpCustomModel{{ID: "other-model", API: strPtr("openai-completions")}},
	}); err != nil {
		t.Fatalf("SaveCustomProvider: %v", err)
	}
	written := readModelsFile(t, path)
	if strings.Contains(written, "null") {
		t.Fatalf("legacy nulls survived the rewrite:\n%s", written)
	}
	if !strings.Contains(written, "litellm:") {
		t.Fatalf("untouched provider was dropped:\n%s", written)
	}
}

func TestDeleteCustomProviderKeepsFileNullFree(t *testing.T) {
	home := sandbox(t)
	if err := SaveCustomProvider("keep", OmpCustomProvider{
		BaseURL: strPtr("https://keep.example.test/v1"),
		APIKey:  strPtr("sk-keep"),
		Models:  []OmpCustomModel{{ID: "keep-model", API: strPtr("openai-completions")}},
	}); err != nil {
		t.Fatalf("SaveCustomProvider: %v", err)
	}
	if err := DeleteCustomProvider("missing"); err != nil {
		t.Fatalf("DeleteCustomProvider: %v", err)
	}
	written := readModelsFile(t, filepath.Join(home, ".omp", "agent", "models.yml"))
	if strings.Contains(written, "null") {
		t.Fatalf("delete rewrite reintroduced nulls:\n%s", written)
	}
}

// TestApplyProfileWritesThinkingLevelAndPresets covers the remaining
// config.yml fields: defaultThinkingLevel and modelPresets.
func TestApplyProfileWritesThinkingLevelAndPresets(t *testing.T) {
	sandbox(t)
	level := "medium"
	if err := writeConfigYML(OmpAgentConfig{DefaultThinkingLevel: &level}); err != nil {
		t.Fatalf("writeConfigYML: %v", err)
	}
	profile := OmpProfile{
		ID: "p1", Name: "P1", CreatedAt: "now", UpdatedAt: "now",
		ModelRoles:           OmpModelRoles{Default: strPtr("prov/default")},
		DefaultThinkingLevel: strPtr("high"),
		ModelPresets: map[string]any{
			"deep": map[string]any{"defaultThinkingLevel": "high"},
		},
	}
	if err := SaveProfile(profile); err != nil {
		t.Fatalf("SaveProfile: %v", err)
	}
	if err := ApplyProfile("p1"); err != nil {
		t.Fatalf("ApplyProfile: %v", err)
	}
	config, err := readConfigYML()
	if err != nil {
		t.Fatalf("readConfigYML: %v", err)
	}
	if config.DefaultThinkingLevel == nil || *config.DefaultThinkingLevel != "high" {
		t.Errorf("defaultThinkingLevel = %v", config.DefaultThinkingLevel)
	}
	if _, ok := config.ModelPresets["deep"]; !ok {
		t.Errorf("modelPresets = %v", config.ModelPresets)
	}

	// A nil field leaves the live value untouched; an empty string clears it.
	profile.DefaultThinkingLevel = nil
	profile.ModelPresets = nil
	if err := SaveProfile(profile); err != nil {
		t.Fatalf("SaveProfile: %v", err)
	}
	if err := ApplyProfile("p1"); err != nil {
		t.Fatalf("ApplyProfile: %v", err)
	}
	config, _ = readConfigYML()
	if config.DefaultThinkingLevel == nil || *config.DefaultThinkingLevel != "high" {
		t.Errorf("nil should keep the live level, got %v", config.DefaultThinkingLevel)
	}
	empty := ""
	profile.DefaultThinkingLevel = &empty
	if err := SaveProfile(profile); err != nil {
		t.Fatalf("SaveProfile: %v", err)
	}
	if err := ApplyProfile("p1"); err != nil {
		t.Fatalf("ApplyProfile: %v", err)
	}
	config, _ = readConfigYML()
	if config.DefaultThinkingLevel != nil {
		t.Errorf("empty string should clear the level, got %v", config.DefaultThinkingLevel)
	}
}
