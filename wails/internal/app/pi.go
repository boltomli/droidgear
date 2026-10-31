package app

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"sort"
	"strings"
	"time"

	"droidgear-wails/internal/core"
)

// ---------------------------------------------------------------------------
// Types (mirroring droidgear-core::pi)
// ---------------------------------------------------------------------------

// PiModelCostTier mirrors pi::PiModelCostTier.
type PiModelCostTier struct {
	InputTokensAbove uint32  `json:"inputTokensAbove"`
	Input            float64 `json:"input"`
	Output           float64 `json:"output"`
	CacheRead        float64 `json:"cacheRead"`
	CacheWrite       float64 `json:"cacheWrite"`
}

// PiModelCost mirrors pi::PiModelCost.
type PiModelCost struct {
	Input      float64                    `json:"input"`
	Output     float64                    `json:"output"`
	CacheRead  float64                    `json:"cacheRead"`
	CacheWrite float64                    `json:"cacheWrite"`
	Tiers      *[]PiModelCostTier         `json:"tiers"`
	Extra      map[string]json.RawMessage `json:"-"`
}

// PiCompatConfig mirrors pi::PiCompatConfig.
type PiCompatConfig struct {
	SupportsStore                               *bool                      `json:"supportsStore"`
	SupportsDeveloperRole                       *bool                      `json:"supportsDeveloperRole"`
	SupportsReasoningEffort                     *bool                      `json:"supportsReasoningEffort"`
	ReasoningEffortMap                          map[string]any             `json:"reasoningEffortMap"`
	SupportsUsageInStreaming                    *bool                      `json:"supportsUsageInStreaming"`
	MaxTokensField                              *string                    `json:"maxTokensField"`
	RequiresToolResultName                      *bool                      `json:"requiresToolResultName"`
	RequiresAssistantAfterToolResult            *bool                      `json:"requiresAssistantAfterToolResult"`
	RequiresThinkingAsText                      *bool                      `json:"requiresThinkingAsText"`
	RequiresReasoningContentOnAssistantMessages *bool                      `json:"requiresReasoningContentOnAssistantMessages"`
	ThinkingFormat                              *string                    `json:"thinkingFormat"`
	CacheControlFormat                          *string                    `json:"cacheControlFormat"`
	SupportsStrictMode                          *bool                      `json:"supportsStrictMode"`
	SupportsLongCacheRetention                  *bool                      `json:"supportsLongCacheRetention"`
	SupportsEagerToolInputStreaming             *bool                      `json:"supportsEagerToolInputStreaming"`
	ForceAdaptiveThinking                       *bool                      `json:"forceAdaptiveThinking"`
	OpenRouterRouting                           any                        `json:"openRouterRouting"`
	VercelGatewayRouting                        any                        `json:"vercelGatewayRouting"`
	Extra                                       map[string]json.RawMessage `json:"-"`
}

// PiModel mirrors pi::PiModel.
type PiModel struct {
	ID               string                     `json:"id"`
	Name             *string                    `json:"name"`
	API              *string                    `json:"api"`
	Reasoning        bool                       `json:"reasoning"`
	ThinkingLevelMap map[string]*string         `json:"thinkingLevelMap"`
	Input            []string                   `json:"input"`
	ContextWindow    uint32                     `json:"contextWindow"`
	MaxTokens        uint32                     `json:"maxTokens"`
	Cost             *PiModelCost               `json:"cost"`
	Headers          map[string]string          `json:"headers"`
	Compat           *PiCompatConfig            `json:"compat"`
	Extra            map[string]json.RawMessage `json:"-"`
}

// PiModelOverride mirrors pi::PiModelOverride.
type PiModelOverride struct {
	Name             *string                    `json:"name"`
	API              *string                    `json:"api"`
	Reasoning        *bool                      `json:"reasoning"`
	ThinkingLevelMap map[string]*string         `json:"thinkingLevelMap"`
	Input            *[]string                  `json:"input"`
	ContextWindow    *uint32                    `json:"contextWindow"`
	MaxTokens        *uint32                    `json:"maxTokens"`
	Cost             *PiModelCost               `json:"cost"`
	Headers          map[string]string          `json:"headers"`
	Compat           *PiCompatConfig            `json:"compat"`
	Extra            map[string]json.RawMessage `json:"-"`
}

// PiProviderConfig mirrors pi::PiProviderConfig.
type PiProviderConfig struct {
	BaseURL        *string                    `json:"baseUrl"`
	API            *string                    `json:"api"`
	APIKey         *string                    `json:"apiKey"`
	OAuth          *string                    `json:"oauth"`
	Headers        map[string]string          `json:"headers"`
	AuthHeader     *bool                      `json:"authHeader"`
	Models         []PiModel                  `json:"models"`
	ModelOverrides map[string]PiModelOverride `json:"modelOverrides"`
	Compat         *PiCompatConfig            `json:"compat"`
	Extra          map[string]json.RawMessage `json:"-"`
}

// PiProfile mirrors pi::PiProfile.
type PiProfile struct {
	ID          string                      `json:"id"`
	Name        string                      `json:"name"`
	Description *string                     `json:"description"`
	CreatedAt   string                      `json:"createdAt"`
	UpdatedAt   string                      `json:"updatedAt"`
	Providers   map[string]PiProviderConfig `json:"providers"`
}

// PiConfigStatus mirrors pi::PiConfigStatus.
type PiConfigStatus struct {
	ConfigExists bool   `json:"configExists"`
	ConfigPath   string `json:"configPath"`
}

// PiCurrentConfig mirrors pi::PiCurrentConfig.
type PiCurrentConfig struct {
	Providers map[string]PiProviderConfig `json:"providers"`
}

// PiProviderTestResult mirrors pi::PiProviderTestResult.
type PiProviderTestResult struct {
	Success      bool    `json:"success"`
	ProviderID   string  `json:"providerId"`
	ModelID      string  `json:"modelId"`
	LatencyMS    uint64  `json:"latencyMs"`
	ResponseText *string `json:"responseText"`
	Error        *string `json:"error"`
}

// ---------------------------------------------------------------------------
// JSON extra preservation
// ---------------------------------------------------------------------------

func (c PiModelCost) MarshalJSON() ([]byte, error) {
	type alias PiModelCost
	return core.MarshalJSONWithExtra(alias(c), c.Extra)
}

func (c *PiModelCost) UnmarshalJSON(data []byte) error {
	type plain PiModelCost
	var p plain
	if err := core.UnmarshalJSONWithExtra(data, &p); err != nil {
		return err
	}
	*c = PiModelCost(p)
	return nil
}

func (c PiCompatConfig) MarshalJSON() ([]byte, error) {
	type alias PiCompatConfig
	return core.MarshalJSONWithExtra(alias(c), c.Extra)
}

func (c *PiCompatConfig) UnmarshalJSON(data []byte) error {
	type plain PiCompatConfig
	var p plain
	if err := core.UnmarshalJSONWithExtra(data, &p); err != nil {
		return err
	}
	*c = PiCompatConfig(p)
	return nil
}

func (m PiModel) MarshalJSON() ([]byte, error) {
	type alias PiModel
	return core.MarshalJSONWithExtra(alias(m), m.Extra)
}

func (m *PiModel) UnmarshalJSON(data []byte) error {
	type plain PiModel
	var p plain
	if err := core.UnmarshalJSONWithExtra(data, &p); err != nil {
		return err
	}
	*m = PiModel(p)
	return nil
}

func (m PiModelOverride) MarshalJSON() ([]byte, error) {
	type alias PiModelOverride
	return core.MarshalJSONWithExtra(alias(m), m.Extra)
}

func (m *PiModelOverride) UnmarshalJSON(data []byte) error {
	type plain PiModelOverride
	var p plain
	if err := core.UnmarshalJSONWithExtra(data, &p); err != nil {
		return err
	}
	*m = PiModelOverride(p)
	return nil
}

func (p PiProviderConfig) MarshalJSON() ([]byte, error) {
	type alias PiProviderConfig
	return core.MarshalJSONWithExtra(alias(p), p.Extra)
}

func (p *PiProviderConfig) UnmarshalJSON(data []byte) error {
	type plain PiProviderConfig
	var v plain
	if err := core.UnmarshalJSONWithExtra(data, &v); err != nil {
		return err
	}
	*p = PiProviderConfig(v)
	return nil
}

// ---------------------------------------------------------------------------
// Paths
// ---------------------------------------------------------------------------

func piDroidGearDir() (string, error) {
	dir, err := core.DroidGearDir()
	if err != nil {
		return "", err
	}
	return filepath.Join(dir, "pi"), nil
}

func piProfilesDir() (string, error) {
	dir, err := piDroidGearDir()
	if err != nil {
		return "", err
	}
	return filepath.Join(dir, "profiles"), nil
}

func piActiveProfilePath() (string, error) {
	dir, err := piDroidGearDir()
	if err != nil {
		return "", err
	}
	return filepath.Join(dir, "active-profile.txt"), nil
}

func piConfigDir() (string, error) {
	dir := core.PiHome()
	if err := core.EnsureDir(dir); err != nil {
		return "", fmt.Errorf("Failed to create pi config directory: %w", err)
	}
	return dir, nil
}

func piConfigPath() (string, error) {
	dir, err := piConfigDir()
	if err != nil {
		return "", err
	}
	return filepath.Join(dir, "models.json"), nil
}

func piProfilePath(id string) (string, error) {
	if err := validateProfileID(id); err != nil {
		return "", err
	}
	dir, err := piProfilesDir()
	if err != nil {
		return "", err
	}
	return filepath.Join(dir, id+".json"), nil
}

// ---------------------------------------------------------------------------
// Profile CRUD helpers
// ---------------------------------------------------------------------------

func readPiProfileFile(path string) (PiProfile, error) {
	data, err := os.ReadFile(path) // #nosec G304 -- profile path
	if err != nil {
		return PiProfile{}, fmt.Errorf("Failed to read profile: %w", err)
	}
	var profile PiProfile
	if err := json.Unmarshal(data, &profile); err != nil {
		return PiProfile{}, fmt.Errorf("Invalid profile JSON: %w", err)
	}
	return profile, nil
}

func writePiProfileFile(profile PiProfile) error {
	path, err := piProfilePath(profile.ID)
	if err != nil {
		return err
	}
	data, err := json.MarshalIndent(profile, "", "  ")
	if err != nil {
		return fmt.Errorf("Failed to serialize profile JSON: %w", err)
	}
	return core.AtomicWrite(path, data)
}

func listPiProfiles() ([]PiProfile, error) {
	dir, err := piProfilesDir()
	if err != nil {
		return nil, err
	}
	if !core.DirExists(dir) {
		return []PiProfile{}, nil
	}
	entries, err := os.ReadDir(dir)
	if err != nil {
		return nil, fmt.Errorf("Failed to read profiles dir: %w", err)
	}
	profiles := []PiProfile{}
	for _, entry := range entries {
		if entry.IsDir() || !strings.HasSuffix(entry.Name(), ".json") {
			continue
		}
		if profile, err := readPiProfileFile(filepath.Join(dir, entry.Name())); err == nil {
			profiles = append(profiles, profile)
		}
	}
	sort.Slice(profiles, func(i, j int) bool {
		return strings.ToLower(profiles[i].Name) < strings.ToLower(profiles[j].Name)
	})
	return profiles, nil
}

// ---------------------------------------------------------------------------
// Commands
// ---------------------------------------------------------------------------

// ListPiProfiles lists all Pi profiles sorted by name.
func (a *App) ListPiProfiles() ([]PiProfile, error) {
	return listPiProfiles()
}

// GetPiProfile returns one profile by id.
func (a *App) GetPiProfile(id string) (PiProfile, error) {
	path, err := piProfilePath(id)
	if err != nil {
		return PiProfile{}, err
	}
	return readPiProfileFile(path)
}

// SavePiProfile creates or updates a profile.
func (a *App) SavePiProfile(profile PiProfile) error {
	if strings.TrimSpace(profile.ID) == "" {
		profile.ID = newUUID()
		profile.CreatedAt = nowRFC3339()
	} else {
		if existing, err := a.GetPiProfile(profile.ID); err == nil {
			profile.CreatedAt = existing.CreatedAt
		} else if strings.TrimSpace(profile.CreatedAt) == "" {
			profile.CreatedAt = nowRFC3339()
		}
	}
	profile.UpdatedAt = nowRFC3339()
	return writePiProfileFile(profile)
}

// DeletePiProfile removes a profile and clears the active pointer.
func (a *App) DeletePiProfile(id string) error {
	path, err := piProfilePath(id)
	if err != nil {
		return err
	}
	if fileExists(path) {
		if err := os.Remove(path); err != nil {
			return fmt.Errorf("Failed to delete profile: %w", err)
		}
	}
	if active, err := a.GetActivePiProfileId(); err == nil && active != nil && *active == id {
		if activePath, err := piActiveProfilePath(); err == nil {
			_ = os.Remove(activePath)
		}
	}
	return nil
}

// DuplicatePiProfile copies a profile under a new name and id.
func (a *App) DuplicatePiProfile(id string, newName string) (PiProfile, error) {
	profile, err := a.GetPiProfile(id)
	if err != nil {
		return PiProfile{}, err
	}
	profile.ID = newUUID()
	profile.Name = newName
	profile.CreatedAt = nowRFC3339()
	profile.UpdatedAt = profile.CreatedAt
	if err := writePiProfileFile(profile); err != nil {
		return PiProfile{}, err
	}
	return profile, nil
}

// CreateDefaultPiProfile seeds the first profile when none exist.
func (a *App) CreateDefaultPiProfile() (PiProfile, error) {
	profiles, err := listPiProfiles()
	if err != nil {
		return PiProfile{}, err
	}
	if len(profiles) > 0 {
		return PiProfile{}, errors.New("Profiles already exist")
	}
	now := nowRFC3339()
	profile := PiProfile{
		ID:        newUUID(),
		Name:      "Default",
		CreatedAt: now,
		UpdatedAt: now,
		Providers: map[string]PiProviderConfig{},
	}
	if err := writePiProfileFile(profile); err != nil {
		return PiProfile{}, err
	}
	return profile, nil
}

// GetActivePiProfileId returns the active profile id (null when unset).
func (a *App) GetActivePiProfileId() (*string, error) {
	path, err := piActiveProfilePath()
	if err != nil {
		return nil, err
	}
	data, err := os.ReadFile(path) // #nosec G304 -- active profile pointer
	if err != nil {
		return nil, nil
	}
	id := strings.TrimSpace(string(data))
	if id == "" {
		return nil, nil
	}
	return &id, nil
}

// SetActivePiProfileId stores the active profile id.
func (a *App) SetActivePiProfileId(id string) error {
	path, err := piActiveProfilePath()
	if err != nil {
		return err
	}
	return core.AtomicWrite(path, core.BytesOf(id))
}

// ApplyPiProfile writes the profile's providers to ~/.pi/agent/models.json.
func (a *App) ApplyPiProfile(id string) error {
	profile, err := a.GetPiProfile(id)
	if err != nil {
		return err
	}
	configPath, err := piConfigPath()
	if err != nil {
		return err
	}
	current := PiCurrentConfig{Providers: profile.Providers}
	if current.Providers == nil {
		current.Providers = map[string]PiProviderConfig{}
	}
	data, err := json.MarshalIndent(current, "", "  ")
	if err != nil {
		return fmt.Errorf("Failed to serialize Pi config: %w", err)
	}
	if err := core.AtomicWrite(configPath, data); err != nil {
		return err
	}
	return a.SetActivePiProfileId(id)
}

// GetPiConfigStatus reports models.json state.
func (a *App) GetPiConfigStatus() (PiConfigStatus, error) {
	path, err := piConfigPath()
	if err != nil {
		return PiConfigStatus{}, err
	}
	return PiConfigStatus{ConfigExists: fileExists(path), ConfigPath: path}, nil
}

// ReadPiCurrentConfig parses ~/.pi/agent/models.json.
func (a *App) ReadPiCurrentConfig() (PiCurrentConfig, error) {
	path, err := piConfigPath()
	if err != nil {
		return PiCurrentConfig{}, err
	}
	data, err := os.ReadFile(path) // #nosec G304 -- tool config path
	if err != nil {
		if os.IsNotExist(err) {
			return PiCurrentConfig{Providers: map[string]PiProviderConfig{}}, nil
		}
		return PiCurrentConfig{}, fmt.Errorf("Failed to read Pi config file: %w", err)
	}
	var config PiCurrentConfig
	if err := json.Unmarshal(data, &config); err != nil {
		return PiCurrentConfig{}, fmt.Errorf("Invalid Pi config JSON: %w", err)
	}
	if config.Providers == nil {
		config.Providers = map[string]PiProviderConfig{}
	}
	return config, nil
}

// ---------------------------------------------------------------------------
// Provider connection test (launches the pi CLI against an isolated config)
// ---------------------------------------------------------------------------

const (
	piTestTimeout     = 60 * time.Second
	piTestOutputLimit = 2000
	piTestPrompt      = "Reply only with OK."
)

func piTestArgs(providerID, modelID string) []string {
	return []string{
		"--provider", providerID,
		"--model", modelID,
		"--thinking", "off",
		"--system-prompt", piTestPrompt,
		"--no-session",
		"--no-tools",
		"--no-extensions",
		"--no-skills",
		"--no-prompt-templates",
		"--no-themes",
		"--no-context-files",
		"--no-approve",
		"--offline",
		"--print",
		piTestPrompt,
	}
}

func firstNonEmptyModelID(config PiProviderConfig) (string, error) {
	for _, model := range config.Models {
		if strings.TrimSpace(model.ID) != "" {
			return model.ID, nil
		}
	}
	return "", errors.New("Pi provider must define at least one model before testing")
}

func findExecutable(program string) (string, bool) {
	path := os.Getenv("PATH")
	for _, dir := range filepath.SplitList(path) {
		candidate := filepath.Join(dir, program)
		if info, err := os.Stat(candidate); err == nil && !info.IsDir() {
			return candidate, true
		}
		for _, ext := range []string{".exe", ".cmd", ".bat"} {
			candidate := filepath.Join(dir, program+ext)
			if info, err := os.Stat(candidate); err == nil && !info.IsDir() {
				return candidate, true
			}
		}
	}
	return "", false
}

func redactProviderSecrets(value string, config PiProviderConfig) string {
	redacted := value
	secrets := []string{}
	if config.APIKey != nil {
		secrets = append(secrets, *config.APIKey)
	}
	for _, header := range config.Headers {
		secrets = append(secrets, header)
	}
	for _, secret := range secrets {
		trimmed := strings.TrimSpace(secret)
		if len(trimmed) >= 4 && !strings.HasPrefix(trimmed, "$") && !strings.HasPrefix(trimmed, "!") {
			redacted = strings.ReplaceAll(redacted, trimmed, "[REDACTED]")
		}
	}
	if len(redacted) > piTestOutputLimit {
		runes := []rune(redacted)
		if len(runes) > piTestOutputLimit {
			redacted = string(runes[:piTestOutputLimit])
		}
	}
	return strings.TrimSpace(redacted)
}

// TestPiProviderConnection validates a provider by launching the pi CLI
// against an isolated temporary models.json.
func (a *App) TestPiProviderConnection(providerID string, config PiProviderConfig) (PiProviderTestResult, error) {
	providerID = strings.TrimSpace(providerID)
	if providerID == "" {
		return PiProviderTestResult{}, errors.New("Pi provider ID cannot be empty")
	}
	modelID, err := firstNonEmptyModelID(config)
	if err != nil {
		return PiProviderTestResult{}, err
	}
	program, ok := findExecutable("pi")
	if !ok {
		return PiProviderTestResult{}, errors.New("Pi CLI is not installed or not available in PATH")
	}

	tempDir, err := os.MkdirTemp("", "droidgear-pi-test-")
	if err != nil {
		return PiProviderTestResult{}, fmt.Errorf("Failed to create temporary Pi config directory: %w", err)
	}
	defer func() { _ = os.RemoveAll(tempDir) }()

	testConfig := PiCurrentConfig{Providers: map[string]PiProviderConfig{providerID: config}}
	modelsJSON, err := json.MarshalIndent(testConfig, "", "  ")
	if err != nil {
		return PiProviderTestResult{}, fmt.Errorf("Failed to serialize temporary Pi config: %w", err)
	}
	if err := os.WriteFile(filepath.Join(tempDir, "models.json"), modelsJSON, 0o600); err != nil { // #nosec G306 -- temp secret file
		return PiProviderTestResult{}, err
	}
	settingsJSON := []byte(`{
  "retry": {
    "enabled": false,
    "provider": {
      "timeoutMs": 45000,
      "maxRetries": 0,
      "maxRetryDelayMs": 30000
    }
  },
  "httpIdleTimeoutMs": 45000,
  "defaultProjectTrust": "never",
  "enableInstallTelemetry": false
}`)
	if err := os.WriteFile(filepath.Join(tempDir, "settings.json"), settingsJSON, 0o600); err != nil { // #nosec G306 -- temp secret file
		return PiProviderTestResult{}, err
	}

	ctx, cancel := context.WithTimeout(context.Background(), piTestTimeout)
	defer cancel()

	startedAt := time.Now()
	cmd := exec.CommandContext(ctx, program, piTestArgs(providerID, modelID)...) // #nosec G204 -- user-configured CLI
	cmd.Dir = tempDir
	cmd.Env = append(os.Environ(),
		"PI_CODING_AGENT_DIR="+tempDir,
		"PI_CODING_AGENT_SESSION_DIR="+filepath.Join(tempDir, "sessions"),
		"PI_SKIP_VERSION_CHECK=1",
		"PI_TELEMETRY=0",
	)
	output, runErr := cmd.Output()
	latencyMS := uint64(time.Since(startedAt).Milliseconds())

	if runErr != nil && ctx.Err() == context.DeadlineExceeded {
		message := fmt.Sprintf("Pi connection test timed out after %d seconds", uint64(piTestTimeout/time.Second))
		return PiProviderTestResult{
			Success: false, ProviderID: providerID, ModelID: modelID, LatencyMS: latencyMS, Error: &message,
		}, nil
	}

	stderr := ""
	var exitErr *exec.ExitError
	if errors.As(runErr, &exitErr) {
		stderr = string(exitErr.Stderr)
	}

	if runErr == nil {
		response := redactProviderSecrets(string(output), config)
		result := PiProviderTestResult{
			Success: true, ProviderID: providerID, ModelID: modelID, LatencyMS: latencyMS,
		}
		if response != "" {
			result.ResponseText = &response
		}
		return result, nil
	}

	raw := stderr
	if strings.TrimSpace(raw) == "" {
		raw = string(output)
	}
	redacted := redactProviderSecrets(raw, config)
	if redacted == "" {
		redacted = "Pi CLI exited without an error message"
	}
	return PiProviderTestResult{
		Success: false, ProviderID: providerID, ModelID: modelID, LatencyMS: latencyMS, Error: &redacted,
	}, nil
}
