package paths

import (
	"context"
	"encoding/json"
	"os"
	"path/filepath"
	"testing"
)

// sandbox points every home-ish root at a temp directory. Windows derives
// extra roots from the environment (the Hermes priority chain consults
// %LOCALAPPDATA%), so all four have to move together or the assertions leak
// into the developer's real profile.
func sandbox(t *testing.T) string {
	t.Helper()
	home := t.TempDir()
	for _, key := range []string{"HOME", "USERPROFILE", "APPDATA", "LOCALAPPDATA"} {
		t.Setenv(key, home)
	}
	return home
}

func TestEffectiveDefaultsStayUnderHome(t *testing.T) {
	home := sandbox(t)
	ctx := context.Background()

	effective, err := Effective(ctx)
	if err != nil {
		t.Fatalf("Effective: %v", err)
	}
	if got := len(effective.List()); got != 9 {
		t.Fatalf("expected nine paths, got %d", got)
	}
	for _, path := range effective.List() {
		if !path.IsDefault {
			t.Errorf("%s: expected default before any override, got %q", path.Key, path.Path)
		}
		if path.Key == "hermes" {
			// Hermes resolves through the Windows priority chain (LOCALAPPDATA
			// → ~/.hermes → WSL), so only assert it is non-empty here.
			if path.Path == "" {
				t.Error("hermes: empty path")
			}
			continue
		}
		if filepath.Dir(path.Path) == "" {
			t.Errorf("%s: empty path", path.Key)
		}
	}
	if got := effective.Codex.Path; got != filepath.Join(home, ".codex") {
		t.Errorf("codex = %q, want %q", got, filepath.Join(home, ".codex"))
	}
}

func TestDefaultsIgnoresOverrides(t *testing.T) {
	sandbox(t)
	ctx := context.Background()

	if err := SaveOverride(ctx, "codex", "/elsewhere/codex"); err != nil {
		t.Fatalf("SaveOverride: %v", err)
	}
	defaults, err := Defaults(ctx)
	if err != nil {
		t.Fatalf("Defaults: %v", err)
	}
	if !defaults.Codex.IsDefault || defaults.Codex.Path == "/elsewhere/codex" {
		t.Errorf("Defaults must ignore overrides, got %+v", defaults.Codex)
	}
}

func TestSaveAndResetOverrideRoundTrip(t *testing.T) {
	home := sandbox(t)
	ctx := context.Background()

	if err := SaveOverride(ctx, "codex", "/elsewhere/codex"); err != nil {
		t.Fatalf("SaveOverride: %v", err)
	}

	effective, err := Effective(ctx)
	if err != nil {
		t.Fatalf("Effective: %v", err)
	}
	if effective.Codex.IsDefault {
		t.Error("codex should no longer be the default")
	}
	if effective.Codex.Path != "/elsewhere/codex" {
		t.Errorf("codex = %q, want /elsewhere/codex", effective.Codex.Path)
	}
	// The other keys stay untouched — a single-key override must not reset
	// or invent the rest of the map.
	if !effective.Claude.IsDefault {
		t.Error("claude must stay default after overriding codex")
	}

	settingsPath := filepath.Join(home, ".droidgear", "settings.json")
	raw, err := os.ReadFile(settingsPath) // #nosec G304 -- temp fixture
	if err != nil {
		t.Fatalf("read settings: %v", err)
	}
	var settings map[string]any
	if err := json.Unmarshal(raw, &settings); err != nil {
		t.Fatalf("parse settings: %v", err)
	}
	configPaths, _ := settings["configPaths"].(map[string]any)
	if configPaths["codex"] != "/elsewhere/codex" {
		t.Errorf("settings.json configPaths.codex = %v, want /elsewhere/codex", configPaths["codex"])
	}

	if err := ResetOverride(ctx, "codex"); err != nil {
		t.Fatalf("ResetOverride: %v", err)
	}
	after, err := Effective(ctx)
	if err != nil {
		t.Fatalf("Effective after reset: %v", err)
	}
	if !after.Codex.IsDefault {
		t.Error("codex should be back to the default")
	}

	raw, err = os.ReadFile(settingsPath) // #nosec G304 -- temp fixture
	if err != nil {
		t.Fatalf("read settings after reset: %v", err)
	}
	// A fresh map: unmarshalling into the old one would merge and keep the
	// stale configPaths.codex from the first parse.
	var afterReset map[string]any
	if err := json.Unmarshal(raw, &afterReset); err != nil {
		t.Fatalf("parse settings after reset: %v", err)
	}
	configPaths, _ = afterReset["configPaths"].(map[string]any)
	if _, stillThere := configPaths["codex"]; stillThere {
		t.Error("configPaths.codex must be removed by reset")
	}
}

func TestUnknownKeyIsRejected(t *testing.T) {
	sandbox(t)
	ctx := context.Background()

	if err := SaveOverride(ctx, "nope", "/tmp/x"); err == nil {
		t.Error("SaveOverride(nope) should fail")
	}
	if err := ResetOverride(ctx, "nope"); err == nil {
		t.Error("ResetOverride(nope) should fail")
	}
}

func TestKeysMatchesList(t *testing.T) {
	if got := len(Keys()); got != 9 {
		t.Fatalf("Keys() = %d entries, want 9", got)
	}
	effective, err := Effective(context.Background())
	if err != nil {
		t.Fatalf("Effective: %v", err)
	}
	list := effective.List()
	for i, key := range Keys() {
		if list[i].Key != key {
			t.Errorf("position %d: List()=%s Keys()=%s", i, list[i].Key, key)
		}
	}
}
