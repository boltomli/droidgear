package omp

import (
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

// seedModelCache writes OMP's cached catalog (models.db model_cache), which is
// what tells a custom models.yml provider apart from a built-in one.
func seedModelCache(t *testing.T, home string, providers map[string][]string) {
	t.Helper()
	path := filepath.Join(home, ".omp", "agent", "models.db")
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		t.Fatalf("mkdir: %v", err)
	}
	db, err := sqlOpen(path)
	if err != nil {
		t.Fatalf("open models.db: %v", err)
	}
	defer func() { _ = db.Close() }()
	if _, err := db.Exec(`CREATE TABLE model_cache (provider_id TEXT PRIMARY KEY, models TEXT)`); err != nil {
		t.Fatalf("create model_cache: %v", err)
	}
	for providerID, modelIDs := range providers {
		models := make([]map[string]any, 0, len(modelIDs))
		for _, id := range modelIDs {
			models = append(models, map[string]any{"id": id})
		}
		encoded, err := json.Marshal(models)
		if err != nil {
			t.Fatalf("marshal models: %v", err)
		}
		if _, err := db.Exec(
			`INSERT INTO model_cache (provider_id, models) VALUES (?, ?)`,
			providerID, string(encoded),
		); err != nil {
			t.Fatalf("insert model_cache: %v", err)
		}
	}
}

func writeConfigFile(t *testing.T, home string, contents string) string {
	t.Helper()
	path := filepath.Join(home, ".omp", "agent", "config.yml")
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		t.Fatalf("mkdir: %v", err)
	}
	if err := os.WriteFile(path, []byte(contents), 0o644); err != nil { // #nosec G306 -- test fixture
		t.Fatalf("write config.yml: %v", err)
	}
	return path
}

func mustRoles(t *testing.T) OmpModelRoles {
	t.Helper()
	config, err := readConfigYML()
	if err != nil {
		t.Fatalf("readConfigYML: %v", err)
	}
	if config.ModelRoles == nil {
		return OmpModelRoles{}
	}
	return *config.ModelRoles
}

func roleValue(t *testing.T, roles OmpModelRoles, name string) string {
	t.Helper()
	slot := ompRoleSlot(&roles, name)
	if slot == nil || *slot == nil {
		return ""
	}
	return **slot
}

// TestDeleteCustomProviderRepointsRoles is the OMP twin of the pi startup-model
// repair: deleting the provider a role points at used to leave config.yml
// dangling, and OMP then exits with "No default model selected".
func TestDeleteCustomProviderRepointsRoles(t *testing.T) {
	home := sandbox(t)
	seedModelCache(t, home, map[string][]string{"anthropic": {"claude-fable-5"}})
	writeModelsFile(t, home, `providers:
  custom-a:
    api: openai-completions
    apiKey: sk-a
    models:
      - id: a1
      - id: a2
  custom-b:
    api: openai-completions
    apiKey: sk-b
    models:
      - id: b1
`)
	writeConfigFile(t, home, `modelRoles:
  default: custom-a/a1
  smol: custom-b/b1
  advisor: custom-a/a2
`)

	result, err := DeleteCustomProvider("custom-a")
	if err != nil {
		t.Fatalf("DeleteCustomProvider: %v", err)
	}
	if !result.Adjusted {
		t.Fatalf("roles not repaired: %+v", result)
	}
	if len(result.Adjustments) != 2 {
		t.Fatalf("adjustments = %+v, want default + advisor", result.Adjustments)
	}
	roles := mustRoles(t)
	if got := roleValue(t, roles, "default"); got != "custom-b/b1" {
		t.Errorf("default = %q, want custom-b/b1", got)
	}
	if got := roleValue(t, roles, "advisor"); got != "custom-b/b1" {
		t.Errorf("advisor = %q, want custom-b/b1", got)
	}
	if got := roleValue(t, roles, "smol"); got != "custom-b/b1" {
		t.Errorf("smol = %q, want custom-b/b1 (untouched)", got)
	}
}

// TestDeleteCustomProviderKeepsCatalogProviderRole: a provider OMP's own model
// cache knows survives the removal of its models.yml override, so its roles
// must be left alone.
func TestDeleteCustomProviderKeepsCatalogProviderRole(t *testing.T) {
	home := sandbox(t)
	seedModelCache(t, home, map[string][]string{"anthropic": {"claude-fable-5"}})
	writeModelsFile(t, home, `providers:
  anthropic:
    baseUrl: https://proxy.example.test/v1
    api: openai-completions
    apiKey: sk-proxy
    models:
      - id: claude-fable-5
`)
	writeConfigFile(t, home, `modelRoles:
  default: anthropic/claude-fable-5
`)

	result, err := DeleteCustomProvider("anthropic")
	if err != nil {
		t.Fatalf("DeleteCustomProvider: %v", err)
	}
	if result.Adjusted {
		t.Fatalf("catalog provider role was rewritten: %+v", result.Adjustments)
	}
	if got := roleValue(t, mustRoles(t), "default"); got != "anthropic/claude-fable-5" {
		t.Errorf("default = %q, want the catalog value untouched", got)
	}
}

// TestDeleteCustomProviderSkipsRepairWithoutCatalog: with no models.db there is
// no way to tell a custom provider from a built-in one, so the pass stays out
// of the way instead of guessing.
func TestDeleteCustomProviderSkipsRepairWithoutCatalog(t *testing.T) {
	home := sandbox(t)
	writeModelsFile(t, home, `providers:
  custom-a:
    api: openai-completions
    apiKey: sk-a
    models:
      - id: a1
`)
	writeConfigFile(t, home, `modelRoles:
  default: custom-a/a1
`)

	result, err := DeleteCustomProvider("custom-a")
	if err != nil {
		t.Fatalf("DeleteCustomProvider: %v", err)
	}
	if result.Adjusted {
		t.Fatalf("repaired without a catalog: %+v", result.Adjustments)
	}
	if got := roleValue(t, mustRoles(t), "default"); got != "custom-a/a1" {
		t.Errorf("default = %q, want it untouched", got)
	}
}

// TestSaveCustomProviderRepointsDroppedModel: a provider's model list is
// declared state, so a role pointing at a model the save dropped must move to
// the provider's first model (OMP would otherwise fall back on its own).
func TestSaveCustomProviderRepointsDroppedModel(t *testing.T) {
	home := sandbox(t)
	seedModelCache(t, home, map[string][]string{"anthropic": {"claude-fable-5"}})
	writeModelsFile(t, home, `providers:
  proxy:
    api: openai-completions
    apiKey: sk-proxy
    models:
      - id: m1
      - id: m2
`)
	writeConfigFile(t, home, `modelRoles:
  default: proxy/m2
`)

	result, err := SaveCustomProvider("proxy", OmpCustomProvider{
		BaseURL: strPtr("https://proxy.example.test/v1"),
		API:     strPtr("openai-completions"),
		APIKey:  strPtr("sk-proxy"),
		Models:  []OmpCustomModel{{ID: "m1"}},
	})
	if err != nil {
		t.Fatalf("SaveCustomProvider: %v", err)
	}
	if !result.Adjusted || len(result.Adjustments) != 1 {
		t.Fatalf("adjustments = %+v, want one", result.Adjustments)
	}
	if got := result.Adjustments[0]; got.Role != "default" || got.To == nil || *got.To != "proxy/m1" {
		t.Fatalf("adjustment = %+v, want default -> proxy/m1", got)
	}
	if got := roleValue(t, mustRoles(t), "default"); got != "proxy/m1" {
		t.Errorf("default = %q, want proxy/m1", got)
	}
}

// TestDeleteCustomProviderClearsRoleWithoutCandidates: with nothing left to
// point at, the role is written as null — how OMP records an unassigned role.
func TestDeleteCustomProviderClearsRoleWithoutCandidates(t *testing.T) {
	home := sandbox(t)
	seedModelCache(t, home, map[string][]string{"empty-provider": {}})
	writeModelsFile(t, home, `providers:
  custom-a:
    api: openai-completions
    apiKey: sk-a
    models:
      - id: a1
`)
	writeConfigFile(t, home, `modelRoles:
  default: custom-a/a1
`)

	result, err := DeleteCustomProvider("custom-a")
	if err != nil {
		t.Fatalf("DeleteCustomProvider: %v", err)
	}
	if !result.Adjusted || len(result.Adjustments) != 1 {
		t.Fatalf("adjustments = %+v, want one", result.Adjustments)
	}
	if result.Adjustments[0].To != nil {
		t.Fatalf("role was not cleared: %+v", result.Adjustments[0])
	}
	roles := mustRoles(t)
	if roles.Default != nil {
		t.Fatalf("default = %q, want null", *roles.Default)
	}
}

// TestDeleteCustomProviderRepairsUnknownRole checks the role names this build
// does not model (kept in Extra) go through the same repair.
func TestDeleteCustomProviderRepairsUnknownRole(t *testing.T) {
	home := sandbox(t)
	seedModelCache(t, home, map[string][]string{"anthropic": {"claude-fable-5"}})
	writeModelsFile(t, home, `providers:
  custom-a:
    api: openai-completions
    apiKey: sk-a
    models:
      - id: a1
  custom-b:
    api: openai-completions
    apiKey: sk-b
    models:
      - id: b1
`)
	writeConfigFile(t, home, `modelRoles:
  default: custom-b/b1
  futureRole: custom-a/a1
`)

	result, err := DeleteCustomProvider("custom-a")
	if err != nil {
		t.Fatalf("DeleteCustomProvider: %v", err)
	}
	roles := mustRoles(t)
	raw, ok := roles.Extra["futureRole"]
	if !ok {
		t.Fatalf("futureRole lost: %+v", roles.Extra)
	}
	var value string
	if err := json.Unmarshal(raw, &value); err != nil {
		t.Fatalf("futureRole is not a string: %s", raw)
	}
	if value != "custom-b/b1" {
		t.Errorf("futureRole = %q, want custom-b/b1", value)
	}
	if len(result.Adjustments) != 1 || result.Adjustments[0].Role != "futureRole" {
		t.Errorf("adjustments = %+v, want the futureRole repair", result.Adjustments)
	}
}

// TestListRoleOptionsAndWarnings pins the candidate list and the apply-time
// warnings shared by the GUI pickers, `dg omp roles candidates` and the TUI.
func TestListRoleOptionsAndWarnings(t *testing.T) {
	home := sandbox(t)
	seedModelCache(t, home, map[string][]string{
		"anthropic": {"claude-fable-5"},
		"zai":       {"glm-4.5"},
	})
	writeModelsFile(t, home, `providers:
  proxy:
    api: openai-completions
    apiKey: sk-proxy
    models:
      - id: m2
      - id: m1
`)

	options, err := ListRoleOptions()
	if err != nil {
		t.Fatalf("ListRoleOptions: %v", err)
	}
	// Provider ids are alphabetical; a provider's models keep models.yml order.
	values := RoleValues(options)
	want := []string{"anthropic/claude-fable-5", "proxy/m2", "proxy/m1", "zai/glm-4.5"}
	if strings.Join(values, ",") != strings.Join(want, ",") {
		t.Fatalf("values = %v, want %v", values, want)
	}

	// OMP accepts fuzzy/bare model names, so only a provider-qualified value can
	// be called wrong.
	bareModel := "m1"
	if warnings, err := ValidateModelRoles(OmpModelRoles{Smol: &bareModel}, nil); err != nil {
		t.Fatalf("ValidateModelRoles: %v", err)
	} else if len(warnings) != 0 {
		t.Fatalf("bare model id flagged: %v", warnings)
	}

	staleModel := "proxy/gone"
	warnings, err := ValidateModelRoles(OmpModelRoles{Default: &staleModel}, nil)
	if err != nil {
		t.Fatalf("ValidateModelRoles: %v", err)
	}
	if len(warnings) != 1 || !strings.Contains(warnings[0], "does not list that model") {
		t.Fatalf("warnings = %v, want the missing-model note", warnings)
	}

	missing := "gone/m1"
	warnings, err = ValidateModelRoles(OmpModelRoles{Default: &missing}, nil)
	if err != nil {
		t.Fatalf("ValidateModelRoles: %v", err)
	}
	if len(warnings) != 1 || !strings.Contains(warnings[0], "not in models.yml or OMP's catalog") {
		t.Fatalf("warnings = %v, want the unknown-provider note", warnings)
	}

	valid := "anthropic/claude-fable-5"
	warnings, err = ValidateModelRoles(OmpModelRoles{Default: &valid}, nil)
	if err != nil {
		t.Fatalf("ValidateModelRoles: %v", err)
	}
	if len(warnings) != 0 {
		t.Fatalf("catalog role flagged: %v", warnings)
	}
}

// TestApplyProfileWarnsAboutUnresolvableRoles: apply does not rewrite the
// profile's choice, it reports it.
func TestApplyProfileWarnsAboutUnresolvableRoles(t *testing.T) {
	home := sandbox(t)
	seedModelCache(t, home, map[string][]string{"anthropic": {"claude-fable-5"}})
	writeModelsFile(t, home, `providers:
  proxy:
    api: openai-completions
    apiKey: sk-proxy
    models:
      - id: m1
`)
	profile := OmpProfile{
		ID:   "p1",
		Name: "Broken",
		ModelRoles: OmpModelRoles{
			Default: strPtr("proxy/gone"),
		},
	}
	if err := SaveProfile(profile); err != nil {
		t.Fatalf("SaveProfile: %v", err)
	}

	result, err := ApplyProfile("p1")
	if err != nil {
		t.Fatalf("ApplyProfile: %v", err)
	}
	if len(result.Warnings) != 1 || !strings.Contains(result.Warnings[0], "does not list that model") {
		t.Fatalf("warnings = %v", result.Warnings)
	}
	if got := roleValue(t, mustRoles(t), "default"); got != "proxy/gone" {
		t.Errorf("apply rewrote the profile choice: default = %q", got)
	}
}

// TestDeleteCustomProviderResetsUnsupportedThinking: the thinking level belongs
// to the model the default role points at, so re-pointing it onto a model with a
// narrower effort list resets the level to auto instead of sending an
// unsupported effort.
func TestDeleteCustomProviderResetsUnsupportedThinking(t *testing.T) {
	home := sandbox(t)
	seedModelCache(t, home, map[string][]string{"anthropic": {"claude-fable-5"}})
	writeModelsFile(t, home, `providers:
  custom-a:
    api: openai-completions
    apiKey: sk-a
    models:
      - id: a1
  custom-b:
    api: openai-completions
    apiKey: sk-b
    models:
      - id: b1
        thinking:
          mode: effort
          efforts:
            - low
            - high
`)
	writeConfigFile(t, home, `defaultThinkingLevel: xhigh
modelRoles:
  default: custom-a/a1
`)

	result, err := DeleteCustomProvider("custom-a")
	if err != nil {
		t.Fatalf("DeleteCustomProvider: %v", err)
	}
	if result.Thinking == nil {
		t.Fatalf("thinking level not reset: %+v", result)
	}
	if result.Thinking.From != "xhigh" || result.Thinking.To != "auto" {
		t.Fatalf("thinking = %+v, want xhigh -> auto", result.Thinking)
	}
	config, err := readConfigYML()
	if err != nil {
		t.Fatalf("readConfigYML: %v", err)
	}
	if config.DefaultThinkingLevel == nil || *config.DefaultThinkingLevel != "auto" {
		t.Fatalf("defaultThinkingLevel = %v, want auto", config.DefaultThinkingLevel)
	}
	if got := roleValue(t, mustRoles(t), "default"); got != "custom-b/b1" {
		t.Errorf("default = %q, want custom-b/b1", got)
	}
}

// TestDeleteCustomProviderKeepsSupportedThinking: a level the (unchanged) default
// model still declares is left alone.
func TestDeleteCustomProviderKeepsSupportedThinking(t *testing.T) {
	home := sandbox(t)
	seedModelCache(t, home, map[string][]string{"anthropic": {"claude-fable-5"}})
	writeModelsFile(t, home, `providers:
  custom-a:
    api: openai-completions
    apiKey: sk-a
    models:
      - id: a1
        thinking:
          mode: effort
          efforts: [low, high]
  custom-b:
    api: openai-completions
    apiKey: sk-b
    models:
      - id: b1
        thinking:
          mode: effort
          efforts: [low, medium, high, xhigh]
`)
	writeConfigFile(t, home, `defaultThinkingLevel: xhigh
modelRoles:
  default: custom-a/a1
`)

	result, err := DeleteCustomProvider("custom-a")
	if err != nil {
		t.Fatalf("DeleteCustomProvider: %v", err)
	}
	if result.Thinking != nil {
		t.Fatalf("thinking level reset although the new model supports it: %+v", result.Thinking)
	}
	config, _ := readConfigYML()
	if config.DefaultThinkingLevel == nil || *config.DefaultThinkingLevel != "xhigh" {
		t.Fatalf("defaultThinkingLevel = %v, want xhigh", config.DefaultThinkingLevel)
	}
}

// TestSaveCustomProviderResetsUnsupportedThinking covers the other write path:
// the default model stays, but the save dropped the level it supported.
func TestSaveCustomProviderResetsUnsupportedThinking(t *testing.T) {
	home := sandbox(t)
	seedModelCache(t, home, map[string][]string{"anthropic": {"claude-fable-5"}})
	writeModelsFile(t, home, `providers:
  proxy:
    api: openai-completions
    apiKey: sk-proxy
    models:
      - id: m1
        thinking:
          mode: effort
          efforts: [low, high]
      - id: m2
`)
	writeConfigFile(t, home, `defaultThinkingLevel: low
modelRoles:
  default: proxy/m1
`)

	result, err := SaveCustomProvider("proxy", OmpCustomProvider{
		BaseURL: strPtr("https://proxy.example.test/v1"),
		API:     strPtr("openai-completions"),
		APIKey:  strPtr("sk-proxy"),
		Models: []OmpCustomModel{
			{ID: "m1", Thinking: &OmpThinkingConfig{
				Mode:    strPtr("effort"),
				Efforts: &[]string{"high"},
			}},
			{ID: "m2"},
		},
	})
	if err != nil {
		t.Fatalf("SaveCustomProvider: %v", err)
	}
	if result.Thinking == nil || result.Thinking.From != "low" {
		t.Fatalf("thinking = %+v, want low -> auto", result.Thinking)
	}
	config, _ := readConfigYML()
	if config.DefaultThinkingLevel == nil || *config.DefaultThinkingLevel != "auto" {
		t.Fatalf("defaultThinkingLevel = %v, want auto", config.DefaultThinkingLevel)
	}
}

// TestValidateThinkingLevelWarnings: apply reports an unsupported level instead
// of rewriting it.
func TestValidateThinkingLevelWarnings(t *testing.T) {
	home := sandbox(t)
	seedModelCache(t, home, map[string][]string{"anthropic": {"claude-fable-5"}})
	writeModelsFile(t, home, `providers:
  proxy:
    api: openai-completions
    apiKey: sk-proxy
    models:
      - id: m1
        thinking:
          mode: effort
          efforts: [low, high]
`)
	role := "proxy/m1"
	level := "xhigh"
	warnings, err := ValidateModelRoles(OmpModelRoles{Default: &role}, &level)
	if err != nil {
		t.Fatalf("ValidateModelRoles: %v", err)
	}
	if len(warnings) != 1 || !strings.Contains(warnings[0], "defaultThinkingLevel") {
		t.Fatalf("warnings = %v", warnings)
	}

	okLevel := "high"
	warnings, err = ValidateModelRoles(OmpModelRoles{Default: &role}, &okLevel)
	if err != nil {
		t.Fatalf("ValidateModelRoles: %v", err)
	}
	if len(warnings) != 0 {
		t.Fatalf("supported level flagged: %v", warnings)
	}
}
