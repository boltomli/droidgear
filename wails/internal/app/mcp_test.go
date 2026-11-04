package app

import (
	"droidgear-wails/internal/core"
	"encoding/json"
	"os"
	"path/filepath"
	"testing"
)

func mcpReadConfig(t *testing.T, home string) map[string]any {
	t.Helper()
	data, err := os.ReadFile(filepath.Join(home, ".factory", "mcp.json")) // #nosec G304 -- test fixture
	if err != nil {
		t.Fatalf("read mcp.json: %v", err)
	}
	value, err := core.DecodeJSONValue(data)
	if err != nil {
		t.Fatalf("parse mcp.json: %v", err)
	}
	obj, ok := value.(map[string]any)
	if !ok {
		t.Fatalf("mcp.json is not an object: %T", value)
	}
	return obj
}

func TestMcpLoadReturnsEmptyWhenFileMissing(t *testing.T) {
	home := t.TempDir()
	servers, err := loadMcpServersForHome(home)
	if err != nil {
		t.Fatalf("loadMcpServersForHome: %v", err)
	}
	if len(servers) != 0 {
		t.Fatalf("expected no servers, got %d", len(servers))
	}
	// The Rust core creates .factory on every mcp path resolution.
	if info, err := os.Stat(filepath.Join(home, ".factory")); err != nil || !info.IsDir() {
		t.Fatalf("factory dir not created: %v", err)
	}
}

func TestMcpSaveLoadRoundTripPreservesUnknownFields(t *testing.T) {
	home := t.TempDir()
	// Unknown top-level keys survive read-modify-write cycles.
	if err := os.MkdirAll(filepath.Join(home, ".factory"), 0o755); err != nil {
		t.Fatalf("mkdir: %v", err)
	}
	seed := `{"mcpServers":{},"futureTopLevel":{"keep":true}}`
	if err := os.WriteFile(filepath.Join(home, ".factory", "mcp.json"), []byte(seed), 0o644); err != nil {
		t.Fatalf("seed: %v", err)
	}

	command := "npx"
	args := []string{"-y", "acme-mcp"}
	env := map[string]string{"ACME_TOKEN": "secret"}
	if err := saveMcpServerForHome(home, McpServer{
		Name: "acme",
		Config: McpServerConfig{
			Type:     McpServerTypeStdio,
			Disabled: false,
			Command:  &command,
			Args:     &args,
			Env:      &env,
		},
	}); err != nil {
		t.Fatalf("saveMcpServerForHome: %v", err)
	}

	servers, err := loadMcpServersForHome(home)
	if err != nil {
		t.Fatalf("loadMcpServersForHome: %v", err)
	}
	if len(servers) != 1 || servers[0].Name != "acme" {
		t.Fatalf("unexpected servers: %+v", servers)
	}
	cfg := servers[0].Config
	if cfg.Type != McpServerTypeStdio || cfg.Command == nil || *cfg.Command != "npx" {
		t.Fatalf("config lost fields: %+v", cfg)
	}
	if cfg.Args == nil || len(*cfg.Args) != 2 || (*cfg.Args)[0] != "-y" {
		t.Fatalf("args lost: %+v", cfg.Args)
	}
	if cfg.Disabled {
		t.Error("expected enabled server")
	}

	raw := mcpReadConfig(t, home)
	keep, ok := raw["futureTopLevel"].(map[string]any)
	if !ok || keep["keep"] != true {
		t.Fatalf("unknown top-level field lost: %#v", raw["futureTopLevel"])
	}
	// `disabled` always serializes, like serde (no skip_serializing_if).
	entry, _ := raw["mcpServers"].(map[string]any)
	acme, _ := entry["acme"].(map[string]any)
	if disabled, present := acme["disabled"].(bool); !present || disabled {
		t.Fatalf("disabled flag missing/false: %#v", acme)
	}
	// Sorted keys (serde_json's BTreeMap default) keep the file stable.
	encoded, err := json.Marshal(raw)
	if err != nil {
		t.Fatalf("marshal: %v", err)
	}
	if string(encoded) == "" {
		t.Fatal("empty encoding")
	}
}

func TestMcpToggleDeleteAndUnknownFields(t *testing.T) {
	home := t.TempDir()
	// A legacy raw record keeps extra keys through a toggle (the toggle edits
	// the JSON tree directly, like the Rust core).
	seed := `{"mcpServers":{"legacy":{"type":"stdio","command":"run","legacyFlag":42}}}`
	if err := os.MkdirAll(filepath.Join(home, ".factory"), 0o755); err != nil {
		t.Fatalf("mkdir: %v", err)
	}
	if err := os.WriteFile(filepath.Join(home, ".factory", "mcp.json"), []byte(seed), 0o644); err != nil {
		t.Fatalf("seed: %v", err)
	}

	if err := toggleMcpServerForHome(home, "legacy", true); err != nil {
		t.Fatalf("toggleMcpServerForHome: %v", err)
	}
	raw := mcpReadConfig(t, home)
	entry := raw["mcpServers"].(map[string]any)["legacy"].(map[string]any)
	if entry["disabled"] != true {
		t.Fatalf("disabled not set: %#v", entry)
	}
	if entry["legacyFlag"] != json.Number("42") {
		t.Fatalf("unknown server field lost: %#v", entry)
	}

	err := toggleMcpServerForHome(home, "ghost", true)
	if err == nil || err.Error() != "Server not found: ghost" {
		t.Fatalf("expected Server not found, got %v", err)
	}

	if err := deleteMcpServerForHome(home, "legacy"); err != nil {
		t.Fatalf("deleteMcpServerForHome: %v", err)
	}
	servers, err := loadMcpServersForHome(home)
	if err != nil {
		t.Fatalf("load: %v", err)
	}
	if len(servers) != 0 {
		t.Fatalf("expected no servers after delete, got %+v", servers)
	}
	// Deleting an unknown name succeeds silently.
	if err := deleteMcpServerForHome(home, "ghost"); err != nil {
		t.Fatalf("delete missing: %v", err)
	}

	// A config without mcpServers toggles to a no-op success (Rust falls
	// through to the write with no error).
	if err := os.WriteFile(filepath.Join(home, ".factory", "mcp.json"), []byte(`{"other":1}`), 0o644); err != nil {
		t.Fatalf("rewrite: %v", err)
	}
	if err := toggleMcpServerForHome(home, "ghost", true); err != nil {
		t.Fatalf("toggle without mcpServers: %v", err)
	}
}

func TestMcpLoadSkipsInvalidServerRecords(t *testing.T) {
	home := t.TempDir()
	seed := `{"mcpServers":{"bad":{"type":"bogus"},"good":{"type":"http","url":"https://x"}}}`
	if err := os.MkdirAll(filepath.Join(home, ".factory"), 0o755); err != nil {
		t.Fatalf("mkdir: %v", err)
	}
	if err := os.WriteFile(filepath.Join(home, ".factory", "mcp.json"), []byte(seed), 0o644); err != nil {
		t.Fatalf("seed: %v", err)
	}
	servers, err := loadMcpServersForHome(home)
	if err != nil {
		t.Fatalf("load: %v", err)
	}
	if len(servers) != 1 || servers[0].Name != "good" {
		t.Fatalf("expected only the valid server, got %+v", servers)
	}
	if servers[0].Config.Type != McpServerTypeHTTP {
		t.Fatalf("wrong type: %+v", servers[0].Config)
	}
}

func TestMcpBaizhiPresetIsFixedAndNeverReplaced(t *testing.T) {
	home := t.TempDir()
	if err := saveBaizhiPresetForHome(home); err != nil {
		t.Fatalf("saveBaizhiPresetForHome: %v", err)
	}

	servers, err := loadMcpServersForHome(home)
	if err != nil {
		t.Fatalf("load: %v", err)
	}
	if len(servers) != 1 || servers[0].Name != "baizhi" {
		t.Fatalf("unexpected servers: %+v", servers)
	}
	cfg := servers[0].Config
	if !cfg.Disabled || cfg.URL == nil || *cfg.URL != "https://agent-toolkit.app.baizhi.cloud/mcp" {
		t.Fatalf("preset shape wrong: %+v", cfg)
	}
	if cfg.Headers == nil || (*cfg.Headers)["Authorization"] != "Bearer ${BAIZHI_API_KEY}" {
		t.Fatalf("preset headers wrong: %+v", cfg.Headers)
	}

	raw := mcpReadConfig(t, home)
	entry := raw["mcpServers"].(map[string]any)["baizhi"].(map[string]any)
	if oauth, ok := entry["oauth"].(bool); !ok || oauth {
		t.Fatalf("preset must store oauth=false: %#v", entry)
	}

	err = saveBaizhiPresetForHome(home)
	if err == nil || err.Error() != "MCP server 'baizhi' already exists; not replaced" {
		t.Fatalf("expected already-exists error, got %v", err)
	}
}
