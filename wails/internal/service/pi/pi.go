package pi

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
	Tiers      *[]PiModelCostTier         `json:"tiers,omitempty"`
	Extra      map[string]json.RawMessage `json:"-"`
}

// PiCompatConfig mirrors pi::PiCompatConfig.
type PiCompatConfig struct {
	SupportsStore                               *bool          `json:"supportsStore,omitempty"`
	SupportsDeveloperRole                       *bool          `json:"supportsDeveloperRole,omitempty"`
	SupportsReasoningEffort                     *bool          `json:"supportsReasoningEffort,omitempty"`
	ReasoningEffortMap                          map[string]any `json:"reasoningEffortMap,omitempty"`
	SupportsUsageInStreaming                    *bool          `json:"supportsUsageInStreaming,omitempty"`
	MaxTokensField                              *string        `json:"maxTokensField,omitempty"`
	RequiresToolResultName                      *bool          `json:"requiresToolResultName,omitempty"`
	RequiresAssistantAfterToolResult            *bool          `json:"requiresAssistantAfterToolResult,omitempty"`
	RequiresThinkingAsText                      *bool          `json:"requiresThinkingAsText,omitempty"`
	RequiresReasoningContentOnAssistantMessages *bool          `json:"requiresReasoningContentOnAssistantMessages,omitempty"`
	ThinkingFormat                              *string        `json:"thinkingFormat,omitempty"`
	CacheControlFormat                          *string        `json:"cacheControlFormat,omitempty"`
	SupportsStrictMode                          *bool          `json:"supportsStrictMode,omitempty"`
	SupportsLongCacheRetention                  *bool          `json:"supportsLongCacheRetention,omitempty"`
	SupportsEagerToolInputStreaming             *bool          `json:"supportsEagerToolInputStreaming,omitempty"`
	ForceAdaptiveThinking                       *bool          `json:"forceAdaptiveThinking,omitempty"`
	OpenRouterRouting                           any            `json:"openRouterRouting,omitempty"`
	VercelGatewayRouting                        any            `json:"vercelGatewayRouting,omitempty"`
	// Upstream additions (pi docs/models-configuration.md):
	SupportsFinishReason            *bool                      `json:"supportsFinishReason,omitempty"`
	RequiresNonNullAssistantContent *bool                      `json:"requiresNonNullAssistantContent,omitempty"`
	ChatTemplateKwargs              any                        `json:"chatTemplateKwargs,omitempty"`
	ChatTemplateArgs                any                        `json:"chatTemplateArgs,omitempty"`
	ZaiToolStream                   *bool                      `json:"zaiToolStream,omitempty"`
	ThinkingTokenBudgetField        *string                    `json:"thinkingTokenBudgetField,omitempty"`
	SupportsThinkingTokenBudget     *bool                      `json:"supportsThinkingTokenBudget,omitempty"`
	SupportsOpenAIGrammarTools      *bool                      `json:"supportsOpenAIGrammarTools,omitempty"`
	SupportsMidConvoSystemMessages  *bool                      `json:"supportsMidConvoSystemMessages,omitempty"`
	SupportsMidConvoToolAdditions   *bool                      `json:"supportsMidConvoToolAdditions,omitempty"`
	SendSessionAffinityHeaders      *bool                      `json:"sendSessionAffinityHeaders,omitempty"`
	SessionAffinityFormat           *string                    `json:"sessionAffinityFormat,omitempty"`
	PromptCacheKeyMode              *string                    `json:"promptCacheKeyMode,omitempty"`
	VllmPriority                    *int                       `json:"vllmPriority,omitempty"`
	SupportsAdditionalTools         *bool                      `json:"supportsAdditionalTools,omitempty"`
	SupportsToolSearch              *bool                      `json:"supportsToolSearch,omitempty"`
	SupportsExplicitPromptCacheMode *bool                      `json:"supportsExplicitPromptCacheMode,omitempty"`
	SupportsMidConvoEffort          *bool                      `json:"supportsMidConvoEffort,omitempty"`
	SupportsMaxOutputTokens         *bool                      `json:"supportsMaxOutputTokens,omitempty"`
	SupportsCacheControlOnTools     *bool                      `json:"supportsCacheControlOnTools,omitempty"`
	SupportsTemperature             *bool                      `json:"supportsTemperature,omitempty"`
	AllowEmptySignature             *bool                      `json:"allowEmptySignature,omitempty"`
	SupportsStrictTools             *bool                      `json:"supportsStrictTools,omitempty"`
	SupportsMidConvoToolChanges     *bool                      `json:"supportsMidConvoToolChanges,omitempty"`
	SupportsMessageThreads          *bool                      `json:"supportsMessageThreads,omitempty"`
	AllowedFallbackModels           any                        `json:"allowedFallbackModels,omitempty"`
	Extra                           map[string]json.RawMessage `json:"-"`
}

// PiModel mirrors pi::PiModel.
type PiModel struct {
	ID string `json:"id"`
	// Type is the operation discriminator (chat | image | classifier);
	// omitted means chat (upstream models-configuration.md).
	Type                          *string                    `json:"type,omitempty"`
	Name                          *string                    `json:"name,omitempty"`
	API                           *string                    `json:"api,omitempty"`
	BaseURL                       *string                    `json:"baseUrl,omitempty"`
	Reasoning                     bool                       `json:"reasoning"`
	ThinkingLevelMap              map[string]*string         `json:"thinkingLevelMap,omitempty"`
	Input                         []string                   `json:"input,omitempty"`
	InputLimits                   any                        `json:"inputLimits,omitempty"`
	ContextWindow                 uint32                     `json:"contextWindow,omitempty"`
	MaxTokens                     uint32                     `json:"maxTokens,omitempty"`
	Cost                          *PiModelCost               `json:"cost,omitempty"`
	PromptCache                   any                        `json:"promptCache,omitempty"`
	SamplingParams                map[string]any             `json:"samplingParams,omitempty"`
	SamplingParamsByThinkingLevel map[string]any             `json:"samplingParamsByThinkingLevel,omitempty"`
	Headers                       map[string]string          `json:"headers,omitempty"`
	Compat                        *PiCompatConfig            `json:"compat,omitempty"`
	Extra                         map[string]json.RawMessage `json:"-"`
}

// PiModelOverride mirrors pi::PiModelOverride.
type PiModelOverride struct {
	Name                          *string                    `json:"name,omitempty"`
	API                           *string                    `json:"api,omitempty"`
	BaseURL                       *string                    `json:"baseUrl,omitempty"`
	Reasoning                     *bool                      `json:"reasoning,omitempty"`
	ThinkingLevelMap              map[string]*string         `json:"thinkingLevelMap,omitempty"`
	Input                         *[]string                  `json:"input,omitempty"`
	InputLimits                   any                        `json:"inputLimits,omitempty"`
	ContextWindow                 *uint32                    `json:"contextWindow,omitempty"`
	MaxTokens                     *uint32                    `json:"maxTokens,omitempty"`
	Cost                          *PiModelCost               `json:"cost,omitempty"`
	PromptCache                   any                        `json:"promptCache,omitempty"`
	SamplingParams                map[string]any             `json:"samplingParams,omitempty"`
	SamplingParamsByThinkingLevel map[string]any             `json:"samplingParamsByThinkingLevel,omitempty"`
	Headers                       map[string]string          `json:"headers,omitempty"`
	Compat                        *PiCompatConfig            `json:"compat,omitempty"`
	Extra                         map[string]json.RawMessage `json:"-"`
}

// PiProviderConfig mirrors pi::PiProviderConfig.
type PiProviderConfig struct {
	Name           *string                    `json:"name,omitempty"`
	BaseURL        *string                    `json:"baseUrl,omitempty"`
	API            *string                    `json:"api,omitempty"`
	APIKey         *string                    `json:"apiKey,omitempty"`
	OAuth          *string                    `json:"oauth,omitempty"`
	Headers        map[string]string          `json:"headers,omitempty"`
	AuthHeader     *bool                      `json:"authHeader,omitempty"`
	Models         []PiModel                  `json:"models"`
	ModelOverrides map[string]PiModelOverride `json:"modelOverrides,omitempty"`
	Compat         *PiCompatConfig            `json:"compat,omitempty"`
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

// PiCurrentConfig mirrors pi::PiCurrentConfig. Schema keeps the optional
// `$schema` root key visible; other unknown root keys survive ApplyProfile's
// read-modify-write merge.
type PiCurrentConfig struct {
	Schema    *string                     `json:"$schema,omitempty"`
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
	a := alias(p)
	// pi's models.json schema requires `models` to be an array. A nil slice
	// would marshal as null and fail validation, so normalize it to [].
	if a.Models == nil {
		a.Models = []PiModel{}
	}
	return core.MarshalJSONWithExtra(a, p.Extra)
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
	if err := core.ValidateProfileID(id); err != nil {
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

func ListProfiles() ([]PiProfile, error) {
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

// GetPiProfile returns one profile by id.
func GetProfile(id string) (PiProfile, error) {
	path, err := piProfilePath(id)
	if err != nil {
		return PiProfile{}, err
	}
	return readPiProfileFile(path)
}

// SavePiProfile creates or updates a profile.
func SaveProfile(profile PiProfile) error {
	if strings.TrimSpace(profile.ID) == "" {
		profile.ID = core.NewUUID()
		profile.CreatedAt = core.NowRFC3339()
	} else {
		if existing, err := GetProfile(profile.ID); err == nil {
			profile.CreatedAt = existing.CreatedAt
		} else if strings.TrimSpace(profile.CreatedAt) == "" {
			profile.CreatedAt = core.NowRFC3339()
		}
	}
	profile.UpdatedAt = core.NowRFC3339()
	return writePiProfileFile(profile)
}

// DeletePiProfile removes a profile and clears the active pointer.
func DeleteProfile(id string) error {
	path, err := piProfilePath(id)
	if err != nil {
		return err
	}
	if core.FileExists(path) {
		if err := core.RemoveFile(path); err != nil {
			return fmt.Errorf("Failed to delete profile: %w", err)
		}
	}
	if active, err := GetActiveProfileID(); err == nil && active != nil && *active == id {
		if activePath, err := piActiveProfilePath(); err == nil {
			_ = core.RemoveFile(activePath)
		}
	}
	return nil
}

// DuplicatePiProfile copies a profile under a new name and id.
func DuplicateProfile(id string, newName string) (PiProfile, error) {
	profile, err := GetProfile(id)
	if err != nil {
		return PiProfile{}, err
	}
	profile.ID = core.NewUUID()
	profile.Name = newName
	profile.CreatedAt = core.NowRFC3339()
	profile.UpdatedAt = profile.CreatedAt
	if err := writePiProfileFile(profile); err != nil {
		return PiProfile{}, err
	}
	return profile, nil
}

// CreateDefaultPiProfile seeds the first profile when none exist.
func CreateDefaultProfile() (PiProfile, error) {
	profiles, err := ListProfiles()
	if err != nil {
		return PiProfile{}, err
	}
	if len(profiles) > 0 {
		return PiProfile{}, errors.New("Profiles already exist")
	}
	now := core.NowRFC3339()
	profile := PiProfile{
		ID:        core.NewUUID(),
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

// GetActiveProfileID returns the active profile id (null when unset).
func GetActiveProfileID() (*string, error) {
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

// SetActiveProfileID stores the active profile id.
func SetActiveProfileID(id string) error {
	path, err := piActiveProfilePath()
	if err != nil {
		return err
	}
	return core.AtomicWrite(path, core.BytesOf(id))
}

// ApplyPiProfile writes the profile's providers to ~/.pi/agent/models.json.
// It merges over the live document instead of rebuilding it so upstream
// fields this build does not model survive (and an optional `$schema` root
// key is kept). Providers absent from the profile are removed.
func ApplyProfile(id string) error {
	profile, err := GetProfile(id)
	if err != nil {
		return err
	}
	configPath, err := piConfigPath()
	if err != nil {
		return err
	}
	live, err := readLiveConfigForMerge(configPath)
	if err != nil {
		return err
	}
	providers := profile.Providers
	if providers == nil {
		providers = map[string]PiProviderConfig{}
	}
	data, err := mergePiConfig(live, providers, false)
	if err != nil {
		return err
	}
	if err := core.AtomicWrite(configPath, data); err != nil {
		return err
	}
	return SetActiveProfileID(id)
}

// GetPiConfigStatus reports models.json state.
func GetConfigStatus() (PiConfigStatus, error) {
	path, err := piConfigPath()
	if err != nil {
		return PiConfigStatus{}, err
	}
	return PiConfigStatus{ConfigExists: core.FileExists(path), ConfigPath: path}, nil
}

// ReadPiCurrentConfig parses ~/.pi/agent/models.json.
func ReadCurrentConfig() (PiCurrentConfig, error) {
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
func TestProviderConnection(providerID string, config PiProviderConfig) (PiProviderTestResult, error) {
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
