package droid

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

// authFixture builds an isolated home whose Factory home carries both auth
// files (mirrors the Rust test setup).
func authFixture(t *testing.T) (home string, factoryHome string) {
	t.Helper()
	home = t.TempDir()
	return home, stampFactoryAuthFiles(t, home)
}

// stampFactoryAuthFiles writes key-1/token-1 auth files into home's Factory
// home and returns its path.
func stampFactoryAuthFiles(t *testing.T, home string) string {
	t.Helper()
	factoryHome := filepath.Join(home, ".factory")
	if err := os.MkdirAll(factoryHome, 0o755); err != nil {
		t.Fatalf("mkdir factory: %v", err)
	}
	if err := os.WriteFile(filepath.Join(factoryHome, "auth.v2.key"), []byte("key-1"), 0o600); err != nil {
		t.Fatalf("write auth key: %v", err)
	}
	if err := os.WriteFile(filepath.Join(factoryHome, "auth.v2.file"), []byte("token-1"), 0o600); err != nil {
		t.Fatalf("write auth token: %v", err)
	}
	return factoryHome
}

// listProfiles reads the manifest through the typed helper so failures point
// at the test that triggered them.
func listProfiles(t *testing.T, home string) AuthProfileState {
	t.Helper()
	state, err := droidListAuthProfiles(home)
	if err != nil {
		t.Fatalf("list profiles: %v", err)
	}
	return state
}

func readAuth(t *testing.T, dir string, name string) string {
	t.Helper()
	data, err := os.ReadFile(filepath.Join(dir, name)) // #nosec G304 -- test fixture
	if err != nil {
		t.Fatalf("read %s: %v", name, err)
	}
	return string(data)
}

func writeAuth(t *testing.T, dir string, name string, contents string) {
	t.Helper()
	if err := os.WriteFile(filepath.Join(dir, name), []byte(contents), 0o600); err != nil {
		t.Fatalf("write %s: %v", name, err)
	}
}

func TestFactoryAuthProfileLifecycle(t *testing.T) {
	home, factoryHome := authFixture(t)
	profilesDir := filepath.Join(home, ".droidgear", "auth-profiles", "droid")

	// Empty manifest initially.
	state := listProfiles(t, home)
	if len(state.Profiles) != 0 || state.Active != nil {
		t.Fatalf("expected empty manifest, got %+v", state)
	}
	if err := droidSaveCurrentAuthProfile(home, "acct-1", "Account 1"); err != nil {
		t.Fatalf("save acct-1: %v", err)
	}

	state = listProfiles(t, home)
	if state.Active == nil || *state.Active != "acct-1" {
		t.Fatalf("expected acct-1 active, got %+v", state.Active)
	}
	if len(state.Profiles) != 1 || state.Profiles[0].Name != "acct-1" ||
		state.Profiles[0].Label != "Account 1" || state.Profiles[0].CreatedAt == "" {
		t.Fatalf("unexpected profiles: %+v", state.Profiles)
	}
	if readAuth(t, filepath.Join(profilesDir, "acct-1"), "auth.v2.key") != "key-1" {
		t.Fatal("auth key not copied into the profile")
	}
	if readAuth(t, filepath.Join(profilesDir, "acct-1"), "auth.v2.file") != "token-1" {
		t.Fatal("auth token not copied into the profile")
	}

	// A second profile snapshots the current Factory auth files; saving must
	// not switch the active profile.
	writeAuth(t, factoryHome, "auth.v2.key", "key-2")
	writeAuth(t, factoryHome, "auth.v2.file", "token-2")
	if err := droidSaveCurrentAuthProfile(home, "acct-2", "Account 2"); err != nil {
		t.Fatalf("save acct-2: %v", err)
	}
	state = listProfiles(t, home)
	if state.Active == nil || *state.Active != "acct-1" {
		t.Fatalf("saving must not switch the active profile: %+v", state.Active)
	}
	if len(state.Profiles) != 2 {
		t.Fatalf("expected 2 profiles: %+v", state.Profiles)
	}
	if readAuth(t, filepath.Join(profilesDir, "acct-2"), "auth.v2.key") != "key-2" {
		t.Fatal("acct-2 did not snapshot the current auth files")
	}

	// Switching to the already-active profile restores its files over the
	// Factory home (no backup of the same profile).
	if err := droidSwitchAuthProfile(home, "acct-1"); err != nil {
		t.Fatalf("switch: %v", err)
	}
	if readAuth(t, factoryHome, "auth.v2.key") != "key-1" {
		t.Fatalf("switch did not restore acct-1 key: %q", readAuth(t, factoryHome, "auth.v2.key"))
	}
	if readAuth(t, factoryHome, "auth.v2.file") != "token-1" {
		t.Fatal("switch did not restore acct-1 token")
	}
	state = listProfiles(t, home)
	if state.Active == nil || *state.Active != "acct-1" {
		t.Fatalf("active not updated: %+v", state.Active)
	}

	// Saving an existing name updates only its label.
	if err := droidSaveCurrentAuthProfile(home, "acct-1", "Renamed 1"); err != nil {
		t.Fatalf("re-save: %v", err)
	}
	state = listProfiles(t, home)
	if len(state.Profiles) != 2 || state.Profiles[0].Label != "Renamed 1" {
		t.Fatalf("label update failed: %+v", state.Profiles)
	}

	// Rename updates the label only.
	if err := droidRenameAuthProfile(home, "acct-1", "Work"); err != nil {
		t.Fatalf("rename: %v", err)
	}
	state = listProfiles(t, home)
	if state.Profiles[0].Label != "Work" || state.Profiles[0].Name != "acct-1" {
		t.Fatalf("rename failed: %+v", state.Profiles[0])
	}

	// The active profile cannot be deleted; a background one can.
	if err := droidDeleteAuthProfile(home, "acct-1"); err == nil || err.Error() != "Cannot delete the currently active profile" {
		t.Fatalf("expected active-delete rejection, got %v", err)
	}
	if err := droidDeleteAuthProfile(home, "acct-2"); err != nil {
		t.Fatalf("delete acct-2: %v", err)
	}
	if _, err := os.Stat(filepath.Join(profilesDir, "acct-2")); !os.IsNotExist(err) {
		t.Fatalf("profile directory not removed: %v", err)
	}
	state = listProfiles(t, home)
	if len(state.Profiles) != 1 || state.Profiles[0].Name != "acct-1" {
		t.Fatalf("manifest not updated: %+v", state.Profiles)
	}
}

func TestFactoryAuthProfileSwitchBacksUpCurrentFiles(t *testing.T) {
	home, factoryHome := authFixture(t)
	profilesDir := filepath.Join(home, ".droidgear", "auth-profiles", "droid")

	if err := droidSaveCurrentAuthProfile(home, "acct-1", "Account 1"); err != nil {
		t.Fatalf("save acct-1: %v", err)
	}
	// The user logs into another account; new auth files land in the Factory
	// home and are snapshotted into acct-2.
	writeAuth(t, factoryHome, "auth.v2.key", "key-2")
	writeAuth(t, factoryHome, "auth.v2.file", "token-2")
	if err := droidSaveCurrentAuthProfile(home, "acct-2", "Account 2"); err != nil {
		t.Fatalf("save acct-2: %v", err)
	}

	// Switching away from acct-2 backs its files up before overwriting the
	// Factory home with acct-1's.
	if err := droidSwitchAuthProfile(home, "acct-1"); err != nil {
		t.Fatalf("switch to acct-1: %v", err)
	}
	if readAuth(t, factoryHome, "auth.v2.key") != "key-1" {
		t.Fatalf("factory home not restored: %q", readAuth(t, factoryHome, "auth.v2.key"))
	}
	// A fresh login lands in the Factory home; switching profiles must back
	// those files up to acct-1 before overwriting them with acct-2's.
	writeAuth(t, factoryHome, "auth.v2.key", "key-3")
	writeAuth(t, factoryHome, "auth.v2.file", "token-3")
	if err := droidSwitchAuthProfile(home, "acct-2"); err != nil {
		t.Fatalf("switch to acct-2: %v", err)
	}
	if readAuth(t, factoryHome, "auth.v2.key") != "key-2" {
		t.Fatalf("factory home not switched: %q", readAuth(t, factoryHome, "auth.v2.key"))
	}
	if readAuth(t, filepath.Join(profilesDir, "acct-1"), "auth.v2.key") != "key-3" {
		t.Fatal("acct-1 was not backed up before the switch")
	}
}

// A corrupt manifest must fail instead of reading as empty: otherwise the next
// save/delete/rename rewrites profiles.json without the existing profiles.
func TestFactoryAuthProfileCorruptManifestIsAnError(t *testing.T) {
	home, _ := authFixture(t)
	manifestPath := filepath.Join(home, ".droidgear", "auth-profiles", "droid", "profiles.json")
	if err := os.MkdirAll(filepath.Dir(manifestPath), 0o755); err != nil {
		t.Fatalf("mkdir manifest dir: %v", err)
	}
	if err := os.WriteFile(manifestPath, []byte("{ truncated"), 0o600); err != nil {
		t.Fatalf("write corrupt manifest: %v", err)
	}

	if _, err := droidListAuthProfiles(home); err == nil || !strings.Contains(err.Error(), "Failed to parse profiles manifest") {
		t.Fatalf("expected parse error, got %v", err)
	}
	if err := droidSaveCurrentAuthProfile(home, "acct-1", "Account 1"); err == nil {
		t.Fatal("save must fail on a corrupt manifest")
	}
	if err := droidDeleteAuthProfile(home, "acct-1"); err == nil {
		t.Fatal("delete must fail on a corrupt manifest")
	}
	if err := droidRenameAuthProfile(home, "acct-1", "Label"); err == nil {
		t.Fatal("rename must fail on a corrupt manifest")
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

func TestFactoryAuthProfileValidationAndMissingState(t *testing.T) {
	home := t.TempDir()

	// No Factory auth files yet.
	if err := droidSaveCurrentAuthProfile(home, "acct-1", "Label"); err == nil ||
		err.Error() != "No active auth files found in Factory home directory" {
		t.Fatalf("expected missing-auth error, got %v", err)
	}
	stampFactoryAuthFiles(t, home)

	// Name validation mirrors validate_profile_name.
	if err := droidSaveCurrentAuthProfile(home, "bad name!", "Label"); err == nil ||
		err.Error() != "Profile name can only contain alphanumeric characters, hyphens, and underscores" {
		t.Fatalf("expected alphanumeric error, got %v", err)
	}
	if err := droidSaveCurrentAuthProfile(home, "", "Label"); err == nil ||
		err.Error() != "Profile name cannot be empty" {
		t.Fatalf("expected empty-name error, got %v", err)
	}

	// Unknown profile operations.
	if err := droidSwitchAuthProfile(home, "ghost"); err == nil || err.Error() != "Profile 'ghost' not found" {
		t.Fatalf("expected not-found on switch, got %v", err)
	}
	if err := droidDeleteAuthProfile(home, "ghost"); err == nil || err.Error() != "Profile 'ghost' not found" {
		t.Fatalf("expected not-found on delete, got %v", err)
	}
	if err := droidRenameAuthProfile(home, "ghost", "Label"); err == nil || err.Error() != "Profile 'ghost' not found" {
		t.Fatalf("expected not-found on rename, got %v", err)
	}

	// A manifest entry without auth files cannot be switched to.
	if err := droidSaveCurrentAuthProfile(home, "acct-1", "Label"); err != nil {
		t.Fatalf("save: %v", err)
	}
	profileDir := filepath.Join(home, ".droidgear", "auth-profiles", "droid", "acct-1")
	for _, name := range []string{"auth.v2.key", "auth.v2.file"} {
		if err := os.Remove(filepath.Join(profileDir, name)); err != nil {
			t.Fatalf("remove %s: %v", name, err)
		}
	}
	if err := droidSwitchAuthProfile(home, "acct-1"); err == nil || err.Error() != "Auth files missing for profile 'acct-1'" {
		t.Fatalf("expected missing-auth-files error, got %v", err)
	}
}
