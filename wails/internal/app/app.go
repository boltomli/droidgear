// Package app exposes the Wails service methods bound to the frontend.
//
// Method names and argument orders mirror the Tauri command surface of the
// main DroidGear app (see src/lib/bindings.ts) so the existing React stores
// can be reused verbatim through the invoke shim.
package app

import (
	"encoding/json"
	"errors"
	"os"
	"path/filepath"
	"runtime"

	"droidgear-wails/internal/core"
	"droidgear-wails/internal/version"
)

// App is the Wails service root. It holds no state; every method resolves
// paths on demand, mirroring the stateless Rust command layer.
type App struct{}

// ---------------------------------------------------------------------------
// Types — preferences
// ---------------------------------------------------------------------------

// DroidRunPreferences mirrors droid_runtime::DroidRunPreferences.
type DroidRunPreferences struct {
	DisableAutoUpdateEnv    *bool `json:"disableAutoUpdateEnv"`
	UnsetAnthropicAuthToken *bool `json:"unsetAnthropicAuthToken"`
}

// AppPreferences mirrors types::AppPreferences (snake_case JSON on purpose —
// the Rust struct does not rename fields).
type AppPreferences struct {
	Theme                string               `json:"theme"`
	Language             *string              `json:"language"`
	TerminalFontFamily   *string              `json:"terminal_font_family"`
	TerminalShellCommand *string              `json:"terminal_shell_command"`
	DisableAutoUpdate    *bool                `json:"disable_auto_update"`
	PreferredTerminal    *string              `json:"preferred_terminal"`
	DroidRun             *DroidRunPreferences `json:"droid_run"`
}

// GetAppVersion returns the application version string (mirrors the Tauri
// `get_app_version` command, which also returns a bare string). The value
// lives in internal/version so the `dg` CLI reports the same build version
// as the GUI (see the ldflags note there).
func (a *App) GetAppVersion() string {
	return version.Version
}

func defaultPreferences() AppPreferences {
	return AppPreferences{Theme: "system"}
}

func appDataDir() (string, error) {
	switch runtime.GOOS {
	case "windows":
		roaming := os.Getenv("APPDATA")
		if roaming == "" {
			return "", errors.New("APPDATA is not set")
		}
		return filepath.Join(roaming, "com.droidgear.app"), nil
	case "darwin":
		home, err := core.HomeDir()
		if err != nil {
			return "", err
		}
		return filepath.Join(home, "Library", "Application Support", "com.droidgear.app"), nil
	default:
		base := os.Getenv("XDG_DATA_HOME")
		if base == "" {
			home, err := core.HomeDir()
			if err != nil {
				return "", err
			}
			base = filepath.Join(home, ".local", "share")
		}
		return filepath.Join(base, "com.droidgear.app"), nil
	}
}

func preferencesPath() (string, error) {
	dir, err := appDataDir()
	if err != nil {
		return "", err
	}
	return filepath.Join(dir, "preferences.json"), nil
}

// LoadPreferences reads preferences.json (shared with the Tauri app).
func (a *App) LoadPreferences() (AppPreferences, error) {
	path, err := preferencesPath()
	if err != nil {
		return defaultPreferences(), nil
	}
	data, err := os.ReadFile(path) // #nosec G304 -- fixed app data path
	if err != nil {
		return defaultPreferences(), nil
	}
	var prefs AppPreferences
	if err := json.Unmarshal(data, &prefs); err != nil {
		return defaultPreferences(), nil
	}
	if prefs.Theme == "" {
		prefs.Theme = "system"
	}
	return prefs, nil
}

// SavePreferences writes preferences.json atomically.
func (a *App) SavePreferences(preferences AppPreferences) error {
	path, err := preferencesPath()
	if err != nil {
		return err
	}
	data, err := json.MarshalIndent(preferences, "", "  ")
	if err != nil {
		return err
	}
	return core.AtomicWrite(path, data)
}
