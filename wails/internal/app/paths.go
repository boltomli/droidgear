// Path management commands for the Preferences "Paths" pane. Mirrors
// src-tauri/src/commands/paths.rs field-for-field (camelCase EffectivePaths).
package app

import (
	"droidgear-wails/internal/core"
)

// EffectivePath mirrors paths::EffectivePath.
type EffectivePath struct {
	Key       string `json:"key"`
	Path      string `json:"path"`
	IsDefault bool   `json:"isDefault"`
}

// EffectivePaths mirrors paths::EffectivePaths (all nine configurable keys).
type EffectivePaths struct {
	Factory      EffectivePath `json:"factory"`
	Opencode     EffectivePath `json:"opencode"`
	OpencodeAuth EffectivePath `json:"opencodeAuth"`
	Codex        EffectivePath `json:"codex"`
	Claude       EffectivePath `json:"claude"`
	Openclaw     EffectivePath `json:"openclaw"`
	Hermes       EffectivePath `json:"hermes"`
	Pi           EffectivePath `json:"pi"`
	Omp          EffectivePath `json:"omp"`
}

// WslInfo mirrors paths::WslInfo.
type WslInfo struct {
	Available bool                `json:"available"`
	Distros   []core.WslDistro    `json:"distros"`
}

func buildEffectivePaths() (EffectivePaths, error) {
	var out EffectivePaths
	for _, entry := range []struct {
		key string
		dst *EffectivePath
	}{
		{"factory", &out.Factory},
		{"opencode", &out.Opencode},
		{"opencodeAuth", &out.OpencodeAuth},
		{"codex", &out.Codex},
		{"claude", &out.Claude},
		{"openclaw", &out.Openclaw},
		{"hermes", &out.Hermes},
		{"pi", &out.Pi},
		{"omp", &out.Omp},
	} {
		path, isDefault, err := core.EffectivePathForKey(entry.key)
		if err != nil {
			return EffectivePaths{}, err
		}
		*entry.dst = EffectivePath{Key: entry.key, Path: path, IsDefault: isDefault}
	}
	return out, nil
}

func buildDefaultPaths() (EffectivePaths, error) {
	var out EffectivePaths
	for _, entry := range []struct {
		key string
		dst *EffectivePath
	}{
		{"factory", &out.Factory},
		{"opencode", &out.Opencode},
		{"opencodeAuth", &out.OpencodeAuth},
		{"codex", &out.Codex},
		{"claude", &out.Claude},
		{"openclaw", &out.Openclaw},
		{"hermes", &out.Hermes},
		{"pi", &out.Pi},
		{"omp", &out.Omp},
	} {
		path, err := core.DefaultPathForKey(entry.key)
		if err != nil {
			return EffectivePaths{}, err
		}
		*entry.dst = EffectivePath{Key: entry.key, Path: path, IsDefault: true}
	}
	return out, nil
}

// GetEffectivePaths returns all paths with their overrides applied.
func (a *App) GetEffectivePaths() (EffectivePaths, error) {
	return buildEffectivePaths()
}

// GetDefaultPaths returns the built-in default paths (for UI display).
func (a *App) GetDefaultPaths() (EffectivePaths, error) {
	return buildDefaultPaths()
}

// SaveConfigPath persists a single path override.
func (a *App) SaveConfigPath(key string, path string) error {
	return core.SaveConfigPathOverride(key, path)
}

// ResetConfigPath removes a single path override.
func (a *App) ResetConfigPath(key string) error {
	return core.ResetConfigPathOverride(key)
}

// GetWslInfo reports WSL availability and distributions (Windows only).
func (a *App) GetWslInfo() (WslInfo, error) {
	available, distros := core.GetWslInfo()
	if distros == nil {
		distros = []core.WslDistro{}
	}
	return WslInfo{Available: available, Distros: distros}, nil
}

// GetWslUsername resolves the username inside a WSL distribution.
func (a *App) GetWslUsername(distro string) (string, error) {
	username, ok := core.GetWslUsername(distro)
	if !ok {
		return "", errString("Failed to get WSL username")
	}
	return username, nil
}

// BuildWslPath builds the \\wsl$ path for a config key.
func (a *App) BuildWslPath(distro string, username string, configKey string) (string, error) {
	return core.BuildWslPath(distro, username, configKey)
}
