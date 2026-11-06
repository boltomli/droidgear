// Pi commands. The logic lives in internal/service/pi (design doc §2 R2);
// this file only marshals arguments for the frontend bindings — every
// signature, parameter names included, is unchanged, because the generated
// bindings (and therefore scripts/check-command-surface.mjs) match on them.
//
// The sessions viewer stays in internal/app (pi_sessions.go): it is a
// read-only poller that emits Wails events and does not belong in the
// service layer.
package app

import (
	"droidgear-wails/internal/service/pi"
)

// Type aliases — one definition, in the service.
type (
	PiProfile            = pi.PiProfile
	PiModel              = pi.PiModel
	PiModelOverride      = pi.PiModelOverride
	PiProviderConfig     = pi.PiProviderConfig
	PiConfigStatus       = pi.PiConfigStatus
	PiCurrentConfig      = pi.PiCurrentConfig
	PiProviderTestResult = pi.PiProviderTestResult
	PiSettings           = pi.PiSettings
	PiSettingsStatus     = pi.PiSettingsStatus
)

// ListPiProfiles lists all Pi profiles sorted by name.
func (a *App) ListPiProfiles() ([]PiProfile, error) {
	return pi.ListProfiles()
}

// GetPiProfile returns one profile by id.
func (a *App) GetPiProfile(id string) (PiProfile, error) {
	return pi.GetProfile(id)
}

// SavePiProfile creates or updates a profile.
func (a *App) SavePiProfile(profile PiProfile) error {
	return pi.SaveProfile(profile)
}

// DeletePiProfile removes a profile and clears the active pointer.
func (a *App) DeletePiProfile(id string) error {
	return pi.DeleteProfile(id)
}

// DuplicatePiProfile copies a profile under a new name and id.
func (a *App) DuplicatePiProfile(id string, newName string) (PiProfile, error) {
	return pi.DuplicateProfile(id, newName)
}

// CreateDefaultPiProfile seeds the first profile when none exist.
func (a *App) CreateDefaultPiProfile() (PiProfile, error) {
	return pi.CreateDefaultProfile()
}

// GetActivePiProfileId returns the active profile id (null when unset).
func (a *App) GetActivePiProfileId() (*string, error) {
	return pi.GetActiveProfileID()
}

// SetActivePiProfileId stores the active profile id.
func (a *App) SetActivePiProfileId(id string) error {
	return pi.SetActiveProfileID(id)
}

// ApplyPiProfile applies a profile to the live ~/.pi/agent/models.json and
// returns the providers it wrote plus the startup model it had to repair in
// settings.json (see pi.ReconcileStartupModel).
func (a *App) ApplyPiProfile(id string) (pi.PiApplyResult, error) {
	return pi.ApplyProfile(id)
}

// GetPiConfigStatus reports whether the live config exists.
func (a *App) GetPiConfigStatus() (PiConfigStatus, error) {
	return pi.GetConfigStatus()
}

// ReadPiCurrentConfig reads the live Pi configuration.
func (a *App) ReadPiCurrentConfig() (PiCurrentConfig, error) {
	return pi.ReadCurrentConfig()
}

// TestPiProviderConnection probes a provider endpoint with the pi CLI.
func (a *App) TestPiProviderConnection(providerID string, config PiProviderConfig) (PiProviderTestResult, error) {
	return pi.TestProviderConnection(providerID, config)
}

// ReadPiSettings reads the startup model selection from ~/.pi/agent/settings.json.
func (a *App) ReadPiSettings() (PiSettingsStatus, error) {
	return pi.ReadSettings()
}

// SavePiSettings writes the startup model selection, keeping every other key
// in settings.json.
func (a *App) SavePiSettings(settings PiSettings) error {
	return pi.SaveSettings(settings)
}

// ---------------------------------------------------------------------------
// Channel import (shared inference: service/pi + channelimport)
// ---------------------------------------------------------------------------

// Type aliases for the import contract.
type (
	PiChannelModel        = pi.PiChannelModel
	PiChannelImportInput  = pi.PiChannelImportInput
	PiChannelImportResult = pi.PiChannelImportResult
)

// PreparePiChannelImport builds the provider entry for a channel import
// without writing anything. The GUI dialog calls it to prefill the form and
// again on import; `dg pi providers import` and the TUI call the service
// directly, so all three share one rule set.
func (a *App) PreparePiChannelImport(input PiChannelImportInput) (PiChannelImportResult, error) {
	return pi.PrepareChannelImport(input)
}
