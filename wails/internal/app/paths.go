// Path management commands for the Preferences "Paths" pane. Mirrors
// src-tauri/src/commands/paths.rs field-for-field (camelCase EffectivePaths).
//
// The logic lives in internal/service/paths; this file only forwards it to
// the frontend (design doc §2 R2: app methods marshal arguments and return,
// they do not compute). The aliases keep the generated bindings — and every
// other caller — on the exact same JSON contract as before.
package app

import (
	"context"

	"droidgear-wails/internal/service/paths"
)

// EffectivePath mirrors paths::EffectivePath.
type EffectivePath = paths.EffectivePath

// EffectivePaths mirrors paths::EffectivePaths (all nine configurable keys).
type EffectivePaths = paths.EffectivePaths

// WslInfo mirrors paths::WslInfo.
type WslInfo = paths.WslInfo

// GetEffectivePaths returns all paths with their overrides applied.
func (a *App) GetEffectivePaths() (EffectivePaths, error) {
	return paths.Effective(context.Background())
}

// GetDefaultPaths returns the built-in default paths (for UI display).
func (a *App) GetDefaultPaths() (EffectivePaths, error) {
	return paths.Defaults(context.Background())
}

// SaveConfigPath persists a single path override.
func (a *App) SaveConfigPath(key string, path string) error {
	return paths.SaveOverride(context.Background(), key, path)
}

// ResetConfigPath removes a single path override.
func (a *App) ResetConfigPath(key string) error {
	return paths.ResetOverride(context.Background(), key)
}

// GetWslInfo reports WSL availability and distributions (Windows only).
func (a *App) GetWslInfo() (WslInfo, error) {
	return paths.Wsl(context.Background())
}

// GetWslUsername resolves the username inside a WSL distribution.
func (a *App) GetWslUsername(distro string) (string, error) {
	return paths.WslUsername(context.Background(), distro)
}

// BuildWslPath builds the \\wsl$ path for a config key.
func (a *App) BuildWslPath(distro string, username string, configKey string) (string, error) {
	return paths.BuildWslPath(context.Background(), distro, username, configKey)
}
