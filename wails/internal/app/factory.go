package app

import (
	"encoding/json"
	"fmt"
	"os"
	"strings"

	"droidgear-wails/internal/core"
)

// ---------------------------------------------------------------------------
// Types (mirroring droidgear-core::factory_settings)
// ---------------------------------------------------------------------------

// CustomModel mirrors factory_settings::CustomModel (BYOK model entry stored
// in ~/.factory/settings.json).
type CustomModel struct {
	Model           string            `json:"model"`
	ID              *string           `json:"id"`
	Index           *uint32           `json:"index"`
	DisplayName     *string           `json:"displayName"`
	BaseURL         string            `json:"baseUrl"`
	APIKey          string            `json:"apiKey"`
	Provider        string            `json:"provider"`
	MaxOutputTokens *uint32           `json:"maxOutputTokens"`
	NoImageSupport  *bool             `json:"noImageSupport"`
	ExtraArgs       map[string]any    `json:"extraArgs"`
	ExtraHeaders    map[string]string `json:"extraHeaders"`
}

// SessionDefaultSettings mirrors factory_settings::SessionDefaultSettings.
type SessionDefaultSettings struct {
	Model                   *string `json:"model"`
	ReasoningEffort         *string `json:"reasoningEffort"`
	SpecModeModel           *string `json:"specModeModel"`
	SpecModeReasoningEffort *string `json:"specModeReasoningEffort"`
	AutonomyMode            *string `json:"autonomyMode"`
	AutonomyLevel           *string `json:"autonomyLevel"`
	InteractionMode         *string `json:"interactionMode"`
}

// ---------------------------------------------------------------------------
// Active settings file helpers
// ---------------------------------------------------------------------------

func readFactoryConfig() (map[string]any, error) {
	path, err := core.ActiveFactorySettingsPath()
	if err != nil {
		return nil, err
	}
	data, err := os.ReadFile(path) // #nosec G304 -- factory settings path
	if err != nil {
		if os.IsNotExist(err) || strings.TrimSpace(string(data)) == "" {
			return map[string]any{}, nil
		}
		return nil, fmt.Errorf("Failed to read config file: %w", err)
	}
	if strings.TrimSpace(string(data)) == "" {
		return map[string]any{}, nil
	}
	var config map[string]any
	if err := json.Unmarshal(data, &config); err != nil {
		return nil, fmt.Errorf("Failed to parse config JSON: %w", err)
	}
	if config == nil {
		config = map[string]any{}
	}
	return config, nil
}

func writeFactoryConfig(config map[string]any) error {
	path, err := core.ActiveFactorySettingsPath()
	if err != nil {
		return err
	}
	data, err := json.MarshalIndent(config, "", "  ")
	if err != nil {
		return fmt.Errorf("Failed to serialize config: %w", err)
	}
	return core.AtomicWrite(path, data)
}

func parseCustomModels(config map[string]any) []CustomModel {
	raw, ok := config["customModels"]
	if !ok {
		return []CustomModel{}
	}
	data, err := json.Marshal(raw)
	if err != nil {
		return []CustomModel{}
	}
	var models []CustomModel
	if err := json.Unmarshal(data, &models); err != nil {
		return []CustomModel{}
	}
	if models == nil {
		models = []CustomModel{}
	}
	return models
}

func canonicalCustomModelID(model CustomModel, index int) string {
	displayName := model.Model
	if model.DisplayName != nil && strings.TrimSpace(*model.DisplayName) != "" {
		displayName = *model.DisplayName
	}
	return fmt.Sprintf("custom:%s-%d", displayName, index)
}

func customModelIDs(models []CustomModel) map[string]bool {
	ids := map[string]bool{}
	for index, model := range models {
		if model.ID != nil && strings.HasPrefix(*model.ID, "custom:") && strings.TrimSpace(*model.ID) != "" {
			ids[*model.ID] = true
			continue
		}
		ids[canonicalCustomModelID(model, index)] = true
	}
	return ids
}

// normalizeModelFavorites trims, filters dangling `custom:` ids and
// deduplicates the favorites list (mirrors the Rust core).
func normalizeModelFavorites(favorites []string, models []CustomModel) []string {
	customIDs := customModelIDs(models)
	seen := map[string]bool{}
	normalized := []string{}
	for _, favorite := range favorites {
		favorite = strings.TrimSpace(favorite)
		if favorite == "" {
			continue
		}
		if strings.HasPrefix(favorite, "custom:") && !customIDs[favorite] {
			continue
		}
		if seen[favorite] {
			continue
		}
		seen[favorite] = true
		normalized = append(normalized, favorite)
	}
	return normalized
}

// ---------------------------------------------------------------------------
// Commands
// ---------------------------------------------------------------------------

// GetConfigPath returns the path of the active Factory settings file.
func (a *App) GetConfigPath() (string, error) {
	return core.ActiveFactorySettingsPath()
}

// ResetConfigFile replaces the active Factory settings file with an empty
// document.
func (a *App) ResetConfigFile() error {
	return writeFactoryConfig(map[string]any{})
}

// LoadCustomModels lists the BYOK custom models.
func (a *App) LoadCustomModels() ([]CustomModel, error) {
	config, err := readFactoryConfig()
	if err != nil {
		return []CustomModel{}, nil
	}
	return parseCustomModels(config), nil
}

// SaveCustomModels replaces the custom model list, re-normalizing favorites
// against the new list when the key exists.
func (a *App) SaveCustomModels(models []CustomModel) error {
	config, err := readFactoryConfig()
	if err != nil {
		return err
	}
	data, err := json.Marshal(models)
	if err != nil {
		return fmt.Errorf("Failed to serialize models: %w", err)
	}
	var modelsValue any
	_ = json.Unmarshal(data, &modelsValue)

	favorites := normalizeModelFavorites(readFavorites(config), models)
	config["customModels"] = modelsValue
	if _, exists := config["modelFavorites"]; exists {
		config["modelFavorites"] = favorites
	}
	return writeFactoryConfig(config)
}

func readFavorites(config map[string]any) []string {
	raw, ok := config["modelFavorites"]
	if !ok {
		return []string{}
	}
	data, err := json.Marshal(raw)
	if err != nil {
		return []string{}
	}
	var favorites []string
	if err := json.Unmarshal(data, &favorites); err != nil {
		return []string{}
	}
	return favorites
}

// GetModelFavorites lists the favorite model ids (normalized).
func (a *App) GetModelFavorites() ([]string, error) {
	config, err := readFactoryConfig()
	if err != nil {
		return []string{}, nil
	}
	return normalizeModelFavorites(readFavorites(config), parseCustomModels(config)), nil
}

// SaveModelFavorites stores the favorite model ids.
func (a *App) SaveModelFavorites(favorites []string) error {
	config, err := readFactoryConfig()
	if err != nil {
		return err
	}
	config["modelFavorites"] = normalizeModelFavorites(favorites, parseCustomModels(config))
	return writeFactoryConfig(config)
}

// GetSessionDefaultSettings reads sessionDefaultSettings.
func (a *App) GetSessionDefaultSettings() (SessionDefaultSettings, error) {
	config, err := readFactoryConfig()
	if err != nil {
		return SessionDefaultSettings{}, nil
	}
	raw, ok := config["sessionDefaultSettings"]
	if !ok {
		return SessionDefaultSettings{}, nil
	}
	data, err := json.Marshal(raw)
	if err != nil {
		return SessionDefaultSettings{}, nil
	}
	var settings SessionDefaultSettings
	if err := json.Unmarshal(data, &settings); err != nil {
		return SessionDefaultSettings{}, nil
	}
	return settings, nil
}

// SaveSessionDefaultSettings stores sessionDefaultSettings.
func (a *App) SaveSessionDefaultSettings(settings SessionDefaultSettings) error {
	config, err := readFactoryConfig()
	if err != nil {
		return err
	}
	data, err := json.Marshal(settings)
	if err != nil {
		return fmt.Errorf("Failed to serialize session default settings: %w", err)
	}
	var value any
	_ = json.Unmarshal(data, &value)
	config["sessionDefaultSettings"] = value
	return writeFactoryConfig(config)
}
