package tui

import (
	"context"
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"time"

	"droidgear-wails/internal/core"
	"droidgear-wails/internal/service/channels"
)

// channelsSource adapts internal/service/channels — the terminal counterpart
// of the Channels view. 'f' fetches the channel's tokens (`TokensFor`, the
// network path the GUI's token dialog uses) and shows them in a reveal-able
// view screen; 'x' clears the stored credentials through the write preview.
type channelsSource struct{}

// ChannelsSource exposes the channel list to the generic screen.
func ChannelsSource() ListSource { return channelsSource{} }

func (channelsSource) Title() string { return "Channels" }

func (channelsSource) Load(ctx context.Context) ([]ListRow, error) {
	list, err := channels.Load(ctx)
	if err != nil {
		return nil, err
	}
	rows := make([]ListRow, 0, len(list))
	for _, channel := range list {
		mark := "  "
		if channel.Enabled {
			mark = "✓ "
		}
		label := channel.Name
		if label == "" {
			label = channel.ID
		}
		rows = append(rows, ListRow{ID: channel.ID, Label: label, Mark: mark})
	}
	return rows, nil
}

func (channelsSource) Detail(ctx context.Context, id string) ([]byte, error) {
	list, err := channels.Load(ctx)
	if err != nil {
		return nil, err
	}
	for _, channel := range list {
		if channel.ID != id {
			continue
		}
		return json.MarshalIndent(channel, "", "  ")
	}
	return nil, fmt.Errorf("channel %q not found", id)
}

// Actions wires 'x' = clear the channel's stored credentials.
func (channelsSource) Actions() []ListAction {
	return []ListAction{{
		Key: "x", Label: "clear credentials",
		Run: func(ctx context.Context, row ListRow) error {
			return channels.DeleteCredentials(ctx, row.ID)
		},
	}}
}

// ViewActions wires 'f' = fetch and display the channel's tokens. The
// masking mirrors `dg channels tokens`: the token key field is not a
// key/value pair the generic redactor recognises, so it is masked here.
func (channelsSource) ViewActions() []ViewAction {
	return []ViewAction{{
		Key: "f", Label: "tokens",
		Run: func(ctx context.Context, row ListRow) (string, []byte, []byte, error) {
			list, err := channels.Load(ctx)
			if err != nil {
				return "", nil, nil, err
			}
			var channel *channels.Channel
			for i := range list {
				if list[i].ID == row.ID {
					channel = &list[i]
					break
				}
			}
			if channel == nil {
				return "", nil, nil, fmt.Errorf("channel %q not found", row.ID)
			}
			tokens, err := channels.TokensFor(ctx, *channel)
			if err != nil {
				return "", nil, nil, err
			}
			full, err := json.MarshalIndent(tokens, "", "  ")
			if err != nil {
				return "", nil, nil, err
			}
			maskedTokens := make([]channels.ChannelToken, len(tokens))
			for i, token := range tokens {
				token.Key = core.RedactSecret(token.Key)
				maskedTokens[i] = token
			}
			masked, err := json.MarshalIndent(maskedTokens, "", "  ")
			if err != nil {
				return "", nil, nil, err
			}
			return "Tokens · " + row.Label, masked, full, nil
		},
	}}
}

// InputActions wire the credential and channel writes: 'k' stores an API
// key, 'p' stores username/password ("user pass", split on the first
// space), 'n' creates/updates a channel from a JSON document.
func (channelsSource) InputActions() []InputAction {
	return []InputAction{
		{
			Key: "k", Label: "set api key",
			Hint: "provider API key (stored under ~/.droidgear/auth)",
			Submit: func(ctx context.Context, row ListRow, value string) writeAction {
				return writeAction{
					title: "set api key for " + row.Label,
					run:   func() error { return channels.SaveApiKey(ctx, row.ID, value) },
				}
			},
		},
		{
			Key: "p", Label: "set credentials",
			Hint: "username password - split on the first space",
			Submit: func(ctx context.Context, row ListRow, value string) writeAction {
				return writeAction{
					title: "set credentials for " + row.Label,
					run: func() error {
						user, pass := splitFirstSpace(value)
						if user == "" || pass == "" {
							return fmt.Errorf("expected: username password")
						}
						return channels.SaveCredentials(ctx, row.ID, user, pass)
					},
				}
			},
		},
		{
			// 'n' = create or update a channel from a JSON document (the TUI
			// twin of the GUI dialog; an existing id is replaced).
			Key: "n", Label: "new from file",
			Hint: "path to a channel JSON document (id/name/type/baseUrl); an existing id is replaced",
			Submit: func(ctx context.Context, row ListRow, value string) writeAction {
				return writeAction{
					title: "save channel from " + value,
					run: func() error {
						raw, err := os.ReadFile(filepath.Clean(value)) // #nosec G304 -- user-supplied channel file
						if err != nil {
							return fmt.Errorf("read channel file: %w", err)
						}
						var channel channels.Channel
						if err := json.Unmarshal(raw, &channel); err != nil {
							return fmt.Errorf("invalid channel JSON: %w", err)
						}
						if strings.TrimSpace(channel.ID) == "" {
							return fmt.Errorf("channel needs a non-empty id")
						}
						list, err := channels.Load(ctx)
						if err != nil {
							return err
						}
						replaced := false
						for i := range list {
							if list[i].ID == channel.ID {
								if channel.CreatedAt == 0 {
									channel.CreatedAt = list[i].CreatedAt
								}
								list[i] = channel
								replaced = true
								break
							}
						}
						if !replaced {
							if channel.CreatedAt == 0 {
								channel.CreatedAt = float64(time.Now().UnixMilli())
							}
							list = append(list, channel)
						}
						return channels.Save(ctx, list)
					},
				}
			},
		},
	}
}

func (channelsSource) Summary(rows []ListRow) string {
	enabled := 0
	for _, row := range rows {
		if row.Mark == "✓ " {
			enabled++
		}
	}
	return fmt.Sprintf(
		"Channels · %d total · %d enabled · credentials are not shown here",
		len(rows), enabled,
	)
}
