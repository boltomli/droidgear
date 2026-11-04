// Codex commands: profile CRUD, apply, auth profiles and the terminal
// launcher. The logic lives in internal/service/codex (design doc §2 R2);
// this file only marshals arguments for the frontend bindings — every
// signature, including parameter names, is unchanged, because the generated
// bindings (and therefore scripts/check-command-surface.mjs) match on them.
//
// Type aliases keep a single definition: the JSON the GUI sees is produced
// by the service structs themselves.
package app

import (
	"droidgear-wails/internal/service/codex"
)

// Type aliases — one definition, in the service.
type (
	CodexProfile          = codex.CodexProfile
	CodexProviderConfig   = codex.CodexProviderConfig
	CodexConfigStatus     = codex.CodexConfigStatus
	CodexCurrentConfig    = codex.CodexCurrentConfig
	CodexCliCapability    = codex.CodexCliCapability
	CodexAuthProfile      = codex.CodexAuthProfile
	CodexAuthProfileState = codex.CodexAuthProfileState
	CodexAuthConflictInfo = codex.CodexAuthConflictInfo
)

// ListCodexProfiles lists all Codex profiles sorted by name.
func (a *App) ListCodexProfiles() ([]CodexProfile, error) {
	return codex.ListProfiles()
}

// SaveCodexProfile creates or updates a profile.
func (a *App) SaveCodexProfile(profile CodexProfile) error {
	return codex.SaveProfile(profile)
}

// DeleteCodexProfile removes a profile and clears the active pointer.
func (a *App) DeleteCodexProfile(id string) error {
	return codex.DeleteProfile(id)
}

// DuplicateCodexProfile copies a profile under a new name and id.
func (a *App) DuplicateCodexProfile(id string, newName string) (CodexProfile, error) {
	return codex.DuplicateProfile(id, newName)
}

// CreateDefaultCodexProfile seeds the first profile when none exist.
func (a *App) CreateDefaultCodexProfile() (CodexProfile, error) {
	return codex.CreateDefaultProfile()
}

// GetActiveCodexProfileId returns the active profile id (null when unset).
func (a *App) GetActiveCodexProfileId() (*string, error) {
	return codex.GetActiveProfileID()
}

// ApplyCodexProfile applies a profile to the live Codex config.
func (a *App) ApplyCodexProfile(id string) error {
	return codex.ApplyProfile(id)
}

// GetCodexConfigStatus reports whether live auth/config files exist.
func (a *App) GetCodexConfigStatus() (CodexConfigStatus, error) {
	return codex.GetConfigStatus()
}

// ReadCodexCurrentConfig reads the live Codex configuration.
func (a *App) ReadCodexCurrentConfig() (CodexCurrentConfig, error) {
	return codex.ReadCurrentConfig()
}

// LaunchCodex launches Codex with a runtime CODEX_HOME snapshot instead of
// mutating the live config. Only the preferred terminal comes from the shell
// (preferences.json); the snapshot/launch path itself is the service's.
func (a *App) LaunchCodex(id string, cwd *string) error {
	preferred := ""
	if prefs, err := a.LoadPreferences(); err == nil && prefs.PreferredTerminal != nil {
		preferred = *prefs.PreferredTerminal
	}
	return codex.LaunchTemporaryRun(id, cwd, preferred)
}

// ListCodexAuthProfiles lists saved auth profiles and the current state.
func (a *App) ListCodexAuthProfiles() (CodexAuthProfileState, error) {
	return codex.ListAuthProfiles()
}

// SaveCurrentCodexAuthProfile snapshots the live auth.json under a name.
func (a *App) SaveCurrentCodexAuthProfile(name string, label string) error {
	return codex.SaveCurrentAuthProfile(name, label)
}

// SwitchCodexAuthProfile restores a saved auth profile into the live config.
func (a *App) SwitchCodexAuthProfile(name string) error {
	return codex.SwitchAuthProfile(name)
}

// DeleteCodexAuthProfile removes a saved auth profile.
func (a *App) DeleteCodexAuthProfile(name string) error {
	return codex.DeleteAuthProfile(name)
}

// RenameCodexAuthProfile updates a saved auth profile's display label.
func (a *App) RenameCodexAuthProfile(name string, label string) error {
	return codex.RenameAuthProfile(name, label)
}

// DetectCodexAuthConflict reports drift between a saved profile and the
// live auth.json.
func (a *App) DetectCodexAuthConflict(profileName string) (CodexAuthConflictInfo, error) {
	return codex.DetectAuthConflict(profileName)
}

// DetectCodexApplyAuthConflict reports drift the next apply would overwrite.
func (a *App) DetectCodexApplyAuthConflict(codexProfileID string) (CodexAuthConflictInfo, error) {
	return codex.DetectApplyAuthConflict(codexProfileID)
}
