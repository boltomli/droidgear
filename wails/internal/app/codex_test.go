package app

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/pelletier/go-toml/v2"
)

// codexTestConfigPath resolves <isolated home>/.codex/config.toml (creating
// the codex dir, exactly like the Rust apply path does).
func codexTestConfigPath(t *testing.T) string {
	t.Helper()
	path, err := codexConfigPath()
	if err != nil {
		t.Fatalf("codexConfigPath: %v", err)
	}
	return path
}

func codexTestWriteConfig(t *testing.T, content string) string {
	t.Helper()
	path := codexTestConfigPath(t)
	if err := os.WriteFile(path, []byte(content), 0o644); err != nil {
		t.Fatalf("write config: %v", err)
	}
	return path
}

// codexTestParseConfig re-reads config.toml into a generic table for
// assertions.
func codexTestParseConfig(t *testing.T) map[string]any {
	t.Helper()
	path := codexTestConfigPath(t)
	data, err := os.ReadFile(path)
	if err != nil {
		t.Fatalf("read config: %v", err)
	}
	var config map[string]any
	if err := toml.Unmarshal(data, &config); err != nil {
		t.Fatalf("parse config: %v", err)
	}
	return config
}

func codexTestCustomProfile(id string, name string) CodexProfile {
	return CodexProfile{
		ID:     id,
		Name:   name,
		APIKey: strPtr("sk-provider"),
		Providers: map[string]CodexProviderConfig{
			"custom": {
				Name:                 strPtr("Custom"),
				BaseURL:              strPtr("https://api.example.com/v1"),
				WireAPI:              strPtr("responses"),
				RequiresOpenaiAuth:   codexPtr(false),
				Model:                strPtr("gpt-5.5"),
				ModelReasoningEffort: strPtr("high"),
				APIKey:               strPtr("sk-provider"),
				EnvKey:               strPtr("EXAMPLE_API_KEY"),
			},
		},
		ModelProvider:        "custom",
		Model:                "gpt-5.5",
		ModelReasoningEffort: strPtr("medium"),
		APIKeyModelDiscovery: true,
	}
}

// TestCodexProfileCRUDRoundTrip covers save (blank id → uuid) → list (sorted
// by lowercased name) → get → duplicate → delete clearing the active pointer.
func TestCodexProfileCRUDRoundTrip(t *testing.T) {
	isolatedHome(t)
	a := &App{}

	// Missing profiles dir → empty list, not an error.
	profiles, err := a.ListCodexProfiles()
	if err != nil || len(profiles) != 0 {
		t.Fatalf("empty list: %v %+v", err, profiles)
	}

	// Blank id gets a fresh UUID + timestamps.
	created := codexTestCustomProfile("", "Second")
	if err := a.SaveCodexProfile(created); err != nil {
		t.Fatalf("SaveCodexProfile: %v", err)
	}
	profiles, err = a.ListCodexProfiles()
	if err != nil || len(profiles) != 1 {
		t.Fatalf("list after save: %v %d", err, len(profiles))
	}
	saved := profiles[0]
	if saved.ID == "" || saved.CreatedAt == "" || saved.UpdatedAt == "" {
		t.Fatalf("expected generated id/timestamps: %+v", saved)
	}
	if saved.CreatedAt != saved.UpdatedAt {
		t.Fatalf("fresh save: created != updated: %q %q", saved.CreatedAt, saved.UpdatedAt)
	}

	// Second profile with a preset id; list sorts case-insensitively.
	other := codexTestCustomProfile("profile-a", "Alpha")
	if err := a.SaveCodexProfile(other); err != nil {
		t.Fatalf("SaveCodexProfile(a): %v", err)
	}
	profiles, err = a.ListCodexProfiles()
	if err != nil || len(profiles) != 2 {
		t.Fatalf("list: %v %d", err, len(profiles))
	}
	if profiles[0].Name != "Alpha" || profiles[1].Name != "Second" {
		t.Fatalf("expected case-insensitive name order, got %q / %q", profiles[0].Name, profiles[1].Name)
	}

	// Get by id round-trips the provider table.
	got, err := a.GetCodexProfile("profile-a")
	if err != nil {
		t.Fatalf("GetCodexProfile: %v", err)
	}
	if got.Providers["custom"].BaseURL == nil || *got.Providers["custom"].BaseURL != "https://api.example.com/v1" {
		t.Fatalf("provider not round-tripped: %+v", got.Providers)
	}

	// Re-saving an existing id preserves created_at.
	updated := got
	updated.Name = "Alpha Renamed"
	updated.UpdatedAt = "" // server stamps it
	if err := a.SaveCodexProfile(updated); err != nil {
		t.Fatalf("SaveCodexProfile(update): %v", err)
	}
	reloaded, err := a.GetCodexProfile("profile-a")
	if err != nil || reloaded.CreatedAt != got.CreatedAt {
		t.Fatalf("created_at not preserved: %v %q vs %q", err, reloaded.CreatedAt, got.CreatedAt)
	}

	// Duplicate gets a new id, the requested name, and fresh timestamps.
	dup, err := a.DuplicateCodexProfile("profile-a", "Copy")
	if err != nil {
		t.Fatalf("DuplicateCodexProfile: %v", err)
	}
	if dup.ID == "profile-a" || dup.Name != "Copy" || dup.CreatedAt == "" {
		t.Fatalf("unexpected duplicate: %+v", dup)
	}
	if _, err := a.GetCodexProfile(dup.ID); err != nil {
		t.Fatalf("duplicate not readable: %v", err)
	}

	// Applying records the active profile; deleting it clears the pointer.
	if err := a.ApplyCodexProfile("profile-a"); err != nil {
		t.Fatalf("ApplyCodexProfile: %v", err)
	}
	active, err := a.GetActiveCodexProfileId()
	if err != nil || active == nil || *active != "profile-a" {
		t.Fatalf("active profile: %v %v", err, active)
	}
	if err := a.DeleteCodexProfile("profile-a"); err != nil {
		t.Fatalf("DeleteCodexProfile: %v", err)
	}
	active, err = a.GetActiveCodexProfileId()
	if err != nil || active != nil {
		t.Fatalf("active not cleared: %v %v", err, active)
	}
}

// TestCodexSaveProfileValidation covers the reserved provider name and the
// invalid-id guard, byte-identical to the Rust error strings.
func TestCodexSaveProfileValidation(t *testing.T) {
	isolatedHome(t)
	a := &App{}

	reserved := codexTestCustomProfile("p1", "P1")
	reserved.Providers = map[string]CodexProviderConfig{"OpenAI": {}}
	err := a.SaveCodexProfile(reserved)
	if err == nil || err.Error() != "Provider name 'OpenAI' is reserved" {
		t.Fatalf("reserved provider: %v", err)
	}

	if _, err := a.GetCodexProfile("bad/id"); err == nil || err.Error() != "Invalid profile id" {
		t.Fatalf("invalid id: %v", err)
	}

	profiles, err := a.ListCodexProfiles()
	if err != nil || len(profiles) != 0 {
		t.Fatalf("failed saves must not write files: %v %d", err, len(profiles))
	}
}

// TestCodexCreateDefaultProfile covers the first-run seed and the
// "Profiles already exist" guard.
func TestCodexCreateDefaultProfile(t *testing.T) {
	isolatedHome(t)
	a := &App{}

	profile, err := a.CreateDefaultCodexProfile()
	if err != nil {
		t.Fatalf("CreateDefaultCodexProfile: %v", err)
	}
	if profile.Name != "默认" || profile.ModelProvider != "custom" || profile.Model != "gpt-5.2" {
		t.Fatalf("unexpected default: %+v", profile)
	}
	if _, ok := profile.Providers["custom"]; !ok {
		t.Fatalf("missing custom provider: %+v", profile.Providers)
	}
	if _, err := a.CreateDefaultCodexProfile(); err == nil || err.Error() != "Profiles already exist" {
		t.Fatalf("second create: %v", err)
	}
}

// TestCodexApplyPreservesUnknownConfigKeys is the core read-modify-write
// contract: applying a profile rewrites only the model-related keys while
// unknown top-level keys, sibling feature flags and table structure survive.
func TestCodexApplyPreservesUnknownConfigKeys(t *testing.T) {
	isolatedHome(t)
	a := &App{}

	codexTestWriteConfig(t, `
network_access = "restricted"
model_context_window = 200000

[features]
shell_snapshot = true

[projects."/work/other"]
trust_level = "trusted"
`)

	profile := codexTestCustomProfile("p1", "P1")
	if err := a.SaveCodexProfile(profile); err != nil {
		t.Fatalf("SaveCodexProfile: %v", err)
	}
	if err := a.ApplyCodexProfile("p1"); err != nil {
		t.Fatalf("ApplyCodexProfile: %v", err)
	}

	config := codexTestParseConfig(t)
	if config["network_access"] != "restricted" {
		t.Fatalf("unknown key lost: %v", config["network_access"])
	}
	// Profile has no context window → the stale top-level value is removed.
	if _, ok := config["model_context_window"]; ok {
		t.Fatalf("stale context window leaked: %v", config["model_context_window"])
	}
	if config["model_provider"] != "custom" || config["model"] != "gpt-5.5" {
		t.Fatalf("model keys: %v / %v", config["model_provider"], config["model"])
	}
	if config["model_reasoning_effort"] != "high" { // provider effort wins over profile "medium"
		t.Fatalf("effort: %v", config["model_reasoning_effort"])
	}
	features, ok := config["features"].(map[string]any)
	if !ok {
		t.Fatalf("features not a table: %T", config["features"])
	}
	if features["shell_snapshot"] != true || features["api_key_model_discovery"] != true {
		t.Fatalf("features: %+v", features)
	}
	projects, ok := config["projects"].(map[string]any)
	if !ok || projects["/work/other"] == nil {
		t.Fatalf("projects table lost: %+v", config["projects"])
	}

	providers, ok := config["model_providers"].(map[string]any)
	if !ok {
		t.Fatalf("model_providers missing: %T", config["model_providers"])
	}
	custom, ok := providers["custom"].(map[string]any)
	if !ok {
		t.Fatalf("custom provider missing: %+v", providers)
	}
	if custom["experimental_bearer_token"] != "sk-provider" {
		t.Fatalf("bearer token: %v", custom["experimental_bearer_token"])
	}
	if _, ok := custom["requires_openai_auth"]; ok {
		t.Fatalf("requires_openai_auth must never be written: %+v", custom)
	}
	if custom["base_url"] != "https://api.example.com/v1" || custom["wire_api"] != "responses" {
		t.Fatalf("provider fields: %+v", custom)
	}

	// Active profile recorded; read_current backfills the active provider.
	active, err := a.GetActiveCodexProfileId()
	if err != nil || active == nil || *active != "p1" {
		t.Fatalf("active: %v %v", err, active)
	}
	current, err := a.ReadCodexCurrentConfig()
	if err != nil {
		t.Fatalf("ReadCodexCurrentConfig: %v", err)
	}
	if current.ModelProvider != "custom" || current.Model != "gpt-5.5" || !current.APIKeyModelDiscovery {
		t.Fatalf("current config: %+v", current)
	}
	if current.Providers["custom"].APIKey == nil || *current.Providers["custom"].APIKey != "sk-provider" {
		t.Fatalf("active provider api key not backfilled: %+v", current.Providers["custom"])
	}
}

// TestCodexApplyOpenAIModeStripsProvidersAndAuthKey covers the official mode:
// custom providers never reach live config, and OPENAI_API_KEY is stripped
// from auth.json (deleting it when nothing else remains — Codex rejects `{}`).
func TestCodexApplyOpenAIModeStripsProvidersAndAuthKey(t *testing.T) {
	isolatedHome(t)
	a := &App{}

	codexTestWriteConfig(t, "model_provider = \"custom\"\n\n[model_providers.custom]\nname = \"Custom\"\n")

	authPath, err := codexAuthPath()
	if err != nil {
		t.Fatalf("codexAuthPath: %v", err)
	}
	// BYOK-only auth: stripping the key leaves nothing → file deleted.
	if err := os.WriteFile(authPath, []byte(`{"OPENAI_API_KEY":"sk-live"}`), 0o644); err != nil {
		t.Fatalf("write auth: %v", err)
	}

	profile := CodexProfile{ID: "official", Name: "Official", ModelProvider: "openai", Model: "gpt-5"}
	if err := a.SaveCodexProfile(profile); err != nil {
		t.Fatalf("SaveCodexProfile: %v", err)
	}
	if err := a.ApplyCodexProfile("official"); err != nil {
		t.Fatalf("ApplyCodexProfile: %v", err)
	}

	config := codexTestParseConfig(t)
	if _, ok := config["model_providers"]; ok {
		t.Fatalf("openai mode must not keep model_providers: %+v", config["model_providers"])
	}
	if config["model_provider"] != "openai" {
		t.Fatalf("model_provider: %v", config["model_provider"])
	}
	if _, err := os.Stat(authPath); !os.IsNotExist(err) {
		t.Fatalf("empty auth.json must be deleted, got err=%v", err)
	}
}

// TestCodexApplyOpenAIKeepsOfficialAuthFields: official auth (auth_mode …)
// survives the key strip without being deleted.
func TestCodexApplyOpenAIKeepsOfficialAuthFields(t *testing.T) {
	isolatedHome(t)
	a := &App{}

	authPath, err := codexAuthPath()
	if err != nil {
		t.Fatalf("codexAuthPath: %v", err)
	}
	official := `{"auth_mode":"chatgpt","OPENAI_API_KEY":"sk-live","tokens":{"access_token":"a"}}`
	if err := os.WriteFile(authPath, []byte(official), 0o644); err != nil {
		t.Fatalf("write auth: %v", err)
	}

	profile := CodexProfile{ID: "official", Name: "Official", ModelProvider: "openai", Model: "gpt-5"}
	if err := a.SaveCodexProfile(profile); err != nil {
		t.Fatalf("SaveCodexProfile: %v", err)
	}
	if err := a.ApplyCodexProfile("official"); err != nil {
		t.Fatalf("ApplyCodexProfile: %v", err)
	}

	auth, err := codexReadJSONObject(authPath)
	if err != nil {
		t.Fatalf("read auth: %v", err)
	}
	if _, ok := auth["OPENAI_API_KEY"]; ok {
		t.Fatalf("OPENAI_API_KEY not stripped: %+v", auth)
	}
	if auth["auth_mode"] != "chatgpt" {
		t.Fatalf("official fields lost: %+v", auth)
	}
}

// TestCodexReadCurrentConfigBackfillsContextWindow mirrors the Rust test:
// top-level model_context_window / model_auto_compact_token_limit land in the
// active provider entry, and OPENAI_API_KEY from auth.json backfills apiKey.
func TestCodexReadCurrentConfigBackfillsContextWindow(t *testing.T) {
	isolatedHome(t)
	a := &App{}

	codexTestWriteConfig(t, `model_provider = "custom"
model = "gpt-5.6-sol"
model_context_window = 1000000
model_auto_compact_token_limit = 900000

[model_providers.custom]
name = "Custom"
base_url = "https://api.example.com/v1"
wire_api = "responses"
`)
	authPath, err := codexAuthPath()
	if err != nil {
		t.Fatalf("codexAuthPath: %v", err)
	}
	if err := os.WriteFile(authPath, []byte(`{"OPENAI_API_KEY":"  sk-auth  "}`), 0o644); err != nil {
		t.Fatalf("write auth: %v", err)
	}

	current, err := a.ReadCodexCurrentConfig()
	if err != nil {
		t.Fatalf("ReadCodexCurrentConfig: %v", err)
	}
	if current.ModelProvider != "custom" || current.Model != "gpt-5.6-sol" {
		t.Fatalf("model keys: %+v", current)
	}
	provider := current.Providers["custom"]
	if provider.ModelContextWindow == nil || *provider.ModelContextWindow != 1000000 {
		t.Fatalf("context window: %+v", provider)
	}
	if provider.ModelAutoCompactTokenLimit == nil || *provider.ModelAutoCompactTokenLimit != 900000 {
		t.Fatalf("auto compact: %+v", provider)
	}
	if provider.APIKey == nil || *provider.APIKey != "sk-auth" {
		t.Fatalf("api key not trimmed/backfilled: %+v", provider)
	}
	if current.APIKey == nil || *current.APIKey != "sk-auth" {
		t.Fatalf("top-level api key: %+v", current.APIKey)
	}

	// Missing config → Rust defaults (openai / empty), no error.
	if err := os.Remove(codexTestConfigPath(t)); err != nil {
		t.Fatalf("remove config: %v", err)
	}
	if err := os.Remove(authPath); err != nil {
		t.Fatalf("remove auth: %v", err)
	}
	current, err = a.ReadCodexCurrentConfig()
	if err != nil {
		t.Fatalf("ReadCodexCurrentConfig(empty): %v", err)
	}
	if current.ModelProvider != "openai" || current.Model != "" || current.APIKey != nil {
		t.Fatalf("defaults: %+v", current)
	}
}

// TestCodexGetConfigStatus reports existence + paths for both live files.
func TestCodexGetConfigStatus(t *testing.T) {
	isolatedHome(t)
	a := &App{}

	status, err := a.GetCodexConfigStatus()
	if err != nil {
		t.Fatalf("GetCodexConfigStatus: %v", err)
	}
	if status.AuthExists || status.ConfigExists {
		t.Fatalf("expected both missing: %+v", status)
	}
	if !strings.HasSuffix(status.ConfigPath, filepath.Join(".codex", "config.toml")) {
		t.Fatalf("config path: %q", status.ConfigPath)
	}

	codexTestWriteConfig(t, "model = \"gpt-5\"\n")
	status, err = a.GetCodexConfigStatus()
	if err != nil || !status.ConfigExists || status.AuthExists {
		t.Fatalf("after write: %v %+v", err, status)
	}
}

// TestCodexProviderToTOML covers the name fallback and bearer-token rules
// straight from the Rust unit tests.
func TestCodexProviderToTOML(t *testing.T) {
	// Missing/blank name falls back to the provider id (Codex rejects "").
	for _, name := range []*string{nil, strPtr(""), strPtr("   ")} {
		table := codexProviderToTOML("custom", CodexProviderConfig{
			Name:    name,
			BaseURL: strPtr("https://example.com"),
		})
		if table["name"] != "custom" {
			t.Fatalf("name fallback for %v: %v", name, table["name"])
		}
		if _, ok := table["requires_openai_auth"]; ok {
			t.Fatalf("requires_openai_auth must never be written: %+v", table)
		}
	}

	// Non-blank name is kept.
	table := codexProviderToTOML("custom", CodexProviderConfig{Name: strPtr("My Provider")})
	if table["name"] != "My Provider" {
		t.Fatalf("name: %v", table["name"])
	}

	// Bearer token wins over env_key (they must not coexist).
	table = codexProviderToTOML("custom", CodexProviderConfig{
		EnvKey:             strPtr("EXAMPLE_API_KEY"),
		EnvKeyInstructions: strPtr("Set EXAMPLE_API_KEY"),
		APIKey:             strPtr("  sk-test  "),
	})
	if table["experimental_bearer_token"] != "sk-test" {
		t.Fatalf("trimmed bearer token: %v", table["experimental_bearer_token"])
	}
	if _, ok := table["env_key"]; ok {
		t.Fatalf("env_key must be skipped with a bearer token: %+v", table)
	}

	// Empty/whitespace key → omitted, env_key kept.
	table = codexProviderToTOML("custom", CodexProviderConfig{
		EnvKey:  strPtr("EXAMPLE_API_KEY"),
		APIKey:  strPtr("   "),
		WireAPI: strPtr("responses"),
	})
	if _, ok := table["experimental_bearer_token"]; ok {
		t.Fatalf("empty bearer token must be omitted: %+v", table)
	}
	if table["env_key"] != "EXAMPLE_API_KEY" {
		t.Fatalf("env_key: %v", table["env_key"])
	}
}

// TestCodexResolveActiveProviderKeepsOpenAI mirrors the Rust test: the
// built-in openai provider never falls back to a custom entry.
func TestCodexResolveActiveProviderKeepsOpenAI(t *testing.T) {
	profile := codexTestCustomProfile("p1", "P1")
	profile.ModelProvider = "openai"
	id, config := codexResolveActiveProvider(profile)
	if id != "openai" || config != nil {
		t.Fatalf("expected bare openai, got %q %+v", id, config)
	}

	// Unknown model_provider with providers present → first provider id.
	profile.ModelProvider = "ghost"
	id, config = codexResolveActiveProvider(profile)
	if id != "custom" || config == nil {
		t.Fatalf("expected fallback to custom, got %q %+v", id, config)
	}

	// No providers → bare id, nil config.
	profile.Providers = map[string]CodexProviderConfig{}
	id, config = codexResolveActiveProvider(profile)
	if id != "ghost" || config != nil {
		t.Fatalf("expected bare id, got %q %+v", id, config)
	}
}

// TestCodexModelCatalogSync covers the DeepSeek/MiMo catalog files plus the
// legacy models.json cleanup, and that non-catalog models reference nothing.
func TestCodexModelCatalogSync(t *testing.T) {
	isolatedHome(t)
	a := &App{}

	// Legacy single-file catalog written by an older release.
	codexDir, err := codexConfigDir()
	if err != nil {
		t.Fatalf("codexConfigDir: %v", err)
	}
	legacy := filepath.Join(codexDir, "models.json")
	if err := os.WriteFile(legacy, []byte(`{"models":[]}`), 0o644); err != nil {
		t.Fatalf("write legacy: %v", err)
	}

	profile := codexTestCustomProfile("deepseek", "DS")
	provider := profile.Providers["custom"]
	provider.Model = strPtr("deepseek-v4-flash")
	profile.Providers["custom"] = provider
	profile.Model = "deepseek-v4-flash"
	if err := a.SaveCodexProfile(profile); err != nil {
		t.Fatalf("SaveCodexProfile: %v", err)
	}
	if err := a.ApplyCodexProfile("deepseek"); err != nil {
		t.Fatalf("ApplyCodexProfile: %v", err)
	}

	catalog := filepath.Join(codexDir, "model-catalogs", "deepseek.json")
	content, err := os.ReadFile(catalog)
	if err != nil {
		t.Fatalf("read catalog: %v", err)
	}
	if !strings.Contains(string(content), `"slug": "deepseek-flash"`) {
		t.Fatalf("catalog content unexpected (len=%d)", len(content))
	}
	if _, err := os.Stat(legacy); !os.IsNotExist(err) {
		t.Fatalf("legacy models.json must be removed, err=%v", err)
	}
	config := codexTestParseConfig(t)
	if config["model_catalog_json"] != "~/.codex/model-catalogs/deepseek.json" {
		t.Fatalf("model_catalog_json: %v", config["model_catalog_json"])
	}
	if _, ok := config["web_search"]; ok {
		t.Fatalf("deepseek must not disable web search: %v", config["web_search"])
	}

	// MiMo writes its own catalog and disables web search.
	mimo := codexTestCustomProfile("mimo", "MiMo")
	mimoProvider := mimo.Providers["custom"]
	mimoProvider.Model = strPtr("mimo-v2.5")
	mimo.Providers["custom"] = mimoProvider
	mimo.Model = "mimo-v2.5"
	if err := a.SaveCodexProfile(mimo); err != nil {
		t.Fatalf("SaveCodexProfile(mimo): %v", err)
	}
	if err := a.ApplyCodexProfile("mimo"); err != nil {
		t.Fatalf("ApplyCodexProfile(mimo): %v", err)
	}
	if _, err := os.Stat(filepath.Join(codexDir, "model-catalogs", "mimo.json")); err != nil {
		t.Fatalf("mimo catalog: %v", err)
	}
	config = codexTestParseConfig(t)
	if config["web_search"] != "disabled" {
		t.Fatalf("web_search for mimo: %v", config["web_search"])
	}

	// Plain model: catalog reference removed, family catalogs kept on disk.
	plain := codexTestCustomProfile("plain", "Plain")
	plainProvider := plain.Providers["custom"]
	plainProvider.Model = strPtr("gpt-5")
	plain.Providers["custom"] = plainProvider
	plain.Model = "gpt-5"
	if err := a.SaveCodexProfile(plain); err != nil {
		t.Fatalf("SaveCodexProfile(plain): %v", err)
	}
	if err := a.ApplyCodexProfile("plain"); err != nil {
		t.Fatalf("ApplyCodexProfile(plain): %v", err)
	}
	config = codexTestParseConfig(t)
	if _, ok := config["model_catalog_json"]; ok {
		t.Fatalf("non-catalog model must not reference a catalog: %v", config["model_catalog_json"])
	}
	if _, err := os.Stat(catalog); err != nil {
		t.Fatalf("existing catalogs must be kept: %v", err)
	}
}

// TestCodexCatalogForModel covers family detection including trimming.
func TestCodexCatalogForModel(t *testing.T) {
	cases := map[string]codexModelCatalog{
		"deepseek-flash":    codexCatalogDeepSeek,
		"deepseek-v4-flash": codexCatalogDeepSeek,
		"deepseek-v4-pro":   codexCatalogDeepSeek,
		"  deepseek-flash ": codexCatalogDeepSeek,
		"mimo-v2.5":         codexCatalogMimo,
		"mimo-v2.5-pro":     codexCatalogMimo,
	}
	for model, want := range cases {
		got, ok := codexCatalogForModel(model)
		if !ok || got != want {
			t.Fatalf("catalog(%q) = %v,%v want %v", model, got, ok, want)
		}
	}
	for _, model := range []string{"gpt-5", "deepseek-chat", "", "  "} {
		if _, ok := codexCatalogForModel(model); ok {
			t.Fatalf("catalog(%q) unexpected match", model)
		}
	}
}

// TestCodexCliCapabilityParsing mirrors the Rust capability tests: --config
// support needs both the help flag and version >= 0.128.0.
func TestCodexCliCapabilityParsing(t *testing.T) {
	capability := codexParseCliCapability(
		"Usage: codex\n  -c, --config <key=value>\n",
		"codex-cli 0.128.0\n",
	)
	if !capability.SupportsConfigOverride || capability.Version != "codex-cli 0.128.0" {
		t.Fatalf("capability: %+v", capability)
	}

	tooOld := codexParseCliCapability(
		"Usage: codex\n  -c, --config <key=value>\n",
		"codex-cli 0.127.9\n",
	)
	if tooOld.SupportsConfigOverride {
		t.Fatalf("0.127.9 must not support --config: %+v", tooOld)
	}

	noFlag := codexParseCliCapability("Usage: codex\n", "codex-cli 0.999.0\n")
	if noFlag.SupportsConfigOverride {
		t.Fatalf("missing help flag must not support --config: %+v", noFlag)
	}

	// Multi-segment numeric comparison (never lexicographic).
	if !codexVersionAtLeast("codex-cli 0.128.1", "0.128.0") ||
		!codexVersionAtLeast("codex-cli 0.129.0", "0.128.0") ||
		codexVersionAtLeast("codex-cli 0.127.9", "0.128.0") ||
		!codexVersionAtLeast("1.0.0", "0.999.999") {
		t.Fatal("version comparison broken")
	}
}

// TestCodexBuildTemporaryRunPlanSnapshotsRuntimeHome covers the zero-write
// launch plan: runtime CODEX_HOME with the live config merged, CODEX_HOME
// env, sorted unset env, and no auth snapshot for custom providers.
func TestCodexBuildTemporaryRunPlanSnapshotsRuntimeHome(t *testing.T) {
	isolatedHome(t)
	a := &App{}

	codexTestWriteConfig(t, "network_access = \"restricted\"\n")

	profile := codexTestCustomProfile("p1", "P1")
	if err := a.SaveCodexProfile(profile); err != nil {
		t.Fatalf("SaveCodexProfile: %v", err)
	}
	plan, err := codexBuildTemporaryRunPlan(profile)
	if err != nil {
		t.Fatalf("codexBuildTemporaryRunPlan: %v", err)
	}

	if plan.program != "codex" || len(plan.args) != 0 {
		t.Fatalf("program/args: %+v", plan)
	}
	if len(plan.env) != 1 || plan.env[0][0] != "CODEX_HOME" {
		t.Fatalf("env: %+v", plan.env)
	}
	runtimeHome := plan.env[0][1]
	if !strings.Contains(runtimeHome, filepath.Join(".droidgear", "runtime", "codex")) ||
		!strings.Contains(runtimeHome, "temporary-run-") {
		t.Fatalf("runtime home: %q", runtimeHome)
	}
	if plan.runtimeHome != runtimeHome {
		t.Fatalf("runtimeHome mismatch: %q %q", plan.runtimeHome, runtimeHome)
	}
	wantUnset := []string{"EXAMPLE_API_KEY", "OPENAI_API_KEY"}
	if len(plan.unsetEnv) != len(wantUnset) {
		t.Fatalf("unset env: %+v", plan.unsetEnv)
	}
	for i, key := range wantUnset {
		if plan.unsetEnv[i] != key {
			t.Fatalf("unset env not sorted: %+v", plan.unsetEnv)
		}
	}

	// Runtime config: live keys preserved + profile applied.
	data, err := os.ReadFile(filepath.Join(runtimeHome, "config.toml"))
	if err != nil {
		t.Fatalf("read runtime config: %v", err)
	}
	var runtime map[string]any
	if err := toml.Unmarshal(data, &runtime); err != nil {
		t.Fatalf("parse runtime config: %v", err)
	}
	if runtime["network_access"] != "restricted" || runtime["model_provider"] != "custom" {
		t.Fatalf("runtime config: %+v", runtime)
	}
	providers, _ := runtime["model_providers"].(map[string]any)
	custom, _ := providers["custom"].(map[string]any)
	if custom["experimental_bearer_token"] != "sk-provider" {
		t.Fatalf("runtime provider: %+v", custom)
	}

	// Custom provider → no auth.json snapshot in the runtime home.
	if _, err := os.Stat(filepath.Join(runtimeHome, "auth.json")); !os.IsNotExist(err) {
		t.Fatalf("runtime auth must not exist for custom providers, err=%v", err)
	}
	// Live config untouched (zero-write contract).
	live, err := os.ReadFile(codexTestConfigPath(t))
	if err != nil || !strings.Contains(string(live), "network_access") || strings.Contains(string(live), "model_provider") {
		t.Fatalf("live config mutated: %v %q", err, live)
	}
}

// TestCodexRuntimeSnapshotOfficialAuth covers the official-mode auth
// snapshot: portable auth is copied (minus OPENAI_API_KEY), keyring-only
// auth fails with the exact Rust message.
func TestCodexRuntimeSnapshotOfficialAuth(t *testing.T) {
	isolatedHome(t)

	authPath, err := codexAuthPath()
	if err != nil {
		t.Fatalf("codexAuthPath: %v", err)
	}
	// Portable official auth (has tokens + auth_mode).
	portable := `{"auth_mode":"chatgpt","OPENAI_API_KEY":"sk-live","tokens":{"access_token":"a","refresh_token":"r"}}`
	if err := os.WriteFile(authPath, []byte(portable), 0o644); err != nil {
		t.Fatalf("write auth: %v", err)
	}
	profile := CodexProfile{ID: "official", Name: "Official", ModelProvider: "openai", Model: "gpt-5"}
	plan, err := codexBuildTemporaryRunPlan(profile)
	if err != nil {
		t.Fatalf("plan(portable): %v", err)
	}
	data, err := os.ReadFile(filepath.Join(plan.runtimeHome, "auth.json"))
	if err != nil {
		t.Fatalf("read runtime auth: %v", err)
	}
	if strings.Contains(string(data), "OPENAI_API_KEY") || !strings.Contains(string(data), "auth_mode") {
		t.Fatalf("runtime auth content: %q", data)
	}

	// Keyring-only auth (no portable fields) → exact Rust error.
	if err := os.WriteFile(authPath, []byte(`{"OPENAI_API_KEY":"sk-live"}`), 0o644); err != nil {
		t.Fatalf("write auth: %v", err)
	}
	_, err = codexBuildTemporaryRunPlan(profile)
	if err == nil || !strings.Contains(err.Error(), "could not snapshot official auth") {
		t.Fatalf("keyring auth must fail, got: %v", err)
	}
}

// TestCodexCleanupStaleRuntimeHomes removes only old temporary-run dirs.
func TestCodexCleanupStaleRuntimeHomes(t *testing.T) {
	isolatedHome(t)

	runtimeDir, err := codexRuntimeDir()
	if err != nil {
		t.Fatalf("codexRuntimeDir: %v", err)
	}
	stale := filepath.Join(runtimeDir, "temporary-run-20000101T000000.000Z-stale")
	fresh := filepath.Join(runtimeDir, "temporary-run-keep")
	other := filepath.Join(runtimeDir, "notes")
	for _, dir := range []string{stale, fresh, other} {
		if err := os.MkdirAll(dir, 0o755); err != nil {
			t.Fatalf("mkdir: %v", err)
		}
	}
	old := time.Now().Add(-48 * time.Hour)
	if err := os.Chtimes(stale, old, old); err != nil {
		t.Fatalf("chtimes: %v", err)
	}

	codexCleanupStaleRuntimeHomes()

	if _, err := os.Stat(stale); !os.IsNotExist(err) {
		t.Fatalf("stale dir must be removed, err=%v", err)
	}
	if _, err := os.Stat(fresh); err != nil {
		t.Fatalf("fresh dir must stay: %v", err)
	}
	if _, err := os.Stat(other); err != nil {
		t.Fatalf("unrelated dir must stay: %v", err)
	}
}

// TestCodexLaunchWrappersCoverEnvUnsetCwd verifies the generated wrapper
// scripts carry env, unset and cwd — the payload the Windows launchers run.
func TestCodexLaunchWrappersCoverEnvUnsetCwd(t *testing.T) {
	supportDir := t.TempDir()
	cwd := filepath.Join(t.TempDir(), "work tree")
	spec := codexLaunchSpec{
		program:    "codex.exe",
		args:       []string{"resume"},
		env:        [][2]string{{"CODEX_HOME", filepath.Join(supportDir, "runtime")}},
		unsetEnv:   []string{"OPENAI_API_KEY"},
		cwd:        &cwd,
		supportDir: supportDir,
	}

	psPath, err := codexWriteSecurePSWrapper(spec, spec.program)
	if err != nil {
		t.Fatalf("codexWriteSecurePSWrapper: %v", err)
	}
	ps, err := os.ReadFile(psPath)
	if err != nil {
		t.Fatalf("read ps wrapper: %v", err)
	}
	psText := string(ps)
	for _, want := range []string{"$env:CODEX_HOME", "Remove-Item Env:OPENAI_API_KEY", "Set-Location", "& 'codex.exe' 'resume'", "$PSCommandPath"} {
		if !strings.Contains(psText, want) {
			t.Fatalf("ps wrapper missing %q:\n%s", want, psText)
		}
	}

	cmdPath, err := codexWriteSecureCmdWrapper(spec, spec.program)
	if err != nil {
		t.Fatalf("codexWriteSecureCmdWrapper: %v", err)
	}
	cmd, err := os.ReadFile(cmdPath)
	if err != nil {
		t.Fatalf("read cmd wrapper: %v", err)
	}
	cmdText := string(cmd)
	for _, want := range []string{`set "CODEX_HOME=`, "set OPENAI_API_KEY=", "cd /d ", `"codex.exe" "resume"`, `del /f /q "%~f0"`} {
		if !strings.Contains(cmdText, want) {
			t.Fatalf("cmd wrapper missing %q:\n%s", want, cmdText)
		}
	}
}

// TestCodexResolveWindowsProgramCandidates covers PATHEXT candidate building
// without touching the real PATH (pure suffix logic).
func TestCodexResolveWindowsProgramCandidates(t *testing.T) {
	// Absolute / path-like inputs pass through untouched.
	if got := codexResolveWindowsProgram(`C:\tools\codex.exe`); got != `C:\tools\codex.exe` {
		t.Fatalf("absolute: %q", got)
	}
	if got := codexResolveWindowsProgram(`sub\codex`); got != `sub\codex` {
		t.Fatalf("relative path: %q", got)
	}
}
