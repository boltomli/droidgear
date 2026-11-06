// Package importflow resolves a DroidGear channel into a provider import for
// Pi or OMP.
//
// It owns the network + credential resolution the GUI dialog does in React
// (token listing, model listing) and delegates the actual inference to
// service/{pi,omp}. `dg … providers import` and the TUI action both call
// this package, which keeps one resolution rule set for every non-GUI
// consumer (AGENTS rule 9).
package importflow

import (
	"context"
	"fmt"
	"strconv"
	"strings"

	"droidgear-wails/internal/service/channels"
	"droidgear-wails/internal/service/hermes"
	"droidgear-wails/internal/service/omp"
	"droidgear-wails/internal/service/pi"
)

// Resolved is the channel + token + model selection one import works from.
type Resolved struct {
	Channel      channels.Channel
	Token        channels.ChannelToken
	ModelIDs     []string
	DisplayNames map[string]*string
}

// Resolve loads the channel, picks the token (name, numeric id or the first
// one) and, when modelIDs is empty, lists the endpoint's models so the import
// can default to all of them.
func Resolve(ctx context.Context, channelID string, tokenRef string, modelIDs []string) (Resolved, error) {
	channel, err := loadChannel(ctx, channelID)
	if err != nil {
		return Resolved{}, err
	}
	tokens, err := channels.TokensFor(ctx, channel)
	if err != nil {
		return Resolved{}, err
	}
	if len(tokens) == 0 {
		return Resolved{}, fmt.Errorf("channel %q has no tokens", channel.ID)
	}
	token := tokens[0]
	if ref := strings.TrimSpace(tokenRef); ref != "" && ref != "-" {
		found := false
		for _, candidate := range tokens {
			if strings.EqualFold(candidate.Name, ref) || strconv.FormatFloat(candidate.ID, 'f', -1, 64) == ref {
				token = candidate
				found = true
				break
			}
		}
		if !found {
			return Resolved{}, fmt.Errorf("channel %q has no token %q", channel.ID, ref)
		}
	}
	resolved := Resolved{Channel: channel, Token: token, ModelIDs: modelIDs}
	if len(resolved.ModelIDs) == 0 {
		models, err := channels.FetchModelsByApiKey(ctx, channel.BaseURL, token.Key, token.Platform)
		if err != nil {
			return Resolved{}, err
		}
		resolved.DisplayNames = make(map[string]*string, len(models))
		for _, model := range models {
			resolved.ModelIDs = append(resolved.ModelIDs, model.ID)
			resolved.DisplayNames[model.ID] = model.Name
		}
	}
	return resolved, nil
}

func loadChannel(ctx context.Context, id string) (channels.Channel, error) {
	list, err := channels.Load(ctx)
	if err != nil {
		return channels.Channel{}, err
	}
	for _, channel := range list {
		if channel.ID == id {
			return channel, nil
		}
	}
	return channels.Channel{}, fmt.Errorf("channel %q not found", id)
}

// ImportPi writes the resolved channel into the live ~/.pi/agent/models.json.
func ImportPi(ctx context.Context, channelID string, tokenRef string, modelIDs []string, providerID string) (pi.PiChannelImportResult, error) {
	resolved, err := Resolve(ctx, channelID, tokenRef, modelIDs)
	if err != nil {
		return pi.PiChannelImportResult{}, err
	}
	current, err := pi.ReadCurrentConfig()
	if err != nil {
		return pi.PiChannelImportResult{}, err
	}
	existingIDs := make([]string, 0, len(current.Providers))
	for id := range current.Providers {
		existingIDs = append(existingIDs, id)
	}
	input := pi.PiChannelImportInput{
		ChannelName:         resolved.Channel.Name,
		ChannelType:         resolved.Channel.Type,
		BaseURL:             resolved.Channel.BaseURL,
		Platform:            resolved.Token.Platform,
		APIKey:              resolved.Token.Key,
		TokenName:           resolved.Token.Name,
		ProviderID:          providerID,
		ModelIDs:            resolved.ModelIDs,
		ExistingProviderIDs: existingIDs,
	}
	for id, name := range resolved.DisplayNames {
		input.ChannelModels = append(input.ChannelModels, pi.PiChannelModel{ID: id, DisplayName: name})
	}
	prepared, err := pi.PrepareChannelImport(input)
	if err != nil {
		return pi.PiChannelImportResult{}, err
	}
	if existing, ok := current.Providers[prepared.ProviderID]; ok {
		input.Existing = &existing
	}
	return pi.ImportChannelProvider(input)
}

// ImportOmp writes the resolved channel into the live ~/.omp/agent/models.yml.
func ImportOmp(ctx context.Context, channelID string, tokenRef string, modelIDs []string, providerID string) (omp.OmpChannelImportResult, error) {
	resolved, err := Resolve(ctx, channelID, tokenRef, modelIDs)
	if err != nil {
		return omp.OmpChannelImportResult{}, err
	}
	list, err := omp.ReadCustomProviders()
	if err != nil {
		return omp.OmpChannelImportResult{}, err
	}
	existingIDs := make([]string, 0, len(list.Providers))
	for _, entry := range list.Providers {
		existingIDs = append(existingIDs, entry.ProviderID)
	}
	input := omp.OmpChannelImportInput{
		ChannelName:         resolved.Channel.Name,
		ChannelType:         resolved.Channel.Type,
		BaseURL:             resolved.Channel.BaseURL,
		Platform:            resolved.Token.Platform,
		APIKey:              resolved.Token.Key,
		TokenName:           resolved.Token.Name,
		ProviderID:          providerID,
		ModelIDs:            resolved.ModelIDs,
		ExistingProviderIDs: existingIDs,
	}
	for id, name := range resolved.DisplayNames {
		input.ChannelModels = append(input.ChannelModels, omp.OmpChannelModel{ID: id, DisplayName: name})
	}
	prepared, err := omp.PrepareChannelImport(input)
	if err != nil {
		return omp.OmpChannelImportResult{}, err
	}
	for _, entry := range list.Providers {
		if entry.ProviderID == prepared.ProviderID {
			existing := entry.Config
			input.Existing = &existing
			break
		}
	}
	return omp.ImportChannelProvider(input)
}

// ImportHermes writes the resolved channel into the live ~/.hermes/config.yaml.
// Hermes uses one model per provider; modelID picks it, otherwise the first
// model the endpoint lists is used.
func ImportHermes(ctx context.Context, channelID string, tokenRef string, modelID string) (hermes.HermesChannelImportResult, error) {
	requested := []string{}
	if strings.TrimSpace(modelID) != "" {
		requested = []string{modelID}
	}
	resolved, err := Resolve(ctx, channelID, tokenRef, requested)
	if err != nil {
		return hermes.HermesChannelImportResult{}, err
	}
	chosen := strings.TrimSpace(modelID)
	if chosen == "" && len(resolved.ModelIDs) > 0 {
		chosen = resolved.ModelIDs[0]
	}
	return hermes.ImportChannelProvider(hermes.HermesChannelImportInput{
		ChannelName:  resolved.Channel.Name,
		ChannelType:  resolved.Channel.Type,
		BaseURL:      resolved.Channel.BaseURL,
		APIKey:       resolved.Token.Key,
		TokenName:    resolved.Token.Name,
		DefaultModel: chosen,
	})
}
