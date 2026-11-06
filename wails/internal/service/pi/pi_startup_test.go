package pi

import (
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"droidgear-wails/internal/testutil"
)

// seedModelsJSON writes a live models.json document (the file ApplyProfile
// merges over).
func seedModelsJSON(t *testing.T, document string) {
	t.Helper()
	path, err := piConfigPath()
	if err != nil {
		t.Fatalf("piConfigPath: %v", err)
	}
	if err := os.WriteFile(path, []byte(document), 0o600); err != nil {
		t.Fatalf("seed models.json: %v", err)
	}
}

// seedCatalog writes the cached pi catalog (models-store.json), which is what
// keeps catalog-only providers out of the repair path.
func seedCatalog(t *testing.T, providers map[string][]string) {
	t.Helper()
	dir, err := piConfigDir()
	if err != nil {
		t.Fatalf("piConfigDir: %v", err)
	}
	store := map[string]any{}
	for providerID, modelIDs := range providers {
		models := make([]map[string]any, 0, len(modelIDs))
		for _, id := range modelIDs {
			models = append(models, map[string]any{"id": id})
		}
		store[providerID] = map[string]any{"models": models}
	}
	data, err := json.Marshal(store)
	if err != nil {
		t.Fatalf("marshal catalog: %v", err)
	}
	path := filepath.Join(dir, "models-store.json")
	if err := os.WriteFile(path, data, 0o600); err != nil {
		t.Fatalf("seed models-store.json: %v", err)
	}
}

func seedSettings(t *testing.T, provider, model, level string) {
	t.Helper()
	settings := PiSettings{}
	if provider != "" {
		settings.DefaultProvider = &provider
	}
	if model != "" {
		settings.DefaultModel = &model
	}
	if level != "" {
		settings.DefaultThinkingLevel = &level
	}
	if err := SaveSettings(settings); err != nil {
		t.Fatalf("SaveSettings: %v", err)
	}
}

func modelWithLevels(id string, levels ...string) PiModel {
	mapped := map[string]*string{}
	for _, level := range levels {
		value := level
		mapped[level] = &value
	}
	return PiModel{ID: id, Reasoning: true, ThinkingLevelMap: mapped}
}

// TestApplyProfileRepointsRemovedStartupProvider is the bug this pass fixes:
// the profile no longer carries the provider the startup model points at, pi
// cannot resolve it and silently falls back, so apply re-points settings.json
// at the first provider/model the profile supplies.
func TestApplyProfileRepointsRemovedStartupProvider(t *testing.T) {
	testutil.IsolatedHome(t)

	seedModelsJSON(t, `{
  "providers": {
    "wududu": {
      "baseUrl": "https://sub.sup.com/v1",
      "api": "openai-completions",
      "models": [{ "id": "deepseek-v4.1-flash" }]
    }
  }
}`)
	seedSettings(t, "wududu", "deepseek-v4.1-flash", "high")

	profile := PiProfile{
		ID:   "corp",
		Name: "Corp",
		Providers: map[string]PiProviderConfig{
			"corp-proxy": {
				Models: []PiModel{{ID: "corp-large"}, {ID: "corp-small"}},
			},
		},
	}
	if err := SaveProfile(profile); err != nil {
		t.Fatalf("SaveProfile: %v", err)
	}

	result, err := ApplyProfile(profile.ID)
	if err != nil {
		t.Fatalf("ApplyProfile: %v", err)
	}
	if !result.StartupAdjusted {
		t.Fatalf("StartupAdjusted = false, want true (%+v)", result)
	}
	if len(result.StartupReasons) == 0 {
		t.Fatalf("no reasons recorded for the repair")
	}
	if got := optionalValue(result.Startup.DefaultProvider); got != "corp-proxy" {
		t.Fatalf("defaultProvider = %q, want corp-proxy", got)
	}
	if got := optionalValue(result.Startup.DefaultModel); got != "corp-large" {
		t.Fatalf("defaultModel = %q, want corp-large", got)
	}
	if result.Startup.DefaultThinkingLevel != nil {
		t.Fatalf("thinking level kept across a model change: %v", *result.Startup.DefaultThinkingLevel)
	}

	status, err := ReadSettings()
	if err != nil {
		t.Fatalf("ReadSettings: %v", err)
	}
	if got := optionalValue(status.DefaultProvider); got != "corp-proxy" {
		t.Fatalf("settings.json defaultProvider = %q, want corp-proxy", got)
	}
	if got := optionalValue(status.DefaultModel); got != "corp-large" {
		t.Fatalf("settings.json defaultModel = %q, want corp-large", got)
	}
}

// TestApplyProfileKeepsCatalogStartupProvider: a startup model served by pi's
// own catalog is not the profile's business, so apply must leave it alone even
// when the profile drops every models.json provider.
func TestApplyProfileKeepsCatalogStartupProvider(t *testing.T) {
	testutil.IsolatedHome(t)

	seedCatalog(t, map[string][]string{"deepseek": {"deepseek-flash"}})
	seedModelsJSON(t, `{
  "providers": {
    "wududu": { "models": [{ "id": "deepseek-v4.1-flash" }] }
  }
}`)
	seedSettings(t, "deepseek", "deepseek-flash", "high")

	profile := PiProfile{ID: "corp", Name: "Corp", Providers: map[string]PiProviderConfig{}}
	if err := SaveProfile(profile); err != nil {
		t.Fatalf("SaveProfile: %v", err)
	}
	result, err := ApplyProfile(profile.ID)
	if err != nil {
		t.Fatalf("ApplyProfile: %v", err)
	}
	if result.StartupAdjusted {
		t.Fatalf("catalog selection was rewritten: %+v", result.StartupReasons)
	}
	status, err := ReadSettings()
	if err != nil {
		t.Fatalf("ReadSettings: %v", err)
	}
	if got := optionalValue(status.DefaultProvider); got != "deepseek" {
		t.Fatalf("defaultProvider = %q, want deepseek", got)
	}
	if got := optionalValue(status.DefaultThinkingLevel); got != "high" {
		t.Fatalf("defaultThinkingLevel = %q, want high", got)
	}
}

// TestApplyProfileClearsUnknownThinkingLevel: thinking levels are model
// specific, so a level the (unchanged) model cannot express is dropped back to
// automatic instead of being written to a provider that ignores it.
func TestApplyProfileClearsUnknownThinkingLevel(t *testing.T) {
	testutil.IsolatedHome(t)

	seedModelsJSON(t, `{ "providers": { "proxy": { "models": [{ "id": "m1" }] } } }`)
	seedSettings(t, "proxy", "m1", "xhigh")

	profile := PiProfile{
		ID:   "corp",
		Name: "Corp",
		Providers: map[string]PiProviderConfig{
			"proxy": {Models: []PiModel{modelWithLevels("m1", "off", "high")}},
		},
	}
	if err := SaveProfile(profile); err != nil {
		t.Fatalf("SaveProfile: %v", err)
	}
	result, err := ApplyProfile(profile.ID)
	if err != nil {
		t.Fatalf("ApplyProfile: %v", err)
	}
	if !result.StartupAdjusted {
		t.Fatalf("unsupported thinking level kept: %+v", result.Startup)
	}
	if result.Startup.DefaultThinkingLevel != nil {
		t.Fatalf("defaultThinkingLevel = %q, want cleared", *result.Startup.DefaultThinkingLevel)
	}
}

// TestListStartupOptionsMergesLiveAndCatalog pins the candidate list the GUI
// pickers, `dg pi settings candidates` and the TUI Defaults screen share.
func TestListStartupOptionsMergesLiveAndCatalog(t *testing.T) {
	testutil.IsolatedHome(t)

	seedCatalog(t, map[string][]string{
		"deepseek": {"deepseek-flash"},
		"proxy":    {"stale"},
	})
	seedModelsJSON(t, `{
  "providers": {
    "proxy": { "models": [{ "id": "m1" }, { "id": "m2" }] }
  }
}`)

	options, err := ListStartupOptions()
	if err != nil {
		t.Fatalf("ListStartupOptions: %v", err)
	}
	wantProviders := []string{"deepseek", "proxy"}
	if len(options.Providers) != len(wantProviders) {
		t.Fatalf("providers = %v, want %v", options.Providers, wantProviders)
	}
	for i, want := range wantProviders {
		if options.Providers[i] != want {
			t.Fatalf("providers = %v, want %v", options.Providers, wantProviders)
		}
	}
	// models.json wins over the catalog for the same provider.
	if got := options.Models["proxy"]; len(got) != 2 || got[0] != "m1" || got[1] != "m2" {
		t.Fatalf("proxy models = %v, want [m1 m2]", got)
	}
	if got := options.Models["deepseek"]; len(got) != 1 || got[0] != "deepseek-flash" {
		t.Fatalf("deepseek models = %v, want [deepseek-flash]", got)
	}
}

func TestStartupWarnings(t *testing.T) {
	testutil.IsolatedHome(t)

	seedCatalog(t, map[string][]string{"deepseek": {"deepseek-flash"}})
	seedModelsJSON(t, `{ "providers": { "proxy": { "models": [{ "id": "m1" }] } } }`)

	provider, model := "proxy", "m1"
	if warnings, err := StartupWarnings(PiSettings{DefaultProvider: &provider, DefaultModel: &model}); err != nil {
		t.Fatalf("StartupWarnings: %v", err)
	} else if len(warnings) != 0 {
		t.Fatalf("unexpected warnings for a valid selection: %v", warnings)
	}

	unknownProvider := "nope"
	if warnings, err := StartupWarnings(PiSettings{DefaultProvider: &unknownProvider}); err != nil {
		t.Fatalf("StartupWarnings: %v", err)
	} else if len(warnings) != 1 {
		t.Fatalf("want one warning for an unknown provider, got %v", warnings)
	}

	unknownModel := "nope"
	if warnings, err := StartupWarnings(PiSettings{DefaultProvider: &provider, DefaultModel: &unknownModel}); err != nil {
		t.Fatalf("StartupWarnings: %v", err)
	} else if len(warnings) != 1 {
		t.Fatalf("want one warning for an unknown model, got %v", warnings)
	}

	catalogModel := "deepseek-flash"
	catalogProvider := "deepseek"
	if warnings, err := StartupWarnings(PiSettings{
		DefaultProvider: &catalogProvider,
		DefaultModel:    &catalogModel,
	}); err != nil {
		t.Fatalf("StartupWarnings: %v", err)
	} else if len(warnings) != 0 {
		t.Fatalf("catalog selection flagged: %v", warnings)
	}
}

// seedCatalogLevels writes the cached catalog with per-model thinking level
// maps, the metadata the repair pass uses for catalog-provided models.
func seedCatalogLevels(t *testing.T, providerID string, models map[string]map[string]*string) {
	t.Helper()
	dir, err := piConfigDir()
	if err != nil {
		t.Fatalf("piConfigDir: %v", err)
	}
	entries := make([]map[string]any, 0, len(models))
	for modelID, levels := range models {
		entries = append(entries, map[string]any{"id": modelID, "thinkingLevelMap": levels})
	}
	store := map[string]any{providerID: map[string]any{"models": entries}}
	data, err := json.Marshal(store)
	if err != nil {
		t.Fatalf("marshal catalog: %v", err)
	}
	if err := os.WriteFile(filepath.Join(dir, "models-store.json"), data, 0o600); err != nil {
		t.Fatalf("seed models-store.json: %v", err)
	}
}

// TestApplyProfileClearsCatalogThinkingLevel: models.json can override a catalog
// provider without repeating its thinkingLevelMap, so the level check has to
// consult the cached catalog too.
func TestApplyProfileClearsCatalogThinkingLevel(t *testing.T) {
	testutil.IsolatedHome(t)

	high := "high"
	seedCatalogLevels(t, "deepseek", map[string]map[string]*string{
		"deepseek-flash": {"high": &high, "xhigh": nil, "max": nil},
	})
	seedModelsJSON(t, `{
  "providers": {
    "deepseek": { "models": [{ "id": "deepseek-flash" }] }
  }
}`)
	seedSettings(t, "deepseek", "deepseek-flash", "xhigh")

	profile := PiProfile{
		ID:   "corp",
		Name: "Corp",
		Providers: map[string]PiProviderConfig{
			"deepseek": {Models: []PiModel{{ID: "deepseek-flash"}}},
			"other":    {Models: []PiModel{{ID: "other-1"}}},
		},
	}
	if err := SaveProfile(profile); err != nil {
		t.Fatalf("SaveProfile: %v", err)
	}
	result, err := ApplyProfile(profile.ID)
	if err != nil {
		t.Fatalf("ApplyProfile: %v", err)
	}
	if !result.StartupAdjusted || result.Startup.DefaultThinkingLevel != nil {
		t.Fatalf("catalog thinking level survived: %+v", result.Startup)
	}
	if got := optionalValue(result.Startup.DefaultProvider); got != "deepseek" {
		t.Fatalf("defaultProvider = %q, want deepseek", got)
	}
}

// TestStartupWarningsFlagsUnsupportedThinkingLevel: `pi settings set|show` warns
// instead of writing a level the model cannot express.
func TestStartupWarningsFlagsUnsupportedThinkingLevel(t *testing.T) {
	testutil.IsolatedHome(t)

	high := "high"
	seedCatalogLevels(t, "deepseek", map[string]map[string]*string{
		"deepseek-flash": {"high": &high, "xhigh": nil},
	})
	provider, model, level := "deepseek", "deepseek-flash", "xhigh"
	warnings, err := StartupWarnings(PiSettings{
		DefaultProvider:      &provider,
		DefaultModel:         &model,
		DefaultThinkingLevel: &level,
	})
	if err != nil {
		t.Fatalf("StartupWarnings: %v", err)
	}
	if len(warnings) != 1 || !strings.Contains(warnings[0], "thinking level") {
		t.Fatalf("warnings = %v", warnings)
	}

	supported := "high"
	warnings, err = StartupWarnings(PiSettings{
		DefaultProvider:      &provider,
		DefaultModel:         &model,
		DefaultThinkingLevel: &supported,
	})
	if err != nil {
		t.Fatalf("StartupWarnings: %v", err)
	}
	if len(warnings) != 0 {
		t.Fatalf("supported level flagged: %v", warnings)
	}
}
