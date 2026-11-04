package tui

import (
	"context"
	"encoding/json"
	"fmt"

	"droidgear-wails/internal/service/mcp"
)

// mcpSource adapts internal/service/mcp.
//
// The GUI nests MCP under Droid; the TUI exposes it as its own module until
// the Droid service package lands (tui-parity §2), so the screen does not
// have to wait for the rest of Droid's sub-views.
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

func (mcpSource) Summary(rows []ListRow) string {
	enabled := 0
	for _, row := range rows {
		if row.Mark == "● " {
			enabled++
		}
	}
	return fmt.Sprintf("MCP · %d servers · %d enabled · r toggle secrets", len(rows), enabled)
}
