// Legacy Factory config cleanup. Mirrors factory_settings::
// check_legacy_config / delete_legacy_config (droidgear-core/src/
// factory_settings.rs) behind the LegacyConfigDialog contract:
//
//	check → `<factory home>/config.json` exists AND
//	        `<factory home>/settings.json` carries a non-empty customModels
//	delete → remove `<factory home>/config.json`
//
// Note: unlike factory.go's readFactoryConfig (which routes through the
// active settings file), the legacy check reads the plain settings.json in
// the factory directory — the same file the Rust command reads.
package legacy

import (
	"fmt"
	"os"
	"path/filepath"

	"droidgear-wails/internal/core"
)

// legacyFactoryDir resolves the factory home (honouring the Preferences →
// Paths override); the error string matches Rust system_home_dir.
func legacyFactoryDir() (string, error) {
	dir := core.FactoryHome()
	if dir == "" {
		return "", errString("Failed to get home directory")
	}
	return dir, nil
}

// legacyConfigJSONPath is the deprecated `<factory>/config.json`.
func legacyConfigJSONPath() (string, error) {
	dir, err := legacyFactoryDir()
	if err != nil {
		return "", err
	}
	return filepath.Join(dir, "config.json"), nil
}

// legacySettingsJSONPath is the plain `<factory>/settings.json` the legacy
// check consults for migrated customModels.
func legacySettingsJSONPath() (string, error) {
	dir, err := legacyFactoryDir()
	if err != nil {
		return "", err
	}
	return filepath.Join(dir, "settings.json"), nil
}

// Check reports whether the deprecated config.json still exists
// and settings.json already holds custom models (Rust
// `check_legacy_config`). Read/parse failures report false — the dialog
// simply stays closed.
func Check() (bool, error) {
	legacyPath, err := legacyConfigJSONPath()
	if err != nil {
		return false, err
	}
	if _, err := os.Stat(legacyPath); err != nil {
		return false, nil
	}
	settingsPath, err := legacySettingsJSONPath()
	if err != nil {
		return false, err
	}
	data, err := os.ReadFile(settingsPath) // #nosec G304 -- fixed factory path
	if err != nil || len(data) == 0 {
		return false, nil
	}
	settings, ok := core.DecodeJSONObject(string(data))
	if !ok {
		return false, nil
	}
	models, ok := settings["customModels"].([]any)
	return ok && len(models) > 0, nil
}

// Delete removes the deprecated config.json (Rust
// `delete_legacy_config`); a missing file is a no-op.
func Delete() error {
	legacyPath, err := legacyConfigJSONPath()
	if err != nil {
		return err
	}
	if _, err := os.Stat(legacyPath); err != nil {
		return nil
	}
	if err := os.Remove(legacyPath); err != nil {
		return fmt.Errorf("Failed to delete legacy config: %w", err)
	}
	return nil
}
