// Package app exposes the Wails service methods bound to the frontend.
//
// Method names and argument orders mirror the Tauri command surface of the
// main DroidGear app (see src/lib/bindings.ts) so the existing React stores
// can be reused verbatim through the invoke shim.
package app

import (
	prefs "droidgear-wails/internal/service/preferences"
	"droidgear-wails/internal/version"
)

// App is the Wails service root. It holds no state; every method resolves
// paths on demand, mirroring the stateless Rust command layer.
type App struct{}

// ---------------------------------------------------------------------------
// Types — preferences
// ---------------------------------------------------------------------------

// DroidRunPreferences mirrors droid_runtime::DroidRunPreferences.
//
// The type lives in internal/service/preferences (the single write path for
// the GUI, the TUI and any future CLI); this alias keeps the generated
// bindings' type name and JSON shape unchanged.
type DroidRunPreferences = prefs.DroidRunPreferences

// AppPreferences mirrors types::AppPreferences (snake_case JSON on purpose —
// the Rust struct does not rename fields).
type AppPreferences = prefs.AppPreferences

// GetAppVersion returns the application version string (mirrors the Tauri
// `get_app_version` command, which also returns a bare string). The value
// lives in internal/version so the `dg` CLI reports the same build version
// as the GUI (see the ldflags note there).
func (a *App) GetAppVersion() string {
	return version.Version
}

func defaultPreferences() AppPreferences {
	return prefs.Default()
}

// LoadPreferences reads preferences.json (shared with the Tauri app).
func (a *App) LoadPreferences() (AppPreferences, error) {
	return prefs.Load()
}

// SavePreferences writes preferences.json atomically.
func (a *App) SavePreferences(preferences AppPreferences) error {
	return prefs.Save(preferences)
}
