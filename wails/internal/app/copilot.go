// GitHub Copilot commands — the GUI-facing forward.
//
// The logic lives in internal/service/copilot (single write path for the GUI
// and the TUI, tui-parity §2); this file only marshals arguments, so the
// generated bindings keep their exact method names and parameter order.
// Type aliases keep a single definition: the JSON the GUI sees is produced
// by the service structs themselves.
package app

import (
	"droidgear-wails/internal/service/copilot"
)

// Type aliases — one definition, in the service.
type (
	CopilotProfile          = copilot.CopilotProfile
	CopilotConfigStatus     = copilot.CopilotConfigStatus
	CopilotCurrentConfig    = copilot.CopilotCurrentConfig
	CopilotChannelSelection = copilot.CopilotChannelSelection
)

// ListCopilotProfiles lists all Copilot profiles sorted by name then id.
func (a *App) ListCopilotProfiles() ([]CopilotProfile, error) {
	return copilot.ListCopilotProfiles()
}

// SaveCopilotProfile normalises and creates or updates a profile, rewriting
// its sibling env file. The live config.env is never touched here.
func (a *App) SaveCopilotProfile(profile CopilotProfile) error {
	return copilot.SaveCopilotProfile(profile)
}

// DeleteCopilotProfile removes a profile, its env file and the active marker.
func (a *App) DeleteCopilotProfile(id string) error {
	return copilot.DeleteCopilotProfile(id)
}

// DuplicateCopilotProfile copies a profile under a new name and id.
func (a *App) DuplicateCopilotProfile(id string, newName string) (CopilotProfile, error) {
	return copilot.DuplicateCopilotProfile(id, newName)
}

// CreateDefaultCopilotProfile seeds the first profile when none exist.
func (a *App) CreateDefaultCopilotProfile() (CopilotProfile, error) {
	return copilot.CreateDefaultCopilotProfile()
}

// GetActiveCopilotProfileId returns the active profile id (null when unset).
func (a *App) GetActiveCopilotProfileId() (*string, error) {
	return copilot.GetActiveCopilotProfileId()
}

// ApplyCopilotProfile applies a profile to the live config.env.
func (a *App) ApplyCopilotProfile(id string) error {
	return copilot.ApplyCopilotProfile(id)
}

// GetCopilotConfigStatus reports whether the live config.env exists.
func (a *App) GetCopilotConfigStatus() (CopilotConfigStatus, error) {
	return copilot.GetCopilotConfigStatus()
}

// PrepareCopilotChannelImport folds a channel's credentials into a profile.
func (a *App) PrepareCopilotChannelImport(profile CopilotProfile, selection CopilotChannelSelection) (CopilotProfile, error) {
	return copilot.PrepareCopilotChannelImport(profile, selection)
}

// ReadCopilotCurrentConfig reads the live config.env.
func (a *App) ReadCopilotCurrentConfig() (CopilotCurrentConfig, error) {
	return copilot.ReadCopilotCurrentConfig()
}

// LaunchCopilot launches the copilot CLI for a profile in the preferred
// terminal. Only the preferred terminal comes from the shell
// (preferences.json); the run plan itself is the service's.
func (a *App) LaunchCopilot(id string, cwd *string) error {
	preferred := ""
	if prefs, err := a.LoadPreferences(); err == nil && prefs.PreferredTerminal != nil {
		preferred = *prefs.PreferredTerminal
	}
	return copilot.LaunchCopilot(id, cwd, preferred)
}
