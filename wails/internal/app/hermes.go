// Hermes commands. The logic lives in internal/service/hermes (design doc
// §2 R2); this file only marshals arguments for the frontend bindings —
// every signature, parameter names included, is unchanged, because the
// generated bindings (and therefore scripts/check-command-surface.mjs)
// match on them.
package app

import (
	"droidgear-wails/internal/service/hermes"
)

// Type aliases — one definition, in the service.
type (
	HermesProfile       = hermes.HermesProfile
	HermesModelConfig   = hermes.HermesModelConfig
	HermesConfigStatus  = hermes.HermesConfigStatus
	HermesCurrentConfig = hermes.HermesCurrentConfig
)

// ListHermesProfiles lists all Hermes profiles sorted by name.
func (a *App) ListHermesProfiles() ([]HermesProfile, error) {
	return hermes.ListProfiles()
}

// GetHermesProfile returns one profile by id.
func (a *App) GetHermesProfile(id string) (HermesProfile, error) {
	return hermes.GetProfile(id)
}

// SaveHermesProfile creates or updates a profile.
func (a *App) SaveHermesProfile(profile HermesProfile) error {
	return hermes.SaveProfile(profile)
}

// DeleteHermesProfile removes a profile and clears the active pointer.
func (a *App) DeleteHermesProfile(id string) error {
	return hermes.DeleteProfile(id)
}

// DuplicateHermesProfile copies a profile under a new name and id.
func (a *App) DuplicateHermesProfile(id string, newName string) (HermesProfile, error) {
	return hermes.DuplicateProfile(id, newName)
}

// CreateDefaultHermesProfile seeds the first profile when none exist.
func (a *App) CreateDefaultHermesProfile() (HermesProfile, error) {
	return hermes.CreateDefaultProfile()
}

// GetActiveHermesProfileId returns the active profile id (null when unset).
func (a *App) GetActiveHermesProfileId() (*string, error) {
	return hermes.GetActiveProfileID()
}

// ApplyHermesProfile applies a profile to the live ~/.hermes/config.yaml.
func (a *App) ApplyHermesProfile(id string) error {
	return hermes.ApplyProfile(id)
}

// GetHermesConfigStatus reports whether the live config exists.
func (a *App) GetHermesConfigStatus() (HermesConfigStatus, error) {
	return hermes.GetConfigStatus()
}

// ReadHermesCurrentConfig reads the live Hermes configuration.
func (a *App) ReadHermesCurrentConfig() (HermesCurrentConfig, error) {
	return hermes.ReadCurrentConfig()
}
