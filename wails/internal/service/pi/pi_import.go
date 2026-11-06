package pi

import (
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"strings"

	"droidgear-wails/internal/core"
	"droidgear-wails/internal/service/channelimport"
)

// ---------------------------------------------------------------------------
// Channel import (shared by the GUI dialog, `dg pi providers import` and the
// TUI's Pi → Providers action). The GUI used to do this inference in React,
// which left `dg`/TUI unable to import a channel (AGENTS rule 9).
// ---------------------------------------------------------------------------

// PiChannelModel is one model listed by a channel token. DisplayName is the
// name the channel returned; the vendored registry wins when it knows the id.
type PiChannelModel struct {
	ID          string  `json:"id"`
	DisplayName *string `json:"name,omitempty"`
}

// PiChannelImportInput is the resolved channel/token pair plus the models the
// caller selected. Channel/model discovery stays in the caller (GUI command
// or CLI) so this package never talks to the network.
type PiChannelImportInput struct {
	ChannelName         string            `json:"channelName"`
	ChannelType         string            `json:"channelType"`
	BaseURL             string            `json:"baseUrl"`
	Platform            *string           `json:"platform,omitempty"`
	APIKey              string            `json:"apiKey"`
	TokenName           string            `json:"tokenName,omitempty"`
	ProviderID          string            `json:"providerId,omitempty"`
	ModelIDs            []string          `json:"modelIds"`
	ChannelModels       []PiChannelModel  `json:"channelModels,omitempty"`
	ExistingProviderIDs []string          `json:"existingProviderIds,omitempty"`
	Existing            *PiProviderConfig `json:"existing,omitempty"`
}

// PiChannelImportResult is what the caller writes: a provider id suggestion,
// the normalised endpoint, the inferred protocol and the enriched models.
type PiChannelImportResult struct {
	ProviderID string    `json:"providerId"`
	BaseURL    string    `json:"baseUrl"`
	API        string    `json:"api"`
	Models     []PiModel `json:"models"`
}

// PrepareChannelImport builds the Pi provider entry for a channel import.
// It is pure: no disk or network access.
func PrepareChannelImport(input PiChannelImportInput) (PiChannelImportResult, error) {
	apiKey := strings.TrimSpace(input.APIKey)
	if apiKey == "" {
		return PiChannelImportResult{}, errors.New("channel import needs an API key")
	}
	ids := channelimport.SanitizeModelIDs(input.ModelIDs)
	if len(ids) == 0 {
		for _, model := range input.ChannelModels {
			ids = append(ids, strings.TrimSpace(model.ID))
		}
		ids = channelimport.SanitizeModelIDs(ids)
	}
	if len(ids) == 0 && input.Existing != nil {
		// A re-import with no selected models keeps the provider's current
		// models and only refreshes credentials/protocol.
		for _, model := range input.Existing.Models {
			ids = append(ids, model.ID)
		}
		ids = channelimport.SanitizeModelIDs(ids)
	}
	if len(ids) == 0 {
		return PiChannelImportResult{}, errors.New("select at least one model to import")
	}
	providerID := strings.TrimSpace(input.ProviderID)
	if providerID == "" {
		providerID = channelimport.SuggestProviderID(input.ChannelName, input.TokenName, input.ExistingProviderIDs)
	}
	displayNames := make(map[string]*string, len(input.ChannelModels))
	for _, model := range input.ChannelModels {
		displayNames[model.ID] = model.DisplayName
	}

	imported := make([]PiModel, 0, len(ids))
	for _, id := range ids {
		imported = append(imported, piModelFromChannel(id, displayNames[id]))
	}
	if input.Existing != nil {
		imported = mergePiModels(input.Existing.Models, imported)
	}
	return PiChannelImportResult{
		ProviderID: providerID,
		BaseURL:    channelimport.NormalizeBaseURLForChannel(input.ChannelType, input.BaseURL, input.Platform),
		API:        channelimport.InferAPI(input.Platform, ids[0]),
		Models:     imported,
	}, nil
}

// piModelFromChannel enriches a channel model from the vendored registry,
// mirroring src/lib/pi-model-metadata.ts (the previous GUI-only logic).
func piModelFromChannel(id string, displayName *string) PiModel {
	model := PiModel{
		ID:            id,
		Input:         []string{"text"},
		ContextWindow: channelimport.DefaultContextWindow,
		MaxTokens:     channelimport.DefaultMaxTokens,
	}
	entry := core.FindRegistryModel(id)
	if entry == nil {
		if name := trimmed(displayName); name != "" {
			model.Name = &name
		}
		return model
	}
	if name := strings.TrimSpace(entry.Name); name != "" {
		model.Name = &name
	}
	model.Reasoning = entry.Reasoning
	if len(entry.Input) > 0 {
		model.Input = append([]string(nil), entry.Input...)
	}
	if entry.ThinkingLevelMap != nil {
		model.ThinkingLevelMap = entry.ThinkingLevelMap
	}
	if entry.ContextWindow != 0 {
		model.ContextWindow = entry.ContextWindow
	}
	if entry.MaxOutputTokens != nil && *entry.MaxOutputTokens != 0 {
		model.MaxTokens = *entry.MaxOutputTokens
	}
	if entry.RequiresAdaptiveThinking() {
		model.Compat = &PiCompatConfig{ForceAdaptiveThinking: piPtr(true)}
	}
	return model
}

func trimmed(value *string) string {
	if value == nil {
		return ""
	}
	return strings.TrimSpace(*value)
}

func piPtr[T any](value T) *T { return &value }

// mergePiModels keeps the existing provider's models and folds the imported
// specs over them by id (a re-import refreshes specs without dropping fields
// the GUI does not model).
func mergePiModels(existing []PiModel, imported []PiModel) []PiModel {
	byID := make(map[string]PiModel, len(existing))
	order := make([]string, 0, len(existing)+len(imported))
	for _, model := range existing {
		if _, ok := byID[model.ID]; !ok {
			order = append(order, model.ID)
		}
		byID[model.ID] = model
	}
	for _, model := range imported {
		current, ok := byID[model.ID]
		if !ok {
			order = append(order, model.ID)
		}
		byID[model.ID] = mergePiModel(current, model)
	}
	out := make([]PiModel, 0, len(order))
	for _, id := range order {
		out = append(out, byID[id])
	}
	return out
}

func mergePiModel(existing PiModel, imported PiModel) PiModel {
	out := existing
	out.ID = imported.ID
	if imported.Name != nil {
		out.Name = imported.Name
	}
	if imported.API != nil {
		out.API = imported.API
	}
	if imported.BaseURL != nil {
		out.BaseURL = imported.BaseURL
	}
	out.Reasoning = imported.Reasoning
	if len(imported.Input) > 0 {
		out.Input = imported.Input
	}
	if imported.ThinkingLevelMap != nil {
		out.ThinkingLevelMap = imported.ThinkingLevelMap
	}
	if imported.ContextWindow != 0 {
		out.ContextWindow = imported.ContextWindow
	}
	if imported.MaxTokens != 0 {
		out.MaxTokens = imported.MaxTokens
	}
	out.Compat = mergePiCompat(out.Compat, imported.Compat)
	return out
}

// mergePiCompat overlays the imported compat only where the existing compat
// leaves a field unset (an explicit user choice always wins).
func mergePiCompat(existing *PiCompatConfig, imported *PiCompatConfig) *PiCompatConfig {
	if imported == nil {
		return existing
	}
	if existing == nil {
		return imported
	}
	mergeBool := func(dst **bool, src *bool) {
		if *dst == nil && src != nil {
			*dst = src
		}
	}
	mergeString := func(dst **string, src *string) {
		if *dst == nil && src != nil {
			*dst = src
		}
	}
	mergeBool(&existing.SupportsStore, imported.SupportsStore)
	mergeBool(&existing.SupportsDeveloperRole, imported.SupportsDeveloperRole)
	mergeBool(&existing.SupportsReasoningEffort, imported.SupportsReasoningEffort)
	mergeBool(&existing.SupportsUsageInStreaming, imported.SupportsUsageInStreaming)
	mergeBool(&existing.RequiresToolResultName, imported.RequiresToolResultName)
	mergeBool(&existing.RequiresAssistantAfterToolResult, imported.RequiresAssistantAfterToolResult)
	mergeBool(&existing.RequiresThinkingAsText, imported.RequiresThinkingAsText)
	mergeBool(&existing.RequiresReasoningContentOnAssistantMessages, imported.RequiresReasoningContentOnAssistantMessages)
	mergeBool(&existing.SupportsStrictMode, imported.SupportsStrictMode)
	mergeBool(&existing.SupportsLongCacheRetention, imported.SupportsLongCacheRetention)
	mergeBool(&existing.SupportsEagerToolInputStreaming, imported.SupportsEagerToolInputStreaming)
	mergeBool(&existing.ForceAdaptiveThinking, imported.ForceAdaptiveThinking)
	mergeString(&existing.MaxTokensField, imported.MaxTokensField)
	mergeString(&existing.ThinkingFormat, imported.ThinkingFormat)
	mergeString(&existing.CacheControlFormat, imported.CacheControlFormat)
	if existing.Extra == nil {
		existing.Extra = imported.Extra
	} else {
		for key, value := range imported.Extra {
			if _, ok := existing.Extra[key]; !ok {
				existing.Extra[key] = value
			}
		}
	}
	return existing
}

// ---------------------------------------------------------------------------
// Lossless apply: merge the profile over the live models.json instead of
// rebuilding it, so upstream fields this build does not model (and the
// optional `$schema` root key) survive an apply.
// ---------------------------------------------------------------------------

// mergePiConfig merges profile providers into the live models.json document.
// Unknown root keys are kept; providers not in the profile are removed (apply
// is the profile's desired state) but matching providers keep their unknown
// keys and model-level extras. keepOthers preserves providers that are not in
// the profile, which is what a single-provider import wants.
func mergePiConfig(live []byte, providers map[string]PiProviderConfig, keepOthers bool) ([]byte, error) {
	root := map[string]json.RawMessage{}
	if len(strings.TrimSpace(string(live))) > 0 {
		if err := json.Unmarshal(live, &root); err != nil {
			return nil, fmt.Errorf("Invalid Pi config JSON: %w", err)
		}
	}
	liveProviders := map[string]PiProviderConfig{}
	if raw, ok := root["providers"]; ok {
		if err := json.Unmarshal(raw, &liveProviders); err != nil {
			return nil, fmt.Errorf("Invalid Pi providers object: %w", err)
		}
	}
	merged := make(map[string]PiProviderConfig, len(providers))
	if keepOthers {
		for id, provider := range liveProviders {
			merged[id] = provider
		}
	}
	for id, provider := range providers {
		merged[id] = mergePiProvider(liveProviders[id], provider)
	}
	raw, err := json.Marshal(merged)
	if err != nil {
		return nil, fmt.Errorf("Failed to serialize Pi providers: %w", err)
	}
	root["providers"] = raw
	data, err := json.MarshalIndent(root, "", "  ")
	if err != nil {
		return nil, fmt.Errorf("Failed to serialize Pi config: %w", err)
	}
	return data, nil
}

func mergePiProvider(existing PiProviderConfig, incoming PiProviderConfig) PiProviderConfig {
	out := incoming
	// Unknown provider-level keys survive; known fields come from the profile.
	if out.Extra == nil {
		out.Extra = map[string]json.RawMessage{}
	}
	for key, value := range existing.Extra {
		if _, ok := out.Extra[key]; !ok {
			out.Extra[key] = value
		}
	}
	if out.OAuth == nil {
		out.OAuth = existing.OAuth
	}
	if out.Headers == nil {
		out.Headers = existing.Headers
	}
	if out.AuthHeader == nil {
		out.AuthHeader = existing.AuthHeader
	}
	if out.Compat == nil {
		out.Compat = existing.Compat
	}
	out.Models = mergePiModels(existing.Models, out.Models)
	out.ModelOverrides = mergePiModelOverrides(existing.ModelOverrides, out.ModelOverrides)
	return out
}

func mergePiModelOverrides(existing map[string]PiModelOverride, incoming map[string]PiModelOverride) map[string]PiModelOverride {
	if len(incoming) == 0 {
		return incoming
	}
	for id, override := range incoming {
		previous, ok := existing[id]
		if !ok {
			continue
		}
		if override.Extra == nil {
			override.Extra = map[string]json.RawMessage{}
		}
		for key, value := range previous.Extra {
			if _, exists := override.Extra[key]; !exists {
				override.Extra[key] = value
			}
		}
		incoming[id] = override
	}
	return incoming
}

// readLiveConfigForMerge returns the current models.json bytes (empty when
// the file does not exist).
func readLiveConfigForMerge(path string) ([]byte, error) {
	data, err := os.ReadFile(path) // #nosec G304 -- tool config path
	if err != nil {
		if os.IsNotExist(err) {
			return nil, nil
		}
		return nil, fmt.Errorf("Failed to read Pi config file: %w", err)
	}
	return data, nil
}

// ImportChannelProvider writes one imported provider into the live
// models.json, keeping every other provider and unknown key. It is the
// single write path behind the GUI import dialog, `dg pi providers import`
// and the TUI action.
func ImportChannelProvider(input PiChannelImportInput) (PiChannelImportResult, error) {
	result, err := PrepareChannelImport(input)
	if err != nil {
		return PiChannelImportResult{}, err
	}
	configPath, err := piConfigPath()
	if err != nil {
		return PiChannelImportResult{}, err
	}
	live, err := readLiveConfigForMerge(configPath)
	if err != nil {
		return PiChannelImportResult{}, err
	}
	baseURL, api, apiKey := result.BaseURL, result.API, input.APIKey
	provider := PiProviderConfig{
		BaseURL: &baseURL,
		API:     &api,
		APIKey:  &apiKey,
		Models:  result.Models,
	}
	if input.Existing != nil {
		provider = mergePiProvider(*input.Existing, provider)
	}
	data, err := mergePiConfig(live, map[string]PiProviderConfig{result.ProviderID: provider}, true)
	if err != nil {
		return PiChannelImportResult{}, err
	}
	if err := core.AtomicWrite(configPath, data); err != nil {
		return PiChannelImportResult{}, err
	}
	return result, nil
}

// DeleteLiveProvider removes one provider from the live models.json while
// keeping every other provider and unknown root key, then repairs the startup
// model the deletion stranded. It is the counterpart of ImportChannelProvider
// for `dg pi providers delete` and the TUI.
func DeleteLiveProvider(providerID string) (PiApplyResult, error) {
	providerID = strings.TrimSpace(providerID)
	if providerID == "" {
		return PiApplyResult{}, errors.New("provider id cannot be empty")
	}
	configPath, err := piConfigPath()
	if err != nil {
		return PiApplyResult{}, err
	}
	live, err := readLiveConfigForMerge(configPath)
	if err != nil {
		return PiApplyResult{}, err
	}
	root := map[string]json.RawMessage{}
	if len(strings.TrimSpace(string(live))) > 0 {
		if err := json.Unmarshal(live, &root); err != nil {
			return PiApplyResult{}, fmt.Errorf("Invalid Pi config JSON: %w", err)
		}
	}
	providers := map[string]PiProviderConfig{}
	if raw, ok := root["providers"]; ok {
		if err := json.Unmarshal(raw, &providers); err != nil {
			return PiApplyResult{}, fmt.Errorf("Invalid Pi providers object: %w", err)
		}
	}
	if _, ok := providers[providerID]; !ok {
		return PiApplyResult{}, fmt.Errorf("Provider %q not found", providerID)
	}
	before := make(map[string]PiProviderConfig, len(providers))
	for id, provider := range providers {
		before[id] = provider
	}
	delete(providers, providerID)
	raw, err := json.Marshal(providers)
	if err != nil {
		return PiApplyResult{}, err
	}
	root["providers"] = raw
	data, err := json.MarshalIndent(root, "", "  ")
	if err != nil {
		return PiApplyResult{}, err
	}
	if err := core.AtomicWrite(configPath, data); err != nil {
		return PiApplyResult{}, err
	}
	result, err := ReconcileStartupModel(before, providers)
	if err != nil {
		return result, err
	}
	result.AppliedProviders = sortedProviderIDs(providers)
	return result, nil
}
