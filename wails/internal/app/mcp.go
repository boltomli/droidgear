// MCP server configuration (~/.factory/mcp.json). Mirrors
// droidgear-core::mcp and src-tauri/src/commands/mcp.rs.
package app

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"sort"
	"strings"

	"droidgear-wails/internal/core"
)

// McpServerType mirrors mcp::McpServerType ("stdio" | "http").
type McpServerType string

// MCP server type values (serde rename_all = "lowercase").
const (
	McpServerTypeStdio McpServerType = "stdio"
	McpServerTypeHTTP  McpServerType = "http"
)

// McpServerConfig mirrors mcp::McpServerConfig. Optional fields are pointers
// so absent values are omitted on write exactly like Rust's
// skip_serializing_if = "Option::is_none"; `disabled` always serializes.
type McpServerConfig struct {
	Type     McpServerType      `json:"type"`
	Disabled bool               `json:"disabled"`
	Command  *string            `json:"command,omitempty"`
	Args     *[]string          `json:"args,omitempty"`
	Env      *map[string]string `json:"env,omitempty"`
	URL      *string            `json:"url,omitempty"`
	Headers  *map[string]string `json:"headers,omitempty"`
}

// UnmarshalJSON reproduces serde's enum rejection: an unknown `type` makes
// the whole config fail to parse, which load_mcp_servers filters out (and
// which rejects a bad `save_mcp_server` argument before it reaches the file).
func (c *McpServerConfig) UnmarshalJSON(data []byte) error {
	type plain McpServerConfig
	var p plain
	if err := json.Unmarshal(data, &p); err != nil {
		return err
	}
	switch p.Type {
	case McpServerTypeStdio, McpServerTypeHTTP:
	default:
		return fmt.Errorf("unknown variant `%s`, expected `http` or `stdio`", p.Type)
	}
	*c = McpServerConfig(p)
	return nil
}

// McpServer mirrors mcp::McpServer (bindings.ts).
type McpServer struct {
	Name   string          `json:"name"`
	Config McpServerConfig `json:"config"`
}

// ---------------------------------------------------------------------------
// File I/O
// ---------------------------------------------------------------------------

func mcpConfigPathForHome(home string) (string, error) {
	factoryHome := droidFactoryHomeForHome(home)
	if err := core.EnsureDir(factoryHome); err != nil {
		return "", fmt.Errorf("Failed to create .factory directory: %w", err)
	}
	return filepath.Join(factoryHome, "mcp.json"), nil
}

func readMcpFileForHome(home string) (any, error) {
	path, err := mcpConfigPathForHome(home)
	if err != nil {
		return nil, err
	}
	if !droidPathExists(path) {
		return map[string]any{"mcpServers": map[string]any{}}, nil
	}
	data, err := os.ReadFile(path) // #nosec G304 -- factory home path
	if err != nil {
		return nil, fmt.Errorf("Failed to read MCP config file: %w", err)
	}
	if strings.TrimSpace(string(data)) == "" {
		return map[string]any{"mcpServers": map[string]any{}}, nil
	}
	config, err := droidDecodeJSONValue(data)
	if err != nil {
		return nil, fmt.Errorf("Failed to parse MCP config JSON: %w", err)
	}
	return config, nil
}

func writeMcpFileForHome(home string, config any) error {
	path, err := mcpConfigPathForHome(home)
	if err != nil {
		return err
	}
	actualPath := path
	if droidIsSymlink(path) {
		resolved, err := filepath.EvalSymlinks(path)
		if err != nil {
			return fmt.Errorf("Failed to resolve symlink: %w", err)
		}
		actualPath = resolved
	}
	data, err := droidMarshalPretty(config)
	if err != nil {
		return fmt.Errorf("Failed to serialize MCP config: %w", err)
	}
	tempPath := droidWithExtension(actualPath, "tmp")           // mcp.json → mcp.tmp
	if err := os.WriteFile(tempPath, data, 0o644); err != nil { // #nosec G306 -- config files
		return fmt.Errorf("Failed to write MCP config file: %w", err)
	}
	if err := os.Rename(tempPath, actualPath); err != nil {
		_ = os.Remove(tempPath)
		return fmt.Errorf("Failed to finalize MCP config file: %w", err)
	}
	return nil
}

// mcpServersObject returns the mcpServers map, mirroring the Rust entry()
// + as_object_mut() dance (a missing key is created as {}; a present
// non-object value is left untouched).
func mcpServersObject(config any) (map[string]any, bool) {
	obj, ok := config.(map[string]any)
	if !ok {
		return nil, false
	}
	inner, exists := obj["mcpServers"]
	if !exists {
		inner = map[string]any{}
		obj["mcpServers"] = inner
	}
	servers, ok := inner.(map[string]any)
	return servers, ok
}

// ---------------------------------------------------------------------------
// Commands
// ---------------------------------------------------------------------------

func loadMcpServersForHome(home string) ([]McpServer, error) {
	config, err := readMcpFileForHome(home)
	if err != nil {
		return nil, err
	}
	servers := []McpServer{}
	obj, ok := config.(map[string]any)
	if !ok {
		return servers, nil
	}
	entries, ok := obj["mcpServers"].(map[string]any)
	if !ok {
		return servers, nil
	}
	// serde_json's default Map is a BTreeMap: names come out sorted.
	names := make([]string, 0, len(entries))
	for name := range entries {
		names = append(names, name)
	}
	sort.Strings(names)

	for _, name := range names {
		data, err := json.Marshal(entries[name])
		if err != nil {
			continue
		}
		var config McpServerConfig
		if err := json.Unmarshal(data, &config); err != nil {
			continue // filter_map(|…| serde_json::from_value(…).ok())
		}
		servers = append(servers, McpServer{Name: name, Config: config})
	}
	return servers, nil
}

// mcpBaizhiPreset mirrors mcp::baizhi_mcp_preset: a fixed, disabled HTTP
// reference that never stores a raw API key.
func mcpBaizhiPreset() McpServer {
	url := "https://agent-toolkit.app.baizhi.cloud/mcp"
	headers := map[string]string{"Authorization": "Bearer ${BAIZHI_API_KEY}"}
	return McpServer{
		Name: "baizhi",
		Config: McpServerConfig{
			Type:     McpServerTypeHTTP,
			Disabled: true,
			URL:      &url,
			Headers:  &headers,
		},
	}
}

func saveBaizhiPresetForHome(home string) error {
	config, err := readMcpFileForHome(home)
	if err != nil {
		return err
	}
	obj, ok := config.(map[string]any)
	if !ok {
		return errString("MCP config must be a JSON object")
	}
	inner, exists := obj["mcpServers"]
	if !exists {
		inner = map[string]any{}
		obj["mcpServers"] = inner
	}
	servers, ok := inner.(map[string]any)
	if !ok {
		return errString("mcpServers must be a JSON object")
	}
	if _, exists := servers["baizhi"]; exists {
		return errString("MCP server 'baizhi' already exists; not replaced")
	}
	value, err := core.ToMap(mcpBaizhiPreset().Config)
	if err != nil {
		return fmt.Errorf("Failed to serialize Baizhi preset: %w", err)
	}
	// Brand-only raw option outside the generic configuration type.
	value["oauth"] = false
	servers["baizhi"] = value
	return writeMcpFileForHome(home, config)
}

func saveMcpServerForHome(home string, server McpServer) error {
	config, err := readMcpFileForHome(home)
	if err != nil {
		return err
	}
	serverValue, err := core.ToMap(server.Config)
	if err != nil {
		return fmt.Errorf("Failed to serialize server config: %w", err)
	}
	if servers, ok := mcpServersObject(config); ok {
		servers[server.Name] = serverValue
	}
	return writeMcpFileForHome(home, config)
}

func deleteMcpServerForHome(home string, name string) error {
	config, err := readMcpFileForHome(home)
	if err != nil {
		return err
	}
	if obj, ok := config.(map[string]any); ok {
		if servers, ok := obj["mcpServers"].(map[string]any); ok {
			delete(servers, name)
		}
	}
	return writeMcpFileForHome(home, config)
}

func toggleMcpServerForHome(home string, name string, disabled bool) error {
	config, err := readMcpFileForHome(home)
	if err != nil {
		return err
	}
	if obj, ok := config.(map[string]any); ok {
		if servers, ok := obj["mcpServers"].(map[string]any); ok {
			if server, exists := servers[name]; exists {
				if serverObj, ok := server.(map[string]any); ok {
					serverObj["disabled"] = disabled
				}
			} else {
				return fmt.Errorf("Server not found: %s", name)
			}
		}
	}
	return writeMcpFileForHome(home, config)
}

// LoadMcpServers lists all MCP servers from ~/.factory/mcp.json.
func (a *App) LoadMcpServers() ([]McpServer, error) {
	home, err := droidHomeDir()
	if err != nil {
		return []McpServer{}, err
	}
	return loadMcpServersForHome(home)
}

// SaveMcpServer creates or updates one MCP server entry.
func (a *App) SaveMcpServer(server McpServer) error {
	home, err := droidHomeDir()
	if err != nil {
		return err
	}
	return saveMcpServerForHome(home, server)
}

// SaveBaizhiPreset stores the fixed disabled Baizhi preset (never replaces an
// existing `baizhi` record).
func (a *App) SaveBaizhiPreset() error {
	home, err := droidHomeDir()
	if err != nil {
		return err
	}
	return saveBaizhiPresetForHome(home)
}

// DeleteMcpServer removes an MCP server by name.
func (a *App) DeleteMcpServer(name string) error {
	home, err := droidHomeDir()
	if err != nil {
		return err
	}
	return deleteMcpServerForHome(home, name)
}

// ToggleMcpServer sets an MCP server's disabled flag.
func (a *App) ToggleMcpServer(name string, disabled bool) error {
	home, err := droidHomeDir()
	if err != nil {
		return err
	}
	return toggleMcpServerForHome(home, name, disabled)
}
