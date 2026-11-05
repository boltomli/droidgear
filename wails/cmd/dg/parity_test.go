package main

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

// P4 命令树补齐（tui-parity §5）：claude/copilot/droid/dsh/sessions/spec 的
// golden 断言。每个写子命令至少一条 --dry-run 不落盘 + 一条真写的用例。

func TestClaudeSettingsCommandRoundTrip(t *testing.T) {
	home := sandbox(t)

	code, out, errOut := runCLI(t, "--json", "claude", "settings", "list")
	if code != 0 {
		t.Fatalf("list exit = %d, stderr = %s", code, errOut)
	}
	if files := decodeList(t, out); len(files) == 0 {
		t.Fatalf("expected the Global row, got %s", out)
	}

	code, _, errOut = runCLI(t, "--json", "claude", "settings", "create", "work")
	if code != 0 {
		t.Fatalf("create exit = %d, stderr = %s", code, errOut)
	}

	code, out, errOut = runCLI(t, "--json", "claude", "settings", "use", "work")
	if code != 0 {
		t.Fatalf("use exit = %d, stderr = %s", code, errOut)
	}
	if got := decode(t, out); got["isActive"] != true || got["name"] != "work" {
		t.Fatalf("use result = %v", got)
	}

	// Round-trip a settings document through save --file + show.
	settingsFile := writeFile(t, home, "settings.json",
		`{"model":"claude-opus-4-9","env":{"ANTHROPIC_API_KEY":"sk-live-abcdef123456"}}`)
	code, _, errOut = runCLI(t, "--json", "claude", "settings", "save", "work", "--file", settingsFile)
	if code != 0 {
		t.Fatalf("save exit = %d, stderr = %s", code, errOut)
	}

	// Secrets are redacted without --reveal.
	code, out, errOut = runCLI(t, "claude", "settings", "show", "work")
	if code != 0 {
		t.Fatalf("show exit = %d, stderr = %s", code, errOut)
	}
	if strings.Contains(out, "sk-live-abcdef123456") {
		t.Fatalf("show leaked the api key:\n%s", out)
	}
	if !strings.Contains(out, "****") {
		t.Fatalf("show has no redaction marker:\n%s", out)
	}

	// Deleting the Global file is a usage error (same as the GUI guard).
	code, _, errOut = runCLI(t, "claude", "settings", "delete", "global")
	if code != exitUsage {
		t.Fatalf("delete global exit = %d, want %d (%s)", code, exitUsage, errOut)
	}

	code, _, errOut = runCLI(t, "--json", "claude", "settings", "delete", "work")
	if code != 0 {
		t.Fatalf("delete exit = %d, stderr = %s", code, errOut)
	}
	code, out, _ = runCLI(t, "--json", "claude", "settings", "list")
	if strings.Contains(out, `"name":"work"`) {
		t.Fatalf("work still listed after delete: %s", out)
	}
}

func TestClaudeSettingsDryRunDoesNotWrite(t *testing.T) {
	home := sandbox(t)
	if code, _, errOut := runCLI(t, "--json", "claude", "settings", "create", "work"); code != 0 {
		t.Fatalf("create exit = %d, stderr = %s", code, errOut)
	}

	before := readClaudeSettingsDir(t, home)
	code, out, errOut := runCLI(t, "--json", "--dry-run", "claude", "settings", "delete", "work")
	if code != 0 {
		t.Fatalf("dry-run delete exit = %d, stderr = %s", code, errOut)
	}
	preview := decode(t, out)
	if preview["applied"] != false {
		t.Fatalf("dry-run receipt applied = %v, want false", preview["applied"])
	}
	if after := readClaudeSettingsDir(t, home); after != before {
		t.Fatalf("dry-run changed the settings dir:\n%s\n---\n%s", before, after)
	}
}

// readClaudeSettingsDir flattens the managed settings dir so a dry-run can
// be compared against the real run byte for byte.
func readClaudeSettingsDir(t *testing.T, home string) string {
	t.Helper()
	dir := filepath.Join(home, ".droidgear", "claude-settings")
	entries, err := os.ReadDir(dir)
	if err != nil {
		return ""
	}
	var b strings.Builder
	for _, entry := range entries {
		data, err := os.ReadFile(filepath.Join(dir, entry.Name())) // #nosec G304 -- test sandbox
		if err != nil {
			continue
		}
		b.WriteString(entry.Name())
		b.WriteString(":")
		b.WriteString(string(data))
		b.WriteString("\n")
	}
	return b.String()
}

func TestCopilotCommandRoundTrip(t *testing.T) {
	home := sandbox(t)
	profileFile := writeFile(t, home, "copilot.json", `{
  "id": "work",
  "name": "Work BYOK",
  "createdAt": "2026-01-01T00:00:00Z",
  "updatedAt": "2026-01-01T00:00:00Z",
  "useOfficialAuth": false,
  "baseUrl": "https://llm.example.test/v1",
  "providerType": "anthropic",
  "apiKey": "sk-copilot-live-key-9876",
  "model": "claude-opus-4-9"
}`)

	code, _, errOut := runCLI(t, "--json", "copilot", "profiles", "save", "--file", profileFile)
	if code != 0 {
		t.Fatalf("save exit = %d, stderr = %s", code, errOut)
	}

	code, out, errOut := runCLI(t, "copilot", "profiles", "list")
	if code != 0 {
		t.Fatalf("list exit = %d, stderr = %s", code, errOut)
	}
	if strings.Contains(out, "sk-copilot-live-key-9876") {
		t.Fatalf("list leaked the api key:\n%s", out)
	}
	if !strings.Contains(out, "work") {
		t.Fatalf("list missing the profile:\n%s", out)
	}

	code, out, errOut = runCLI(t, "--json", "copilot", "profiles", "apply", "work")
	if code != 0 {
		t.Fatalf("apply exit = %d, stderr = %s", code, errOut)
	}
	if decode(t, out)["applied"] != true {
		t.Fatalf("apply receipt = %s", out)
	}

	code, out, errOut = runCLI(t, "--json", "copilot", "status")
	if code != 0 {
		t.Fatalf("status exit = %d, stderr = %s", code, errOut)
	}
	status := decode(t, out)
	if status["configExists"] != true {
		t.Fatalf("status after apply = %v", status)
	}

	// delete dry-run must not remove the profile store.
	code, out, errOut = runCLI(t, "--json", "--dry-run", "copilot", "profiles", "delete", "work")
	if code != 0 {
		t.Fatalf("dry-run delete exit = %d, stderr = %s", code, errOut)
	}
	if decode(t, out)["applied"] != false {
		t.Fatalf("dry-run receipt = %s", out)
	}
	code, _, _ = runCLI(t, "--json", "copilot", "profiles", "list")
	if code != 0 {
		t.Fatal("profile vanished during dry-run delete")
	}

	code, _, errOut = runCLI(t, "--json", "copilot", "profiles", "delete", "work")
	if code != 0 {
		t.Fatalf("delete exit = %d, stderr = %s", code, errOut)
	}
	code, out, _ = runCLI(t, "--json", "copilot", "profiles", "list")
	if strings.Contains(out, `"id":"work"`) {
		t.Fatalf("profile still listed after delete: %s", out)
	}
}

func TestCopilotMissingProfileIsNotFound(t *testing.T) {
	sandbox(t)
	code, _, errOut := runCLI(t, "--json", "copilot", "profiles", "apply", "nope")
	if code != exitRuntime {
		t.Fatalf("exit = %d, want %d (%s)", code, exitRuntime, errOut)
	}
	if !strings.Contains(errOut, "not found") {
		t.Fatalf("stderr = %s", errOut)
	}
}

func TestDroidCommandSurface(t *testing.T) {
	home := sandbox(t)

	// settings list: the Global row is always there.
	code, out, errOut := runCLI(t, "--json", "droid", "settings", "list")
	if code != 0 {
		t.Fatalf("settings list exit = %d, stderr = %s", code, errOut)
	}
	files := decodeList(t, out)
	if len(files) == 0 {
		t.Fatalf("settings list empty: %s", out)
	}
	if first, _ := files[0].(map[string]any); first == nil || first["isGlobal"] != true {
		t.Fatalf("settings list missing the global row: %s", out)
	}

	// values: the Droid → Settings pane keys, as one JSON document.
	code, out, errOut = runCLI(t, "--json", "droid", "settings", "values")
	if code != 0 {
		t.Fatalf("values exit = %d, stderr = %s", code, errOut)
	}
	values := decode(t, out)
	for _, key := range []string{"diffMode", "cloudSessionSync", "compactionTokenLimit", "reasoningEffort"} {
		if _, ok := values[key]; !ok {
			t.Fatalf("values missing %q: %s", key, out)
		}
	}

	// trusted: add → list → dry-run remove keeps it → real remove clears it.
	// The path must be absolute on the host OS, so build it under the sandbox.
	trusted := filepath.Join(home, "some-dir")
	if err := os.MkdirAll(trusted, 0o755); err != nil {
		t.Fatalf("mkdir trusted: %v", err)
	}
	code, _, errOut = runCLI(t, "--json", "droid", "trusted", "add", trusted)
	if code != 0 {
		t.Fatalf("trusted add exit = %d, stderr = %s", code, errOut)
	}
	code, out, _ = runCLI(t, "--json", "droid", "trusted", "list")
	if !strings.Contains(out, "some-dir") {
		t.Fatalf("trusted list missing the folder: %s", out)
	}
	if code, _, _ = runCLI(t, "--json", "--dry-run", "droid", "trusted", "remove", trusted); code != 0 {
		t.Fatal("dry-run trusted remove failed")
	}
	code, out, _ = runCLI(t, "--json", "droid", "trusted", "list")
	if !strings.Contains(out, "some-dir") {
		t.Fatalf("dry-run removed the trusted folder: %s", out)
	}
	code, _, errOut = runCLI(t, "--json", "droid", "trusted", "remove", trusted)
	if code != 0 {
		t.Fatalf("trusted remove exit = %d, stderr = %s", code, errOut)
	}
	code, out, _ = runCLI(t, "--json", "droid", "trusted", "list")
	if strings.Contains(out, "some-dir") {
		t.Fatalf("folder still trusted after remove: %s", out)
	}

	// missions show + legacy check/delete round out the section.
	if code, _, errOut = runCLI(t, "--json", "droid", "missions", "show"); code != 0 {
		t.Fatalf("missions show exit = %d, stderr = %s", code, errOut)
	}
	code, out, errOut = runCLI(t, "--json", "droid", "legacy", "check")
	if code != 0 {
		t.Fatalf("legacy check exit = %d, stderr = %s", code, errOut)
	}
	if decode(t, out)["legacyExists"] != false {
		t.Fatalf("legacy check = %s", out)
	}
	if code, _, errOut = runCLI(t, "--json", "droid", "legacy", "delete"); code != 0 {
		t.Fatalf("legacy delete exit = %d, stderr = %s", code, errOut)
	}
}

func TestDshProvidersListAndStatus(t *testing.T) {
	sandbox(t)

	code, out, errOut := runCLI(t, "--json", "dsh", "status")
	if code != 0 {
		t.Fatalf("status exit = %d, stderr = %s", code, errOut)
	}
	status := decode(t, out)
	if _, ok := status["configExists"]; !ok {
		t.Fatalf("status payload = %s", out)
	}

	code, out, errOut = runCLI(t, "--json", "dsh", "providers", "list")
	if code != 0 {
		t.Fatalf("providers list exit = %d, stderr = %s", code, errOut)
	}
	if providers, ok := decode(t, out)["providers"]; !ok || providers == nil {
		t.Fatalf("providers list payload = %s", out)
	}
}

func TestSessionsCommandsAreReadOnly(t *testing.T) {
	home := sandbox(t)

	code, out, errOut := runCLI(t, "--json", "sessions", "projects")
	if code != 0 {
		t.Fatalf("projects exit = %d, stderr = %s", code, errOut)
	}
	if got := decodeList(t, out); len(got) != 0 {
		t.Fatalf("expected no projects, got %s", out)
	}

	// One seeded session round-trips through list + show (redacted).
	dir := filepath.Join(home, ".factory", "sessions", "-demo")
	if err := os.MkdirAll(dir, 0o755); err != nil {
		t.Fatalf("mkdir: %v", err)
	}
	session := filepath.Join(dir, "sess-1.jsonl")
	// The secret sits in a `token=` pair so the §5.3 redaction contract has
	// something to mask (free-form prose is not scanned, same as the GUI).
	doc := `{"type":"session_start","id":"sess-1","sessionTitle":"Demo","cwd":"/work"}` + "\n" +
		`{"type":"message","id":"m1","message":{"role":"user","content":[{"type":"text","text":"export token=sk-session-secret-42"}]}}` + "\n"
	if err := os.WriteFile(session, []byte(doc), 0o644); err != nil { // #nosec G306 -- test fixture
		t.Fatalf("write session: %v", err)
	}

	code, out, errOut = runCLI(t, "--json", "sessions", "list")
	if code != 0 {
		t.Fatalf("list exit = %d, stderr = %s", code, errOut)
	}
	if !strings.Contains(out, "sess-1") {
		t.Fatalf("list missing the session: %s", out)
	}

	code, out, errOut = runCLI(t, "sessions", "show", session[:len(session)-len(".jsonl")])
	if code != 0 {
		t.Fatalf("show exit = %d, stderr = %s", code, errOut)
	}
	if !strings.Contains(out, "Demo") {
		t.Fatalf("show missing the transcript:\n%s", out)
	}
	if strings.Contains(out, "sk-session-secret-42") {
		t.Fatalf("show leaked transcript content:\n%s", out)
	}

	// Missing session → not found (exit 1, code not_found).
	code, _, errOut = runCLI(t, "--json", "sessions", "show", filepath.Join(dir, "missing"))
	if code != exitRuntime || !strings.Contains(errOut, "not found") {
		t.Fatalf("show missing exit = %d stderr = %s", code, errOut)
	}
}

func TestSpecCommandsListCatDelete(t *testing.T) {
	home := sandbox(t)

	code, out, errOut := runCLI(t, "--json", "spec", "list")
	if code != 0 {
		t.Fatalf("list exit = %d, stderr = %s", code, errOut)
	}
	if got := decodeList(t, out); len(got) != 0 {
		t.Fatalf("expected no specs, got %s", out)
	}

	writeFile(t, home, ".factory/specs/plan.md", "# plan\ntoken: sk-spec-secret-77\n")
	code, out, errOut = runCLI(t, "--json", "spec", "list")
	if code != 0 {
		t.Fatalf("list exit = %d, stderr = %s", code, errOut)
	}
	if !strings.Contains(out, "plan.md") {
		t.Fatalf("list missing plan.md: %s", out)
	}

	code, out, errOut = runCLI(t, "spec", "cat", "plan.md")
	if code != 0 {
		t.Fatalf("cat exit = %d, stderr = %s", code, errOut)
	}
	if strings.Contains(out, "sk-spec-secret-77") {
		t.Fatalf("cat leaked the token:\n%s", out)
	}

	// dry-run delete keeps the file; the real delete removes it.
	code, out, errOut = runCLI(t, "--json", "--dry-run", "spec", "delete", "plan.md")
	if code != 0 {
		t.Fatalf("dry-run delete exit = %d, stderr = %s", code, errOut)
	}
	if decode(t, out)["applied"] != false {
		t.Fatalf("dry-run receipt = %s", out)
	}
	specPath := filepath.Join(home, ".factory", "specs", "plan.md")
	if _, err := os.Stat(specPath); err != nil {
		t.Fatalf("dry-run deleted the spec: %v", err)
	}

	code, _, errOut = runCLI(t, "--json", "spec", "delete", "plan.md")
	if code != 0 {
		t.Fatalf("delete exit = %d, stderr = %s", code, errOut)
	}
	if _, err := os.Stat(specPath); err == nil {
		t.Fatal("spec still on disk after delete")
	}

	code, _, errOut = runCLI(t, "--json", "spec", "cat", "plan.md")
	if code != exitRuntime || !strings.Contains(errOut, "not found") {
		t.Fatalf("cat after delete exit = %d stderr = %s", code, errOut)
	}
}
