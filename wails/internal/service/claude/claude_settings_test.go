package claude

import (
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"droidgear-wails/internal/core"
	"droidgear-wails/internal/testutil"
)

// ---------------------------------------------------------------------------
// Pure helpers
// ---------------------------------------------------------------------------

func TestClaudeWithJSONExtReplacesLastExtension(t *testing.T) {
	cases := map[string]string{
		"work":      filepath.Join("dir", "work.json"),
		"my.file":   filepath.Join("dir", "my.json"),
		"a.b.c":     filepath.Join("dir", "a.b.json"),
		".hidden":   filepath.Join("dir", ".hidden.json"),
		"trailing.": filepath.Join("dir", "trailing.json"),
	}
	for name, want := range cases {
		if got := claudeWithJSONExt("dir", name); got != want {
			t.Fatalf("claudeWithJSONExt(%q) = %q, want %q", name, got, want)
		}
	}
}

func TestClaudeSlugifyProfileName(t *testing.T) {
	cases := map[string]string{
		"My Profile":       "my-profile",
		"Already-Snake_1":  "already-snake_1",
		"dots.every.where": "dots.every.where",
		"  spaced  out  ":  "spaced-out",
		"中文名字":             "profile",
		"...leading":       "leading",
		"!!!":              "profile",
	}
	for input, want := range cases {
		if got := claudeSlugifyProfileName(input); got != want {
			t.Fatalf("claudeSlugifyProfileName(%q) = %q, want %q", input, got, want)
		}
	}
}

func TestClaudeValidateCustomFileName(t *testing.T) {
	valid := []string{"work", "my-profile_2", "v1.2"}
	for _, name := range valid {
		if err := claudeValidateCustomFileName(name); err != nil {
			t.Fatalf("expected %q to be valid: %v", name, err)
		}
	}

	invalid := map[string]string{
		"":                       "File name cannot be empty",
		"   ":                    "File name cannot be empty",
		"global":                 "Cannot use 'Global' as a custom file name",
		"GLOBAL":                 "Cannot use 'Global' as a custom file name",
		"bad name":               "Invalid file name: only letters, digits, dashes, underscores, and dots are allowed",
		"slash/name":             "Invalid file name: only letters, digits, dashes, underscores, and dots are allowed",
		"..":                     "File name cannot start with '.' or contain '..'",
		".hidden":                "File name cannot start with '.' or contain '..'",
		"has..dots":              "File name cannot start with '.' or contain '..'",
		strings.Repeat("x", 101): "File name too long (max 100 characters)",
	}
	for name, want := range invalid {
		err := claudeValidateCustomFileName(name)
		if err == nil || err.Error() != want {
			t.Fatalf("validate(%q) = %v, want %q", name, err, want)
		}
	}
}

// ---------------------------------------------------------------------------
// File listing / active pointer
// ---------------------------------------------------------------------------

func TestClaudeSettingsListIncludesGlobalAndCustom(t *testing.T) {
	home := claudeTestHome(t)

	claudeTestWrite(t, filepath.Join(home, ".claude", "settings.json"), "{}")
	claudeTestWrite(t, filepath.Join(home, ".droidgear", "claude-settings", "work.json"), "{}")

	if _, err := SetActiveSettingsFile(core.StrPtr("work")); err != nil {
		t.Fatalf("SetActiveSettingsFile: %v", err)
	}

	files, err := ListSettingsFiles()
	if err != nil {
		t.Fatalf("ListSettingsFiles: %v", err)
	}
	if len(files) != 2 {
		t.Fatalf("expected 2 files, got %d: %+v", len(files), files)
	}
	if !files[0].IsGlobal || files[0].Name != "Global" {
		t.Fatalf("first entry should be Global: %+v", files[0])
	}
	if files[0].IsActive {
		t.Fatalf("Global should not be active: %+v", files[0])
	}
	if files[1].Name != "work" || !files[1].IsActive || files[1].IsGlobal || !files[1].Exists {
		t.Fatalf("work entry wrong: %+v", files[1])
	}

	active, err := GetActiveSettingsFile()
	if err != nil {
		t.Fatalf("GetActiveSettingsFile: %v", err)
	}
	if active.Name != "work" || active.IsGlobal {
		t.Fatalf("active file wrong: %+v", active)
	}
}

func TestClaudeSettingsSetActiveFallsBackAndValidates(t *testing.T) {
	home := claudeTestHome(t)

	claudeTestWrite(t, filepath.Join(home, ".claude", "settings.json"), "{}")

	// nil → Global.
	info, err := SetActiveSettingsFile(nil)
	if err != nil {
		t.Fatalf("SetActiveSettingsFile(nil): %v", err)
	}
	if !info.IsGlobal || info.Name != "Global" || !info.IsActive {
		t.Fatalf("expected Global active: %+v", info)
	}

	// Empty string → Global too.
	info, err = SetActiveSettingsFile(core.StrPtr(""))
	if err != nil || !info.IsGlobal {
		t.Fatalf("expected Global for empty name: %+v (err=%v)", info, err)
	}

	// Unknown custom file → the Rust error string.
	_, err = SetActiveSettingsFile(core.StrPtr("ghost"))
	if err == nil || err.Error() != "Settings file 'ghost' does not exist" {
		t.Fatalf("expected missing-file error, got %v", err)
	}

	// A stored active name whose file vanished falls back to Global.
	claudeTestWrite(t, filepath.Join(home, ".droidgear", "settings.json"),
		`{"claudeSettingsActiveFile":"vanished"}`)
	info, err = GetActiveSettingsFile()
	if err != nil {
		t.Fatalf("GetActiveSettingsFile: %v", err)
	}
	if !info.IsGlobal {
		t.Fatalf("stale active name must fall back to Global: %+v", info)
	}
}

// ---------------------------------------------------------------------------
// Create / delete / duplicate
// ---------------------------------------------------------------------------

func TestClaudeSettingsCreateCopiesActiveAndActivates(t *testing.T) {
	home := claudeTestHome(t)

	claudeTestWrite(t, filepath.Join(home, ".claude", "settings.json"),
		`{"autoUpdate":false,"env":{"FOO":"bar"}}`)

	info, err := CreateSettingsFile("personal", true)
	if err != nil {
		t.Fatalf("CreateSettingsFile: %v", err)
	}
	if info.Name != "personal" || !info.IsActive || info.IsGlobal {
		t.Fatalf("created file info wrong: %+v", info)
	}

	value, err := ReadSettingsFile("personal")
	if err != nil {
		t.Fatalf("ReadSettingsFile: %v", err)
	}
	doc := value.(map[string]any)
	if enabled, _ := doc["autoUpdate"].(bool); enabled {
		t.Fatalf("autoUpdate should be false: %+v", doc)
	}
	if env := doc["env"].(map[string]any); env["FOO"] != "bar" {
		t.Fatalf("env not copied: %+v", doc)
	}

	// A fresh file without copy starts empty.
	fresh, err := CreateSettingsFile("fresh", false)
	if err != nil {
		t.Fatalf("CreateSettingsFile (fresh): %v", err)
	}
	value, err = ReadSettingsFile(fresh.Name)
	if err != nil {
		t.Fatalf("ReadSettingsFile (fresh): %v", err)
	}
	if len(value.(map[string]any)) != 0 {
		t.Fatalf("fresh file should be empty: %+v", value)
	}
}

func TestClaudeSettingsCreateRejectsReservedAndInvalidNames(t *testing.T) {
	claudeTestHome(t)

	if _, err := CreateSettingsFile("global", false); err == nil || !strings.Contains(err.Error(), "Cannot use 'Global'") {
		t.Fatalf("expected reserved-name error, got %v", err)
	}
	if _, err := CreateSettingsFile("bad name", false); err == nil || !strings.Contains(err.Error(), "Invalid file name") {
		t.Fatalf("expected invalid-name error, got %v", err)
	}
}

func TestClaudeSettingsDeleteResetsActiveToGlobal(t *testing.T) {
	home := claudeTestHome(t)

	claudeTestWrite(t, filepath.Join(home, ".claude", "settings.json"), "{}")
	if _, err := CreateSettingsFile("drop", false); err != nil {
		t.Fatalf("CreateSettingsFile: %v", err)
	}
	if err := DeleteSettingsFile("drop"); err != nil {
		t.Fatalf("DeleteSettingsFile: %v", err)
	}

	active, err := GetActiveSettingsFile()
	if err != nil {
		t.Fatalf("GetActiveSettingsFile: %v", err)
	}
	if !active.IsGlobal {
		t.Fatalf("active should reset to Global: %+v", active)
	}

	if err := DeleteSettingsFile("Global"); err == nil || err.Error() != "Cannot delete the global settings file" {
		t.Fatalf("expected global-delete error, got %v", err)
	}
	if err := DeleteSettingsFile("nope"); err == nil || err.Error() != "Settings file 'nope' does not exist" {
		t.Fatalf("expected missing-file error, got %v", err)
	}
}

func TestClaudeSettingsDuplicateCopiesAndActivates(t *testing.T) {
	home := claudeTestHome(t)

	claudeTestWrite(t, filepath.Join(home, ".claude", "settings.json"), `{"theme":"dark"}`)
	source, err := CreateSettingsFile("base", false)
	if err != nil {
		t.Fatalf("CreateSettingsFile: %v", err)
	}
	if err := SaveSettingsFile(source.Name, map[string]any{"theme": "dark"}); err != nil {
		t.Fatalf("SaveSettingsFile: %v", err)
	}

	duplicate, err := DuplicateSettingsFile("base", "copy")
	if err != nil {
		t.Fatalf("DuplicateSettingsFile: %v", err)
	}
	if duplicate.Name != "copy" || !duplicate.IsActive {
		t.Fatalf("duplicate info wrong: %+v", duplicate)
	}
	value, err := ReadSettingsFile("copy")
	if err != nil {
		t.Fatalf("ReadSettingsFile: %v", err)
	}
	if value.(map[string]any)["theme"] != "dark" {
		t.Fatalf("duplicate lost contents: %+v", value)
	}

	if _, err := DuplicateSettingsFile("base", "copy"); err == nil || err.Error() != "Settings file 'copy' already exists" {
		t.Fatalf("expected already-exists error, got %v", err)
	}
}

// ---------------------------------------------------------------------------
// Read / save / merge / load-from-live
// ---------------------------------------------------------------------------

func TestClaudeSettingsReadMissingGlobalYieldsEmptyObject(t *testing.T) {
	claudeTestHome(t)

	value, err := ReadSettingsFile("Global")
	if err != nil {
		t.Fatalf("ReadSettingsFile: %v", err)
	}
	doc, ok := value.(map[string]any)
	if !ok || len(doc) != 0 {
		t.Fatalf("expected empty object, got %#v", value)
	}

	if _, err := ReadSettingsFile("ghost"); err == nil || err.Error() != "Settings file 'ghost' does not exist" {
		t.Fatalf("expected missing-file error, got %v", err)
	}
}

func TestClaudeSettingsSaveRoundTripsThroughRead(t *testing.T) {
	claudeTestHome(t)

	if _, err := CreateSettingsFile("round", false); err != nil {
		t.Fatalf("CreateSettingsFile: %v", err)
	}
	want := map[string]any{
		"env":         map[string]any{"DISABLE_TELEMETRY": "0"},
		"permissions": map[string]any{"defaultMode": "acceptEdits"},
	}
	if err := SaveSettingsFile("round", want); err != nil {
		t.Fatalf("SaveSettingsFile: %v", err)
	}

	got, err := ReadSettingsFile("round")
	if err != nil {
		t.Fatalf("ReadSettingsFile: %v", err)
	}
	encoded, _ := json.Marshal(got)
	expected, _ := json.Marshal(want)
	if string(encoded) != string(expected) {
		t.Fatalf("round trip mismatch:\n got %s\nwant %s", encoded, expected)
	}

	// Non-object roots are rejected with the Rust error string.
	if err := SaveSettingsFile("round", []any{1, 2}); err == nil || err.Error() != "Settings file root must be a JSON object" {
		t.Fatalf("expected root error, got %v", err)
	}
	if err := SaveSettingsFile("round", "plain string"); err == nil {
		t.Fatalf("expected root error for string payload")
	}
}

func TestClaudeSettingsSaveCleansTopLevelModelWhenEnvModelPresent(t *testing.T) {
	claudeTestHome(t)

	if _, err := CreateSettingsFile("clean", false); err != nil {
		t.Fatalf("CreateSettingsFile: %v", err)
	}
	if err := SaveSettingsFile("clean", map[string]any{
		"model": "claude-sonnet-4-5",
		"env":   map[string]any{claudeModelEnv: "claude-sonnet-4-5"},
	}); err != nil {
		t.Fatalf("SaveSettingsFile: %v", err)
	}
	got, err := ReadSettingsFile("clean")
	if err != nil {
		t.Fatalf("ReadSettingsFile: %v", err)
	}
	doc := got.(map[string]any)
	if _, found := doc["model"]; found {
		t.Fatalf("stale top-level model must be dropped: %+v", doc)
	}
	if env := doc["env"].(map[string]any); env[claudeModelEnv] != "claude-sonnet-4-5" {
		t.Fatalf("env model lost: %+v", doc)
	}

	// Without an env model the top-level model survives.
	if err := SaveSettingsFile("clean", map[string]any{"model": "claude-sonnet-4-5"}); err != nil {
		t.Fatalf("SaveSettingsFile (keep): %v", err)
	}
	got, err = ReadSettingsFile("clean")
	if err != nil {
		t.Fatalf("ReadSettingsFile (keep): %v", err)
	}
	if got.(map[string]any)["model"] != "claude-sonnet-4-5" {
		t.Fatalf("top-level model should survive: %+v", got)
	}
}

func TestClaudeSettingsLoadFromLiveCleansTopLevelModel(t *testing.T) {
	home := claudeTestHome(t)

	claudeTestWrite(t, filepath.Join(home, ".claude", "settings.json"),
		`{"model":"stale","env":{"ANTHROPIC_MODEL":"claude-sonnet-4-5"}}`)
	if _, err := CreateSettingsFile("load", false); err != nil {
		t.Fatalf("CreateSettingsFile: %v", err)
	}
	if err := LoadSettingsFromLive("load"); err != nil {
		t.Fatalf("LoadSettingsFromLive: %v", err)
	}

	got, err := ReadSettingsFile("load")
	if err != nil {
		t.Fatalf("ReadSettingsFile: %v", err)
	}
	doc := got.(map[string]any)
	if _, found := doc["model"]; found {
		t.Fatalf("loaded document must be normalised: %+v", doc)
	}
	if env := doc["env"].(map[string]any); env[claudeModelEnv] != "claude-sonnet-4-5" {
		t.Fatalf("env model lost: %+v", doc)
	}
}

func TestClaudeSettingsMergeToGlobalShallowMergesEnvAndPermissions(t *testing.T) {
	home := claudeTestHome(t)

	globalPath := filepath.Join(home, ".claude", "settings.json")
	claudeTestWrite(t, globalPath, `{
		"theme": "light",
		"model": "keep-me",
		"env": {"GLOBAL_ONLY": "1", "SHARED": "old"},
		"permissions": {"globalOnly": true}
	}`)
	if _, err := CreateSettingsFile("source", false); err != nil {
		t.Fatalf("CreateSettingsFile: %v", err)
	}
	if err := SaveSettingsFile("source", map[string]any{
		"theme":       "dark",
		"env":         map[string]any{"SHARED": "new", "SOURCE_ONLY": "1"},
		"permissions": map[string]any{"defaultMode": "acceptEdits"},
	}); err != nil {
		t.Fatalf("SaveSettingsFile: %v", err)
	}

	if err := MergeSettingsToGlobal("source"); err != nil {
		t.Fatalf("MergeSettingsToGlobal: %v", err)
	}

	got, err := ReadSettingsFile("Global")
	if err != nil {
		t.Fatalf("ReadSettingsFile: %v", err)
	}
	doc := got.(map[string]any)
	if doc["theme"] != "dark" {
		t.Fatalf("theme should be overridden: %+v", doc)
	}
	if doc["model"] != "keep-me" {
		t.Fatalf("global-only top-level key must survive: %+v", doc)
	}
	env := doc["env"].(map[string]any)
	if env["GLOBAL_ONLY"] != "1" || env["SHARED"] != "new" || env["SOURCE_ONLY"] != "1" {
		t.Fatalf("env shallow merge wrong: %+v", env)
	}
	permissions := doc["permissions"].(map[string]any)
	if permissions["globalOnly"] != true || permissions["defaultMode"] != "acceptEdits" {
		t.Fatalf("permissions shallow merge wrong: %+v", permissions)
	}
}

// ---------------------------------------------------------------------------
// Legacy profile migration
// ---------------------------------------------------------------------------

func TestClaudeLegacyProfilesAreMigratedIntoSettingsFiles(t *testing.T) {
	home := claudeTestHome(t)

	claudeTestWrite(t, filepath.Join(home, ".droidgear", "claude", "profiles", "abc.json"), `{
		"id": "abc",
		"name": "My Profile",
		"baseUrl": "https://proxy.example.com",
		"bearerToken": "token-1",
		"model": "claude-sonnet-4-5",
		"smallModelUsesMainModel": true,
		"reasoningEffort": "high",
		"thinkingMode": "on",
		"createdAt": "2026-01-01T00:00:00Z",
		"updatedAt": "2026-01-01T00:00:00Z"
	}`)

	files, err := ListSettingsFiles()
	if err != nil {
		t.Fatalf("ListSettingsFiles: %v", err)
	}
	customs := []ClaudeSettingsFileInfo{}
	for _, file := range files {
		if !file.IsGlobal {
			customs = append(customs, file)
		}
	}
	if len(customs) != 1 || customs[0].Name != "my-profile" {
		t.Fatalf("expected migrated file 'my-profile', got %+v", customs)
	}

	value, err := ReadSettingsFile("my-profile")
	if err != nil {
		t.Fatalf("ReadSettingsFile: %v", err)
	}
	doc := value.(map[string]any)
	env := doc["env"].(map[string]any)
	if env[claudeAuthTokenEnv] != "token-1" || env[claudeModelEnv] != "claude-sonnet-4-5" {
		t.Fatalf("migrated env wrong: %+v", env)
	}
	if enabled, _ := doc["alwaysThinkingEnabled"].(bool); !enabled {
		t.Fatalf("migrated thinking mode wrong: %+v", doc)
	}
	if effort, _ := env[claudeEffortEnv].(string); effort != "high" {
		t.Fatalf("migrated effort wrong: %+v", env)
	}

	// Idempotent: the migration flag stops a second run.
	again, err := ListSettingsFiles()
	if err != nil {
		t.Fatalf("ListSettingsFiles (2): %v", err)
	}
	if len(again) != len(files) {
		t.Fatalf("migration is not idempotent: %d vs %d", len(again), len(files))
	}
}

// ---------------------------------------------------------------------------
// Launch command & temporary-run preview
// ---------------------------------------------------------------------------

func TestClaudeSettingsLaunchCommand(t *testing.T) {
	home := claudeTestHome(t)

	claudeTestWrite(t, filepath.Join(home, ".claude", "settings.json"), "{}")

	got, err := GetSettingsLaunchCommand(false)
	if err != nil {
		t.Fatalf("GetSettingsLaunchCommand: %v", err)
	}
	wantPath := filepath.Join(home, ".claude", "settings.json")
	if got[1] != wantPath {
		t.Fatalf("path = %q, want %q", got[1], wantPath)
	}
	if got[0] != `claude --settings "`+wantPath+`"` {
		t.Fatalf("command = %q", got[0])
	}

	got, err = GetSettingsLaunchCommand(true)
	if err != nil {
		t.Fatalf("GetSettingsLaunchCommand (dangerous): %v", err)
	}
	if got[0] != `claude --settings "`+wantPath+`" --dangerously-skip-permissions` {
		t.Fatalf("command with skip flag = %q", got[0])
	}
}

func TestClaudePreviewTemporaryRunFromFile(t *testing.T) {
	home := claudeTestHome(t)
	t.Setenv("CLAUDE_CONFIG_DIR", "")
	t.Setenv("CLAUDE_ENV_FILE", "")

	claudeTestWrite(t, filepath.Join(home, ".claude", "settings.json"),
		`{"env":{"ANTHROPIC_AUTH_TOKEN":"secret"}}`)
	if _, err := CreateSettingsFile("runme", false); err != nil {
		t.Fatalf("CreateSettingsFile: %v", err)
	}
	if err := SaveSettingsFile("runme", map[string]any{
		"env": map[string]any{claudeAuthTokenEnv: "secret"},
	}); err != nil {
		t.Fatalf("SaveSettingsFile: %v", err)
	}

	preview, err := PreviewTemporaryRunFromFile("runme")
	if err != nil {
		t.Fatalf("PreviewTemporaryRunFromFile: %v", err)
	}

	if preview.ProfileID != "runme" || preview.ProfileName != "runme" {
		t.Fatalf("profile identity wrong: %+v", preview)
	}
	// The preview renders the same direct plan launch executes:
	// `claude --settings <runtime copy>` (generated, never materialised here).
	if preview.Program != "claude" {
		t.Fatalf("program should be the direct claude command: %+v", preview.Program)
	}
	settingsPath := filepath.Join(home, ".droidgear", "claude-settings", "runme.json")
	runtimeRoot := filepath.Join(home, ".droidgear", "runtime", "claude")
	if preview.LiveConfigDir != settingsPath {
		t.Fatalf("live config dir should fall back to the settings path: %q", preview.LiveConfigDir)
	}
	if len(preview.Args) != 2 || preview.Args[0] != "--settings" {
		t.Fatalf("args wrong: %+v", preview.Args)
	}
	runtimeCopy := preview.Args[1]
	if !strings.HasPrefix(runtimeCopy, runtimeRoot+string(filepath.Separator)) {
		t.Fatalf("args should point at a runtime copy: %q", runtimeCopy)
	}
	if filepath.Base(runtimeCopy) != "claude-settings.json" || !strings.Contains(filepath.Base(filepath.Dir(runtimeCopy)), "temporary-run-") {
		t.Fatalf("runtime copy path wrong: %q", runtimeCopy)
	}
	if preview.ChildProgram != "claude" || len(preview.ChildArgs) != 2 ||
		preview.ChildArgs[0] != "--settings" || preview.ChildArgs[1] != runtimeCopy {
		t.Fatalf("child command should match the direct plan: %+v", preview)
	}
	if claudePathExists(runtimeCopy) {
		t.Fatalf("preview must not materialise the runtime copy: %s", runtimeCopy)
	}
	if len(preview.SecretEnvKeys) != 0 {
		t.Fatalf("settings preview must not expose secret env keys: %+v", preview.SecretEnvKeys)
	}
	if preview.SettingsOverlayJSON == "" || !strings.Contains(preview.SettingsOverlayJSON, claudeAuthTokenEnv) {
		t.Fatalf("overlay json should carry the file contents: %q", preview.SettingsOverlayJSON)
	}

	unset := map[string]bool{}
	for _, key := range preview.UnsetEnv {
		unset[key] = true
	}
	for _, key := range []string{"CLAUDE_CONFIG_DIR", "CLAUDE_ENV_FILE", claudeAuthTokenEnv, claudeAPIKeyEnv, "CLAUDE_CODE_USE_BEDROCK"} {
		if !unset[key] {
			t.Fatalf("unset env should contain %s: %v", key, preview.UnsetEnv)
		}
	}
	for i := 1; i < len(preview.UnsetEnv); i++ {
		if preview.UnsetEnv[i-1] > preview.UnsetEnv[i] {
			t.Fatalf("unset env must be sorted: %v", preview.UnsetEnv)
		}
	}
	if len(preview.Env) != 0 {
		t.Fatalf("no CLAUDE_CONFIG_DIR override expected: %+v", preview.Env)
	}
	if len(preview.Warnings) != 0 {
		t.Fatalf("unexpected warnings: %+v", preview.Warnings)
	}
}

func TestClaudePreviewHonoursConfigDirOverrideAndEnvFileWarning(t *testing.T) {
	home := claudeTestHome(t)
	t.Setenv("CLAUDE_CONFIG_DIR", "")

	claudeTestWrite(t, filepath.Join(home, ".claude", "settings.json"), "{}")
	customDir := filepath.Join(home, "custom-claude")
	if err := core.SaveConfigPathOverride("claude", customDir); err != nil {
		t.Fatalf("save path override: %v", err)
	}
	t.Setenv("CLAUDE_ENV_FILE", filepath.Join(home, "missing.env"))

	preview, err := PreviewTemporaryRunFromFile("Global")
	if err != nil {
		t.Fatalf("PreviewTemporaryRunFromFile: %v", err)
	}
	if preview.LiveConfigDir != customDir {
		t.Fatalf("live config dir should be the override: %q", preview.LiveConfigDir)
	}
	if preview.Program != "claude" {
		t.Fatalf("program should be the direct claude command: %+v", preview.Program)
	}
	if len(preview.Env) != 1 || preview.Env[0][0] != "CLAUDE_CONFIG_DIR" || preview.Env[0][1] != customDir {
		t.Fatalf("visible env should carry the override: %+v", preview.Env)
	}
	if len(preview.Warnings) != 1 || !strings.Contains(preview.Warnings[0], "Failed to copy inherited CLAUDE_ENV_FILE") {
		t.Fatalf("expected env-file warning, got %+v", preview.Warnings)
	}
	if preview.InheritedEnvFileSource == nil || *preview.InheritedEnvFileSource != filepath.Join(home, "missing.env") {
		t.Fatalf("inherited env file source wrong: %v", preview.InheritedEnvFileSource)
	}
}

// ---------------------------------------------------------------------------
// Launch plan, runtime materialisation & wrappers (no terminal is spawned)
// ---------------------------------------------------------------------------

func TestClaudeSettingsLaunchPlanIsDirectCommand(t *testing.T) {
	home := claudeTestHome(t)
	t.Setenv("CLAUDE_CONFIG_DIR", "")
	t.Setenv("CLAUDE_ENV_FILE", "")

	plan, err := claudeBuildSettingsLaunchPlan(false)
	if err != nil {
		t.Fatalf("claudeBuildSettingsLaunchPlan: %v", err)
	}
	if plan.program != "claude" {
		t.Fatalf("program should be claude: %q", plan.program)
	}
	if len(plan.args) != 2 || plan.args[0] != "--settings" {
		t.Fatalf("args should be [--settings <runtime copy>]: %v", plan.args)
	}
	runtimeRoot := filepath.Join(home, ".droidgear", "runtime", "claude")
	runtimeCopy := plan.args[1]
	if !strings.HasPrefix(runtimeCopy, runtimeRoot+string(filepath.Separator)) {
		t.Fatalf("runtime copy should live under %s: %q", runtimeRoot, runtimeCopy)
	}
	if filepath.Base(runtimeCopy) != "claude-settings.json" {
		t.Fatalf("runtime copy file name wrong: %q", runtimeCopy)
	}
	if plan.runtimeDir == "" || !strings.Contains(filepath.Base(plan.runtimeDir), "temporary-run-") {
		t.Fatalf("runtime dir name wrong: %q", plan.runtimeDir)
	}
	if len(plan.env) != 0 || len(plan.warnings) != 0 || plan.inheritedEnvFile != nil {
		t.Fatalf("no override/env-file expected: %+v", plan)
	}

	dangerous, err := claudeBuildSettingsLaunchPlan(true)
	if err != nil {
		t.Fatalf("claudeBuildSettingsLaunchPlan(true): %v", err)
	}
	if len(dangerous.args) != 3 || dangerous.args[0] != "--dangerously-skip-permissions" ||
		dangerous.args[1] != "--settings" {
		t.Fatalf("skipDangerous args wrong: %v", dangerous.args)
	}

	unset := map[string]bool{}
	for _, key := range plan.unsetEnv {
		unset[key] = true
	}
	for _, key := range []string{"CLAUDE_CONFIG_DIR", "CLAUDE_ENV_FILE", claudeAuthTokenEnv, claudeAPIKeyEnv, "CLAUDE_CODE_USE_BEDROCK"} {
		if !unset[key] {
			t.Fatalf("unset env should contain %s: %v", key, plan.unsetEnv)
		}
	}
}

func TestClaudeMaterializeSettingsRuntimeCopiesSettings(t *testing.T) {
	home := claudeTestHome(t)
	t.Setenv("CLAUDE_CONFIG_DIR", "")
	t.Setenv("CLAUDE_ENV_FILE", "")

	settingsPath := filepath.Join(home, ".claude", "settings.json")
	claudeTestWrite(t, settingsPath, `{"env":{"ANTHROPIC_AUTH_TOKEN":"secret"}}`)

	plan, err := claudeBuildSettingsLaunchPlan(false)
	if err != nil {
		t.Fatalf("claudeBuildSettingsLaunchPlan: %v", err)
	}
	if err := claudeMaterializeSettingsRuntime(&plan, settingsPath); err != nil {
		t.Fatalf("claudeMaterializeSettingsRuntime: %v", err)
	}

	runtimeCopy := filepath.Join(plan.runtimeDir, claudeRuntimeSettingsFile)
	if !claudePathExists(runtimeCopy) {
		t.Fatalf("runtime copy missing: %s", runtimeCopy)
	}
	if got := claudeTestRead(t, runtimeCopy); !strings.Contains(got, "ANTHROPIC_AUTH_TOKEN") {
		t.Fatalf("runtime copy lost contents: %s", got)
	}
	// The live configuration is never touched.
	if got := claudeTestRead(t, settingsPath); got != `{"env":{"ANTHROPIC_AUTH_TOKEN":"secret"}}` {
		t.Fatalf("live settings mutated: %s", got)
	}

	// A missing source yields the Rust "{}\n" placeholder.
	plan2, err := claudeBuildSettingsLaunchPlan(false)
	if err != nil {
		t.Fatalf("claudeBuildSettingsLaunchPlan (2): %v", err)
	}
	absent := filepath.Join(home, ".claude", "absent.json")
	if err := claudeMaterializeSettingsRuntime(&plan2, absent); err != nil {
		t.Fatalf("claudeMaterializeSettingsRuntime (absent): %v", err)
	}
	if got := claudeTestRead(t, filepath.Join(plan2.runtimeDir, claudeRuntimeSettingsFile)); got != "{}\n" {
		t.Fatalf("placeholder should be %q, got %q", "{}\n", got)
	}
}

func TestClaudeMaterializeSettingsRuntimeCopiesInheritedEnvFile(t *testing.T) {
	home := claudeTestHome(t)
	t.Setenv("CLAUDE_CONFIG_DIR", "")
	envFile := filepath.Join(home, "inherited.env")
	claudeTestWrite(t, envFile, "export EXAMPLE=1\n")
	t.Setenv("CLAUDE_ENV_FILE", envFile)

	settingsPath := filepath.Join(home, ".claude", "settings.json")
	claudeTestWrite(t, settingsPath, "{}")

	plan, err := claudeBuildSettingsLaunchPlan(false)
	if err != nil {
		t.Fatalf("claudeBuildSettingsLaunchPlan: %v", err)
	}
	if plan.inheritedEnvFile == nil || *plan.inheritedEnvFile != envFile {
		t.Fatalf("inherited env file wrong: %v", plan.inheritedEnvFile)
	}
	if err := claudeMaterializeSettingsRuntime(&plan, settingsPath); err != nil {
		t.Fatalf("claudeMaterializeSettingsRuntime: %v", err)
	}

	copied := filepath.Join(plan.runtimeDir, claudeRuntimeEnvFile)
	if got := claudeTestRead(t, copied); got != "export EXAMPLE=1\n" {
		t.Fatalf("env file copy wrong: %q", got)
	}
	found := false
	for _, pair := range plan.env {
		if pair[0] == "CLAUDE_ENV_FILE" && pair[1] == copied {
			found = true
		}
	}
	if !found {
		t.Fatalf("env should point at the copy: %+v", plan.env)
	}

	// An unreadable env file only warns — the run continues without it.
	t.Setenv("CLAUDE_ENV_FILE", filepath.Join(home, "missing.env"))
	warnPlan, err := claudeBuildSettingsLaunchPlan(false)
	if err != nil {
		t.Fatalf("claudeBuildSettingsLaunchPlan (warn): %v", err)
	}
	if len(warnPlan.warnings) != 1 || !strings.Contains(warnPlan.warnings[0], "Failed to copy inherited CLAUDE_ENV_FILE") {
		t.Fatalf("expected env-file warning, got %+v", warnPlan.warnings)
	}
	if err := claudeMaterializeSettingsRuntime(&warnPlan, settingsPath); err != nil {
		t.Fatalf("claudeMaterializeSettingsRuntime (warn): %v", err)
	}
	for _, pair := range warnPlan.env {
		if pair[0] == "CLAUDE_ENV_FILE" {
			t.Fatalf("env file must not be inherited: %+v", warnPlan.env)
		}
	}
}

func TestClaudeWriteSecurePowerShellWrapperRendersScript(t *testing.T) {
	home := claudeTestHome(t)
	cwd := filepath.Join(home, "work tree")
	spec := claudeLaunchSpec{
		program:    "claude",
		args:       []string{"--settings", filepath.Join(home, "runtime", "claude-settings.json")},
		env:        [][2]string{{"CLAUDE_CONFIG_DIR", filepath.Join(home, ".claude")}},
		unsetEnv:   []string{"ANTHROPIC_AUTH_TOKEN", "CLAUDE_CONFIG_DIR"},
		warnings:   []string{"inherited env file missing"},
		cwd:        &cwd,
		supportDir: filepath.Join(home, ".droidgear", "runtime", "claude", "temporary-run-test"),
		noKeepOpen: true,
	}

	path, err := claudeWriteSecurePowerShellWrapper(&spec)
	if err != nil {
		t.Fatalf("claudeWriteSecurePowerShellWrapper: %v", err)
	}
	script := claudeTestRead(t, path)
	for _, want := range []string{
		"Clear-Host",
		"Write-Host 'Starting claude...'",
		"Warning: inherited env file missing",
		"Remove-Item Env:ANTHROPIC_AUTH_TOKEN -ErrorAction SilentlyContinue",
		"$env:CLAUDE_CONFIG_DIR = '" + filepath.Join(home, ".claude") + "'",
		"Set-Location '" + cwd + "'",
		"& 'claude' '--settings'",
		"Remove-Item -LiteralPath $PSCommandPath",
	} {
		if !strings.Contains(script, want) {
			t.Fatalf("wrapper missing %q:\n%s", want, script)
		}
	}

	// The wrapper is the only artefact — no sibling temp files left behind.
	entries, err := os.ReadDir(filepath.Dir(path))
	if err != nil {
		t.Fatalf("read runtime dir: %v", err)
	}
	if len(entries) != 1 || entries[0].Name() != filepath.Base(path) {
		names := []string{}
		for _, entry := range entries {
			names = append(names, entry.Name())
		}
		t.Fatalf("unexpected runtime artefacts: %v", names)
	}
}

func TestClaudeCleanupStaleRuntimeDirs(t *testing.T) {
	home := claudeTestHome(t)
	root := filepath.Join(home, ".droidgear", "runtime", "claude")
	stale := filepath.Join(root, "temporary-run-old")
	fresh := filepath.Join(root, "temporary-run-new")
	other := filepath.Join(root, "keep-me")
	for _, dir := range []string{stale, fresh, other} {
		claudeTestWrite(t, filepath.Join(dir, "claude-settings.json"), "{}")
	}
	old := time.Now().Add(-48 * time.Hour)
	if err := os.Chtimes(stale, old, old); err != nil {
		t.Fatalf("chtimes: %v", err)
	}

	claudeCleanupStaleRuntimeDirs()

	if claudePathExists(stale) {
		t.Fatalf("stale runtime dir must be removed: %s", stale)
	}
	if !claudePathExists(fresh) {
		t.Fatalf("fresh runtime dir must survive: %s", fresh)
	}
	if !claudePathExists(other) {
		t.Fatalf("non-temporary-run dir must survive: %s", other)
	}
}

func TestClaudePreparePosixCommandRendersEnvAndCwd(t *testing.T) {
	cwd := "/work tree"
	spec := claudeLaunchSpec{
		program:  "claude",
		args:     []string{"--settings", "/tmp/runtime/claude-settings.json"},
		unsetEnv: []string{"ANTHROPIC_AUTH_TOKEN"},
		env:      [][2]string{{"CLAUDE_CONFIG_DIR", "/home/u/.claude"}},
		cwd:      &cwd,
	}

	command, keepOpen := claudePreparePosixCommand(&spec)
	for _, want := range []string{
		"unset ANTHROPIC_AUTH_TOKEN",
		"export CLAUDE_CONFIG_DIR='/home/u/.claude'",
		"cd '/work tree'",
		"'claude' '--settings' '/tmp/runtime/claude-settings.json'",
	} {
		if !strings.Contains(command, want) {
			t.Fatalf("command missing %q: %s", want, command)
		}
	}
	if !strings.HasSuffix(keepOpen, "; exec bash") {
		t.Fatalf("keep-open command should exec bash: %s", keepOpen)
	}
}

func TestClaudeWindowsProgramCandidatesPreferExecutables(t *testing.T) {
	got := strings.Join(claudeWindowsProgramCandidates("claude"), ",")
	if got != "claude.exe,claude.cmd,claude.bat,claude.ps1" {
		t.Fatalf("bare program candidates wrong: %s", got)
	}
	single := claudeWindowsProgramCandidates("tool.exe")
	if len(single) != 1 || single[0] != "tool.exe" {
		t.Fatalf("known extension should pass through: %v", single)
	}
}

// claudeTestHome returns the isolated home created by testutil.IsolatedHome.
func claudeTestHome(t *testing.T) string {
	t.Helper()
	return testutil.IsolatedHome(t)
}

func claudeTestWrite(t *testing.T, path string, content string) {
	t.Helper()
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		t.Fatalf("mkdir %s: %v", filepath.Dir(path), err)
	}
	if err := os.WriteFile(path, []byte(content), 0o644); err != nil {
		t.Fatalf("write %s: %v", path, err)
	}
}

func claudeTestRead(t *testing.T, path string) string {
	t.Helper()
	data, err := os.ReadFile(path) // #nosec G304 -- test fixture
	if err != nil {
		t.Fatalf("read %s: %v", path, err)
	}
	return string(data)
}

func claudeTestReadJSON(t *testing.T, path string) map[string]any {
	t.Helper()
	var out map[string]any
	if err := json.Unmarshal([]byte(claudeTestRead(t, path)), &out); err != nil {
		t.Fatalf("parse %s: %v", path, err)
	}
	return out
}

func claudeTestProfile(id string, name string) ClaudeCodeProfile {
	return ClaudeCodeProfile{
		ID:              id,
		Name:            name,
		Description:     core.StrPtr("A test profile"),
		BaseURL:         core.StrPtr("https://example.com/v1"),
		BearerToken:     core.StrPtr("secret-token"),
		Model:           core.StrPtr("claude-sonnet-4-5"),
		ReasoningEffort: func() *ClaudeReasoningEffort { e := ClaudeReasoningEffortHigh; return &e }(),
		ThinkingMode:    ClaudeThinkingModeOn,
		CreatedAt:       "2026-01-01T00:00:00Z",
		UpdatedAt:       "2026-01-01T00:00:00Z",
	}
}
