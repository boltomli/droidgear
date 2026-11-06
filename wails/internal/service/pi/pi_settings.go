package pi

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"strings"

	"droidgear-wails/internal/core"
)

// ---------------------------------------------------------------------------
// Startup model selection (~/.pi/agent/settings.json)
// ---------------------------------------------------------------------------
//
// pi keeps two documents in the agent directory: models.json (providers and
// their models — what a profile carries) and settings.json (user settings,
// including which model to start with). DroidGear's profile flow only owns
// models.json, so the startup model lives outside profiles and is written
// straight into the live settings.json, exactly like pi itself reads it.
//
// Upstream reference (pi docs/settings.md § Model and thinking):
//   defaultProvider      startup AI provider
//   defaultModel         startup model ID
//   defaultThinkingLevel startup thinking level
//
// Every other key in settings.json (retry, theme, compaction, …) is preserved
// verbatim: saves are a read-modify-write over the raw JSON object.

// ThinkingLevels is the upstream `defaultThinkingLevel` enum, in display order.
var ThinkingLevels = []string{"off", "minimal", "low", "medium", "high", "xhigh", "max"}

// PiSettings is the startup model selection DroidGear edits. A nil field means
// "leave pi's automatic choice" and is written as a removed key.
type PiSettings struct {
	DefaultProvider      *string `json:"defaultProvider,omitempty"`
	DefaultModel         *string `json:"defaultModel,omitempty"`
	DefaultThinkingLevel *string `json:"defaultThinkingLevel,omitempty"`
}

// PiSettingsStatus is PiSettings plus the document's location/state, so the
// GUI, `dg pi settings show` and the TUI render one shape.
type PiSettingsStatus struct {
	PiSettings
	SettingsExists bool   `json:"settingsExists"`
	SettingsPath   string `json:"settingsPath"`
}

// settingKeys are the root keys this file owns; everything else is foreign.
var settingKeys = []string{"defaultProvider", "defaultModel", "defaultThinkingLevel"}

func piSettingsPath() (string, error) {
	dir, err := piConfigDir()
	if err != nil {
		return "", err
	}
	return filepath.Join(dir, "settings.json"), nil
}

// normalizeThinkingLevel trims the value, treats "" as unset and rejects
// values outside the upstream enum (a typo here silently disables the
// setting, which is worse than failing the write).
func normalizeThinkingLevel(value *string) (*string, error) {
	if value == nil {
		return nil, nil
	}
	trimmed := strings.TrimSpace(*value)
	if trimmed == "" {
		return nil, nil
	}
	for _, level := range ThinkingLevels {
		if trimmed == level {
			return &trimmed, nil
		}
	}
	return nil, fmt.Errorf(
		"Invalid Pi thinking level %q (want %s)",
		trimmed, strings.Join(ThinkingLevels, " | "),
	)
}

func trimSetting(value *string) *string {
	if value == nil {
		return nil
	}
	trimmed := strings.TrimSpace(*value)
	if trimmed == "" {
		return nil
	}
	return &trimmed
}

// ReadSettings parses ~/.pi/agent/settings.json (a missing file is not an
// error — it means "nothing pinned yet").
func ReadSettings() (PiSettingsStatus, error) {
	path, err := piSettingsPath()
	if err != nil {
		return PiSettingsStatus{}, err
	}
	status := PiSettingsStatus{SettingsPath: path}
	data, err := os.ReadFile(path) // #nosec G304 -- tool config path
	if err != nil {
		if os.IsNotExist(err) {
			return status, nil
		}
		return PiSettingsStatus{}, fmt.Errorf("Failed to read Pi settings file: %w", err)
	}
	status.SettingsExists = true
	var settings PiSettings
	if err := json.Unmarshal(data, &settings); err != nil {
		return PiSettingsStatus{}, fmt.Errorf("Invalid Pi settings JSON: %w", err)
	}
	status.PiSettings = settings
	return status, nil
}

// SaveSettings writes the startup model selection into the live
// settings.json, keeping every other root key byte-for-byte compatible (the
// file is re-serialized, so key order and formatting are normalized the same
// way pi's own writes are).
func SaveSettings(settings PiSettings) error {
	thinking, err := normalizeThinkingLevel(settings.DefaultThinkingLevel)
	if err != nil {
		return err
	}
	path, err := piSettingsPath()
	if err != nil {
		return err
	}
	root := map[string]json.RawMessage{}
	if data, err := os.ReadFile(path); err == nil { // #nosec G304 -- tool config path
		if len(strings.TrimSpace(string(data))) > 0 {
			if err := json.Unmarshal(data, &root); err != nil {
				return fmt.Errorf("Invalid Pi settings JSON: %w", err)
			}
		}
	} else if !os.IsNotExist(err) {
		return fmt.Errorf("Failed to read Pi settings file: %w", err)
	}

	values := map[string]*string{
		"defaultProvider":      trimSetting(settings.DefaultProvider),
		"defaultModel":         trimSetting(settings.DefaultModel),
		"defaultThinkingLevel": thinking,
	}
	for _, key := range settingKeys {
		value := values[key]
		if value == nil {
			// Unset = pi picks automatically, so the key is removed rather
			// than written as "" or null (upstream validates both).
			delete(root, key)
			continue
		}
		encoded, err := json.Marshal(*value)
		if err != nil {
			return fmt.Errorf("Failed to serialize Pi setting %s: %w", key, err)
		}
		root[key] = encoded
	}

	out, err := json.MarshalIndent(root, "", "  ")
	if err != nil {
		return fmt.Errorf("Failed to serialize Pi settings: %w", err)
	}
	return core.AtomicWrite(path, append(out, '\n'))
}

// ClearSettings removes the startup model selection while keeping the rest of
// settings.json intact — the TUI's reset action and `dg pi settings reset`.
func ClearSettings() error {
	return SaveSettings(PiSettings{})
}
