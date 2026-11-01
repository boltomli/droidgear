// GitHub Copilot CLI profile and BYOK launch management. Mirrors
// droidgear-core::copilot + copilot_runtime (Rust): profile CRUD under
// ~/.droidgear/copilot/, materialising the selected profile as a local
// config.env, and launching the copilot CLI with a per-profile environment
// overlay in the user's preferred terminal.
package app

import (
	"bytes"
	"encoding/json"
	"errors"
	"fmt"
	"net/url"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"sort"
	"strconv"
	"strings"
	"sync/atomic"
	"syscall"
	"time"

	"droidgear-wails/internal/core"
)

// ---------------------------------------------------------------------------
// Types (mirroring droidgear-core::copilot)
// ---------------------------------------------------------------------------

// CopilotProfile mirrors copilot::CopilotProfile.
type CopilotProfile struct {
	ID              string  `json:"id"`
	Name            string  `json:"name"`
	Description     *string `json:"description,omitempty"`
	CreatedAt       string  `json:"createdAt"`
	UpdatedAt       string  `json:"updatedAt"`
	UseOfficialAuth bool    `json:"useOfficialAuth"`
	BaseURL         *string `json:"baseUrl,omitempty"`
	ProviderType    *string `json:"providerType,omitempty"`
	APIKey          *string `json:"apiKey,omitempty"`
	Model           *string `json:"model,omitempty"`
	MaxPromptTokens *uint64 `json:"maxPromptTokens,omitempty"`
	MaxOutputTokens *uint64 `json:"maxOutputTokens,omitempty"`
}

// CopilotConfigStatus mirrors copilot::CopilotConfigStatus.
type CopilotConfigStatus struct {
	ConfigExists bool   `json:"configExists"`
	ConfigPath   string `json:"configPath"`
}

// CopilotCurrentConfig mirrors copilot::CopilotCurrentConfig.
type CopilotCurrentConfig struct {
	IsByok          bool    `json:"isByok"`
	BaseURL         *string `json:"baseUrl,omitempty"`
	ProviderType    *string `json:"providerType,omitempty"`
	APIKey          *string `json:"apiKey,omitempty"`
	Model           *string `json:"model,omitempty"`
	MaxPromptTokens *uint64 `json:"maxPromptTokens,omitempty"`
	MaxOutputTokens *uint64 `json:"maxOutputTokens,omitempty"`
}

// CopilotChannelSelection mirrors copilot::CopilotChannelSelection.
// ChannelType/Provider are string enums in bindings.ts.
type CopilotChannelSelection struct {
	ChannelType     string  `json:"channelType"`
	BaseURL         string  `json:"baseUrl"`
	APIKey          string  `json:"apiKey"`
	Platform        *string `json:"platform"`
	Provider        *string `json:"provider"`
	Model           string  `json:"model"`
	MaxOutputTokens *uint64 `json:"maxOutputTokens"`
}

// ---------------------------------------------------------------------------
// Env key names (copilot.rs consts)
// ---------------------------------------------------------------------------

const (
	copilotEnvOffline         = "COPILOT_OFFLINE"
	copilotEnvAuthToken       = "COPILOT_AUTH_TOKEN"
	copilotEnvProviderBaseURL = "COPILOT_PROVIDER_BASE_URL"
	copilotEnvProviderType    = "COPILOT_PROVIDER_TYPE"
	copilotEnvProviderAPIKey  = "COPILOT_PROVIDER_API_KEY"
	copilotEnvModel           = "COPILOT_MODEL"
	copilotEnvMaxPromptTokens = "COPILOT_PROVIDER_MAX_PROMPT_TOKENS"
	copilotEnvMaxOutputTokens = "COPILOT_PROVIDER_MAX_OUTPUT_TOKENS"
)

// copilotInheritedEnvKeys are cleared from the terminal environment before
// launching so a stale global configuration can never win over the profile.
var copilotInheritedEnvKeys = []string{
	copilotEnvOffline,
	copilotEnvProviderBaseURL,
	copilotEnvProviderType,
	copilotEnvProviderAPIKey,
	copilotEnvModel,
	copilotEnvMaxPromptTokens,
	copilotEnvMaxOutputTokens,
	"COPILOT_PROVIDER_MODEL_ID",
	"COPILOT_PROVIDER_WIRE_MODEL",
	"COPILOT_PROVIDER_BEARER_TOKEN",
	"COPILOT_PROVIDER_API_KEY_COMMAND",
	"COPILOT_PROVIDER_HEADERS",
	"COPILOT_PROVIDER_WIRE_API",
	"COPILOT_PROVIDER_TRANSPORT",
}

// ---------------------------------------------------------------------------
// Paths
// ---------------------------------------------------------------------------

func copilotDroidGearDir() (string, error) {
	dir, err := core.DroidGearDir()
	if err != nil {
		return "", err
	}
	return filepath.Join(dir, "copilot"), nil
}

func copilotProfilesDir() (string, error) {
	dir, err := copilotDroidGearDir()
	if err != nil {
		return "", err
	}
	profiles := filepath.Join(dir, "profiles")
	if err := core.EnsureDir(profiles); err != nil {
		return "", fmt.Errorf("Failed to create copilot profiles directory: %w", err)
	}
	return profiles, nil
}

func copilotLiveConfigPath() (string, error) {
	dir, err := copilotDroidGearDir()
	if err != nil {
		return "", err
	}
	if err := core.EnsureDir(dir); err != nil {
		return "", fmt.Errorf("Failed to create copilot directory: %w", err)
	}
	return filepath.Join(dir, "config.env"), nil
}

func copilotActiveProfilePath() (string, error) {
	dir, err := copilotDroidGearDir()
	if err != nil {
		return "", err
	}
	if err := core.EnsureDir(dir); err != nil {
		return "", fmt.Errorf("Failed to create copilot directory: %w", err)
	}
	return filepath.Join(dir, "active-profile.txt"), nil
}

func copilotProfilePath(id string) (string, error) {
	if err := validateProfileID(id); err != nil {
		return "", err
	}
	dir, err := copilotProfilesDir()
	if err != nil {
		return "", err
	}
	return filepath.Join(dir, id+".json"), nil
}

func copilotProfileEnvPath(id string) (string, error) {
	if err := validateProfileID(id); err != nil {
		return "", err
	}
	dir, err := copilotProfilesDir()
	if err != nil {
		return "", err
	}
	return filepath.Join(dir, id+".env"), nil
}

// ---------------------------------------------------------------------------
// Validation / normalisation
// ---------------------------------------------------------------------------

func copilotTrimNonEmpty(value *string) *string {
	if value == nil {
		return nil
	}
	trimmed := strings.TrimSpace(*value)
	if trimmed == "" {
		return nil
	}
	return &trimmed
}

func copilotValidateProfile(profile CopilotProfile) error {
	if strings.TrimSpace(profile.Name) == "" {
		return errString("Profile name is required")
	}
	for _, field := range []struct {
		name  string
		value *string
	}{
		{"name", &profile.Name},
		{"baseUrl", profile.BaseURL},
		{"providerType", profile.ProviderType},
		{"apiKey", profile.APIKey},
		{"model", profile.Model},
	} {
		if field.value == nil {
			continue
		}
		if strings.ContainsAny(*field.value, "\n\r\x00") {
			return fmt.Errorf("%s must be a single line without NUL characters", field.name)
		}
	}
	if profile.MaxPromptTokens != nil && *profile.MaxPromptTokens == 0 {
		return errString("Token limits must be positive integers")
	}
	if profile.MaxOutputTokens != nil && *profile.MaxOutputTokens == 0 {
		return errString("Token limits must be positive integers")
	}
	return nil
}

// copilotParseHTTPURL validates an absolute http(s) URL with a host, mirroring
// reqwest::Url::parse plus the scheme/host checks in normalize_profile.
func copilotParseHTTPURL(raw string) (*url.URL, error) {
	parsed, err := url.Parse(strings.TrimSpace(raw))
	if err != nil {
		return nil, err
	}
	if (parsed.Scheme != "http" && parsed.Scheme != "https") || parsed.Host == "" {
		return nil, errors.New("not an absolute http(s) URL")
	}
	return parsed, nil
}

// copilotNormalizeBaseURL canonicalises the endpoint for the provider
// protocol: openai gets a trailing /v1, anthropic drops it.
func copilotNormalizeBaseURL(raw string, provider string) (string, error) {
	parsed, err := copilotParseHTTPURL(raw)
	if err != nil {
		return "", errString("Base URL must be an absolute HTTP(S) URL")
	}
	path := strings.TrimRight(parsed.Path, "/")
	switch {
	case provider == "openai" && !strings.HasSuffix(path, "/v1"):
		path += "/v1"
	case provider == "anthropic":
		path = strings.TrimSuffix(path, "/v1")
	}
	if path == "" {
		// Special schemes serialise an empty path as "/": Rust ends up with
		// "https://host/" (trimmed when there is no query/fragment) or
		// "https://host/?query".
		parsed.Path = ""
		parsed.RawPath = ""
		if parsed.RawQuery == "" && parsed.Fragment == "" {
			return strings.TrimRight(parsed.String(), "/"), nil
		}
		parsed.Path = "/" // keeps "/" before ?query/#fragment
		return parsed.String(), nil
	}
	parsed.Path = path
	parsed.RawPath = ""
	return parsed.String(), nil
}

// copilotNormalizeProfile validates a profile and canonicalises its BYOK
// fields (mirrors copilot::normalize_profile).
func copilotNormalizeProfile(profile CopilotProfile) (CopilotProfile, error) {
	if err := copilotValidateProfile(profile); err != nil {
		return CopilotProfile{}, err
	}
	if profile.UseOfficialAuth {
		return profile, nil
	}
	provider := "openai"
	if trimmed := copilotTrimNonEmpty(profile.ProviderType); trimmed != nil {
		provider = strings.ToLower(*trimmed)
	}
	if provider != "openai" && provider != "anthropic" {
		return CopilotProfile{}, errString("Copilot supports OpenAI and Anthropic provider types")
	}
	if baseURL := copilotTrimNonEmpty(profile.BaseURL); baseURL != nil {
		normalized, err := copilotNormalizeBaseURL(*baseURL, provider)
		if err != nil {
			return CopilotProfile{}, err
		}
		profile.BaseURL = &normalized
	}
	profile.ProviderType = &provider
	return profile, nil
}

// ---------------------------------------------------------------------------
// Profile file IO
// ---------------------------------------------------------------------------

func copilotReadProfileFile(path string) (CopilotProfile, error) {
	data, err := os.ReadFile(path) // #nosec G304 -- profile path
	if err != nil {
		return CopilotProfile{}, fmt.Errorf("Failed to read profile: %w", err)
	}
	var profile CopilotProfile
	if err := json.Unmarshal(data, &profile); err != nil {
		return CopilotProfile{}, fmt.Errorf("Invalid profile JSON: %w", err)
	}
	return profile, nil
}

func copilotLoadProfile(id string) (CopilotProfile, error) {
	path, err := copilotProfilePath(id)
	if err != nil {
		return CopilotProfile{}, err
	}
	profile, err := copilotReadProfileFile(path)
	if err != nil {
		return CopilotProfile{}, err
	}
	if profile.ID != id {
		return CopilotProfile{}, errString("Profile id does not match its file name")
	}
	return profile, nil
}

// copilotWritePrivateFile writes an owner-only document (NamedTempFile +
// persist in Rust) via the shared atomic private writer.
func copilotWritePrivateFile(path string, contents string) error {
	return core.AtomicWritePrivate(path, core.BytesOf(contents))
}

func copilotWriteProfileFile(profile CopilotProfile) error {
	path, err := copilotProfilePath(profile.ID)
	if err != nil {
		return err
	}
	// serde_json::to_string_pretty does not escape HTML; keep base URLs with
	// query strings byte-identical to the Rust writer.
	var buf bytes.Buffer
	encoder := json.NewEncoder(&buf)
	encoder.SetEscapeHTML(false)
	encoder.SetIndent("", "  ")
	if err := encoder.Encode(profile); err != nil {
		return fmt.Errorf("Failed to serialize profile JSON: %w", err)
	}
	return copilotWritePrivateFile(path, string(bytes.TrimSuffix(buf.Bytes(), []byte("\n"))))
}

// copilotRenderEnvFile renders the profile as a human-editable env file;
// missing values are commented out so users can see every knob.
func copilotRenderEnvFile(profile CopilotProfile) string {
	if profile.UseOfficialAuth {
		return "# Official GitHub Copilot subscription mode.\nCOPILOT_OFFLINE=false\n"
	}
	line := func(key string, value *string) string {
		if trimmed := copilotTrimNonEmpty(value); trimmed != nil {
			return key + "=" + *trimmed
		}
		return "# " + key + "="
	}
	tokenLine := func(key string, value *uint64) string {
		if value != nil {
			return fmt.Sprintf("%s=%d", key, *value)
		}
		return "# " + key + "="
	}
	providerType := "openai"
	if trimmed := copilotTrimNonEmpty(profile.ProviderType); trimmed != nil {
		providerType = *trimmed
	}
	profileLine := fmt.Sprintf("#  Profile: %s (%s)", profile.Name, profile.ID)

	var builder strings.Builder
	builder.WriteString("# ============================================================\n")
	builder.WriteString("#  DroidGear-managed config for the GitHub Copilot BYOK profile\n")
	builder.WriteString(profileLine + "\n")
	builder.WriteString("# ============================================================\n\n")
	builder.WriteString("COPILOT_OFFLINE=true\n\n")
	builder.WriteString("# ---- 模型提供商 API 地址 ----\n")
	builder.WriteString(line(copilotEnvProviderBaseURL, profile.BaseURL) + "\n\n")
	builder.WriteString("# ---- 提供商类型（openai / anthropic）----\n")
	builder.WriteString(line(copilotEnvProviderType, &providerType) + "\n\n")
	builder.WriteString("# ---- API Key ----\n")
	builder.WriteString(line(copilotEnvProviderAPIKey, profile.APIKey) + "\n\n")
	builder.WriteString("# ---- 默认模型名称 ----\n")
	builder.WriteString(line(copilotEnvModel, profile.Model) + "\n\n")
	builder.WriteString("# ---- 最大 Prompt Token 数 ----\n")
	builder.WriteString(tokenLine(copilotEnvMaxPromptTokens, profile.MaxPromptTokens) + "\n\n")
	builder.WriteString("# ---- 最大输出 Token 数 ----\n")
	builder.WriteString(tokenLine(copilotEnvMaxOutputTokens, profile.MaxOutputTokens) + "\n")
	return builder.String()
}

func copilotWriteProfileAndEnv(profile CopilotProfile) error {
	if err := copilotValidateProfile(profile); err != nil {
		return err
	}
	if err := copilotWriteProfileFile(profile); err != nil {
		return err
	}
	envPath, err := copilotProfileEnvPath(profile.ID)
	if err != nil {
		return err
	}
	return copilotWritePrivateFile(envPath, copilotRenderEnvFile(profile))
}

func copilotListProfiles() ([]CopilotProfile, error) {
	dir, err := copilotProfilesDir()
	if err != nil {
		return nil, err
	}
	entries, err := os.ReadDir(dir)
	if err != nil {
		return nil, fmt.Errorf("Failed to read profiles dir: %w", err)
	}
	profiles := []CopilotProfile{}
	for _, entry := range entries {
		name := entry.Name()
		if entry.IsDir() || filepath.Ext(name) != ".json" || strings.TrimSuffix(name, ".json") == "" {
			continue
		}
		profile, err := copilotReadProfileFile(filepath.Join(dir, name))
		if err != nil {
			continue
		}
		// A profile whose id disagrees with its file name is skipped.
		if strings.TrimSuffix(name, ".json") != profile.ID {
			continue
		}
		profiles = append(profiles, profile)
	}
	sort.SliceStable(profiles, func(i, j int) bool {
		left := strings.ToLower(profiles[i].Name)
		right := strings.ToLower(profiles[j].Name)
		if left != right {
			return left < right
		}
		return profiles[i].ID < profiles[j].ID
	})
	return profiles, nil
}

// ---------------------------------------------------------------------------
// Commands — profile CRUD
// ---------------------------------------------------------------------------

// ListCopilotProfiles lists all Copilot profiles sorted by name then id.
func (a *App) ListCopilotProfiles() ([]CopilotProfile, error) {
	return copilotListProfiles()
}

// SaveCopilotProfile normalises and creates or updates a profile, rewriting
// its sibling env file. The live config.env is never touched here.
func (a *App) SaveCopilotProfile(profile CopilotProfile) error {
	profile, err := copilotNormalizeProfile(profile)
	if err != nil {
		return err
	}
	if strings.TrimSpace(profile.ID) == "" {
		profile.ID = newUUID()
		profile.CreatedAt = nowRFC3339()
	} else {
		path, err := copilotProfilePath(profile.ID)
		if err != nil {
			return err
		}
		if fileExists(path) {
			if old, err := copilotReadProfileFile(path); err == nil {
				profile.CreatedAt = old.CreatedAt
			}
		} else if strings.TrimSpace(profile.CreatedAt) == "" {
			profile.CreatedAt = nowRFC3339()
		}
	}
	profile.UpdatedAt = nowRFC3339()
	return copilotWriteProfileAndEnv(profile)
}

// DeleteCopilotProfile removes a profile, its env file and the active marker.
func (a *App) DeleteCopilotProfile(id string) error {
	path, err := copilotProfilePath(id)
	if err != nil {
		return err
	}
	if fileExists(path) {
		if err := os.Remove(path); err != nil {
			return fmt.Errorf("Failed to delete profile: %w", err)
		}
	}
	envPath, err := copilotProfileEnvPath(id)
	if err != nil {
		return err
	}
	if fileExists(envPath) {
		if err := os.Remove(envPath); err != nil {
			return fmt.Errorf("Failed to remove profile config.env: %w", err)
		}
	}
	active, err := a.GetActiveCopilotProfileId()
	if err != nil {
		return err
	}
	if active != nil && *active == id {
		activePath, err := copilotActiveProfilePath()
		if err != nil {
			return err
		}
		if fileExists(activePath) {
			if err := os.Remove(activePath); err != nil {
				return fmt.Errorf("Failed to remove active profile id: %w", err)
			}
		}
	}
	return nil
}

// DuplicateCopilotProfile copies a profile under a new name and id.
func (a *App) DuplicateCopilotProfile(id string, newName string) (CopilotProfile, error) {
	profile, err := copilotLoadProfile(id)
	if err != nil {
		return CopilotProfile{}, err
	}
	profile, err = copilotNormalizeProfile(profile)
	if err != nil {
		return CopilotProfile{}, err
	}
	profile.ID = newUUID()
	profile.Name = newName
	profile.CreatedAt = nowRFC3339()
	profile.UpdatedAt = profile.CreatedAt
	if err := copilotWriteProfileAndEnv(profile); err != nil {
		return CopilotProfile{}, err
	}
	return profile, nil
}

// CreateDefaultCopilotProfile seeds the first profile ("默认") when none
// exist. Unlike OpenCode it does not touch the active marker.
func (a *App) CreateDefaultCopilotProfile() (CopilotProfile, error) {
	profiles, err := copilotListProfiles()
	if err != nil {
		return CopilotProfile{}, err
	}
	if len(profiles) > 0 {
		return CopilotProfile{}, errString("Profiles already exist")
	}
	now := nowRFC3339()
	openai := "openai"
	profile := CopilotProfile{
		ID:           newUUID(),
		Name:         "默认",
		CreatedAt:    now,
		UpdatedAt:    now,
		ProviderType: &openai,
	}
	if err := copilotWriteProfileAndEnv(profile); err != nil {
		return CopilotProfile{}, err
	}
	return profile, nil
}

// GetActiveCopilotProfileId returns the active profile id (null when unset).
func (a *App) GetActiveCopilotProfileId() (*string, error) {
	path, err := copilotActiveProfilePath()
	if err != nil {
		return nil, err
	}
	data, err := os.ReadFile(path) // #nosec G304 -- active profile pointer
	if err != nil {
		if os.IsNotExist(err) {
			return nil, nil
		}
		return nil, fmt.Errorf("Failed to read active profile id: %w", err)
	}
	id := strings.TrimSpace(string(data))
	if id == "" {
		return nil, nil
	}
	return &id, nil
}

func copilotSetActiveProfileID(id string) error {
	path, err := copilotActiveProfilePath()
	if err != nil {
		return err
	}
	return core.AtomicWrite(path, core.BytesOf(strings.TrimSpace(id)))
}

// ApplyCopilotProfile validates the profile can run, then materialises it as
// the live config.env and records it as active.
func (a *App) ApplyCopilotProfile(id string) error {
	profile, err := copilotLoadProfile(id)
	if err != nil {
		return err
	}
	profile, err = copilotNormalizeProfile(profile)
	if err != nil {
		return err
	}
	if _, err := copilotBuildRunPlan(profile); err != nil {
		return err
	}
	configPath, err := copilotLiveConfigPath()
	if err != nil {
		return err
	}
	if err := copilotWritePrivateFile(configPath, copilotRenderEnvFile(profile)); err != nil {
		return err
	}
	return copilotSetActiveProfileID(id)
}

// GetCopilotConfigStatus reports whether the live config.env exists.
func (a *App) GetCopilotConfigStatus() (CopilotConfigStatus, error) {
	path, err := copilotLiveConfigPath()
	if err != nil {
		return CopilotConfigStatus{}, err
	}
	return CopilotConfigStatus{ConfigExists: fileExists(path), ConfigPath: path}, nil
}

// ---------------------------------------------------------------------------
// Channel import (prepare only — never writes)
// ---------------------------------------------------------------------------

// copilotSupportsChannelPlatform reports whether a channel platform exposes
// an OpenAI- or Anthropic-shaped API Copilot can talk to.
func copilotSupportsChannelPlatform(platform *string) bool {
	value := ""
	if platform != nil {
		value = strings.ToLower(*platform)
	}
	switch value {
	case "", "openai", "anthropic", "claude", "grok", "deepseek":
		return true
	default:
		return false
	}
}

// PrepareCopilotChannelImport fills an editable profile from a channel
// selection without saving or applying it (mirrors copilot::prepare_channel_import).
func (a *App) PrepareCopilotChannelImport(profile CopilotProfile, selection CopilotChannelSelection) (CopilotProfile, error) {
	if !copilotSupportsChannelPlatform(selection.Platform) {
		return CopilotProfile{}, errString("This channel platform does not expose a supported OpenAI or Anthropic API")
	}
	platform := ""
	if selection.Platform != nil {
		platform = strings.ToLower(*selection.Platform)
	}
	usesModelProtocol := selection.ChannelType == "new-api" ||
		selection.ChannelType == "cli-proxy-api" ||
		selection.ChannelType == "general"

	anthropic := false
	switch {
	case selection.Provider != nil:
		anthropic = *selection.Provider == "anthropic"
	case !usesModelProtocol && (platform == "anthropic" || platform == "claude" || platform == "grok"):
		anthropic = true
	case !usesModelProtocol && (platform == "openai" || platform == "deepseek"):
		anthropic = false
	default:
		anthropic = strings.HasPrefix(strings.ToLower(selection.Model), "claude-")
	}

	providerType := "openai"
	if anthropic {
		providerType = "anthropic"
	}
	profile.UseOfficialAuth = false
	profile.ProviderType = &providerType
	baseURL := selection.BaseURL
	profile.BaseURL = &baseURL
	apiKey := selection.APIKey
	profile.APIKey = &apiKey
	model := selection.Model
	profile.Model = &model
	profile.MaxPromptTokens = nil
	profile.MaxOutputTokens = selection.MaxOutputTokens
	return copilotNormalizeProfile(profile)
}

// ---------------------------------------------------------------------------
// Live config.env reading
// ---------------------------------------------------------------------------

func copilotEnvValue(values map[string]string, key string) *string {
	raw, ok := values[key]
	if !ok {
		return nil
	}
	trimmed := strings.TrimSpace(raw)
	if trimmed == "" {
		return nil
	}
	return &trimmed
}

// copilotParseEnvFile parses KEY=value lines; comments and blank lines are
// skipped, later assignments win.
func copilotParseEnvFile(data string) map[string]string {
	values := map[string]string{}
	for _, raw := range strings.Split(data, "\n") {
		line := strings.TrimSpace(raw)
		if line == "" || strings.HasPrefix(line, "#") {
			continue
		}
		key, value, ok := strings.Cut(line, "=")
		if !ok {
			continue
		}
		values[strings.TrimSpace(key)] = strings.TrimSpace(value)
	}
	return values
}

func copilotTokenLimit(values map[string]string, key string) (*uint64, error) {
	raw := copilotEnvValue(values, key)
	if raw == nil {
		return nil, nil
	}
	// Rust: value.parse::<u64>().ok().filter(|v| *v > 0) — integers only,
	// a leading '+' is accepted, anything else (signs, radix, junk) fails.
	number := strings.TrimPrefix(*raw, "+")
	parsed, err := strconv.ParseUint(number, 10, 64)
	if err != nil || parsed == 0 {
		return nil, fmt.Errorf("%s must be a positive integer", key)
	}
	return &parsed, nil
}

// copilotReadEnvConfig parses an env document into CopilotCurrentConfig
// (mirrors copilot::read_env_config_from_path).
func copilotReadEnvConfig(data string) (CopilotCurrentConfig, error) {
	values := copilotParseEnvFile(data)
	maxPrompt, err := copilotTokenLimit(values, copilotEnvMaxPromptTokens)
	if err != nil {
		return CopilotCurrentConfig{}, err
	}
	maxOutput, err := copilotTokenLimit(values, copilotEnvMaxOutputTokens)
	if err != nil {
		return CopilotCurrentConfig{}, err
	}
	offline := copilotEnvValue(values, copilotEnvOffline)
	baseURL := copilotEnvValue(values, copilotEnvProviderBaseURL)
	providerType := copilotEnvValue(values, copilotEnvProviderType)
	apiKey := copilotEnvValue(values, copilotEnvProviderAPIKey)
	model := copilotEnvValue(values, copilotEnvModel)

	offlineValue := ""
	if offline != nil {
		offlineValue = *offline
	}
	isByok := offlineValue != "false" &&
		(offlineValue == "true" || baseURL != nil || providerType != nil || apiKey != nil || model != nil)

	return CopilotCurrentConfig{
		IsByok:          isByok,
		BaseURL:         baseURL,
		ProviderType:    providerType,
		APIKey:          apiKey,
		Model:           model,
		MaxPromptTokens: maxPrompt,
		MaxOutputTokens: maxOutput,
	}, nil
}

// ReadCopilotCurrentConfig reads the live config.env (missing file = all
// unset, not byok).
func (a *App) ReadCopilotCurrentConfig() (CopilotCurrentConfig, error) {
	path, err := copilotLiveConfigPath()
	if err != nil {
		return CopilotCurrentConfig{}, err
	}
	data, err := os.ReadFile(path) // #nosec G304 -- live config.env
	if err != nil {
		if os.IsNotExist(err) {
			return CopilotCurrentConfig{}, nil
		}
		return CopilotCurrentConfig{}, fmt.Errorf("Failed to read config.env: %w", err)
	}
	return copilotReadEnvConfig(string(data))
}

// ---------------------------------------------------------------------------
// Run plan (copilot_runtime.rs)
// ---------------------------------------------------------------------------

// copilotRunPlan mirrors copilot_runtime::CopilotRunPlan.
type copilotRunPlan struct {
	program   string
	args      []string
	env       [][2]string
	secretEnv [][2]string
	unsetEnv  []string
}

func copilotBuildByokEnv(profile CopilotProfile) ([][2]string, [][2]string) {
	providerType := "openai"
	if trimmed := copilotTrimNonEmpty(profile.ProviderType); trimmed != nil {
		providerType = *trimmed
	}
	env := [][2]string{
		{copilotEnvOffline, "true"},
		{copilotEnvProviderType, providerType},
	}
	secretEnv := [][2]string{}
	if value := copilotTrimNonEmpty(profile.BaseURL); value != nil {
		env = append(env, [2]string{copilotEnvProviderBaseURL, *value})
	}
	if value := copilotTrimNonEmpty(profile.APIKey); value != nil {
		secretEnv = append(secretEnv, [2]string{copilotEnvProviderAPIKey, *value})
	}
	if value := copilotTrimNonEmpty(profile.Model); value != nil {
		env = append(env, [2]string{copilotEnvModel, *value})
	}
	if profile.MaxPromptTokens != nil {
		env = append(env, [2]string{copilotEnvMaxPromptTokens, fmt.Sprint(*profile.MaxPromptTokens)})
	}
	if profile.MaxOutputTokens != nil {
		env = append(env, [2]string{copilotEnvMaxOutputTokens, fmt.Sprint(*profile.MaxOutputTokens)})
	}
	return env, secretEnv
}

// copilotBuildRunPlan validates a profile and computes the terminal launch
// environment (mirrors copilot_runtime::build_run_plan).
func copilotBuildRunPlan(profile CopilotProfile) (copilotRunPlan, error) {
	profile, err := copilotNormalizeProfile(profile)
	if err != nil {
		return copilotRunPlan{}, err
	}
	unsetEnv := append([]string{}, copilotInheritedEnvKeys...)
	env := [][2]string{}
	secretEnv := [][2]string{}
	args := []string{}

	if !profile.UseOfficialAuth {
		missing := []string{}
		if copilotTrimNonEmpty(profile.BaseURL) == nil {
			missing = append(missing, "Base URL")
		}
		if copilotTrimNonEmpty(profile.Model) == nil {
			missing = append(missing, "Model")
		}
		if copilotTrimNonEmpty(profile.APIKey) == nil {
			missing = append(missing, "API key")
		}
		if len(missing) > 0 {
			return copilotRunPlan{}, fmt.Errorf(
				"Complete the Copilot BYOK profile before launching: %s", strings.Join(missing, ", "))
		}
		unsetEnv = append(unsetEnv, copilotEnvAuthToken)
		env, secretEnv = copilotBuildByokEnv(profile)
		// --model also overrides the interactive UI's persisted selection.
		args = []string{"--model", strings.TrimSpace(*profile.Model)}
	}

	return copilotRunPlan{
		program:   "copilot",
		args:      args,
		env:       env,
		secretEnv: secretEnv,
		unsetEnv:  unsetEnv,
	}, nil
}

// ---------------------------------------------------------------------------
// Terminal launcher (terminal_launch.rs — copilot spec shape: support_dir
// always nil, no_keep_open always false)
// ---------------------------------------------------------------------------

type copilotLaunchSpec struct {
	program   string
	args      []string
	env       [][2]string
	secretEnv [][2]string
	unsetEnv  []string
	cwd       *string
}

type copilotPreparedCommand struct {
	command         string
	keepOpenCommand string
}

func copilotQuotePosix(value string) string {
	return "'" + strings.ReplaceAll(value, "'", `'\''`) + "'"
}

func copilotQuotePowershell(value string) string {
	return "'" + strings.ReplaceAll(value, "'", "''") + "'"
}

func copilotEscapeCmdValue(value string) string {
	value = strings.ReplaceAll(value, "^", "^^")
	value = strings.ReplaceAll(value, `"`, `^"`)
	value = strings.ReplaceAll(value, "%", "%%")
	return value
}

func copilotQuoteCmd(value string) string {
	return "\"" + copilotEscapeCmdValue(value) + "\""
}

func copilotFileName(path string) string {
	return filepath.Base(path)
}

func copilotStartupMessage(program string) string {
	name := copilotFileName(program)
	if name == "" || name == "." {
		name = program
	}
	return "Starting " + name + "..."
}

func copilotRenderPowershellBanner(program string) string {
	return "Write-Host " + copilotQuotePowershell(copilotStartupMessage(program))
}

func copilotRenderPosixBanner(program string) string {
	return "printf '%s\\n' " + copilotQuotePosix(copilotStartupMessage(program))
}

func copilotAllEnvPairs(spec copilotLaunchSpec) [][2]string {
	pairs := make([][2]string, 0, len(spec.env)+len(spec.secretEnv))
	pairs = append(pairs, spec.env...)
	pairs = append(pairs, spec.secretEnv...)
	return pairs
}

func copilotNeedsSecureWrapper(spec copilotLaunchSpec) bool {
	return len(spec.secretEnv) > 0
}

func copilotRenderPosixEnv(spec copilotLaunchSpec) string {
	parts := []string{}
	for _, key := range spec.unsetEnv {
		parts = append(parts, "unset "+key)
	}
	for _, pair := range copilotAllEnvPairs(spec) {
		parts = append(parts, "export "+pair[0]+"="+copilotQuotePosix(pair[1]))
	}
	return strings.Join(parts, "; ")
}

func copilotRenderPowershellEnv(spec copilotLaunchSpec) string {
	parts := []string{}
	for _, key := range spec.unsetEnv {
		parts = append(parts, "Remove-Item Env:"+key+" -ErrorAction SilentlyContinue")
	}
	for _, pair := range copilotAllEnvPairs(spec) {
		parts = append(parts, "$env:"+pair[0]+" = "+copilotQuotePowershell(pair[1]))
	}
	return strings.Join(parts, "; ")
}

func copilotRenderCmdEnv(spec copilotLaunchSpec) string {
	parts := []string{}
	for _, key := range spec.unsetEnv {
		parts = append(parts, "set "+key+"=")
	}
	for _, pair := range copilotAllEnvPairs(spec) {
		parts = append(parts, "set \""+pair[0]+"="+copilotEscapeCmdValue(pair[1])+"\"")
	}
	return strings.Join(parts, " && ")
}

func copilotRenderProgramCommand(spec copilotLaunchSpec, quote func(string) string) string {
	parts := make([]string, 0, len(spec.args)+1)
	parts = append(parts, quote(spec.program))
	for _, arg := range spec.args {
		parts = append(parts, quote(arg))
	}
	return strings.Join(parts, " ")
}

func copilotJoinNonEmpty(parts []string, separator string) string {
	kept := make([]string, 0, len(parts))
	for _, part := range parts {
		if part != "" {
			kept = append(kept, part)
		}
	}
	return strings.Join(kept, separator)
}

func copilotRenderPosixCD(cwd *string) string {
	if cwd == nil {
		return ""
	}
	return "cd " + copilotQuotePosix(*cwd)
}

func copilotRenderPowershellCD(cwd *string) string {
	if cwd == nil {
		return ""
	}
	return "Set-Location " + copilotQuotePowershell(*cwd)
}

func copilotRenderCmdCD(cwd *string) string {
	if cwd == nil {
		return ""
	}
	return "cd /d " + copilotQuoteCmd(*cwd)
}

func copilotRenderPowershellPathRefresh() string {
	return "$machinePath = [Environment]::GetEnvironmentVariable('Path','Machine'); $userPath = [Environment]::GetEnvironmentVariable('Path','User'); if ($machinePath -or $userPath) { $env:Path = @($machinePath, $userPath, $env:Path | Where-Object { $_ }) -join ';' }"
}

// copilotRenderPosixChildCommand is the in-wrapper command: env(1) applies
// the overlay without exposing secrets in the outer argv.
func copilotRenderPosixChildCommand(spec copilotLaunchSpec) string {
	command := copilotRenderProgramCommand(spec, copilotQuotePosix)
	parts := []string{}
	for _, key := range spec.unsetEnv {
		parts = append(parts, "-u "+key)
	}
	for _, pair := range copilotAllEnvPairs(spec) {
		parts = append(parts, pair[0]+"="+copilotQuotePosix(pair[1]))
	}
	if len(parts) == 0 {
		return command
	}
	return "env " + strings.Join(parts, " ") + " " + command
}

func copilotPreparePosixCommand(spec copilotLaunchSpec) copilotPreparedCommand {
	envSetup := copilotRenderPosixEnv(spec)
	cd := copilotRenderPosixCD(spec.cwd)
	command := copilotRenderProgramCommand(spec, copilotQuotePosix)
	base := copilotJoinNonEmpty([]string{envSetup, cd, command}, "; ")
	return copilotPreparedCommand{command: base, keepOpenCommand: base + "; exec bash"}
}

func copilotPreparePowershellCommand(spec copilotLaunchSpec) copilotPreparedCommand {
	pathRefresh := copilotRenderPowershellPathRefresh()
	envSetup := copilotRenderPowershellEnv(spec)
	cd := copilotRenderPowershellCD(spec.cwd)
	command := "& " + copilotRenderProgramCommand(spec, copilotQuotePowershell)
	banner := copilotRenderPowershellBanner(spec.program)
	base := copilotJoinNonEmpty([]string{pathRefresh, envSetup, cd, command}, "; ")
	return copilotPreparedCommand{
		command:         base,
		keepOpenCommand: copilotJoinNonEmpty([]string{banner, base}, "; "),
	}
}

func copilotPrepareCmdCommand(spec copilotLaunchSpec) copilotPreparedCommand {
	envSetup := copilotRenderCmdEnv(spec)
	cd := copilotRenderCmdCD(spec.cwd)
	command := copilotRenderProgramCommand(spec, copilotQuoteCmd)
	base := copilotJoinNonEmpty([]string{envSetup, cd, command}, " && ")
	return copilotPreparedCommand{command: base, keepOpenCommand: base}
}

var copilotTempCounter atomic.Uint64

func copilotLaunchArtifactPath(filename string) string {
	timestamp := time.Now().UTC().Format("20060102T150405.000") + "Z"
	counter := copilotTempCounter.Add(1)
	return filepath.Join(os.TempDir(), fmt.Sprintf("%s-%d-%d-%s", timestamp, os.Getpid(), counter, filename))
}

func copilotWriteLaunchScript(path string, contents string) error {
	if parent := filepath.Dir(path); parent != "" && !core.DirExists(parent) {
		if err := os.MkdirAll(parent, 0o755); err != nil { // #nosec G301 -- temp directory
			return fmt.Errorf("Failed to create launch script directory: %w", err)
		}
	}
	if err := os.WriteFile(path, []byte(contents), 0o644); err != nil { // #nosec G306 -- temp scripts
		return fmt.Errorf("Failed to write launch script: %w", err)
	}
	if runtime.GOOS != "windows" {
		if err := os.Chmod(path, 0o700); err != nil {
			return fmt.Errorf("Failed to set launch script permissions: %w", err)
		}
	}
	return nil
}

// copilotWriteSecurePosixWrapper writes a self-deleting bash script that
// applies the env overlay internally, keeping secrets out of `ps`.
func copilotWriteSecurePosixWrapper(spec copilotLaunchSpec) (string, error) {
	path := copilotLaunchArtifactPath("terminal-launch.sh")
	lines := []string{"#!/bin/bash", `rm -f -- "$0"`}
	if cd := copilotRenderPosixCD(spec.cwd); cd != "" {
		lines = append(lines, cd)
	}
	lines = append(lines, copilotRenderPosixChildCommand(spec))
	if err := copilotWriteLaunchScript(path, strings.Join(lines, "\n")+"\n"); err != nil {
		return "", err
	}
	return path, nil
}

func copilotPrepareSecurePosixCommand(scriptPath string) copilotPreparedCommand {
	command := "bash " + copilotQuotePosix(scriptPath)
	return copilotPreparedCommand{
		command:         command,
		keepOpenCommand: command + "; exec bash",
	}
}

func copilotWriteSecureCmdWrapper(spec copilotLaunchSpec) (string, error) {
	path := copilotLaunchArtifactPath("terminal-launch.cmd")
	lines := []string{
		"@echo off",
		"setlocal",
		"cls",
		"echo " + copilotStartupMessage(spec.program),
	}
	for _, key := range spec.unsetEnv {
		lines = append(lines, "set "+key+"=")
	}
	for _, pair := range copilotAllEnvPairs(spec) {
		lines = append(lines, fmt.Sprintf("set \"%s=%s\"", pair[0], copilotEscapeCmdValue(pair[1])))
	}
	if cd := copilotRenderCmdCD(spec.cwd); cd != "" {
		lines = append(lines, cd)
	}
	lines = append(lines, copilotRenderProgramCommand(spec, copilotQuoteCmd))
	lines = append(lines, "set \"DROIDGEAR_EXIT_CODE=%ERRORLEVEL%\"")
	lines = append(lines, "start \"\" /b cmd /c del /f /q \"%~f0\" >nul 2>nul")
	lines = append(lines, "endlocal & exit /b %DROIDGEAR_EXIT_CODE%")
	if err := copilotWriteLaunchScript(path, strings.Join(lines, "\r\n")+"\r\n"); err != nil {
		return "", err
	}
	return path, nil
}

func copilotWriteSecurePowershellWrapper(spec copilotLaunchSpec) (string, error) {
	path := copilotLaunchArtifactPath("terminal-launch.ps1")
	lines := []string{
		"$ErrorActionPreference = 'Continue'",
		"try {",
		"  " + copilotRenderPowershellPathRefresh(),
		"  Clear-Host",
		"  " + copilotRenderPowershellBanner(spec.program),
	}
	for _, key := range spec.unsetEnv {
		lines = append(lines, "  Remove-Item Env:"+key+" -ErrorAction SilentlyContinue")
	}
	for _, pair := range copilotAllEnvPairs(spec) {
		lines = append(lines, "  $env:"+pair[0]+" = "+copilotQuotePowershell(pair[1]))
	}
	if cd := copilotRenderPowershellCD(spec.cwd); cd != "" {
		lines = append(lines, "  "+cd)
	}
	lines = append(lines, "  & "+copilotRenderProgramCommand(spec, copilotQuotePowershell))
	lines = append(lines, "} finally {")
	lines = append(lines, "  Remove-Item -LiteralPath $PSCommandPath -Force -ErrorAction SilentlyContinue")
	lines = append(lines, "}")
	if err := copilotWriteLaunchScript(path, strings.Join(lines, "\r\n")+"\r\n"); err != nil {
		return "", err
	}
	return path, nil
}

// copilotNeedsWindowsWrapper reports when inline multi-statement PowerShell
// or cmd is unsafe through the terminal's argv re-parse — secrets always
// force the wrapper, and so do any env/unset/cwd entries.
func copilotNeedsWindowsWrapper(spec copilotLaunchSpec) bool {
	return copilotNeedsSecureWrapper(spec) || len(spec.env) > 0 || len(spec.unsetEnv) > 0 || spec.cwd != nil
}

func copilotWindowsCmdStartArgs(payload string, cwd *string) []string {
	args := []string{"/c", "start", ""}
	if cwd != nil {
		args = append(args, "/D", *cwd)
	}
	args = append(args, "cmd", "/k", payload)
	return args
}

func copilotWindowsWtPSArgs(shell string, command string, cwd *string) []string {
	args := []string{}
	if cwd != nil {
		args = append(args, "-d", *cwd)
	}
	args = append(args, shell, "-NoExit", "-Command", command)
	return args
}

func copilotWindowsPowershellArgs(command string, cwd *string) []string {
	args := []string{}
	if cwd != nil {
		args = append(args, "-WorkingDirectory", *cwd)
	}
	args = append(args, "-NoExit", "-Command", command)
	return args
}

func copilotIsRegularFile(path string) bool {
	info, err := os.Stat(path)
	return err == nil && info.Mode().IsRegular()
}

func copilotWindowsProgramCandidates(program string) []string {
	lower := strings.ToLower(program)
	for _, suffix := range []string{".exe", ".cmd", ".bat", ".ps1", ".com"} {
		if strings.HasSuffix(lower, suffix) {
			return []string{program}
		}
	}
	return []string{program + ".exe", program + ".cmd", program + ".bat", program + ".ps1"}
}

func copilotSearchProgramInPathValue(pathValue string, candidates []string) (string, bool) {
	for _, part := range strings.Split(pathValue, ";") {
		part = strings.TrimSpace(part)
		if part == "" {
			continue
		}
		for _, name := range candidates {
			candidate := filepath.Join(part, name)
			if copilotIsRegularFile(candidate) {
				return candidate, true
			}
		}
	}
	return "", false
}

// copilotWindowsEnvironmentPath probes the registry-backed Machine/User PATH
// (GUI apps often miss user installs), hidden like the Rust helper.
func copilotWindowsEnvironmentPath(scope string) string {
	cmd := exec.Command("powershell", "-NoLogo", "-NoProfile", "-Command",
		"[Environment]::GetEnvironmentVariable('Path','"+scope+"')") // #nosec G204 -- fixed scope from a two-item list
	cmd.SysProcAttr = &syscall.SysProcAttr{HideWindow: true, CreationFlags: 0x08000000}
	output, err := cmd.Output()
	if err != nil {
		return ""
	}
	return strings.TrimSpace(string(output))
}

func copilotResolveWindowsProgram(program string) string {
	if filepath.IsAbs(program) || strings.ContainsAny(program, `\/`) {
		return program
	}
	candidates := copilotWindowsProgramCandidates(program)
	if processPath := os.Getenv("PATH"); processPath != "" {
		if found, ok := copilotSearchProgramInPathValue(processPath, candidates); ok {
			return found
		}
	}
	for _, scope := range []string{"Machine", "User"} {
		if value := copilotWindowsEnvironmentPath(scope); value != "" {
			if found, ok := copilotSearchProgramInPathValue(value, candidates); ok {
				return found
			}
		}
	}
	return program
}

func copilotFindWindowsShellExecutable(name string) (string, bool) {
	if resolved := copilotResolveWindowsProgram(name); copilotIsRegularFile(resolved) {
		return resolved, true
	}
	if strings.EqualFold(name, "pwsh") {
		var candidates []string
		if programFiles := os.Getenv("ProgramFiles"); programFiles != "" {
			candidates = append(candidates,
				filepath.Join(programFiles, "PowerShell", "7", "pwsh.exe"),
				filepath.Join(programFiles, "PowerShell", "7-preview", "pwsh.exe"))
		}
		if localAppData := os.Getenv("LOCALAPPDATA"); localAppData != "" {
			candidates = append(candidates,
				filepath.Join(localAppData, "Microsoft", "powershell", "pwsh.exe"),
				filepath.Join(localAppData, "Microsoft", "WindowsApps", "pwsh.exe"))
		}
		for _, candidate := range candidates {
			if _, err := os.Stat(candidate); err == nil { // app execution aliases are reparse points
				return candidate, true
			}
		}
	}
	return "", false
}

func copilotResolveWindowsPowershellShell() string {
	if path, ok := copilotFindWindowsShellExecutable("pwsh"); ok {
		return path
	}
	if path, ok := copilotFindWindowsShellExecutable("powershell"); ok {
		return path
	}
	return "powershell"
}

func copilotLaunchWindowsCmdPayload(payload string, cwd *string) error {
	cmd := exec.Command("cmd", copilotWindowsCmdStartArgs(payload, cwd)...) // #nosec G204 -- launch payload
	// CREATE_NO_WINDOW: hide the outer `cmd /c start` launcher hop.
	cmd.SysProcAttr = &syscall.SysProcAttr{HideWindow: true, CreationFlags: 0x08000000}
	if err := cmd.Start(); err != nil {
		return fmt.Errorf("Failed to launch cmd: %w", err)
	}
	go func() { _ = cmd.Wait() }()
	return nil
}

func copilotLaunchWindowsPowershellPayload(shell string, command string, cwd *string) error {
	cmd := exec.Command(shell, copilotWindowsPowershellArgs(command, cwd)...) // #nosec G204 -- launch payload
	// CREATE_NEW_CONSOLE: dedicated console when the parent is a GUI app.
	cmd.SysProcAttr = &syscall.SysProcAttr{CreationFlags: 0x00000010}
	if err := cmd.Start(); err != nil {
		return fmt.Errorf("Failed to launch PowerShell: %w", err)
	}
	go func() { _ = cmd.Wait() }()
	return nil
}

func copilotLaunchWindowsWtOrPSPayload(shell string, command string, cwd *string) error {
	wt := exec.Command("wt", copilotWindowsWtPSArgs(shell, command, cwd)...) // #nosec G204 -- launch payload
	if err := wt.Start(); err == nil {
		go func() { _ = wt.Wait() }()
		return nil
	}
	return copilotLaunchWindowsPowershellPayload(shell, command, cwd)
}

func copilotWindowsLaunchCommand(spec copilotLaunchSpec) (string, error) {
	if copilotNeedsWindowsWrapper(spec) {
		wrapper, err := copilotWriteSecurePowershellWrapper(spec)
		if err != nil {
			return "", err
		}
		return "& " + copilotQuotePowershell(wrapper), nil
	}
	return copilotPreparePowershellCommand(spec).keepOpenCommand, nil
}

func copilotLaunchWindows(spec copilotLaunchSpec, preferred string) error {
	resolved := spec
	resolved.program = copilotResolveWindowsProgram(resolved.program)
	cwd := resolved.cwd
	shell := copilotResolveWindowsPowershellShell()

	switch preferred {
	case "cmd":
		if copilotNeedsWindowsWrapper(resolved) {
			wrapper, err := copilotWriteSecureCmdWrapper(resolved)
			if err != nil {
				return err
			}
			return copilotLaunchWindowsCmdPayload(wrapper, cwd)
		}
		return copilotLaunchWindowsCmdPayload(copilotPrepareCmdCommand(resolved).keepOpenCommand, cwd)
	case "powershell":
		command, err := copilotWindowsLaunchCommand(resolved)
		if err != nil {
			return err
		}
		return copilotLaunchWindowsPowershellPayload(shell, command, cwd)
	default:
		// Default: Windows Terminal + PowerShell/pwsh (never bare cmd).
		command, err := copilotWindowsLaunchCommand(resolved)
		if err != nil {
			return err
		}
		return copilotLaunchWindowsWtOrPSPayload(shell, command, cwd)
	}
}

// --- macOS ------------------------------------------------------------------

func copilotAppleScriptEscape(value string) string {
	return strings.NewReplacer(`\`, `\\`, `"`, `\"`).Replace(value)
}

func copilotLaunchITerm2(spec copilotLaunchSpec, prepared copilotPreparedCommand, banner string) error {
	script := fmt.Sprintf(`tell application "iTerm2"
    activate
    delay 0.5
    if (count of windows) = 0 then
        create window with default profile
        delay 0.3
    end if
    tell current window
        create tab with default profile
        tell current session
            write text "clear; %s; %s"
        end tell
    end tell
end tell`, copilotAppleScriptEscape(banner), copilotAppleScriptEscape(prepared.keepOpenCommand))

	err := exec.Command("osascript", "-e", script).Run() // #nosec G204 -- static AppleScript
	if err == nil {
		return nil
	}
	var exitErr *exec.ExitError
	if !errors.As(err, &exitErr) {
		return fmt.Errorf("Failed to launch iTerm2: %w", err)
	}

	// osascript failed (e.g. iTerm not registered): run a self-deleting script.
	filePath := copilotLaunchArtifactPath("terminal-launch.command")
	content := "#!/bin/bash\nrm -f -- \"$0\"\nclear\n" + banner + "\n" + prepared.command + "\nexit\n"
	if err := copilotWriteLaunchScript(filePath, content); err != nil {
		return err
	}
	err = exec.Command("open", "-a", "iTerm", filePath).Run() // #nosec G204 -- launch script path
	if err != nil {
		if errors.As(err, &exitErr) {
			return errString("Failed to launch iTerm2")
		}
		return fmt.Errorf("Failed to launch iTerm2: %w", err)
	}
	return nil
}

func copilotLaunchGhostty(prepared copilotPreparedCommand, banner string) error {
	filePath := copilotLaunchArtifactPath("terminal-launch.command")
	content := "#!/bin/bash\nrm -f -- \"$0\"\nclear\n" + banner + "\n" + prepared.keepOpenCommand + "\nexit\n"
	if err := copilotWriteLaunchScript(filePath, content); err != nil {
		return err
	}
	err := exec.Command("open", "-a", "Ghostty", filePath).Run() // #nosec G204 -- launch script path
	if err == nil {
		return nil
	}
	var exitErr *exec.ExitError
	if errors.As(err, &exitErr) {
		return errString("Failed to launch Ghostty")
	}
	return fmt.Errorf("Failed to launch Ghostty: %w", err)
}

func copilotLaunchTerminalApp(prepared copilotPreparedCommand, banner string) error {
	script := fmt.Sprintf(`tell application "Terminal"
    activate
    do script "clear; %s; %s; exit"
end tell`, copilotAppleScriptEscape(banner), copilotAppleScriptEscape(prepared.command))

	err := exec.Command("osascript", "-e", script).Run() // #nosec G204 -- static AppleScript
	if err == nil {
		return nil
	}
	var exitErr *exec.ExitError
	if errors.As(err, &exitErr) {
		return errString("Failed to open Terminal")
	}
	return fmt.Errorf("Failed to open Terminal: %w", err)
}

func copilotLaunchSystemDefaultMacOS(prepared copilotPreparedCommand, banner string) error {
	filePath := copilotLaunchArtifactPath("terminal-launch.command")
	content := "#!/bin/bash\nrm -f -- \"$0\"\nclear\n" + banner + "\n" + prepared.command + "\nexit\n"
	if err := copilotWriteLaunchScript(filePath, content); err != nil {
		return err
	}
	err := exec.Command("open", filePath).Run() // #nosec G204 -- launch script path
	if err == nil {
		return nil
	}
	var exitErr *exec.ExitError
	if errors.As(err, &exitErr) {
		return errString("Failed to open terminal")
	}
	return fmt.Errorf("Failed to open terminal: %w", err)
}

func copilotMacOSPrepared(spec copilotLaunchSpec) (copilotPreparedCommand, error) {
	if copilotNeedsSecureWrapper(spec) {
		scriptPath, err := copilotWriteSecurePosixWrapper(spec)
		if err != nil {
			return copilotPreparedCommand{}, err
		}
		return copilotPrepareSecurePosixCommand(scriptPath), nil
	}
	return copilotPreparePosixCommand(spec), nil
}

func copilotLaunchMacOS(spec copilotLaunchSpec, preferred string) error {
	prepared, err := copilotMacOSPrepared(spec)
	if err != nil {
		return err
	}
	banner := copilotRenderPosixBanner(spec.program)
	switch preferred {
	case "iterm2":
		return copilotLaunchITerm2(spec, prepared, banner)
	case "ghostty":
		return copilotLaunchGhostty(prepared, banner)
	case "terminal":
		return copilotLaunchTerminalApp(prepared, banner)
	default:
		return copilotLaunchSystemDefaultMacOS(prepared, banner)
	}
}

// --- Linux ------------------------------------------------------------------

type copilotTerminalCandidate struct {
	name string
	args []string
}

func copilotLinuxTerminalCandidates(preferred string, prepared copilotPreparedCommand) []copilotTerminalCandidate {
	gnome := copilotTerminalCandidate{
		name: "gnome-terminal",
		args: []string{"--tab", "--", "bash", "-c", prepared.keepOpenCommand},
	}
	konsole := copilotTerminalCandidate{
		name: "konsole",
		args: []string{"--new-tab", "-e", "bash", "-c", prepared.keepOpenCommand},
	}
	xfce := copilotTerminalCandidate{
		name: "xfce4-terminal",
		args: []string{"--tab", "-e", "bash -c " + copilotQuotePosix(prepared.keepOpenCommand)},
	}
	xtermEmulator := copilotTerminalCandidate{
		name: "x-terminal-emulator",
		args: []string{"-e", "bash", "-c", prepared.keepOpenCommand},
	}
	xterm := copilotTerminalCandidate{
		name: "xterm",
		args: []string{"-e", "bash", "-c", prepared.keepOpenCommand},
	}

	switch preferred {
	case "gnome-terminal":
		return []copilotTerminalCandidate{gnome}
	case "konsole":
		return []copilotTerminalCandidate{konsole}
	case "xfce4-terminal":
		return []copilotTerminalCandidate{xfce}
	case "x-terminal-emulator":
		return []copilotTerminalCandidate{xtermEmulator}
	default:
		return []copilotTerminalCandidate{gnome, konsole, xfce, xtermEmulator, xterm}
	}
}

func copilotLaunchLinux(spec copilotLaunchSpec, preferred string) error {
	var prepared copilotPreparedCommand
	if copilotNeedsSecureWrapper(spec) {
		scriptPath, err := copilotWriteSecurePosixWrapper(spec)
		if err != nil {
			return err
		}
		prepared = copilotPrepareSecurePosixCommand(scriptPath)
	} else {
		prepared = copilotPreparePosixCommand(spec)
	}
	lastErr := ""
	for _, candidate := range copilotLinuxTerminalCandidates(preferred, prepared) {
		cmd := exec.Command(candidate.name, candidate.args...) // #nosec G204 -- terminal emulator from a fixed list
		err := cmd.Start()
		if err == nil {
			go func() { _ = cmd.Wait() }()
			return nil
		}
		lastErr = fmt.Sprintf("%s: %v", candidate.name, err)
	}
	return fmt.Errorf("Could not find a terminal emulator. Tried: %s", lastErr)
}

// copilotLaunchInTerminal dispatches by platform (mirrors launch_in_terminal).
func copilotLaunchInTerminal(spec copilotLaunchSpec, preferred string) error {
	switch runtime.GOOS {
	case "windows":
		return copilotLaunchWindows(spec, preferred)
	case "darwin":
		return copilotLaunchMacOS(spec, preferred)
	case "linux":
		return copilotLaunchLinux(spec, preferred)
	default:
		return errString("Unsupported platform")
	}
}

// LaunchCopilot starts the copilot CLI for a profile in the preferred
// terminal. The live config.env is never changed; secrets travel in a
// short-lived self-deleting wrapper.
func (a *App) LaunchCopilot(id string, cwd *string) error {
	profile, err := copilotLoadProfile(id)
	if err != nil {
		return err
	}
	plan, err := copilotBuildRunPlan(profile)
	if err != nil {
		return err
	}
	prefs, err := a.LoadPreferences()
	if err != nil {
		prefs = defaultPreferences()
	}
	preferred := ""
	if prefs.PreferredTerminal != nil {
		preferred = *prefs.PreferredTerminal
	}
	spec := copilotLaunchSpec{
		program:   plan.program,
		args:      plan.args,
		env:       plan.env,
		secretEnv: plan.secretEnv,
		unsetEnv:  plan.unsetEnv,
		cwd:       cwd, // Rust: cwd.map(PathBuf::from)
	}
	return copilotLaunchInTerminal(spec, preferred)
}
