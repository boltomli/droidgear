// Legacy Factory config cleanup — the GUI-facing forward.
//
// The logic lives in internal/service/legacy (the LegacyConfigDialog
// contract: check for the pre-settings.json config.json, delete it); this
// file only keeps the generated bindings' method names and parameter order.
package app

import (
	"droidgear-wails/internal/service/legacy"
)

// CheckLegacyConfig reports whether a legacy config.json still exists.
func (a *App) CheckLegacyConfig() (bool, error) {
	return legacy.Check()
}

// DeleteLegacyConfig removes the legacy config.json (no-op when missing).
func (a *App) DeleteLegacyConfig() error {
	return legacy.Delete()
}
