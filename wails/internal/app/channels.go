// Channel commands: the channel list, its stored credentials and the
// network helpers behind the Channels pane. The logic lives in
// internal/service/channels (design doc §2 R2); this file only marshals
// arguments and returns the same JSON the bindings already expose.
package app

import (
	"context"

	// The service is imported under an alias so the method parameters can
	// keep their original names: the generated bindings (and therefore the
	// frontend shim) match on parameter names — scripts/check-command-surface.mjs
	// fails the build when they drift.
	svc "droidgear-wails/internal/service/channels"
)

// Channel mirrors channel::Channel.
type Channel = svc.Channel

// ChannelToken mirrors channel::ChannelToken.
type ChannelToken = svc.ChannelToken

// ModelInfo mirrors factory_settings::ModelInfo.
type ModelInfo = svc.ModelInfo

// LoadChannels reads ~/.droidgear/channels.json (with Factory migration).
func (a *App) LoadChannels() ([]Channel, error) {
	return svc.Load(context.Background())
}

// SaveChannels writes the channel list atomically.
func (a *App) SaveChannels(channels []Channel) error {
	return svc.Save(context.Background(), channels)
}

// SaveChannelApiKey stores an api_key auth entry for a channel.
func (a *App) SaveChannelApiKey(channelID string, apiKey string) error {
	return svc.SaveApiKey(context.Background(), channelID, apiKey)
}

// GetChannelApiKey returns the stored api key (null when absent).
func (a *App) GetChannelApiKey(channelID string) (*string, error) {
	return svc.ApiKey(context.Background(), channelID)
}

// SaveChannelCredentials stores username/password auth for a channel.
func (a *App) SaveChannelCredentials(channelID string, username string, password string) error {
	return svc.SaveCredentials(context.Background(), channelID, username, password)
}

// GetChannelCredentials returns [username, password] (null when absent).
func (a *App) GetChannelCredentials(channelID string) (*[2]string, error) {
	return svc.Credentials(context.Background(), channelID)
}

// DeleteChannelCredentials removes a channel's auth file.
func (a *App) DeleteChannelCredentials(channelID string) error {
	return svc.DeleteCredentials(context.Background(), channelID)
}

// DetectChannelType probes well-known endpoints to classify a channel.
func (a *App) DetectChannelType(baseUrl string) (string, error) {
	return svc.DetectType(context.Background(), baseUrl)
}

// FetchChannelTokens logs in to a channel and lists its tokens.
func (a *App) FetchChannelTokens(channelType string, baseUrl string, username string, password string) ([]ChannelToken, error) {
	return svc.FetchTokens(context.Background(), channelType, baseUrl, username, password)
}

// FetchModelsByApiKey probes the platform-specific model endpoints.
func (a *App) FetchModelsByApiKey(baseUrl string, apiKey string, platform *string) ([]ModelInfo, error) {
	return svc.FetchModelsByApiKey(context.Background(), baseUrl, apiKey, platform)
}
