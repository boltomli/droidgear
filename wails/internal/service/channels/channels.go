// Package channels owns the DroidGear channel store: ~/.droidgear/channels.json
// plus the per-channel credential files under ~/.droidgear/auth/. The GUI
// (internal/app), the `dg channels` CLI subcommands and the future Go TUI all
// go through this package — it is the single writer of those files.
//
// Network-facing helpers (token listing, model discovery, type detection)
// live in fetch.go; they are part of the same domain because the CLI has to
// run them without a GUI process.
package channels

import (
	"context"
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"

	"droidgear-wails/internal/core"
)

// ---------------------------------------------------------------------------
// Types (mirroring droidgear-core::channel)
// ---------------------------------------------------------------------------

// Channel mirrors channel::Channel.
type Channel struct {
	ID        string  `json:"id"`
	Name      string  `json:"name"`
	Type      string  `json:"type"`
	BaseURL   string  `json:"baseUrl"`
	Enabled   bool    `json:"enabled"`
	CreatedAt float64 `json:"createdAt"`
}

// channelAuth mirrors channel::ChannelAuth (tagged enum):
// {"type":"credentials","username":..,"password":..} or
// {"type":"api_key","api_key":..}.
type channelAuth struct {
	Type     string `json:"type"`
	Username string `json:"username,omitempty"`
	Password string `json:"password,omitempty"`
	APIKey   string `json:"api_key,omitempty"`
}

// ---------------------------------------------------------------------------
// Paths
// ---------------------------------------------------------------------------

func channelsPath() (string, error) {
	dir, err := core.DroidGearDir()
	if err != nil {
		return "", err
	}
	if err := core.EnsureDir(dir); err != nil {
		return "", fmt.Errorf("Failed to create .droidgear directory: %w", err)
	}
	return filepath.Join(dir, "channels.json"), nil
}

func channelAuthDir() (string, error) {
	dir, err := core.DroidGearDir()
	if err != nil {
		return "", err
	}
	return filepath.Join(dir, "auth"), nil
}

func channelAuthPath(channelID string) (string, error) {
	dir, err := channelAuthDir()
	if err != nil {
		return "", err
	}
	return filepath.Join(dir, channelID+".json"), nil
}

// ---------------------------------------------------------------------------
// Channel list
// ---------------------------------------------------------------------------

// Load reads ~/.droidgear/channels.json, falling back to the legacy Factory
// settings `channels` array for migration (mirrors the Rust core).
func Load(ctx context.Context) ([]Channel, error) {
	path, err := channelsPath()
	if err != nil {
		return nil, err
	}
	if core.FileExists(path) {
		data, err := os.ReadFile(path) // #nosec G304 -- channels file
		if err != nil {
			return nil, fmt.Errorf("Failed to read channels file: %w", err)
		}
		var list []Channel
		if err := json.Unmarshal(data, &list); err != nil {
			return nil, fmt.Errorf("Failed to parse channels file: %w", err)
		}
		return list, nil
	}

	// Migration: ~/.factory/settings.json → channels array
	settings := core.ReadJSONObjectFile(filepath.Join(core.FactoryHome(), "settings.json"))
	if raw, ok := settings["channels"]; ok {
		data, err := json.Marshal(raw)
		if err == nil {
			var list []Channel
			if json.Unmarshal(data, &list) == nil && len(list) > 0 {
				if out, err := json.MarshalIndent(list, "", "  "); err == nil {
					_ = core.AtomicWrite(path, out)
				}
				return list, nil
			}
		}
	}
	return []Channel{}, nil
}

// Save writes the channel list atomically.
func Save(ctx context.Context, list []Channel) error {
	path, err := channelsPath()
	if err != nil {
		return err
	}
	data, err := json.MarshalIndent(list, "", "  ")
	if err != nil {
		return fmt.Errorf("Failed to serialize channels: %w", err)
	}
	return core.AtomicWrite(path, data)
}

// ---------------------------------------------------------------------------
// Credentials / API keys
// ---------------------------------------------------------------------------

func readChannelAuth(channelID string) (*channelAuth, error) {
	path, err := channelAuthPath(channelID)
	if err != nil {
		return nil, err
	}
	data, err := os.ReadFile(path) // #nosec G304 -- auth file
	if err != nil {
		if os.IsNotExist(err) {
			return nil, nil
		}
		return nil, fmt.Errorf("Failed to read auth file: %w", err)
	}
	var auth channelAuth
	if err := json.Unmarshal(data, &auth); err != nil {
		return nil, fmt.Errorf("Failed to parse auth file: %w", err)
	}
	return &auth, nil
}

func writeChannelAuth(channelID string, auth channelAuth) error {
	dir, err := channelAuthDir()
	if err != nil {
		return err
	}
	if err := core.EnsureDir(dir); err != nil {
		return fmt.Errorf("Failed to create auth directory: %w", err)
	}
	path, err := channelAuthPath(channelID)
	if err != nil {
		return err
	}
	data, err := json.MarshalIndent(auth, "", "  ")
	if err != nil {
		return fmt.Errorf("Failed to serialize auth: %w", err)
	}
	return core.AtomicWrite(path, data)
}

// SaveApiKey stores an api_key auth entry for a channel.
func SaveApiKey(ctx context.Context, channelID string, apiKey string) error {
	return writeChannelAuth(channelID, channelAuth{Type: "api_key", APIKey: apiKey})
}

// ApiKey returns the stored api key (nil when absent or of another type).
func ApiKey(ctx context.Context, channelID string) (*string, error) {
	auth, err := readChannelAuth(channelID)
	if err != nil {
		return nil, err
	}
	if auth == nil || auth.Type != "api_key" {
		return nil, nil
	}
	key := auth.APIKey
	return &key, nil
}

// SaveCredentials stores username/password auth for a channel.
func SaveCredentials(ctx context.Context, channelID string, username string, password string) error {
	return writeChannelAuth(channelID, channelAuth{
		Type: "credentials", Username: username, Password: password,
	})
}

// Credentials returns [username, password] (nil when absent).
func Credentials(ctx context.Context, channelID string) (*[2]string, error) {
	auth, err := readChannelAuth(channelID)
	if err != nil {
		return nil, err
	}
	if auth == nil || auth.Type != "credentials" {
		return nil, nil
	}
	return &[2]string{auth.Username, auth.Password}, nil
}

// DeleteCredentials removes a channel's auth file.
func DeleteCredentials(ctx context.Context, channelID string) error {
	path, err := channelAuthPath(channelID)
	if err != nil {
		return err
	}
	if core.FileExists(path) {
		return os.Remove(path)
	}
	return nil
}

// TokensFor returns the token list for a channel: credential channels list
// tokens via login, API-key channels yield one synthetic token carrying the
// key (mirrors channel_export::fetch_tokens_for_channel). Failures talking
// to the provider are marked with ErrUpstream.
func TokensFor(ctx context.Context, channel Channel) ([]ChannelToken, error) {
	if channel.Type == "new-api" || channel.Type == "sub-2-api" {
		creds, err := Credentials(ctx, channel.ID)
		if err != nil {
			return nil, err
		}
		if creds == nil {
			return nil, fmt.Errorf("No credentials for channel '%s'", channel.Name)
		}
		return FetchTokens(ctx, channel.Type, channel.BaseURL, creds[0], creds[1])
	}
	apiKey, err := ApiKey(ctx, channel.ID)
	if err != nil {
		return nil, err
	}
	if apiKey == nil {
		return nil, fmt.Errorf("No API key for channel '%s'", channel.Name)
	}
	return []ChannelToken{{
		ID:             0,
		Name:           "API Key",
		Key:            *apiKey,
		Status:         1,
		UnlimitedQuota: true,
	}}, nil
}
