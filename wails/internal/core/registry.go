package core

import (
	_ "embed"
	"encoding/json"
	"strings"
	"sync"
)

//go:embed registrydata/model-registry-data.json
var registryJSON []byte

// RegistryReasoningConfig mirrors the registry's `reasoningConfig` block.
type RegistryReasoningConfig struct {
	Efforts  []string          `json:"efforts,omitempty"`
	Profiles map[string]string `json:"profiles,omitempty"`
}

// RegistryModel is one entry of the DroidGear model registry
// (src/lib/model-registry-data.json).
type RegistryModel struct {
	ID               string                   `json:"id"`
	Name             string                   `json:"name"`
	Aliases          []string                 `json:"aliases"`
	Platform         string                   `json:"platform"`
	Reasoning        bool                     `json:"reasoning"`
	Input            []string                 `json:"input"`
	ThinkingLevelMap map[string]*string       `json:"thinkingLevelMap"`
	ContextWindow    uint32                   `json:"contextWindow"`
	MaxOutputTokens  *uint32                  `json:"maxOutputTokens"`
	ReasoningConfig  *RegistryReasoningConfig `json:"reasoningConfig"`
}

var (
	registryOnce   sync.Once
	registryModels []RegistryModel
)

func registry() []RegistryModel {
	registryOnce.Do(func() {
		if err := json.Unmarshal(registryJSON, &registryModels); err != nil {
			registryModels = nil
		}
	})
	return registryModels
}

// FindRegistryModel looks a model up by id or alias.
func FindRegistryModel(id string) *RegistryModel {
	for i := range registry() {
		entry := &registry()[i]
		if entry.ID == id {
			return entry
		}
		for _, alias := range entry.Aliases {
			if alias == id {
				return entry
			}
		}
	}
	return nil
}

// RequiresAdaptiveThinking reports whether the model's Anthropic encoding
// profile requires the adaptive thinking request format.
func (m *RegistryModel) RequiresAdaptiveThinking() bool {
	return m.ReasoningConfig != nil && m.ReasoningConfig.Profiles["anthropic"] == "anthropic-adaptive"
}

// RegistryReasoningEfforts builds Dsh `reasoningEfforts` for a registry
// entry: always starts with `off: null`, then maps each whitelisted effort
// from `reasoningConfig.efforts` to itself. Falls back to the registry's
// Pi-style thinkingLevelMap when no whitelist exists.
func RegistryReasoningEfforts(m *RegistryModel) map[string]*string {
	if m.ReasoningConfig != nil && len(m.ReasoningConfig.Efforts) > 0 {
		efforts := map[string]*string{"off": nil}
		for _, effort := range m.ReasoningConfig.Efforts {
			if effort == "none" || strings.TrimSpace(effort) == "" {
				continue
			}
			value := effort
			efforts[effort] = &value
		}
		return efforts
	}
	if m.ThinkingLevelMap == nil {
		return nil
	}
	efforts := make(map[string]*string, len(m.ThinkingLevelMap)+1)
	for level, target := range m.ThinkingLevelMap {
		efforts[level] = target
	}
	if _, ok := efforts["off"]; !ok {
		efforts["off"] = nil
	}
	return efforts
}

// InputModalities trims, lowercases, validates and deduplicates request
// modalities against the pi-ai whitelist (`text`, `image`).
func InputModalities(values []string) []string {
	var normalized []string
	for _, value := range values {
		value = strings.ToLower(strings.TrimSpace(value))
		if value != "text" && value != "image" {
			continue
		}
		duplicate := false
		for _, existing := range normalized {
			if existing == value {
				duplicate = true
				break
			}
		}
		if !duplicate {
			normalized = append(normalized, value)
		}
	}
	return normalized
}
