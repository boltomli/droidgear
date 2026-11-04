package tui

import (
	"context"
	"encoding/json"
	"fmt"

	"droidgear-wails/internal/service/channels"
)

// channelsSource adapts internal/service/channels — the terminal counterpart
// of the Channels view. Reading a channel's tokens needs the network
// (`TokensFor`), so the P1 pane stays on local data; fetching tokens joins
// with the write phase (tui-parity P2).
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
