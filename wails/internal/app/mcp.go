// MCP server configuration — the GUI-facing forward.
//
// The logic lives in internal/service/mcp (single write path for the GUI, the
// `dg mcp` subcommands and the TUI); this file only marshals arguments, so
// the generated bindings keep their exact method names and parameter order.
package app

import (
	"droidgear-wails/internal/service/mcp"
)

// LoadMcpServers lists every configured MCP server.
func (a *App) LoadMcpServers() ([]mcp.McpServer, error) {
	return mcp.LoadServers()
}

// SaveMcpServer creates or updates one MCP server entry.
func (a *App) SaveMcpServer(server mcp.McpServer) error {
	return mcp.SaveServer(server)
}

// SaveBaizhiPreset stores the fixed disabled Baizhi preset (never replaces an
// existing `baizhi` record).
func (a *App) SaveBaizhiPreset() error {
	return mcp.SaveBaizhiPreset()
}

// DeleteMcpServer removes an MCP server by name.
func (a *App) DeleteMcpServer(name string) error {
	return mcp.DeleteServer(name)
}

// ToggleMcpServer sets an MCP server's disabled flag.
func (a *App) ToggleMcpServer(name string, disabled bool) error {
	return mcp.ToggleServer(name, disabled)
}
