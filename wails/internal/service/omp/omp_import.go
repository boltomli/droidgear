package omp

import (
	"errors"
	"strings"

	"droidgear-wails/internal/core"
	"droidgear-wails/internal/service/channelimport"
)

// ---------------------------------------------------------------------------
// Channel import (shared by the GUI dialog, `dg omp providers import` and the
// TUI's OMP → Providers action). Inference used to live in
// src/components/omp/channel-import.ts, which left `dg`/TUI unable to import
// a channel (AGENTS rule 9).
// ---------------------------------------------------------------------------

// OmpChannelModel is one model listed by a channel token.
type OmpChannelModel struct {
	ID          string  `json:"id"`
	DisplayName *string `json:"name,omitempty"`
}

// OmpChannelImportInput is the resolved channel/token pair plus the models the
// caller selected. Channel/model discovery stays in the caller so this
// package never talks to the network.
type OmpChannelImportInput struct {
	ChannelName         string             `json:"channelName"`
	ChannelType         string             `json:"channelType"`
	BaseURL             string             `json:"baseUrl"`
	Platform            *string            `json:"platform,omitempty"`
	APIKey              string             `json:"apiKey"`
	TokenName           string             `json:"tokenName,omitempty"`
	ProviderID          string             `json:"providerId,omitempty"`
	ModelIDs            []string           `json:"modelIds"`
	ChannelModels       []OmpChannelModel  `json:"channelModels,omitempty"`
	ExistingProviderIDs []string           `json:"existingProviderIds,omitempty"`
	Existing            *OmpCustomProvider `json:"existing,omitempty"`
}

// OmpChannelImportResult is the provider entry the caller saves.
type OmpChannelImportResult struct {
	ProviderID string            `json:"providerId"`
	Provider   OmpCustomProvider `json:"provider"`
}

// PrepareChannelImport builds the OMP `models.yml` provider entry for a
// channel import. It is pure: no disk or network access.
func PrepareChannelImport(input OmpChannelImportInput) (OmpChannelImportResult, error) {
	apiKey := strings.TrimSpace(input.APIKey)
	if apiKey == "" {
		return OmpChannelImportResult{}, errors.New("channel import needs an API key")
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
		return OmpChannelImportResult{}, errors.New("select at least one model to import")
	}
	providerID := strings.TrimSpace(input.ProviderID)
	if providerID == "" {
		providerID = channelimport.SuggestProviderID(input.ChannelName, input.TokenName, input.ExistingProviderIDs)
	}
	displayNames := make(map[string]*string, len(input.ChannelModels))
	for _, model := range input.ChannelModels {
		displayNames[model.ID] = model.DisplayName
	}
	imported := make([]OmpCustomModel, 0, len(ids))
	for _, id := range ids {
		imported = append(imported, ompModelFromChannel(id, displayNames[id]))
	}

	provider := OmpCustomProvider{}
	if input.Existing != nil {
		provider = *input.Existing
	}
	baseURL := channelimport.NormalizeBaseURLForChannel(input.ChannelType, input.BaseURL, input.Platform)
	api := channelimport.InferAPI(input.Platform, ids[0])
	provider.BaseURL = &baseURL
	provider.API = &api
	provider.APIKey = &apiKey
	if provider.Auth == nil || strings.TrimSpace(*provider.Auth) == "" {
		auth := "apiKey"
		provider.Auth = &auth
	}
	provider.Models = mergeOmpModels(provider.Models, imported)
	if err := ValidateCustomProvider(provider); err != nil {
		return OmpChannelImportResult{}, err
	}
	return OmpChannelImportResult{ProviderID: providerID, Provider: provider}, nil
}

// ompModelFromChannel enriches a channel model from the vendored registry,
// mirroring src/components/omp/channel-import.ts.
func ompModelFromChannel(id string, displayName *string) OmpCustomModel {
	model := OmpCustomModel{ID: id}
	entry := core.FindRegistryModel(id)
	name := trimmedPtr(displayName)
	if entry != nil {
		if value := strings.TrimSpace(entry.Name); value != "" {
			name = &value
		}
	}
	if name != nil {
		model.Name = name
	}
	reasoning := false
	if entry != nil {
		reasoning = entry.Reasoning
		if len(entry.Input) > 0 {
			input := append([]string(nil), entry.Input...)
			model.Input = &input
		}
	}
	model.Reasoning = &reasoning
	contextWindow := channelimport.DefaultContextWindow
	if entry != nil && entry.ContextWindow != 0 {
		contextWindow = entry.ContextWindow
	}
	maxTokens := channelimport.DefaultMaxTokens
	if entry != nil && entry.MaxOutputTokens != nil && *entry.MaxOutputTokens != 0 {
		maxTokens = *entry.MaxOutputTokens
	}
	model.ContextWindow = &contextWindow
	model.MaxTokens = &maxTokens
	return model
}

func trimmedPtr(value *string) *string {
	if value == nil {
		return nil
	}
	trimmed := strings.TrimSpace(*value)
	if trimmed == "" {
		return nil
	}
	return &trimmed
}

// mergeOmpModels keeps existing models (and their unknown fields) and folds
// imported specs over them by id, mirroring channel-import.ts.
func mergeOmpModels(existing []OmpCustomModel, imported []OmpCustomModel) []OmpCustomModel {
	byID := make(map[string]OmpCustomModel, len(existing))
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
		byID[model.ID] = mergeOmpModel(current, model)
	}
	out := make([]OmpCustomModel, 0, len(order))
	for _, id := range order {
		out = append(out, byID[id])
	}
	return out
}

func mergeOmpModel(existing OmpCustomModel, imported OmpCustomModel) OmpCustomModel {
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
	if imported.Reasoning != nil {
		out.Reasoning = imported.Reasoning
	}
	if imported.Thinking != nil {
		out.Thinking = imported.Thinking
	}
	if imported.Input != nil {
		out.Input = imported.Input
	}
	if imported.ImageInputDecoder != nil {
		out.ImageInputDecoder = imported.ImageInputDecoder
	}
	if imported.Cost != nil {
		out.Cost = imported.Cost
	}
	if imported.ContextWindow != nil {
		out.ContextWindow = imported.ContextWindow
	}
	if imported.MaxContextWindow != nil {
		out.MaxContextWindow = imported.MaxContextWindow
	}
	if imported.MaxTokens != nil {
		out.MaxTokens = imported.MaxTokens
	}
	if imported.OmitMaxOutputTokens != nil {
		out.OmitMaxOutputTokens = imported.OmitMaxOutputTokens
	}
	if imported.PreferWebsockets != nil {
		out.PreferWebsockets = imported.PreferWebsockets
	}
	if imported.Headers != nil {
		out.Headers = imported.Headers
	}
	if imported.Compat != nil {
		out.Compat = imported.Compat
	}
	if imported.RemoteCompaction != nil {
		out.RemoteCompaction = imported.RemoteCompaction
	}
	return out
}

// ImportChannelProvider builds and writes one imported provider into the live
// models.yml through SaveCustomProvider (which also repairs null-valued
// files and merges unknown keys already on disk).
func ImportChannelProvider(input OmpChannelImportInput) (OmpChannelImportResult, error) {
	result, err := PrepareChannelImport(input)
	if err != nil {
		return OmpChannelImportResult{}, err
	}
	if err := SaveCustomProvider(result.ProviderID, result.Provider); err != nil {
		return OmpChannelImportResult{}, err
	}
	return result, nil
}

// mergeOmpModelRoles applies a profile's known roles over the live
// config.yml map, keeping roles this build does not model (stored in Extra)
// so upstream additions survive an apply. A nil profile field clears the
// role, matching the GUI's "empty input unassigns" behaviour.
func mergeOmpModelRoles(existing *OmpModelRoles, profile OmpModelRoles) OmpModelRoles {
	out := OmpModelRoles{}
	if existing != nil {
		out = *existing
	}
	out.Default = profile.Default
	out.Smol = profile.Smol
	out.Slow = profile.Slow
	out.Vision = profile.Vision
	out.Plan = profile.Plan
	out.Commit = profile.Commit
	out.Tiny = profile.Tiny
	out.Memory = profile.Memory
	out.Task = profile.Task
	out.Advisor = profile.Advisor
	out.Image = profile.Image
	out.Web = profile.Web
	out.Speech = profile.Speech
	out.Dictation = profile.Dictation
	out.Judge = profile.Judge
	return out
}
