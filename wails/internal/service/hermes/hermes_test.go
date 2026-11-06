package hermes

import (
	"os"
	"path/filepath"
	"testing"

	"droidgear-wails/internal/core"
	"droidgear-wails/internal/testutil"
	"gopkg.in/yaml.v3"
)

// writeHermesTestConfig drops a config.yaml into the isolated ~/.hermes so
// the Windows priority chain resolves to it deterministically.
func writeHermesTestConfig(t *testing.T, content string) string {
	t.Helper()
	dir := filepath.Join(mustHome(t), ".hermes")
	if err := os.MkdirAll(dir, 0o755); err != nil {
		t.Fatalf("mkdir: %v", err)
	}
	path := filepath.Join(dir, "config.yaml")
	if err := os.WriteFile(path, []byte(content), 0o644); err != nil {
		t.Fatalf("write config: %v", err)
	}
	return path
}

func mustHome(t *testing.T) string {
	t.Helper()
	home, err := os.UserHomeDir()
	if err != nil || home == "" {
		t.Fatalf("home dir: %v", err)
	}
	return home
}

// TestHermesReadCurrentKeepsEndpointForUnknownNamedProvider covers the load
// half of the "loads a wrong config and later destroys it" chain: when the
// live model section references `custom:<name>` but custom_providers has no
// matching entry, the loaded entry must still carry the model section's
// base_url/api_key.
func TestHermesReadCurrentKeepsEndpointForUnknownNamedProvider(t *testing.T) {
	testutil.IsolatedHome(t)

	writeHermesTestConfig(t, `
model:
  default: gpt-ghost
  provider: custom:ghost
  base_url: https://ghost.example.com/v1
  api_key: sk-ghost
`)

	current, err := ReadCurrentConfig()
	if err != nil {
		t.Fatalf("ReadHermesCurrentConfig: %v", err)
	}
	if len(current.Models) != 1 {
		t.Fatalf("expected 1 model, got %d: %+v", len(current.Models), current.Models)
	}
	loaded := current.Models[0]
	if loaded.Name == nil || *loaded.Name != "ghost" {
		t.Fatalf("expected name=ghost, got %+v", loaded)
	}
	if loaded.BaseURL == nil || *loaded.BaseURL != "https://ghost.example.com/v1" {
		t.Fatalf("base_url lost on load: %+v", loaded)
	}
	if loaded.APIKey == nil || *loaded.APIKey != "sk-ghost" {
		t.Fatalf("api_key lost on load: %+v", loaded)
	}
}

// TestHermesLoadThenApplyDoesNotStripEndpoint is the write half: loading a
// ghost `custom:<name>` config into a profile and applying it must upsert
// the endpoint into custom_providers instead of stripping base_url/api_key
// from the model section (which left Hermes with no endpoint at all).
func TestHermesLoadThenApplyDoesNotStripEndpoint(t *testing.T) {
	testutil.IsolatedHome(t)

	configPath := writeHermesTestConfig(t, `
model:
  default: gpt-ghost
  provider: custom:ghost
  base_url: https://ghost.example.com/v1
  api_key: sk-ghost
`)

	// Mirror the GUI flow: load from live config, then apply the profile.
	current, err := ReadCurrentConfig()
	if err != nil {
		t.Fatalf("ReadHermesCurrentConfig: %v", err)
	}
	profile := HermesProfile{
		ID:        "p1",
		Name:      "ghost",
		CreatedAt: core.NowRFC3339(),
		UpdatedAt: core.NowRFC3339(),
		Models:    current.Models,
	}
	if err := SaveProfile(profile); err != nil {
		t.Fatalf("SaveHermesProfile: %v", err)
	}
	if err := ApplyProfile("p1"); err != nil {
		t.Fatalf("ApplyHermesProfile: %v", err)
	}

	after, err := ReadCurrentConfig()
	if err != nil {
		t.Fatalf("ReadHermesCurrentConfig (after): %v", err)
	}
	if len(after.Models) != 1 {
		t.Fatalf("expected 1 model after apply, got %d: %+v", len(after.Models), after.Models)
	}
	got := after.Models[0]
	if got.BaseURL == nil || *got.BaseURL != "https://ghost.example.com/v1" {
		t.Fatalf("apply stripped base_url: %+v (config: %s)", got, testutil.MustRead(t, configPath))
	}
	if got.APIKey == nil || *got.APIKey != "sk-ghost" {
		t.Fatalf("apply stripped api_key: %+v (config: %s)", got, testutil.MustRead(t, configPath))
	}
	if got.Default == nil || *got.Default != "gpt-ghost" {
		t.Fatalf("apply lost model.default: %+v", got)
	}
	root := mustParseConfig(t, configPath)
	if core.MappingGetMap(core.MappingGetMap(root, "providers"), "ghost") == nil {
		t.Fatalf("ghost should be upserted into the providers dict: %s", testutil.MustRead(t, configPath))
	}
}

// TestHermesReadCommandsDoNotCreateConfigDir pins the no-side-effect
// contract: status/read must not create ~/.hermes (nor any fallback dir).
// A premature dir at a priority location changes what the next resolution
// picks, which is how read paths used to "configure themselves".
func TestHermesReadCommandsDoNotCreateConfigDir(t *testing.T) {
	testutil.IsolatedHome(t)

	status, err := GetConfigStatus()
	if err != nil {
		t.Fatalf("GetHermesConfigStatus: %v", err)
	}
	if status.ConfigExists {
		t.Fatalf("config should not exist in a fresh home: %+v", status)
	}
	if _, err := ReadCurrentConfig(); err != nil {
		t.Fatalf("ReadHermesCurrentConfig: %v", err)
	}

	hermesDir := filepath.Join(mustHome(t), ".hermes")
	if _, err := os.Stat(hermesDir); err == nil {
		t.Fatalf("read-only commands created %s", hermesDir)
	}
}

func mustParseConfig(t *testing.T, path string) *yaml.Node {
	t.Helper()
	root, err := core.ParseYAML(testutil.MustRead(t, path))
	if err != nil {
		t.Fatalf("parse config: %v", err)
	}
	return root
}

// TestHermesReadCurrentConfigFromProvidersDict pins the v12 load path: the
// providers dictionary is primary, endpoint-less catalog overrides are
// skipped, and a bare-name model.provider matches a dict key.
func TestHermesReadCurrentConfigFromProvidersDict(t *testing.T) {
	testutil.IsolatedHome(t)

	writeHermesTestConfig(t, `
model:
  default: mimo-v2.6-flash
  provider: token-plan
providers:
  deepseek:
    models:
      deepseek-vision:
        supports_vision: true
  sensenova:
    api: https://token.sensenova.cn/v1
    api_key: sk-sn
    default_model: sensenova-6.8-flash-lite
  token-plan:
    api: https://token.example.com/v1
    api_key: sk-tp
    default_model: mimo-v2.6-flash
`)

	current, err := ReadCurrentConfig()
	if err != nil {
		t.Fatalf("ReadHermesCurrentConfig: %v", err)
	}
	if len(current.Models) != 2 {
		t.Fatalf("endpoint-less override entries must be skipped, got %+v", current.Models)
	}
	var active, sensenova *HermesModelConfig
	for i := range current.Models {
		switch {
		case current.Models[i].IsDefault:
			active = &current.Models[i]
		case current.Models[i].Name != nil && *current.Models[i].Name == "sensenova":
			sensenova = &current.Models[i]
		}
	}
	if active == nil || active.Name == nil || *active.Name != "token-plan" {
		t.Fatalf("bare-name provider should match the dict key: %+v", current.Models)
	}
	if active.Default == nil || *active.Default != "mimo-v2.6-flash" {
		t.Fatalf("active default wrong: %+v", active)
	}
	if active.APIKey == nil || *active.APIKey != "sk-tp" {
		t.Fatalf("active api_key wrong: %+v", active)
	}
	if sensenova == nil || sensenova.BaseURL == nil || *sensenova.BaseURL != "https://token.sensenova.cn/v1" {
		t.Fatalf("sensenova entry wrong: %+v", sensenova)
	}
}

// TestHermesApplyUpsertsProvidersDictPreservingFields: apply updates only
// api/api_key/default_model and keeps discover_models/models intact.
func TestHermesApplyUpsertsProvidersDictPreservingFields(t *testing.T) {
	testutil.IsolatedHome(t)

	configPath := writeHermesTestConfig(t, `
providers:
  mine:
    api: https://old.example.com/v1
    default_model: old-model
    discover_models: true
    models:
      old-model: {}
`)

	profile := HermesProfile{
		ID: "p1", Name: "Profile 1", CreatedAt: core.NowRFC3339(), UpdatedAt: core.NowRFC3339(),
		Models: []HermesModelConfig{{
			Name: core.StrPtr("mine"), Default: core.StrPtr("gpt-4"),
			BaseURL: core.StrPtr("https://new.example.com/v1"), APIKey: core.StrPtr("sk-new"),
			IsDefault: true,
		}},
	}
	if err := SaveProfile(profile); err != nil {
		t.Fatalf("SaveHermesProfile: %v", err)
	}
	if err := ApplyProfile("p1"); err != nil {
		t.Fatalf("ApplyHermesProfile: %v", err)
	}

	root := mustParseConfig(t, configPath)
	mine := core.MappingGetMap(core.MappingGetMap(root, "providers"), "mine")
	if mine == nil {
		t.Fatalf("providers.mine missing: %s", testutil.MustRead(t, configPath))
	}
	if got := core.MappingGetString(mine, "api"); got != "https://new.example.com/v1" {
		t.Fatalf("api not updated: %q", got)
	}
	if got := core.MappingGetString(mine, "api_key"); got != "sk-new" {
		t.Fatalf("api_key not updated: %q", got)
	}
	if got := core.MappingGetString(mine, "default_model"); got != "gpt-4" {
		t.Fatalf("default_model not updated: %q", got)
	}
	if core.MappingGet(mine, "discover_models") == nil {
		t.Fatalf("discover_models must be preserved")
	}
	if core.MappingGet(mine, "models") == nil {
		t.Fatalf("models must be preserved")
	}
	model := core.MappingGetMap(root, "model")
	if got := core.MappingGetString(model, "provider"); got != "custom:mine" {
		t.Fatalf("model.provider wrong: %q", got)
	}
}

// TestHermesApplyMigratesLegacyCustomProvidersEntry: same-name legacy
// custom_providers entries move into the providers dict and the (emptied)
// key disappears.
func TestHermesApplyMigratesLegacyCustomProvidersEntry(t *testing.T) {
	testutil.IsolatedHome(t)

	configPath := writeHermesTestConfig(t, `
model:
  default: old-model
  provider: custom:wududu
custom_providers:
- name: wududu
  base_url: https://old.example.com/v1
  model: old-model
  api_key: sk-old
`)

	profile := HermesProfile{
		ID: "p1", Name: "Profile 1", CreatedAt: core.NowRFC3339(), UpdatedAt: core.NowRFC3339(),
		Models: []HermesModelConfig{{
			Name: core.StrPtr("wududu"), Default: core.StrPtr("gpt-new"),
			BaseURL: core.StrPtr("https://new.example.com/v1"), APIKey: core.StrPtr("sk-new"),
			IsDefault: true,
		}},
	}
	if err := SaveProfile(profile); err != nil {
		t.Fatalf("SaveHermesProfile: %v", err)
	}
	if err := ApplyProfile("p1"); err != nil {
		t.Fatalf("ApplyHermesProfile: %v", err)
	}

	root := mustParseConfig(t, configPath)
	if core.MappingGet(root, "custom_providers") != nil {
		t.Fatalf("legacy custom_providers should be removed: %s", testutil.MustRead(t, configPath))
	}
	wududu := core.MappingGetMap(core.MappingGetMap(root, "providers"), "wududu")
	if wududu == nil {
		t.Fatalf("providers.wududu missing: %s", testutil.MustRead(t, configPath))
	}
	if got := core.MappingGetString(wududu, "api"); got != "https://new.example.com/v1" {
		t.Fatalf("api wrong: %q", got)
	}
	if got := core.MappingGetString(wududu, "api_key"); got != "sk-new" {
		t.Fatalf("api_key wrong: %q", got)
	}
	if got := core.MappingGetString(wududu, "default_model"); got != "gpt-new" {
		t.Fatalf("default_model wrong: %q", got)
	}
	model := core.MappingGetMap(root, "model")
	if got := core.MappingGetString(model, "provider"); got != "custom:wududu" {
		t.Fatalf("model.provider wrong: %q", got)
	}
	if got := core.MappingGetString(model, "default"); got != "gpt-new" {
		t.Fatalf("model.default wrong: %q", got)
	}
	if core.MappingGet(model, "base_url") != nil || core.MappingGet(model, "api_key") != nil {
		t.Fatalf("model section should not keep base_url/api_key for named providers")
	}
}

// TestHermesProviderOptionsRoundTrip covers the v12 request knobs and the
// per-model capabilities added for upstream drift.
func TestHermesProviderOptionsRoundTrip(t *testing.T) {
	testutil.IsolatedHome(t)
	discover := false
	promptCaching := true
	answerInReasoning := true
	contextLength := uint32(1000000)
	streamingOff := false
	profile := HermesProfile{
		ID: "p-knobs", Name: "Knobs", CreatedAt: core.NowRFC3339(), UpdatedAt: core.NowRFC3339(),
		Models: []HermesModelConfig{{
			Name: core.StrPtr("gateway"), Default: core.StrPtr("fable"),
			BaseURL: core.StrPtr("https://gateway.example.com/v1"), APIKey: core.StrPtr("sk"),
			IsDefault:             true,
			ExtraHeaders:          map[string]string{"CF-Access-Client-Id": "client"},
			SessionAffinityHeader: core.StrPtr("x-litellm-session-id"),
			DiscoverModels:        &discover,
			Transport:             core.StrPtr("anthropic_messages"),
			Capabilities:          map[string]any{"openai_native_compaction": true},
			ContextLength:         &contextLength,
			PromptCaching:         &promptCaching,
			AnswerInReasoning:     &answerInReasoning,
			APIMode:               core.StrPtr("chat_completions"),
			Streaming:             &streamingOff,
		}},
	}
	if err := SaveProfile(profile); err != nil {
		t.Fatalf("SaveProfile: %v", err)
	}
	if err := ApplyProfile("p-knobs"); err != nil {
		t.Fatalf("ApplyProfile: %v", err)
	}

	current, err := ReadCurrentConfig()
	if err != nil {
		t.Fatalf("ReadCurrentConfig: %v", err)
	}
	if len(current.Models) != 1 {
		t.Fatalf("models = %+v", current.Models)
	}
	loaded := current.Models[0]
	if loaded.ExtraHeaders["CF-Access-Client-Id"] != "client" {
		t.Errorf("extra headers = %v", loaded.ExtraHeaders)
	}
	if loaded.SessionAffinityHeader == nil || *loaded.SessionAffinityHeader != "x-litellm-session-id" {
		t.Errorf("session affinity = %v", loaded.SessionAffinityHeader)
	}
	if loaded.DiscoverModels == nil || *loaded.DiscoverModels {
		t.Errorf("discover models = %v", loaded.DiscoverModels)
	}
	if loaded.Transport == nil || *loaded.Transport != "anthropic_messages" {
		t.Errorf("transport = %v", loaded.Transport)
	}
	if loaded.Capabilities["openai_native_compaction"] != true {
		t.Errorf("capabilities = %v", loaded.Capabilities)
	}
	if loaded.ContextLength == nil || *loaded.ContextLength != contextLength {
		t.Errorf("context length = %v", loaded.ContextLength)
	}
	if loaded.PromptCaching == nil || !*loaded.PromptCaching || loaded.AnswerInReasoning == nil || !*loaded.AnswerInReasoning {
		t.Errorf("model flags = %v / %v", loaded.PromptCaching, loaded.AnswerInReasoning)
	}
	if loaded.APIMode == nil || *loaded.APIMode != "chat_completions" {
		t.Errorf("api mode = %v", loaded.APIMode)
	}
	if loaded.Streaming == nil || *loaded.Streaming {
		t.Errorf("streaming = %v", loaded.Streaming)
	}
}

// TestHermesImportChannelProvider covers the shared channel-import path and
// DeleteLiveProvider.
func TestHermesImportChannelProvider(t *testing.T) {
	testutil.IsolatedHome(t)
	configPath := writeHermesTestConfig(t, `
model:
  default: existing
  provider: custom:mine
providers:
  mine:
    api: https://mine.example.com/v1
    api_key: sk-mine
  spare:
    api: https://spare.example.com/v1
`)

	result, err := ImportChannelProvider(HermesChannelImportInput{
		ChannelName: "My Chan", ChannelType: "general",
		BaseURL: "https://api.example.com", APIKey: "sk-test", DefaultModel: "gpt-5.2",
	})
	if err != nil {
		t.Fatalf("ImportChannelProvider: %v", err)
	}
	if result.Provider != "custom" || result.BaseURL != "https://api.example.com/v1" {
		t.Fatalf("result = %+v", result)
	}
	root := mustParseConfig(t, configPath)
	providers := core.MappingGetMap(root, "providers")
	imported := core.MappingGetMap(providers, "My Chan")
	if imported == nil {
		t.Fatalf("imported provider missing: %s", testutil.MustRead(t, configPath))
	}
	if got := core.MappingGetString(imported, "api"); got != "https://api.example.com/v1" {
		t.Errorf("api = %q", got)
	}
	if core.MappingGetMap(providers, "spare") == nil {
		t.Error("unrelated provider dropped")
	}
	model := core.MappingGetMap(root, "model")
	if got := core.MappingGetString(model, "provider"); got != "custom:My Chan" {
		t.Errorf("model.provider = %q", got)
	}

	if err := DeleteLiveProvider("spare"); err != nil {
		t.Fatalf("DeleteLiveProvider(spare): %v", err)
	}
	root = mustParseConfig(t, configPath)
	if core.MappingGetMap(core.MappingGetMap(root, "providers"), "spare") != nil {
		t.Error("spare was not deleted")
	}
	if err := DeleteLiveProvider("My Chan"); err == nil {
		t.Error("deleting the active provider should be refused")
	}
}
