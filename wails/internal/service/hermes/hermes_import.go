package hermes

import (
	"errors"
	"fmt"
	"os"
	"strings"

	"droidgear-wails/internal/core"
	"droidgear-wails/internal/service/channelimport"

	"gopkg.in/yaml.v3"
)

// ---------------------------------------------------------------------------
// Channel import + live-provider management (shared by the GUI dialog,
// `dg hermes providers …` and the TUI).
// ---------------------------------------------------------------------------

// HermesChannelImportInput is the resolved channel/token pair plus the model
// the user picked. Hermes always writes a `custom` provider and appends /v1.
type HermesChannelImportInput struct {
	ChannelName  string `json:"channelName"`
	ChannelType  string `json:"channelType"`
	BaseURL      string `json:"baseUrl"`
	APIKey       string `json:"apiKey"`
	TokenName    string `json:"tokenName,omitempty"`
	DefaultModel string `json:"defaultModel,omitempty"`
}

// HermesChannelImportResult is the editable model entry for the import.
type HermesChannelImportResult struct {
	BaseURL      string  `json:"baseUrl"`
	APIKey       string  `json:"apiKey"`
	Provider     string  `json:"provider"`
	Name         string  `json:"name"`
	DefaultModel *string `json:"defaultModel,omitempty"`
}

// PrepareChannelImport builds the Hermes model entry for a channel import.
// It is pure: no disk or network access.
func PrepareChannelImport(input HermesChannelImportInput) (HermesChannelImportResult, error) {
	apiKey := strings.TrimSpace(input.APIKey)
	if apiKey == "" {
		return HermesChannelImportResult{}, errors.New("channel import needs an API key")
	}
	if strings.TrimSpace(input.ChannelName) == "" {
		return HermesChannelImportResult{}, errors.New("channel import needs a channel name")
	}
	result := HermesChannelImportResult{
		BaseURL:  channelimport.NormalizeBaseURL(input.BaseURL, "/v1"),
		APIKey:   apiKey,
		Provider: "custom",
		Name:     strings.TrimSpace(input.ChannelName),
	}
	if model := strings.TrimSpace(input.DefaultModel); model != "" {
		result.DefaultModel = &model
	}
	return result, nil
}

// ImportChannelProvider writes the imported provider into the live
// config.yaml. It reuses applyHermesProfileToConfig so the v12 upsert,
// legacy migration and unknown-key preservation stay in one place.
func ImportChannelProvider(input HermesChannelImportInput) (HermesChannelImportResult, error) {
	result, err := PrepareChannelImport(input)
	if err != nil {
		return HermesChannelImportResult{}, err
	}
	configPath, err := hermesConfigPath()
	if err != nil {
		return HermesChannelImportResult{}, err
	}
	profile := HermesProfile{
		Models: []HermesModelConfig{{
			Name:      &result.Name,
			Default:   result.DefaultModel,
			Provider:  &result.Provider,
			BaseURL:   &result.BaseURL,
			APIKey:    &result.APIKey,
			IsDefault: true,
		}},
	}
	if err := applyHermesProfileToConfig(profile, configPath); err != nil {
		return HermesChannelImportResult{}, err
	}
	return result, nil
}

// DeleteLiveProvider removes one named custom provider from the live
// config.yaml, keeping the rest of the document intact. Deleting the active
// provider is refused (switch the model first), because the model section
// would point at a provider that no longer exists.
func DeleteLiveProvider(name string) error {
	name = strings.TrimSpace(name)
	if name == "" {
		return errors.New("provider name cannot be empty")
	}
	configPath, err := hermesConfigPath()
	if err != nil {
		return err
	}
	data, err := os.ReadFile(configPath) // #nosec G304 -- tool config path
	if err != nil {
		if os.IsNotExist(err) {
			return fmt.Errorf("Provider %q not found", name)
		}
		return fmt.Errorf("Failed to read config.yaml: %w", err)
	}
	root, err := core.ParseYAML(string(data))
	if err != nil {
		return fmt.Errorf("Failed to parse config.yaml: %w", err)
	}
	if !core.IsMapping(root) {
		return errors.New("config.yaml root must be a YAML mapping")
	}
	providers := core.MappingGetMap(root, "providers")
	if providers == nil || core.MappingGet(providers, name) == nil {
		return fmt.Errorf("Provider %q not found", name)
	}
	if modelSection := core.MappingGetMap(root, "model"); modelSection != nil {
		if provider := core.MappingGetString(modelSection, "provider"); provider == "custom:"+name {
			return errors.New("provider is active; switch the model before deleting it")
		}
	}
	core.MappingRemove(providers, name)
	if list := core.MappingGetSeq(root, "custom_providers"); list != nil {
		kept := list.Content[:0]
		for _, item := range list.Content {
			if item.Kind == yaml.MappingNode && core.MappingGetString(item, "name") == name {
				continue
			}
			kept = append(kept, item)
		}
		if len(kept) == 0 {
			core.MappingRemove(root, "custom_providers")
		} else {
			list.Content = kept
		}
	}
	text, err := core.MarshalYAML(root)
	if err != nil {
		return fmt.Errorf("Failed to serialize config.yaml: %w", err)
	}
	return core.AtomicWrite(configPath, core.BytesOf(text))
}
