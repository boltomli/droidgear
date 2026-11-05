// Package preferences manages preferences.json (the app-level language,
// theme and terminal preferences shared with the old Tauri app). It is the
// single write path — the GUI forwards here, the TUI Settings screen reads
// and writes the same document.
package preferences

import (
	"encoding/json"
	"errors"
	"os"
	"path/filepath"
	"runtime"

	"droidgear-wails/internal/core"
)

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

// Default returns the built-in preferences (theme=system).
func Default() AppPreferences {
	return AppPreferences{Theme: "system"}
}

// AppDataDir returns the platform app-data directory holding
// preferences.json (%APPDATA%/com.droidgear.app, ~/Library/…, $XDG_DATA_HOME/…).
func AppDataDir() (string, error) {
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

// Path returns the preferences.json location.
func Path() (string, error) {
	dir, err := AppDataDir()
	if err != nil {
		return "", err
	}
	return filepath.Join(dir, "preferences.json"), nil
}

// Load reads preferences.json, falling back to the defaults on any problem
// (matching the old Tauri `load_preferences` behaviour).
func Load() (AppPreferences, error) {
	path, err := Path()
	if err != nil {
		return Default(), nil
	}
	data, err := os.ReadFile(path) // #nosec G304 -- fixed app data path
	if err != nil {
		return Default(), nil
	}
	var prefs AppPreferences
	if err := json.Unmarshal(data, &prefs); err != nil {
		return Default(), nil
	}
	if prefs.Theme == "" {
		prefs.Theme = "system"
	}
	return prefs, nil
}

// Save writes preferences.json atomically.
func Save(preferences AppPreferences) error {
	path, err := Path()
	if err != nil {
		return err
	}
	data, err := json.MarshalIndent(preferences, "", "  ")
	if err != nil {
		return err
	}
	return core.AtomicWrite(path, data)
}
