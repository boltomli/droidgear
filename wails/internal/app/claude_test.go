package app

import (
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"droidgear-wails/internal/core"
)

// claudeTestHome returns the isolated home created by isolatedHome(t).
func claudeTestHome(t *testing.T) string {
	t.Helper()
	return isolatedHome(t)
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
		Description:     strPtr("A test profile"),
		BaseURL:         strPtr("https://example.com/v1"),
		BearerToken:     strPtr("secret-token"),
		Model:           strPtr("claude-sonnet-4-5"),
		ReasoningEffort: func() *ClaudeReasoningEffort { e := ClaudeReasoningEffortHigh; return &e }(),
		ThinkingMode:    ClaudeThinkingModeOn,
		CreatedAt:       "2026-01-01T00:00:00Z",
		UpdatedAt:       "2026-01-01T00:00:00Z",
	}
}

// ---------------------------------------------------------------------------
// Types & defaults
// ---------------------------------------------------------------------------

func TestClaudeProfileDeserializationAppliesDefaults(t *testing.T) {
	data := []byte(`{
		"id": "test-id",
		"name": "Test Profile",
		"createdAt": "2026-01-01T00:00:00Z",
		"updatedAt": "2026-01-01T00:00:00Z"
	}`)

	var profile ClaudeCodeProfile
	if err := json.Unmarshal(data, &profile); err != nil {
		t.Fatalf("unmarshal: %v", err)
	}
	if profile.SmallModelUsesMainModel {
		t.Fatalf("smallModelUsesMainModel should default to false: %+v", profile)
	}
	if profile.ThinkingMode != ClaudeThinkingModeInherit {
		t.Fatalf("thinkingMode should default to inherit, got %q", profile.ThinkingMode)
	}
	if profile.ReasoningEffort != nil {
		t.Fatalf("reasoningEffort should default to nil, got %v", *profile.ReasoningEffort)
	}
}

func TestClaudeProfileUnmarshalRejectsUnknownEnumVariants(t *testing.T) {
	badThinking := []byte(`{"id":"a","name":"n","thinkingMode":"sideways","createdAt":"x","updatedAt":"y"}`)
	if err := json.Unmarshal(badThinking, &ClaudeCodeProfile{}); err == nil || !strings.Contains(err.Error(), "unknown variant") {
		t.Fatalf("expected unknown-variant error for thinkingMode, got %v", err)
	}

	badEffort := []byte(`{"id":"a","name":"n","reasoningEffort":"ultra","createdAt":"x","updatedAt":"y"}`)
	if err := json.Unmarshal(badEffort, &ClaudeCodeProfile{}); err == nil || !strings.Contains(err.Error(), "unknown variant") {
		t.Fatalf("expected unknown-variant error for reasoningEffort, got %v", err)
	}
}

func TestClaudeProfileRoundTripOmitsEmptyOptionals(t *testing.T) {
	profile := ClaudeCodeProfile{
		ID: "p1", Name: "Only basics", ThinkingMode: ClaudeThinkingModeInherit,
		CreatedAt: "2026-01-01T00:00:00Z", UpdatedAt: "2026-01-01T00:00:00Z",
	}
	data, err := json.Marshal(profile)
	if err != nil {
		t.Fatalf("marshal: %v", err)
	}
	text := string(data)
	for _, key := range []string{`"description"`, `"baseUrl"`, `"bearerToken"`, `"model"`, `"smallModel"`, `"reasoningEffort"`} {
		if strings.Contains(text, key) {
			t.Fatalf("optional key %s should be omitted when empty: %s", key, text)
		}
	}
	if !strings.Contains(text, `"smallModelUsesMainModel":false`) {
		t.Fatalf("smallModelUsesMainModel must always be serialized: %s", text)
	}
	if !strings.Contains(text, `"thinkingMode":"inherit"`) {
		t.Fatalf("thinkingMode must be serialized: %s", text)
	}
}

// ---------------------------------------------------------------------------
// Profile CRUD
// ---------------------------------------------------------------------------

func TestClaudeCreateDefaultProfile(t *testing.T) {
	claudeTestHome(t)
	a := &App{}

	profile, err := a.CreateDefaultClaudeProfile()
	if err != nil {
		t.Fatalf("CreateDefaultClaudeProfile: %v", err)
	}
	if profile.ID == "" || profile.Name != "Default" {
		t.Fatalf("unexpected default profile: %+v", profile)
	}
	if profile.ThinkingMode != ClaudeThinkingModeInherit {
		t.Fatalf("default thinking mode should be inherit: %+v", profile)
	}

	_, err = a.CreateDefaultClaudeProfile()
	if err == nil || err.Error() != "Profiles already exist" {
		t.Fatalf("expected 'Profiles already exist', got %v", err)
	}
}

func TestClaudeSaveAndGetProfile(t *testing.T) {
	claudeTestHome(t)
	a := &App{}

	if err := a.SaveClaudeProfile(claudeTestProfile("p1", "Profile 1")); err != nil {
		t.Fatalf("SaveClaudeProfile: %v", err)
	}
	loaded, err := a.GetClaudeProfile("p1")
	if err != nil {
		t.Fatalf("GetClaudeProfile: %v", err)
	}
	if loaded.Name != "Profile 1" || loaded.Model == nil || *loaded.Model != "claude-sonnet-4-5" {
		t.Fatalf("loaded profile mismatch: %+v", loaded)
	}

	// Invalid ids are rejected with the Rust error string.
	bad := claudeTestProfile("bad id", "Bad")
	if err := a.SaveClaudeProfile(bad); err == nil || err.Error() != "Invalid profile id" {
		t.Fatalf("expected 'Invalid profile id', got %v", err)
	}
}

func TestClaudeSaveGeneratesIDWhenEmpty(t *testing.T) {
	claudeTestHome(t)
	a := &App{}

	if err := a.SaveClaudeProfile(ClaudeCodeProfile{Name: "New Profile"}); err != nil {
		t.Fatalf("SaveClaudeProfile: %v", err)
	}
	profiles, err := a.ListClaudeProfiles()
	if err != nil {
		t.Fatalf("ListClaudeProfiles: %v", err)
	}
	if len(profiles) != 1 {
		t.Fatalf("expected 1 profile, got %d", len(profiles))
	}
	if profiles[0].ID == "" || profiles[0].CreatedAt == "" || profiles[0].UpdatedAt == "" {
		t.Fatalf("generated fields missing: %+v", profiles[0])
	}
}

func TestClaudeSaveKeepsCreatedAtAndRefreshesUpdatedAt(t *testing.T) {
	claudeTestHome(t)
	a := &App{}

	if err := a.SaveClaudeProfile(claudeTestProfile("p1", "Profile 1")); err != nil {
		t.Fatalf("SaveClaudeProfile: %v", err)
	}
	first, err := a.GetClaudeProfile("p1")
	if err != nil {
		t.Fatalf("GetClaudeProfile: %v", err)
	}

	updated := first
	updated.Name = "Updated Name"
	// createdAt travels with the payload but must be restored from disk;
	// updated_at is always rewritten from now (nowRFC3339 has second
	// precision, so the witness is the stale payload value).
	updated.CreatedAt = "1999-01-01T00:00:00Z"
	updated.UpdatedAt = "1999-01-01T00:00:00Z"
	if err := a.SaveClaudeProfile(updated); err != nil {
		t.Fatalf("SaveClaudeProfile (update): %v", err)
	}

	reloaded, err := a.GetClaudeProfile("p1")
	if err != nil {
		t.Fatalf("GetClaudeProfile (reload): %v", err)
	}
	if reloaded.Name != "Updated Name" {
		t.Fatalf("name not updated: %+v", reloaded)
	}
	if reloaded.CreatedAt != first.CreatedAt {
		t.Fatalf("createdAt must be preserved: %q vs %q", reloaded.CreatedAt, first.CreatedAt)
	}
	if reloaded.UpdatedAt == "1999-01-01T00:00:00Z" {
		t.Fatalf("updatedAt must be rewritten from now: %q", reloaded.UpdatedAt)
	}
}

func TestClaudeListProfilesSortedByName(t *testing.T) {
	home := claudeTestHome(t)
	a := &App{}

	for _, entry := range [][2]string{{"p1", "Zebra"}, {"p2", "Alpha"}, {"p3", "Middle"}} {
		if err := a.SaveClaudeProfile(claudeTestProfile(entry[0], entry[1])); err != nil {
			t.Fatalf("SaveClaudeProfile: %v", err)
		}
	}
	profiles, err := a.ListClaudeProfiles()
	if err != nil {
		t.Fatalf("ListClaudeProfiles: %v", err)
	}
	if len(profiles) != 3 {
		t.Fatalf("expected 3 profiles, got %d", len(profiles))
	}
	names := []string{profiles[0].Name, profiles[1].Name, profiles[2].Name}
	if names[0] != "Alpha" || names[1] != "Middle" || names[2] != "Zebra" {
		t.Fatalf("wrong sort order: %v", names)
	}

	// Non-JSON and unparsable files are skipped silently.
	claudeTestWrite(t, filepath.Join(home, ".droidgear", "claude", "profiles", "notes.txt"), "ignore me")
	claudeTestWrite(t, filepath.Join(home, ".droidgear", "claude", "profiles", "broken.json"), "{not-json")
	profiles, err = a.ListClaudeProfiles()
	if err != nil {
		t.Fatalf("ListClaudeProfiles (2): %v", err)
	}
	if len(profiles) != 3 {
		t.Fatalf("broken/non-json entries must be skipped, got %d", len(profiles))
	}
}

func TestClaudeDuplicateProfile(t *testing.T) {
	claudeTestHome(t)
	a := &App{}

	if err := a.SaveClaudeProfile(claudeTestProfile("p1", "Original")); err != nil {
		t.Fatalf("SaveClaudeProfile: %v", err)
	}
	duplicate, err := a.DuplicateClaudeProfile("p1", "Copy")
	if err != nil {
		t.Fatalf("DuplicateClaudeProfile: %v", err)
	}
	if duplicate.ID == "p1" || duplicate.Name != "Copy" {
		t.Fatalf("unexpected duplicate: %+v", duplicate)
	}
	if duplicate.BaseURL == nil || *duplicate.BaseURL != "https://example.com/v1" {
		t.Fatalf("duplicate lost base_url: %+v", duplicate)
	}

	profiles, err := a.ListClaudeProfiles()
	if err != nil || len(profiles) != 2 {
		t.Fatalf("expected 2 profiles, got %d (err=%v)", len(profiles), err)
	}
}

func TestClaudeDeleteProfileClearsActivePointer(t *testing.T) {
	claudeTestHome(t)
	a := &App{}

	if err := a.SaveClaudeProfile(claudeTestProfile("p1", "Profile 1")); err != nil {
		t.Fatalf("SaveClaudeProfile: %v", err)
	}
	if err := a.ApplyClaudeProfile("p1"); err != nil {
		t.Fatalf("ApplyClaudeProfile: %v", err)
	}

	if err := a.DeleteClaudeProfile("p1"); err != nil {
		t.Fatalf("DeleteClaudeProfile: %v", err)
	}
	profiles, err := a.ListClaudeProfiles()
	if err != nil || len(profiles) != 0 {
		t.Fatalf("expected 0 profiles, got %d (err=%v)", len(profiles), err)
	}
	active, err := a.GetActiveClaudeProfileId()
	if err != nil {
		t.Fatalf("GetActiveClaudeProfileId: %v", err)
	}
	if active != nil {
		t.Fatalf("active pointer should be cleared, got %q", *active)
	}
}

func TestClaudeActiveProfileRoundtripAndValidation(t *testing.T) {
	claudeTestHome(t)
	a := &App{}

	active, err := a.GetActiveClaudeProfileId()
	if err != nil || active != nil {
		t.Fatalf("expected nil active id, got %v (err=%v)", active, err)
	}

	if err := claudeSetActiveProfileID("profile_1"); err != nil {
		t.Fatalf("set active: %v", err)
	}
	active, err = a.GetActiveClaudeProfileId()
	if err != nil || active == nil || *active != "profile_1" {
		t.Fatalf("active id mismatch: %v (err=%v)", active, err)
	}

	if err := claudeSetActiveProfileID("bad id"); err == nil || err.Error() != "Invalid profile id" {
		t.Fatalf("expected 'Invalid profile id', got %v", err)
	}

	if err := claudeSetActiveProfileID(""); err != nil {
		t.Fatalf("clear active: %v", err)
	}
	active, err = a.GetActiveClaudeProfileId()
	if err != nil || active != nil {
		t.Fatalf("expected cleared active id, got %v (err=%v)", active, err)
	}
}

// ---------------------------------------------------------------------------
// Apply / status / current config
// ---------------------------------------------------------------------------

func TestClaudeApplyCreatesSettingsFileWhenMissing(t *testing.T) {
	home := claudeTestHome(t)
	a := &App{}

	if err := a.SaveClaudeProfile(claudeTestProfile("p1", "Profile 1")); err != nil {
		t.Fatalf("SaveClaudeProfile: %v", err)
	}
	if err := a.ApplyClaudeProfile("p1"); err != nil {
		t.Fatalf("ApplyClaudeProfile: %v", err)
	}

	settingsPath := filepath.Join(home, ".claude", "settings.json")
	settings := claudeTestReadJSON(t, settingsPath)
	env, ok := settings["env"].(map[string]any)
	if !ok {
		t.Fatalf("env missing: %s", claudeTestRead(t, settingsPath))
	}
	checks := map[string]string{
		claudeBaseURLEnv:         "https://example.com/v1",
		claudeAuthTokenEnv:       "secret-token",
		claudeModelEnv:           "claude-sonnet-4-5",
		claudeEffortEnv:          "high",
		claudeDisableAdaptiveEnv: "1",
	}
	for key, want := range checks {
		if got, _ := env[key].(string); got != want {
			t.Fatalf("env[%s] = %q, want %q (settings: %s)", key, got, want, claudeTestRead(t, settingsPath))
		}
	}
	if enabled, _ := settings["alwaysThinkingEnabled"].(bool); !enabled {
		t.Fatalf("alwaysThinkingEnabled should be true: %s", claudeTestRead(t, settingsPath))
	}

	active, err := a.GetActiveClaudeProfileId()
	if err != nil || active == nil || *active != "p1" {
		t.Fatalf("active id should be p1, got %v (err=%v)", active, err)
	}
}

func TestClaudeApplyPreservesUnrelatedSettingsAndCleansConflicts(t *testing.T) {
	home := claudeTestHome(t)
	a := &App{}

	settingsPath := filepath.Join(home, ".claude", "settings.json")
	claudeTestWrite(t, settingsPath, `{
		"theme": "dark",
		"model": "top-level-model",
		"env": {
			"KEEP_ME": "1",
			"ANTHROPIC_API_KEY": "bad",
			"CLAUDE_CODE_USE_BEDROCK": "1",
			"CLAUDE_CODE_DISABLE_THINKING": "1",
			"MAX_THINKING_TOKENS": "4096"
		}
	}`)

	profile := claudeTestProfile("p1", "Profile 1")
	effort := ClaudeReasoningEffortMedium
	profile.ReasoningEffort = &effort
	profile.ThinkingMode = ClaudeThinkingModeOn
	if err := a.SaveClaudeProfile(profile); err != nil {
		t.Fatalf("SaveClaudeProfile: %v", err)
	}
	if err := a.ApplyClaudeProfile("p1"); err != nil {
		t.Fatalf("ApplyClaudeProfile: %v", err)
	}

	settings := claudeTestReadJSON(t, settingsPath)
	if theme, _ := settings["theme"].(string); theme != "dark" {
		t.Fatalf("theme must survive: %s", claudeTestRead(t, settingsPath))
	}
	if model, _ := settings["model"].(string); model != "top-level-model" {
		t.Fatalf("top-level model must survive: %s", claudeTestRead(t, settingsPath))
	}
	env := settings["env"].(map[string]any)
	if keep, _ := env["KEEP_ME"].(string); keep != "1" {
		t.Fatalf("KEEP_ME must survive: %+v", env)
	}
	for _, key := range []string{claudeAPIKeyEnv, "CLAUDE_CODE_USE_BEDROCK", claudeDisableThinkingEnv, claudeMaxThinkingTokensEnv} {
		if _, found := env[key]; found {
			t.Fatalf("conflicting key %s must be removed: %+v", key, env)
		}
	}
	if effort, _ := env[claudeEffortEnv].(string); effort != "medium" {
		t.Fatalf("effort should be medium, got %v", env[claudeEffortEnv])
	}
	if _, found := env[claudeDisableAdaptiveEnv]; found {
		t.Fatalf("medium effort must not set %s: %+v", claudeDisableAdaptiveEnv, env)
	}
}

func TestClaudeApplyThinkingModesManageExpectedKeys(t *testing.T) {
	home := claudeTestHome(t)
	a := &App{}

	settingsPath := filepath.Join(home, ".claude", "settings.json")
	claudeTestWrite(t, settingsPath, `{
		"alwaysThinkingEnabled": false,
		"env": {"CLAUDE_CODE_DISABLE_THINKING": "1", "MAX_THINKING_TOKENS": "1024"}
	}`)

	profile := claudeTestProfile("p1", "Profile 1")
	profile.ReasoningEffort = nil
	profile.ThinkingMode = ClaudeThinkingModeInherit
	if err := a.SaveClaudeProfile(profile); err != nil {
		t.Fatalf("SaveClaudeProfile: %v", err)
	}
	if err := a.ApplyClaudeProfile("p1"); err != nil {
		t.Fatalf("ApplyClaudeProfile (inherit): %v", err)
	}
	settings := claudeTestReadJSON(t, settingsPath)
	if _, found := settings["alwaysThinkingEnabled"]; found {
		t.Fatalf("inherit must drop alwaysThinkingEnabled: %s", claudeTestRead(t, settingsPath))
	}
	env := settings["env"].(map[string]any)
	if _, found := env[claudeDisableThinkingEnv]; found {
		t.Fatalf("inherit must drop %s: %+v", claudeDisableThinkingEnv, env)
	}
	if tokens, _ := env[claudeMaxThinkingTokensEnv].(string); tokens != "1024" {
		t.Fatalf("MAX_THINKING_TOKENS must survive inherit: %+v", env)
	}

	profile.ThinkingMode = ClaudeThinkingModeOn
	if err := a.SaveClaudeProfile(profile); err != nil {
		t.Fatalf("SaveClaudeProfile (on): %v", err)
	}
	if err := a.ApplyClaudeProfile("p1"); err != nil {
		t.Fatalf("ApplyClaudeProfile (on): %v", err)
	}
	settings = claudeTestReadJSON(t, settingsPath)
	if enabled, _ := settings["alwaysThinkingEnabled"].(bool); !enabled {
		t.Fatalf("on must set alwaysThinkingEnabled=true: %s", claudeTestRead(t, settingsPath))
	}
	env = settings["env"].(map[string]any)
	for _, key := range []string{claudeDisableThinkingEnv, claudeMaxThinkingTokensEnv} {
		if _, found := env[key]; found {
			t.Fatalf("on must drop %s: %+v", key, env)
		}
	}

	profile.ThinkingMode = ClaudeThinkingModeOff
	if err := a.SaveClaudeProfile(profile); err != nil {
		t.Fatalf("SaveClaudeProfile (off): %v", err)
	}
	if err := a.ApplyClaudeProfile("p1"); err != nil {
		t.Fatalf("ApplyClaudeProfile (off): %v", err)
	}
	settings = claudeTestReadJSON(t, settingsPath)
	if enabled, _ := settings["alwaysThinkingEnabled"].(bool); enabled {
		t.Fatalf("off must set alwaysThinkingEnabled=false: %s", claudeTestRead(t, settingsPath))
	}
	env = settings["env"].(map[string]any)
	if value, _ := env[claudeDisableThinkingEnv].(string); value != "1" {
		t.Fatalf("off must set %s=1: %+v", claudeDisableThinkingEnv, env)
	}
	if _, found := env[claudeMaxThinkingTokensEnv]; found {
		t.Fatalf("off must drop %s: %+v", claudeMaxThinkingTokensEnv, env)
	}
}

func TestClaudeApplyReasoningEffortWritesAndClearsExpectedKeys(t *testing.T) {
	home := claudeTestHome(t)
	a := &App{}

	settingsPath := filepath.Join(home, ".claude", "settings.json")
	claudeTestWrite(t, settingsPath, `{"env":{"CLAUDE_CODE_EFFORT_LEVEL":"high","CLAUDE_CODE_DISABLE_ADAPTIVE_THINKING":"1"}}`)

	profile := claudeTestProfile("p1", "Profile 1")
	profile.ThinkingMode = ClaudeThinkingModeInherit

	low := ClaudeReasoningEffortLow
	profile.ReasoningEffort = &low
	if err := a.SaveClaudeProfile(profile); err != nil {
		t.Fatalf("SaveClaudeProfile (low): %v", err)
	}
	if err := a.ApplyClaudeProfile("p1"); err != nil {
		t.Fatalf("ApplyClaudeProfile (low): %v", err)
	}
	env := claudeTestReadJSON(t, settingsPath)["env"].(map[string]any)
	if value, _ := env[claudeEffortEnv].(string); value != "low" {
		t.Fatalf("effort should be low: %+v", env)
	}
	if _, found := env[claudeDisableAdaptiveEnv]; found {
		t.Fatalf("low must drop %s: %+v", claudeDisableAdaptiveEnv, env)
	}

	max := ClaudeReasoningEffortMax
	profile.ReasoningEffort = &max
	if err := a.SaveClaudeProfile(profile); err != nil {
		t.Fatalf("SaveClaudeProfile (max): %v", err)
	}
	if err := a.ApplyClaudeProfile("p1"); err != nil {
		t.Fatalf("ApplyClaudeProfile (max): %v", err)
	}
	env = claudeTestReadJSON(t, settingsPath)["env"].(map[string]any)
	if value, _ := env[claudeEffortEnv].(string); value != "max" {
		t.Fatalf("effort should be max: %+v", env)
	}
	if value, _ := env[claudeDisableAdaptiveEnv].(string); value != "1" {
		t.Fatalf("max must set %s=1: %+v", claudeDisableAdaptiveEnv, env)
	}

	profile.ReasoningEffort = nil
	if err := a.SaveClaudeProfile(profile); err != nil {
		t.Fatalf("SaveClaudeProfile (none): %v", err)
	}
	if err := a.ApplyClaudeProfile("p1"); err != nil {
		t.Fatalf("ApplyClaudeProfile (none): %v", err)
	}
	env = claudeTestReadJSON(t, settingsPath)["env"].(map[string]any)
	for _, key := range []string{claudeEffortEnv, claudeDisableAdaptiveEnv} {
		if _, found := env[key]; found {
			t.Fatalf("absent effort must drop %s: %+v", key, env)
		}
	}
}

func TestClaudeApplySmallModelUsesMainModelMirrors(t *testing.T) {
	home := claudeTestHome(t)
	a := &App{}

	settingsPath := filepath.Join(home, ".claude", "settings.json")
	claudeTestWrite(t, settingsPath, `{"env":{"ANTHROPIC_DEFAULT_HAIKU_MODEL":"claude-haiku-4"}}`)

	profile := claudeTestProfile("p1", "Profile 1")
	profile.SmallModelUsesMainModel = true
	profile.SmallModel = strPtr("claude-haiku-4")
	if err := a.SaveClaudeProfile(profile); err != nil {
		t.Fatalf("SaveClaudeProfile: %v", err)
	}
	if err := a.ApplyClaudeProfile("p1"); err != nil {
		t.Fatalf("ApplyClaudeProfile: %v", err)
	}

	env := claudeTestReadJSON(t, settingsPath)["env"].(map[string]any)
	if value, _ := env[claudeSmallModelEnv].(string); value != "claude-sonnet-4-5" {
		t.Fatalf("small model should mirror the main model, got %v", env[claudeSmallModelEnv])
	}
}

func TestClaudeApplyReturnsErrorForMalformedSettingsJSON(t *testing.T) {
	home := claudeTestHome(t)
	a := &App{}

	settingsPath := filepath.Join(home, ".claude", "settings.json")
	claudeTestWrite(t, settingsPath, "{not-json")
	if err := a.SaveClaudeProfile(claudeTestProfile("p1", "Profile 1")); err != nil {
		t.Fatalf("SaveClaudeProfile: %v", err)
	}

	err := a.ApplyClaudeProfile("p1")
	if err == nil || !strings.Contains(err.Error(), "Failed to parse settings.json") {
		t.Fatalf("expected parse error, got %v", err)
	}
}

func TestClaudeConfigStatusReportsParseErrors(t *testing.T) {
	home := claudeTestHome(t)
	a := &App{}

	settingsPath := filepath.Join(home, ".claude", "settings.json")
	claudeTestWrite(t, settingsPath, "{not-json")

	status, err := a.GetClaudeConfigStatus()
	if err != nil {
		t.Fatalf("GetClaudeConfigStatus: %v", err)
	}
	if !status.SettingsExists {
		t.Fatalf("settings should exist: %+v", status)
	}
	if status.ParseError == nil || !strings.Contains(*status.ParseError, "Failed to parse settings.json") {
		t.Fatalf("expected parse error, got %+v", status)
	}
	if !strings.HasSuffix(filepath.ToSlash(status.SettingsPath), ".claude/settings.json") {
		t.Fatalf("unexpected settings path: %s", status.SettingsPath)
	}
	if !strings.HasSuffix(filepath.ToSlash(status.ConfigDir), ".claude") {
		t.Fatalf("unexpected config dir: %s", status.ConfigDir)
	}
}

func TestClaudeConfigDirHonorsPathOverride(t *testing.T) {
	home := claudeTestHome(t)
	a := &App{}

	customDir := filepath.Join(home, "custom-claude")
	if err := core.SaveConfigPathOverride("claude", customDir); err != nil {
		t.Fatalf("save path override: %v", err)
	}

	status, err := a.GetClaudeConfigStatus()
	if err != nil {
		t.Fatalf("GetClaudeConfigStatus: %v", err)
	}
	if status.ConfigDir != customDir {
		t.Fatalf("config dir should honour override, got %q want %q", status.ConfigDir, customDir)
	}
	if status.SettingsPath != filepath.Join(customDir, "settings.json") {
		t.Fatalf("settings path wrong: %s", status.SettingsPath)
	}
}

func TestClaudeReadCurrentConfigPrefersManagedEnvAndFallsBack(t *testing.T) {
	home := claudeTestHome(t)
	a := &App{}

	settingsPath := filepath.Join(home, ".claude", "settings.json")
	claudeTestWrite(t, settingsPath, `{
		"model": "top-level-model",
		"effortLevel": "max",
		"alwaysThinkingEnabled": true,
		"env": {
			"ANTHROPIC_BASE_URL": "https://proxy.example.com",
			"ANTHROPIC_AUTH_TOKEN": "bearer-123",
			"ANTHROPIC_MODEL": "env-model",
			"ANTHROPIC_DEFAULT_HAIKU_MODEL": "env-small",
			"CLAUDE_CODE_EFFORT_LEVEL": "medium"
		}
	}`)

	current, err := a.ReadClaudeCurrentConfig()
	if err != nil {
		t.Fatalf("ReadClaudeCurrentConfig: %v", err)
	}
	if current.BaseURL == nil || *current.BaseURL != "https://proxy.example.com" {
		t.Fatalf("base_url wrong: %+v", current)
	}
	if current.BearerToken == nil || *current.BearerToken != "bearer-123" {
		t.Fatalf("bearer_token wrong: %+v", current)
	}
	if current.Model == nil || *current.Model != "env-model" {
		t.Fatalf("model should prefer env: %+v", current)
	}
	if current.SmallModelUsesMainModel {
		t.Fatalf("small model differs from main: %+v", current)
	}
	if current.ReasoningEffort == nil || *current.ReasoningEffort != ClaudeReasoningEffortMedium {
		t.Fatalf("effort should prefer env: %+v", current)
	}
	if current.ThinkingMode != ClaudeThinkingModeOn {
		t.Fatalf("thinking mode should be on: %+v", current)
	}

	// Fallback document: only top-level keys, no env.
	claudeTestWrite(t, settingsPath, `{
		"model": "top-level-model",
		"effortLevel": "high",
		"alwaysThinkingEnabled": false
	}`)
	fallback, err := a.ReadClaudeCurrentConfig()
	if err != nil {
		t.Fatalf("ReadClaudeCurrentConfig (fallback): %v", err)
	}
	if fallback.Model == nil || *fallback.Model != "top-level-model" {
		t.Fatalf("fallback model wrong: %+v", fallback)
	}
	if fallback.ReasoningEffort == nil || *fallback.ReasoningEffort != ClaudeReasoningEffortHigh {
		t.Fatalf("fallback effort wrong: %+v", fallback)
	}
	if fallback.ThinkingMode != ClaudeThinkingModeOff {
		t.Fatalf("fallback thinking mode wrong: %+v", fallback)
	}
	if fallback.SmallModelUsesMainModel || fallback.SmallModel != nil {
		t.Fatalf("fallback small model wrong: %+v", fallback)
	}
}

func TestClaudeReadCurrentConfigInfersMirroring(t *testing.T) {
	home := claudeTestHome(t)
	a := &App{}

	settingsPath := filepath.Join(home, ".claude", "settings.json")
	claudeTestWrite(t, settingsPath, `{"env":{"ANTHROPIC_MODEL":"claude-sonnet-4-5","ANTHROPIC_DEFAULT_HAIKU_MODEL":"claude-sonnet-4-5"}}`)

	current, err := a.ReadClaudeCurrentConfig()
	if err != nil {
		t.Fatalf("ReadClaudeCurrentConfig: %v", err)
	}
	if !current.SmallModelUsesMainModel {
		t.Fatalf("equal small/main model should infer mirroring: %+v", current)
	}

	claudeTestWrite(t, settingsPath, `{"env":{"ANTHROPIC_MODEL":"claude-sonnet-4-5","ANTHROPIC_DEFAULT_HAIKU_MODEL":"claude-haiku-4"}}`)
	current, err = a.ReadClaudeCurrentConfig()
	if err != nil {
		t.Fatalf("ReadClaudeCurrentConfig (2): %v", err)
	}
	if current.SmallModelUsesMainModel {
		t.Fatalf("different models must not infer mirroring: %+v", current)
	}
}

func TestClaudeReadCurrentConfigRejectsNonStringEnvValues(t *testing.T) {
	home := claudeTestHome(t)
	a := &App{}

	settingsPath := filepath.Join(home, ".claude", "settings.json")
	claudeTestWrite(t, settingsPath, `{"env":{"ANTHROPIC_MODEL":42}}`)

	_, err := a.ReadClaudeCurrentConfig()
	if err == nil || !strings.Contains(err.Error(), "settings.json ANTHROPIC_MODEL must be a string") {
		t.Fatalf("expected type error, got %v", err)
	}
}
