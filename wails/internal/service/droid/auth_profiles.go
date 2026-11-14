// Factory Droid auth profile management. Mirrors
// droidgear-core::factory_auth_profiles and
// src-tauri/src/commands/factory_auth_profiles.rs.
//
// Profiles live in ~/.droidgear/auth-profiles/droid/: a profiles.json
// manifest plus one directory per profile holding the Factory auth files
// (auth.v2.key / auth.v2.file) that get copied in and out of the Factory home
// on switch.
package droid

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"

	"droidgear-wails/internal/core"
)

const (
	droidAuthKeyFile   = "auth.v2.key"
	droidAuthTokenFile = "auth.v2.file"
)

// AuthProfile mirrors factory_auth_profiles::AuthProfile (bindings.ts).
type AuthProfile struct {
	Name      string `json:"name"`
	Label     string `json:"label"`
	CreatedAt string `json:"createdAt"`
}

// AuthProfileState mirrors factory_auth_profiles::AuthProfileState. The same
// shape is the on-disk profiles.json manifest (serde's ProfilesManifest).
type AuthProfileState struct {
	Active   *string       `json:"active"`
	Profiles []AuthProfile `json:"profiles"`
}

// ---------------------------------------------------------------------------
// Paths
// ---------------------------------------------------------------------------

func droidAuthProfilesDir(home string) string {
	return filepath.Join(home, ".droidgear", "auth-profiles", "droid")
}

func droidAuthManifestPath(home string) string {
	return filepath.Join(droidAuthProfilesDir(home), "profiles.json")
}

func droidAuthProfileDir(home string, name string) string {
	return filepath.Join(droidAuthProfilesDir(home), name)
}

// ---------------------------------------------------------------------------
// Manifest I/O
// ---------------------------------------------------------------------------

// droidReadAuthManifest returns the saved profile manifest.
//
// A missing manifest means "no profiles yet". An unreadable or malformed one
// is an error: treating it as empty would let the next write (save, switch,
// delete, rename) rewrite profiles.json without the existing profiles.
func droidReadAuthManifest(home string) (AuthProfileState, error) {
	path := droidAuthManifestPath(home)
	data, err := os.ReadFile(path) // #nosec G304 -- droidgear manifest path
	if err != nil {
		if os.IsNotExist(err) {
			return AuthProfileState{Profiles: []AuthProfile{}}, nil
		}
		return AuthProfileState{Profiles: []AuthProfile{}},
			fmt.Errorf("Failed to read profiles manifest %s: %w", path, err)
	}
	var manifest AuthProfileState
	if err := json.Unmarshal(data, &manifest); err != nil {
		return AuthProfileState{Profiles: []AuthProfile{}},
			fmt.Errorf("Failed to parse profiles manifest %s: %w", path, err)
	}
	if manifest.Profiles == nil {
		manifest.Profiles = []AuthProfile{}
	}
	return manifest, nil
}

func droidWriteAuthManifest(home string, manifest AuthProfileState) error {
	path := droidAuthManifestPath(home)
	dir := filepath.Dir(path)
	if err := core.EnsureDir(dir); err != nil {
		return fmt.Errorf("Failed to create directory %s: %w", dir, err)
	}
	if manifest.Profiles == nil {
		manifest.Profiles = []AuthProfile{} // serde Vec serializes as []
	}
	data, err := core.MarshalPretty(manifest)
	if err != nil {
		return fmt.Errorf("Failed to serialize profiles manifest: %w", err)
	}
	return droidAtomicWrite(path, data)
}

// ---------------------------------------------------------------------------
// Validation & auth file operations
// ---------------------------------------------------------------------------

// droidValidateProfileName mirrors validate_profile_name: non-empty ASCII
// alphanumerics plus '-' and '_'.
func droidValidateProfileName(name string) error {
	if name == "" {
		return errString("Profile name cannot be empty")
	}
	for _, char := range name {
		if (char >= 'a' && char <= 'z') || (char >= 'A' && char <= 'Z') ||
			(char >= '0' && char <= '9') || char == '-' || char == '_' {
			continue
		}
		return errString("Profile name can only contain alphanumeric characters, hyphens, and underscores")
	}
	return nil
}

// droidAuthCopyFiles copies the Factory auth files between the Factory home
// and a profile directory (missing sources are skipped, like fs::copy guards).
func droidAuthCopyFiles(srcDir string, dstDir string) error {
	if err := core.EnsureDir(dstDir); err != nil {
		return fmt.Errorf("Failed to create directory %s: %w", dstDir, err)
	}
	for _, filename := range []string{droidAuthKeyFile, droidAuthTokenFile} {
		src := filepath.Join(srcDir, filename)
		info, err := os.Stat(src)
		if err != nil {
			continue
		}
		data, err := os.ReadFile(src) // #nosec G304 -- factory/profile auth files
		if err != nil {
			return fmt.Errorf("Failed to copy %s from %s to %s: %w", filename, srcDir, dstDir, err)
		}
		dst := filepath.Join(dstDir, filename)
		if !core.RecordWrite(dst, data) {
			if err := os.WriteFile(dst, data, info.Mode().Perm()); err != nil { // #nosec G306 -- auth files, mode copied from source
				return fmt.Errorf("Failed to copy %s from %s to %s: %w", filename, srcDir, dstDir, err)
			}
			// WriteFile only applies the mode on create; enforce the source mode
			// so overwrites behave like fs::copy.
			_ = os.Chmod(dst, info.Mode().Perm())
		}
	}
	return nil
}

func droidAuthFilesExist(dir string) bool {
	return core.PathExists(filepath.Join(dir, droidAuthKeyFile)) &&
		core.PathExists(filepath.Join(dir, droidAuthTokenFile))
}

func droidAuthProfileExists(manifest AuthProfileState, name string) bool {
	for _, profile := range manifest.Profiles {
		if profile.Name == name {
			return true
		}
	}
	return false
}

// ---------------------------------------------------------------------------
// Commands
// ---------------------------------------------------------------------------

func droidListAuthProfiles(home string) (AuthProfileState, error) {
	return droidReadAuthManifest(home)
}

func droidSaveCurrentAuthProfile(home string, name string, label string) error {
	if err := droidValidateProfileName(name); err != nil {
		return err
	}
	factoryHome := core.FactoryHomeForHome(home)
	if !droidAuthFilesExist(factoryHome) {
		return errString("No active auth files found in Factory home directory")
	}
	profileDir := droidAuthProfileDir(home, name)
	if err := droidAuthCopyFiles(factoryHome, profileDir); err != nil {
		return err
	}

	manifest, err := droidReadAuthManifest(home)
	if err != nil {
		return err
	}
	updated := false
	for i := range manifest.Profiles {
		if manifest.Profiles[i].Name == name {
			manifest.Profiles[i].Label = label
			updated = true
			break
		}
	}
	if !updated {
		manifest.Profiles = append(manifest.Profiles, AuthProfile{
			Name:      name,
			Label:     label,
			CreatedAt: core.NowRFC3339(),
		})
	}
	if manifest.Active == nil {
		active := name
		manifest.Active = &active
	}
	return droidWriteAuthManifest(home, manifest)
}

func droidSwitchAuthProfile(home string, name string) error {
	if err := droidValidateProfileName(name); err != nil {
		return err
	}
	manifest, err := droidReadAuthManifest(home)
	if err != nil {
		return err
	}
	if !droidAuthProfileExists(manifest, name) {
		return fmt.Errorf("Profile '%s' not found", name)
	}
	targetDir := droidAuthProfileDir(home, name)
	if !droidAuthFilesExist(targetDir) {
		return fmt.Errorf("Auth files missing for profile '%s'", name)
	}
	factoryHome := core.FactoryHomeForHome(home)

	// Back up the current auth files to the active profile before switching.
	if manifest.Active != nil && *manifest.Active != name && droidAuthFilesExist(factoryHome) {
		currentDir := droidAuthProfileDir(home, *manifest.Active)
		if err := droidAuthCopyFiles(factoryHome, currentDir); err != nil {
			return err
		}
	}
	if err := droidAuthCopyFiles(targetDir, factoryHome); err != nil {
		return err
	}
	active := name
	manifest.Active = &active
	return droidWriteAuthManifest(home, manifest)
}

func droidDeleteAuthProfile(home string, name string) error {
	if err := droidValidateProfileName(name); err != nil {
		return err
	}
	manifest, err := droidReadAuthManifest(home)
	if err != nil {
		return err
	}
	if !droidAuthProfileExists(manifest, name) {
		return fmt.Errorf("Profile '%s' not found", name)
	}
	if manifest.Active != nil && *manifest.Active == name {
		return errString("Cannot delete the currently active profile")
	}
	profileDir := droidAuthProfileDir(home, name)
	if core.PathExists(profileDir) && !core.RecordRemove(profileDir) {
		if err := os.RemoveAll(profileDir); err != nil {
			return fmt.Errorf("Failed to delete profile directory: %w", err)
		}
	}
	remaining := make([]AuthProfile, 0, len(manifest.Profiles))
	for _, profile := range manifest.Profiles {
		if profile.Name != name {
			remaining = append(remaining, profile)
		}
	}
	manifest.Profiles = remaining
	return droidWriteAuthManifest(home, manifest)
}

func droidRenameAuthProfile(home string, name string, newLabel string) error {
	if err := droidValidateProfileName(name); err != nil {
		return err
	}
	manifest, err := droidReadAuthManifest(home)
	if err != nil {
		return err
	}
	for i := range manifest.Profiles {
		if manifest.Profiles[i].Name == name {
			manifest.Profiles[i].Label = newLabel
			return droidWriteAuthManifest(home, manifest)
		}
	}
	return fmt.Errorf("Profile '%s' not found", name)
}

// ---------------------------------------------------------------------------
// Wails service methods
// ---------------------------------------------------------------------------

// ListAuthProfiles returns the auth profile manifest (active name +
// profiles).
func ListAuthProfiles() (AuthProfileState, error) {
	home, err := droidHomeDir()
	if err != nil {
		return AuthProfileState{Profiles: []AuthProfile{}}, err
	}
	return droidListAuthProfiles(home)
}

// SwitchAuthProfile copies the target profile's auth files into the
// Factory home (backing up the current ones first).
func SwitchAuthProfile(name string) error {
	home, err := droidHomeDir()
	if err != nil {
		return err
	}
	return droidSwitchAuthProfile(home, name)
}

// SaveCurrentAuthProfile snapshots the Factory home's auth files into
// a (new or existing) profile.
func SaveCurrentAuthProfile(name string, label string) error {
	home, err := droidHomeDir()
	if err != nil {
		return err
	}
	return droidSaveCurrentAuthProfile(home, name, label)
}

// DeleteAuthProfile removes a non-active profile and its files.
func DeleteAuthProfile(name string) error {
	home, err := droidHomeDir()
	if err != nil {
		return err
	}
	return droidDeleteAuthProfile(home, name)
}

// RenameAuthProfile updates a profile's display label.
func RenameAuthProfile(name string, label string) error {
	home, err := droidHomeDir()
	if err != nil {
		return err
	}
	return droidRenameAuthProfile(home, name, label)
}
