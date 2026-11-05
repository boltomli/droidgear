package tui

import (
	"context"
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"

	"droidgear-wails/internal/service/mcp"
)

// mcpSource adapts internal/service/mcp — the MCP section of the Droid pane
// (the GUI nests MCP under Droid, tui-parity §2).
type mcpSource struct{}

// McpSource exposes the MCP servers to the generic screen.
func McpSource() ListSource { return mcpSource{} }

func (mcpSource) Title() string { return "MCP" }

func (mcpSource) Load(context.Context) ([]ListRow, error) {
	servers, err := mcp.LoadServers()
	if err != nil {
		return nil, err
	}
	rows := make([]ListRow, 0, len(servers))
	for _, server := range servers {
		mark := "● "
		if server.Config.Disabled {
			mark = "○ "
		}
		rows = append(rows, ListRow{ID: server.Name, Label: server.Name, Mark: mark})
	}
	return rows, nil
}

func (mcpSource) Detail(_ context.Context, name string) ([]byte, error) {
	servers, err := mcp.LoadServers()
	if err != nil {
		return nil, err
	}
	for _, server := range servers {
		if server.Name != name {
			continue
		}
		return json.MarshalIndent(server, "", "  ")
	}
	return nil, fmt.Errorf("MCP server %q not found", name)
}

// Actions wires 't' = flip the server's disabled flag and 'b' = the fixed
// Baizhi preset (same contract as the GUI's quick-add card: stored disabled,
// never carries a key).
func (mcpSource) Actions() []ListAction {
	return []ListAction{
		{
			Key: "t", Label: "toggle",
			Run: func(_ context.Context, row ListRow) error {
				servers, err := mcp.LoadServers()
				if err != nil {
					return err
				}
				for _, server := range servers {
					if server.Name == row.ID {
						return mcp.ToggleServer(row.ID, !server.Config.Disabled)
					}
				}
				return fmt.Errorf("MCP server %q not found", row.ID)
			},
		},
		{
			Key: "b", Label: "baizhi preset",
			Run: func(context.Context, ListRow) error { return mcp.SaveBaizhiPreset() },
		},
	}
}

// InputActions wires 's' = import a server document from a JSON file (the
// TUI twin of `dg mcp save --file`).
func (mcpSource) InputActions() []InputAction {
	return []InputAction{{
		Key: "s", Label: "save from file",
		Hint: "path to an McpServer JSON document (same shape as dg mcp save --file)",
		Submit: func(_ context.Context, row ListRow, value string) writeAction {
			return writeAction{
				title: "save MCP server from " + value,
				run: func() error {
					raw, err := os.ReadFile(filepath.Clean(value)) // #nosec G304 -- user-supplied server file
					if err != nil {
						return fmt.Errorf("read server file: %w", err)
					}
					var server mcp.McpServer
					if err := json.Unmarshal(raw, &server); err != nil {
						return fmt.Errorf("invalid MCP server JSON: %w", err)
					}
					return mcp.SaveServer(server)
				},
			}
		},
	}}
}

func (mcpSource) Summary(rows []ListRow) string {
	enabled := 0
	for _, row := range rows {
		if row.Mark == "● " {
			enabled++
		}
	}
	return fmt.Sprintf("MCP · %d servers · %d enabled · r toggle secrets", len(rows), enabled)
}
