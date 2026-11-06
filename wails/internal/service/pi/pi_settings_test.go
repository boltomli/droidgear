package pi

import (
	"encoding/json"
	"os"
	"testing"

	"droidgear-wails/internal/testutil"
)

// TestReadSettingsMissingFile reports "nothing pinned yet" instead of failing:
// pi itself starts with automatic model selection when settings.json is absent.
func TestReadSettingsMissingFile(t *testing.T) {
	testutil.IsolatedHome(t)

	status, err := ReadSettings()
	if err != nil {
		t.Fatalf("ReadSettings: %v", err)
	}
	if status.SettingsExists {
		t.Fatalf("SettingsExists = true, want false")
	}
	if status.SettingsPath == "" {
		t.Fatalf("SettingsPath empty")
	}
	if status.DefaultProvider != nil || status.DefaultModel != nil || status.DefaultThinkingLevel != nil {
		t.Fatalf("expected empty settings, got %+v", status.PiSettings)
	}
}

// TestSaveSettingsKeepsForeignKeys guards the read-modify-write contract:
// settings.json holds dozens of keys DroidGear does not model (retry, theme,
// compaction, …) and a save must not drop them.
func TestSaveSettingsKeepsForeignKeys(t *testing.T) {
	testutil.IsolatedHome(t)

	path, err := piSettingsPath()
	if err != nil {
		t.Fatalf("piSettingsPath: %v", err)
	}
	seed := `{
  "$schema": "https://pi.dev/schemas/settings.schema.json",
  "retry": { "enabled": true },
  "theme": "dark"
}`
	if err := os.WriteFile(path, []byte(seed), 0o600); err != nil {
		t.Fatalf("seed settings.json: %v", err)
	}

	provider, model, level := "wududu", "deepseek-v4.1-flash", "high"
	if err := SaveSettings(PiSettings{
		DefaultProvider:      &provider,
		DefaultModel:         &model,
		DefaultThinkingLevel: &level,
	}); err != nil {
		t.Fatalf("SaveSettings: %v", err)
	}

	data, err := os.ReadFile(path)
	if err != nil {
		t.Fatalf("read settings.json: %v", err)
	}
	var root map[string]any
	if err := json.Unmarshal(data, &root); err != nil {
		t.Fatalf("unmarshal: %v", err)
	}
	if root["defaultProvider"] != provider {
		t.Errorf("defaultProvider = %v, want %q", root["defaultProvider"], provider)
	}
	if root["defaultModel"] != model {
		t.Errorf("defaultModel = %v, want %q", root["defaultModel"], model)
	}
	if root["defaultThinkingLevel"] != level {
		t.Errorf("defaultThinkingLevel = %v, want %q", root["defaultThinkingLevel"], level)
	}
	if root["theme"] != "dark" {
		t.Errorf("foreign key theme dropped: %v", root["theme"])
	}
	if _, ok := root["retry"].(map[string]any); !ok {
		t.Errorf("foreign key retry dropped: %v", root["retry"])
	}
	if root["$schema"] == nil {
		t.Errorf("$schema dropped")
	}

	status, err := ReadSettings()
	if err != nil {
		t.Fatalf("ReadSettings: %v", err)
	}
	if !status.SettingsExists {
		t.Fatalf("SettingsExists = false after save")
	}
	if status.DefaultThinkingLevel == nil || *status.DefaultThinkingLevel != level {
		t.Fatalf("round-trip thinking level = %v, want %q", status.DefaultThinkingLevel, level)
	}
}

// TestSaveSettingsBlankClearsKey pins the unset semantics: an empty string
// removes the key (pi then picks automatically) instead of writing "".
func TestSaveSettingsBlankClearsKey(t *testing.T) {
	testutil.IsolatedHome(t)

	provider, model := "wududu", "m1"
	if err := SaveSettings(PiSettings{DefaultProvider: &provider, DefaultModel: &model}); err != nil {
		t.Fatalf("SaveSettings: %v", err)
	}
	blank := "  "
	if err := SaveSettings(PiSettings{DefaultProvider: &blank, DefaultModel: &model}); err != nil {
		t.Fatalf("SaveSettings(blank): %v", err)
	}

	data, err := os.ReadFile(mustSettingsPath(t))
	if err != nil {
		t.Fatalf("read settings.json: %v", err)
	}
	var root map[string]any
	if err := json.Unmarshal(data, &root); err != nil {
		t.Fatalf("unmarshal: %v", err)
	}
	if _, ok := root["defaultProvider"]; ok {
		t.Fatalf("defaultProvider still present: %v", root["defaultProvider"])
	}
	if root["defaultModel"] != model {
		t.Fatalf("defaultModel = %v, want %q", root["defaultModel"], model)
	}
}

func TestClearSettings(t *testing.T) {
	testutil.IsolatedHome(t)

	provider, model, level := "wududu", "m1", "low"
	if err := SaveSettings(PiSettings{
		DefaultProvider:      &provider,
		DefaultModel:         &model,
		DefaultThinkingLevel: &level,
	}); err != nil {
		t.Fatalf("SaveSettings: %v", err)
	}
	if err := ClearSettings(); err != nil {
		t.Fatalf("ClearSettings: %v", err)
	}

	status, err := ReadSettings()
	if err != nil {
		t.Fatalf("ReadSettings: %v", err)
	}
	if status.DefaultProvider != nil || status.DefaultModel != nil || status.DefaultThinkingLevel != nil {
		t.Fatalf("settings not cleared: %+v", status.PiSettings)
	}
}

// TestSaveSettingsRejectsUnknownThinkingLevel: a typo would silently disable
// the setting upstream, so the write fails instead.
func TestSaveSettingsRejectsUnknownThinkingLevel(t *testing.T) {
	testutil.IsolatedHome(t)

	level := "highh"
	err := SaveSettings(PiSettings{DefaultThinkingLevel: &level})
	if err == nil {
		t.Fatalf("expected an error for thinking level %q", level)
	}
	if _, statErr := os.Stat(mustSettingsPath(t)); statErr == nil {
		t.Fatalf("settings.json written despite the invalid thinking level")
	}
}

func mustSettingsPath(t *testing.T) string {
	t.Helper()
	path, err := piSettingsPath()
	if err != nil {
		t.Fatalf("piSettingsPath: %v", err)
	}
	return path
}
