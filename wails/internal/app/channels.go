package app

import (
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
// Commands
// ---------------------------------------------------------------------------

// LoadChannels reads ~/.droidgear/channels.json, falling back to the legacy
// Factory settings `channels` array for migration (mirrors the Rust core).
func (a *App) LoadChannels() ([]Channel, error) {
	path, err := channelsPath()
	if err != nil {
		return nil, err
	}
	if fileExists(path) {
		data, err := os.ReadFile(path) // #nosec G304 -- channels file
		if err != nil {
			return nil, fmt.Errorf("Failed to read channels file: %w", err)
		}
		var channels []Channel
		if err := json.Unmarshal(data, &channels); err != nil {
			return nil, fmt.Errorf("Failed to parse channels file: %w", err)
		}
		return channels, nil
	}

	// Migration: ~/.factory/settings.json → channels array
	settings := core.ReadJSONObjectFile(filepath.Join(core.FactoryHome(), "settings.json"))
	if raw, ok := settings["channels"]; ok {
		data, err := json.Marshal(raw)
		if err == nil {
			var channels []Channel
			if json.Unmarshal(data, &channels) == nil && len(channels) > 0 {
				if out, err := json.MarshalIndent(channels, "", "  "); err == nil {
					_ = core.AtomicWrite(path, out)
				}
				return channels, nil
			}
		}
	}
	return []Channel{}, nil
}

// SaveChannels writes the channel list atomically.
func (a *App) SaveChannels(channels []Channel) error {
	path, err := channelsPath()
	if err != nil {
		return err
	}
	data, err := json.MarshalIndent(channels, "", "  ")
	if err != nil {
		return fmt.Errorf("Failed to serialize channels: %w", err)
	}
	return core.AtomicWrite(path, data)
}

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

// SaveChannelApiKey stores an api_key auth entry for a channel.
func (a *App) SaveChannelApiKey(channelID string, apiKey string) error {
	return writeChannelAuth(channelID, channelAuth{Type: "api_key", APIKey: apiKey})
}

// GetChannelApiKey returns the stored api key (null when absent).
func (a *App) GetChannelApiKey(channelID string) (*string, error) {
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

// SaveChannelCredentials stores username/password auth for a channel.
func (a *App) SaveChannelCredentials(channelID string, username string, password string) error {
	return writeChannelAuth(channelID, channelAuth{
		Type: "credentials", Username: username, Password: password,
	})
}

// GetChannelCredentials returns [username, password] (null when absent).
func (a *App) GetChannelCredentials(channelID string) (*[2]string, error) {
	auth, err := readChannelAuth(channelID)
	if err != nil {
		return nil, err
	}
	if auth == nil || auth.Type != "credentials" {
		return nil, nil
	}
	return &[2]string{auth.Username, auth.Password}, nil
}

// DeleteChannelCredentials removes a channel's auth file.
func (a *App) DeleteChannelCredentials(channelID string) error {
	path, err := channelAuthPath(channelID)
	if err != nil {
		return err
	}
	if fileExists(path) {
		return os.Remove(path)
	}
	return nil
}
