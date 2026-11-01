// Path override management and per-tool default resolution for all tools
// supported by the Preferences "Paths" pane. Mirrors droidgear-core::paths
// (Rust) for the nine configurable keys.
package core

import (
	"encoding/json"
	"errors"
	"fmt"
	"path/filepath"
)

// configPathKeys lists every supported override key in storage order.
var configPathKeys = []string{
	"factory", "opencode", "opencodeAuth", "codex", "claude", "openclaw",
	"hermes", "pi", "omp",
}

func isKnownPathKey(key string) bool {
	for _, k := range configPathKeys {
		if k == key {
			return true
		}
	}
	return false
}

// DefaultPathForKey resolves the built-in default directory for a key
// (without consulting overrides). Mirrors the Rust default_*_for_home chain.
func DefaultPathForKey(key string) (string, error) {
	home, err := HomeDir()
	if err != nil {
		return "", err
	}
	switch key {
	case "factory":
		return filepath.Join(home, ".factory"), nil
	case "opencode":
		return filepath.Join(home, ".config", "opencode"), nil
	case "opencodeAuth":
		return filepath.Join(home, ".local", "share", "opencode"), nil
	case "codex":
		return filepath.Join(home, ".codex"), nil
	case "claude":
		return filepath.Join(home, ".claude"), nil
	case "openclaw":
		return filepath.Join(home, ".openclaw"), nil
	case "hermes":
		// Windows uses the WSL-aware priority chain.
		return HermesHome(), nil
	case "pi":
		return PiHome(), nil
	case "omp":
		return OmpHome(), nil
	default:
		return "", fmt.Errorf("Unknown config path key: %s", key)
	}
}

// EffectivePathForKey returns the resolved directory for a key honouring the
// user override, plus whether the value is still the built-in default.
func EffectivePathForKey(key string) (path string, isDefault bool, err error) {
	if !isKnownPathKey(key) {
		return "", false, fmt.Errorf("Unknown config path key: %s", key)
	}
	if custom := overrideForKey(key); custom != "" {
		return custom, false, nil
	}
	path, err = DefaultPathForKey(key)
	if err != nil {
		return "", false, err
	}
	return path, true, nil
}

// SaveConfigPathOverride persists a single path override into
// ~/.droidgear/settings.json → configPaths.
func SaveConfigPathOverride(key string, path string) error {
	if !isKnownPathKey(key) {
		return fmt.Errorf("Unknown config path key: %s", key)
	}
	if path == "" {
		return errors.New("Path cannot be empty")
	}
	settings, err := readSettingsForWrite()
	if err != nil {
		return err
	}
	paths, _ := settings["configPaths"].(map[string]any)
	if paths == nil {
		paths = map[string]any{}
		settings["configPaths"] = paths
	}
	paths[key] = path
	return writeSettings(settings)
}

// ResetConfigPathOverride removes a single path override.
func ResetConfigPathOverride(key string) error {
	if !isKnownPathKey(key) {
		return fmt.Errorf("Unknown config path key: %s", key)
	}
	settings, err := readSettingsForWrite()
	if err != nil {
		return err
	}
	paths, _ := settings["configPaths"].(map[string]any)
	if paths != nil {
		delete(paths, key)
		if len(paths) == 0 {
			delete(settings, "configPaths")
		}
	}
	return writeSettings(settings)
}

func readSettingsForWrite() (map[string]any, error) {
	path, err := DroidGearSettingsPath()
	if err != nil {
		return nil, err
	}
	return ReadJSONObjectFile(path), nil
}

func writeSettings(settings map[string]any) error {
	path, err := DroidGearSettingsPath()
	if err != nil {
		return err
	}
	if err := EnsureDir(filepath.Dir(path)); err != nil {
		return fmt.Errorf("Failed to create .droidgear directory: %w", err)
	}
	data, err := json.MarshalIndent(settings, "", "  ")
	if err != nil {
		return fmt.Errorf("Failed to serialize settings: %w", err)
	}
	return AtomicWrite(path, data)
}

// ---------------------------------------------------------------------------
// WSL exposure (mirrors paths::get_wsl_info / get_wsl_username / build_wsl_path)
// ---------------------------------------------------------------------------

// GetWslInfo reports whether WSL is available plus the distribution list.
func GetWslInfo() (available bool, distros []WslDistro) {
	distros = getWslInfo()
	return len(distros) > 0, distros
}

// GetWslUsername resolves the Linux username inside a distribution.
func GetWslUsername(distro string) (string, bool) {
	return getWslUsername(distro)
}

// BuildWslPath builds the \\wsl$ path for a config key inside a distro.
func BuildWslPath(distro string, username string, configKey string) (string, error) {
	subdir := map[string]string{
		"factory":      ".factory",
		"opencode":     ".config/opencode",
		"opencodeAuth": ".local/share/opencode",
		"codex":        ".codex",
		"claude":       ".claude",
		"openclaw":     ".openclaw",
		"hermes":       ".hermes",
		"pi":           ".pi/agent",
	}[configKey]
	if subdir == "" {
		return "", fmt.Errorf("Unknown config key: %s", configKey)
	}
	return `\\wsl$\` + distro + `\home\` + username + `\` + subdir, nil
}
