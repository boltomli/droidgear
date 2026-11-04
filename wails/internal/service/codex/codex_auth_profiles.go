// Codex auth profile management: named backups of ~/.codex/auth.json stored
// under ~/.droidgear/auth-profiles/codex/, official-vs-BYOK conflict
// detection, and the auth-model snapshot applied on switch. Mirrors
// droidgear-core::codex_auth_profiles (Rust); the frontend contract lives in
// src/components/codex/CodexAuthPage.tsx and CodexConfigPage.tsx.
package codex

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"strings"

	"droidgear-wails/internal/core"
)

// ---------------------------------------------------------------------------
// Types (camelCase JSON, matching the specta-generated TS types)
// ---------------------------------------------------------------------------

// CodexAuthProfile mirrors codex_auth_profiles::CodexAuthProfile.
type CodexAuthProfile struct {
	Name       string `json:"name"`
	Label      string `json:"label"`
	CreatedAt  string `json:"createdAt"`
	IsOfficial bool   `json:"isOfficial"`
	// The CodexProfile ID active when this auth was saved; restored together
	// with the auth on switch (config.toml only).
	CodexProfileID *string `json:"codexProfileId,omitempty"`
	// Live config.toml `model` snapshotted when this auth was saved.
	Model *string `json:"model,omitempty"`
	// Live config.toml `model_reasoning_effort` snapshotted when saved.
	ModelReasoningEffort *string `json:"modelReasoningEffort,omitempty"`
}

// CodexAuthProfileState mirrors codex_auth_profiles::CodexAuthProfileState.
type CodexAuthProfileState struct {
	Active            *string            `json:"active"`
	Profiles          []CodexAuthProfile `json:"profiles"`
	IsCurrentOfficial bool               `json:"isCurrentOfficial"`
}

// CodexAuthConflictInfo mirrors codex_auth_profiles::CodexAuthConflictInfo.
type CodexAuthConflictInfo struct {
	HasConflict       bool `json:"hasConflict"`
	CurrentIsOfficial bool `json:"currentIsOfficial"`
	TargetIsOfficial  bool `json:"targetIsOfficial"`
}

// codexAuthProfilesManifest is the on-disk profiles.json shape.
type codexAuthProfilesManifest struct {
	Active   *string            `json:"active"`
	Profiles []CodexAuthProfile `json:"profiles"`
}

// ---------------------------------------------------------------------------
// Paths
// ---------------------------------------------------------------------------

// codexAuthProfilesDir returns ~/.droidgear/auth-profiles/codex.
func codexAuthProfilesDir() (string, error) {
	dir, err := core.DroidGearDir()
	if err != nil {
		return "", err
	}
	return filepath.Join(dir, "auth-profiles", "codex"), nil
}

func codexAuthManifestPath() (string, error) {
	dir, err := codexAuthProfilesDir()
	if err != nil {
		return "", err
	}
	return filepath.Join(dir, "profiles.json"), nil
}

func AuthProfileDir(name string) (string, error) {
	dir, err := codexAuthProfilesDir()
	if err != nil {
		return "", err
	}
	return filepath.Join(dir, name), nil
}

func AuthProfileFile(name string) (string, error) {
	dir, err := AuthProfileDir(name)
	if err != nil {
		return "", err
	}
	return filepath.Join(dir, "auth.json"), nil
}

// AuthLivePath is the live <codex home>/auth.json. Unlike codexAuthPath
// it never creates the codex home (Rust auth-profile reads are side-effect
// free; matches codex_auth_path_for_home).
func AuthLivePath() (string, error) {
	dir, err := ConfigDirNoCreate()
	if err != nil {
		return "", err
	}
	return filepath.Join(dir, "auth.json"), nil
}

// codexConfigLivePath is the live <codex home>/config.toml (no side effects).
func codexConfigLivePath() (string, error) {
	dir, err := ConfigDirNoCreate()
	if err != nil {
		return "", err
	}
	return filepath.Join(dir, "config.toml"), nil
}

// EnsureDir creates a directory, mirroring Rust ensure_dir's message.
func EnsureDir(dir string) error {
	if err := core.EnsureDir(dir); err != nil {
		return fmt.Errorf("Failed to create directory %s: %w", dir, err)
	}
	return nil
}

// ---------------------------------------------------------------------------
// Manifest I/O
// ---------------------------------------------------------------------------

func ReadAuthManifest() codexAuthProfilesManifest {
	path, err := codexAuthManifestPath()
	if err != nil {
		return codexAuthProfilesManifest{Profiles: []CodexAuthProfile{}}
	}
	data, err := os.ReadFile(path) // #nosec G304 -- manifest path
	if err != nil || strings.TrimSpace(string(data)) == "" {
		return codexAuthProfilesManifest{Profiles: []CodexAuthProfile{}}
	}
	var manifest codexAuthProfilesManifest
	if err := json.Unmarshal(data, &manifest); err != nil || manifest.Profiles == nil {
		return codexAuthProfilesManifest{Profiles: []CodexAuthProfile{}}
	}
	return manifest
}

func WriteAuthManifest(manifest codexAuthProfilesManifest) error {
	path, err := codexAuthManifestPath()
	if err != nil {
		return err
	}
	parent := filepath.Dir(path)
	if err := EnsureDir(parent); err != nil {
		return err
	}
	if manifest.Profiles == nil {
		manifest.Profiles = []CodexAuthProfile{}
	}
	data, err := json.MarshalIndent(manifest, "", "  ")
	if err != nil {
		return fmt.Errorf("Failed to serialize profiles manifest: %w", err)
	}
	return core.AtomicWrite(path, data)
}

// ---------------------------------------------------------------------------
// Validation
// ---------------------------------------------------------------------------

func ValidateAuthProfileName(name string) error {
	if name == "" {
		return errString("Profile name cannot be empty")
	}
	for _, r := range name {
		if (r >= 'a' && r <= 'z') || (r >= 'A' && r <= 'Z') || (r >= '0' && r <= '9') || r == '-' || r == '_' {
			continue
		}
		return errString("Profile name can only contain alphanumeric characters, hyphens, and underscores")
	}
	return nil
}

// ---------------------------------------------------------------------------
// Auth detection + reads
// ---------------------------------------------------------------------------

// IsAuthOfficial: official ChatGPT login carries an auth_mode field;
// BYOK auth only holds OPENAI_API_KEY.
func IsAuthOfficial(auth map[string]any) bool {
	_, ok := auth["auth_mode"]
	return ok
}

// ReadLiveAuth reads the live auth.json (missing/blank/non-object →
// empty map; parse failures error like the Rust helper).
func ReadLiveAuth() (map[string]any, error) {
	path, err := AuthLivePath()
	if err != nil {
		return nil, err
	}
	data, err := os.ReadFile(path) // #nosec G304 -- live auth.json
	if err != nil {
		if os.IsNotExist(err) {
			return map[string]any{}, nil
		}
		return nil, fmt.Errorf("Failed to read auth.json: %w", err)
	}
	if strings.TrimSpace(string(data)) == "" {
		return map[string]any{}, nil
	}
	var v any
	if err := json.Unmarshal(data, &v); err != nil {
		return nil, fmt.Errorf("Invalid auth.json: %w", err)
	}
	m, ok := v.(map[string]any)
	if !ok {
		return map[string]any{}, nil
	}
	return m, nil
}

// ReadProfileAuth reads a saved profile's auth.json with the
// profile-specific error strings.
func ReadProfileAuth(name string) (map[string]any, error) {
	path, err := AuthProfileFile(name)
	if err != nil {
		return nil, err
	}
	data, err := os.ReadFile(path) // #nosec G304 -- profile auth.json
	if err != nil {
		if os.IsNotExist(err) {
			return map[string]any{}, nil
		}
		return nil, fmt.Errorf("Failed to read profile auth.json: %w", err)
	}
	if strings.TrimSpace(string(data)) == "" {
		return map[string]any{}, nil
	}
	var v any
	if err := json.Unmarshal(data, &v); err != nil {
		return nil, fmt.Errorf("Invalid profile auth.json: %w", err)
	}
	m, ok := v.(map[string]any)
	if !ok {
		return map[string]any{}, nil
	}
	return m, nil
}

func codexAuthJSONExists() bool {
	path, err := AuthLivePath()
	if err != nil {
		return false
	}
	return PathExists(path)
}

// ReadActiveCodexProfileID reads ~/.droidgear/codex/active-profile.txt
// (read-only — no directory creation, matching Rust's plain path join).
func ReadActiveCodexProfileID() *string {
	dir, err := codexDroidGearDir()
	if err != nil {
		return nil
	}
	data, err := os.ReadFile(filepath.Join(dir, "active-profile.txt")) // #nosec G304 -- active profile pointer
	if err != nil {
		return nil
	}
	id := strings.TrimSpace(string(data))
	if id == "" {
		return nil
	}
	return &id
}

// ReadLiveModelSnapshot extracts trimmed non-empty `model` /
// `model_reasoning_effort` from the live config.toml (best effort).
func ReadLiveModelSnapshot() (*string, *string) {
	path, err := codexConfigLivePath()
	if err != nil || !PathExists(path) {
		return nil, nil
	}
	config, err := codexReadConfigFile(path)
	if err != nil {
		return nil, nil
	}
	trimmed := func(key string) *string {
		if s, ok := config[key].(string); ok {
			if v := strings.TrimSpace(s); v != "" {
				return &v
			}
		}
		return nil
	}
	return trimmed("model"), trimmed("model_reasoning_effort")
}

// CurrentAuthIsOfficial reports whether the live auth is official,
// treating a missing file as BYOK (false).
func CurrentAuthIsOfficial() (bool, error) {
	if !codexAuthJSONExists() {
		return false, nil
	}
	auth, err := ReadLiveAuth()
	if err != nil {
		return false, err
	}
	return IsAuthOfficial(auth), nil
}

// ---------------------------------------------------------------------------
// Model snapshot overlay
// ---------------------------------------------------------------------------

// ApplyAuthProfileModelToLiveConfig writes the snapshotted model /
// reasoning effort back into config.toml after an auth switch. No snapshot
// fields → no write.
func ApplyAuthProfileModelToLiveConfig(profile CodexAuthProfile) error {
	var model, effort *string
	if profile.Model != nil {
		if v := strings.TrimSpace(*profile.Model); v != "" {
			model = &v
		}
	}
	if profile.ModelReasoningEffort != nil {
		if v := strings.TrimSpace(*profile.ModelReasoningEffort); v != "" {
			effort = &v
		}
	}
	if model == nil && effort == nil {
		return nil
	}

	configPath, err := codexConfigLivePath()
	if err != nil {
		return err
	}
	if err := EnsureDir(filepath.Dir(configPath)); err != nil {
		return err
	}
	config, err := codexReadConfigFile(configPath)
	if err != nil {
		return err
	}
	if model != nil {
		config["model"] = *model
	}
	if effort != nil {
		config["model_reasoning_effort"] = *effort
	}
	return codexWriteConfigFile(configPath, config)
}

// ---------------------------------------------------------------------------
// Commands (Rust public API → App methods)
// ---------------------------------------------------------------------------

// ListCodexAuthProfiles lists saved auth profiles plus whether the live auth
// is official.
func ListAuthProfiles() (CodexAuthProfileState, error) {
	manifest := ReadAuthManifest()
	isCurrentOfficial, err := CurrentAuthIsOfficial()
	if err != nil {
		isCurrentOfficial = false
	}
	return CodexAuthProfileState{
		Active:            manifest.Active,
		Profiles:          manifest.Profiles,
		IsCurrentOfficial: isCurrentOfficial,
	}, nil
}

// SaveCurrentCodexAuthProfile copies the live auth.json into a named profile
// and snapshots the active CodexProfile id + model for later restore.
func SaveCurrentAuthProfile(name string, label string) error {
	if err := ValidateAuthProfileName(name); err != nil {
		return err
	}
	isOfficial, err := CurrentAuthIsOfficial()
	if err != nil {
		return err
	}
	if !isOfficial {
		return errString("Current auth is not an official login configuration. Only official login (codex login) credentials can be saved as auth profiles.")
	}
	livePath, err := AuthLivePath()
	if err != nil {
		return err
	}
	if !PathExists(livePath) {
		return errString("No auth.json found")
	}
	profileDir, err := AuthProfileDir(name)
	if err != nil {
		return err
	}
	if err := EnsureDir(profileDir); err != nil {
		return err
	}
	profilePath, err := AuthProfileFile(name)
	if err != nil {
		return err
	}
	if err := CopyFile(livePath, profilePath, "Failed to copy auth.json to profile"); err != nil {
		return err
	}

	auth, err := ReadLiveAuth()
	if err != nil {
		return err
	}
	model, effort := ReadLiveModelSnapshot()
	entry := CodexAuthProfile{
		Name:                 name,
		Label:                label,
		CreatedAt:            core.NowRFC3339(),
		IsOfficial:           IsAuthOfficial(auth),
		CodexProfileID:       ReadActiveCodexProfileID(),
		Model:                model,
		ModelReasoningEffort: effort,
	}

	manifest := ReadAuthManifest()
	found := false
	for i := range manifest.Profiles {
		if manifest.Profiles[i].Name == name {
			manifest.Profiles[i].Label = label
			manifest.Profiles[i].IsOfficial = entry.IsOfficial
			manifest.Profiles[i].CodexProfileID = entry.CodexProfileID
			manifest.Profiles[i].Model = entry.Model
			manifest.Profiles[i].ModelReasoningEffort = entry.ModelReasoningEffort
			found = true
			break
		}
	}
	if !found {
		manifest.Profiles = append(manifest.Profiles, entry)
	}
	if manifest.Active == nil {
		manifest.Active = &name
	}
	return WriteAuthManifest(manifest)
}

// SwitchCodexAuthProfile restores a saved auth.json into the live CODEX_HOME,
// backing up the current one, then re-applies the associated CodexProfile
// (config.toml only) and the snapshotted model.
func SwitchAuthProfile(name string) error {
	if err := ValidateAuthProfileName(name); err != nil {
		return err
	}
	manifest := ReadAuthManifest()
	found := false
	for _, p := range manifest.Profiles {
		if p.Name == name {
			found = true
			break
		}
	}
	if !found {
		return fmt.Errorf("Profile '%s' not found", name)
	}
	targetPath, err := AuthProfileFile(name)
	if err != nil {
		return err
	}
	if !PathExists(targetPath) {
		return fmt.Errorf("Auth file missing for profile '%s'", name)
	}
	livePath, err := AuthLivePath()
	if err != nil {
		return err
	}

	// Backup the current live auth into the currently active profile.
	if manifest.Active != nil && *manifest.Active != name && PathExists(livePath) {
		currentActive := *manifest.Active
		currentDir, err := AuthProfileDir(currentActive)
		if err != nil {
			return err
		}
		if err := EnsureDir(currentDir); err != nil {
			return err
		}
		currentPath, err := AuthProfileFile(currentActive)
		if err != nil {
			return err
		}
		if err := CopyFile(livePath, currentPath, fmt.Sprintf("Failed to backup current auth to profile '%s'", currentActive)); err != nil {
			return err
		}
		// Refresh the backed-up profile's flags + model snapshot.
		if auth, err := ReadLiveAuth(); err == nil {
			isOfficial := IsAuthOfficial(auth)
			model, effort := ReadLiveModelSnapshot()
			for i := range manifest.Profiles {
				if manifest.Profiles[i].Name == currentActive {
					manifest.Profiles[i].IsOfficial = isOfficial
					manifest.Profiles[i].Model = model
					manifest.Profiles[i].ModelReasoningEffort = effort
					if err := WriteAuthManifest(manifest); err != nil {
						return err
					}
					break
				}
			}
		}
	}

	// Copy the target profile auth into the live location.
	if err := EnsureDir(filepath.Dir(livePath)); err != nil {
		return err
	}
	if err := CopyFile(targetPath, livePath, "Failed to copy profile auth to live location"); err != nil {
		return err
	}

	manifest = ReadAuthManifest()
	manifest.Active = &name
	var authProfile *CodexAuthProfile
	var codexProfileID *string
	for i := range manifest.Profiles {
		if manifest.Profiles[i].Name == name {
			authProfile = &manifest.Profiles[i]
			codexProfileID = manifest.Profiles[i].CodexProfileID
			break
		}
	}
	if err := WriteAuthManifest(manifest); err != nil {
		return err
	}

	// Restore the associated CodexProfile config; failure only warns (Rust
	// log::warn) because the auth restore itself already succeeded.
	if codexProfileID != nil && *codexProfileID != "" {
		_ = ApplyProfileConfigOnly(*codexProfileID)
	}
	// The auth profile's model snapshot wins over a possibly-empty
	// CodexProfile.model.
	if authProfile != nil {
		return ApplyAuthProfileModelToLiveConfig(*authProfile)
	}
	return nil
}

// DeleteCodexAuthProfile removes a saved auth profile (the active one is
// protected).
func DeleteAuthProfile(name string) error {
	if err := ValidateAuthProfileName(name); err != nil {
		return err
	}
	manifest := ReadAuthManifest()
	found := false
	for _, p := range manifest.Profiles {
		if p.Name == name {
			found = true
			break
		}
	}
	if !found {
		return fmt.Errorf("Profile '%s' not found", name)
	}
	if manifest.Active != nil && *manifest.Active == name {
		return errString("Cannot delete the currently active profile")
	}
	profileDir, err := AuthProfileDir(name)
	if err != nil {
		return err
	}
	if PathExists(profileDir) {
		if err := os.RemoveAll(profileDir); err != nil {
			return fmt.Errorf("Failed to delete profile directory: %w", err)
		}
	}
	kept := make([]CodexAuthProfile, 0, len(manifest.Profiles))
	for _, p := range manifest.Profiles {
		if p.Name != name {
			kept = append(kept, p)
		}
	}
	manifest.Profiles = kept
	return WriteAuthManifest(manifest)
}

// RenameCodexAuthProfile updates a profile's display label.
func RenameAuthProfile(name string, label string) error {
	if err := ValidateAuthProfileName(name); err != nil {
		return err
	}
	manifest := ReadAuthManifest()
	updated := false
	for i := range manifest.Profiles {
		if manifest.Profiles[i].Name == name {
			manifest.Profiles[i].Label = label
			updated = true
			break
		}
	}
	if !updated {
		return fmt.Errorf("Profile '%s' not found", name)
	}
	return WriteAuthManifest(manifest)
}

// DetectCodexAuthConflict reports whether switching to the named auth
// profile crosses the official/BYOK boundary.
func DetectAuthConflict(profileName string) (CodexAuthConflictInfo, error) {
	currentIsOfficial, err := CurrentAuthIsOfficial()
	if err != nil {
		currentIsOfficial = false
	}
	targetAuth, err := ReadProfileAuth(profileName)
	if err != nil {
		return CodexAuthConflictInfo{}, err
	}
	targetIsOfficial := IsAuthOfficial(targetAuth)
	return CodexAuthConflictInfo{
		HasConflict:       currentIsOfficial != targetIsOfficial,
		CurrentIsOfficial: currentIsOfficial,
		TargetIsOfficial:  targetIsOfficial,
	}, nil
}

// DetectCodexApplyAuthConflict reports the conflict for applying a
// CodexProfile (official target ⇔ model_provider == "openai"). Custom
// providers no longer overwrite auth.json, so BYOK→official is the only
// conflicting direction.
func DetectApplyAuthConflict(codexProfileID string) (CodexAuthConflictInfo, error) {
	currentIsOfficial, err := CurrentAuthIsOfficial()
	if err != nil {
		currentIsOfficial = false
	}
	profile, err := GetProfile(codexProfileID)
	if err != nil {
		return CodexAuthConflictInfo{}, err
	}
	targetIsOfficial := profile.ModelProvider == "openai"
	return CodexAuthConflictInfo{
		HasConflict:       targetIsOfficial && !currentIsOfficial,
		CurrentIsOfficial: currentIsOfficial,
		TargetIsOfficial:  targetIsOfficial,
	}, nil
}

// ---------------------------------------------------------------------------
// Internals shared with codex.go apply
// ---------------------------------------------------------------------------

// CopyFile copies src over dst atomically, wrapping failures with the
// caller's message prefix (Rust fs::copy error strings).
func CopyFile(src string, dst string, message string) error {
	data, err := os.ReadFile(src) // #nosec G304 -- tool-managed paths
	if err != nil {
		return fmt.Errorf("%s: %w", message, err)
	}
	if err := core.AtomicWrite(dst, data); err != nil {
		return fmt.Errorf("%s: %w", message, err)
	}
	return nil
}

// codexRestoreAuthFile restores a saved official auth profile into the live
// CODEX_HOME and marks it active (used when applying a CodexProfile with
// auth_profile_name). Mirrors restore_auth_file_for_home.
func codexRestoreAuthFile(name string) error {
	if err := ValidateAuthProfileName(name); err != nil {
		return err
	}
	manifest := ReadAuthManifest()
	var profile *CodexAuthProfile
	for i := range manifest.Profiles {
		if manifest.Profiles[i].Name == name {
			profile = &manifest.Profiles[i]
			break
		}
	}
	if profile == nil {
		return fmt.Errorf("Auth profile '%s' not found", name)
	}
	if !profile.IsOfficial {
		return fmt.Errorf("Auth profile '%s' is not an official login backup", name)
	}
	targetPath, err := AuthProfileFile(name)
	if err != nil {
		return err
	}
	if !PathExists(targetPath) {
		return fmt.Errorf("Auth file missing for profile '%s'", name)
	}
	livePath, err := AuthLivePath()
	if err != nil {
		return err
	}
	if err := EnsureDir(filepath.Dir(livePath)); err != nil {
		return err
	}
	if err := CopyFile(targetPath, livePath, fmt.Sprintf("Failed to restore auth profile '%s'", name)); err != nil {
		return err
	}

	manifest = ReadAuthManifest()
	manifest.Active = &name
	if err := WriteAuthManifest(manifest); err != nil {
		return err
	}
	return ApplyAuthProfileModelToLiveConfig(*profile)
}

// codexClearActiveAuthProfile drops the active marker (the live auth no
// longer matches any saved profile). Errors are ignored by callers, matching
// Rust's `let _ = clear_active_for_home(...)`.
func codexClearActiveAuthProfile() error {
	manifest := ReadAuthManifest()
	manifest.Active = nil
	return WriteAuthManifest(manifest)
}
