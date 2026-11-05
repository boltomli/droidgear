// Droid commands: settings-file profiles, settings values, trusted folders,
// Factory auth profiles and the `droid` launch surface.
//
// The logic lives in internal/service/droid (single write path for the GUI
// and the TUI, tui-parity §2); this file only marshals arguments, so the
// generated bindings keep their exact method names and parameter order.
// Type aliases keep a single definition: the JSON the GUI sees is produced
// by the service structs themselves.
package app

import (
	"droidgear-wails/internal/service/droid"
)

// Type aliases — one definition, in the service.
type (
	SettingsFileInfo     = droid.SettingsFileInfo
	MissionModelSettings = droid.MissionModelSettings
	TrustedFolder        = droid.TrustedFolder
	AuthProfile          = droid.AuthProfile
	AuthProfileState     = droid.AuthProfileState
)

// ---------------------------------------------------------------------------
// Settings values (active settings-file half of factory settings)
// ---------------------------------------------------------------------------

// GetCloudSessionSync reads cloudSessionSync (default true).
func (a *App) GetCloudSessionSync() (bool, error) { return droid.GetCloudSessionSync() }

// SaveCloudSessionSync stores cloudSessionSync.
func (a *App) SaveCloudSessionSync(enabled bool) error { return droid.SaveCloudSessionSync(enabled) }

// GetReasoningEffort reads reasoningEffort (with session-defaults fallback).
func (a *App) GetReasoningEffort() (*string, error) { return droid.GetReasoningEffort() }

// SaveReasoningEffort stores reasoningEffort.
func (a *App) SaveReasoningEffort(value string) error { return droid.SaveReasoningEffort(value) }

// GetDiffMode reads diffMode (default "github").
func (a *App) GetDiffMode() (string, error) { return droid.GetDiffMode() }

// SaveDiffMode stores diffMode.
func (a *App) SaveDiffMode(value string) error { return droid.SaveDiffMode(value) }

// GetTodoDisplayMode reads todoDisplayMode.
func (a *App) GetTodoDisplayMode() (string, error) { return droid.GetTodoDisplayMode() }

// SaveTodoDisplayMode stores todoDisplayMode.
func (a *App) SaveTodoDisplayMode(value string) error { return droid.SaveTodoDisplayMode(value) }

// GetIncludeCoAuthoredByDroid reads includeCoAuthoredByDroid.
func (a *App) GetIncludeCoAuthoredByDroid() (bool, error) {
	return droid.GetIncludeCoAuthoredByDroid()
}

// SaveIncludeCoAuthoredByDroid stores includeCoAuthoredByDroid.
func (a *App) SaveIncludeCoAuthoredByDroid(enabled bool) error {
	return droid.SaveIncludeCoAuthoredByDroid(enabled)
}

// GetShowThinkingInMainView reads showThinkingInMainView.
func (a *App) GetShowThinkingInMainView() (bool, error) {
	return droid.GetShowThinkingInMainView()
}

// SaveShowThinkingInMainView stores showThinkingInMainView.
func (a *App) SaveShowThinkingInMainView(enabled bool) error {
	return droid.SaveShowThinkingInMainView(enabled)
}

// GetMissionModelSettings reads mission model settings (Missions pane).
func (a *App) GetMissionModelSettings() (MissionModelSettings, error) {
	return droid.GetMissionModelSettings()
}

// SaveMissionModelSettings stores mission model settings.
func (a *App) SaveMissionModelSettings(settings MissionModelSettings) error {
	return droid.SaveMissionModelSettings(settings)
}

// GetCompactionModelMode reads compactionModelMode.
func (a *App) GetCompactionModelMode() (string, error) { return droid.GetCompactionModelMode() }

// SaveCompactionModelMode stores compactionModelMode.
func (a *App) SaveCompactionModelMode(value string) error {
	return droid.SaveCompactionModelMode(value)
}

// GetCompactionTokenLimit reads compactionTokenLimit.
func (a *App) GetCompactionTokenLimit() (int32, error) { return droid.GetCompactionTokenLimit() }

// SaveCompactionTokenLimit stores compactionTokenLimit.
func (a *App) SaveCompactionTokenLimit(value int32) error {
	return droid.SaveCompactionTokenLimit(value)
}

// GetCompactionTokenLimitPerModel reads the per-model compaction overrides.
func (a *App) GetCompactionTokenLimitPerModel() (map[string]int32, error) {
	return droid.GetCompactionTokenLimitPerModel()
}

// SaveCompactionTokenLimitPerModel stores the per-model compaction overrides.
func (a *App) SaveCompactionTokenLimitPerModel(overrides map[string]int32) error {
	return droid.SaveCompactionTokenLimitPerModel(overrides)
}

// ---------------------------------------------------------------------------
// Settings-file profiles + launch
// ---------------------------------------------------------------------------

// ListDroidSettingsFiles lists the Global file, managed profiles and linked
// external files.
func (a *App) ListDroidSettingsFiles() ([]SettingsFileInfo, error) {
	return droid.ListSettingsFiles()
}

// SetActiveDroidSettingsFile selects the active file; nil/empty switches to
// Global.
func (a *App) SetActiveDroidSettingsFile(name *string) (SettingsFileInfo, error) {
	return droid.SetActiveSettingsFile(name)
}

// CreateDroidSettingsFile creates a managed profile and switches to it.
func (a *App) CreateDroidSettingsFile(name string, copyFromActive bool) (SettingsFileInfo, error) {
	return droid.CreateSettingsFile(name, copyFromActive)
}

// DeleteDroidSettingsFile deletes a managed profile (never Global).
func (a *App) DeleteDroidSettingsFile(name string) error {
	return droid.DeleteSettingsFile(name)
}

// LinkDroidSettingsFile registers a local JSON file by reference.
func (a *App) LinkDroidSettingsFile(path string) (SettingsFileInfo, error) {
	return droid.LinkSettingsFile(path)
}

// UnlinkDroidSettingsFile removes a linked file's registration.
func (a *App) UnlinkDroidSettingsFile(path string) error {
	return droid.UnlinkSettingsFile(path)
}

// GetDroidLaunchCommand returns [command, settingsPath] for the active file.
func (a *App) GetDroidLaunchCommand() ([2]string, error) {
	return droid.GetLaunchCommand()
}

// LaunchDroid opens the active Droid profile in an external terminal. Only
// the preferences (preferred terminal, droid run) come from the shell; the
// run plan itself is the service's.
func (a *App) LaunchDroid(cwd *string) error {
	prefs, _ := a.LoadPreferences() // Rust: load_preferences(&app).unwrap_or_default()
	preferred := ""
	if prefs.PreferredTerminal != nil {
		preferred = *prefs.PreferredTerminal
	}
	runPrefs := DroidRunPreferences{}
	if prefs.DroidRun != nil {
		runPrefs = *prefs.DroidRun
	}
	return droid.Launch(cwd, runPrefs, preferred)
}

// ---------------------------------------------------------------------------
// Trusted folders
// ---------------------------------------------------------------------------

// ListDroidTrustedFolders lists the trusted folders.
func (a *App) ListDroidTrustedFolders() ([]TrustedFolder, error) {
	return droid.ListTrustedFolders()
}

// AddDroidTrustedFolder appends a trusted folder.
func (a *App) AddDroidTrustedFolder(path string) (TrustedFolder, error) {
	return droid.AddTrustedFolder(path)
}

// RemoveDroidTrustedFolder removes one directory from the list.
func (a *App) RemoveDroidTrustedFolder(path string) error {
	return droid.RemoveTrustedFolder(path)
}

// RemoveDroidTrustedFolders removes several directories in one operation.
func (a *App) RemoveDroidTrustedFolders(paths []string) error {
	return droid.RemoveTrustedFolders(paths)
}

// ---------------------------------------------------------------------------
// Factory auth profiles
// ---------------------------------------------------------------------------

// ListFactoryAuthProfiles lists saved Factory auth profiles and state.
func (a *App) ListFactoryAuthProfiles() (AuthProfileState, error) {
	return droid.ListAuthProfiles()
}

// SwitchFactoryAuthProfile restores a saved auth profile into the Factory home.
func (a *App) SwitchFactoryAuthProfile(name string) error {
	return droid.SwitchAuthProfile(name)
}

// SaveCurrentFactoryAuthProfile snapshots the live Factory auth files.
func (a *App) SaveCurrentFactoryAuthProfile(name string, label string) error {
	return droid.SaveCurrentAuthProfile(name, label)
}

// DeleteFactoryAuthProfile removes a saved auth profile.
func (a *App) DeleteFactoryAuthProfile(name string) error {
	return droid.DeleteAuthProfile(name)
}

// RenameFactoryAuthProfile updates a saved auth profile's display label.
func (a *App) RenameFactoryAuthProfile(name string, label string) error {
	return droid.RenameAuthProfile(name, label)
}
