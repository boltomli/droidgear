// Claude Code settings commands — the GUI-facing forward.
//
// The logic lives in internal/service/claude (single write path for the GUI
// and the TUI, tui-parity §2); this file only marshals arguments, so the
// generated bindings keep their exact method names and parameter order.
// Type aliases keep a single definition: the JSON the GUI sees is produced
// by the service structs themselves.
package app

import (
	"droidgear-wails/internal/service/claude"
)

// Type aliases — one definition, in the service.
type (
	ClaudeSettingsFileInfo         = claude.ClaudeSettingsFileInfo
	ClaudeTemporaryRunDebugPreview = claude.ClaudeTemporaryRunDebugPreview
)

// ListSettingsFiles lists the managed Claude settings files.
func (a *App) ListClaudeSettingsFiles() ([]ClaudeSettingsFileInfo, error) {
	return claude.ListSettingsFiles()
}

// GetActiveSettingsFile returns the currently selected settings file.
func (a *App) GetActiveClaudeSettingsFile() (ClaudeSettingsFileInfo, error) {
	return claude.GetActiveSettingsFile()
}

// SetActiveSettingsFile selects a settings file (nil = live global).
func (a *App) SetActiveClaudeSettingsFile(name *string) (ClaudeSettingsFileInfo, error) {
	return claude.SetActiveSettingsFile(name)
}

// CreateSettingsFile creates a new managed settings file.
func (a *App) CreateClaudeSettingsFile(name string, copyFromActive bool) (ClaudeSettingsFileInfo, error) {
	return claude.CreateSettingsFile(name, copyFromActive)
}

// DeleteSettingsFile removes a managed settings file.
func (a *App) DeleteClaudeSettingsFile(name string) error {
	return claude.DeleteSettingsFile(name)
}

// ReadSettingsFile returns the parsed contents of a settings file.
func (a *App) ReadClaudeSettingsFile(name string) (any, error) {
	return claude.ReadSettingsFile(name)
}

// SaveSettingsFile replaces the contents of a settings file.
func (a *App) SaveClaudeSettingsFile(name string, contents any) error {
	return claude.SaveSettingsFile(name, contents)
}

// DuplicateSettingsFile copies a settings file under a new name.
func (a *App) DuplicateClaudeSettingsFile(name string, newName string) (ClaudeSettingsFileInfo, error) {
	return claude.DuplicateSettingsFile(name, newName)
}

// MergeSettingsToGlobal merges a settings file into the live global one.
func (a *App) MergeClaudeSettingsToGlobal(name string) error {
	return claude.MergeSettingsToGlobal(name)
}

// LoadSettingsFromLive snapshots the live global settings into a file.
func (a *App) LoadClaudeSettingsFromLive(name string) error {
	return claude.LoadSettingsFromLive(name)
}

// LaunchWithSettings launches Claude with the active settings file in
// the preferred terminal. Only the preferred terminal comes from the shell
// (preferences.json); the run plan itself is the service's.
func (a *App) LaunchClaudeWithSettings(cwd *string, skipDangerous bool) error {
	preferred := ""
	if prefs, err := a.LoadPreferences(); err == nil && prefs.PreferredTerminal != nil {
		preferred = *prefs.PreferredTerminal
	}
	return claude.LaunchWithSettings(cwd, skipDangerous, preferred)
}

// GetSettingsLaunchCommand previews the launch command.
func (a *App) GetClaudeSettingsLaunchCommand(skipDangerous bool) ([2]string, error) {
	return claude.GetSettingsLaunchCommand(skipDangerous)
}

// PreviewTemporaryRunFromFile builds the debug preview for a temporary
// run driven by a named settings file.
func (a *App) PreviewClaudeTemporaryRunFromFile(name string) (ClaudeTemporaryRunDebugPreview, error) {
	return claude.PreviewTemporaryRunFromFile(name)
}
