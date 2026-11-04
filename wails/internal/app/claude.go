// Claude Code profile management: DroidGear-managed profiles under
// ~/.droidgear/claude/ plus the live ~/.claude/settings.json they apply to.
// Mirrors droidgear-core::claude (Rust) — behaviour, JSON shape and error
// strings included.
package app

import (
	"bytes"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"sort"
	"strings"

	"droidgear-wails/internal/core"
)

// ---------------------------------------------------------------------------
// Types (mirroring droidgear-core::claude)
// ---------------------------------------------------------------------------

// ClaudeReasoningEffort mirrors claude::ClaudeReasoningEffort.
type ClaudeReasoningEffort string

// Reasoning effort values (serde `rename_all = "lowercase"`).
const (
	ClaudeReasoningEffortLow    ClaudeReasoningEffort = "low"
	ClaudeReasoningEffortMedium ClaudeReasoningEffort = "medium"
	ClaudeReasoningEffortHigh   ClaudeReasoningEffort = "high"
	ClaudeReasoningEffortMax    ClaudeReasoningEffort = "max"
)

// ClaudeThinkingMode mirrors claude::ClaudeThinkingMode.
type ClaudeThinkingMode string

// Thinking mode values (serde `rename_all = "lowercase"`).
const (
	ClaudeThinkingModeInherit ClaudeThinkingMode = "inherit"
	ClaudeThinkingModeOn      ClaudeThinkingMode = "on"
	ClaudeThinkingModeOff     ClaudeThinkingMode = "off"
)

// ClaudeCodeProfile mirrors claude::ClaudeCodeProfile (camelCase JSON).
type ClaudeCodeProfile struct {
	ID                      string                 `json:"id"`
	Name                    string                 `json:"name"`
	Description             *string                `json:"description,omitempty"`
	BaseURL                 *string                `json:"baseUrl,omitempty"`
	BearerToken             *string                `json:"bearerToken,omitempty"`
	Model                   *string                `json:"model,omitempty"`
	SmallModelUsesMainModel bool                   `json:"smallModelUsesMainModel"`
	SmallModel              *string                `json:"smallModel,omitempty"`
	ReasoningEffort         *ClaudeReasoningEffort `json:"reasoningEffort,omitempty"`
	ThinkingMode            ClaudeThinkingMode     `json:"thinkingMode"`
	CreatedAt               string                 `json:"createdAt"`
	UpdatedAt               string                 `json:"updatedAt"`
}

// UnmarshalJSON applies the Rust `#[serde(default)]` semantics: a missing
// thinking mode becomes "inherit", and enum values are validated the way
// serde's `unknown variant` check does.
func (p *ClaudeCodeProfile) UnmarshalJSON(data []byte) error {
	type plainClaudeCodeProfile ClaudeCodeProfile
	var decoded plainClaudeCodeProfile
	if err := json.Unmarshal(data, &decoded); err != nil {
		return err
	}
	*p = ClaudeCodeProfile(decoded)
	if p.ThinkingMode == "" {
		p.ThinkingMode = ClaudeThinkingModeInherit
	} else {
		switch p.ThinkingMode {
		case ClaudeThinkingModeInherit, ClaudeThinkingModeOn, ClaudeThinkingModeOff:
		default:
			return fmt.Errorf("unknown variant `%s`, expected one of `inherit`, `on`, `off`", p.ThinkingMode)
		}
	}
	if p.ReasoningEffort != nil {
		switch *p.ReasoningEffort {
		case ClaudeReasoningEffortLow, ClaudeReasoningEffortMedium,
			ClaudeReasoningEffortHigh, ClaudeReasoningEffortMax:
		default:
			return fmt.Errorf("unknown variant `%s`, expected one of `low`, `medium`, `high`, `max`", *p.ReasoningEffort)
		}
	}
	return nil
}

// ClaudeConfigStatus mirrors claude::ClaudeConfigStatus.
type ClaudeConfigStatus struct {
	SettingsExists bool    `json:"settingsExists"`
	SettingsPath   string  `json:"settingsPath"`
	ConfigDir      string  `json:"configDir"`
	ParseError     *string `json:"parseError,omitempty"`
}

// ClaudeCurrentConfig mirrors claude::ClaudeCurrentConfig.
type ClaudeCurrentConfig struct {
	BaseURL                 *string                `json:"baseUrl,omitempty"`
	BearerToken             *string                `json:"bearerToken,omitempty"`
	Model                   *string                `json:"model,omitempty"`
	SmallModelUsesMainModel bool                   `json:"smallModelUsesMainModel"`
	SmallModel              *string                `json:"smallModel,omitempty"`
	ReasoningEffort         *ClaudeReasoningEffort `json:"reasoningEffort,omitempty"`
	ThinkingMode            ClaudeThinkingMode     `json:"thinkingMode"`
}

// ---------------------------------------------------------------------------
// Managed env keys (mirroring the CLAUDE_*_ENV constants)
// ---------------------------------------------------------------------------

const (
	claudeBaseURLEnv           = "ANTHROPIC_BASE_URL"
	claudeAuthTokenEnv         = "ANTHROPIC_AUTH_TOKEN"
	claudeAPIKeyEnv            = "ANTHROPIC_API_KEY"
	claudeModelEnv             = "ANTHROPIC_MODEL"
	claudeSmallModelEnv        = "ANTHROPIC_DEFAULT_HAIKU_MODEL"
	claudeEffortEnv            = "CLAUDE_CODE_EFFORT_LEVEL"
	claudeDisableThinkingEnv   = "CLAUDE_CODE_DISABLE_THINKING"
	claudeMaxThinkingTokensEnv = "MAX_THINKING_TOKENS"
	claudeDisableAdaptiveEnv   = "CLAUDE_CODE_DISABLE_ADAPTIVE_THINKING"
)

// claudeConflictEnvKeys mirrors claude::CLAUDE_CONFLICT_ENV_KEYS: provider
// specific settings that must never survive a DroidGear-managed write.
var claudeConflictEnvKeys = []string{
	claudeAPIKeyEnv,
	"CLAUDE_CODE_USE_BEDROCK",
	"CLAUDE_CODE_USE_VERTEX",
	"CLAUDE_CODE_USE_FOUNDRY",
	"ANTHROPIC_BEDROCK_BASE_URL",
	"ANTHROPIC_BEDROCK_MANTLE_BASE_URL",
	"ANTHROPIC_VERTEX_BASE_URL",
	"ANTHROPIC_VERTEX_PROJECT_ID",
	"ANTHROPIC_FOUNDRY_BASE_URL",
	"ANTHROPIC_FOUNDRY_RESOURCE",
	"ANTHROPIC_FOUNDRY_API_KEY",
	"CLAUDE_CODE_PROVIDER_MANAGED_BY_HOST",
}

// ---------------------------------------------------------------------------
// Paths
// ---------------------------------------------------------------------------

// claudePathExists mirrors Rust Path::exists (directories count too).
func claudePathExists(path string) bool {
	_, err := os.Stat(path)
	return err == nil
}

// claudeDroidGearDir returns ~/.droidgear/claude.
func claudeDroidGearDir() (string, error) {
	dir, err := core.DroidGearDir()
	if err != nil {
		return "", err
	}
	return filepath.Join(dir, "claude"), nil
}

// claudeProfilesDir returns ~/.droidgear/claude/profiles/, creating it when
// missing (mirrors profiles_dir_for_home).
func claudeProfilesDir() (string, error) {
	dir, err := claudeDroidGearDir()
	if err != nil {
		return "", err
	}
	dir = filepath.Join(dir, "profiles")
	if err := core.EnsureDir(dir); err != nil {
		return "", fmt.Errorf("Failed to create claude profiles directory: %w", err)
	}
	return dir, nil
}

// claudeActiveProfilePath returns ~/.droidgear/claude/active-profile.txt,
// creating the parent directory when missing.
func claudeActiveProfilePath() (string, error) {
	dir, err := claudeDroidGearDir()
	if err != nil {
		return "", err
	}
	if err := core.EnsureDir(dir); err != nil {
		return "", fmt.Errorf("Failed to create claude directory: %w", err)
	}
	return filepath.Join(dir, "active-profile.txt"), nil
}

// claudeConfigDirNoCreate resolves the Claude Code config directory
// (Preferences → Paths override or ~/.claude) without touching disk.
func claudeConfigDirNoCreate() (string, error) {
	path, _, err := core.EffectivePathForKey("claude")
	if err != nil {
		return "", err
	}
	return path, nil
}

// claudeConfigDir resolves the Claude Code config directory and creates it
// when missing — the Rust claude_config_dir_for_home creates eagerly too.
func claudeConfigDir() (string, error) {
	dir, err := claudeConfigDirNoCreate()
	if err != nil {
		return "", err
	}
	if err := core.EnsureDir(dir); err != nil {
		return "", fmt.Errorf("Failed to create claude config directory: %w", err)
	}
	return dir, nil
}

// claudeSettingsPath returns <config dir>/settings.json (creating the dir).
func claudeSettingsPath() (string, error) {
	dir, err := claudeConfigDir()
	if err != nil {
		return "", err
	}
	return filepath.Join(dir, "settings.json"), nil
}

// claudeProfilePath resolves profiles/<id>.json after validating the id.
func claudeProfilePath(id string) (string, error) {
	if err := core.ValidateProfileID(id); err != nil {
		return "", err
	}
	dir, err := claudeProfilesDir()
	if err != nil {
		return "", err
	}
	return filepath.Join(dir, id+".json"), nil
}

// ---------------------------------------------------------------------------
// JSON helpers (serde_json::Value equivalents with number fidelity)
// ---------------------------------------------------------------------------

var errClaudeRootNotObject = errors.New("root must be a JSON object")

// claudeDecodeJSON parses a whole document the way serde_json::from_str
// does: numbers keep their literal form (json.Number) and trailing content
// is rejected.
func claudeDecodeJSON(data []byte) (any, error) {
	dec := json.NewDecoder(bytes.NewReader(data))
	dec.UseNumber()
	var value any
	if err := dec.Decode(&value); err != nil {
		return nil, err
	}
	var trailing json.RawMessage
	if err := dec.Decode(&trailing); !errors.Is(err, io.EOF) {
		return nil, errors.New("trailing characters")
	}
	return value, nil
}

// claudeDecodeJSONObject parses a document that must be a JSON object;
// a non-object root yields errClaudeRootNotObject.
func claudeDecodeJSONObject(data []byte) (map[string]any, error) {
	value, err := claudeDecodeJSON(data)
	if err != nil {
		return nil, err
	}
	obj, ok := value.(map[string]any)
	if !ok {
		return nil, errClaudeRootNotObject
	}
	return obj, nil
}

// claudeReadSettingsObject reads a settings.json document; a missing or
// empty file yields an empty object (mirrors read_settings_object_from_path).
func claudeReadSettingsObject(path string) (map[string]any, error) {
	data, err := os.ReadFile(path) // #nosec G304 -- tool config path
	if err != nil {
		if os.IsNotExist(err) {
			return map[string]any{}, nil
		}
		return nil, fmt.Errorf("Failed to read settings.json: %w", err)
	}
	if strings.TrimSpace(string(data)) == "" {
		return map[string]any{}, nil
	}
	root, err := claudeDecodeJSONObject(data)
	if err != nil {
		if errors.Is(err, errClaudeRootNotObject) {
			return nil, errors.New("settings.json root must be a JSON object")
		}
		return nil, fmt.Errorf("Failed to parse settings.json: %w", err)
	}
	return root, nil
}

// claudeEnvObject returns the `env` sub-object of a settings document.
// A missing key yields (nil, nil); a non-object value is an error.
func claudeEnvObject(root map[string]any) (map[string]any, error) {
	raw, ok := root["env"]
	if !ok {
		return nil, nil
	}
	obj, ok := raw.(map[string]any)
	if !ok {
		return nil, errors.New("settings.json env must be a JSON object")
	}
	return obj, nil
}

// claudeNormalizeOptionalString trims a value and drops empty strings
// (mirrors claude::normalize_optional_string).
func claudeNormalizeOptionalString(value *string) *string {
	if value == nil {
		return nil
	}
	trimmed := strings.TrimSpace(*value)
	if trimmed == "" {
		return nil
	}
	return &trimmed
}

// claudeSetEnvString sets or removes one managed env key.
func claudeSetEnvString(env map[string]any, key string, value *string) {
	if normalized := claudeNormalizeOptionalString(value); normalized != nil {
		env[key] = *normalized
		return
	}
	delete(env, key)
}

// claudeGetStringFromMap reads a string field: null/absent/blank → nil,
// non-string → the Rust `settings.json {key} must be a string` error.
func claudeGetStringFromMap(m map[string]any, key string) (*string, error) {
	raw, ok := m[key]
	if !ok || raw == nil {
		return nil, nil
	}
	value, ok := raw.(string)
	if !ok {
		return nil, fmt.Errorf("settings.json %s must be a string", key)
	}
	trimmed := strings.TrimSpace(value)
	if trimmed == "" {
		return nil, nil
	}
	return &trimmed, nil
}

// claudeGetBoolFromMap reads a boolean field (mirrors get_bool_from_map).
func claudeGetBoolFromMap(m map[string]any, key string) (*bool, error) {
	raw, ok := m[key]
	if !ok || raw == nil {
		return nil, nil
	}
	value, ok := raw.(bool)
	if !ok {
		return nil, fmt.Errorf("settings.json %s must be a boolean", key)
	}
	return &value, nil
}

// claudeEnvValueIsTruthy mirrors env_value_is_truthy: true, "1/true/yes/on"
// or a non-zero integer.
func claudeEnvValueIsTruthy(value any) bool {
	switch typed := value.(type) {
	case bool:
		return typed
	case string:
		switch strings.ToLower(strings.TrimSpace(typed)) {
		case "1", "true", "yes", "on":
			return true
		}
		return false
	case json.Number:
		parsed, err := typed.Int64()
		return err == nil && parsed != 0
	default:
		return false
	}
}

// claudeParseReasoningEffort maps an env/top-level string to an effort.
func claudeParseReasoningEffort(value *string) *ClaudeReasoningEffort {
	if value == nil {
		return nil
	}
	switch strings.ToLower(strings.TrimSpace(*value)) {
	case "low":
		return &[]ClaudeReasoningEffort{ClaudeReasoningEffortLow}[0]
	case "medium":
		return &[]ClaudeReasoningEffort{ClaudeReasoningEffortMedium}[0]
	case "high":
		return &[]ClaudeReasoningEffort{ClaudeReasoningEffortHigh}[0]
	case "max":
		return &[]ClaudeReasoningEffort{ClaudeReasoningEffortMax}[0]
	}
	return nil
}

// claudeRemoveConflictingEnvKeys drops every provider-specific key.
func claudeRemoveConflictingEnvKeys(env map[string]any) {
	for _, key := range claudeConflictEnvKeys {
		delete(env, key)
	}
}

// claudeApplyReasoningEnv writes/clears the effort env keys
// (mirrors apply_reasoning_env).
func claudeApplyReasoningEnv(env map[string]any, effort *ClaudeReasoningEffort) {
	if effort == nil {
		delete(env, claudeEffortEnv)
		delete(env, claudeDisableAdaptiveEnv)
		return
	}
	switch *effort {
	case ClaudeReasoningEffortLow, ClaudeReasoningEffortMedium:
		env[claudeEffortEnv] = string(*effort)
		delete(env, claudeDisableAdaptiveEnv)
	case ClaudeReasoningEffortHigh, ClaudeReasoningEffortMax:
		env[claudeEffortEnv] = string(*effort)
		env[claudeDisableAdaptiveEnv] = "1"
	default:
		delete(env, claudeEffortEnv)
		delete(env, claudeDisableAdaptiveEnv)
	}
}

// claudeApplyThinkingSettings writes the thinking-mode keys into the root
// document and env map (mirrors apply_thinking_settings).
func claudeApplyThinkingSettings(root map[string]any, env map[string]any, mode ClaudeThinkingMode) {
	switch mode {
	case ClaudeThinkingModeOn:
		root["alwaysThinkingEnabled"] = true
		delete(env, claudeDisableThinkingEnv)
		delete(env, claudeMaxThinkingTokensEnv)
	case ClaudeThinkingModeOff:
		root["alwaysThinkingEnabled"] = false
		env[claudeDisableThinkingEnv] = "1"
		delete(env, claudeMaxThinkingTokensEnv)
	default: // inherit
		delete(root, "alwaysThinkingEnabled")
		delete(env, claudeDisableThinkingEnv)
	}
}

// claudeResolvedSmallModel returns the small-model value a profile implies:
// the main model when mirroring is enabled, otherwise the explicit value.
func claudeResolvedSmallModel(profile ClaudeCodeProfile) *string {
	if profile.SmallModelUsesMainModel {
		return claudeNormalizeOptionalString(profile.Model)
	}
	return claudeNormalizeOptionalString(profile.SmallModel)
}

// claudeApplyProfileToSettings merges a profile into settings.json with a
// read-modify-write cycle so unrelated keys survive (mirrors
// apply_profile_to_settings_path).
func claudeApplyProfileToSettings(profile ClaudeCodeProfile, settingsPath string) error {
	root, err := claudeReadSettingsObject(settingsPath)
	if err != nil {
		return err
	}
	env, err := claudeEnvObject(root)
	if err != nil {
		return err
	}
	if env == nil {
		env = map[string]any{}
	}

	claudeSetEnvString(env, claudeBaseURLEnv, profile.BaseURL)
	claudeSetEnvString(env, claudeAuthTokenEnv, profile.BearerToken)
	claudeSetEnvString(env, claudeModelEnv, profile.Model)
	claudeSetEnvString(env, claudeSmallModelEnv, claudeResolvedSmallModel(profile))

	claudeApplyReasoningEnv(env, profile.ReasoningEffort)
	claudeApplyThinkingSettings(root, env, profile.ThinkingMode)
	claudeRemoveConflictingEnvKeys(env)

	if len(env) == 0 {
		delete(root, "env")
	} else {
		root["env"] = env
	}
	if inner, ok := root["env"].(map[string]any); ok && len(inner) == 0 {
		delete(root, "env")
	}

	data, err := json.MarshalIndent(root, "", "  ")
	if err != nil {
		return fmt.Errorf("Failed to serialize settings.json: %w", err)
	}
	return core.AtomicWrite(settingsPath, data)
}

// claudeBuildCurrentConfig reads the managed values back out of a settings
// document (mirrors build_current_config_from_settings).
func claudeBuildCurrentConfig(root map[string]any) (ClaudeCurrentConfig, error) {
	env, err := claudeEnvObject(root)
	if err != nil {
		return ClaudeCurrentConfig{}, err
	}

	getEnvString := func(key string) (*string, error) {
		if env == nil {
			return nil, nil
		}
		return claudeGetStringFromMap(env, key)
	}

	baseURL, err := getEnvString(claudeBaseURLEnv)
	if err != nil {
		return ClaudeCurrentConfig{}, err
	}
	bearerToken, err := getEnvString(claudeAuthTokenEnv)
	if err != nil {
		return ClaudeCurrentConfig{}, err
	}

	model, err := getEnvString(claudeModelEnv)
	if err != nil {
		return ClaudeCurrentConfig{}, err
	}
	if model == nil {
		model, err = claudeGetStringFromMap(root, "model")
		if err != nil {
			return ClaudeCurrentConfig{}, err
		}
	}

	effortString, err := getEnvString(claudeEffortEnv)
	if err != nil {
		return ClaudeCurrentConfig{}, err
	}
	if effortString == nil {
		effortString, err = claudeGetStringFromMap(root, "effortLevel")
		if err != nil {
			return ClaudeCurrentConfig{}, err
		}
	}
	reasoningEffort := claudeParseReasoningEffort(effortString)

	smallModel, err := getEnvString(claudeSmallModelEnv)
	if err != nil {
		return ClaudeCurrentConfig{}, err
	}

	var thinkingMode ClaudeThinkingMode = ClaudeThinkingModeInherit
	if env != nil && claudeEnvValueIsTruthy(env[claudeDisableThinkingEnv]) {
		thinkingMode = ClaudeThinkingModeOff
	} else {
		alwaysThinking, err := claudeGetBoolFromMap(root, "alwaysThinkingEnabled")
		if err != nil {
			return ClaudeCurrentConfig{}, err
		}
		switch {
		case alwaysThinking == nil:
			thinkingMode = ClaudeThinkingModeInherit
		case *alwaysThinking:
			thinkingMode = ClaudeThinkingModeOn
		default:
			thinkingMode = ClaudeThinkingModeOff
		}
	}

	smallModelUsesMainModel := smallModel != nil && model != nil && *smallModel == *model

	return ClaudeCurrentConfig{
		BaseURL:                 baseURL,
		BearerToken:             bearerToken,
		Model:                   model,
		SmallModelUsesMainModel: smallModelUsesMainModel,
		SmallModel:              smallModel,
		ReasoningEffort:         reasoningEffort,
		ThinkingMode:            thinkingMode,
	}, nil
}

// ---------------------------------------------------------------------------
// Profile CRUD helpers
// ---------------------------------------------------------------------------

func claudeReadProfileFile(path string) (ClaudeCodeProfile, error) {
	data, err := os.ReadFile(path) // #nosec G304 -- profile path
	if err != nil {
		return ClaudeCodeProfile{}, fmt.Errorf("Failed to read profile: %w", err)
	}
	var profile ClaudeCodeProfile
	if err := json.Unmarshal(data, &profile); err != nil {
		return ClaudeCodeProfile{}, fmt.Errorf("Invalid profile JSON: %w", err)
	}
	return profile, nil
}

func claudeWriteProfileFile(profile ClaudeCodeProfile) error {
	path, err := claudeProfilePath(profile.ID)
	if err != nil {
		return err
	}
	data, err := json.MarshalIndent(profile, "", "  ")
	if err != nil {
		return fmt.Errorf("Failed to serialize profile JSON: %w", err)
	}
	return core.AtomicWrite(path, data)
}

// claudeListProfiles reads every profiles/*.json entry, skipping files that
// fail to parse, sorted by lower-cased name (mirrors
// list_claude_profiles_for_home).
func claudeListProfiles() ([]ClaudeCodeProfile, error) {
	dir, err := claudeProfilesDir()
	if err != nil {
		return nil, err
	}
	if !claudePathExists(dir) {
		return []ClaudeCodeProfile{}, nil
	}
	entries, err := os.ReadDir(dir)
	if err != nil {
		return nil, fmt.Errorf("Failed to read profiles dir: %w", err)
	}
	profiles := []ClaudeCodeProfile{}
	for _, entry := range entries {
		if entry.IsDir() || filepath.Ext(entry.Name()) != ".json" {
			continue
		}
		if profile, err := claudeReadProfileFile(filepath.Join(dir, entry.Name())); err == nil {
			profiles = append(profiles, profile)
		}
	}
	sort.SliceStable(profiles, func(i, j int) bool {
		return strings.ToLower(profiles[i].Name) < strings.ToLower(profiles[j].Name)
	})
	return profiles, nil
}

// claudeSetActiveProfileID persists (or clears) active-profile.txt. The
// exported set_active_claude_profile_id command is not part of this slice;
// apply_claude_profile uses the same core behaviour.
func claudeSetActiveProfileID(id string) error {
	path, err := claudeActiveProfilePath()
	if err != nil {
		return err
	}
	trimmed := strings.TrimSpace(id)
	if trimmed == "" {
		if claudePathExists(path) {
			if err := os.Remove(path); err != nil {
				return fmt.Errorf("Failed to clear active profile id: %w", err)
			}
		}
		return nil
	}
	if err := core.ValidateProfileID(trimmed); err != nil {
		return err
	}
	return core.AtomicWrite(path, []byte(trimmed))
}

// ---------------------------------------------------------------------------
// Commands — profile CRUD
// ---------------------------------------------------------------------------

// ---------------------------------------------------------------------------
// Apply / status / read current config
// ---------------------------------------------------------------------------
