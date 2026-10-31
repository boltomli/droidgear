package app

import (
	"os"
	"path/filepath"
	"testing"
)

// writeHermesTestConfig drops a config.yaml into the isolated ~/.hermes so
// the Windows priority chain resolves to it deterministically.
func writeHermesTestConfig(t *testing.T, content string) string {
	t.Helper()
	dir := filepath.Join(mustHome(t), ".hermes")
	if err := os.MkdirAll(dir, 0o755); err != nil {
		t.Fatalf("mkdir: %v", err)
	}
	path := filepath.Join(dir, "config.yaml")
	if err := os.WriteFile(path, []byte(content), 0o644); err != nil {
		t.Fatalf("write config: %v", err)
	}
	return path
}

func mustHome(t *testing.T) string {
	t.Helper()
	home, err := os.UserHomeDir()
	if err != nil || home == "" {
		t.Fatalf("home dir: %v", err)
	}
	return home
}

// TestHermesReadCurrentKeepsEndpointForUnknownNamedProvider covers the load
// half of the "loads a wrong config and later destroys it" chain: when the
// live model section references `custom:<name>` but custom_providers has no
// matching entry, the loaded entry must still carry the model section's
// base_url/api_key.
func TestHermesReadCurrentKeepsEndpointForUnknownNamedProvider(t *testing.T) {
	isolatedHome(t)
	a := &App{}

	writeHermesTestConfig(t, `
model:
  default: gpt-ghost
  provider: custom:ghost
  base_url: https://ghost.example.com/v1
  api_key: sk-ghost
`)

	current, err := a.ReadHermesCurrentConfig()
	if err != nil {
		t.Fatalf("ReadHermesCurrentConfig: %v", err)
	}
	if len(current.Models) != 1 {
		t.Fatalf("expected 1 model, got %d: %+v", len(current.Models), current.Models)
	}
	loaded := current.Models[0]
	if loaded.Name == nil || *loaded.Name != "ghost" {
		t.Fatalf("expected name=ghost, got %+v", loaded)
	}
	if loaded.BaseURL == nil || *loaded.BaseURL != "https://ghost.example.com/v1" {
		t.Fatalf("base_url lost on load: %+v", loaded)
	}
	if loaded.APIKey == nil || *loaded.APIKey != "sk-ghost" {
		t.Fatalf("api_key lost on load: %+v", loaded)
	}
}

// TestHermesLoadThenApplyDoesNotStripEndpoint is the write half: loading a
// ghost `custom:<name>` config into a profile and applying it must upsert
// the endpoint into custom_providers instead of stripping base_url/api_key
// from the model section (which left Hermes with no endpoint at all).
func TestHermesLoadThenApplyDoesNotStripEndpoint(t *testing.T) {
	isolatedHome(t)
	a := &App{}

	configPath := writeHermesTestConfig(t, `
model:
  default: gpt-ghost
  provider: custom:ghost
  base_url: https://ghost.example.com/v1
  api_key: sk-ghost
`)

	// Mirror the GUI flow: load from live config, then apply the profile.
	current, err := a.ReadHermesCurrentConfig()
	if err != nil {
		t.Fatalf("ReadHermesCurrentConfig: %v", err)
	}
	profile := HermesProfile{
		ID:        "p1",
		Name:      "ghost",
		CreatedAt: nowRFC3339(),
		UpdatedAt: nowRFC3339(),
		Models:    current.Models,
	}
	if err := a.SaveHermesProfile(profile); err != nil {
		t.Fatalf("SaveHermesProfile: %v", err)
	}
	if err := a.ApplyHermesProfile("p1"); err != nil {
		t.Fatalf("ApplyHermesProfile: %v", err)
	}

	after, err := a.ReadHermesCurrentConfig()
	if err != nil {
		t.Fatalf("ReadHermesCurrentConfig (after): %v", err)
	}
	if len(after.Models) != 1 {
		t.Fatalf("expected 1 model after apply, got %d: %+v", len(after.Models), after.Models)
	}
	got := after.Models[0]
	if got.BaseURL == nil || *got.BaseURL != "https://ghost.example.com/v1" {
		t.Fatalf("apply stripped base_url: %+v (config: %s)", got, mustRead(t, configPath))
	}
	if got.APIKey == nil || *got.APIKey != "sk-ghost" {
		t.Fatalf("apply stripped api_key: %+v (config: %s)", got, mustRead(t, configPath))
	}
	if got.Default == nil || *got.Default != "gpt-ghost" {
		t.Fatalf("apply lost model.default: %+v", got)
	}
}

// TestHermesReadCommandsDoNotCreateConfigDir pins the no-side-effect
// contract: status/read must not create ~/.hermes (nor any fallback dir).
// A premature dir at a priority location changes what the next resolution
// picks, which is how read paths used to "configure themselves".
func TestHermesReadCommandsDoNotCreateConfigDir(t *testing.T) {
	isolatedHome(t)
	a := &App{}

	status, err := a.GetHermesConfigStatus()
	if err != nil {
		t.Fatalf("GetHermesConfigStatus: %v", err)
	}
	if status.ConfigExists {
		t.Fatalf("config should not exist in a fresh home: %+v", status)
	}
	if _, err := a.ReadHermesCurrentConfig(); err != nil {
		t.Fatalf("ReadHermesCurrentConfig: %v", err)
	}

	hermesDir := filepath.Join(mustHome(t), ".hermes")
	if _, err := os.Stat(hermesDir); err == nil {
		t.Fatalf("read-only commands created %s", hermesDir)
	}
}

func mustRead(t *testing.T, path string) string {
	t.Helper()
	data, err := os.ReadFile(path) // #nosec G304 -- test fixture
	if err != nil {
		t.Fatalf("read %s: %v", path, err)
	}
	return string(data)
}
