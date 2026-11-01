// Claude Code settings-file manager: the Global file (~/.claude/settings.json)
// plus custom files under ~/.droidgear/claude-settings/, the active-file
// pointer in ~/.droidgear/settings.json, the legacy-profile migration, the
// temporary-run preview and the direct terminal launch. Mirrors
// droidgear-core::claude_settings_files (Rust); the launch path materialises
// the runtime copy in-process instead of re-executing the binary as
// `__droidgear_internal claude-settings-launcher` (the Wails main.go has no
// internal-launcher dispatch), so preview and launch share one plan.
package app

import (
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"reflect"
	"runtime"
	"sort"
	"strings"
	"syscall"
	"time"

	"droidgear-wails/internal/core"
)

// ---------------------------------------------------------------------------
// Constants & types
// ---------------------------------------------------------------------------

const (
	claudeSettingsDirName   = "claude-settings"
	claudeActiveFileKey     = "claudeSettingsActiveFile"
	claudeLegacyMigratedKey = "claudeLegacyProfilesMigrated"
)

// ClaudeSettingsFileInfo mirrors claude_settings_files::ClaudeSettingsFileInfo.
type ClaudeSettingsFileInfo struct {
	// Display name (`Global` or the filename without extension).
	Name string `json:"name"`
	// Full path to the settings file.
	Path string `json:"path"`
	// Whether this is the global ~/.claude/settings.json.
	IsGlobal bool `json:"isGlobal"`
	// Whether this file is currently active for editing.
	IsActive bool `json:"isActive"`
	// Whether the file exists on disk.
	Exists bool `json:"exists"`
}

// ClaudeTemporaryRunDebugPreview mirrors
// claude_runtime::ClaudeTemporaryRunDebugPreview.
type ClaudeTemporaryRunDebugPreview struct {
	ProfileID              string      `json:"profileId"`
	ProfileName            string      `json:"profileName"`
	Program                string      `json:"program"`
	Args                   []string    `json:"args"`
	ChildProgram           string      `json:"childProgram"`
	ChildArgs              []string    `json:"childArgs"`
	LiveConfigDir          string      `json:"liveConfigDir"`
	InheritedEnvFileSource *string     `json:"inheritedEnvFileSource,omitempty"`
	Env                    [][2]string `json:"env"`
	UnsetEnv               []string    `json:"unsetEnv"`
	SecretEnvKeys          []string    `json:"secretEnvKeys"`
	Warnings               []string    `json:"warnings"`
	SettingsOverlayJSON    string      `json:"settingsOverlayJson"`
}

// ---------------------------------------------------------------------------
// Path helpers
// ---------------------------------------------------------------------------

// claudeSettingsDir returns ~/.droidgear/claude-settings (no side effects).
func claudeSettingsDir() (string, error) {
	dir, err := core.DroidGearDir()
	if err != nil {
		return "", err
	}
	return filepath.Join(dir, claudeSettingsDirName), nil
}

// claudeGlobalSettingsPath returns <claude config dir>/settings.json without
// creating anything (mirrors global_settings_path_for_home).
func claudeGlobalSettingsPath() (string, error) {
	dir, err := claudeConfigDirNoCreate()
	if err != nil {
		return "", err
	}
	return filepath.Join(dir, "settings.json"), nil
}

// claudeWithJSONExt mirrors PathBuf::with_extension("json"): the last
// extension of the final component is replaced ("a.b" → "a.json") while a
// leading dot stays part of the stem (".hidden" → ".hidden.json").
func claudeWithJSONExt(dir string, name string) string {
	base := filepath.Join(dir, name)
	fileName := filepath.Base(base)
	if dot := strings.LastIndex(fileName, "."); dot > 0 {
		base = base[:len(base)-len(fileName)+dot]
	}
	return base + ".json"
}

// claudeSettingsFileDisplayName returns the display name of a resolved
// settings path (the `Global` label or the file stem).
func claudeSettingsFileDisplayName(path string) string {
	fileName := filepath.Base(path)
	ext := filepath.Ext(fileName)
	return strings.TrimSuffix(fileName, ext)
}

// claudeSettingsPathByName resolves a settings file by display name.
// `Global` (case-insensitive) maps to the global path without an existence
// check; custom names must exist (mirrors get_settings_path_by_name_for_home).
func claudeSettingsPathByName(name string) (string, error) {
	if strings.EqualFold(name, "global") {
		return claudeGlobalSettingsPath()
	}
	dir, err := claudeSettingsDir()
	if err != nil {
		return "", err
	}
	path := claudeWithJSONExt(dir, name)
	if claudePathExists(path) {
		return path, nil
	}
	return "", fmt.Errorf("Settings file '%s' does not exist", name)
}

// claudeActiveSettingsPath resolves the absolute path of the currently active
// settings file, falling back to the global path when the active file is
// missing (mirrors get_active_settings_path_for_home).
func claudeActiveSettingsPath() (string, error) {
	name, err := claudeLoadActiveFileName()
	if err != nil {
		return "", err
	}
	if name != nil && *name != "" {
		dir, err := claudeSettingsDir()
		if err != nil {
			return "", err
		}
		custom := claudeWithJSONExt(dir, *name)
		if claudePathExists(custom) {
			return custom, nil
		}
	}
	return claudeGlobalSettingsPath()
}

// ---------------------------------------------------------------------------
// Active-file pointer & migration flag (~/.droidgear/settings.json)
// ---------------------------------------------------------------------------

// claudeReadDroidGearSettingsValue reads ~/.droidgear/settings.json with a
// missing/empty file yielding an empty object; unknown keys survive because
// the document round-trips through a generic value.
func claudeReadDroidGearSettingsValue() (any, error) {
	path, err := core.DroidGearSettingsPath()
	if err != nil {
		return nil, err
	}
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
	value, err := claudeDecodeJSON(data)
	if err != nil {
		return nil, fmt.Errorf("Failed to parse settings: %w", err)
	}
	return value, nil
}

func claudeWriteDroidGearSettingsValue(value any) error {
	data, err := json.MarshalIndent(value, "", "  ")
	if err != nil {
		return fmt.Errorf("Failed to serialize settings: %w", err)
	}
	path, err := core.DroidGearSettingsPath()
	if err != nil {
		return err
	}
	return core.AtomicWrite(path, data)
}

// claudeMutateDroidGearSettings applies mutate to the settings object and
// writes the document back; a non-object document is written unchanged
// (mirrors `as_object_mut()` being None).
func claudeMutateDroidGearSettings(mutate func(obj map[string]any)) error {
	value, err := claudeReadDroidGearSettingsValue()
	if err != nil {
		return err
	}
	if obj, ok := value.(map[string]any); ok {
		mutate(obj)
	}
	return claudeWriteDroidGearSettingsValue(value)
}

func claudeLoadActiveFileName() (*string, error) {
	value, err := claudeReadDroidGearSettingsValue()
	if err != nil {
		return nil, err
	}
	obj, ok := value.(map[string]any)
	if !ok {
		return nil, nil
	}
	raw, ok := obj[claudeActiveFileKey]
	if !ok {
		return nil, nil
	}
	name, ok := raw.(string)
	if !ok {
		return nil, nil
	}
	return &name, nil
}

// claudeSaveActiveFileName persists the active file name; nil/empty clears it.
func claudeSaveActiveFileName(name *string) error {
	return claudeMutateDroidGearSettings(func(obj map[string]any) {
		if name != nil && *name != "" {
			obj[claudeActiveFileKey] = *name
		} else {
			delete(obj, claudeActiveFileKey)
		}
	})
}

func claudeLegacyAlreadyMigrated() (bool, error) {
	value, err := claudeReadDroidGearSettingsValue()
	if err != nil {
		return false, err
	}
	obj, ok := value.(map[string]any)
	if !ok {
		return false, nil
	}
	migrated, _ := obj[claudeLegacyMigratedKey].(bool)
	return migrated, nil
}

func claudeMarkLegacyMigrated() error {
	return claudeMutateDroidGearSettings(func(obj map[string]any) {
		obj[claudeLegacyMigratedKey] = true
	})
}

// ---------------------------------------------------------------------------
// File-name validation
// ---------------------------------------------------------------------------

func claudeValidateCustomFileName(name string) error {
	trimmed := strings.TrimSpace(name)
	if trimmed == "" {
		return errors.New("File name cannot be empty")
	}
	if strings.EqualFold(trimmed, "global") {
		return errors.New("Cannot use 'Global' as a custom file name")
	}
	if len([]rune(trimmed)) > 100 {
		return errors.New("File name too long (max 100 characters)")
	}
	for _, r := range trimmed {
		switch {
		case r >= 'a' && r <= 'z', r >= 'A' && r <= 'Z', r >= '0' && r <= '9',
			r == '-', r == '_', r == '.':
		default:
			return errors.New("Invalid file name: only letters, digits, dashes, underscores, and dots are allowed")
		}
	}
	if strings.Contains(trimmed, "..") || strings.HasPrefix(trimmed, ".") {
		return errors.New("File name cannot start with '.' or contain '..'")
	}
	return nil
}

// ---------------------------------------------------------------------------
// Legacy DroidGear profile migration
// ---------------------------------------------------------------------------

func claudeLegacyProfilesDir() (string, error) {
	dir, err := claudeDroidGearDir()
	if err != nil {
		return "", err
	}
	return filepath.Join(dir, "profiles"), nil
}

// claudeSlugifyProfileName lowers and sanitises a profile name into a file
// slug (mirrors slugify_profile_name).
func claudeSlugifyProfileName(name string) string {
	var slug strings.Builder
	lastWasDash := false
	for _, r := range name {
		switch {
		case r >= 'a' && r <= 'z', r >= 'A' && r <= 'Z', r >= '0' && r <= '9':
			if r >= 'A' && r <= 'Z' {
				r = r - 'A' + 'a'
			}
			slug.WriteRune(r)
			lastWasDash = false
		case r == '-' || r == '_' || r == '.':
			slug.WriteRune(r)
			lastWasDash = false
		case !lastWasDash:
			slug.WriteByte('-')
			lastWasDash = true
		}
	}
	trimmed := strings.Trim(slug.String(), "-.")
	if trimmed == "" {
		return "profile"
	}
	return trimmed
}

// claudePickAvailableFileName returns base, base-2 … base-999, falling back
// to a uuid suffix (mirrors pick_available_file_name).
func claudePickAvailableFileName(dir string, base string) string {
	if !claudePathExists(claudeWithJSONExt(dir, base)) {
		return base
	}
	for i := 2; i < 1000; i++ {
		next := fmt.Sprintf("%s-%d", base, i)
		if !claudePathExists(claudeWithJSONExt(dir, next)) {
			return next
		}
	}
	return fmt.Sprintf("%s-%s", base, newUUID())
}

// claudeBuildSettingsFromProfile converts a legacy profile document into a
// settings.json document (mirrors build_settings_from_profile).
func claudeBuildSettingsFromProfile(profile ClaudeCodeProfile) map[string]any {
	root := map[string]any{}
	env := map[string]any{}

	if value := claudeNormalizeOptionalString(profile.BaseURL); value != nil {
		env[claudeBaseURLEnv] = *value
	}
	if value := claudeNormalizeOptionalString(profile.BearerToken); value != nil {
		env[claudeAuthTokenEnv] = *value
	}
	if value := claudeNormalizeOptionalString(profile.Model); value != nil {
		env[claudeModelEnv] = *value
	}
	if value := claudeResolvedSmallModel(profile); value != nil {
		env[claudeSmallModelEnv] = *value
	}
	if profile.ReasoningEffort != nil {
		env[claudeEffortEnv] = string(*profile.ReasoningEffort)
		switch *profile.ReasoningEffort {
		case ClaudeReasoningEffortHigh, ClaudeReasoningEffortMax:
			env[claudeDisableAdaptiveEnv] = "1"
		}
	}

	switch profile.ThinkingMode {
	case ClaudeThinkingModeOn:
		root["alwaysThinkingEnabled"] = true
	case ClaudeThinkingModeOff:
		root["alwaysThinkingEnabled"] = false
		env[claudeDisableThinkingEnv] = "1"
	}

	if len(env) > 0 {
		root["env"] = env
	}
	return root
}

// claudeMigrateLegacyProfiles copies ~/.droidgear/claude/profiles/*.json into
// settings files once per machine (mirrors migrate_legacy_profiles_for_home).
func claudeMigrateLegacyProfiles() (int, error) {
	migratedFlag, err := claudeLegacyAlreadyMigrated()
	if err != nil {
		return 0, err
	}
	if migratedFlag {
		return 0, nil
	}

	legacyDir, err := claudeLegacyProfilesDir()
	if err != nil {
		return 0, err
	}
	if !claudePathExists(legacyDir) {
		_ = claudeMarkLegacyMigrated()
		return 0, nil
	}

	targetDir, err := claudeSettingsDir()
	if err != nil {
		return 0, err
	}
	if err := core.EnsureDir(targetDir); err != nil {
		return 0, fmt.Errorf("Failed to create claude-settings directory: %w", err)
	}

	entries, err := os.ReadDir(legacyDir)
	if err != nil {
		_ = claudeMarkLegacyMigrated()
		return 0, nil
	}

	migrated := 0
	for _, entry := range entries {
		if entry.IsDir() || filepath.Ext(entry.Name()) != ".json" {
			continue
		}
		content, err := os.ReadFile(filepath.Join(legacyDir, entry.Name())) // #nosec G304 -- legacy profile path
		if err != nil {
			continue
		}
		var profile ClaudeCodeProfile
		if err := json.Unmarshal(content, &profile); err != nil {
			continue
		}

		chosen := claudePickAvailableFileName(targetDir, claudeSlugifyProfileName(profile.Name))
		targetPath := claudeWithJSONExt(targetDir, chosen)
		payload := claudeBuildSettingsFromProfile(profile)
		data, err := json.MarshalIndent(payload, "", "  ")
		if err != nil {
			return 0, fmt.Errorf("Failed to serialize migrated settings: %w", err)
		}
		if err := core.AtomicWrite(targetPath, data); err == nil {
			migrated++
		}
	}

	_ = claudeMarkLegacyMigrated()
	return migrated, nil
}

// ---------------------------------------------------------------------------
// Commands — settings file management
// ---------------------------------------------------------------------------

// ListClaudeSettingsFiles lists every settings file (global + custom).
func (a *App) ListClaudeSettingsFiles() ([]ClaudeSettingsFileInfo, error) {
	return claudeListSettingsFiles()
}

// GetActiveClaudeSettingsFile returns the currently active settings file info.
func (a *App) GetActiveClaudeSettingsFile() (ClaudeSettingsFileInfo, error) {
	return claudeGetActiveSettingsFile()
}

// claudeListSettingsFiles is the shared implementation behind the commands.
func claudeListSettingsFiles() ([]ClaudeSettingsFileInfo, error) {
	_, _ = claudeMigrateLegacyProfiles()

	activePath, err := claudeActiveSettingsPath()
	if err != nil {
		return nil, err
	}
	globalPath, err := claudeGlobalSettingsPath()
	if err != nil {
		return nil, err
	}

	files := []ClaudeSettingsFileInfo{{
		Name:     "Global",
		Path:     globalPath,
		IsGlobal: true,
		IsActive: activePath == globalPath,
		Exists:   claudePathExists(globalPath),
	}}

	dir, err := claudeSettingsDir()
	if err != nil {
		return nil, err
	}
	if claudePathExists(dir) {
		entries, err := os.ReadDir(dir)
		if err == nil {
			customs := []ClaudeSettingsFileInfo{}
			for _, entry := range entries {
				ext := filepath.Ext(entry.Name())
				if ext != ".json" {
					continue
				}
				path := filepath.Join(dir, entry.Name())
				customs = append(customs, ClaudeSettingsFileInfo{
					Name:     strings.TrimSuffix(entry.Name(), ext),
					Path:     path,
					IsGlobal: false,
					IsActive: activePath == path,
					Exists:   true,
				})
			}
			sort.SliceStable(customs, func(i, j int) bool {
				return strings.ToLower(customs[i].Name) < strings.ToLower(customs[j].Name)
			})
			files = append(files, customs...)
		}
	}

	return files, nil
}

// SetActiveClaudeSettingsFile sets the active settings file. Pass nil or an
// empty string to switch to Global.
func (a *App) SetActiveClaudeSettingsFile(name *string) (ClaudeSettingsFileInfo, error) {
	if name != nil && *name != "" && !strings.EqualFold(*name, "global") {
		dir, err := claudeSettingsDir()
		if err != nil {
			return ClaudeSettingsFileInfo{}, err
		}
		path := claudeWithJSONExt(dir, *name)
		if !claudePathExists(path) {
			return ClaudeSettingsFileInfo{}, fmt.Errorf("Settings file '%s' does not exist", *name)
		}
		if err := claudeSaveActiveFileName(name); err != nil {
			return ClaudeSettingsFileInfo{}, err
		}
	} else {
		if err := claudeSaveActiveFileName(nil); err != nil {
			return ClaudeSettingsFileInfo{}, err
		}
	}

	activePath, err := claudeActiveSettingsPath()
	if err != nil {
		return ClaudeSettingsFileInfo{}, err
	}
	globalPath, err := claudeGlobalSettingsPath()
	if err != nil {
		return ClaudeSettingsFileInfo{}, err
	}
	isGlobal := activePath == globalPath
	info := ClaudeSettingsFileInfo{
		Path:     activePath,
		IsGlobal: isGlobal,
		IsActive: true,
		Exists:   claudePathExists(activePath),
	}
	if isGlobal {
		info.Name = "Global"
	} else {
		info.Name = claudeSettingsFileDisplayName(activePath)
	}
	return info, nil
}

// CreateClaudeSettingsFile creates a new custom settings file and activates it.
func (a *App) CreateClaudeSettingsFile(name string, copyFromActive bool) (ClaudeSettingsFileInfo, error) {
	if err := claudeValidateCustomFileName(name); err != nil {
		return ClaudeSettingsFileInfo{}, err
	}
	dir, err := claudeSettingsDir()
	if err != nil {
		return ClaudeSettingsFileInfo{}, err
	}
	if err := core.EnsureDir(dir); err != nil {
		return ClaudeSettingsFileInfo{}, fmt.Errorf("Failed to create claude-settings directory: %w", err)
	}
	path := claudeWithJSONExt(dir, name)
	if claudePathExists(path) {
		return ClaudeSettingsFileInfo{}, fmt.Errorf("Settings file '%s' already exists", name)
	}

	if copyFromActive {
		activePath, err := claudeActiveSettingsPath()
		if err != nil {
			return ClaudeSettingsFileInfo{}, err
		}
		if claudePathExists(activePath) {
			data, err := os.ReadFile(activePath) // #nosec G304 -- settings file path
			if err != nil {
				return ClaudeSettingsFileInfo{}, fmt.Errorf("Failed to copy settings: %w", err)
			}
			if err := core.AtomicWrite(path, data); err != nil {
				return ClaudeSettingsFileInfo{}, fmt.Errorf("Failed to copy settings: %w", err)
			}
		} else if err := core.AtomicWrite(path, []byte("{}\n")); err != nil {
			return ClaudeSettingsFileInfo{}, err
		}
	} else if err := core.AtomicWrite(path, []byte("{}\n")); err != nil {
		return ClaudeSettingsFileInfo{}, err
	}

	if err := claudeSaveActiveFileName(&name); err != nil {
		return ClaudeSettingsFileInfo{}, err
	}
	return claudeGetActiveSettingsFile()
}

func claudeGetActiveSettingsFile() (ClaudeSettingsFileInfo, error) {
	files, err := claudeListSettingsFiles()
	if err != nil {
		return ClaudeSettingsFileInfo{}, err
	}
	for _, file := range files {
		if file.IsActive {
			return file, nil
		}
	}
	return ClaudeSettingsFileInfo{}, errors.New("No active Claude settings file found")
}

// DeleteClaudeSettingsFile deletes a custom settings file. The Global file
// cannot be deleted.
func (a *App) DeleteClaudeSettingsFile(name string) error {
	if strings.EqualFold(name, "global") {
		return errors.New("Cannot delete the global settings file")
	}
	if err := claudeValidateCustomFileName(name); err != nil {
		return err
	}
	dir, err := claudeSettingsDir()
	if err != nil {
		return err
	}
	path := claudeWithJSONExt(dir, name)
	if !claudePathExists(path) {
		return fmt.Errorf("Settings file '%s' does not exist", name)
	}
	if err := os.Remove(path); err != nil {
		return fmt.Errorf("Failed to delete settings file: %w", err)
	}

	active, err := claudeLoadActiveFileName()
	if err != nil {
		return err
	}
	if active != nil && *active == name {
		if err := claudeSaveActiveFileName(nil); err != nil {
			return err
		}
	}
	return nil
}

// ReadClaudeSettingsFile reads the raw JSON document stored in a settings
// file (by display name); a missing or empty file yields `{}`.
func (a *App) ReadClaudeSettingsFile(name string) (any, error) {
	return claudeReadSettingsFile(name)
}

func claudeReadSettingsFile(name string) (any, error) {
	path, err := claudeSettingsPathByName(name)
	if err != nil {
		return nil, err
	}
	if !claudePathExists(path) {
		return map[string]any{}, nil
	}
	data, err := os.ReadFile(path) // #nosec G304 -- settings file path
	if err != nil {
		return nil, fmt.Errorf("Failed to read settings file: %w", err)
	}
	if strings.TrimSpace(string(data)) == "" {
		return map[string]any{}, nil
	}
	value, err := claudeDecodeJSON(data)
	if err != nil {
		return nil, fmt.Errorf("Failed to parse settings file: %w", err)
	}
	return value, nil
}

// claudeCleanSettingsDocument drops stale top-level fields before persisting:
// a non-empty env.ANTHROPIC_MODEL wins over the top-level `model` key.
func claudeCleanSettingsDocument(root map[string]any) {
	raw, ok := root["env"]
	if !ok {
		return
	}
	env, ok := raw.(map[string]any)
	if !ok {
		return
	}
	model, ok := env[claudeModelEnv].(string)
	if !ok || strings.TrimSpace(model) == "" {
		return
	}
	delete(root, "model")
}

// SaveClaudeSettingsFile persists the given JSON object as the named settings
// file (by display name). The root must be a JSON object.
func (a *App) SaveClaudeSettingsFile(name string, contents any) error {
	return claudeSaveSettingsFile(name, contents)
}

// DuplicateClaudeSettingsFile copies a settings file to a new name and
// activates it.
func (a *App) DuplicateClaudeSettingsFile(name string, newName string) (ClaudeSettingsFileInfo, error) {
	if err := claudeValidateCustomFileName(newName); err != nil {
		return ClaudeSettingsFileInfo{}, err
	}
	sourcePath, err := claudeSettingsPathByName(name)
	if err != nil {
		return ClaudeSettingsFileInfo{}, err
	}
	if !claudePathExists(sourcePath) {
		return ClaudeSettingsFileInfo{}, fmt.Errorf("Settings file '%s' does not exist", name)
	}
	dir, err := claudeSettingsDir()
	if err != nil {
		return ClaudeSettingsFileInfo{}, err
	}
	if err := core.EnsureDir(dir); err != nil {
		return ClaudeSettingsFileInfo{}, fmt.Errorf("Failed to create claude-settings directory: %w", err)
	}
	destPath := claudeWithJSONExt(dir, newName)
	if claudePathExists(destPath) {
		return ClaudeSettingsFileInfo{}, fmt.Errorf("Settings file '%s' already exists", newName)
	}
	data, err := os.ReadFile(sourcePath) // #nosec G304 -- settings file path
	if err != nil {
		return ClaudeSettingsFileInfo{}, fmt.Errorf("Failed to copy settings file: %w", err)
	}
	if err := core.AtomicWrite(destPath, data); err != nil {
		return ClaudeSettingsFileInfo{}, fmt.Errorf("Failed to copy settings file: %w", err)
	}

	if err := claudeSaveActiveFileName(&newName); err != nil {
		return ClaudeSettingsFileInfo{}, err
	}
	return claudeGetActiveSettingsFile()
}

// MergeClaudeSettingsToGlobal merges the contents of a settings file into the
// global settings.json: `env` and `permissions` are shallow-merged, every
// other source key overrides, and global-only keys survive.
func (a *App) MergeClaudeSettingsToGlobal(name string) error {
	source, err := claudeReadSettingsFile(name)
	if err != nil {
		return err
	}
	sourceObj, ok := source.(map[string]any)
	if !ok {
		return errors.New("Settings file root must be a JSON object")
	}

	globalPath, err := claudeGlobalSettingsPath()
	if err != nil {
		return err
	}
	live, err := claudeReadSettingsFile("Global")
	if err != nil {
		return err
	}
	merged := map[string]any{}
	if liveObj, ok := live.(map[string]any); ok {
		merged = liveObj
	}

	for key, value := range sourceObj {
		if key == "env" || key == "permissions" {
			sourceSub, sourceIsObject := value.(map[string]any)
			liveSub, liveIsObject := merged[key].(map[string]any)
			if sourceIsObject && liveIsObject {
				combined := map[string]any{}
				for subKey, subValue := range liveSub {
					combined[subKey] = subValue
				}
				for subKey, subValue := range sourceSub {
					combined[subKey] = subValue
				}
				merged[key] = combined
				continue
			}
		}
		merged[key] = value
	}

	data, err := json.MarshalIndent(merged, "", "  ")
	if err != nil {
		return fmt.Errorf("Failed to serialize merged settings: %w", err)
	}
	if parent := filepath.Dir(globalPath); !claudePathExists(parent) {
		if err := core.EnsureDir(parent); err != nil {
			return fmt.Errorf("Failed to create claude config dir: %w", err)
		}
	}
	return core.AtomicWrite(globalPath, data)
}

// LoadClaudeSettingsFromLive overwrites a settings file with the current
// contents of the global settings.json (normalised through save).
func (a *App) LoadClaudeSettingsFromLive(name string) error {
	live, err := claudeReadSettingsFile("Global")
	if err != nil {
		return err
	}
	return claudeSaveSettingsFile(name, live)
}

// claudeSaveSettingsFile is the shared write path (used by
// LoadClaudeSettingsFromLive) — mirrors save_settings_file_for_home.
func claudeSaveSettingsFile(name string, value any) error {
	root, ok := value.(map[string]any)
	if !ok {
		return errors.New("Settings file root must be a JSON object")
	}
	claudeCleanSettingsDocument(root)

	var path string
	if strings.EqualFold(name, "global") {
		globalPath, err := claudeGlobalSettingsPath()
		if err != nil {
			return err
		}
		if parent := filepath.Dir(globalPath); !claudePathExists(parent) {
			if err := core.EnsureDir(parent); err != nil {
				return fmt.Errorf("Failed to create claude config dir: %w", err)
			}
		}
		path = globalPath
	} else {
		if err := claudeValidateCustomFileName(name); err != nil {
			return err
		}
		dir, err := claudeSettingsDir()
		if err != nil {
			return err
		}
		if err := core.EnsureDir(dir); err != nil {
			return fmt.Errorf("Failed to create claude-settings directory: %w", err)
		}
		path = claudeWithJSONExt(dir, name)
	}

	data, err := json.MarshalIndent(root, "", "  ")
	if err != nil {
		return fmt.Errorf("Failed to serialize settings file: %w", err)
	}
	return core.AtomicWrite(path, data)
}

// ---------------------------------------------------------------------------
// Temporary-run plan, runtime materialisation & terminal launch
// ---------------------------------------------------------------------------

const (
	claudeRuntimeDirName      = "runtime"
	claudeRuntimeSubDir       = "claude"
	claudeRuntimePrefix       = "temporary-run-"
	claudeRuntimeSettingsFile = "claude-settings.json"
	claudeRuntimeEnvFile      = "claude.env"
)

// claudeRuntimeRoot returns ~/.droidgear/runtime/claude — the private home of
// every temporary-run artefact (mirrors runtime_dir_for_home).
func claudeRuntimeRoot() (string, error) {
	dir, err := core.DroidGearDir()
	if err != nil {
		return "", err
	}
	return filepath.Join(dir, claudeRuntimeDirName, claudeRuntimeSubDir), nil
}

// claudeNextRuntimeDirPath builds — but does not create — a fresh
// runtime-private directory: temporary-run-<timestamp>-<uuid>.
func claudeNextRuntimeDirPath() (string, error) {
	root, err := claudeRuntimeRoot()
	if err != nil {
		return "", err
	}
	stamp := time.Now().UTC().Format("20060102T150405.000Z")
	return filepath.Join(root, claudeRuntimePrefix+stamp+"-"+newUUID()), nil
}

// claudeCleanupStaleRuntimeDirs removes temporary-run-* directories older than
// 24 hours (mirrors cleanup_stale_runtime_dirs_for_home). Best effort.
func claudeCleanupStaleRuntimeDirs() {
	root, err := claudeRuntimeRoot()
	if err != nil {
		return
	}
	entries, err := os.ReadDir(root)
	if err != nil {
		return
	}
	cutoff := time.Now().Add(-24 * time.Hour)
	for _, entry := range entries {
		if !strings.HasPrefix(entry.Name(), claudeRuntimePrefix) {
			continue
		}
		info, err := entry.Info()
		if err != nil || info.ModTime().After(cutoff) {
			continue
		}
		_ = os.RemoveAll(filepath.Join(root, entry.Name())) // #nosec G104 -- stale runtime cleanup
	}
}

// claudeConfigDirOverride resolves the CLAUDE_CONFIG_DIR binding: the
// Preferences → Paths override first, then an inherited CLAUDE_CONFIG_DIR.
// An empty result means the live config dir is the plain default (mirrors
// resolve_live_config_dir_binding_for_home_with_env).
func claudeConfigDirOverride() (string, error) {
	path, isDefault, err := core.EffectivePathForKey("claude")
	if err != nil {
		return "", err
	}
	if !isDefault {
		if trimmed := strings.TrimSpace(path); trimmed != "" {
			return trimmed, nil
		}
	}
	if inherited := claudeNormalizeOptionalString(strPtr(os.Getenv("CLAUDE_CONFIG_DIR"))); inherited != nil {
		return *inherited, nil
	}
	return "", nil
}

// claudeCheckInheritedEnvFile returns the inherited CLAUDE_ENV_FILE source,
// pushing a warning when the file cannot be read (mirrors
// check_inherited_env_file_source).
func claudeCheckInheritedEnvFile(warnings *[]string) *string {
	source := claudeNormalizeOptionalString(strPtr(os.Getenv("CLAUDE_ENV_FILE")))
	if source == nil {
		return nil
	}
	if _, err := os.ReadFile(*source); err != nil { // #nosec G304 -- user env file
		detail := err
		if pathErr, ok := err.(*os.PathError); ok {
			detail = pathErr.Err
		}
		*warnings = append(*warnings, fmt.Sprintf(
			"Failed to copy inherited CLAUDE_ENV_FILE from %s: %s. Claude temporary run will continue without inheriting that runtime env file.",
			*source, detail,
		))
	}
	return source
}

// claudeSettingsUnsetEnv mirrors build_settings_unset_env: the ambient
// variables that would clobber the settings file, sorted like a BTreeSet.
func claudeSettingsUnsetEnv() []string {
	keys := map[string]bool{
		"CLAUDE_CONFIG_DIR": true,
		"CLAUDE_ENV_FILE":   true,
		claudeAuthTokenEnv:  true,
		claudeAPIKeyEnv:     true,
	}
	for _, key := range claudeConflictEnvKeys {
		keys[key] = true
	}
	sorted := make([]string, 0, len(keys))
	for key := range keys {
		sorted = append(sorted, key)
	}
	sort.Strings(sorted)
	return sorted
}

// claudeVisibleSettingsEnv mirrors build_visible_env: only an explicit
// CLAUDE_CONFIG_DIR override is exported to the terminal.
func claudeVisibleSettingsEnv(configDirOverride string) [][2]string {
	if configDirOverride == "" {
		return [][2]string{}
	}
	return [][2]string{{"CLAUDE_CONFIG_DIR", configDirOverride}}
}

// claudeSettingsLaunchPlan is the single structure shared by the launch
// command and its preview: the terminal runs `claude --settings <runtime
// copy>` directly — there is no `__droidgear_internal` re-exec in this build
// (see the file header).
type claudeSettingsLaunchPlan struct {
	program          string
	args             []string
	env              [][2]string
	unsetEnv         []string
	warnings         []string
	inheritedEnvFile *string
	runtimeDir       string
}

// claudeBuildSettingsLaunchPlan builds the direct launch plan. skipDangerous
// appends --dangerously-skip-permissions before the --settings pair (mirrors
// build_settings_launch_plan_for_home's extra_child_args).
func claudeBuildSettingsLaunchPlan(skipDangerous bool) (claudeSettingsLaunchPlan, error) {
	runtimeDir, err := claudeNextRuntimeDirPath()
	if err != nil {
		return claudeSettingsLaunchPlan{}, err
	}
	override, err := claudeConfigDirOverride()
	if err != nil {
		return claudeSettingsLaunchPlan{}, err
	}
	warnings := []string{}
	inherited := claudeCheckInheritedEnvFile(&warnings)

	args := []string{}
	if skipDangerous {
		args = append(args, "--dangerously-skip-permissions")
	}
	args = append(args, "--settings", filepath.Join(runtimeDir, claudeRuntimeSettingsFile))

	return claudeSettingsLaunchPlan{
		program:          "claude",
		args:             args,
		env:              claudeVisibleSettingsEnv(override),
		unsetEnv:         claudeSettingsUnsetEnv(),
		warnings:         warnings,
		inheritedEnvFile: inherited,
		runtimeDir:       runtimeDir,
	}, nil
}

// claudeMaterializeSettingsRuntime copies the source settings file — and an
// inherited CLAUDE_ENV_FILE — into the runtime-private directory right before
// the terminal starts. The live configuration is never mutated (mirrors
// materialize_settings_child_launch_from_payload, executed in-process instead
// of by the re-exec'd launcher).
func claudeMaterializeSettingsRuntime(plan *claudeSettingsLaunchPlan, settingsPath string) error {
	if err := core.EnsureDir(plan.runtimeDir); err != nil {
		return fmt.Errorf("Failed to create Claude runtime directory: %w", err)
	}

	runtimeSettings := filepath.Join(plan.runtimeDir, claudeRuntimeSettingsFile)
	if claudePathExists(settingsPath) {
		data, err := os.ReadFile(settingsPath) // #nosec G304 -- settings file path
		if err != nil {
			return fmt.Errorf("Failed to read Claude settings file '%s': %w", settingsPath, err)
		}
		if err := core.AtomicWritePrivate(runtimeSettings, data); err != nil {
			return err
		}
	} else if err := core.AtomicWritePrivate(runtimeSettings, []byte("{}\n")); err != nil {
		return err
	}

	if plan.inheritedEnvFile == nil {
		return nil
	}
	data, err := os.ReadFile(*plan.inheritedEnvFile) // #nosec G304 -- user env file
	if err != nil {
		// Same tolerance as Rust: the run continues without inheriting the
		// env file; the preview already surfaced the warning.
		return nil
	}
	dest := filepath.Join(plan.runtimeDir, claudeRuntimeEnvFile)
	if err := core.AtomicWritePrivate(dest, data); err != nil {
		return err
	}
	plan.env = append(plan.env, [2]string{"CLAUDE_ENV_FILE", dest})
	return nil
}

// claudeLaunchSpec mirrors terminal_launch::LaunchSpec for this slice. There
// is no `secret_env`: with in-process materialisation the settings file never
// travels through argv or an env payload — only its path does.
type claudeLaunchSpec struct {
	program    string
	args       []string
	env        [][2]string
	unsetEnv   []string
	warnings   []string
	cwd        *string
	supportDir string
	noKeepOpen bool
}

// LaunchClaudeWithSettings launches Claude Code in a terminal using the active
// settings file. The file is copied into a runtime-private directory so the
// live configuration is never mutated; when skipDangerous is true the
// `--dangerously-skip-permissions` flag is appended.
func (a *App) LaunchClaudeWithSettings(cwd *string, skipDangerous bool) error {
	if _, err := core.HomeDir(); err != nil {
		return errors.New("Failed to get home directory")
	}
	claudeCleanupStaleRuntimeDirs()

	settingsPath, err := claudeActiveSettingsPath()
	if err != nil {
		return err
	}
	plan, err := claudeBuildSettingsLaunchPlan(skipDangerous)
	if err != nil {
		return err
	}
	if err := claudeMaterializeSettingsRuntime(&plan, settingsPath); err != nil {
		return err
	}

	prefs, err := a.LoadPreferences()
	if err != nil {
		prefs = defaultPreferences()
	}
	preferred := ""
	if prefs.PreferredTerminal != nil {
		preferred = *prefs.PreferredTerminal
	}

	return claudeLaunchInTerminal(claudeLaunchSpec{
		program:    plan.program,
		args:       plan.args,
		env:        plan.env,
		unsetEnv:   plan.unsetEnv,
		warnings:   plan.warnings,
		cwd:        cwd,
		supportDir: plan.runtimeDir,
		noKeepOpen: true,
	}, preferred)
}

// ---------------------------------------------------------------------------
// Preview
// ---------------------------------------------------------------------------

// GetClaudeSettingsLaunchCommand returns a shell command preview for
// launching Claude with the active settings file, plus the resolved path.
func (a *App) GetClaudeSettingsLaunchCommand(skipDangerous bool) ([2]string, error) {
	active, err := claudeActiveSettingsPath()
	if err != nil {
		return [2]string{}, err
	}
	command := fmt.Sprintf("claude --settings \"%s\"", active)
	if skipDangerous {
		command += " --dangerously-skip-permissions"
	}
	return [2]string{command, active}, nil
}

// PreviewClaudeTemporaryRunFromFile builds the debug preview for a temporary
// run driven by a named settings file. It renders the same direct plan
// LaunchClaudeWithSettings executes (`claude --settings <runtime copy>`) —
// the runtime path is generated but no file is created.
func (a *App) PreviewClaudeTemporaryRunFromFile(name string) (ClaudeTemporaryRunDebugPreview, error) {
	if _, err := core.HomeDir(); err != nil {
		return ClaudeTemporaryRunDebugPreview{}, errors.New("Failed to get home directory")
	}
	settingsPath, err := claudeSettingsPathByName(name)
	if err != nil {
		return ClaudeTemporaryRunDebugPreview{}, err
	}

	plan, err := claudeBuildSettingsLaunchPlan(false)
	if err != nil {
		return ClaudeTemporaryRunDebugPreview{}, err
	}

	overlayJSON := "{}"
	if data, err := os.ReadFile(settingsPath); err == nil { // #nosec G304 -- settings file path
		overlayJSON = string(data)
	}

	liveConfigDir := ""
	if len(plan.env) > 0 && plan.env[0][0] == "CLAUDE_CONFIG_DIR" {
		liveConfigDir = plan.env[0][1]
	}
	if liveConfigDir == "" {
		liveConfigDir = settingsPath
	}

	return ClaudeTemporaryRunDebugPreview{
		ProfileID:              name,
		ProfileName:            name,
		Program:                plan.program,
		Args:                   plan.args,
		ChildProgram:           plan.program,
		ChildArgs:              plan.args,
		LiveConfigDir:          liveConfigDir,
		InheritedEnvFileSource: plan.inheritedEnvFile,
		Env:                    plan.env,
		UnsetEnv:               plan.unsetEnv,
		SecretEnvKeys:          []string{},
		Warnings:               plan.warnings,
		SettingsOverlayJSON:    overlayJSON,
	}, nil
}

// ---------------------------------------------------------------------------
// Terminal launch (mirrors src-tauri/src/utils/terminal_launch.rs)
// ---------------------------------------------------------------------------

// Windows process-creation flags (CREATE_NO_WINDOW / CREATE_NEW_CONSOLE).
const (
	claudeCreateNoWindow   = 0x08000000
	claudeCreateNewConsole = 0x00000010
)

// claudeSetWindowsProcessFlags attaches the Windows process-creation flags to
// cmd. syscall.SysProcAttr only carries those fields on Windows, and this
// package must compile everywhere, so they are set reflectively; on other
// platforms the helper is a no-op.
func claudeSetWindowsProcessFlags(cmd *exec.Cmd, hideWindow bool, creationFlags uint32) {
	if !hideWindow && creationFlags == 0 {
		return
	}
	attr := &syscall.SysProcAttr{}
	value := reflect.ValueOf(attr).Elem()
	applied := false
	if hideWindow {
		if field := value.FieldByName("HideWindow"); field.IsValid() && field.CanSet() && field.Kind() == reflect.Bool {
			field.SetBool(true)
			applied = true
		}
	}
	if creationFlags != 0 {
		if field := value.FieldByName("CreationFlags"); field.IsValid() && field.CanSet() && field.Kind() == reflect.Uint32 {
			field.SetUint(uint64(creationFlags))
			applied = true
		}
	}
	if applied {
		cmd.SysProcAttr = attr
	}
}

func claudeQuotePowerShell(value string) string {
	return "'" + strings.ReplaceAll(value, "'", "''") + "'"
}

func claudeEscapeCmdValue(value string) string {
	value = strings.ReplaceAll(value, "^", "^^")
	value = strings.ReplaceAll(value, `"`, `^"`)
	value = strings.ReplaceAll(value, "%", "%%")
	return value
}

func claudeQuoteCmd(value string) string {
	return `"` + claudeEscapeCmdValue(value) + `"`
}

func claudeQuotePosix(value string) string {
	return "'" + strings.ReplaceAll(value, "'", `'\''`) + "'"
}

// claudeStartupMessage renders the terminal banner ("Starting claude...").
func claudeStartupMessage(program string) string {
	display := filepath.Base(program)
	if display == "" || display == "." || display == string(filepath.Separator) {
		display = program
	}
	return "Starting " + display + "..."
}

// claudePowerShellPathRefresh merges the registry Machine + User PATH into
// $env:Path so user-scope installs (npm/uv) resolve inside the wrapper.
func claudePowerShellPathRefresh() string {
	return "$machinePath = [Environment]::GetEnvironmentVariable('Path','Machine'); $userPath = [Environment]::GetEnvironmentVariable('Path','User'); if ($machinePath -or $userPath) { $env:Path = @($machinePath, $userPath, $env:Path | Where-Object { $_ }) -join ';' }"
}

// claudeRenderProgramCommand quotes the program and every argument.
func claudeRenderProgramCommand(spec *claudeLaunchSpec, quote func(string) string) string {
	parts := make([]string, 0, len(spec.args)+1)
	parts = append(parts, quote(spec.program))
	for _, arg := range spec.args {
		parts = append(parts, quote(arg))
	}
	return strings.Join(parts, " ")
}

func claudeLaunchArtifactPath(spec *claudeLaunchSpec, filename string) string {
	if spec.supportDir != "" {
		return filepath.Join(spec.supportDir, filename)
	}
	stamp := time.Now().UTC().Format("20060102T150405.000Z")
	return filepath.Join(os.TempDir(), fmt.Sprintf("%s-%d-%s", stamp, os.Getpid(), filename))
}

func claudeWriteLaunchScript(path string, contents string) error {
	if err := core.EnsureDir(filepath.Dir(path)); err != nil {
		return fmt.Errorf("Failed to create launch script directory: %w", err)
	}
	if err := os.WriteFile(path, []byte(contents), 0o600); err != nil { // #nosec G306 -- wrapper may carry env paths
		return fmt.Errorf("Failed to write launch script: %w", err)
	}
	return nil
}

// claudeWriteSecurePowerShellWrapper renders the self-deleting .ps1 used for
// every Windows launch (unsetEnv is never empty for Claude, so the inline
// branch of the Rust helper is unreachable here).
func claudeWriteSecurePowerShellWrapper(spec *claudeLaunchSpec) (string, error) {
	path := claudeLaunchArtifactPath(spec, "terminal-launch.ps1")
	lines := []string{
		"$ErrorActionPreference = 'Continue'",
		"try {",
		"  " + claudePowerShellPathRefresh(),
		"  Clear-Host",
		"  Write-Host " + claudeQuotePowerShell(claudeStartupMessage(spec.program)),
	}
	for _, warning := range spec.warnings {
		lines = append(lines, "  Write-Host "+claudeQuotePowerShell("Warning: "+warning))
	}
	for _, key := range spec.unsetEnv {
		lines = append(lines, "  Remove-Item Env:"+key+" -ErrorAction SilentlyContinue")
	}
	for _, pair := range spec.env {
		lines = append(lines, "  $env:"+pair[0]+" = "+claudeQuotePowerShell(pair[1]))
	}
	if spec.cwd != nil && *spec.cwd != "" {
		lines = append(lines, "  Set-Location "+claudeQuotePowerShell(*spec.cwd))
	}
	lines = append(lines,
		"  & "+claudeRenderProgramCommand(spec, claudeQuotePowerShell),
		"} finally {",
		"  Remove-Item -LiteralPath $PSCommandPath -Force -ErrorAction SilentlyContinue",
		"}",
	)
	script := strings.Join(lines, "\r\n") + "\r\n"
	return path, claudeWriteLaunchScript(path, script)
}

// claudeWriteSecureCmdWrapper renders the self-deleting .cmd used when the
// user explicitly prefers cmd.
func claudeWriteSecureCmdWrapper(spec *claudeLaunchSpec) (string, error) {
	path := claudeLaunchArtifactPath(spec, "terminal-launch.cmd")
	lines := []string{
		"@echo off",
		"setlocal",
		"cls",
		"echo " + claudeStartupMessage(spec.program),
	}
	for _, warning := range spec.warnings {
		lines = append(lines, "echo Warning: "+warning)
	}
	for _, key := range spec.unsetEnv {
		lines = append(lines, "set "+key+"=")
	}
	for _, pair := range spec.env {
		lines = append(lines, "set \""+pair[0]+"="+claudeEscapeCmdValue(pair[1])+"\"")
	}
	if spec.cwd != nil && *spec.cwd != "" {
		lines = append(lines, "cd /d "+claudeQuoteCmd(*spec.cwd))
	}
	lines = append(lines,
		claudeRenderProgramCommand(spec, claudeQuoteCmd),
		"set \"DROIDGEAR_EXIT_CODE=%ERRORLEVEL%\"",
		"start \"\" /b cmd /c del /f /q \"%~f0\" >nul 2>nul",
		"endlocal & exit /b %DROIDGEAR_EXIT_CODE%",
	)
	script := strings.Join(lines, "\r\n") + "\r\n"
	return path, claudeWriteLaunchScript(path, script)
}

// claudeWindowsProgramCandidates lists PATHEXT-style candidates for a bare
// program name (extensionless shims such as npm's `claude` are skipped).
func claudeWindowsProgramCandidates(program string) []string {
	lower := strings.ToLower(program)
	for _, ext := range []string{".exe", ".cmd", ".bat", ".ps1", ".com"} {
		if strings.HasSuffix(lower, ext) {
			return []string{program}
		}
	}
	return []string{program + ".exe", program + ".cmd", program + ".bat", program + ".ps1"}
}

// claudeSearchProgramInPATH resolves a bare program against the process PATH.
// The Rust helper additionally probes the registry-backed Machine/User PATH
// (via a hidden PowerShell call); that probe is not ported — the PowerShell
// wrapper refreshes PATH itself, so user-scope installs still resolve.
func claudeSearchProgramInPATH(candidates []string) string {
	pathValue := os.Getenv("PATH")
	for _, dir := range filepath.SplitList(pathValue) {
		dir = strings.TrimSpace(dir)
		if dir == "" {
			continue
		}
		for _, name := range candidates {
			candidate := filepath.Join(dir, name)
			if info, err := os.Stat(candidate); err == nil && info.Mode().IsRegular() {
				return candidate
			}
		}
	}
	return ""
}

func claudeResolveWindowsProgram(program string) string {
	if filepath.IsAbs(program) {
		return program
	}
	if strings.ContainsAny(program, `\/`) {
		return program
	}
	if found := claudeSearchProgramInPATH(claudeWindowsProgramCandidates(program)); found != "" {
		return found
	}
	return program
}

func claudeIsRegularFile(path string) bool {
	info, err := os.Stat(path)
	return err == nil && info.Mode().IsRegular()
}

// claudeFindWindowsShell resolves pwsh (preferred) or Windows PowerShell,
// including the common install locations when PATH is stale.
func claudeFindWindowsShell(name string) string {
	if resolved := claudeResolveWindowsProgram(name); claudeIsRegularFile(resolved) {
		return resolved
	}
	if !strings.EqualFold(name, "pwsh") {
		return ""
	}
	var candidates []string
	if programFiles := os.Getenv("ProgramFiles"); programFiles != "" {
		candidates = append(candidates,
			filepath.Join(programFiles, "PowerShell", "7", "pwsh.exe"),
			filepath.Join(programFiles, "PowerShell", "7-preview", "pwsh.exe"),
		)
	}
	if localAppData := os.Getenv("LOCALAPPDATA"); localAppData != "" {
		candidates = append(candidates,
			filepath.Join(localAppData, "Microsoft", "powershell", "pwsh.exe"),
			filepath.Join(localAppData, "Microsoft", "WindowsApps", "pwsh.exe"),
		)
	}
	for _, candidate := range candidates {
		if claudePathExists(candidate) {
			return candidate
		}
	}
	return ""
}

func claudeResolveWindowsPowerShell() string {
	if path := claudeFindWindowsShell("pwsh"); path != "" {
		return path
	}
	if path := claudeFindWindowsShell("powershell"); path != "" {
		return path
	}
	return "powershell"
}

// claudeWindowsPSLaunchCommand always writes the wrapper: unsetEnv is never
// empty for Claude, so the inline multi-statement branch never applies.
func claudeWindowsPSLaunchCommand(spec *claudeLaunchSpec) (string, error) {
	path, err := claudeWriteSecurePowerShellWrapper(spec)
	if err != nil {
		return "", err
	}
	return "& " + claudeQuotePowerShell(path), nil
}

// claudeLaunchWindowsCmdPayload runs `cmd /c start "" [/D cwd] cmd /k
// <payload>` so the visible console detaches from the GUI process tree; the
// outer hop stays hidden (CREATE_NO_WINDOW).
func claudeLaunchWindowsCmdPayload(payload string, cwd *string) error {
	args := []string{"/c", "start", ""}
	if cwd != nil && *cwd != "" {
		args = append(args, "/D", *cwd)
	}
	args = append(args, "cmd", "/k", payload)

	cmd := exec.Command("cmd", args...) // #nosec G204 -- static hop, user cwd only
	claudeSetWindowsProcessFlags(cmd, true, claudeCreateNoWindow)
	if err := cmd.Start(); err != nil {
		return fmt.Errorf("Failed to launch cmd: %w", err)
	}
	return nil
}

// claudeLaunchWindowsPowerShellPayload opens a dedicated PowerShell window.
func claudeLaunchWindowsPowerShellPayload(shell string, command string, cwd *string, noKeepOpen bool) error {
	args := []string{}
	if cwd != nil && *cwd != "" {
		args = append(args, "-WorkingDirectory", *cwd)
	}
	if !noKeepOpen {
		args = append(args, "-NoExit")
	}
	args = append(args, "-Command", command)

	cmd := exec.Command(shell, args...) // #nosec G204 -- resolved shell
	claudeSetWindowsProcessFlags(cmd, false, claudeCreateNewConsole)
	if err := cmd.Start(); err != nil {
		return fmt.Errorf("Failed to launch PowerShell: %w", err)
	}
	return nil
}

// claudeLaunchWindowsWtOrPsPayload prefers Windows Terminal, falling back to
// a plain PowerShell window.
func claudeLaunchWindowsWtOrPsPayload(shell string, command string, cwd *string, noKeepOpen bool) error {
	args := []string{}
	if cwd != nil && *cwd != "" {
		args = append(args, "-d", *cwd)
	}
	args = append(args, shell)
	if !noKeepOpen {
		args = append(args, "-NoExit")
	}
	args = append(args, "-Command", command)

	if cmd := exec.Command("wt", args...); cmd.Start() == nil { // #nosec G204 -- Windows Terminal
		return nil
	}
	return claudeLaunchWindowsPowerShellPayload(shell, command, cwd, noKeepOpen)
}

// claudeLaunchWindows dispatches on the preferred terminal (cmd /
// powershell / default = Windows Terminal + PowerShell).
func claudeLaunchWindows(spec claudeLaunchSpec, preferred string) error {
	spec.program = claudeResolveWindowsProgram(spec.program)
	shell := claudeResolveWindowsPowerShell()

	switch preferred {
	case "cmd":
		path, err := claudeWriteSecureCmdWrapper(&spec)
		if err != nil {
			return err
		}
		return claudeLaunchWindowsCmdPayload(path, spec.cwd)
	case "powershell":
		command, err := claudeWindowsPSLaunchCommand(&spec)
		if err != nil {
			return err
		}
		return claudeLaunchWindowsPowerShellPayload(shell, command, spec.cwd, spec.noKeepOpen)
	default:
		command, err := claudeWindowsPSLaunchCommand(&spec)
		if err != nil {
			return err
		}
		return claudeLaunchWindowsWtOrPsPayload(shell, command, spec.cwd, spec.noKeepOpen)
	}
}

// ---------------------------------------------------------------------------
// POSIX launch (linux terminal chain, macOS system default)
// ---------------------------------------------------------------------------

func claudeRenderPosixEnv(spec *claudeLaunchSpec) string {
	parts := []string{}
	for _, key := range spec.unsetEnv {
		parts = append(parts, "unset "+key)
	}
	for _, pair := range spec.env {
		parts = append(parts, "export "+pair[0]+"="+claudeQuotePosix(pair[1]))
	}
	return strings.Join(parts, "; ")
}

// claudePreparePosixCommand renders the inline command; without a secret env
// payload the Rust secure-wrapper branch does not apply.
func claudePreparePosixCommand(spec *claudeLaunchSpec) (string, string) {
	envSetup := claudeRenderPosixEnv(spec)
	cd := ""
	if spec.cwd != nil && *spec.cwd != "" {
		cd = "cd " + claudeQuotePosix(*spec.cwd)
	}
	parts := []string{}
	for _, part := range []string{envSetup, cd, claudeRenderProgramCommand(spec, claudeQuotePosix)} {
		if part != "" {
			parts = append(parts, part)
		}
	}
	base := strings.Join(parts, "; ")
	return base, base + "; exec bash"
}

func claudePosixBanner(spec *claudeLaunchSpec) string {
	return "printf '%s\\n' " + claudeQuotePosix(claudeStartupMessage(spec.program))
}

// claudeLaunchPosixMac writes a self-deleting .command file and opens it with
// the system default handler (the iTerm2/Ghostty/Terminal.app special cases of
// the Rust helper are not ported — the Wails build targets Windows).
func claudeLaunchPosixMac(spec *claudeLaunchSpec, command string) error {
	path := claudeLaunchArtifactPath(spec, "terminal-launch.command")
	script := "#!/bin/bash\nrm -f -- \"$0\"\nclear\n" + claudePosixBanner(spec) + "\n" + command + "\nexit\n"
	if err := claudeWriteLaunchScript(path, script); err != nil {
		return err
	}
	if err := os.Chmod(path, 0o700); err != nil { // #nosec G302 -- executable wrapper
		return fmt.Errorf("Failed to write launch script: %w", err)
	}
	if err := exec.Command("open", path).Run(); err != nil { // #nosec G204 -- opens the wrapper
		return fmt.Errorf("Failed to open terminal: %w", err)
	}
	return nil
}

// claudeLaunchPosixLinux tries the preferred terminal emulator (or the whole
// detection chain) with a keep-open command.
func claudeLaunchPosixLinux(spec *claudeLaunchSpec, preferred string) error {
	_, keepOpen := claudePreparePosixCommand(spec)

	type terminal struct {
		name string
		args []string
	}
	bashc := []string{"bash", "-c", keepOpen}
	chain := []terminal{
		{"gnome-terminal", append([]string{"--tab", "--"}, bashc...)},
		{"konsole", append([]string{"--new-tab", "-e"}, bashc...)},
		{"xfce4-terminal", []string{"--tab", "-e", "bash -c " + claudeQuotePosix(keepOpen)}},
		{"x-terminal-emulator", append([]string{"-e"}, bashc...)},
		{"xterm", append([]string{"-e"}, bashc...)},
	}
	switch preferred {
	case "gnome-terminal", "konsole", "xfce4-terminal", "x-terminal-emulator":
		for _, entry := range chain {
			if entry.name == preferred {
				chain = []terminal{entry}
				break
			}
		}
	}

	lastErr := ""
	for _, entry := range chain {
		cmd := exec.Command(entry.name, entry.args...) // #nosec G204 -- user's terminal
		if err := cmd.Start(); err == nil {
			return nil
		} else {
			lastErr = fmt.Sprintf("%s: %v", entry.name, err)
		}
	}
	return fmt.Errorf("Could not find a terminal emulator. Tried: %s", lastErr)
}

func claudeLaunchPosix(spec claudeLaunchSpec, preferred string) error {
	if runtime.GOOS == "darwin" {
		_, command := claudePreparePosixCommand(&spec)
		return claudeLaunchPosixMac(&spec, command)
	}
	return claudeLaunchPosixLinux(&spec, preferred)
}

// claudeLaunchInTerminal dispatches to the platform launcher.
func claudeLaunchInTerminal(spec claudeLaunchSpec, preferred string) error {
	if runtime.GOOS == "windows" {
		return claudeLaunchWindows(spec, preferred)
	}
	return claudeLaunchPosix(spec, preferred)
}
