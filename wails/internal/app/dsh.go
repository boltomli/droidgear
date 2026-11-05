// Dsh commands: profile discovery, providers, config snapshot/write and
// credentials — the GUI-facing forward.
//
// The logic lives in internal/service/dsh (single write path for the GUI
// and the TUI, tui-parity §2); this file only marshals arguments, so the
// generated bindings keep their exact method names and parameter order.
// Type aliases keep a single definition: the JSON the GUI sees is produced
// by the service structs themselves.
package app

import (
	"droidgear-wails/internal/service/dsh"
)

// Type aliases — one definition, in the service.
type (
	DshCompatConfig      = dsh.DshCompatConfig
	DshModel             = dsh.DshModel
	DshProviderConfig    = dsh.DshProviderConfig
	DshCurrentConfig     = dsh.DshCurrentConfig
	DshProfile           = dsh.DshProfile
	DshConfigStatus      = dsh.DshConfigStatus
	DshCredentials       = dsh.DshCredentials
	DshMigrationWarning  = dsh.DshMigrationWarning
	DshConfigFieldView   = dsh.DshConfigFieldView
	DshConfigEntryView   = dsh.DshConfigEntryView
	DshConfigSnapshot    = dsh.DshConfigSnapshot
	DshConfigWriteResult = dsh.DshConfigWriteResult
)

// ListDshProfiles lists the known Dsh profiles (desktop, web).
func (a *App) ListDshProfiles() ([]DshProfile, error) {
	return dsh.ListProfiles()
}

// ReadDshCurrentConfig reads the live config of a profile.
func (a *App) ReadDshCurrentConfig(profile *string) (DshCurrentConfig, error) {
	return dsh.ReadCurrentConfig(profile)
}

// SaveDshProvider creates or updates a provider in the profile's patch file.
func (a *App) SaveDshProvider(profile *string, providerID string, config DshProviderConfig) error {
	return dsh.SaveProvider(profile, providerID, config)
}

// DeleteDshProvider removes a provider from the profile's patch file.
func (a *App) DeleteDshProvider(profile *string, providerID string) error {
	return dsh.DeleteProvider(profile, providerID)
}

// GetDshConfigStatus reports whether the live config files exist.
func (a *App) GetDshConfigStatus(profile *string) (DshConfigStatus, error) {
	return dsh.GetConfigStatus(profile)
}

// ReadDshCredentials reads the credential references.
func (a *App) ReadDshCredentials() (DshCredentials, error) {
	return dsh.ReadCredentials()
}

// SaveDshCredentialRef stores one credential reference.
func (a *App) SaveDshCredentialRef(name string, value string) error {
	return dsh.SaveCredentialRef(name, value)
}

// DeleteDshCredentialRef removes one credential reference.
func (a *App) DeleteDshCredentialRef(name string) error {
	return dsh.DeleteCredentialRef(name)
}

// FetchDshModels fetches the model list from a provider's models endpoint.
func (a *App) FetchDshModels(baseURL string, apiKey string, api *string) ([]DshModel, error) {
	return dsh.FetchModels(baseURL, apiKey, api)
}

// ReadDshConfigSnapshot reads the composed config of a profile.
func (a *App) ReadDshConfigSnapshot(profile *string) (DshConfigSnapshot, error) {
	return dsh.ReadConfigSnapshot(profile)
}

// SaveDshConfigEntry writes one config entry with an optimistic hash check.
func (a *App) SaveDshConfigEntry(profile *string, entryID string, config any, expectedHash *string) (DshConfigWriteResult, error) {
	return dsh.SaveConfigEntry(profile, entryID, config, expectedHash)
}

// SetDshConfigEntryDisabled toggles one config entry with a hash check.
func (a *App) SetDshConfigEntryDisabled(profile *string, entryID string, disabled bool, expectedHash *string) (DshConfigWriteResult, error) {
	return dsh.SetConfigEntryDisabled(profile, entryID, disabled, expectedHash)
}
