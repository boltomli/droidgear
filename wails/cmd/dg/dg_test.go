package main

import (
	"bytes"
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

// sandbox points every home-ish root at a temp directory and restores it
// after the test (run() also overrides these when --home is passed, so the
// restore here is what keeps the process clean for the next test).
func sandbox(t *testing.T) string {
	t.Helper()
	home := t.TempDir()
	for _, key := range []string{"HOME", "USERPROFILE", "APPDATA", "LOCALAPPDATA"} {
		t.Setenv(key, home)
	}
	return home
}

func runCLI(t *testing.T, args ...string) (int, string, string) {
	t.Helper()
	var stdout, stderr bytes.Buffer
	code := run(args, &stdout, &stderr)
	return code, stdout.String(), stderr.String()
}

// decode parses the single JSON document a --json command must print.
func decode(t *testing.T, out string) map[string]any {
	t.Helper()
	var parsed map[string]any
	if err := json.Unmarshal([]byte(out), &parsed); err != nil {
		t.Fatalf("stdout is not one JSON document: %v\n%s", err, out)
	}
	return parsed
}

func decodeList(t *testing.T, out string) []any {
	t.Helper()
	var parsed []any
	if err := json.Unmarshal([]byte(out), &parsed); err != nil {
		t.Fatalf("stdout is not a JSON array: %v\n%s", err, out)
	}
	return parsed
}

func writeFile(t *testing.T, home string, rel string, contents string) string {
	t.Helper()
	path := filepath.Join(home, filepath.FromSlash(rel))
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		t.Fatalf("mkdir: %v", err)
	}
	if err := os.WriteFile(path, []byte(contents), 0o644); err != nil { // #nosec G306 -- test fixture
		t.Fatalf("write %s: %v", path, err)
	}
	return path
}

func TestVersionIsASingleJSONDocument(t *testing.T) {
	code, out, errOut := runCLI(t, "--json", "version")
	if code != 0 {
		t.Fatalf("exit = %d, stderr = %s", code, errOut)
	}
	if got := decode(t, out)["version"]; got == nil || got == "" {
		t.Errorf("version = %v", got)
	}
	if errOut != "" {
		t.Errorf("stderr should be empty, got %q", errOut)
	}
}

func TestUnknownCommandIsAUsageError(t *testing.T) {
	code, _, errOut := runCLI(t, "--json", "frobnicate")
	if code != exitUsage {
		t.Errorf("exit = %d, want %d", code, exitUsage)
	}
	errObj := decode(t, errOut)
	detail, _ := errObj["error"].(map[string]any)
	if detail["code"] != "usage" {
		t.Errorf("error.code = %v, want usage", detail["code"])
	}
}

func TestGlobalFlagsAreAcceptedAfterTheCommand(t *testing.T) {
	code, out, _ := runCLI(t, "version", "--json")
	if code != 0 {
		t.Fatalf("exit = %d", code)
	}
	if decode(t, out)["version"] == nil {
		t.Errorf("flags after the command were not parsed: %s", out)
	}
}

func TestApplyAndDryRunAreMutuallyExclusive(t *testing.T) {
	code, _, errOut := runCLI(t, "--json", "--apply", "--dry-run", "path", "show")
	if code != exitUsage {
		t.Errorf("exit = %d, want %d", code, exitUsage)
	}
	if !strings.Contains(errOut, "mutually exclusive") {
		t.Errorf("stderr = %s", errOut)
	}
}

func TestBareInvocationIsUsageErrorButHelpIsNot(t *testing.T) {
	code, _, _ := runCLI(t)
	if code != exitUsage {
		t.Errorf("bare dg exit = %d, want %d", code, exitUsage)
	}
	for _, helpArg := range [][]string{{"help"}, {"--help"}} {
		code, out, _ := runCLI(t, helpArg...)
		if code != 0 {
			t.Errorf("%v exit = %d, want 0", helpArg, code)
		}
		if !strings.Contains(out, "Usage:") {
			t.Errorf("%v should print usage, got %q", helpArg, out)
		}
	}
}

func TestPathSetShowResetRoundTrip(t *testing.T) {
	home := sandbox(t)

	code, out, errOut := runCLI(t, "--json", "path", "set", "codex", "/elsewhere/codex")
	if code != 0 {
		t.Fatalf("path set exit = %d, stderr = %s", code, errOut)
	}
	receipt := decode(t, out)
	if receipt["applied"] != true || receipt["key"] != "codex" || receipt["path"] != "/elsewhere/codex" {
		t.Errorf("receipt = %v", receipt)
	}

	code, out, errOut = runCLI(t, "path", "show", "--json")
	if code != 0 {
		t.Fatalf("path show exit = %d, stderr = %s", code, errOut)
	}
	codex, _ := decode(t, out)["codex"].(map[string]any)
	if codex["path"] != "/elsewhere/codex" || codex["isDefault"] != false {
		t.Errorf("codex after set = %v", codex)
	}

	// The override must be on disk, in the shape the GUI writes.
	raw, err := os.ReadFile(filepath.Join(home, ".droidgear", "settings.json")) // #nosec G304 -- temp fixture
	if err != nil {
		t.Fatalf("read settings: %v", err)
	}
	if !strings.Contains(string(raw), "/elsewhere/codex") {
		t.Errorf("settings.json missing the override: %s", raw)
	}

	// Dry run must not write.
	code, out, errOut = runCLI(t, "--json", "--dry-run", "path", "reset", "codex")
	if code != 0 {
		t.Fatalf("dry reset exit = %d, stderr = %s", code, errOut)
	}
	if decode(t, out)["applied"] != false {
		t.Errorf("dry-run receipt = %v", out)
	}
	raw, err = os.ReadFile(filepath.Join(home, ".droidgear", "settings.json")) // #nosec G304 -- temp fixture
	if err != nil {
		t.Fatalf("read settings: %v", err)
	}
	if !strings.Contains(string(raw), "/elsewhere/codex") {
		t.Error("dry-run changed settings.json")
	}

	code, _, errOut = runCLI(t, "--json", "path", "reset", "codex")
	if code != 0 {
		t.Fatalf("reset exit = %d, stderr = %s", code, errOut)
	}
	code, out, _ = runCLI(t, "path", "show", "--json")
	if code != 0 {
		t.Fatalf("path show exit = %d", code)
	}
	codex, _ = decode(t, out)["codex"].(map[string]any)
	if codex["isDefault"] != true {
		t.Errorf("codex after reset = %v", codex)
	}
}

func TestUnknownPathKeyIsAUsageError(t *testing.T) {
	sandbox(t)
	code, _, errOut := runCLI(t, "--json", "path", "set", "nope", "/tmp/x")
	if code != exitUsage {
		t.Errorf("exit = %d, want %d (stderr %s)", code, exitUsage, errOut)
	}
	if !strings.Contains(errOut, "unknown path key") {
		t.Errorf("stderr = %s", errOut)
	}
}

func TestDoctorIsReadOnly(t *testing.T) {
	home := sandbox(t)
	code, out, errOut := runCLI(t, "--json", "doctor")
	if code != 0 {
		t.Fatalf("exit = %d, stderr = %s", code, errOut)
	}
	report := decode(t, out)
	if report["version"] == nil || report["home"] == nil {
		t.Errorf("report missing basics: %v", report)
	}
	paths, _ := report["paths"].(map[string]any)
	if paths["codex"] == nil {
		t.Errorf("report missing paths: %v", report)
	}
	for _, created := range []string{
		".droidgear/channels.json",
		".droidgear/settings.json",
		".droidgear/export-templates.yaml",
	} {
		if _, err := os.Stat(filepath.Join(home, filepath.FromSlash(created))); !os.IsNotExist(err) {
			t.Errorf("doctor must not create %s", created)
		}
	}
}

func TestChannelsListAndShow(t *testing.T) {
	home := sandbox(t)
	writeFile(t, home, ".droidgear/channels.json",
		`[{"id":"ch1","name":"Chan One","type":"general","baseUrl":"http://a.test","enabled":true,"createdAt":1}]`)

	code, out, errOut := runCLI(t, "--json", "channels", "list")
	if code != 0 {
		t.Fatalf("list exit = %d, stderr = %s", code, errOut)
	}
	list := decodeList(t, out)
	if len(list) != 1 {
		t.Fatalf("channels = %v", list)
	}

	code, out, _ = runCLI(t, "channels", "show", "ch1", "--json")
	if code != 0 {
		t.Fatalf("show exit = %d", code)
	}
	detail := decode(t, out)
	channel, _ := detail["channel"].(map[string]any)
	if channel["id"] != "ch1" || detail["authKind"] != "none" {
		t.Errorf("detail = %v", detail)
	}

	// Unknown id is a not_found (exit 1), not a usage error.
	code, _, errOut = runCLI(t, "--json", "channels", "show", "ghost")
	if code != exitRuntime {
		t.Errorf("unknown channel exit = %d, want %d", code, exitRuntime)
	}
	if decode(t, errOut)["error"].(map[string]any)["code"] != "not_found" {
		t.Errorf("error = %s", errOut)
	}
}

func TestChannelsTokensRedactsUnlessRevealed(t *testing.T) {
	home := sandbox(t)
	writeFile(t, home, ".droidgear/channels.json",
		`[{"id":"ch1","name":"Chan One","type":"general","baseUrl":"http://a.test","enabled":true,"createdAt":1}]`)
	writeFile(t, home, ".droidgear/auth/ch1.json",
		`{"type":"api_key","api_key":"sk-cli-9999"}`)

	code, out, errOut := runCLI(t, "--json", "channels", "tokens", "ch1")
	if code != 0 {
		t.Fatalf("tokens exit = %d, stderr = %s", code, errOut)
	}
	tokens := decodeList(t, out)
	if len(tokens) != 1 {
		t.Fatalf("tokens = %v", tokens)
	}
	if key := tokens[0].(map[string]any)["key"].(string); key != "****9999" {
		t.Errorf("default output must be redacted, got %q", key)
	}

	code, out, errOut = runCLI(t, "--json", "--reveal", "channels", "tokens", "ch1")
	if code != 0 {
		t.Fatalf("reveal exit = %d, stderr = %s", code, errOut)
	}
	if key := decodeList(t, out)[0].(map[string]any)["key"].(string); key != "sk-cli-9999" {
		t.Errorf("--reveal should print the real key, got %q", key)
	}
}

func TestExportSaveRunDeleteLifecycle(t *testing.T) {
	home := sandbox(t)
	writeFile(t, home, ".droidgear/channels.json",
		`[{"id":"ch1","name":"Chan One","type":"general","baseUrl":"http://a.test","enabled":true,"createdAt":1}]`)
	writeFile(t, home, ".droidgear/auth/ch1.json",
		`{"type":"api_key","api_key":"sk-export-4242"}`)

	outputPath := filepath.Join(home, "out", "records.json")
	templatePath := writeFile(t, home, "template.json", `{
	  "name": "t1",
	  "format": "json",
	  "outputPath": `+mustJSON(t, outputPath)+`,
	  "channels": {},
	  "tokens": {},
	  "fetchModels": false
	}`)

	// list on an empty store
	code, out, errOut := runCLI(t, "--json", "export", "list")
	if code != 0 {
		t.Fatalf("list exit = %d, stderr = %s", code, errOut)
	}
	if got := decodeList(t, out); len(got) != 0 {
		t.Errorf("expected no templates, got %v", got)
	}

	// save
	code, out, errOut = runCLI(t, "--json", "export", "save", "--file", templatePath)
	if code != 0 {
		t.Fatalf("save exit = %d, stderr = %s", code, errOut)
	}
	if receipt := decode(t, out); receipt["applied"] != true || receipt["name"] != "t1" {
		t.Errorf("save receipt = %v", receipt)
	}

	// dry run: no file, redacted preview
	code, out, errOut = runCLI(t, "--json", "--dry-run", "export", "run", "t1")
	if code != 0 {
		t.Fatalf("dry run exit = %d, stderr = %s", code, errOut)
	}
	outcome := decode(t, out)
	if outcome["applied"] != false {
		t.Errorf("dry-run applied = %v", outcome["applied"])
	}
	changes, _ := outcome["changes"].([]any)
	if len(changes) != 1 {
		t.Fatalf("changes = %v", outcome["changes"])
	}
	change := changes[0].(map[string]any)
	if change["redacted"] != true {
		t.Errorf("preview should be redacted: %v", change)
	}
	if after, _ := change["after"].(string); strings.Contains(after, "sk-export-4242") {
		t.Error("dry-run preview leaked the key")
	}
	if _, err := os.Stat(outputPath); !os.IsNotExist(err) {
		t.Error("dry run wrote the output file")
	}

	// real run
	code, out, errOut = runCLI(t, "--json", "export", "run", "t1")
	if code != 0 {
		t.Fatalf("run exit = %d, stderr = %s", code, errOut)
	}
	if applied := decode(t, out)["applied"]; applied != true {
		t.Errorf("run applied = %v", applied)
	}
	raw, err := os.ReadFile(outputPath) // #nosec G304 -- temp fixture
	if err != nil {
		t.Fatalf("read output: %v", err)
	}
	if !strings.Contains(string(raw), "sk-export-4242") {
		t.Errorf("written file must carry the real key: %s", raw)
	}

	// delete, then delete again → not_found
	code, _, errOut = runCLI(t, "--json", "export", "delete", "t1")
	if code != 0 {
		t.Fatalf("delete exit = %d, stderr = %s", code, errOut)
	}
	code, _, errOut = runCLI(t, "--json", "export", "delete", "t1")
	if code != exitRuntime {
		t.Errorf("second delete exit = %d, want %d", code, exitRuntime)
	}
	if decode(t, errOut)["error"].(map[string]any)["code"] != "not_found" {
		t.Errorf("error = %s", errOut)
	}

	// running a template that never existed is not_found, not usage
	code, _, errOut = runCLI(t, "--json", "export", "run", "ghost")
	if code != exitRuntime || decode(t, errOut)["error"].(map[string]any)["code"] != "not_found" {
		t.Errorf("run ghost: exit=%d stderr=%s", code, errOut)
	}
}

func TestHomeFlagSandboxesTheWholeProfile(t *testing.T) {
	// The real profile must stay untouched: the flag redirects HOME and the
	// Windows-derived roots together (design doc §4.3).
	home := sandbox(t)
	other := t.TempDir()

	code, out, errOut := runCLI(t, "--home", other, "--json", "path", "set", "codex", "/sandboxed/codex")
	if code != 0 {
		t.Fatalf("exit = %d, stderr = %s", code, errOut)
	}
	if decode(t, out)["applied"] != true {
		t.Errorf("receipt = %s", out)
	}
	if _, err := os.Stat(filepath.Join(other, ".droidgear", "settings.json")); err != nil {
		t.Errorf("override was not written under --home: %v", err)
	}
	if _, err := os.Stat(filepath.Join(home, ".droidgear", "settings.json")); !os.IsNotExist(err) {
		t.Error("--home leaked into the ambient home")
	}
}

func TestHumanOutputKeepsSecretsOffStdout(t *testing.T) {
	home := sandbox(t)
	writeFile(t, home, ".droidgear/channels.json",
		`[{"id":"ch1","name":"Chan One","type":"general","baseUrl":"http://a.test","enabled":true,"createdAt":1}]`)
	writeFile(t, home, ".droidgear/auth/ch1.json",
		`{"type":"api_key","api_key":"sk-cli-7777"}`)

	code, out, errOut := runCLI(t, "channels", "tokens", "ch1")
	if code != 0 {
		t.Fatalf("exit = %d, stderr = %s", code, errOut)
	}
	if strings.Contains(out, "sk-cli-7777") {
		t.Errorf("human output leaked the key: %s", out)
	}
	if !strings.Contains(out, "****7777") {
		t.Errorf("human output should show the masked key: %s", out)
	}
}

func mustJSON(t *testing.T, value string) string {
	t.Helper()
	encoded, err := json.Marshal(value)
	if err != nil {
		t.Fatalf("marshal: %v", err)
	}
	return string(encoded)
}
