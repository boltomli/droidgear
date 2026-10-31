package app

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

// isolatedHome points the app's path resolution at a throwaway home
// directory so smoke tests never touch the real ~/.droidgear, ~/.pi, etc.
//
// LOCALAPPDATA and PATH are isolated as well: HermesHome's Windows
// priority chain reads %LOCALAPPDATA% directly (bypassing USERPROFILE),
// and the final fallback probes the default WSL distro via `wsl.exe`.
// Without these two, TestHermesAndOmpMountSequenceSmoke's ApplyHermesProfile
// rewrote the developer's REAL %LOCALAPPDATA%\hermes\config.yaml — or
// created \\wsl$\...\.hermes inside the real distro — on every `go test`.
func isolatedHome(t *testing.T) string {
	t.Helper()
	home := t.TempDir()
	t.Setenv("USERPROFILE", home)
	t.Setenv("HOME", home)
	t.Setenv("LOCALAPPDATA", filepath.Join(home, "AppData", "Local"))
	// Empty PATH: `wsl.exe` can't be resolved, so defaultHermesHomeWSL
	// skips the WSL branch and falls back to the (isolated) ~/.hermes.
	t.Setenv("PATH", filepath.Join(home, "empty-bin"))
	return home
}

// TestPiMountSequenceSmoke mirrors exactly what the Pi tab runs on mount
// (list → create default when empty → active id → select → status → read
// current). Before the MarshalJSON recursion fix this sequence killed the
// process with a stack overflow as soon as a profile carried providers.
func TestPiMountSequenceSmoke(t *testing.T) {
	isolatedHome(t)
	a := &App{}

	profiles, err := a.ListPiProfiles()
	if err != nil {
		t.Fatalf("ListPiProfiles: %v", err)
	}
	if len(profiles) == 0 {
		if _, err := a.CreateDefaultPiProfile(); err != nil {
			t.Fatalf("CreateDefaultPiProfile: %v", err)
		}
		if profiles, err = a.ListPiProfiles(); err != nil || len(profiles) == 0 {
			t.Fatalf("ListPiProfiles after create: %v (n=%d)", err, len(profiles))
		}
	}

	// Save a profile carrying a provider with an unknown field — the data
	// shape that triggered the crash.
	profile := profiles[0]
	profile.Providers = map[string]PiProviderConfig{
		"openrouter": {
			BaseURL: strPtr("https://openrouter.ai/api/v1"),
			API:     strPtr("openai-completions"),
			APIKey:  strPtr("sk-test"),
			Models:  []PiModel{{ID: "openai/gpt-5.2", Input: []string{"text"}}},
		},
	}
	if err := a.SavePiProfile(profile); err != nil {
		t.Fatalf("SavePiProfile: %v", err)
	}

	// Mount sequence again — this is where the panic used to happen.
	profiles, err = a.ListPiProfiles()
	if err != nil {
		t.Fatalf("ListPiProfiles (2): %v", err)
	}
	if len(profiles) != 1 || len(profiles[0].Providers) != 1 {
		t.Fatalf("saved provider not round-tripped: %+v", profiles)
	}

	if _, err := a.GetActivePiProfileId(); err != nil {
		t.Fatalf("GetActivePiProfileId: %v", err)
	}
	if _, err := a.GetPiProfile(profile.ID); err != nil {
		t.Fatalf("GetPiProfile: %v", err)
	}
	if _, err := a.GetPiConfigStatus(); err != nil {
		t.Fatalf("GetPiConfigStatus: %v", err)
	}

	if err := a.ApplyPiProfile(profile.ID); err != nil {
		t.Fatalf("ApplyPiProfile: %v", err)
	}
	current, err := a.ReadPiCurrentConfig()
	if err != nil {
		t.Fatalf("ReadPiCurrentConfig: %v", err)
	}
	if len(current.Providers) != 1 {
		t.Fatalf("apply did not write providers: %+v", current)
	}

	active, err := a.GetActivePiProfileId()
	if err != nil || active == nil || *active != profile.ID {
		t.Fatalf("active profile not set: %v (%v)", active, err)
	}
}

// TestHermesAndOmpMountSequenceSmoke covers the other two YAML-based tools'
// mount paths (status + read current + custom providers) so a marshal
// regression surfaces in CI rather than as a GUI crash.
func TestHermesAndOmpMountSequenceSmoke(t *testing.T) {
	isolatedHome(t)
	a := &App{}

	if _, err := a.GetHermesConfigStatus(); err != nil {
		t.Fatalf("GetHermesConfigStatus: %v", err)
	}
	if _, err := a.ReadHermesCurrentConfig(); err != nil {
		t.Fatalf("ReadHermesCurrentConfig: %v", err)
	}
	hermesProfile, err := a.CreateDefaultHermesProfile()
	if err != nil {
		t.Fatalf("CreateDefaultHermesProfile: %v", err)
	}
	hermesProfile.Models = []HermesModelConfig{{
		Name:      strPtr("custom1"),
		Default:   strPtr("gpt-5.2"),
		BaseURL:   strPtr("https://x/v1"),
		APIKey:    strPtr("k"),
		IsDefault: true,
	}}
	if err := a.SaveHermesProfile(hermesProfile); err != nil {
		t.Fatalf("SaveHermesProfile: %v", err)
	}
	if err := a.ApplyHermesProfile(hermesProfile.ID); err != nil {
		t.Fatalf("ApplyHermesProfile: %v", err)
	}
	if _, err := a.ReadHermesCurrentConfig(); err != nil {
		t.Fatalf("ReadHermesCurrentConfig (2): %v", err)
	}
	if _, err := a.ListHermesProfiles(); err != nil {
		t.Fatalf("ListHermesProfiles: %v", err)
	}

	if _, err := a.GetOmpConfigStatus(); err != nil {
		t.Fatalf("GetOmpConfigStatus: %v", err)
	}
	if _, err := a.ReadOmpCurrentConfig(); err != nil {
		t.Fatalf("ReadOmpCurrentConfig: %v", err)
	}
	ompProfile, err := a.CreateDefaultOmpProfile()
	if err != nil {
		t.Fatalf("CreateDefaultOmpProfile: %v", err)
	}
	ompProfile.ModelRoles = OmpModelRoles{Default: strPtr("anthropic/claude-sonnet-4-5")}
	if err := a.SaveOmpProfile(ompProfile); err != nil {
		t.Fatalf("SaveOmpProfile: %v", err)
	}
	if err := a.ApplyOmpProfile(ompProfile.ID); err != nil {
		t.Fatalf("ApplyOmpProfile: %v", err)
	}
	if _, err := a.ReadOmpCustomProviders(); err != nil {
		t.Fatalf("ReadOmpCustomProviders: %v", err)
	}
	if _, err := a.ListOmpProfiles(); err != nil {
		t.Fatalf("ListOmpProfiles: %v", err)
	}

	// The factory settings helpers also marshal typed structs.
	if _, err := a.GetConfigPath(); err != nil {
		t.Fatalf("GetConfigPath: %v", err)
	}
	if _, err := a.GetSessionDefaultSettings(); err != nil {
		t.Fatalf("GetSessionDefaultSettings: %v", err)
	}
	if _, err := a.LoadCustomModels(); err != nil {
		t.Fatalf("LoadCustomModels: %v", err)
	}
	if _, err := a.LoadPreferences(); err != nil {
		t.Fatalf("LoadPreferences: %v", err)
	}
}

// TestDshProviderSaveToPatchProfile exercises the provider write path that
// rewrites a profile cordis.patch.yml (the Dsh Models tab): the llm-pi-ai
// entry is inserted while other entries and header comments survive.
func TestDshProviderSaveToPatchProfile(t *testing.T) {
	dshTestHome(t)
	a := &App{}
	profile := "desktop"

	// Pre-existing unrelated entry + header comment must survive the rewrite.
	patchPath := filepath.Join(HomeDirForTest(t), ".dsh", "profiles", "desktop", "cordis.patch.yml")
	if err := os.WriteFile(patchPath, []byte("# template header\n- id: locale\n  config:\n    preference: zh\n"), 0o600); err != nil {
		t.Fatalf("seed patch: %v", err)
	}

	provider := DshProviderConfig{
		DisplayName: strPtr("wududu"),
		BaseURL:     strPtr("https://sub.example.com/v1"),
		APIKeyEnv:   strPtr("WUDUDU_API_KEY"),
		API:         strPtr("openai-completions"),
		Models:      []DshModel{{ID: "deepseek-v4-pro"}},
	}
	if err := a.SaveDshProvider(&profile, "wududu", provider); err != nil {
		t.Fatalf("SaveDshProvider: %v", err)
	}

	current, err := a.ReadDshCurrentConfig(&profile)
	if err != nil {
		t.Fatalf("ReadDshCurrentConfig: %v", err)
	}
	if _, ok := current.Providers["wududu"]; !ok {
		t.Fatalf("provider not round-tripped: %#v", current.Providers)
	}

	raw, _ := os.ReadFile(patchPath)
	text := string(raw)
	if !strings.HasPrefix(text, "# template header\n") {
		t.Errorf("header comment lost: %q", text)
	}
	if !strings.Contains(text, "- id: locale") || !strings.Contains(text, "- id: llm-pi-ai") {
		t.Errorf("entry blocks malformed (missing sequence markers): %s", text)
	}
	if err := a.DeleteDshProvider(&profile, "wududu"); err != nil {
		t.Fatalf("DeleteDshProvider: %v", err)
	}
}

// HomeDirForTest exposes the isolated home directory of the current test.
func HomeDirForTest(t *testing.T) string {
	t.Helper()
	home, err := os.UserHomeDir()
	if err != nil {
		t.Fatalf("home: %v", err)
	}
	return home
}
