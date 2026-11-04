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
//
// PATH moves too: HermesHome's Windows priority chain falls back to the
// default WSL distro, and with wsl.exe resolvable that would write into the
// developer's REAL \\wsl$ profile (the same trap testutil.IsolatedHome
// documents for internal/app's tests).
func sandbox(t *testing.T) string {
	t.Helper()
	home := t.TempDir()
	for _, key := range []string{"HOME", "USERPROFILE", "APPDATA", "LOCALAPPDATA"} {
		t.Setenv(key, home)
	}
	t.Setenv("PATH", filepath.Join(home, "empty-bin"))
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

func TestCodexProfilesLifecycle(t *testing.T) {
	home := sandbox(t)

	// Fresh home: no profiles, no live config, no active pointer.
	code, out, errOut := runCLI(t, "--json", "codex", "profiles", "list")
	if code != 0 {
		t.Fatalf("list exit = %d, stderr = %s", code, errOut)
	}
	if got := decodeList(t, out); len(got) != 0 {
		t.Errorf("expected no profiles, got %v", got)
	}

	code, out, errOut = runCLI(t, "--json", "codex", "status")
	if code != 0 {
		t.Fatalf("status exit = %d, stderr = %s", code, errOut)
	}
	status := decode(t, out)
	if status["configExists"] != false || status["authExists"] != false {
		t.Errorf("fresh status should report missing files: %v", status)
	}
	if status["activeProfileId"] != nil {
		t.Errorf("fresh status should have no active profile: %v", status)
	}

	// Save a profile from a JSON document (two API keys to check redaction).
	profilePath := writeFile(t, home, "profile.json", `{
	  "id": "p1",
	  "name": "One",
	  "modelProvider": "custom",
	  "model": "gpt-5.5",
	  "modelReasoningEffort": "medium",
	  "apiKey": "sk-secret-1234",
	  "apiKeyModelDiscovery": true,
	  "providers": {
	    "custom": {
	      "name": "Custom",
	      "baseUrl": "https://api.example.com/v1",
	      "wireApi": "responses",
	      "requiresOpenaiAuth": false,
	      "model": "gpt-5.5",
	      "modelReasoningEffort": "high",
	      "apiKey": "sk-provider-5678",
	      "envKey": "EXAMPLE_API_KEY"
	    }
	  }
	}`)
	code, out, errOut = runCLI(t, "--json", "codex", "profiles", "save", "--file", profilePath)
	if code != 0 {
		t.Fatalf("save exit = %d, stderr = %s", code, errOut)
	}
	if receipt := decode(t, out); receipt["applied"] != true || receipt["name"] != "One" {
		t.Errorf("save receipt = %v", receipt)
	}

	// API keys are redacted by default, at both levels of the document.
	code, out, errOut = runCLI(t, "--json", "codex", "profiles", "list")
	if code != 0 {
		t.Fatalf("list exit = %d, stderr = %s", code, errOut)
	}
	list := decodeList(t, out)
	if len(list) != 1 {
		t.Fatalf("profiles = %v", list)
	}
	profile, _ := list[0].(map[string]any)
	if profile["apiKey"] != "****1234" {
		t.Errorf("profile.apiKey = %v, want ****1234", profile["apiKey"])
	}
	providers, _ := profile["providers"].(map[string]any)
	custom, _ := providers["custom"].(map[string]any)
	if custom["apiKey"] != "****5678" {
		t.Errorf("provider.apiKey = %v, want ****5678", custom["apiKey"])
	}

	code, out, errOut = runCLI(t, "--json", "--reveal", "codex", "profiles", "list")
	if code != 0 {
		t.Fatalf("reveal list exit = %d, stderr = %s", code, errOut)
	}
	revealed, _ := decodeList(t, out)[0].(map[string]any)
	if revealed["apiKey"] != "sk-secret-1234" {
		t.Errorf("--reveal apiKey = %v", revealed["apiKey"])
	}

	// Unknown id is not_found, not a usage error.
	code, _, errOut = runCLI(t, "--json", "codex", "profiles", "show", "ghost")
	if code != exitRuntime {
		t.Errorf("show ghost exit = %d, want %d", code, exitRuntime)
	}
	if decode(t, errOut)["error"].(map[string]any)["code"] != "not_found" {
		t.Errorf("error = %s", errOut)
	}

	// Apply writes the live config and pins the active profile.
	code, out, errOut = runCLI(t, "--json", "codex", "profiles", "apply", "p1")
	if code != 0 {
		t.Fatalf("apply exit = %d, stderr = %s", code, errOut)
	}
	if applied := decode(t, out)["applied"]; applied != true {
		t.Errorf("apply receipt = %v", out)
	}
	configPath := filepath.Join(home, ".codex", "config.toml")
	raw, err := os.ReadFile(configPath) // #nosec G304 -- temp fixture
	if err != nil {
		t.Fatalf("read live config: %v", err)
	}
	if !strings.Contains(string(raw), "gpt-5.5") {
		t.Errorf("live config missing the model: %s", raw)
	}

	code, out, errOut = runCLI(t, "--json", "codex", "profiles", "active")
	if code != 0 {
		t.Fatalf("active exit = %d, stderr = %s", code, errOut)
	}
	if active := strings.TrimSpace(out); active != `"p1"` {
		t.Errorf("active = %s, want \"p1\"", active)
	}

	// --dry-run records the writes instead of performing them: the ChangeSet
	// replaces the receipt, the profile is still in the store, and no API key
	// from the document being deleted leaks into the preview.
	code, out, errOut = runCLI(t, "--json", "--dry-run", "codex", "profiles", "delete", "p1")
	if code != 0 {
		t.Fatalf("dry-run exit = %d, stderr = %s", code, errOut)
	}
	preview := decode(t, out)
	if preview["applied"] != false {
		t.Errorf("dry-run applied = %v, want false", preview["applied"])
	}
	if changes, _ := preview["changes"].([]any); len(changes) == 0 {
		t.Errorf("dry run recorded no changes: %s", out)
	}
	if strings.Contains(out, "sk-secret-1234") || strings.Contains(out, "sk-provider-5678") {
		t.Error("dry-run preview leaked a profile API key")
	}
	code, out, errOut = runCLI(t, "--json", "codex", "profiles", "list")
	if code != 0 || len(decodeList(t, out)) != 1 {
		t.Errorf("dry-run delete changed the store: exit=%d out=%s", code, out)
	}

	// Delete, then the profile is gone.
	code, _, errOut = runCLI(t, "--json", "codex", "profiles", "delete", "p1")
	if code != 0 {
		t.Fatalf("delete exit = %d, stderr = %s", code, errOut)
	}
	code, _, errOut = runCLI(t, "--json", "codex", "profiles", "show", "p1")
	if code != exitRuntime || decode(t, errOut)["error"].(map[string]any)["code"] != "not_found" {
		t.Errorf("show after delete: exit=%d stderr=%s", code, errOut)
	}
}

func TestHermesProfilesLifecycle(t *testing.T) {
	home := sandbox(t)

	code, out, errOut := runCLI(t, "--json", "hermes", "profiles", "list")
	if code != 0 {
		t.Fatalf("list exit = %d, stderr = %s", code, errOut)
	}
	if got := decodeList(t, out); len(got) != 0 {
		t.Errorf("expected no profiles, got %v", got)
	}

	code, out, errOut = runCLI(t, "--json", "hermes", "status")
	if code != 0 {
		t.Fatalf("status exit = %d, stderr = %s", code, errOut)
	}
	if status := decode(t, out); status["configExists"] != false {
		t.Errorf("fresh status = %v", status)
	}

	profilePath := writeFile(t, home, "hermes-profile.json", `{
	  "id": "h1",
	  "name": "Hermes One",
	  "models": [
	    {
	      "name": "DeepSeek",
	      "provider": "deepseek",
	      "baseUrl": "https://api.deepseek.com/v1",
	      "apiKey": "sk-hermes-1234",
	      "isDefault": true
	    }
	  ],
	  "reasoningEffort": "high"
	}`)
	code, out, errOut = runCLI(t, "--json", "hermes", "profiles", "save", "--file", profilePath)
	if code != 0 {
		t.Fatalf("save exit = %d, stderr = %s", code, errOut)
	}
	if receipt := decode(t, out); receipt["applied"] != true || receipt["name"] != "Hermes One" {
		t.Errorf("save receipt = %v", receipt)
	}

	// The per-model apiKey is redacted by default, revealed on demand.
	code, out, errOut = runCLI(t, "--json", "hermes", "profiles", "list")
	if code != 0 {
		t.Fatalf("list exit = %d, stderr = %s", code, errOut)
	}
	list := decodeList(t, out)
	if len(list) != 1 {
		t.Fatalf("profiles = %v", list)
	}
	models, _ := list[0].(map[string]any)["models"].([]any)
	first, _ := models[0].(map[string]any)
	if first["apiKey"] != "****1234" {
		t.Errorf("model.apiKey = %v, want ****1234", first["apiKey"])
	}

	code, out, errOut = runCLI(t, "--json", "--reveal", "hermes", "profiles", "list")
	if code != 0 {
		t.Fatalf("reveal exit = %d, stderr = %s", code, errOut)
	}
	models, _ = decodeList(t, out)[0].(map[string]any)["models"].([]any)
	if key, _ := models[0].(map[string]any)["apiKey"].(string); key != "sk-hermes-1234" {
		t.Errorf("--reveal apiKey = %v", key)
	}

	// Unknown id → not_found.
	code, _, errOut = runCLI(t, "--json", "hermes", "profiles", "show", "ghost")
	if code != exitRuntime || decode(t, errOut)["error"].(map[string]any)["code"] != "not_found" {
		t.Errorf("show ghost: exit=%d stderr=%s", code, errOut)
	}

	// Apply writes ~/.hermes/config.yaml (PATH is isolated, so the WSL
	// fallback in HermesHome's Windows priority chain cannot fire).
	code, out, errOut = runCLI(t, "--json", "hermes", "profiles", "apply", "h1")
	if code != 0 {
		t.Fatalf("apply exit = %d, stderr = %s", code, errOut)
	}
	if applied := decode(t, out)["applied"]; applied != true {
		t.Errorf("apply receipt = %s", out)
	}
	configPath := filepath.Join(home, ".hermes", "config.yaml")
	raw, err := os.ReadFile(configPath) // #nosec G304 -- temp fixture
	if err != nil {
		t.Fatalf("read live config: %v", err)
	}
	if !strings.Contains(string(raw), "deepseek") {
		t.Errorf("live config missing the provider: %s", raw)
	}
	if _, err := os.Stat(filepath.Join(home, "wsl")); !os.IsNotExist(err) {
		t.Error("apply escaped the sandbox into a WSL path")
	}

	// --dry-run records the writes instead of performing them: the ChangeSet
	// replaces the receipt, the profile survives, and the model API key never
	// reaches stdout.
	code, out, errOut = runCLI(t, "--json", "--dry-run", "hermes", "profiles", "delete", "h1")
	if code != 0 {
		t.Fatalf("dry-run exit = %d, stderr = %s", code, errOut)
	}
	preview := decode(t, out)
	if preview["applied"] != false {
		t.Errorf("dry-run applied = %v, want false", preview["applied"])
	}
	if changes, _ := preview["changes"].([]any); len(changes) == 0 {
		t.Errorf("dry run recorded no changes: %s", out)
	}
	if strings.Contains(out, "sk-hermes-1234") {
		t.Error("dry-run preview leaked the model API key")
	}
	code, out, errOut = runCLI(t, "--json", "hermes", "profiles", "list")
	if code != 0 || len(decodeList(t, out)) != 1 {
		t.Errorf("dry-run delete changed the store: exit=%d out=%s", code, out)
	}

	code, _, errOut = runCLI(t, "--json", "hermes", "profiles", "delete", "h1")
	if code != 0 {
		t.Fatalf("delete exit = %d, stderr = %s", code, errOut)
	}
	code, _, errOut = runCLI(t, "--json", "hermes", "profiles", "show", "h1")
	if code != exitRuntime || decode(t, errOut)["error"].(map[string]any)["code"] != "not_found" {
		t.Errorf("show after delete: exit=%d stderr=%s", code, errOut)
	}
}

func TestPiProfilesLifecycle(t *testing.T) {
	home := sandbox(t)

	code, out, errOut := runCLI(t, "--json", "pi", "profiles", "list")
	if code != 0 {
		t.Fatalf("list exit = %d, stderr = %s", code, errOut)
	}
	if got := decodeList(t, out); len(got) != 0 {
		t.Errorf("expected no profiles, got %v", got)
	}

	profilePath := writeFile(t, home, "pi-profile.json", `{
	  "id": "p1",
	  "name": "Pi One",
	  "providers": {
	    "deepseek": {
	      "baseUrl": "https://api.deepseek.com/v1",
	      "api": "openai-completions",
	      "apiKey": "sk-pi-1234",
	      "oauth": "oauth-blob-5678",
	      "models": [{"id": "deepseek-chat"}]
	    }
	  }
	}`)
	code, out, errOut = runCLI(t, "--json", "pi", "profiles", "save", "--file", profilePath)
	if code != 0 {
		t.Fatalf("save exit = %d, stderr = %s", code, errOut)
	}
	if receipt := decode(t, out); receipt["applied"] != true || receipt["name"] != "Pi One" {
		t.Errorf("save receipt = %v", receipt)
	}

	// Both provider secrets are redacted by default.
	code, out, errOut = runCLI(t, "--json", "pi", "profiles", "list")
	if code != 0 {
		t.Fatalf("list exit = %d, stderr = %s", code, errOut)
	}
	list := decodeList(t, out)
	if len(list) != 1 {
		t.Fatalf("profiles = %v", list)
	}
	providers, _ := list[0].(map[string]any)["providers"].(map[string]any)
	deepseek, _ := providers["deepseek"].(map[string]any)
	if deepseek["apiKey"] != "****1234" || deepseek["oauth"] != "****5678" {
		t.Errorf("provider secrets = apiKey %v, oauth %v", deepseek["apiKey"], deepseek["oauth"])
	}

	code, out, errOut = runCLI(t, "--json", "--reveal", "pi", "profiles", "list")
	if code != 0 {
		t.Fatalf("reveal exit = %d, stderr = %s", code, errOut)
	}
	providers, _ = decodeList(t, out)[0].(map[string]any)["providers"].(map[string]any)
	deepseek, _ = providers["deepseek"].(map[string]any)
	if deepseek["apiKey"] != "sk-pi-1234" || deepseek["oauth"] != "oauth-blob-5678" {
		t.Errorf("--reveal secrets = apiKey %v, oauth %v", deepseek["apiKey"], deepseek["oauth"])
	}

	code, _, errOut = runCLI(t, "--json", "pi", "profiles", "show", "ghost")
	if code != exitRuntime || decode(t, errOut)["error"].(map[string]any)["code"] != "not_found" {
		t.Errorf("show ghost: exit=%d stderr=%s", code, errOut)
	}

	code, out, errOut = runCLI(t, "--json", "pi", "profiles", "apply", "p1")
	if code != 0 {
		t.Fatalf("apply exit = %d, stderr = %s", code, errOut)
	}
	if applied := decode(t, out)["applied"]; applied != true {
		t.Errorf("apply receipt = %s", out)
	}
	configPath := filepath.Join(home, ".pi", "agent", "models.json")
	raw, err := os.ReadFile(configPath) // #nosec G304 -- temp fixture
	if err != nil {
		t.Fatalf("read live config: %v", err)
	}
	if !strings.Contains(string(raw), "deepseek-chat") {
		t.Errorf("live config missing the model: %s", raw)
	}

	code, out, errOut = runCLI(t, "--json", "pi", "profiles", "active")
	if code != 0 {
		t.Fatalf("active exit = %d, stderr = %s", code, errOut)
	}
	if active := strings.TrimSpace(out); active != `"p1"` {
		t.Errorf("active = %s, want \"p1\"", active)
	}

	// --dry-run records instead of writing: a ChangeSet replaces the receipt,
	// the profile survives, and the provider's apiKey/oauth never reach stdout.
	code, out, errOut = runCLI(t, "--json", "--dry-run", "pi", "profiles", "delete", "p1")
	if code != 0 {
		t.Fatalf("dry-run exit = %d, stderr = %s", code, errOut)
	}
	preview := decode(t, out)
	if preview["applied"] != false {
		t.Errorf("dry-run applied = %v, want false", preview["applied"])
	}
	if changes, _ := preview["changes"].([]any); len(changes) == 0 {
		t.Errorf("dry run recorded no changes: %s", out)
	}
	if strings.Contains(out, "sk-pi-1234") || strings.Contains(out, "oauth-blob-5678") {
		t.Error("dry-run preview leaked a provider secret")
	}
	code, out, errOut = runCLI(t, "--json", "pi", "profiles", "list")
	if code != 0 || len(decodeList(t, out)) != 1 {
		t.Errorf("dry-run delete changed the store: exit=%d out=%s", code, out)
	}

	code, _, errOut = runCLI(t, "--json", "pi", "profiles", "delete", "p1")
	if code != 0 {
		t.Fatalf("delete exit = %d, stderr = %s", code, errOut)
	}
	code, _, errOut = runCLI(t, "--json", "pi", "profiles", "show", "p1")
	if code != exitRuntime || decode(t, errOut)["error"].(map[string]any)["code"] != "not_found" {
		t.Errorf("show after delete: exit=%d stderr=%s", code, errOut)
	}
}

func TestOmpProfilesAndProvidersLifecycle(t *testing.T) {
	home := sandbox(t)

	code, out, errOut := runCLI(t, "--json", "omp", "profiles", "list")
	if code != 0 {
		t.Fatalf("list exit = %d, stderr = %s", code, errOut)
	}
	if got := decodeList(t, out); len(got) != 0 {
		t.Errorf("expected no profiles, got %v", got)
	}

	profilePath := writeFile(t, home, "omp-profile.json", `{
	  "id": "o1",
	  "name": "OMP One",
	  "modelRoles": {}
	}`)
	code, out, errOut = runCLI(t, "--json", "omp", "profiles", "save", "--file", profilePath)
	if code != 0 {
		t.Fatalf("save exit = %d, stderr = %s", code, errOut)
	}
	if receipt := decode(t, out); receipt["applied"] != true || receipt["name"] != "OMP One" {
		t.Errorf("save receipt = %v", receipt)
	}

	code, out, errOut = runCLI(t, "--json", "omp", "profiles", "apply", "o1")
	if code != 0 {
		t.Fatalf("apply exit = %d, stderr = %s", code, errOut)
	}
	if applied := decode(t, out)["applied"]; applied != true {
		t.Errorf("apply receipt = %s", out)
	}
	if raw, err := os.ReadFile(filepath.Join(home, ".omp", "agent", "config.yml")); err != nil {
		t.Fatalf("read live config: %v", err)
	} else if !strings.Contains(string(raw), "model_roles") && !strings.Contains(string(raw), "modelRoles") {
		t.Errorf("live config missing model roles: %s", raw)
	}

	code, out, errOut = runCLI(t, "--json", "omp", "profiles", "active")
	if code != 0 {
		t.Fatalf("active exit = %d, stderr = %s", code, errOut)
	}
	if active := strings.TrimSpace(out); active != `"o1"` {
		t.Errorf("active = %s, want \"o1\"", active)
	}

	// Custom providers carry the apiKey that has to stay redacted.
	providerPath := writeFile(t, home, "omp-provider.json", `{
	  "providerId": "deepseek",
	  "config": {
	    "baseUrl": "https://api.deepseek.com/v1",
	    "auth": "apiKey",
	    "apiKey": "sk-omp-1234"
	  }
	}`)
	code, out, errOut = runCLI(t, "--json", "omp", "providers", "save", "--file", providerPath)
	if code != 0 {
		t.Fatalf("provider save exit = %d, stderr = %s", code, errOut)
	}
	if receipt := decode(t, out); receipt["applied"] != true || receipt["name"] != "deepseek" {
		t.Errorf("provider save receipt = %v", receipt)
	}

	code, out, errOut = runCLI(t, "--json", "omp", "providers", "list")
	if code != 0 {
		t.Fatalf("provider list exit = %d, stderr = %s", code, errOut)
	}
	list, _ := decode(t, out)["providers"].([]any)
	if len(list) != 1 {
		t.Fatalf("providers = %v", list)
	}
	config, _ := list[0].(map[string]any)["config"].(map[string]any)
	if config["apiKey"] != "****1234" {
		t.Errorf("provider apiKey = %v, want ****1234", config["apiKey"])
	}

	code, out, errOut = runCLI(t, "--json", "--reveal", "omp", "providers", "list")
	if code != 0 {
		t.Fatalf("reveal exit = %d, stderr = %s", code, errOut)
	}
	config, _ = decode(t, out)["providers"].([]any)[0].(map[string]any)["config"].(map[string]any)
	if config["apiKey"] != "sk-omp-1234" {
		t.Errorf("--reveal apiKey = %v", config["apiKey"])
	}

	code, _, errOut = runCLI(t, "--json", "omp", "providers", "delete", "deepseek")
	if code != 0 {
		t.Fatalf("provider delete exit = %d, stderr = %s", code, errOut)
	}
	code, _, errOut = runCLI(t, "--json", "omp", "providers", "delete", "deepseek")
	if code != exitRuntime || decode(t, errOut)["error"].(map[string]any)["code"] != "not_found" {
		t.Errorf("second provider delete: exit=%d stderr=%s", code, errOut)
	}

	// --dry-run records instead of writing: a ChangeSet replaces the receipt
	// and the profile is still there afterwards.
	code, out, errOut = runCLI(t, "--json", "--dry-run", "omp", "profiles", "delete", "o1")
	if code != 0 {
		t.Fatalf("dry-run exit = %d, stderr = %s", code, errOut)
	}
	preview := decode(t, out)
	if preview["applied"] != false {
		t.Errorf("dry-run applied = %v, want false", preview["applied"])
	}
	if changes, _ := preview["changes"].([]any); len(changes) == 0 {
		t.Errorf("dry run recorded no changes: %s", out)
	}
	code, out, errOut = runCLI(t, "--json", "omp", "profiles", "list")
	if code != 0 || len(decodeList(t, out)) != 1 {
		t.Errorf("dry-run delete changed the store: exit=%d out=%s", code, out)
	}

	code, _, errOut = runCLI(t, "--json", "omp", "profiles", "delete", "o1")
	if code != 0 {
		t.Fatalf("profile delete exit = %d, stderr = %s", code, errOut)
	}
	code, _, errOut = runCLI(t, "--json", "omp", "profiles", "show", "o1")
	if code != exitRuntime || decode(t, errOut)["error"].(map[string]any)["code"] != "not_found" {
		t.Errorf("show after delete: exit=%d stderr=%s", code, errOut)
	}
}

func TestMcpSaveListToggleDelete(t *testing.T) {
	home := sandbox(t)

	// An empty home lists nothing.
	code, out, errOut := runCLI(t, "mcp", "list")
	if code != 0 {
		t.Fatalf("mcp list exit = %d, stderr = %s", code, errOut)
	}
	if !strings.Contains(out, "no MCP servers") {
		t.Errorf("mcp list = %q", out)
	}

	// Save from a JSON document (the same shape the GUI writes).
	doc := writeFile(t, home, "server.json",
		`{"name":"acme","config":{"type":"stdio","command":"npx","args":["-y","acme-mcp"]}}`)
	code, out, errOut = runCLI(t, "--json", "mcp", "save", "--file", doc)
	if code != 0 {
		t.Fatalf("mcp save exit = %d, stderr = %s", code, errOut)
	}
	if decode(t, out)["applied"] != true {
		t.Errorf("save receipt = %v", out)
	}

	raw, err := os.ReadFile(filepath.Join(home, ".factory", "mcp.json")) // #nosec G304 -- temp fixture
	if err != nil {
		t.Fatalf("read mcp.json: %v", err)
	}
	if !strings.Contains(string(raw), "acme") {
		t.Errorf("mcp.json missing the server: %s", raw)
	}

	code, out, errOut = runCLI(t, "--json", "mcp", "list")
	if code != 0 {
		t.Fatalf("mcp list exit = %d, stderr = %s", code, errOut)
	}
	if servers := decodeList(t, out); len(servers) != 1 {
		t.Fatalf("servers = %v, want one", servers)
	}

	// toggle flips the disabled flag…
	code, out, errOut = runCLI(t, "--json", "mcp", "toggle", "acme")
	if code != 0 {
		t.Fatalf("mcp toggle exit = %d, stderr = %s", code, errOut)
	}
	if decode(t, out)["disabled"] != true {
		t.Errorf("toggle receipt = %v", out)
	}

	// …and a dry run records the write instead of performing it.
	code, out, errOut = runCLI(t, "--json", "--dry-run", "mcp", "toggle", "acme")
	if code != 0 {
		t.Fatalf("dry mcp toggle exit = %d, stderr = %s", code, errOut)
	}
	if decode(t, out)["applied"] != false {
		t.Errorf("dry-run receipt = %v", out)
	}
	raw, err = os.ReadFile(filepath.Join(home, ".factory", "mcp.json")) // #nosec G304 -- temp fixture
	if err != nil {
		t.Fatalf("read mcp.json: %v", err)
	}
	if strings.Count(string(raw), `"disabled": true`) != 1 {
		t.Errorf("dry-run changed mcp.json: %s", raw)
	}

	// show redacts env secrets unless --reveal is passed.
	code, out, errOut = runCLI(t, "--json", "mcp", "show", "acme")
	if code != 0 {
		t.Fatalf("mcp show exit = %d, stderr = %s", code, errOut)
	}
	if !strings.Contains(out, `"disabled": true`) {
		t.Errorf("show = %v", out)
	}

	code, _, errOut = runCLI(t, "--json", "mcp", "delete", "acme")
	if code != 0 {
		t.Fatalf("mcp delete exit = %d, stderr = %s", code, errOut)
	}
	code, out, errOut = runCLI(t, "--json", "mcp", "list")
	if code != 0 {
		t.Fatalf("mcp list exit = %d, stderr = %s", code, errOut)
	}
	if servers := decodeList(t, out); len(servers) != 0 {
		t.Errorf("servers after delete = %v", servers)
	}
}
