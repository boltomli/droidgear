package app

import (
	"path/filepath"
	"strings"
	"testing"

	"droidgear-wails/internal/core"
	"droidgear-wails/internal/testutil"
)

// ---------------------------------------------------------------------------
// render_env_file byte-parity with droidgear-core::copilot
// ---------------------------------------------------------------------------

// rustCopilotEnvTemplate is the exact format! literal from
// src-tauri/crates/droidgear-core/src/copilot.rs (extracted byte-for-byte);
// the six `{}` slots are filled in order by copilotRenderEnvFile.
const rustCopilotEnvTemplate = "# ============================================================\n#  DroidGear-managed config for the GitHub Copilot BYOK profile\n{profile_line}\n# ============================================================\n\nCOPILOT_OFFLINE=true\n\n# ---- 模型提供商 API 地址 ----\n{}\n\n# ---- 提供商类型（openai / anthropic）----\n{}\n\n# ---- API Key ----\n{}\n\n# ---- 默认模型名称 ----\n{}\n\n# ---- 最大 Prompt Token 数 ----\n{}\n\n# ---- 最大输出 Token 数 ----\n{}\n"

func TestCopilotRenderEnvFileMatchesRustTemplate(t *testing.T) {
	profile := CopilotProfile{
		ID:              "id",
		Name:            "name",
		BaseURL:         core.StrPtr("https://example.test/v1"),
		ProviderType:    core.StrPtr("anthropic"),
		APIKey:          core.StrPtr("sk-test"),
		Model:           core.StrPtr("model"),
		MaxPromptTokens: uint64Ptr(100),
		MaxOutputTokens: uint64Ptr(50),
	}

	// Substitute the Rust format slots the same way format! does.
	filled := strings.Replace(rustCopilotEnvTemplate, "{profile_line}", "#  Profile: name (id)", 1)
	slots := []string{
		"COPILOT_PROVIDER_BASE_URL=https://example.test/v1",
		"COPILOT_PROVIDER_TYPE=anthropic",
		"COPILOT_PROVIDER_API_KEY=sk-test",
		"COPILOT_MODEL=model",
		"COPILOT_PROVIDER_MAX_PROMPT_TOKENS=100",
		"COPILOT_PROVIDER_MAX_OUTPUT_TOKENS=50",
	}
	parts := strings.Split(filled, "{}")
	if len(parts) != len(slots)+1 {
		t.Fatalf("template must have 6 slots, got %d", len(parts)-1)
	}
	var expected strings.Builder
	for i, slot := range slots {
		expected.WriteString(parts[i])
		expected.WriteString(slot)
	}
	expected.WriteString(parts[len(parts)-1])

	got := copilotRenderEnvFile(profile)
	if got != expected.String() {
		t.Fatalf("render_env_file diverges from Rust template\n--- got ---\n%s\n--- want ---\n%s", got, expected.String())
	}

	// Official auth mode: the two-line official document.
	official := profile
	official.UseOfficialAuth = true
	wantOfficial := "# Official GitHub Copilot subscription mode.\nCOPILOT_OFFLINE=false\n"
	if got := copilotRenderEnvFile(official); got != wantOfficial {
		t.Fatalf("official render wrong: %q", got)
	}

	// Missing values are commented out so users can see every knob.
	profile.BaseURL = nil
	profile.MaxPromptTokens = nil
	profile.MaxOutputTokens = nil
	got = copilotRenderEnvFile(profile)
	for _, want := range []string{"# COPILOT_PROVIDER_BASE_URL=", "COPILOT_MODEL=model", "# COPILOT_PROVIDER_MAX_PROMPT_TOKENS=", "# COPILOT_PROVIDER_MAX_OUTPUT_TOKENS="} {
		if !strings.Contains(got, want) {
			t.Fatalf("render missing %q:\n%s", want, got)
		}
	}
}

// ---------------------------------------------------------------------------
// Endpoint normalisation (Rust: save_and_apply_normalize_endpoints_by_protocol)
// ---------------------------------------------------------------------------

// ---------------------------------------------------------------------------
// Channel import (Rust: channel_import_maps_protocol_and_preserves_identity)
// ---------------------------------------------------------------------------

// ---------------------------------------------------------------------------
// Validation before writing (Rust: invalid_provider_and_url_are_rejected)
// ---------------------------------------------------------------------------

// ---------------------------------------------------------------------------
// CRUD + apply round trip (Rust: profile_crud_and_apply_round_trip)
// ---------------------------------------------------------------------------

func TestCopilotProfileCRUDAndApplyRoundTrip(t *testing.T) {
	testutil.IsolatedHome(t)
	a := &App{}

	if err := a.SaveCopilotProfile(copilotFixtureProfile("one", "One")); err != nil {
		t.Fatalf("SaveCopilotProfile: %v", err)
	}
	if err := a.ApplyCopilotProfile("one"); err != nil {
		t.Fatalf("ApplyCopilotProfile: %v", err)
	}
	current, err := a.ReadCopilotCurrentConfig()
	if err != nil {
		t.Fatalf("ReadCopilotCurrentConfig: %v", err)
	}
	if !current.IsByok || current.Model == nil || *current.Model != "model" {
		t.Fatalf("live config wrong: %+v", current)
	}
	active, err := a.GetActiveCopilotProfileId()
	if err != nil || active == nil || *active != "one" {
		t.Fatalf("active id wrong: %v / %v", active, err)
	}

	// A saved profile has a sibling .env file rendered from its values.
	envPath, err := copilotProfileEnvPath("one")
	if err != nil {
		t.Fatalf("copilotProfileEnvPath: %v", err)
	}
	if !fileExists(envPath) {
		t.Fatalf("profile env file missing: %s", envPath)
	}

	copy, err := a.DuplicateCopilotProfile("one", "Two")
	if err != nil {
		t.Fatalf("DuplicateCopilotProfile: %v", err)
	}
	if copy.ID == "one" {
		t.Fatalf("duplicate must get a fresh id: %+v", copy)
	}
	if copy.APIKey == nil || *copy.APIKey != "sk-test" {
		t.Fatalf("duplicate lost api key: %+v", copy)
	}

	// Saving the applied profile must not touch the live config.env.
	status, err := a.GetCopilotConfigStatus()
	if err != nil {
		t.Fatalf("GetCopilotConfigStatus: %v", err)
	}
	liveBefore := testutil.MustRead(t, status.ConfigPath)
	updated := copilotFixtureProfile("one", "One")
	updated.Model = core.StrPtr("another-model")
	if err := a.SaveCopilotProfile(updated); err != nil {
		t.Fatalf("SaveCopilotProfile (update): %v", err)
	}
	if live := testutil.MustRead(t, status.ConfigPath); live != liveBefore {
		t.Fatalf("save must not change live config:\nbefore=%q\nafter=%q", liveBefore, live)
	}
	// The profile env file follows the update.
	if env := testutil.MustRead(t, envPath); !strings.Contains(env, "COPILOT_MODEL=another-model") {
		t.Fatalf("profile env not updated: %s", env)
	}

	// Deleting the active profile clears the marker.
	if err := a.DeleteCopilotProfile("one"); err != nil {
		t.Fatalf("DeleteCopilotProfile: %v", err)
	}
	active, err = a.GetActiveCopilotProfileId()
	if err != nil || active != nil {
		t.Fatalf("delete must clear active id: %v / %v", active, err)
	}
	profiles, err := a.ListCopilotProfiles()
	if err != nil || len(profiles) != 1 {
		t.Fatalf("expected 1 profile left: %v (n=%d)", err, len(profiles))
	}
	if fileExists(envPath) {
		t.Fatalf("profile env file must be removed with the profile")
	}
}

// TestCopilotCreateDefaultOnlyWhenEmpty pins the store's loadProfiles
// fallback and the Chinese default name from the Rust core.
func TestCopilotCreateDefaultOnlyWhenEmpty(t *testing.T) {
	testutil.IsolatedHome(t)
	a := &App{}

	created, err := a.CreateDefaultCopilotProfile()
	if err != nil {
		t.Fatalf("CreateDefaultCopilotProfile: %v", err)
	}
	if created.Name != "默认" || created.ProviderType == nil || *created.ProviderType != "openai" {
		t.Fatalf("default profile wrong: %+v", created)
	}
	// Unlike OpenCode, creating the default does NOT set the active marker.
	active, err := a.GetActiveCopilotProfileId()
	if err != nil || active != nil {
		t.Fatalf("create default must not set active id: %v / %v", active, err)
	}
	if _, err := a.CreateDefaultCopilotProfile(); err == nil || err.Error() != "Profiles already exist" {
		t.Fatalf("second create must fail with the known error, got %v", err)
	}
}

// TestCopilotListSortsAndSkipsMismatches: sorted by (name.lower, id), and a
// file whose stem disagrees with its embedded id is skipped.
func TestCopilotListSortsAndSkipsMismatches(t *testing.T) {
	testutil.IsolatedHome(t)
	a := &App{}

	for _, p := range []CopilotProfile{
		copilotFixtureProfile("b-id", "beta"),
		copilotFixtureProfile("a-id", "Alpha"),
	} {
		if err := a.SaveCopilotProfile(p); err != nil {
			t.Fatalf("SaveCopilotProfile: %v", err)
		}
	}
	// Hand-write a mismatched file: stem "wrong" vs id "right".
	dir, err := copilotProfilesDir()
	if err != nil {
		t.Fatalf("copilotProfilesDir: %v", err)
	}
	writeTestFile(t, filepath.Join(dir, "wrong.json"),
		`{"id":"right","name":"ghost","createdAt":"2026-01-01T00:00:00Z","updatedAt":"2026-01-01T00:00:00Z"}`)

	profiles, err := a.ListCopilotProfiles()
	if err != nil {
		t.Fatalf("ListCopilotProfiles: %v", err)
	}
	if len(profiles) != 2 {
		t.Fatalf("mismatched file must be skipped, got %d profiles", len(profiles))
	}
	if profiles[0].Name != "Alpha" || profiles[1].Name != "beta" {
		t.Fatalf("wrong sort order: %s, %s", profiles[0].Name, profiles[1].Name)
	}
}

// ---------------------------------------------------------------------------
// Official auth export (Rust: official_config_removes_stored_byok_credentials)
// ---------------------------------------------------------------------------

func TestCopilotOfficialConfigRemovesStoredCredentials(t *testing.T) {
	testutil.IsolatedHome(t)
	a := &App{}

	profile := copilotFixtureProfile("one", "One")
	if err := a.SaveCopilotProfile(profile); err != nil {
		t.Fatalf("SaveCopilotProfile: %v", err)
	}
	if err := a.ApplyCopilotProfile("one"); err != nil {
		t.Fatalf("ApplyCopilotProfile: %v", err)
	}

	profile.UseOfficialAuth = true
	if err := a.SaveCopilotProfile(profile); err != nil {
		t.Fatalf("SaveCopilotProfile (official): %v", err)
	}
	if err := a.ApplyCopilotProfile("one"); err != nil {
		t.Fatalf("ApplyCopilotProfile (official): %v", err)
	}

	current, err := a.ReadCopilotCurrentConfig()
	if err != nil {
		t.Fatalf("ReadCopilotCurrentConfig: %v", err)
	}
	if current.IsByok || current.APIKey != nil || current.Model != nil {
		t.Fatalf("official config must drop BYOK credentials: %+v", current)
	}
	status, _ := a.GetCopilotConfigStatus()
	contents := testutil.MustRead(t, status.ConfigPath)
	if strings.Contains(contents, "sk-test") {
		t.Fatalf("live config must not contain stored credentials: %s", contents)
	}
}

// ---------------------------------------------------------------------------
// Env parsing (literal values, positive ints, byok detection)
// ---------------------------------------------------------------------------

func TestCopilotReadEnvConfig(t *testing.T) {
	// Official document → not byok.
	official, err := copilotReadEnvConfig("# Official GitHub Copilot subscription mode.\nCOPILOT_OFFLINE=false\n")
	if err != nil {
		t.Fatalf("copilotReadEnvConfig: %v", err)
	}
	if official.IsByok {
		t.Fatalf("official config must not be byok: %+v", official)
	}

	// BYOK document → byok with parsed token limits.
	byok, err := copilotReadEnvConfig("COPILOT_OFFLINE=true\nCOPILOT_PROVIDER_BASE_URL=https://example.test/v1\nCOPILOT_MODEL=model\nCOPILOT_PROVIDER_MAX_PROMPT_TOKENS=1000\n# commented=\n")
	if err != nil {
		t.Fatalf("copilotReadEnvConfig (byok): %v", err)
	}
	if !byok.IsByok || byok.BaseURL == nil || *byok.BaseURL != "https://example.test/v1" ||
		byok.Model == nil || *byok.Model != "model" ||
		byok.MaxPromptTokens == nil || *byok.MaxPromptTokens != 1000 || byok.MaxOutputTokens != nil {
		t.Fatalf("byok config wrong: %+v", byok)
	}

	// No offline marker but credentials present → still byok.
	implicit, err := copilotReadEnvConfig("COPILOT_MODEL=x\n")
	if err != nil || !implicit.IsByok {
		t.Fatalf("implicit byok detection failed: %+v / %v", implicit, err)
	}

	// Non-positive / malformed token limits error with the key name.
	_, err = copilotReadEnvConfig("COPILOT_PROVIDER_MAX_OUTPUT_TOKENS=0\n")
	if err == nil || err.Error() != "COPILOT_PROVIDER_MAX_OUTPUT_TOKENS must be a positive integer" {
		t.Fatalf("zero token limit must fail, got %v", err)
	}
	_, err = copilotReadEnvConfig("COPILOT_PROVIDER_MAX_PROMPT_TOKENS=abc\n")
	if err == nil || !strings.Contains(err.Error(), "COPILOT_PROVIDER_MAX_PROMPT_TOKENS") {
		t.Fatalf("malformed token limit must fail, got %v", err)
	}
}

// TestCopilotReadCurrentConfigMissingFile: a missing live config.env reports
// "everything unset" instead of failing (store treats it as optional).
func TestCopilotReadCurrentConfigMissingFile(t *testing.T) {
	testutil.IsolatedHome(t)
	a := &App{}

	current, err := a.ReadCopilotCurrentConfig()
	if err != nil {
		t.Fatalf("ReadCopilotCurrentConfig: %v", err)
	}
	if current.IsByok || current.BaseURL != nil || current.Model != nil {
		t.Fatalf("missing file must be all-unset: %+v", current)
	}
	status, err := a.GetCopilotConfigStatus()
	if err != nil || status.ConfigExists {
		t.Fatalf("status must report missing: %+v / %v", status, err)
	}
}

// ---------------------------------------------------------------------------
// Run plan (copilot_runtime.rs)
// ---------------------------------------------------------------------------

func TestCopilotBuildRunPlanSeparatesSecretsAndClearsInheritedKeys(t *testing.T) {
	profile := copilotFixtureProfile("id", "name")
	profile.Model = core.StrPtr("  provider/custom-model  ")
	plan, err := copilotBuildRunPlan(profile)
	if err != nil {
		t.Fatalf("copilotBuildRunPlan: %v", err)
	}

	if plan.program != "copilot" {
		t.Fatalf("program wrong: %q", plan.program)
	}
	// --model overrides the interactive UI's persisted selection.
	if len(plan.args) != 2 || plan.args[0] != "--model" || plan.args[1] != "provider/custom-model" {
		t.Fatalf("args wrong: %#v", plan.args)
	}
	// The API key travels in secret_env only.
	if len(plan.secretEnv) != 1 || plan.secretEnv[0][0] != copilotEnvProviderAPIKey || plan.secretEnv[0][1] != "sk-test" {
		t.Fatalf("secret_env wrong: %#v", plan.secretEnv)
	}
	for _, pair := range plan.env {
		if pair[0] == copilotEnvProviderAPIKey {
			t.Fatalf("api key must not appear in public env: %#v", plan.env)
		}
	}
	// The model is trimmed in both args and env.
	modelSet := false
	for _, pair := range plan.env {
		if pair[0] == copilotEnvModel {
			modelSet = pair[1] == "provider/custom-model"
		}
	}
	if !modelSet {
		t.Fatalf("COPILOT_MODEL not trimmed: %#v", plan.env)
	}
	// The plan must not mutate the input profile (Rust normalises a clone).
	if profile.BaseURL == nil || *profile.BaseURL != "https://example.test/v1" {
		t.Fatalf("input profile mutated: %+v", profile)
	}
	// Inherited Copilot keys are cleared, including COPILOT_AUTH_TOKEN.
	for _, key := range append([]string{copilotEnvAuthToken}, copilotInheritedEnvKeys...) {
		found := false
		for _, unset := range plan.unsetEnv {
			if unset == key {
				found = true
			}
		}
		if !found {
			t.Fatalf("unset_env missing %s: %#v", key, plan.unsetEnv)
		}
	}
}

func TestCopilotBuildRunPlanOfficialMode(t *testing.T) {
	profile := copilotFixtureProfile("id", "name")
	profile.UseOfficialAuth = true
	profile.BaseURL = nil
	profile.Model = nil
	profile.APIKey = nil
	plan, err := copilotBuildRunPlan(profile)
	if err != nil {
		t.Fatalf("official profile must launch: %v", err)
	}
	if len(plan.env) != 0 || len(plan.secretEnv) != 0 || len(plan.args) != 0 {
		t.Fatalf("official mode must not inject byok values: %#v", plan)
	}
	for _, unset := range plan.unsetEnv {
		if unset == copilotEnvAuthToken {
			t.Fatalf("official mode must keep COPILOT_AUTH_TOKEN: %#v", plan.unsetEnv)
		}
	}
}

func TestCopilotBuildRunPlanRejectsIncompleteProfile(t *testing.T) {
	profile := copilotFixtureProfile("id", "name")
	profile.Model = core.StrPtr("   ")
	_, err := copilotBuildRunPlan(profile)
	if err == nil || !strings.Contains(err.Error(), "Model") {
		t.Fatalf("incomplete profile must be rejected naming Model, got %v", err)
	}
	profile.Model = nil
	profile.BaseURL = nil
	profile.APIKey = nil
	_, err = copilotBuildRunPlan(profile)
	if err == nil || err.Error() != "Complete the Copilot BYOK profile before launching: Base URL, Model, API key" {
		t.Fatalf("missing-field list wrong: %v", err)
	}

	// apply_copilot_profile runs the same validation before writing.
	testutil.IsolatedHome(t)
	a := &App{}
	if err := a.SaveCopilotProfile(profile); err != nil {
		t.Fatalf("SaveCopilotProfile: %v", err)
	}
	if err := a.ApplyCopilotProfile(profile.ID); err == nil || !strings.Contains(err.Error(), "Base URL") {
		t.Fatalf("apply must validate the run plan, got %v", err)
	}
	status, _ := a.GetCopilotConfigStatus()
	if status.ConfigExists {
		t.Fatalf("failed apply must not write config.env")
	}
}

// ---------------------------------------------------------------------------
// helpers
// ---------------------------------------------------------------------------

func copilotFixtureProfile(id string, name string) CopilotProfile {
	return CopilotProfile{
		ID:           id,
		Name:         name,
		CreatedAt:    "2026-01-01T00:00:00Z",
		UpdatedAt:    "2026-01-01T00:00:00Z",
		BaseURL:      core.StrPtr("https://example.test/v1"),
		ProviderType: core.StrPtr("openai"),
		APIKey:       core.StrPtr("sk-test"),
		Model:        core.StrPtr("model"),
		// MaxPrompt/MaxOutput stay nil unless a test sets them.
	}
}

func uint64Ptr(value uint64) *uint64 {
	return &value
}
