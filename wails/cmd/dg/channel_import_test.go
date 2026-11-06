package main

import (
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func seedApiKeyChannel(t *testing.T, home string) {
	t.Helper()
	writeFile(t, home, ".droidgear/channels.json",
		`[{"id":"c1","name":"My Chan","type":"general","baseUrl":"https://api.example.com","enabled":true,"createdAt":0}]`)
	writeFile(t, home, ".droidgear/auth/c1.json",
		`{"type":"api_key","api_key":"sk-test"}`)
}

func TestPiProvidersImport(t *testing.T) {
	home := sandbox(t)
	seedApiKeyChannel(t, home)

	code, out, errOut := runCLI(t, "--json", "pi", "providers", "import",
		"--channel", "c1", "--models", "claude-opus-5-5", "--provider", "imported")
	if code != 0 {
		t.Fatalf("exit = %d, stderr = %s", code, errOut)
	}
	receipt := decode(t, out)
	if receipt["providerId"] != "imported" {
		t.Errorf("providerId = %v", receipt["providerId"])
	}
	if strings.Contains(out, "sk-test") {
		t.Errorf("receipt leaked the API key: %s", out)
	}
	models, _ := receipt["models"].([]any)
	if len(models) != 1 {
		t.Fatalf("models = %v", receipt["models"])
	}

	path := filepath.Join(home, ".pi", "agent", "models.json")
	data, err := os.ReadFile(path) // #nosec G304 -- isolated test path
	if err != nil {
		t.Fatalf("read models.json: %v", err)
	}
	var root map[string]any
	if err := json.Unmarshal(data, &root); err != nil {
		t.Fatalf("unmarshal: %v", err)
	}
	provider := root["providers"].(map[string]any)["imported"].(map[string]any)
	if provider["baseUrl"] != "https://api.example.com/v1" {
		t.Errorf("baseUrl = %v", provider["baseUrl"])
	}
	if provider["api"] != "openai-completions" {
		t.Errorf("api = %v", provider["api"])
	}
	if provider["apiKey"] != "sk-test" {
		t.Errorf("apiKey = %v", provider["apiKey"])
	}
	model := provider["models"].([]any)[0].(map[string]any)
	if model["contextWindow"] != float64(1000000) {
		t.Errorf("contextWindow = %v", model["contextWindow"])
	}
	compat, _ := model["compat"].(map[string]any)
	if compat["forceAdaptiveThinking"] != true {
		t.Errorf("compat = %v, want forceAdaptiveThinking", model["compat"])
	}

	// The list subcommand answers with the live providers (secrets masked).
	code, out, errOut = runCLI(t, "--json", "pi", "providers", "list")
	if code != 0 {
		t.Fatalf("list exit = %d, stderr = %s", code, errOut)
	}
	if strings.Contains(out, "sk-test") {
		t.Errorf("list leaked the API key: %s", out)
	}
}

func TestPiProvidersImportDryRunDoesNotWrite(t *testing.T) {
	home := sandbox(t)
	seedApiKeyChannel(t, home)

	code, out, errOut := runCLI(t, "--json", "--dry-run", "pi", "providers", "import",
		"--channel", "c1", "--models", "claude-opus-5-5", "--provider", "imported")
	if code != 0 {
		t.Fatalf("exit = %d, stderr = %s", code, errOut)
	}
	preview := decode(t, out)
	if preview["applied"] != false {
		t.Errorf("dry-run applied = %v want false", preview["applied"])
	}
	if _, err := os.Stat(filepath.Join(home, ".pi", "agent", "models.json")); !os.IsNotExist(err) {
		t.Errorf("dry run wrote models.json: %v", err)
	}
}

func TestOmpProvidersImport(t *testing.T) {
	home := sandbox(t)
	seedApiKeyChannel(t, home)

	code, out, errOut := runCLI(t, "--json", "omp", "providers", "import",
		"--channel", "c1", "--models", "deepseek-v4.1-flash", "--provider", "imported")
	if code != 0 {
		t.Fatalf("exit = %d, stderr = %s", code, errOut)
	}
	receipt := decode(t, out)
	if receipt["providerId"] != "imported" {
		t.Errorf("providerId = %v", receipt["providerId"])
	}

	path := filepath.Join(home, ".omp", "agent", "models.yml")
	data, err := os.ReadFile(path) // #nosec G304 -- isolated test path
	if err != nil {
		t.Fatalf("read models.yml: %v", err)
	}
	text := string(data)
	for _, want := range []string{"imported:", "baseUrl: https://api.example.com/v1", "api: openai-completions", "deepseek-v4.1-flash"} {
		if !strings.Contains(text, want) {
			t.Errorf("models.yml missing %q:\n%s", want, text)
		}
	}
}
