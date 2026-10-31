// Package core provides path resolution, atomic file writes and JSON helpers
// shared by the Wails command layer. It mirrors the semantics of
// `droidgear-core` (Rust) for the four supported tools: dsh, pi, hermes, omp.
package core

import (
	"encoding/json"
	"errors"
	"os"
	"path/filepath"
	"runtime"
	"strings"
)

// ---------------------------------------------------------------------------
// Home & DroidGear settings
// ---------------------------------------------------------------------------

// HomeDir resolves the current user's home directory.
func HomeDir() (string, error) {
	home, err := os.UserHomeDir()
	if err != nil || home == "" {
		return "", errors.New("failed to get home directory")
	}
	return home, nil
}

// DroidGearDir returns ~/.droidgear.
func DroidGearDir() (string, error) {
	home, err := HomeDir()
	if err != nil {
		return "", err
	}
	return filepath.Join(home, ".droidgear"), nil
}

// DroidGearSettingsPath returns ~/.droidgear/settings.json.
func DroidGearSettingsPath() (string, error) {
	dir, err := DroidGearDir()
	if err != nil {
		return "", err
	}
	return filepath.Join(dir, "settings.json"), nil
}

// ReadDroidGearSettings reads ~/.droidgear/settings.json as a generic map.
// A missing or empty file yields an empty map.
func ReadDroidGearSettings() map[string]any {
	path, err := DroidGearSettingsPath()
	if err != nil {
		return map[string]any{}
	}
	return ReadJSONObjectFile(path)
}

// ReadJSONObjectFile parses a JSON object file into map[string]any.
// Missing/empty/invalid files yield an empty map (best effort).
func ReadJSONObjectFile(path string) map[string]any {
	data, err := os.ReadFile(path) // #nosec G304 -- config path from app logic
	if err != nil || strings.TrimSpace(string(data)) == "" {
		return map[string]any{}
	}
	var out map[string]any
	if err := json.Unmarshal(data, &out); err != nil || out == nil {
		return map[string]any{}
	}
	return out
}

// ---------------------------------------------------------------------------
// Config path overrides (~/.droidgear/settings.json → configPaths)
// ---------------------------------------------------------------------------

// configPaths reads the configPaths override section.
func configPaths() map[string]any {
	settings := ReadDroidGearSettings()
	paths, _ := settings["configPaths"].(map[string]any)
	return paths
}

func overrideForKey(key string) string {
	paths := configPaths()
	value, _ := paths[key].(string)
	return value
}

// FactoryHome returns ~/.factory (or the configured override).
func FactoryHome() string {
	if custom := overrideForKey("factory"); custom != "" {
		return custom
	}
	home, err := HomeDir()
	if err != nil {
		return ""
	}
	return filepath.Join(home, ".factory")
}

// PiHome returns ~/.pi/agent (or the configured override).
func PiHome() string {
	if custom := overrideForKey("pi"); custom != "" {
		return custom
	}
	home, err := HomeDir()
	if err != nil {
		return ""
	}
	return filepath.Join(home, ".pi", "agent")
}

// OmpHome returns ~/.omp/agent (or the configured override).
func OmpHome() string {
	if custom := overrideForKey("omp"); custom != "" {
		return custom
	}
	home, err := HomeDir()
	if err != nil {
		return ""
	}
	return filepath.Join(home, ".omp", "agent")
}

// HermesHome resolves the Hermes config directory.
//
// Windows priority (mirrors the Rust core's
// default_hermes_home_with_priority):
//  1. %LOCALAPPDATA%/hermes when it contains config.yaml
//  2. ~/.hermes when it contains config.yaml
//  3. the default WSL distribution's \\wsl$ home
//  4. ~/.hermes (default)
func HermesHome() string {
	if custom := overrideForKey("hermes"); custom != "" {
		return custom
	}
	home, err := HomeDir()
	if err != nil {
		return ""
	}
	if runtime.GOOS == "windows" {
		return defaultHermesHomeWSL(home)
	}
	return filepath.Join(home, ".hermes")
}

// DshHome returns ~/.dsh.
func DshHome() (string, error) {
	home, err := HomeDir()
	if err != nil {
		return "", err
	}
	return filepath.Join(home, ".dsh"), nil
}

// ---------------------------------------------------------------------------
// Factory settings (active settings file) resolution
// ---------------------------------------------------------------------------

// ActiveFactorySettingsPath resolves the active Factory settings JSON file:
// the value of `droidSettingsActiveFile` in ~/.droidgear/settings.json (an
// absolute linked file, or a managed name under ~/.droidgear/droid-settings/),
// defaulting to ~/.factory/settings.json.
func ActiveFactorySettingsPath() (string, error) {
	global := filepath.Join(FactoryHome(), "settings.json")

	settings := ReadDroidGearSettings()
	name, _ := settings["droidSettingsActiveFile"].(string)
	if name == "" {
		return global, nil
	}
	if filepath.IsAbs(name) {
		if fileExists(name) {
			return name, nil
		}
		return global, nil
	}
	dir, err := DroidGearDir()
	if err != nil {
		return global, nil
	}
	managed := filepath.Join(dir, "droid-settings", strings.TrimSuffix(name, ".json")+".json")
	if fileExists(managed) {
		return managed, nil
	}
	return global, nil
}

// ---------------------------------------------------------------------------
// Small helpers
// ---------------------------------------------------------------------------

func fileExists(path string) bool {
	info, err := os.Stat(path)
	return err == nil && info.Mode().IsRegular()
}

// DirExists reports whether path is an existing directory.
func DirExists(path string) bool {
	info, err := os.Stat(path)
	return err == nil && info.IsDir()
}

// EnsureDir creates dir (and parents) when missing.
func EnsureDir(path string) error {
	if DirExists(path) {
		return nil
	}
	return os.MkdirAll(path, 0o755) // #nosec G301 -- config directories
}
