// OMP commands. The logic lives in internal/service/omp (design doc §2 R2);
// this file only marshals arguments for the frontend bindings — every
// signature, parameter names included, is unchanged, because the generated
// bindings (and therefore scripts/check-command-surface.mjs) match on them.
//
// The sessions viewer stays in internal/app (omp_sessions.go): it is a
// read-only poller over the OMP SQLite databases that emits Wails events.
package app

import (
	"droidgear-wails/internal/service/omp"
)

// Type aliases — one definition, in the service.
type (
	OmpProfile             = omp.OmpProfile
	OmpModelRoles          = omp.OmpModelRoles
	OmpCustomProvider      = omp.OmpCustomProvider
	OmpCustomProviderEntry = omp.OmpCustomProviderEntry
	OmpCustomProviders     = omp.OmpCustomProviders
	OmpConfigStatus        = omp.OmpConfigStatus
	OmpCurrentConfig       = omp.OmpCurrentConfig
	OmpProviderTestResult  = omp.OmpProviderTestResult
	OmpRoleAdjustment      = omp.OmpRoleAdjustment
	OmpRoleReconcile       = omp.OmpRoleReconcile
	OmpRoleOptions         = omp.OmpRoleOptions
	OmpApplyResult         = omp.OmpApplyResult
)

// ReadOmpCustomProviders lists the custom providers from models.yml.
func (a *App) ReadOmpCustomProviders() (OmpCustomProviders, error) {
	return omp.ReadCustomProviders()
}

// SaveOmpCustomProvider creates or updates a custom provider and reports the
// config.yml roles the new model list made unresolvable.
func (a *App) SaveOmpCustomProvider(providerID string, provider OmpCustomProvider) (OmpRoleReconcile, error) {
	return omp.SaveCustomProvider(providerID, provider)
}

// DeleteOmpCustomProvider removes a custom provider and reports the config.yml
// roles that pointed at it (OMP exits with "No default model selected" when the
// default role loses its provider).
func (a *App) DeleteOmpCustomProvider(providerID string) (OmpRoleReconcile, error) {
	return omp.DeleteCustomProvider(providerID)
}

// ListOmpRoleOptions lists the "provider/model-id" values a role can point at
// (models.yml plus OMP's cached catalog), shared by the GUI pickers, the TUI
// hints and `dg omp roles candidates`.
func (a *App) ListOmpRoleOptions() (OmpRoleOptions, error) {
	return omp.ListRoleOptions()
}

// ListOmpProfiles lists all OMP profiles sorted by name.
func (a *App) ListOmpProfiles() ([]OmpProfile, error) {
	return omp.ListProfiles()
}

// GetOmpProfile returns one profile by id.
func (a *App) GetOmpProfile(id string) (OmpProfile, error) {
	return omp.GetProfile(id)
}

// SaveOmpProfile creates or updates a profile.
func (a *App) SaveOmpProfile(profile OmpProfile) error {
	return omp.SaveProfile(profile)
}

// DeleteOmpProfile removes a profile and clears the active pointer.
func (a *App) DeleteOmpProfile(id string) error {
	return omp.DeleteProfile(id)
}

// DuplicateOmpProfile copies a profile under a new name and id.
func (a *App) DuplicateOmpProfile(id string, newName string) (OmpProfile, error) {
	return omp.DuplicateProfile(id, newName)
}

// GetActiveOmpProfileId returns the active profile id (null when unset).
func (a *App) GetActiveOmpProfileId() (*string, error) {
	return omp.GetActiveProfileID()
}

// SetActiveOmpProfileId stores the active profile id.
func (a *App) SetActiveOmpProfileId(id string) error {
	return omp.SetActiveProfileID(id)
}

// ApplyOmpProfile applies a profile to the live ~/.omp/agent/config.yml and
// reports roles that OMP will not be able to resolve.
func (a *App) ApplyOmpProfile(id string) (OmpApplyResult, error) {
	return omp.ApplyProfile(id)
}

// GetOmpConfigStatus reports whether the live config exists.
func (a *App) GetOmpConfigStatus() (OmpConfigStatus, error) {
	return omp.GetConfigStatus()
}

// ReadOmpCurrentConfig reads the live OMP configuration.
func (a *App) ReadOmpCurrentConfig() (OmpCurrentConfig, error) {
	return omp.ReadCurrentConfig()
}

// TestOmpProviderConnection probes a provider endpoint.
func (a *App) TestOmpProviderConnection(providerID string) (OmpProviderTestResult, error) {
	return omp.TestProviderConnection(providerID)
}

// ---------------------------------------------------------------------------
// Channel import (shared inference: service/omp + channelimport)
// ---------------------------------------------------------------------------

// Type aliases for the import contract.
type (
	OmpChannelModel        = omp.OmpChannelModel
	OmpChannelImportInput  = omp.OmpChannelImportInput
	OmpChannelImportResult = omp.OmpChannelImportResult
)

// PrepareOmpChannelImport builds the provider entry for a channel import
// without writing anything. The GUI dialog calls it to prefill the form and
// again on import; `dg omp providers import` and the TUI call the service
// directly, so all three share one rule set.
func (a *App) PrepareOmpChannelImport(input OmpChannelImportInput) (OmpChannelImportResult, error) {
	return omp.PrepareChannelImport(input)
}
