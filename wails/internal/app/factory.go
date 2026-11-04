// Factory settings (custom models, model favorites, session defaults) — the
// GUI-facing forward.
//
// The logic lives in internal/service/factory (single write path for the GUI,
// the `dg factory` subcommands and the TUI); this file only marshals
// arguments, so the generated bindings keep their exact method names and
// parameter order.
package app

import (
	"droidgear-wails/internal/service/factory"
)

// GetConfigPath returns the path of the active Factory settings file.
func (a *App) GetConfigPath() (string, error) {
	return factory.GetConfigPath()
}

// ResetConfigFile replaces the active Factory settings file with an empty
// document.
func (a *App) ResetConfigFile() error {
	return factory.ResetConfigFile()
}

// LoadCustomModels lists the BYOK custom models.
func (a *App) LoadCustomModels() ([]factory.CustomModel, error) {
	return factory.LoadCustomModels()
}

// SaveCustomModels replaces the custom model list, re-normalizing favorites
// against the new list when the key exists.
func (a *App) SaveCustomModels(models []factory.CustomModel) error {
	return factory.SaveCustomModels(models)
}

// GetModelFavorites lists the favorite model ids (normalized).
func (a *App) GetModelFavorites() ([]string, error) {
	return factory.GetModelFavorites()
}

// SaveModelFavorites stores the favorite model ids.
func (a *App) SaveModelFavorites(favorites []string) error {
	return factory.SaveModelFavorites(favorites)
}

// GetSessionDefaultSettings reads sessionDefaultSettings.
func (a *App) GetSessionDefaultSettings() (factory.SessionDefaultSettings, error) {
	return factory.GetSessionDefaultSettings()
}

// SaveSessionDefaultSettings stores sessionDefaultSettings.
func (a *App) SaveSessionDefaultSettings(settings factory.SessionDefaultSettings) error {
	return factory.SaveSessionDefaultSettings(settings)
}
