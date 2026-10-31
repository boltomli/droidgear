package app

import (
	"os"
	"path/filepath"
	"testing"

	"droidgear-wails/internal/core"
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
	isolatedHome(t)
	a := &App{}

	writeHermesTestConfig(t, `
model:
  default: gpt-ghost
  provider: custom:ghost
  base_url: https://ghost.example.com/v1
  api_key: sk-ghost
`)

	current, err := a.ReadHermesCurrentConfig()
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
	isolatedHome(t)
	a := &App{}

	configPath := writeHermesTestConfig(t, `
model:
  default: gpt-ghost
  provider: custom:ghost
  base_url: https://ghost.example.com/v1
  api_key: sk-ghost
`)

	// Mirror the GUI flow: load from live config, then apply the profile.
	current, err := a.ReadHermesCurrentConfig()
	if err != nil {
		t.Fatalf("ReadHermesCurrentConfig: %v", err)
	}
	profile := HermesProfile{
		ID:        "p1",
		Name:      "ghost",
		CreatedAt: nowRFC3339(),
		UpdatedAt: nowRFC3339(),
		Models:    current.Models,
	}
	if err := a.SaveHermesProfile(profile); err != nil {
		t.Fatalf("SaveHermesProfile: %v", err)
	}
	if err := a.ApplyHermesProfile("p1"); err != nil {
		t.Fatalf("ApplyHermesProfile: %v", err)
	}

	after, err := a.ReadHermesCurrentConfig()
	if err != nil {
		t.Fatalf("ReadHermesCurrentConfig (after): %v", err)
	}
	if len(after.Models) != 1 {
		t.Fatalf("expected 1 model after apply, got %d: %+v", len(after.Models), after.Models)
	}
	got := after.Models[0]
	if got.BaseURL == nil || *got.BaseURL != "https://ghost.example.com/v1" {
		t.Fatalf("apply stripped base_url: %+v (config: %s)", got, mustRead(t, configPath))
	}
	if got.APIKey == nil || *got.APIKey != "sk-ghost" {
		t.Fatalf("apply stripped api_key: %+v (config: %s)", got, mustRead(t, configPath))
	}
	if got.Default == nil || *got.Default != "gpt-ghost" {
		t.Fatalf("apply lost model.default: %+v", got)
	}
	root := mustParseConfig(t, configPath)
	if core.MappingGetMap(core.MappingGetMap(root, "providers"), "ghost") == nil {
		t.Fatalf("ghost should be upserted into the providers dict: %s", mustRead(t, configPath))
	}
}

// TestHermesReadCommandsDoNotCreateConfigDir pins the no-side-effect
// contract: status/read must not create ~/.hermes (nor any fallback dir).
// A premature dir at a priority location changes what the next resolution
// picks, which is how read paths used to "configure themselves".
func TestHermesReadCommandsDoNotCreateConfigDir(t *testing.T) {
	isolatedHome(t)
	a := &App{}

	status, err := a.GetHermesConfigStatus()
	if err != nil {
		t.Fatalf("GetHermesConfigStatus: %v", err)
	}
	if status.ConfigExists {
		t.Fatalf("config should not exist in a fresh home: %+v", status)
	}
	if _, err := a.ReadHermesCurrentConfig(); err != nil {
		t.Fatalf("ReadHermesCurrentConfig: %v", err)
	}

	hermesDir := filepath.Join(mustHome(t), ".hermes")
	if _, err := os.Stat(hermesDir); err == nil {
		t.Fatalf("read-only commands created %s", hermesDir)
	}
}

func mustRead(t *testing.T, path string) string {
	t.Helper()
	data, err := os.ReadFile(path) // #nosec G304 -- test fixture
	if err != nil {
		t.Fatalf("read %s: %v", path, err)
	}
	return string(data)
}

func mustParseConfig(t *testing.T, path string) *yaml.Node {
	t.Helper()
	root, err := core.ParseYAML(mustRead(t, path))
	if err != nil {
		t.Fatalf("parse config: %v", err)
	}
	return root
}

// TestHermesReadCurrentConfigFromProvidersDict pins the v12 load path: the
// providers dictionary is primary, endpoint-less catalog overrides are
// skipped, and a bare-name model.provider matches a dict key.
func TestHermesReadCurrentConfigFromProvidersDict(t *testing.T) {
	isolatedHome(t)
	a := &App{}

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

	current, err := a.ReadHermesCurrentConfig()
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
	isolatedHome(t)
	a := &App{}

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
		ID: "p1", Name: "Profile 1", CreatedAt: nowRFC3339(), UpdatedAt: nowRFC3339(),
		Models: []HermesModelConfig{{
			Name: strPtr("mine"), Default: strPtr("gpt-4"),
			BaseURL: strPtr("https://new.example.com/v1"), APIKey: strPtr("sk-new"),
			IsDefault: true,
		}},
	}
	if err := a.SaveHermesProfile(profile); err != nil {
		t.Fatalf("SaveHermesProfile: %v", err)
	}
	if err := a.ApplyHermesProfile("p1"); err != nil {
		t.Fatalf("ApplyHermesProfile: %v", err)
	}

	root := mustParseConfig(t, configPath)
	mine := core.MappingGetMap(core.MappingGetMap(root, "providers"), "mine")
	if mine == nil {
		t.Fatalf("providers.mine missing: %s", mustRead(t, configPath))
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
	isolatedHome(t)
	a := &App{}

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
		ID: "p1", Name: "Profile 1", CreatedAt: nowRFC3339(), UpdatedAt: nowRFC3339(),
		Models: []HermesModelConfig{{
			Name: strPtr("wududu"), Default: strPtr("gpt-new"),
			BaseURL: strPtr("https://new.example.com/v1"), APIKey: strPtr("sk-new"),
			IsDefault: true,
		}},
	}
	if err := a.SaveHermesProfile(profile); err != nil {
		t.Fatalf("SaveHermesProfile: %v", err)
	}
	if err := a.ApplyHermesProfile("p1"); err != nil {
		t.Fatalf("ApplyHermesProfile: %v", err)
	}

	root := mustParseConfig(t, configPath)
	if core.MappingGet(root, "custom_providers") != nil {
		t.Fatalf("legacy custom_providers should be removed: %s", mustRead(t, configPath))
	}
	wududu := core.MappingGetMap(core.MappingGetMap(root, "providers"), "wududu")
	if wududu == nil {
		t.Fatalf("providers.wududu missing: %s", mustRead(t, configPath))
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
