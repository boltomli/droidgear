package app

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

// dshTestHome builds an isolated home with a `desktop` profile declaring the
// real base + web-app bundles, mirroring the Rust test fixture.
func dshTestHome(t *testing.T) string {
	t.Helper()
	home := isolatedHome(t)
	profileDir := filepath.Join(home, ".dsh", "profiles", "desktop")
	if err := os.MkdirAll(profileDir, 0o755); err != nil {
		t.Fatalf("profile dir: %v", err)
	}
	manifest := `{
  "name": "dsh-profile-desktop",
  "private": true,
  "dsh": { "profile": { "bundles": ["@deepseek-ai/dsh-base", "@deepseek-ai/dsh-web-app"] } }
}`
	if err := os.WriteFile(filepath.Join(profileDir, "package.json"), []byte(manifest), 0o644); err != nil {
		t.Fatalf("manifest: %v", err)
	}
	if err := os.WriteFile(filepath.Join(profileDir, "cordis.yml"), []byte("[]\n"), 0o644); err != nil {
		t.Fatalf("cordis.yml: %v", err)
	}
	return home
}

func writeDshPatch(t *testing.T, home string, text string) {
	t.Helper()
	path := filepath.Join(home, ".dsh", "profiles", "desktop", "cordis.patch.yml")
	if err := os.WriteFile(path, []byte(text), 0o600); err != nil {
		t.Fatalf("write patch: %v", err)
	}
}

func findEntry(t *testing.T, snapshot DshConfigSnapshot, id string) DshConfigEntryView {
	t.Helper()
	for _, entry := range snapshot.Entries {
		if entry.ID == id {
			return entry
		}
	}
	t.Fatalf("entry %s missing (have %d entries)", id, len(snapshot.Entries))
	return DshConfigEntryView{}
}

func TestDshConfigSnapshotComposesLayers(t *testing.T) {
	home := dshTestHome(t)
	writeDshPatch(t, home, "# header comment\n- id: tools\n  name: \"@deepseek-ai/dsh-tools\"\n  config:\n    mode: ptc\n")
	homePatch := filepath.Join(home, ".dsh", "cordis.patch.yml")
	if err := os.WriteFile(homePatch, []byte("- id: web-search-deepseek\n  config:\n    maxUses: 3\n"), 0o644); err != nil {
		t.Fatalf("home patch: %v", err)
	}

	a := &App{}
	snapshot, err := a.ReadDshConfigSnapshot(nil)
	if err != nil {
		t.Fatalf("ReadDshConfigSnapshot: %v", err)
	}
	if snapshot.Profile == nil || *snapshot.Profile != "desktop" {
		t.Fatalf("profile not resolved: %v", snapshot.Profile)
	}
	if len(snapshot.Entries) == 0 {
		t.Fatal("no entries composed from the real schema")
	}
	if len(snapshot.PatchHash) != 16 {
		t.Fatalf("patch hash should be 16 hex chars: %q", snapshot.PatchHash)
	}

	tools := findEntry(t, snapshot, "tools")
	if tools.Layer != "profile" || !tools.Overridden {
		t.Errorf("tools layer/override wrong: %+v", tools)
	}
	if object, ok := tools.Config.(map[string]any); !ok || object["mode"] != "ptc" {
		t.Errorf("tools config wrong: %#v", tools.Config)
	}

	search := findEntry(t, snapshot, "web-search-deepseek")
	if search.Layer != "home" || !search.ShadowedByHome {
		t.Errorf("web-search layer/shadow wrong: %+v", search)
	}

	// Field views carry enum metadata from the schema.
	var mode FieldProbe
	for _, field := range tools.Fields {
		if field.Name == "mode" {
			mode.found = true
			mode.kind = field.Kind
			mode.value = field.Value
			mode.enumValues = field.EnumValues
			mode.overridden = field.Overridden
		}
	}
	if !mode.found || mode.kind != "enum" || mode.value != "ptc" || !mode.overridden {
		t.Errorf("mode field view wrong: %+v", mode)
	}
	if len(mode.enumValues) != 2 || mode.enumValues[0] != "native" || mode.enumValues[1] != "ptc" {
		t.Errorf("mode enum values wrong: %v", mode.enumValues)
	}
}

// FieldProbe collects one field view for assertions.
type FieldProbe struct {
	found      bool
	kind       string
	value      any
	enumValues []string
	overridden bool
}

func TestDshConfigSavePreservesCommentsAndHash(t *testing.T) {
	home := dshTestHome(t)
	writeDshPatch(t, home, "# keep me\n- id: tools\n  name: \"@deepseek-ai/dsh-tools\"\n  config: {mode: native}\n")

	a := &App{}
	snapshot, err := a.ReadDshConfigSnapshot(nil)
	if err != nil {
		t.Fatalf("snapshot: %v", err)
	}

	profile := "desktop"
	result, err := a.SaveDshConfigEntry(&profile, "tools", map[string]any{"mode": "ptc"}, &snapshot.PatchHash)
	if err != nil {
		t.Fatalf("SaveDshConfigEntry: %v", err)
	}
	if !result.Changed {
		t.Fatal("expected change")
	}

	text, _ := os.ReadFile(filepath.Join(home, ".dsh", "profiles", "desktop", "cordis.patch.yml"))
	content := string(text)
	if !strings.HasPrefix(content, "# keep me\n") {
		t.Errorf("header comment lost: %q", content)
	}
	if !strings.Contains(content, "mode: ptc") {
		t.Errorf("new value missing: %s", content)
	}
	if result.PatchHash != fileHash(filepath.Join(home, ".dsh", "profiles", "desktop", "cordis.patch.yml")) {
		t.Errorf("returned hash does not match file")
	}

	// Stale hash → conflict with the DSH_CONFIG_CONFLICT prefix the store matches.
	stale := "0000000000000000"
	_, err = a.SaveDshConfigEntry(&profile, "tools", map[string]any{"mode": "native"}, &stale)
	if err == nil || !strings.Contains(err.Error(), "DSH_CONFIG_CONFLICT") {
		t.Fatalf("expected conflict error, got %v", err)
	}

	// Removing the override (config null) keeps the row's name and drops config.
	removed, err := a.SaveDshConfigEntry(&profile, "tools", nil, &result.PatchHash)
	if err != nil {
		t.Fatalf("remove override: %v", err)
	}
	if !removed.Changed {
		t.Fatal("expected removal change")
	}
	text, _ = os.ReadFile(filepath.Join(home, ".dsh", "profiles", "desktop", "cordis.patch.yml"))
	content = string(text)
	if strings.Contains(content, "config:") {
		t.Errorf("config not removed: %s", content)
	}
	// The row carried only id+name (2 keys ≤ 2), so it is removed entirely —
	// matching the Rust core — while the header comment survives.
	if strings.Contains(content, "id: tools") {
		t.Errorf("empty row should be removed: %s", content)
	}
	if !strings.HasPrefix(content, "# keep me\n") {
		t.Errorf("header comment lost after removal: %q", content)
	}
}

func TestDshConfigToggleDisabled(t *testing.T) {
	home := dshTestHome(t)
	a := &App{}
	profile := "desktop"

	// Disable an entry that has no row yet → a `disabled: true` row is added.
	result, err := a.SetDshConfigEntryDisabled(&profile, "tools", true, nil)
	if err != nil {
		t.Fatalf("disable: %v", err)
	}
	if !result.Changed {
		t.Fatal("expected change")
	}
	text, _ := os.ReadFile(filepath.Join(home, ".dsh", "profiles", "desktop", "cordis.patch.yml"))
	if !strings.Contains(string(text), "disabled: true") {
		t.Errorf("disabled row missing: %s", text)
	}

	// Re-enable → the row only carried id+disabled, so it is removed entirely.
	result, err = a.SetDshConfigEntryDisabled(&profile, "tools", false, &result.PatchHash)
	if err != nil {
		t.Fatalf("enable: %v", err)
	}
	if !result.Changed {
		t.Fatal("expected change on enable")
	}
	text, _ = os.ReadFile(filepath.Join(home, ".dsh", "profiles", "desktop", "cordis.patch.yml"))
	if strings.Contains(string(text), "id: tools") {
		t.Errorf("empty row should be removed: %s", text)
	}

	// Enabling a missing entry is a no-op.
	result, err = a.SetDshConfigEntryDisabled(&profile, "tools", false, nil)
	if err != nil {
		t.Fatalf("enable missing: %v", err)
	}
	if result.Changed {
		t.Error("enabling a missing entry must not change the file")
	}
}

func TestDshConfigHomeShadowRefused(t *testing.T) {
	home := dshTestHome(t)
	homePatch := filepath.Join(home, ".dsh", "cordis.patch.yml")
	if err := os.WriteFile(homePatch, []byte("- id: tools\n  config:\n    mode: native\n"), 0o644); err != nil {
		t.Fatalf("home patch: %v", err)
	}
	a := &App{}
	profile := "desktop"
	_, err := a.SaveDshConfigEntry(&profile, "tools", map[string]any{"mode": "ptc"}, nil)
	if err == nil || !strings.Contains(err.Error(), "DSH_CONFIG_SHADOWED") {
		t.Fatalf("expected shadowed error, got %v", err)
	}
}

func TestDshConfigJsMarkersRoundTrip(t *testing.T) {
	home := dshTestHome(t)
	writeDshPatch(t, home, "- id: tools\n  config:\n    mode: !js $state.tools.mode\n")

	a := &App{}
	snapshot, err := a.ReadDshConfigSnapshot(nil)
	if err != nil {
		t.Fatalf("snapshot: %v", err)
	}
	tools := findEntry(t, snapshot, "tools")
	if !tools.HasJS {
		t.Errorf("hasJs should be true: %+v", tools)
	}

	// Save another field on the same entry; the !js value must survive.
	profile := "desktop"
	result, err := a.SaveDshConfigEntry(&profile, "tools", map[string]any{
		"mode":                "ptc",
		"maxParallelSubCalls": float64(4),
	}, &snapshot.PatchHash)
	if err != nil {
		t.Fatalf("save: %v", err)
	}
	_ = result
	snapshot2, err := a.ReadDshConfigSnapshot(nil)
	if err != nil {
		t.Fatalf("snapshot2: %v", err)
	}
	tools2 := findEntry(t, snapshot2, "tools")
	object, ok := tools2.Config.(map[string]any)
	if !ok {
		t.Fatalf("config not an object: %#v", tools2.Config)
	}
	// The UI wrote a concrete mode, replacing the marker — validation must
	// have accepted the enum and the file must still parse.
	if object["mode"] != "ptc" || object["maxParallelSubCalls"] != float64(4) {
		t.Errorf("saved config wrong: %#v", object)
	}
}

func TestDshConfigSecretHandling(t *testing.T) {
	home := dshTestHome(t)
	a := &App{}
	profile := "desktop"

	// Literal secret values are refused with a helpful pointer.
	_, err := a.SaveDshConfigEntry(&profile, "web-search-deepseek", map[string]any{
		"apiKey": "sk-real-secret",
	}, nil)
	if err == nil || !strings.Contains(err.Error(), "credentials.yaml") {
		t.Fatalf("expected secret refusal, got %v", err)
	}

	// $secret markers are dropped (keeping the inherited value).
	result, err := a.SaveDshConfigEntry(&profile, "web-search-deepseek", map[string]any{
		"apiKey":    map[string]any{"$secret": true},
		"apiKeyEnv": "MY_KEY",
	}, nil)
	if err != nil {
		t.Fatalf("marker save: %v", err)
	}
	if !result.Changed {
		t.Fatal("expected change")
	}
	text, _ := os.ReadFile(filepath.Join(home, ".dsh", "profiles", "desktop", "cordis.patch.yml"))
	if strings.Contains(string(text), "$secret") {
		t.Errorf("secret marker leaked into the file: %s", text)
	}

	// The snapshot redacts the stored secret.
	snapshot, err := a.ReadDshConfigSnapshot(nil)
	if err != nil {
		t.Fatalf("snapshot: %v", err)
	}
	search := findEntry(t, snapshot, "web-search-deepseek")
	for _, field := range search.Fields {
		if field.Name == "apiKey" && field.Value != nil {
			if marker, ok := field.Value.(map[string]any); !ok || marker["$secret"] != true {
				t.Errorf("secret not redacted: %#v", field.Value)
			}
		}
	}
}

func TestDshConfigValidation(t *testing.T) {
	dshTestHome(t) // profile must exist for the write path
	a := &App{}
	profile := "desktop"

	// Enum violation.
	_, err := a.SaveDshConfigEntry(&profile, "tools", map[string]any{"mode": "both"}, nil)
	if err == nil || !strings.Contains(err.Error(), "must be one of") {
		t.Fatalf("expected enum error, got %v", err)
	}
	// Type violation.
	_, err = a.SaveDshConfigEntry(&profile, "tools", map[string]any{"maxParallelSubCalls": "four"}, nil)
	if err == nil || !strings.Contains(err.Error(), "must be a number") {
		t.Fatalf("expected number error, got %v", err)
	}
	// Unknown entry.
	_, err = a.SaveDshConfigEntry(&profile, "no-such-entry", map[string]any{}, nil)
	if err == nil || !strings.Contains(err.Error(), "Unknown DSH configuration entry") {
		t.Fatalf("expected unknown entry error, got %v", err)
	}
	// Empty id.
	_, err = a.SaveDshConfigEntry(&profile, "  ", map[string]any{}, nil)
	if err == nil || !strings.Contains(err.Error(), "Entry id is required") {
		t.Fatalf("expected entry id error, got %v", err)
	}
}

func TestDshConfigNoProfileSnapshot(t *testing.T) {
	isolatedHome(t) // empty home: no profiles at all
	a := &App{}
	snapshot, err := a.ReadDshConfigSnapshot(nil)
	if err != nil {
		t.Fatalf("snapshot: %v", err)
	}
	if snapshot.Profile != nil {
		t.Errorf("expected no profile, got %v", snapshot.Profile)
	}
	if snapshot.PatchHash != "absent" {
		t.Errorf("expected absent hash, got %q", snapshot.PatchHash)
	}
	found := false
	for _, warning := range snapshot.Warnings {
		if warning.Code == "DSH_CONFIG_NO_PROFILE" {
			found = true
		}
	}
	if !found {
		t.Errorf("missing DSH_CONFIG_NO_PROFILE warning: %+v", snapshot.Warnings)
	}
}

func TestFileHashMatchesRustFnv1a(t *testing.T) {
	// Known FNV-1a 64 vectors keep the Go port byte-compatible with the
	// Rust core the frontend already round-trips.
	if got := hashBytes([]byte("")); got != "cbf29ce484222325" {
		t.Errorf("empty hash: %s", got)
	}
	if got := hashBytes([]byte("a")); got != "af63dc4c8601ec8c" {
		t.Errorf("hash('a'): %s", got)
	}
}
