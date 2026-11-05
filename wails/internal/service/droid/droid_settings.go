// Droid settings-file profiles, Droid settings values (diff mode, cloud sync,
// reasoning effort, compaction, todo display, thinking, co-authoring, mission
// models) and the `droid` launch surface.
//
// Mirrors droidgear-core::{droid_settings_files, droid_runtime} plus the
// active-file half of factory_settings (get/save_* in commands/config.rs) and
// the Tauri wrappers in src-tauri/src/commands/droid_settings.rs. The
// external terminal launcher mirrors src-tauri/src/utils/terminal_launch.rs
// for this spec shape (no secret env, no support dir, no_keep_open = false).
package droid

import (
	"droidgear-wails/internal/service/factory"
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"strings"
	"time"

	"droidgear-wails/internal/core"
)

// ---------------------------------------------------------------------------
// Types
// ---------------------------------------------------------------------------

// SettingsFileInfo mirrors droid_settings_files::SettingsFileInfo.
type SettingsFileInfo struct {
	Name       string `json:"name"`
	Path       string `json:"path"`
	IsGlobal   bool   `json:"isGlobal"`
	IsActive   bool   `json:"isActive"`
	Exists     bool   `json:"exists"`
	IsExternal bool   `json:"isExternal"`
}

// droidExternalFile mirrors droid_settings_files::ExternalSettingsFile — a
// linked (never copied) external settings file registered under
// ~/.droidgear/settings.json → droidSettingsExternalFiles.
type droidExternalFile struct {
	Name string `json:"name"`
	Path string `json:"path"`
}

// MissionModelSettings mirrors factory_settings::MissionModelSettings
// (bindings.ts). Pointers + omitempty reproduce skip_serializing_if.
type MissionModelSettings struct {
	WorkerModel                     *string `json:"workerModel,omitempty"`
	WorkerReasoningEffort           *string `json:"workerReasoningEffort,omitempty"`
	ValidationWorkerModel           *string `json:"validationWorkerModel,omitempty"`
	ValidationWorkerReasoningEffort *string `json:"validationWorkerReasoningEffort,omitempty"`
}

// ---------------------------------------------------------------------------
// Path resolution (mirrors droid_settings_files)
// ---------------------------------------------------------------------------

const (
	droidActiveFileKey    = "droidSettingsActiveFile"
	droidExternalFilesKey = "droidSettingsExternalFiles"
)

func droidDroidGearSettingsPath(home string) string {
	return filepath.Join(home, ".droidgear", "settings.json")
}

func droidSettingsFilesDir(home string) string {
	return filepath.Join(home, ".droidgear", "droid-settings")
}

// droidGlobalSettingsPath is the "Global" settings file. The Rust core
// hardcodes <home>/.factory/settings.json here; the Wails port resolves
// through core.FactoryHomeForHome so the Preferences → Paths override keeps
// working across the whole Go service (see the porting report).
func droidGlobalSettingsPath(home string) string {
	return filepath.Join(core.FactoryHomeForHome(home), "settings.json")
}

// droidReadDroidGearSettings parses ~/.droidgear/settings.json (missing or
// empty → empty object; errors are byte-identical to the Rust core).
func droidReadDroidGearSettings(home string) (any, error) {
	path := droidDroidGearSettingsPath(home)
	data, err := os.ReadFile(path) // #nosec G304 -- droidgear settings path
	if err != nil {
		if os.IsNotExist(err) {
			return map[string]any{}, nil
		}
		return nil, fmt.Errorf("Failed to read settings: %w", err)
	}
	if strings.TrimSpace(string(data)) == "" {
		return map[string]any{}, nil
	}
	settings, err := core.DecodeJSONValue(data)
	if err != nil {
		return nil, fmt.Errorf("Failed to parse settings: %w", err)
	}
	return settings, nil
}

// droidWriteDroidGearSettings rewrites ~/.droidgear/settings.json through a
// sibling settings.tmp + rename (mirrors write_droidgear_settings_to_path).
func droidWriteDroidGearSettings(home string, settings any) error {
	path := droidDroidGearSettingsPath(home)
	dir := filepath.Dir(path)
	if !core.PathExists(dir) {
		if err := os.MkdirAll(dir, 0o755); err != nil { // #nosec G301 -- config directory
			return fmt.Errorf("Failed to create .droidgear directory: %w", err)
		}
	}
	data, err := core.MarshalPretty(settings)
	if err != nil {
		return fmt.Errorf("Failed to serialize settings: %w", err)
	}
	tempPath := core.WithExtension(path, "tmp")                 // settings.json → settings.tmp
	if err := os.WriteFile(tempPath, data, 0o644); err != nil { // #nosec G306 -- config files
		return fmt.Errorf("Failed to write settings: %w", err)
	}
	if err := os.Rename(tempPath, path); err != nil {
		_ = os.Remove(tempPath)
		return fmt.Errorf("Failed to finalize settings: %w", err)
	}
	return nil
}

// droidLoadActiveFileName returns the stored droidSettingsActiveFile value
// ("" when absent or not a string).
func droidLoadActiveFileName(home string) (string, error) {
	settings, err := droidReadDroidGearSettings(home)
	if err != nil {
		return "", err
	}
	obj, ok := settings.(map[string]any)
	if !ok {
		return "", nil
	}
	name, _ := obj[droidActiveFileKey].(string)
	return name, nil
}

// droidSaveActiveFileName stores the active file key; "" removes it (Rust
// treats None and Some("") identically).
func droidSaveActiveFileName(home string, name string) error {
	settings, err := droidReadDroidGearSettings(home)
	if err != nil {
		return err
	}
	if obj, ok := settings.(map[string]any); ok {
		if name != "" {
			obj[droidActiveFileKey] = name
		} else {
			delete(obj, droidActiveFileKey)
		}
	}
	return droidWriteDroidGearSettings(home, settings)
}

// droidActiveSettingsPath resolves the active settings file: an absolute
// stored value is a linked external file, a bare name is a managed file under
// droid-settings/; anything missing falls back to Global.
func droidActiveSettingsPath(home string) (string, error) {
	name, err := droidLoadActiveFileName(home)
	if err != nil {
		return "", err
	}
	global := droidGlobalSettingsPath(home)
	if name == "" {
		return global, nil
	}
	if filepath.IsAbs(name) {
		if core.PathExists(name) {
			return name, nil
		}
		return global, nil
	}
	managed := core.WithExtension(filepath.Join(droidSettingsFilesDir(home), name), "json")
	if core.PathExists(managed) {
		return managed, nil
	}
	return global, nil
}

func droidLoadExternalFiles(home string) ([]droidExternalFile, error) {
	settings, err := droidReadDroidGearSettings(home)
	if err != nil {
		return nil, err
	}
	value := any([]any{})
	if obj, ok := settings.(map[string]any); ok {
		if raw, present := obj[droidExternalFilesKey]; present {
			value = raw
		}
	}
	if _, isList := value.([]any); !isList {
		return nil, fmt.Errorf("Failed to parse external settings file registry: %w",
			errString("invalid type: not a sequence"))
	}
	data, err := json.Marshal(value)
	if err != nil {
		return nil, fmt.Errorf("Failed to parse external settings file registry: %w", err)
	}
	var files []droidExternalFile
	if err := json.Unmarshal(data, &files); err != nil {
		return nil, fmt.Errorf("Failed to parse external settings file registry: %w", err)
	}
	return files, nil
}

func droidSaveExternalFiles(home string, files []droidExternalFile) error {
	settings, err := droidReadDroidGearSettings(home)
	if err != nil {
		return err
	}
	if obj, ok := settings.(map[string]any); ok {
		obj[droidExternalFilesKey] = files
	}
	return droidWriteDroidGearSettings(home, settings)
}

func droidIsExternalLinked(home string, path string) (bool, error) {
	files, err := droidLoadExternalFiles(home)
	if err != nil {
		return false, err
	}
	for _, file := range files {
		if droidPathsEqual(file.Path, path) {
			return true, nil
		}
	}
	return false, nil
}

// droidDisplayNameInUse guards link display names: "Global" (any case), an
// existing linked name (case-sensitive), or an existing managed file.
func droidDisplayNameInUse(home string, name string) (bool, error) {
	if strings.EqualFold(name, "global") {
		return true, nil
	}
	files, err := droidLoadExternalFiles(home)
	if err != nil {
		return false, err
	}
	for _, file := range files {
		if file.Name == name {
			return true, nil
		}
	}
	managed := core.WithExtension(filepath.Join(droidSettingsFilesDir(home), name), "json")
	return core.PathExists(managed), nil
}

// droidPickLinkDisplayName appends -2, -3, … until a display name is free
// (mirrors pick_link_display_name_for_home: 2..1000).
func droidPickLinkDisplayName(home string, base string) (string, error) {
	inUse, err := droidDisplayNameInUse(home, base)
	if err != nil {
		return "", err
	}
	if !inUse {
		return base, nil
	}
	for i := 2; i < 1000; i++ {
		candidate := fmt.Sprintf("%s-%d", base, i)
		inUse, err := droidDisplayNameInUse(home, candidate)
		if err != nil {
			return "", err
		}
		if !inUse {
			return candidate, nil
		}
	}
	return "", fmt.Errorf("Could not find a free display name for '%s'", base)
}

// ---------------------------------------------------------------------------
// Settings-file commands
// ---------------------------------------------------------------------------

func droidListSettingsFiles(home string) ([]SettingsFileInfo, error) {
	activePath, err := droidActiveSettingsPath(home)
	if err != nil {
		return nil, err
	}
	globalPath := droidGlobalSettingsPath(home)
	files := []SettingsFileInfo{{
		Name:     "Global",
		Path:     globalPath,
		IsGlobal: true,
		IsActive: droidPathsEqual(activePath, globalPath),
		Exists:   core.PathExists(globalPath),
	}}

	if entries, err := os.ReadDir(droidSettingsFilesDir(home)); err == nil {
		for _, entry := range entries {
			name := entry.Name()
			if droidExtension(name) != "json" {
				continue
			}
			path := filepath.Join(droidSettingsFilesDir(home), name)
			files = append(files, SettingsFileInfo{
				Name:     droidFileStem(name),
				Path:     path,
				IsActive: droidPathsEqual(activePath, path),
				Exists:   true,
			})
		}
	}

	external, err := droidLoadExternalFiles(home)
	if err != nil {
		return nil, err
	}
	for _, file := range external {
		files = append(files, SettingsFileInfo{
			Name:       file.Name,
			Path:       file.Path,
			IsActive:   droidPathsEqual(activePath, file.Path),
			Exists:     core.PathExists(file.Path),
			IsExternal: true,
		})
	}
	return files, nil
}

func droidGetActiveSettingsFile(home string) (SettingsFileInfo, error) {
	files, err := droidListSettingsFiles(home)
	if err != nil {
		return SettingsFileInfo{}, err
	}
	for _, file := range files {
		if file.IsActive {
			return file, nil
		}
	}
	return SettingsFileInfo{}, errString("No active settings file found")
}

func droidSetActiveSettingsFile(home string, name string) (SettingsFileInfo, error) {
	if name != "" {
		if filepath.IsAbs(name) {
			linked, err := droidIsExternalLinked(home, name)
			if err != nil {
				return SettingsFileInfo{}, err
			}
			if !linked {
				return SettingsFileInfo{}, fmt.Errorf("Settings file '%s' is not a linked settings file", name)
			}
		} else {
			managed := core.WithExtension(filepath.Join(droidSettingsFilesDir(home), name), "json")
			if !core.PathExists(managed) {
				return SettingsFileInfo{}, fmt.Errorf("Settings file '%s' does not exist", name)
			}
		}
	}
	if err := droidSaveActiveFileName(home, name); err != nil {
		return SettingsFileInfo{}, err
	}

	activePath, err := droidActiveSettingsPath(home)
	if err != nil {
		return SettingsFileInfo{}, err
	}
	globalPath := droidGlobalSettingsPath(home)
	isGlobal := droidPathsEqual(activePath, globalPath)
	displayName := "Global"
	if !isGlobal {
		displayName = droidFileStem(droidFileName(activePath))
		if displayName == "" {
			displayName = "unknown"
		}
	}
	isExternal, err := droidIsExternalLinked(home, activePath)
	if err != nil {
		return SettingsFileInfo{}, err
	}
	return SettingsFileInfo{
		Name:       displayName,
		Path:       activePath,
		IsGlobal:   isGlobal,
		IsActive:   true,
		Exists:     core.PathExists(activePath),
		IsExternal: isExternal,
	}, nil
}

func droidCreateSettingsFile(home string, name string, copyFromActive bool) (SettingsFileInfo, error) {
	if name == "" {
		return SettingsFileInfo{}, errString("File name cannot be empty")
	}
	if strings.EqualFold(name, "global") {
		return SettingsFileInfo{}, errString("Cannot use 'Global' as a custom file name")
	}

	dir := droidSettingsFilesDir(home)
	if !core.PathExists(dir) {
		if err := os.MkdirAll(dir, 0o755); err != nil { // #nosec G301 -- config directory
			return SettingsFileInfo{}, fmt.Errorf("Failed to create droid-settings directory: %w", err)
		}
	}
	path := core.WithExtension(filepath.Join(dir, name), "json")
	if core.PathExists(path) {
		return SettingsFileInfo{}, fmt.Errorf("Settings file '%s' already exists", name)
	}

	if copyFromActive {
		activePath, err := droidActiveSettingsPath(home)
		if err != nil {
			return SettingsFileInfo{}, err
		}
		if core.PathExists(activePath) {
			data, err := os.ReadFile(activePath) // #nosec G304 -- active settings path
			if err != nil {
				return SettingsFileInfo{}, fmt.Errorf("Failed to copy settings: %w", err)
			}
			if err := os.WriteFile(path, data, 0o644); err != nil { // #nosec G306 -- settings files
				return SettingsFileInfo{}, fmt.Errorf("Failed to copy settings: %w", err)
			}
		} else if err := os.WriteFile(path, []byte("{}"), 0o644); err != nil { // #nosec G306 -- settings files
			return SettingsFileInfo{}, fmt.Errorf("Failed to create settings file: %w", err)
		}
	} else if err := os.WriteFile(path, []byte("{}"), 0o644); err != nil { // #nosec G306 -- settings files
		return SettingsFileInfo{}, fmt.Errorf("Failed to create settings file: %w", err)
	}

	// Auto-switch to the new file.
	if err := droidSaveActiveFileName(home, name); err != nil {
		return SettingsFileInfo{}, err
	}
	return droidGetActiveSettingsFile(home)
}

func droidDeleteSettingsFile(home string, name string) error {
	if strings.EqualFold(name, "global") {
		return errString("Cannot delete the global settings file")
	}
	path := core.WithExtension(filepath.Join(droidSettingsFilesDir(home), name), "json")
	if !core.PathExists(path) {
		return fmt.Errorf("Settings file '%s' does not exist", name)
	}
	if err := os.Remove(path); err != nil {
		return fmt.Errorf("Failed to delete settings file: %w", err)
	}
	// If the deleted file was active, switch back to Global (Rust swallows
	// load errors here: load_active_file_name().unwrap_or(None)).
	if activeName, err := droidLoadActiveFileName(home); err == nil && activeName != "" && activeName == name {
		return droidSaveActiveFileName(home, "")
	}
	return nil
}

func droidLinkSettingsFile(home string, sourcePath string) (SettingsFileInfo, error) {
	resolved, err := droidResolvePath(sourcePath)
	if err != nil {
		return SettingsFileInfo{}, fmt.Errorf("Settings file '%s' does not exist or cannot be read: %w", sourcePath, err)
	}
	if droidExtension(droidFileName(resolved)) != "json" {
		return SettingsFileInfo{}, errString("Settings file must have a .json extension")
	}
	data, err := os.ReadFile(resolved) // #nosec G304 -- user-picked settings file
	if err != nil {
		return SettingsFileInfo{}, fmt.Errorf("Failed to read settings file '%s': %w", resolved, err)
	}
	if _, err := core.DecodeJSONValue(data); err != nil {
		return SettingsFileInfo{}, fmt.Errorf("Settings file '%s' is not valid JSON: %w", resolved, err)
	}

	source := resolved
	files, err := droidLoadExternalFiles(home)
	if err != nil {
		return SettingsFileInfo{}, err
	}
	// Re-linking the same file just activates it (exact stored-path match,
	// like the Rust String comparison).
	for _, file := range files {
		if file.Path == source {
			if err := droidSaveActiveFileName(home, source); err != nil {
				return SettingsFileInfo{}, err
			}
			return droidGetActiveSettingsFile(home)
		}
	}

	stem := droidFileStem(droidFileName(source))
	if stem == "" {
		stem = "settings"
	}
	name, err := droidPickLinkDisplayName(home, stem)
	if err != nil {
		return SettingsFileInfo{}, err
	}
	files = append(files, droidExternalFile{Name: name, Path: source})
	if err := droidSaveExternalFiles(home, files); err != nil {
		return SettingsFileInfo{}, err
	}
	if err := droidSaveActiveFileName(home, source); err != nil {
		return SettingsFileInfo{}, err
	}
	return droidGetActiveSettingsFile(home)
}

func droidUnlinkSettingsFile(home string, path string) error {
	files, err := droidLoadExternalFiles(home)
	if err != nil {
		return err
	}
	canonical, canonicalErr := droidResolvePath(path)
	index := -1
	for i, file := range files {
		if droidPathsEqual(file.Path, path) ||
			(canonicalErr == nil && droidPathsEqual(file.Path, canonical)) {
			index = i
			break
		}
	}
	if index < 0 {
		return fmt.Errorf("Settings file '%s' is not linked", path)
	}
	removed := files[index]
	files = append(files[:index], files[index+1:]...)
	if err := droidSaveExternalFiles(home, files); err != nil {
		return err
	}
	activeName, err := droidLoadActiveFileName(home)
	if err != nil {
		return err
	}
	if activeName == removed.Path {
		return droidSaveActiveFileName(home, "")
	}
	return nil
}

// ---------------------------------------------------------------------------
// Settings values (active settings file; mirrors factory_settings get/save_*)
// ---------------------------------------------------------------------------

type droidConfigStatus int

const (
	droidConfigOK droidConfigStatus = iota
	droidConfigNotFound
	droidConfigParseError
)

// droidReadActiveConfig mirrors factory_settings::read_active_config_file:
// every path failure degrades to NotFound, content failures carry the inner
// message used by the CONFIG_PARSE_ERROR prefix.
func droidReadActiveConfig(home string) (any, droidConfigStatus, error) {
	activePath, err := droidActiveSettingsPath(home)
	if err != nil {
		return nil, droidConfigNotFound, nil
	}
	if !core.PathExists(activePath) {
		return nil, droidConfigNotFound, nil
	}
	data, err := os.ReadFile(activePath) // #nosec G304 -- active settings path
	if err != nil {
		return nil, droidConfigParseError, fmt.Errorf("Failed to read config file: %w", err)
	}
	if strings.TrimSpace(string(data)) == "" {
		return nil, droidConfigNotFound, nil
	}
	value, err := core.DecodeJSONValue(data)
	if err != nil {
		return nil, droidConfigParseError, fmt.Errorf("Failed to parse config JSON: %w", err)
	}
	return value, droidConfigOK, nil
}

// droidWriteActiveConfig mirrors factory_settings::write_active_config_file
// (parent dir, symlink resolution, settings.tmp + rename).
func droidWriteActiveConfig(home string, config any) error {
	activePath, err := droidActiveSettingsPath(home)
	if err != nil {
		return err
	}
	parent := filepath.Dir(activePath)
	if !core.PathExists(parent) {
		if err := os.MkdirAll(parent, 0o755); err != nil { // #nosec G301 -- config directory
			return fmt.Errorf("Failed to create directory: %w", err)
		}
	}
	actualPath := activePath
	if core.IsSymlink(activePath) {
		resolved, err := filepath.EvalSymlinks(activePath)
		if err != nil {
			return fmt.Errorf("Failed to resolve symlink: %w", err)
		}
		actualPath = resolved
	}
	data, err := core.MarshalPretty(config)
	if err != nil {
		return fmt.Errorf("Failed to serialize config: %w", err)
	}
	tempPath := core.WithExtension(actualPath, "tmp")           // settings.json → settings.tmp
	if err := os.WriteFile(tempPath, data, 0o644); err != nil { // #nosec G306 -- config files
		return fmt.Errorf("Failed to write config file: %w", err)
	}
	if err := os.Rename(tempPath, actualPath); err != nil {
		_ = os.Remove(tempPath)
		return fmt.Errorf("Failed to finalize config file: %w", err)
	}
	return nil
}

// droidActiveConfigForGetter resolves the config for read-only getters; Rust
// maps path failures to NotFound so getters return their defaults.
func droidActiveConfigForGetter() (any, droidConfigStatus) {
	home, err := droidHomeDir()
	if err != nil {
		return nil, droidConfigNotFound
	}
	value, status, _ := droidReadActiveConfig(home)
	return value, status
}

// droidSaveActiveSetting upserts one top-level key in the active settings
// file. A parse failure surfaces as the CONFIG_PARSE_ERROR-prefixed string the
// frontend matches on (model-store.ts).
func droidSaveActiveSetting(key string, value any) error {
	home, err := droidHomeDir()
	if err != nil {
		return err
	}
	config, status, err := droidReadActiveConfig(home)
	switch status {
	case droidConfigParseError:
		return fmt.Errorf("CONFIG_PARSE_ERROR: %w", err)
	case droidConfigNotFound:
		config = map[string]any{}
	}
	if obj, ok := config.(map[string]any); ok {
		obj[key] = value
	}
	return droidWriteActiveConfig(home, config)
}

// droidConfigObject narrows a getter config to its object view.
func droidConfigObject(config any, status droidConfigStatus) (map[string]any, bool) {
	if status != droidConfigOK {
		return nil, false
	}
	obj, ok := config.(map[string]any)
	return obj, ok
}

// GetCloudSessionSync reads cloudSessionSync (default true).
func GetCloudSessionSync() (bool, error) {
	config, status := droidActiveConfigForGetter()
	obj, ok := droidConfigObject(config, status)
	if !ok {
		return true, nil
	}
	if enabled, isBool := obj["cloudSessionSync"].(bool); isBool {
		return enabled, nil
	}
	return true, nil
}

// SaveCloudSessionSync stores cloudSessionSync.
func SaveCloudSessionSync(enabled bool) error {
	return droidSaveActiveSetting("cloudSessionSync", enabled)
}

// GetReasoningEffort reads reasoningEffort, falling back to
// sessionDefaultSettings.reasoningEffort when the top level is absent.
func GetReasoningEffort() (*string, error) {
	config, status := droidActiveConfigForGetter()
	obj, ok := droidConfigObject(config, status)
	if !ok {
		return nil, nil
	}
	if value, isString := obj["reasoningEffort"].(string); isString {
		return &value, nil
	}
	if raw, present := obj["sessionDefaultSettings"]; present {
		data, err := json.Marshal(raw)
		if err == nil {
			var settings factory.SessionDefaultSettings
			if err := json.Unmarshal(data, &settings); err == nil && settings.ReasoningEffort != nil {
				return settings.ReasoningEffort, nil
			}
		}
	}
	return nil, nil
}

// SaveReasoningEffort stores reasoningEffort.
func SaveReasoningEffort(value string) error {
	return droidSaveActiveSetting("reasoningEffort", value)
}

// GetDiffMode reads diffMode (default "github").
func GetDiffMode() (string, error) {
	config, status := droidActiveConfigForGetter()
	obj, ok := droidConfigObject(config, status)
	if !ok {
		return "github", nil
	}
	if value, isString := obj["diffMode"].(string); isString {
		return value, nil
	}
	return "github", nil
}

// SaveDiffMode stores diffMode.
func SaveDiffMode(value string) error {
	return droidSaveActiveSetting("diffMode", value)
}

// GetTodoDisplayMode reads todoDisplayMode (default "pinned").
func GetTodoDisplayMode() (string, error) {
	config, status := droidActiveConfigForGetter()
	obj, ok := droidConfigObject(config, status)
	if !ok {
		return "pinned", nil
	}
	if value, isString := obj["todoDisplayMode"].(string); isString {
		return value, nil
	}
	return "pinned", nil
}

// SaveTodoDisplayMode stores todoDisplayMode.
func SaveTodoDisplayMode(value string) error {
	return droidSaveActiveSetting("todoDisplayMode", value)
}

// GetIncludeCoAuthoredByDroid reads includeCoAuthoredByDroid (default true).
func GetIncludeCoAuthoredByDroid() (bool, error) {
	config, status := droidActiveConfigForGetter()
	obj, ok := droidConfigObject(config, status)
	if !ok {
		return true, nil
	}
	if enabled, isBool := obj["includeCoAuthoredByDroid"].(bool); isBool {
		return enabled, nil
	}
	return true, nil
}

// SaveIncludeCoAuthoredByDroid stores includeCoAuthoredByDroid.
func SaveIncludeCoAuthoredByDroid(enabled bool) error {
	return droidSaveActiveSetting("includeCoAuthoredByDroid", enabled)
}

// GetShowThinkingInMainView reads showThinkingInMainView (default false).
func GetShowThinkingInMainView() (bool, error) {
	config, status := droidActiveConfigForGetter()
	obj, ok := droidConfigObject(config, status)
	if !ok {
		return false, nil
	}
	if enabled, isBool := obj["showThinkingInMainView"].(bool); isBool {
		return enabled, nil
	}
	return false, nil
}

// SaveShowThinkingInMainView stores showThinkingInMainView.
func SaveShowThinkingInMainView(enabled bool) error {
	return droidSaveActiveSetting("showThinkingInMainView", enabled)
}

// GetMissionModelSettings reads missionModelSettings (empty when unset).
func GetMissionModelSettings() (MissionModelSettings, error) {
	config, status := droidActiveConfigForGetter()
	obj, ok := droidConfigObject(config, status)
	if !ok {
		return MissionModelSettings{}, nil
	}
	raw, present := obj["missionModelSettings"]
	if !present {
		return MissionModelSettings{}, nil
	}
	data, err := json.Marshal(raw)
	if err != nil {
		return MissionModelSettings{}, nil
	}
	var settings MissionModelSettings
	if err := json.Unmarshal(data, &settings); err != nil {
		return MissionModelSettings{}, nil
	}
	return settings, nil
}

// SaveMissionModelSettings stores missionModelSettings (whole-value replace,
// like the Rust command).
func SaveMissionModelSettings(settings MissionModelSettings) error {
	value, err := core.ToMap(settings)
	if err != nil {
		return fmt.Errorf("Failed to serialize mission model settings: %w", err)
	}
	return droidSaveActiveSetting("missionModelSettings", value)
}

// GetCompactionModelMode reads compactionModelMode (default "current-model").
func GetCompactionModelMode() (string, error) {
	config, status := droidActiveConfigForGetter()
	obj, ok := droidConfigObject(config, status)
	if !ok {
		return "current-model", nil
	}
	if value, isString := obj["compactionModelMode"].(string); isString {
		return value, nil
	}
	return "current-model", nil
}

// SaveCompactionModelMode stores compactionModelMode.
func SaveCompactionModelMode(value string) error {
	return droidSaveActiveSetting("compactionModelMode", value)
}

// GetCompactionTokenLimit reads compactionTokenLimit (default 200000; only
// integer JSON literals count, matching serde's as_i64).
func GetCompactionTokenLimit() (int32, error) {
	config, status := droidActiveConfigForGetter()
	obj, ok := droidConfigObject(config, status)
	if !ok {
		return 200000, nil
	}
	number, isNumber := obj["compactionTokenLimit"].(json.Number)
	if !isNumber {
		return 200000, nil
	}
	value, err := number.Int64()
	if err != nil {
		return 200000, nil
	}
	return int32(value), nil // Rust `i64 as i32` truncates the same way
}

// SaveCompactionTokenLimit stores compactionTokenLimit.
func SaveCompactionTokenLimit(value int32) error {
	return droidSaveActiveSetting("compactionTokenLimit", value)
}

// GetCompactionTokenLimitPerModel reads compactionTokenLimitPerModel (empty
// map unless the whole value parses as a string → i32 record).
func GetCompactionTokenLimitPerModel() (map[string]int32, error) {
	overrides := map[string]int32{}
	config, status := droidActiveConfigForGetter()
	obj, ok := droidConfigObject(config, status)
	if !ok {
		return overrides, nil
	}
	raw, present := obj["compactionTokenLimitPerModel"]
	if !present {
		return overrides, nil
	}
	data, err := json.Marshal(raw)
	if err != nil {
		return map[string]int32{}, nil
	}
	if err := json.Unmarshal(data, &overrides); err != nil || overrides == nil {
		return map[string]int32{}, nil
	}
	return overrides, nil
}

// SaveCompactionTokenLimitPerModel stores compactionTokenLimitPerModel.
func SaveCompactionTokenLimitPerModel(overrides map[string]int32) error {
	if overrides == nil {
		overrides = map[string]int32{} // Rust to_value(HashMap) never yields null
	}
	return droidSaveActiveSetting("compactionTokenLimitPerModel", overrides)
}

// ---------------------------------------------------------------------------
// Launch plan (mirrors droidgear-core::droid_runtime)
// ---------------------------------------------------------------------------

type droidRunPlan struct {
	program      string
	args         []string
	env          [][2]string
	unsetEnv     []string
	settingsPath *string
}

func droidBuildRunPlanFromSettingsPath(home string, settingsPath string, prefs DroidRunPreferences) droidRunPlan {
	disableAutoUpdate := prefs.DisableAutoUpdateEnv == nil || *prefs.DisableAutoUpdateEnv
	unsetAnthropicToken := prefs.UnsetAnthropicAuthToken == nil || *prefs.UnsetAnthropicAuthToken

	plan := droidRunPlan{program: "droid"}
	if disableAutoUpdate {
		plan.env = append(plan.env, [2]string{"FACTORY_DROID_AUTO_UPDATE_ENABLED", "0"})
	}
	if unsetAnthropicToken {
		plan.unsetEnv = append(plan.unsetEnv, "ANTHROPIC_AUTH_TOKEN")
	}
	if !droidPathsEqual(settingsPath, droidGlobalSettingsPath(home)) {
		plan.args = []string{"--settings", settingsPath}
		path := settingsPath
		plan.settingsPath = &path
	}
	return plan
}

func droidBuildRunPlanForHome(home string, prefs DroidRunPreferences) (droidRunPlan, error) {
	settingsPath, err := droidActiveSettingsPath(home)
	if err != nil {
		return droidRunPlan{}, err
	}
	return droidBuildRunPlanFromSettingsPath(home, settingsPath, prefs), nil
}

// GetLaunchCommand returns [command, settingsPath] for the active file
// (plain `droid` for Global, `droid --settings "<path>"` otherwise).
func GetLaunchCommand() ([2]string, error) {
	home, err := droidHomeDir()
	if err != nil {
		return [2]string{}, err
	}
	activePath, err := droidActiveSettingsPath(home)
	if err != nil {
		return [2]string{}, err
	}
	command := "droid"
	if !droidPathsEqual(activePath, droidGlobalSettingsPath(home)) {
		command = fmt.Sprintf("droid --settings \"%s\"", activePath)
	}
	return [2]string{command, activePath}, nil
}

// Launch opens the active Droid profile in an external terminal at cwd.
//
// runPrefs and preferredTerminal come from preferences.json (the shell's
// business, mirroring commands/droid_settings.rs); the run plan itself is
// built here so the GUI forward, `dg` and the TUI share one path.
func Launch(cwd *string, runPrefs DroidRunPreferences, preferredTerminal string) error {
	home, err := droidHomeDir()
	if err != nil {
		return err
	}
	plan, err := droidBuildRunPlanForHome(home, runPrefs)
	if err != nil {
		return err
	}
	spec := droidLaunchSpec{
		program:  plan.program,
		args:     plan.args,
		env:      plan.env,
		unsetEnv: plan.unsetEnv,
	}
	spec.cwd = cwd // Rust: cwd.map(PathBuf::from)
	return droidLaunchInTerminal(spec, preferredTerminal)
}

// ---------------------------------------------------------------------------
// External terminal launcher (terminal_launch.rs, droid spec shape only:
// secret_env and support_dir are always empty, no_keep_open always false)
// ---------------------------------------------------------------------------

type droidLaunchSpec struct {
	program  string
	args     []string
	env      [][2]string
	unsetEnv []string
	cwd      *string
}

type droidPreparedCommand struct {
	command         string
	keepOpenCommand string
}

// --- quoting / rendering ---------------------------------------------------

func droidQuotePosix(value string) string {
	return "'" + strings.ReplaceAll(value, "'", `'\''`) + "'"
}

func droidQuotePowershell(value string) string {
	return "'" + strings.ReplaceAll(value, "'", "''") + "'"
}

func droidEscapeCmdValue(value string) string {
	value = strings.ReplaceAll(value, "^", "^^")
	value = strings.ReplaceAll(value, `"`, `^"`)
	value = strings.ReplaceAll(value, "%", "%%")
	return value
}

func droidQuoteCmd(value string) string {
	return "\"" + droidEscapeCmdValue(value) + "\""
}

func droidStartupMessage(program string) string {
	name := droidFileName(program)
	if name == "" {
		name = program
	}
	return "Starting " + name + "..."
}

func droidRenderPowershellBanner(program string) string {
	return "Write-Host " + droidQuotePowershell(droidStartupMessage(program))
}

func droidRenderPosixBanner(program string) string {
	return "printf '%s\\n' " + droidQuotePosix(droidStartupMessage(program))
}

func droidRenderPosixEnv(spec droidLaunchSpec) string {
	parts := []string{}
	for _, key := range spec.unsetEnv {
		parts = append(parts, "unset "+key)
	}
	for _, pair := range spec.env {
		parts = append(parts, "export "+pair[0]+"="+droidQuotePosix(pair[1]))
	}
	return strings.Join(parts, "; ")
}

func droidRenderPowershellEnv(spec droidLaunchSpec) string {
	parts := []string{}
	for _, key := range spec.unsetEnv {
		parts = append(parts, "Remove-Item Env:"+key+" -ErrorAction SilentlyContinue")
	}
	for _, pair := range spec.env {
		parts = append(parts, "$env:"+pair[0]+" = "+droidQuotePowershell(pair[1]))
	}
	return strings.Join(parts, "; ")
}

func droidRenderCmdEnv(spec droidLaunchSpec) string {
	parts := []string{}
	for _, key := range spec.unsetEnv {
		parts = append(parts, "set "+key+"=")
	}
	for _, pair := range spec.env {
		parts = append(parts, "set \""+pair[0]+"="+droidEscapeCmdValue(pair[1])+"\"")
	}
	return strings.Join(parts, " && ")
}

func droidRenderProgramCommand(spec droidLaunchSpec, quote func(string) string) string {
	parts := make([]string, 0, len(spec.args)+1)
	parts = append(parts, quote(spec.program))
	for _, arg := range spec.args {
		parts = append(parts, quote(arg))
	}
	return strings.Join(parts, " ")
}

// droidJoinNonEmpty joins non-empty parts (Rust join_non_empty).
func droidJoinNonEmpty(parts []string, separator string) string {
	kept := make([]string, 0, len(parts))
	for _, part := range parts {
		if part != "" {
			kept = append(kept, part)
		}
	}
	return strings.Join(kept, separator)
}

func droidRenderPosixCD(cwd *string) string {
	if cwd == nil {
		return ""
	}
	return "cd " + droidQuotePosix(*cwd)
}

func droidRenderPowershellCD(cwd *string) string {
	if cwd == nil {
		return ""
	}
	return "Set-Location " + droidQuotePowershell(*cwd)
}

func droidRenderCmdCD(cwd *string) string {
	if cwd == nil {
		return ""
	}
	return "cd /d " + droidQuoteCmd(*cwd)
}

func droidRenderPowershellPathRefresh() string {
	return "$machinePath = [Environment]::GetEnvironmentVariable('Path','Machine'); $userPath = [Environment]::GetEnvironmentVariable('Path','User'); if ($machinePath -or $userPath) { $env:Path = @($machinePath, $userPath, $env:Path | Where-Object { $_ }) -join ';' }"
}

// --- command preparation ---------------------------------------------------

func droidPreparePosixCommand(spec droidLaunchSpec) droidPreparedCommand {
	envSetup := droidRenderPosixEnv(spec)
	cd := droidRenderPosixCD(spec.cwd)
	command := droidRenderProgramCommand(spec, droidQuotePosix)
	base := droidJoinNonEmpty([]string{envSetup, cd, command}, "; ")
	return droidPreparedCommand{command: base, keepOpenCommand: base + "; exec bash"}
}

func droidPreparePowershellCommand(spec droidLaunchSpec) droidPreparedCommand {
	pathRefresh := droidRenderPowershellPathRefresh()
	envSetup := droidRenderPowershellEnv(spec)
	cd := droidRenderPowershellCD(spec.cwd)
	command := "& " + droidRenderProgramCommand(spec, droidQuotePowershell)
	banner := droidRenderPowershellBanner(spec.program)
	base := droidJoinNonEmpty([]string{pathRefresh, envSetup, cd, command}, "; ")
	return droidPreparedCommand{
		command:         base,
		keepOpenCommand: droidJoinNonEmpty([]string{banner, base}, "; "),
	}
}

func droidPrepareCmdCommand(spec droidLaunchSpec) droidPreparedCommand {
	envSetup := droidRenderCmdEnv(spec)
	cd := droidRenderCmdCD(spec.cwd)
	command := droidRenderProgramCommand(spec, droidQuoteCmd)
	base := droidJoinNonEmpty([]string{envSetup, cd, command}, " && ")
	return droidPreparedCommand{command: base, keepOpenCommand: base}
}

// --- launch scripts --------------------------------------------------------

func droidLaunchArtifactPath(filename string) string {
	timestamp := time.Now().UTC().Format("20060102T150405.000") + "Z"
	counter := droidTempCounter.Add(1)
	return filepath.Join(os.TempDir(), fmt.Sprintf("%s-%d-%d-%s", timestamp, os.Getpid(), counter, filename))
}

func droidWriteLaunchScript(path string, contents string) error {
	if parent := filepath.Dir(path); parent != "" && !core.PathExists(parent) {
		if err := os.MkdirAll(parent, 0o755); err != nil { // #nosec G301 -- temp directory
			return fmt.Errorf("Failed to create launch script directory: %w", err)
		}
	}
	if err := os.WriteFile(path, []byte(contents), 0o644); err != nil { // #nosec G306 -- temp scripts
		return fmt.Errorf("Failed to write launch script: %w", err)
	}
	if runtime.GOOS != "windows" {
		if err := os.Chmod(path, 0o700); err != nil {
			return fmt.Errorf("Failed to set launch script permissions: %w", err)
		}
	}
	return nil
}

func droidWriteSecureCmdWrapper(spec droidLaunchSpec) (string, error) {
	path := droidLaunchArtifactPath("terminal-launch.cmd")
	lines := []string{
		"@echo off",
		"setlocal",
		"cls",
		"echo " + droidStartupMessage(spec.program),
	}
	for _, key := range spec.unsetEnv {
		lines = append(lines, "set "+key+"=")
	}
	for _, pair := range spec.env {
		lines = append(lines, fmt.Sprintf("set \"%s=%s\"", pair[0], droidEscapeCmdValue(pair[1])))
	}
	if cd := droidRenderCmdCD(spec.cwd); cd != "" {
		lines = append(lines, cd)
	}
	lines = append(lines, droidRenderProgramCommand(spec, droidQuoteCmd))
	lines = append(lines, "set \"DROIDGEAR_EXIT_CODE=%ERRORLEVEL%\"")
	lines = append(lines, "start \"\" /b cmd /c del /f /q \"%~f0\" >nul 2>nul")
	lines = append(lines, "endlocal & exit /b %DROIDGEAR_EXIT_CODE%")
	if err := droidWriteLaunchScript(path, strings.Join(lines, "\r\n")+"\r\n"); err != nil {
		return "", err
	}
	return path, nil
}

func droidWriteSecurePowershellWrapper(spec droidLaunchSpec) (string, error) {
	path := droidLaunchArtifactPath("terminal-launch.ps1")
	lines := []string{
		"$ErrorActionPreference = 'Continue'",
		"try {",
		"  " + droidRenderPowershellPathRefresh(),
		"  Clear-Host",
		"  " + droidRenderPowershellBanner(spec.program),
	}
	for _, key := range spec.unsetEnv {
		lines = append(lines, "  Remove-Item Env:"+key+" -ErrorAction SilentlyContinue")
	}
	for _, pair := range spec.env {
		lines = append(lines, "  $env:"+pair[0]+" = "+droidQuotePowershell(pair[1]))
	}
	if cd := droidRenderPowershellCD(spec.cwd); cd != "" {
		lines = append(lines, "  "+cd)
	}
	lines = append(lines, "  & "+droidRenderProgramCommand(spec, droidQuotePowershell))
	lines = append(lines, "} finally {")
	lines = append(lines, "  Remove-Item -LiteralPath $PSCommandPath -Force -ErrorAction SilentlyContinue")
	lines = append(lines, "}")
	if err := droidWriteLaunchScript(path, strings.Join(lines, "\r\n")+"\r\n"); err != nil {
		return "", err
	}
	return path, nil
}

// --- Windows ----------------------------------------------------------------

// droidNeedsWindowsWrapper reports when inline multi-statement PowerShell/cmd
// is unsafe through the terminal's argv re-parse (secrets never occur in the
// droid spec, so only env/unset/cwd matter here).
func droidNeedsWindowsWrapper(spec droidLaunchSpec) bool {
	return len(spec.env) > 0 || len(spec.unsetEnv) > 0 || spec.cwd != nil
}

func droidWindowsCmdStartArgs(payload string, cwd *string) []string {
	args := []string{"/c", "start", ""}
	if cwd != nil {
		args = append(args, "/D", *cwd)
	}
	args = append(args, "cmd", "/k", payload)
	return args
}

func droidWindowsWtPSArgs(shell string, command string, cwd *string, noKeepOpen bool) []string {
	args := []string{}
	if cwd != nil {
		args = append(args, "-d", *cwd)
	}
	args = append(args, shell)
	if !noKeepOpen {
		args = append(args, "-NoExit")
	}
	args = append(args, "-Command", command)
	return args
}

func droidWindowsPowershellArgs(command string, cwd *string, noKeepOpen bool) []string {
	args := []string{}
	if cwd != nil {
		args = append(args, "-WorkingDirectory", *cwd)
	}
	if !noKeepOpen {
		args = append(args, "-NoExit")
	}
	args = append(args, "-Command", command)
	return args
}

func droidIsRegularFile(path string) bool {
	info, err := os.Stat(path)
	return err == nil && info.Mode().IsRegular()
}

func droidWindowsProgramCandidates(program string) []string {
	lower := strings.ToLower(program)
	for _, suffix := range []string{".exe", ".cmd", ".bat", ".ps1", ".com"} {
		if strings.HasSuffix(lower, suffix) {
			return []string{program}
		}
	}
	return []string{program + ".exe", program + ".cmd", program + ".bat", program + ".ps1"}
}

func droidSearchProgramInPathValue(pathValue string, candidates []string) (string, bool) {
	for _, part := range strings.Split(pathValue, ";") {
		part = strings.TrimSpace(part)
		if part == "" {
			continue
		}
		for _, name := range candidates {
			candidate := filepath.Join(part, name)
			if droidIsRegularFile(candidate) {
				return candidate, true
			}
		}
	}
	return "", false
}

// droidWindowsEnvironmentPath probes the registry-backed Machine/User PATH
// (GUI apps often miss user installs), hidden like the Rust helper.
func droidWindowsEnvironmentPath(scope string) string {
	cmd := exec.Command("powershell", "-NoLogo", "-NoProfile", "-Command",
		"[Environment]::GetEnvironmentVariable('Path','"+scope+"')") // #nosec G204 -- fixed scope from a two-item list
	core.HideConsoleWindow(cmd)
	output, err := cmd.Output()
	if err != nil {
		return ""
	}
	return strings.TrimSpace(string(output))
}

// droidResolveWindowsProgram resolves a bare program against process PATH
// first, then the registry Machine/User PATH.
func droidResolveWindowsProgram(program string) string {
	if filepath.IsAbs(program) || strings.ContainsAny(program, `\/`) {
		return program
	}
	candidates := droidWindowsProgramCandidates(program)
	if processPath := os.Getenv("PATH"); processPath != "" {
		if found, ok := droidSearchProgramInPathValue(processPath, candidates); ok {
			return found
		}
	}
	for _, scope := range []string{"Machine", "User"} {
		if value := droidWindowsEnvironmentPath(scope); value != "" {
			if found, ok := droidSearchProgramInPathValue(value, candidates); ok {
				return found
			}
		}
	}
	return program
}

func droidFindWindowsShellExecutable(name string) (string, bool) {
	if resolved := droidResolveWindowsProgram(name); droidIsRegularFile(resolved) {
		return resolved, true
	}
	if strings.EqualFold(name, "pwsh") {
		var candidates []string
		if programFiles := os.Getenv("ProgramFiles"); programFiles != "" {
			candidates = append(candidates,
				filepath.Join(programFiles, "PowerShell", "7", "pwsh.exe"),
				filepath.Join(programFiles, "PowerShell", "7-preview", "pwsh.exe"))
		}
		if localAppData := os.Getenv("LOCALAPPDATA"); localAppData != "" {
			candidates = append(candidates,
				filepath.Join(localAppData, "Microsoft", "powershell", "pwsh.exe"),
				filepath.Join(localAppData, "Microsoft", "WindowsApps", "pwsh.exe"))
		}
		for _, candidate := range candidates {
			if core.PathExists(candidate) { // app execution aliases are reparse points
				return candidate, true
			}
		}
	}
	return "", false
}

// droidResolveWindowsPowershellShell prefers PowerShell 7+ when installed.
func droidResolveWindowsPowershellShell() string {
	if path, ok := droidFindWindowsShellExecutable("pwsh"); ok {
		return path
	}
	if path, ok := droidFindWindowsShellExecutable("powershell"); ok {
		return path
	}
	return "powershell"
}

func droidLaunchWindowsCmdPayload(payload string, cwd *string) error {
	cmd := exec.Command("cmd", droidWindowsCmdStartArgs(payload, cwd)...) // #nosec G204 -- launch payload
	core.HideConsoleWindow(cmd)
	if err := cmd.Start(); err != nil {
		return fmt.Errorf("Failed to launch cmd: %w", err)
	}
	return nil
}

func droidLaunchWindowsPowershellPayload(shell string, command string, cwd *string, noKeepOpen bool) error {
	cmd := exec.Command(shell, droidWindowsPowershellArgs(command, cwd, noKeepOpen)...) // #nosec G204 -- launch payload
	core.UseNewConsole(cmd)
	if err := cmd.Start(); err != nil {
		return fmt.Errorf("Failed to launch PowerShell: %w", err)
	}
	return nil
}

func droidLaunchWindowsWtOrPSPayload(shell string, command string, cwd *string, noKeepOpen bool) error {
	if err := exec.Command("wt", droidWindowsWtPSArgs(shell, command, cwd, noKeepOpen)...).Start(); err == nil { // #nosec G204 -- launch payload
		return nil
	}
	return droidLaunchWindowsPowershellPayload(shell, command, cwd, noKeepOpen)
}

func droidWindowsPowershellLaunchCommand(spec droidLaunchSpec) (string, error) {
	if droidNeedsWindowsWrapper(spec) {
		wrapper, err := droidWriteSecurePowershellWrapper(spec)
		if err != nil {
			return "", err
		}
		return "& " + droidQuotePowershell(wrapper), nil
	}
	return droidPreparePowershellCommand(spec).keepOpenCommand, nil
}

func droidLaunchWindows(spec droidLaunchSpec, preferred string) error {
	resolved := spec
	resolved.program = droidResolveWindowsProgram(resolved.program)
	cwd := resolved.cwd
	shell := droidResolveWindowsPowershellShell()

	switch preferred {
	case "cmd":
		if droidNeedsWindowsWrapper(resolved) {
			wrapper, err := droidWriteSecureCmdWrapper(resolved)
			if err != nil {
				return err
			}
			return droidLaunchWindowsCmdPayload(wrapper, cwd)
		}
		return droidLaunchWindowsCmdPayload(droidPrepareCmdCommand(resolved).keepOpenCommand, cwd)
	case "powershell":
		command, err := droidWindowsPowershellLaunchCommand(resolved)
		if err != nil {
			return err
		}
		return droidLaunchWindowsPowershellPayload(shell, command, cwd, false)
	default:
		// Default: Windows Terminal + PowerShell/pwsh (never bare cmd).
		command, err := droidWindowsPowershellLaunchCommand(resolved)
		if err != nil {
			return err
		}
		return droidLaunchWindowsWtOrPSPayload(shell, command, cwd, false)
	}
}

// --- macOS ------------------------------------------------------------------

func droidAppleScriptEscape(value string) string {
	return strings.NewReplacer(`\`, `\\`, `"`, `\"`).Replace(value)
}

func droidLaunchITerm2(spec droidLaunchSpec, prepared droidPreparedCommand, banner string) error {
	script := fmt.Sprintf(`tell application "iTerm2"
    activate
    delay 0.5
    if (count of windows) = 0 then
        create window with default profile
        delay 0.3
    end if
    tell current window
        create tab with default profile
        tell current session
            write text "clear; %s; %s"
        end tell
    end tell
end tell`, droidAppleScriptEscape(banner), droidAppleScriptEscape(prepared.keepOpenCommand))

	err := exec.Command("osascript", "-e", script).Run() // #nosec G204 -- static AppleScript
	if err == nil {
		return nil
	}
	var exitErr *exec.ExitError
	if !errors.As(err, &exitErr) {
		return fmt.Errorf("Failed to launch iTerm2: %w", err)
	}

	// osascript failed (e.g. iTerm not registered): run a self-deleting script.
	filePath := droidLaunchArtifactPath("terminal-launch.command")
	content := "#!/bin/bash\nrm -f -- \"$0\"\nclear\n" + banner + "\n" + prepared.command + "\nexit\n"
	if err := droidWriteLaunchScript(filePath, content); err != nil {
		return err
	}
	err = exec.Command("open", "-a", "iTerm", filePath).Run() // #nosec G204 -- launch script path
	if err != nil {
		if errors.As(err, &exitErr) {
			return errString("Failed to launch iTerm2")
		}
		return fmt.Errorf("Failed to launch iTerm2: %w", err)
	}
	return nil
}

func droidLaunchGhostty(spec droidLaunchSpec, prepared droidPreparedCommand, banner string) error {
	filePath := droidLaunchArtifactPath("terminal-launch.command")
	content := "#!/bin/bash\nrm -f -- \"$0\"\nclear\n" + banner + "\n" + prepared.keepOpenCommand + "\nexit\n"
	if err := droidWriteLaunchScript(filePath, content); err != nil {
		return err
	}
	err := exec.Command("open", "-a", "Ghostty", filePath).Run() // #nosec G204 -- launch script path
	if err == nil {
		return nil
	}
	var exitErr *exec.ExitError
	if errors.As(err, &exitErr) {
		return errString("Failed to launch Ghostty")
	}
	return fmt.Errorf("Failed to launch Ghostty: %w", err)
}

func droidLaunchTerminalApp(prepared droidPreparedCommand, banner string) error {
	script := fmt.Sprintf(`tell application "Terminal"
    activate
    do script "clear; %s; %s; exit"
end tell`, droidAppleScriptEscape(banner), droidAppleScriptEscape(prepared.command))

	err := exec.Command("osascript", "-e", script).Run() // #nosec G204 -- static AppleScript
	if err == nil {
		return nil
	}
	var exitErr *exec.ExitError
	if errors.As(err, &exitErr) {
		return errString("Failed to open Terminal")
	}
	return fmt.Errorf("Failed to open Terminal: %w", err)
}

func droidLaunchSystemDefaultMacOS(spec droidLaunchSpec, prepared droidPreparedCommand, banner string) error {
	filePath := droidLaunchArtifactPath("terminal-launch.command")
	content := "#!/bin/bash\nrm -f -- \"$0\"\nclear\n" + banner + "\n" + prepared.command + "\nexit\n"
	if err := droidWriteLaunchScript(filePath, content); err != nil {
		return err
	}
	err := exec.Command("open", filePath).Run() // #nosec G204 -- launch script path
	if err == nil {
		return nil
	}
	var exitErr *exec.ExitError
	if errors.As(err, &exitErr) {
		return errString("Failed to open terminal")
	}
	return fmt.Errorf("Failed to open terminal: %w", err)
}

func droidLaunchMacOS(spec droidLaunchSpec, preferred string) error {
	prepared := droidPreparePosixCommand(spec)
	banner := droidRenderPosixBanner(spec.program)
	switch preferred {
	case "iterm2":
		return droidLaunchITerm2(spec, prepared, banner)
	case "ghostty":
		return droidLaunchGhostty(spec, prepared, banner)
	case "terminal":
		return droidLaunchTerminalApp(prepared, banner)
	default:
		return droidLaunchSystemDefaultMacOS(spec, prepared, banner)
	}
}

// --- Linux ------------------------------------------------------------------

type droidTerminalCandidate struct {
	name string
	args []string
}

func droidLinuxTerminalCandidates(preferred string, prepared droidPreparedCommand) []droidTerminalCandidate {
	gnome := droidTerminalCandidate{
		name: "gnome-terminal",
		args: []string{"--tab", "--", "bash", "-c", prepared.keepOpenCommand},
	}
	konsole := droidTerminalCandidate{
		name: "konsole",
		args: []string{"--new-tab", "-e", "bash", "-c", prepared.keepOpenCommand},
	}
	xfce := droidTerminalCandidate{
		name: "xfce4-terminal",
		args: []string{"--tab", "-e", "bash -c " + droidQuotePosix(prepared.keepOpenCommand)},
	}
	xtermEmulator := droidTerminalCandidate{
		name: "x-terminal-emulator",
		args: []string{"-e", "bash", "-c", prepared.keepOpenCommand},
	}
	xterm := droidTerminalCandidate{
		name: "xterm",
		args: []string{"-e", "bash", "-c", prepared.keepOpenCommand},
	}

	switch preferred {
	case "gnome-terminal":
		return []droidTerminalCandidate{gnome}
	case "konsole":
		return []droidTerminalCandidate{konsole}
	case "xfce4-terminal":
		return []droidTerminalCandidate{xfce}
	case "x-terminal-emulator":
		return []droidTerminalCandidate{xtermEmulator}
	default:
		return []droidTerminalCandidate{gnome, konsole, xfce, xtermEmulator, xterm}
	}
}

func droidLaunchLinux(spec droidLaunchSpec, preferred string) error {
	prepared := droidPreparePosixCommand(spec)
	lastErr := ""
	for _, candidate := range droidLinuxTerminalCandidates(preferred, prepared) {
		cmd := exec.Command(candidate.name, candidate.args...) // #nosec G204 -- terminal emulator from a fixed list
		if err := cmd.Start(); err == nil {
			return nil
		} else {
			lastErr = fmt.Sprintf("%s: %v", candidate.name, err)
		}
	}
	return fmt.Errorf("Could not find a terminal emulator. Tried: %s", lastErr)
}

// droidLaunchInTerminal dispatches by platform (mirrors launch_in_terminal).
func droidLaunchInTerminal(spec droidLaunchSpec, preferred string) error {
	switch runtime.GOOS {
	case "windows":
		return droidLaunchWindows(spec, preferred)
	case "darwin":
		return droidLaunchMacOS(spec, preferred)
	case "linux":
		return droidLaunchLinux(spec, preferred)
	default:
		return errString("Unsupported platform")
	}
}

// ---------------------------------------------------------------------------
// Commands (src-tauri/src/commands/droid_settings.rs)
// ---------------------------------------------------------------------------

// ListSettingsFiles lists the Global file, managed profiles under
// ~/.droidgear/droid-settings/ and linked external files.
func ListSettingsFiles() ([]SettingsFileInfo, error) {
	home, err := droidHomeDir()
	if err != nil {
		return []SettingsFileInfo{}, err
	}
	files, err := droidListSettingsFiles(home)
	if err != nil {
		return []SettingsFileInfo{}, err
	}
	return files, nil
}

// SetActiveSettingsFile selects the active file; nil/empty switches to
// Global, an absolute path must be a linked external file.
func SetActiveSettingsFile(name *string) (SettingsFileInfo, error) {
	home, err := droidHomeDir()
	if err != nil {
		return SettingsFileInfo{}, err
	}
	activeName := ""
	if name != nil {
		activeName = *name
	}
	return droidSetActiveSettingsFile(home, activeName)
}

// CreateSettingsFile creates a managed profile (optionally copying the
// active file) and switches to it.
func CreateSettingsFile(name string, copyFromActive bool) (SettingsFileInfo, error) {
	home, err := droidHomeDir()
	if err != nil {
		return SettingsFileInfo{}, err
	}
	return droidCreateSettingsFile(home, name, copyFromActive)
}

// DeleteSettingsFile deletes a managed profile (never Global).
func DeleteSettingsFile(name string) error {
	home, err := droidHomeDir()
	if err != nil {
		return err
	}
	return droidDeleteSettingsFile(home, name)
}

// LinkSettingsFile registers a local JSON file by reference and makes it
// active (the file itself is never copied or moved).
func LinkSettingsFile(path string) (SettingsFileInfo, error) {
	home, err := droidHomeDir()
	if err != nil {
		return SettingsFileInfo{}, err
	}
	return droidLinkSettingsFile(home, path)
}

// UnlinkSettingsFile removes a linked file's registration; the file on
// disk is untouched and the active file resets to Global when needed.
func UnlinkSettingsFile(path string) error {
	home, err := droidHomeDir()
	if err != nil {
		return err
	}
	return droidUnlinkSettingsFile(home, path)
}
