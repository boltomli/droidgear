// Factory Droid trusted-folder management. Mirrors
// droidgear-core::trusted_folders and src-tauri/src/commands/droid_settings.rs
// (the command wrappers live next to the settings-file commands there).
//
// Trusted folders are stored in the global Factory settings file and
// intentionally do not follow DroidGear's settings-file profiles: a folder
// trust decision is a machine-level permission, not a model choice.
package app

import (
	"fmt"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"time"

	"droidgear-wails/internal/core"
)

const (
	droidTrustedFoldersKey = "trustedFolders"
	droidTrustedAtKey      = "trustedAt"
)

// TrustedFolder mirrors trusted_folders::TrustedFolder (bindings.ts).
type TrustedFolder struct {
	Path      string `json:"path"`
	TrustedAt string `json:"trustedAt"`
}

// droidTrustedConfigPath resolves <factory home>/settings.json, creating the
// Factory home directory like factory_settings::factory_config_path_for_home.
func droidTrustedConfigPath(home string) (string, error) {
	factoryHome := core.FactoryHomeForHome(home)
	if err := core.EnsureDir(factoryHome); err != nil {
		return "", fmt.Errorf("Failed to create .factory directory: %w", err)
	}
	return filepath.Join(factoryHome, "settings.json"), nil
}

// droidTrustedReadConfig parses the Factory settings file (missing/empty →
// empty object; parse errors keep the CONFIG_PARSE_ERROR prefix).
func droidTrustedReadConfig(path string) (any, error) {
	data, err := os.ReadFile(path) // #nosec G304 -- factory settings path
	if err != nil {
		if os.IsNotExist(err) {
			return map[string]any{}, nil
		}
		return nil, fmt.Errorf("Failed to read Factory settings: %w", err)
	}
	if strings.TrimSpace(string(data)) == "" {
		return map[string]any{}, nil
	}
	config, err := core.DecodeJSONValue(data)
	if err != nil {
		return nil, fmt.Errorf("CONFIG_PARSE_ERROR: Failed to parse config JSON: %w", err)
	}
	return config, nil
}

// droidTrustedWriteConfig rewrites the Factory settings file through the
// shared atomic writer (symlink targets are resolved first, like Rust).
func droidTrustedWriteConfig(path string, config any) error {
	actualPath := path
	if core.IsSymlink(path) {
		resolved, err := filepath.EvalSymlinks(path)
		if err != nil {
			return fmt.Errorf("Failed to resolve Factory settings symlink: %w", err)
		}
		actualPath = resolved
	}
	data, err := core.MarshalPretty(config)
	if err != nil {
		return fmt.Errorf("Failed to serialize Factory settings: %w", err)
	}
	return droidAtomicWrite(actualPath, data)
}

// droidTrustedEntryTimestamp accepts `{ trustedAt: string }` entries and the
// bare-string shape from older experimental builds.
func droidTrustedEntryTimestamp(value any) (string, error) {
	if timestamp, isString := value.(string); isString {
		return timestamp, nil
	}
	entry, ok := value.(map[string]any)
	if !ok {
		return "", errString("Factory settings trustedFolders contains an invalid entry")
	}
	if timestamp, isString := entry[droidTrustedAtKey].(string); isString {
		return timestamp, nil
	}
	return "", nil
}

// droidTrustedNowTimestamp renders chrono's
// to_rfc3339_opts(SecondsFormat::Millis, true).
func droidTrustedNowTimestamp() string {
	return time.Now().UTC().Format("2006-01-02T15:04:05.000") + "Z"
}

// droidNormalizeTrustedFolderInput trims and validates an add request.
func droidNormalizeTrustedFolderInput(path string) (string, error) {
	trimmed := strings.TrimSpace(path)
	if trimmed == "" {
		return "", errString("Trusted folder path cannot be empty")
	}
	if !filepath.IsAbs(trimmed) {
		return "", errString("Trusted folder path must be absolute")
	}
	info, err := os.Stat(trimmed)
	if err != nil {
		return "", fmt.Errorf("Trusted folder does not exist: %w", err)
	}
	if !info.IsDir() {
		return "", errString("Trusted folder path must point to a directory")
	}
	return trimmed, nil
}

func droidListTrustedFoldersForHome(home string) ([]TrustedFolder, error) {
	configPath, err := droidTrustedConfigPath(home)
	if err != nil {
		return nil, err
	}
	config, err := droidTrustedReadConfig(configPath)
	if err != nil {
		return nil, err
	}
	folders := []TrustedFolder{}
	root, isObject := config.(map[string]any)
	if !isObject {
		return folders, nil // config.get() on a non-object → None
	}
	raw, present := root[droidTrustedFoldersKey]
	if !present || raw == nil {
		return folders, nil // absent or null → no list
	}
	entries, ok := raw.(map[string]any)
	if !ok {
		return nil, errString("Factory settings trustedFolders must be a JSON object")
	}
	for path, value := range entries {
		trustedAt, err := droidTrustedEntryTimestamp(value)
		if err != nil {
			return nil, err
		}
		folders = append(folders, TrustedFolder{Path: path, TrustedAt: trustedAt})
	}
	sort.Slice(folders, func(i, j int) bool { return folders[i].Path < folders[j].Path })
	return folders, nil
}

func droidAddTrustedFolderForHome(home string, path string) (TrustedFolder, error) {
	normalized, err := droidNormalizeTrustedFolderInput(path)
	if err != nil {
		return TrustedFolder{}, err
	}
	configPath, err := droidTrustedConfigPath(home)
	if err != nil {
		return TrustedFolder{}, err
	}
	config, err := droidTrustedReadConfig(configPath)
	if err != nil {
		return TrustedFolder{}, err
	}
	root, ok := config.(map[string]any)
	if !ok {
		return TrustedFolder{}, errString("Factory settings must contain a JSON object")
	}
	raw, present := root[droidTrustedFoldersKey]
	if !present {
		raw = map[string]any{}
		root[droidTrustedFoldersKey] = raw
	}
	entries, ok := raw.(map[string]any)
	if !ok {
		return TrustedFolder{}, errString("Factory settings trustedFolders must be a JSON object")
	}

	trustedAt := ""
	if existing, present := entries[normalized]; present {
		trustedAt, err = droidTrustedEntryTimestamp(existing)
		if err != nil {
			return TrustedFolder{}, err
		}
	}
	if trustedAt == "" {
		// New entries (and empty timestamps) are written as { trustedAt }.
		trustedAt = droidTrustedNowTimestamp()
		entries[normalized] = map[string]any{droidTrustedAtKey: trustedAt}
	}

	if err := droidTrustedWriteConfig(configPath, config); err != nil {
		return TrustedFolder{}, err
	}
	return TrustedFolder{Path: normalized, TrustedAt: trustedAt}, nil
}

func droidRemoveTrustedFoldersForHome(home string, paths []string) error {
	normalized := make([]string, 0, len(paths))
	for _, path := range paths {
		trimmed := strings.TrimSpace(path)
		if trimmed == "" {
			return errString("Trusted folder path cannot be empty")
		}
		normalized = append(normalized, trimmed)
	}
	if len(normalized) == 0 {
		return nil
	}

	configPath, err := droidTrustedConfigPath(home)
	if err != nil {
		return err
	}
	config, err := droidTrustedReadConfig(configPath)
	if err != nil {
		return err
	}
	root, ok := config.(map[string]any)
	if !ok {
		return nil // as_object_mut() → None → Ok without writing
	}
	raw, present := root[droidTrustedFoldersKey]
	if !present {
		return nil // key absent → Ok without writing
	}
	entries, ok := raw.(map[string]any)
	if !ok {
		return errString("Factory settings trustedFolders must be a JSON object")
	}
	for _, path := range normalized {
		delete(entries, path)
	}
	return droidTrustedWriteConfig(configPath, config)
}

// ListDroidTrustedFolders lists trusted folders from the global Factory
// settings file, sorted by path.
func (a *App) ListDroidTrustedFolders() ([]TrustedFolder, error) {
	home, err := droidHomeDir()
	if err != nil {
		return []TrustedFolder{}, err
	}
	folders, err := droidListTrustedFoldersForHome(home)
	if err != nil {
		return []TrustedFolder{}, err
	}
	return folders, nil
}

// AddDroidTrustedFolder adds a directory to Droid's trusted-folder list
// (idempotent: an existing entry keeps its original timestamp).
func (a *App) AddDroidTrustedFolder(path string) (TrustedFolder, error) {
	home, err := droidHomeDir()
	if err != nil {
		return TrustedFolder{}, err
	}
	return droidAddTrustedFolderForHome(home, path)
}

// RemoveDroidTrustedFolder removes one directory from the list.
func (a *App) RemoveDroidTrustedFolder(path string) error {
	return a.RemoveDroidTrustedFolders([]string{path})
}

// RemoveDroidTrustedFolders removes several directories in one
// read-modify-write operation.
func (a *App) RemoveDroidTrustedFolders(paths []string) error {
	home, err := droidHomeDir()
	if err != nil {
		return err
	}
	return droidRemoveTrustedFoldersForHome(home, paths)
}
