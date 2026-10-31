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
	"time"

	"droidgear-wails/internal/core"
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

// appVersion is shown in the minimal shell's sidebar footer.
const appVersion = "0.2.0"

// GetAppVersion returns the spike's version string (mirrors the Tauri
// `get_app_version` command, which also returns a bare string).
func (a *App) GetAppVersion() string {
	return appVersion
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

// ---------------------------------------------------------------------------
// Small helpers shared by the tool modules
// ---------------------------------------------------------------------------

func nowRFC3339() string {
	return time.Now().UTC().Format(time.RFC3339)
}

// validateProfileID accepts the same ids as the Rust core: non-empty,
// alphanumeric plus '-' and '_'.
func validateProfileID(id string) error {
	if id == "" {
		return errors.New("Invalid profile id")
	}
	for _, r := range id {
		if (r >= 'a' && r <= 'z') || (r >= 'A' && r <= 'Z') || (r >= '0' && r <= '9') || r == '-' || r == '_' {
			continue
		}
		return errors.New("Invalid profile id")
	}
	return nil
}
