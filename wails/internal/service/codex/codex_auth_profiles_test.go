package codex

import (
	"os"
	"path/filepath"
	"strings"
	"testing"

	"droidgear-wails/internal/core"
	"droidgear-wails/internal/testutil"
)

// codexTestWriteLiveAuth drops an auth.json into the isolated ~/.codex.
func codexTestWriteLiveAuth(t *testing.T, content string) string {
	t.Helper()
	path, err := codexAuthPath()
	if err != nil {
		t.Fatalf("codexAuthPath: %v", err)
	}
	if err := os.WriteFile(path, []byte(content), 0o644); err != nil {
		t.Fatalf("write auth: %v", err)
	}
	return path
}

const codexTestOfficialAuth = `{
  "auth_mode": "chatgpt",
  "OPENAI_API_KEY": "sk-test-key",
  "tokens": {
    "id_token": "test-id-token",
    "access_token": "test-access-token",
    "refresh_token": "test-refresh-token",
    "account_id": "test-account-id"
  },
  "last_refresh": "2026-06-29T00:00:00Z"
}`

// TestCodexAuthListEmptyInitially covers the first-run state: no profiles,
// no active marker, live auth missing → not official.

// TestCodexAuthOfficialDetection: auth_mode present → official; BYOK-only →
// not official; missing file → not official (no error).

// TestCodexAuthCorruptManifestIsAnError: a malformed profiles.json must fail
// instead of reading as empty — otherwise the next save/switch/delete/rename
// rewrites the manifest without the existing profiles.
func TestCodexAuthCorruptManifestIsAnError(t *testing.T) {
	testutil.IsolatedHome(t)

	manifestPath, err := codexAuthManifestPath()
	if err != nil {
		t.Fatalf("codexAuthManifestPath: %v", err)
	}
	if err := os.MkdirAll(filepath.Dir(manifestPath), 0o755); err != nil {
		t.Fatalf("mkdir manifest dir: %v", err)
	}
	if err := os.WriteFile(manifestPath, []byte("{ truncated"), 0o600); err != nil {
		t.Fatalf("write corrupt manifest: %v", err)
	}

	if _, err := ReadAuthManifest(); err == nil || !strings.Contains(err.Error(), "Failed to parse profiles manifest") {
		t.Fatalf("expected parse error, got %v", err)
	}
	if _, err := ListAuthProfiles(); err == nil {
		t.Fatal("list must fail on a corrupt manifest")
	}
	codexTestWriteLiveAuth(t, codexTestOfficialAuth)
	if err := SaveCurrentAuthProfile("acct-1", "Account 1"); err == nil {
		t.Fatal("save must fail on a corrupt manifest")
	}
	if err := SwitchAuthProfile("acct-1"); err == nil {
		t.Fatal("switch must fail on a corrupt manifest")
	}
	if err := DeleteAuthProfile("acct-1"); err == nil {
		t.Fatal("delete must fail on a corrupt manifest")
	}
	if err := RenameAuthProfile("acct-1", "Label"); err == nil {
		t.Fatal("rename must fail on a corrupt manifest")
	}
	if err := codexClearActiveAuthProfile(); err == nil {
		t.Fatal("clear-active must fail on a corrupt manifest")
	}
	// The corrupt file is left untouched for the user to fix.
	data, err := os.ReadFile(manifestPath) // #nosec G304 -- test fixture
	if err != nil {
		t.Fatalf("read manifest: %v", err)
	}
	if string(data) != "{ truncated" {
		t.Fatalf("manifest was rewritten: %q", string(data))
	}
}

// TestCodexAuthSaveRejectsBYOK: only official login credentials can be saved.
func TestCodexAuthSaveRejectsBYOK(t *testing.T) {
	testutil.IsolatedHome(t)

	codexTestWriteLiveAuth(t, `{"OPENAI_API_KEY":"sk-byok"}`)
	err := SaveCurrentAuthProfile("test", "Test")
	if err == nil {
		t.Fatal("BYOK save must fail")
	}
	if want := "Current auth is not an official login configuration"; !strings.Contains(err.Error(), want) {
		t.Fatalf("error %q must contain %q", err.Error(), want)
	}

	// Invalid names are rejected before any I/O.
	if err := SaveCurrentAuthProfile("", "x"); err == nil || err.Error() != "Profile name cannot be empty" {
		t.Fatalf("empty name: %v", err)
	}
	if err := SaveCurrentAuthProfile("bad name", "x"); err == nil ||
		err.Error() != "Profile name can only contain alphanumeric characters, hyphens, and underscores" {
		t.Fatalf("invalid name: %v", err)
	}
}

// TestCodexAuthSaveAndListOfficial covers the happy path: save → manifest
// entry with official flag + active marker + live state flags.
func TestCodexAuthSaveAndListOfficial(t *testing.T) {
	testutil.IsolatedHome(t)

	codexTestWriteLiveAuth(t, codexTestOfficialAuth)
	if err := SaveCurrentAuthProfile("acct-1", "Account 1"); err != nil {
		t.Fatalf("SaveCurrentCodexAuthProfile: %v", err)
	}

	state, err := ListAuthProfiles()
	if err != nil {
		t.Fatalf("ListCodexAuthProfiles: %v", err)
	}
	if len(state.Profiles) != 1 {
		t.Fatalf("profiles: %+v", state.Profiles)
	}
	profile := state.Profiles[0]
	if profile.Name != "acct-1" || profile.Label != "Account 1" || !profile.IsOfficial {
		t.Fatalf("profile: %+v", profile)
	}
	if profile.CreatedAt == "" {
		t.Fatalf("createdAt missing: %+v", profile)
	}
	if state.Active == nil || *state.Active != "acct-1" {
		t.Fatalf("active: %v", state.Active)
	}
	if !state.IsCurrentOfficial {
		t.Fatalf("isCurrentOfficial: %+v", state)
	}

	// The saved copy is a byte-for-byte auth.json.
	saved, err := os.ReadFile(mustCodexAuthProfileFile(t, "acct-1"))
	if err != nil {
		t.Fatalf("read saved auth: %v", err)
	}
	if !strings.Contains(string(saved), "auth_mode") {
		t.Fatalf("saved auth content: %q", saved)
	}
}

func mustCodexAuthProfileFile(t *testing.T, name string) string {
	t.Helper()
	path, err := AuthProfileFile(name)
	if err != nil {
		t.Fatalf("AuthProfileFile: %v", err)
	}
	return path
}

// TestCodexAuthSaveSnapshotsModelAndProfileID: the active CodexProfile id and
// the live config.toml model are snapshotted into the auth profile.
func TestCodexAuthSaveSnapshotsModelAndProfileID(t *testing.T) {
	testutil.IsolatedHome(t)

	codexTestWriteLiveAuth(t, codexTestOfficialAuth)
	codexTestWriteConfig(t, "model_provider = \"openai\"\nmodel = \"gpt-5.4\"\nmodel_reasoning_effort = \"high\"\n")
	profile := CodexProfile{ID: "codex-p1", Name: "P1", ModelProvider: "openai", Model: "gpt-5.4", ModelReasoningEffort: core.StrPtr("high")}
	if err := SaveProfile(profile); err != nil {
		t.Fatalf("SaveCodexProfile: %v", err)
	}
	if err := ApplyProfile("codex-p1"); err != nil {
		t.Fatalf("ApplyCodexProfile: %v", err)
	}

	if err := SaveCurrentAuthProfile("acct-1", "Account 1"); err != nil {
		t.Fatalf("SaveCurrentCodexAuthProfile: %v", err)
	}
	state, err := ListAuthProfiles()
	if err != nil || len(state.Profiles) != 1 {
		t.Fatalf("list: %v %+v", err, state)
	}
	saved := state.Profiles[0]
	if saved.CodexProfileID == nil || *saved.CodexProfileID != "codex-p1" {
		t.Fatalf("codexProfileId: %v", saved.CodexProfileID)
	}
	if saved.Model == nil || *saved.Model != "gpt-5.4" {
		t.Fatalf("model snapshot: %v", saved.Model)
	}
	if saved.ModelReasoningEffort == nil || *saved.ModelReasoningEffort != "high" {
		t.Fatalf("effort snapshot: %v", saved.ModelReasoningEffort)
	}
}

// TestCodexAuthSwitchBacksUpAndRestores covers the switch dance: saving a
// second profile backs up the mutated live auth into the first, and switching
// restores the target's auth + marks it active.
func TestCodexAuthSwitchBacksUpAndRestores(t *testing.T) {
	testutil.IsolatedHome(t)

	codexTestWriteLiveAuth(t, codexTestOfficialAuth)
	if err := SaveCurrentAuthProfile("acct-1", "Account 1"); err != nil {
		t.Fatalf("save acct-1: %v", err)
	}

	// Mutate live auth (another login) and save it as acct-2.
	mutated := `{"auth_mode":"chatgpt","OPENAI_API_KEY":"sk-account-2","tokens":{"access_token":"a2"}}`
	authPath := codexTestWriteLiveAuth(t, mutated)
	if err := SaveCurrentAuthProfile("acct-2", "Account 2"); err != nil {
		t.Fatalf("save acct-2: %v", err)
	}

	// Switch back to acct-1: live auth restored, acct-2 holds the backup.
	if err := SwitchAuthProfile("acct-1"); err != nil {
		t.Fatalf("SwitchCodexAuthProfile: %v", err)
	}
	live, err := os.ReadFile(authPath)
	if err != nil {
		t.Fatalf("read live auth: %v", err)
	}
	if !strings.Contains(string(live), "sk-test-key") || strings.Contains(string(live), "sk-account-2") {
		t.Fatalf("live auth not restored: %q", live)
	}
	backup, err := os.ReadFile(mustCodexAuthProfileFile(t, "acct-2"))
	if err != nil {
		t.Fatalf("read backup: %v", err)
	}
	if !strings.Contains(string(backup), "sk-account-2") {
		t.Fatalf("backup missing: %q", backup)
	}

	state, err := ListAuthProfiles()
	if err != nil {
		t.Fatalf("list: %v", err)
	}
	if state.Active == nil || *state.Active != "acct-1" || len(state.Profiles) != 2 {
		t.Fatalf("state: %+v", state)
	}
}

// TestCodexAuthSwitchAppliesModelSnapshot: after wiping the live model, a
// switch restores the snapshotted model + effort into config.toml.
func TestCodexAuthSwitchAppliesModelSnapshot(t *testing.T) {
	testutil.IsolatedHome(t)

	codexTestWriteLiveAuth(t, codexTestOfficialAuth)
	codexTestWriteConfig(t, "model_provider = \"openai\"\nmodel = \"gpt-5.4\"\nmodel_reasoning_effort = \"high\"\n")
	if err := SaveCurrentAuthProfile("acct-1", "Account 1"); err != nil {
		t.Fatalf("save: %v", err)
	}

	// Wipe the live model.
	codexTestWriteConfig(t, "model_provider = \"openai\"\nmodel = \"\"\n")
	if err := SwitchAuthProfile("acct-1"); err != nil {
		t.Fatalf("switch: %v", err)
	}

	config := codexTestParseConfig(t)
	if config["model"] != "gpt-5.4" || config["model_reasoning_effort"] != "high" {
		t.Fatalf("model snapshot not restored: %+v", config)
	}
}

// TestCodexAuthSwitchAppliesAssociatedProfile: a saved auth profile carrying a
// codexProfileId re-applies that profile's config.toml on switch (auth.json
// untouched by that apply).
func TestCodexAuthSwitchAppliesAssociatedProfile(t *testing.T) {
	testutil.IsolatedHome(t)

	codexTestWriteLiveAuth(t, codexTestOfficialAuth)
	profile := codexTestCustomProfile("p1", "P1")
	if err := SaveProfile(profile); err != nil {
		t.Fatalf("save profile: %v", err)
	}
	if err := ApplyProfile("p1"); err != nil {
		t.Fatalf("apply profile: %v", err)
	}
	if err := SaveCurrentAuthProfile("acct-1", "Account 1"); err != nil {
		t.Fatalf("save auth: %v", err)
	}

	// Live config drifts away from the profile.
	codexTestWriteConfig(t, "model_provider = \"ghost\"\nmodel = \"drifted\"\n")

	// auth.json after the switch is the saved official one.
	if err := SwitchAuthProfile("acct-1"); err != nil {
		t.Fatalf("switch: %v", err)
	}
	config := codexTestParseConfig(t)
	if config["model_provider"] != "custom" || config["model"] != "gpt-5.5" {
		t.Fatalf("associated profile not applied: %+v", config)
	}
	live, err := os.ReadFile(mustCodexAuthProfileFile(t, "acct-1"))
	if err != nil {
		t.Fatalf("read saved auth: %v", err)
	}
	if !strings.Contains(string(live), "auth_mode") {
		t.Fatalf("auth snapshot: %q", live)
	}
}

// TestCodexAuthDeleteAndRename covers label updates and the active-profile
// protection.
func TestCodexAuthDeleteAndRename(t *testing.T) {
	testutil.IsolatedHome(t)

	codexTestWriteLiveAuth(t, codexTestOfficialAuth)
	if err := SaveCurrentAuthProfile("acct-1", "Old Label"); err != nil {
		t.Fatalf("save acct-1: %v", err)
	}
	// Second profile (current live auth) so acct-1 is no longer the only one.
	if err := SaveCurrentAuthProfile("acct-2", "Second"); err != nil {
		t.Fatalf("save acct-2: %v", err)
	}

	// The first saved profile owns the active marker (save only fills it when
	// unset) — deleting it must fail; the inactive one deletes fine.
	if err := DeleteAuthProfile("acct-1"); err == nil || err.Error() != "Cannot delete the currently active profile" {
		t.Fatalf("delete active: %v", err)
	}

	if err := RenameAuthProfile("acct-2", "New Label"); err != nil {
		t.Fatalf("rename: %v", err)
	}
	state, err := ListAuthProfiles()
	if err != nil {
		t.Fatalf("list: %v", err)
	}
	for _, profile := range state.Profiles {
		if profile.Name == "acct-2" && profile.Label != "New Label" {
			t.Fatalf("label not updated: %+v", profile)
		}
	}

	if err := DeleteAuthProfile("acct-2"); err != nil {
		t.Fatalf("delete: %v", err)
	}
	state, err = ListAuthProfiles()
	if err != nil || len(state.Profiles) != 1 || state.Profiles[0].Name != "acct-1" {
		t.Fatalf("after delete: %v %+v", err, state)
	}
	// Profile directory removed too.
	if _, err := os.Stat(filepath.Join(mustCodexAuthProfilesDir(t), "acct-2")); !os.IsNotExist(err) {
		t.Fatalf("profile dir must be removed, err=%v", err)
	}

	// Unknown profile errors match Rust.
	if err := DeleteAuthProfile("ghost"); err == nil || err.Error() != "Profile 'ghost' not found" {
		t.Fatalf("delete ghost: %v", err)
	}
	if err := RenameAuthProfile("ghost", "x"); err == nil || err.Error() != "Profile 'ghost' not found" {
		t.Fatalf("rename ghost: %v", err)
	}
}

func mustCodexAuthProfilesDir(t *testing.T) string {
	t.Helper()
	dir, err := codexAuthProfilesDir()
	if err != nil {
		t.Fatalf("codexAuthProfilesDir: %v", err)
	}
	return dir
}

// TestCodexAuthConflictDetection: official→byok (and back) is a conflict;
// same-mode switches are not.
func TestCodexAuthConflictDetection(t *testing.T) {
	testutil.IsolatedHome(t)

	// Live auth is BYOK; the saved profile is official.
	codexTestWriteLiveAuth(t, `{"OPENAI_API_KEY":"sk-byok"}`)
	profileDir := filepath.Join(mustCodexAuthProfilesDir(t), "official-prof")
	if err := os.MkdirAll(profileDir, 0o755); err != nil {
		t.Fatalf("mkdir: %v", err)
	}
	if err := os.WriteFile(filepath.Join(profileDir, "auth.json"), []byte(codexTestOfficialAuth), 0o644); err != nil {
		t.Fatalf("write profile auth: %v", err)
	}
	manifest := codexAuthProfilesManifest{
		Profiles: []CodexAuthProfile{{Name: "official-prof", Label: "Official", CreatedAt: core.NowRFC3339(), IsOfficial: true}},
	}
	if err := WriteAuthManifest(manifest); err != nil {
		t.Fatalf("write manifest: %v", err)
	}

	conflict, err := DetectAuthConflict("official-prof")
	if err != nil {
		t.Fatalf("DetectCodexAuthConflict: %v", err)
	}
	if !conflict.HasConflict || conflict.CurrentIsOfficial || !conflict.TargetIsOfficial {
		t.Fatalf("conflict: %+v", conflict)
	}

	// Same mode → no conflict.
	codexTestWriteLiveAuth(t, codexTestOfficialAuth)
	if err := SaveCurrentAuthProfile("acct-1", "Account 1"); err != nil {
		t.Fatalf("save: %v", err)
	}
	conflict, err = DetectAuthConflict("acct-1")
	if err != nil || conflict.HasConflict {
		t.Fatalf("same mode: %v %+v", err, conflict)
	}
}

// TestCodexApplyAuthConflictDetection covers the apply-direction check:
// official target with BYOK live auth conflicts; custom targets never do.
func TestCodexApplyAuthConflictDetection(t *testing.T) {
	testutil.IsolatedHome(t)

	codexTestWriteLiveAuth(t, `{"OPENAI_API_KEY":"sk-byok"}`)
	official := CodexProfile{ID: "official", Name: "Official", ModelProvider: "openai", Model: "gpt-5"}
	if err := SaveProfile(official); err != nil {
		t.Fatalf("save official: %v", err)
	}
	custom := codexTestCustomProfile("custom", "Custom")
	if err := SaveProfile(custom); err != nil {
		t.Fatalf("save custom: %v", err)
	}

	conflict, err := DetectApplyAuthConflict("official")
	if err != nil {
		t.Fatalf("DetectCodexApplyAuthConflict(official): %v", err)
	}
	if !conflict.HasConflict || conflict.CurrentIsOfficial || !conflict.TargetIsOfficial {
		t.Fatalf("official conflict: %+v", conflict)
	}

	conflict, err = DetectApplyAuthConflict("custom")
	if err != nil {
		t.Fatalf("DetectCodexApplyAuthConflict(custom): %v", err)
	}
	if conflict.HasConflict || conflict.CurrentIsOfficial || conflict.TargetIsOfficial {
		t.Fatalf("custom must not conflict: %+v", conflict)
	}

	// Live official + official target → no conflict.
	codexTestWriteLiveAuth(t, codexTestOfficialAuth)
	conflict, err = DetectApplyAuthConflict("official")
	if err != nil || conflict.HasConflict {
		t.Fatalf("official/official: %v %+v", err, conflict)
	}
}

// TestCodexApplyRestoresNamedAuthProfile: applying an openai CodexProfile
// with auth_profile_name restores that saved auth into the live location.
func TestCodexApplyRestoresNamedAuthProfile(t *testing.T) {
	testutil.IsolatedHome(t)

	// Saved official auth profile (no codexProfileId → config-only apply skipped).
	profileDir := filepath.Join(mustCodexAuthProfilesDir(t), "saved-login")
	if err := os.MkdirAll(profileDir, 0o755); err != nil {
		t.Fatalf("mkdir: %v", err)
	}
	savedAuth := `{"auth_mode":"chatgpt","tokens":{"access_token":"restored"}}`
	if err := os.WriteFile(filepath.Join(profileDir, "auth.json"), []byte(savedAuth), 0o644); err != nil {
		t.Fatalf("write profile auth: %v", err)
	}
	if err := WriteAuthManifest(codexAuthProfilesManifest{
		Profiles: []CodexAuthProfile{{Name: "saved-login", Label: "Saved", CreatedAt: core.NowRFC3339(), IsOfficial: true}},
	}); err != nil {
		t.Fatalf("write manifest: %v", err)
	}

	// Live auth currently BYOK with a residual key.
	authPath := codexTestWriteLiveAuth(t, `{"OPENAI_API_KEY":"sk-residual"}`)

	profile := CodexProfile{
		ID:              "official",
		Name:            "Official",
		ModelProvider:   "openai",
		Model:           "gpt-5",
		AuthProfileName: core.StrPtr("saved-login"),
	}
	if err := SaveProfile(profile); err != nil {
		t.Fatalf("save profile: %v", err)
	}
	if err := ApplyProfile("official"); err != nil {
		t.Fatalf("apply: %v", err)
	}

	live, err := os.ReadFile(authPath)
	if err != nil {
		t.Fatalf("read live auth: %v", err)
	}
	if !strings.Contains(string(live), "restored") || strings.Contains(string(live), "OPENAI_API_KEY") {
		t.Fatalf("auth not restored/stripped: %q", live)
	}
	state, err := ListAuthProfiles()
	if err != nil || state.Active == nil || *state.Active != "saved-login" {
		t.Fatalf("active marker: %v %+v", err, state)
	}
}

// TestCodexRestoreAuthFileRejectsUnsavedOrBYOK mirrors the guard rails of
// restore_auth_file_for_home.
func TestCodexRestoreAuthFileRejectsUnsavedOrBYOK(t *testing.T) {
	testutil.IsolatedHome(t)

	if err := codexRestoreAuthFile("ghost"); err == nil || err.Error() != "Auth profile 'ghost' not found" {
		t.Fatalf("unknown profile: %v", err)
	}

	profileDir := filepath.Join(mustCodexAuthProfilesDir(t), "byok-prof")
	if err := os.MkdirAll(profileDir, 0o755); err != nil {
		t.Fatalf("mkdir: %v", err)
	}
	if err := os.WriteFile(filepath.Join(profileDir, "auth.json"), []byte(`{"OPENAI_API_KEY":"sk"}`), 0o644); err != nil {
		t.Fatalf("write: %v", err)
	}
	if err := WriteAuthManifest(codexAuthProfilesManifest{
		Profiles: []CodexAuthProfile{{Name: "byok-prof", Label: "BYOK", CreatedAt: core.NowRFC3339(), IsOfficial: false}},
	}); err != nil {
		t.Fatalf("manifest: %v", err)
	}
	err := codexRestoreAuthFile("byok-prof")
	if err == nil || !strings.Contains(err.Error(), "is not an official login backup") {
		t.Fatalf("byok restore: %v", err)
	}

	// Declared official but the auth file is missing.
	if err := WriteAuthManifest(codexAuthProfilesManifest{
		Profiles: []CodexAuthProfile{{Name: "byok-prof", Label: "BYOK", CreatedAt: core.NowRFC3339(), IsOfficial: true}},
	}); err != nil {
		t.Fatalf("manifest: %v", err)
	}
	if err := os.Remove(filepath.Join(profileDir, "auth.json")); err != nil {
		t.Fatalf("remove: %v", err)
	}
	err = codexRestoreAuthFile("byok-prof")
	if err == nil || !strings.Contains(err.Error(), "Auth file missing for profile 'byok-prof'") {
		t.Fatalf("missing file: %v", err)
	}
}
