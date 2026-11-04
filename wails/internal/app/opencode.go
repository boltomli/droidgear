// OpenCode configuration management. Mirrors droidgear-core::opencode (Rust):
// profile CRUD under ~/.droidgear/opencode/profiles plus applying profiles to
// the OpenCode config/auth files (with Preferences → Paths overrides honoured
// through the core path helpers).
package app

import (
	"bytes"
	"encoding/json"
	"fmt"
	"net/http"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"time"

	"droidgear-wails/internal/core"
)

// ---------------------------------------------------------------------------
// Types (mirroring droidgear-core::opencode)
// ---------------------------------------------------------------------------

// OpenCodeProviderOptions mirrors opencode::OpenCodeProviderOptions.
type OpenCodeProviderOptions struct {
	BaseURL *string           `json:"baseURL,omitempty"`
	APIKey  *string           `json:"apiKey,omitempty"`
	Timeout *uint32           `json:"timeout,omitempty"`
	Headers map[string]string `json:"headers,omitempty"`
}

// OpenCodeModelLimit mirrors opencode::OpenCodeModelLimit.
type OpenCodeModelLimit struct {
	Context *uint32 `json:"context,omitempty"`
	Output  *uint32 `json:"output,omitempty"`
}

// OpenCodeModelConfig mirrors opencode::OpenCodeModelConfig.
type OpenCodeModelConfig struct {
	Name  *string             `json:"name,omitempty"`
	Limit *OpenCodeModelLimit `json:"limit,omitempty"`
}

// OpenCodeProviderConfig mirrors opencode::OpenCodeProviderConfig.
type OpenCodeProviderConfig struct {
	NPM     *string                        `json:"npm,omitempty"`
	Name    *string                        `json:"name,omitempty"`
	Options *OpenCodeProviderOptions       `json:"options,omitempty"`
	Models  map[string]OpenCodeModelConfig `json:"models,omitempty"`
}

// OpenCodeProfile mirrors opencode::OpenCodeProfile.
type OpenCodeProfile struct {
	ID          string                            `json:"id"`
	Name        string                            `json:"name"`
	Description *string                           `json:"description,omitempty"`
	CreatedAt   string                            `json:"createdAt"`
	UpdatedAt   string                            `json:"updatedAt"`
	Providers   map[string]OpenCodeProviderConfig `json:"providers"`
	Auth        map[string]any                    `json:"auth"`
}

// OpenCodeConfigStatus mirrors opencode::OpenCodeConfigStatus.
type OpenCodeConfigStatus struct {
	ConfigExists bool   `json:"configExists"`
	AuthExists   bool   `json:"authExists"`
	ConfigPath   string `json:"configPath"`
	AuthPath     string `json:"authPath"`
}

// ProviderTemplate mirrors opencode::ProviderTemplate.
type ProviderTemplate struct {
	ID             string  `json:"id"`
	Name           string  `json:"name"`
	NPM            *string `json:"npm,omitempty"`
	DefaultBaseURL *string `json:"defaultBaseUrl,omitempty"`
	RequiresAPIKey bool    `json:"requiresApiKey"`
}

// OpenCodeCurrentConfig mirrors opencode::OpenCodeCurrentConfig.
type OpenCodeCurrentConfig struct {
	Providers map[string]OpenCodeProviderConfig `json:"providers"`
	Auth      map[string]any                    `json:"auth"`
}

// ---------------------------------------------------------------------------
// Paths
// ---------------------------------------------------------------------------

func opencodeDroidGearDir() (string, error) {
	dir, err := core.DroidGearDir()
	if err != nil {
		return "", err
	}
	return filepath.Join(dir, "opencode"), nil
}

// opencodeProfilesDir resolves ~/.droidgear/opencode/profiles, creating it.
func opencodeProfilesDir() (string, error) {
	base, err := opencodeDroidGearDir()
	if err != nil {
		return "", err
	}
	dir := filepath.Join(base, "profiles")
	if err := core.EnsureDir(dir); err != nil {
		return "", fmt.Errorf("Failed to create profiles directory: %w", err)
	}
	return dir, nil
}

// opencodeActiveProfilePath resolves active-profile.txt, creating the parent.
func opencodeActiveProfilePath() (string, error) {
	base, err := opencodeDroidGearDir()
	if err != nil {
		return "", err
	}
	if err := core.EnsureDir(base); err != nil {
		return "", fmt.Errorf("Failed to create opencode directory: %w", err)
	}
	return filepath.Join(base, "active-profile.txt"), nil
}

// opencodeConfigDir resolves the OpenCode config dir (configPaths override or
// ~/.config/opencode), creating it — same side effect as the Rust helper.
func opencodeConfigDir() (string, error) {
	dir, _, err := core.EffectivePathForKey("opencode")
	if err != nil {
		return "", err
	}
	if err := core.EnsureDir(dir); err != nil {
		return "", fmt.Errorf("Failed to create opencode config directory: %w", err)
	}
	return dir, nil
}

// opencodeAuthDir resolves the OpenCode auth dir (configPaths override or
// ~/.local/share/opencode), creating it.
func opencodeAuthDir() (string, error) {
	dir, _, err := core.EffectivePathForKey("opencodeAuth")
	if err != nil {
		return "", err
	}
	if err := core.EnsureDir(dir); err != nil {
		return "", fmt.Errorf("Failed to create opencode auth directory: %w", err)
	}
	return dir, nil
}

// opencodeResolveConfigFile prefers <base>.jsonc over <base>.json.
func opencodeResolveConfigFile(dir string, base string) string {
	jsonc := filepath.Join(dir, base+".jsonc")
	if fileExists(jsonc) {
		return jsonc
	}
	return filepath.Join(dir, base+".json")
}

func opencodeConfigPath() (string, error) {
	dir, err := opencodeConfigDir()
	if err != nil {
		return "", err
	}
	return opencodeResolveConfigFile(dir, "opencode"), nil
}

func opencodeAuthPath() (string, error) {
	dir, err := opencodeAuthDir()
	if err != nil {
		return "", err
	}
	return opencodeResolveConfigFile(dir, "auth"), nil
}

// ---------------------------------------------------------------------------
// JSON helpers (comment-stripped reads, serde-compatible pretty output)
// ---------------------------------------------------------------------------

// opencodeStripJSONComments removes // and /* *\/ comments outside string
// literals — the Go equivalent of json_comments::StripComments. Comment runs
// are replaced by whitespace so token separation survives.
func opencodeStripJSONComments(src []byte) []byte {
	out := make([]byte, 0, len(src))
	inString := false
	escaped := false
	for i := 0; i < len(src); {
		c := src[i]
		switch {
		case inString:
			out = append(out, c)
			switch {
			case escaped:
				escaped = false
			case c == '\\':
				escaped = true
			case c == '"':
				inString = false
			}
			i++
		case c == '"':
			inString = true
			out = append(out, c)
			i++
		case c == '/' && i+1 < len(src) && src[i+1] == '/':
			j := i + 2
			for j < len(src) && src[j] != '\n' {
				j++
			}
			out = append(out, '\n')
			i = j + 1
		case c == '/' && i+1 < len(src) && src[i+1] == '*':
			j := i + 2
			for j+1 < len(src) && !(src[j] == '*' && src[j+1] == '/') {
				j++
			}
			out = append(out, ' ')
			if j+1 < len(src) {
				i = j + 2
			} else {
				i = len(src)
			}
		default:
			out = append(out, c)
			i++
		}
	}
	return out
}

// opencodeReadJSONValue reads a JSON/JSONC document. Missing files, read
// failures and parse failures all yield an empty object — mirroring Rust
// read_json_file's `unwrap_or(json!({}))`. Numbers keep their literal form
// (json.Number) so a rewrite never reorders or reformats them.
func opencodeReadJSONValue(path string) any {
	data, err := os.ReadFile(path) // #nosec G304 -- tool config path
	if err != nil {
		return map[string]any{}
	}
	decoder := json.NewDecoder(bytes.NewReader(opencodeStripJSONComments(data)))
	decoder.UseNumber()
	var value any
	if err := decoder.Decode(&value); err != nil {
		return map[string]any{}
	}
	// A literal `null` root stays null (Rust keeps Value::Null and skips the
	// object merge); only unreadable/unparsable documents become {}.
	return value
}

// opencodeMarshalJSON renders serde_json::to_string_pretty output: two-space
// indent, no trailing newline, HTML characters not escaped.
func opencodeMarshalJSON(value any) ([]byte, error) {
	var buf bytes.Buffer
	encoder := json.NewEncoder(&buf)
	encoder.SetEscapeHTML(false)
	encoder.SetIndent("", "  ")
	if err := encoder.Encode(value); err != nil {
		return nil, err
	}
	return bytes.TrimSuffix(buf.Bytes(), []byte("\n")), nil
}

// ---------------------------------------------------------------------------
// Profile files
// ---------------------------------------------------------------------------

func opencodeProfilePath(id string) (string, error) {
	if err := core.ValidateProfileID(id); err != nil {
		return "", err
	}
	dir, err := opencodeProfilesDir()
	if err != nil {
		return "", err
	}
	return filepath.Join(dir, id+".json"), nil
}

func opencodeReadProfileFile(path string) (OpenCodeProfile, error) {
	data, err := os.ReadFile(path) // #nosec G304 -- profile path
	if err != nil {
		return OpenCodeProfile{}, fmt.Errorf("Failed to read profile: %w", err)
	}
	var profile OpenCodeProfile
	if err := json.Unmarshal(data, &profile); err != nil {
		return OpenCodeProfile{}, fmt.Errorf("Invalid profile JSON: %w", err)
	}
	if profile.Providers == nil {
		profile.Providers = map[string]OpenCodeProviderConfig{}
	}
	if profile.Auth == nil {
		profile.Auth = map[string]any{}
	}
	return profile, nil
}

func opencodeWriteProfileFile(profile OpenCodeProfile) error {
	path, err := opencodeProfilePath(profile.ID)
	if err != nil {
		return err
	}
	data, err := opencodeMarshalJSON(profile)
	if err != nil {
		return fmt.Errorf("Failed to serialize profile JSON: %w", err)
	}
	return core.AtomicWrite(path, data)
}

func opencodeLoadProfile(id string) (OpenCodeProfile, error) {
	path, err := opencodeProfilePath(id)
	if err != nil {
		return OpenCodeProfile{}, err
	}
	return opencodeReadProfileFile(path)
}

func opencodeListProfiles() ([]OpenCodeProfile, error) {
	dir, err := opencodeProfilesDir()
	if err != nil {
		return nil, err
	}
	entries, err := os.ReadDir(dir)
	if err != nil {
		return nil, fmt.Errorf("Failed to read profiles dir: %w", err)
	}
	profiles := []OpenCodeProfile{}
	for _, entry := range entries {
		name := entry.Name()
		// Rust compares Path::extension() == "json"; a bare ".json" has no
		// extension there, so exclude the stem-less case explicitly.
		if filepath.Ext(name) != ".json" || strings.TrimSuffix(name, ".json") == "" {
			continue
		}
		if entry.IsDir() {
			continue
		}
		if profile, err := opencodeReadProfileFile(filepath.Join(dir, name)); err == nil {
			profiles = append(profiles, profile)
		}
	}
	sort.SliceStable(profiles, func(i, j int) bool {
		return strings.ToLower(profiles[i].Name) < strings.ToLower(profiles[j].Name)
	})
	return profiles, nil
}

// ---------------------------------------------------------------------------
// Commands — profile CRUD
// ---------------------------------------------------------------------------

// ListOpencodeProfiles lists all OpenCode profiles sorted by name.
func (a *App) ListOpencodeProfiles() ([]OpenCodeProfile, error) {
	return opencodeListProfiles()
}

// GetOpencodeProfile returns one profile by id.
func (a *App) GetOpencodeProfile(id string) (OpenCodeProfile, error) {
	return opencodeLoadProfile(id)
}

// SaveOpencodeProfile creates or updates a profile (timestamps per the Rust
// core: new ids get a fresh created_at; updates keep the stored created_at).
func (a *App) SaveOpencodeProfile(profile OpenCodeProfile) error {
	if strings.TrimSpace(profile.ID) == "" {
		profile.ID = core.NewUUID()
		profile.CreatedAt = core.NowRFC3339()
	} else {
		path, err := opencodeProfilePath(profile.ID)
		if err != nil {
			return err
		}
		if fileExists(path) {
			if old, err := opencodeReadProfileFile(path); err == nil {
				profile.CreatedAt = old.CreatedAt
			}
		} else if strings.TrimSpace(profile.CreatedAt) == "" {
			profile.CreatedAt = core.NowRFC3339()
		}
	}
	profile.UpdatedAt = core.NowRFC3339()
	if profile.Providers == nil {
		profile.Providers = map[string]OpenCodeProviderConfig{}
	}
	if profile.Auth == nil {
		profile.Auth = map[string]any{}
	}
	return opencodeWriteProfileFile(profile)
}

// DeleteOpencodeProfile removes a profile and clears the active pointer.
func (a *App) DeleteOpencodeProfile(id string) error {
	path, err := opencodeProfilePath(id)
	if err != nil {
		return err
	}
	if fileExists(path) {
		if err := os.Remove(path); err != nil {
			return fmt.Errorf("Failed to delete profile: %w", err)
		}
	}
	if active, err := a.GetActiveOpencodeProfileId(); err == nil && active != nil && *active == id {
		if activePath, err := opencodeActiveProfilePath(); err == nil {
			_ = os.Remove(activePath)
		}
	}
	return nil
}

// DuplicateOpencodeProfile copies a profile under a new name and id.
func (a *App) DuplicateOpencodeProfile(id string, newName string) (OpenCodeProfile, error) {
	profile, err := opencodeLoadProfile(id)
	if err != nil {
		return OpenCodeProfile{}, err
	}
	profile.ID = core.NewUUID()
	profile.Name = newName
	profile.CreatedAt = core.NowRFC3339()
	profile.UpdatedAt = profile.CreatedAt
	if err := opencodeWriteProfileFile(profile); err != nil {
		return OpenCodeProfile{}, err
	}
	return profile, nil
}

// CreateDefaultProfile seeds the first profile ("Default") when none exist
// and marks it active.
func (a *App) CreateDefaultProfile() (OpenCodeProfile, error) {
	profiles, err := opencodeListProfiles()
	if err != nil {
		return OpenCodeProfile{}, err
	}
	if len(profiles) > 0 {
		return OpenCodeProfile{}, errString("Profiles already exist")
	}
	now := core.NowRFC3339()
	profile := OpenCodeProfile{
		ID:        core.NewUUID(),
		Name:      "Default",
		CreatedAt: now,
		UpdatedAt: now,
		Providers: map[string]OpenCodeProviderConfig{},
		Auth:      map[string]any{},
	}
	if err := a.SaveOpencodeProfile(profile); err != nil {
		return OpenCodeProfile{}, err
	}
	activePath, err := opencodeActiveProfilePath()
	if err != nil {
		return OpenCodeProfile{}, err
	}
	if err := core.AtomicWrite(activePath, core.BytesOf(profile.ID)); err != nil {
		return OpenCodeProfile{}, err
	}
	return profile, nil
}

// GetActiveOpencodeProfileId returns the active profile id (null when unset).
func (a *App) GetActiveOpencodeProfileId() (*string, error) {
	path, err := opencodeActiveProfilePath()
	if err != nil {
		return nil, err
	}
	data, err := os.ReadFile(path) // #nosec G304 -- active profile pointer
	if err != nil {
		if os.IsNotExist(err) {
			return nil, nil
		}
		return nil, fmt.Errorf("Failed to read active profile: %w", err)
	}
	id := strings.TrimSpace(string(data))
	if id == "" {
		return nil, nil
	}
	return &id, nil
}

// ---------------------------------------------------------------------------
// Apply / status / read current / templates / connection test
// ---------------------------------------------------------------------------

// ApplyOpencodeProfile merges the profile's providers and auth into the live
// OpenCode config/auth files and records the active profile id. Unknown
// config fields survive because the documents are edited as generic JSON.
func (a *App) ApplyOpencodeProfile(id string) error {
	profile, err := a.GetOpencodeProfile(id)
	if err != nil {
		return err
	}

	configPath, err := opencodeConfigPath()
	if err != nil {
		return err
	}
	config := opencodeReadJSONValue(configPath)
	if len(profile.Providers) > 0 {
		providersRaw, err := json.Marshal(profile.Providers)
		if err != nil {
			return fmt.Errorf("Failed to serialize providers: %w", err)
		}
		providersValue := opencodeDecodeObject(providersRaw)
		if cfg, ok := config.(map[string]any); ok {
			existing, present := cfg["provider"]
			if !present {
				existing = map[string]any{}
				cfg["provider"] = existing
			}
			// Only merge when the existing `provider` node is an object —
			// a non-object value is left untouched, as in Rust.
			if existingObj, ok := existing.(map[string]any); ok {
				for key, value := range providersValue {
					existingObj[key] = value
				}
			}
		}
	}
	configData, err := opencodeMarshalJSON(config)
	if err != nil {
		return fmt.Errorf("Failed to serialize config: %w", err)
	}
	if err := core.AtomicWrite(configPath, configData); err != nil {
		return err
	}

	authPath, err := opencodeAuthPath()
	if err != nil {
		return err
	}
	auth := opencodeReadJSONValue(authPath)
	if len(profile.Auth) > 0 {
		if authObj, ok := auth.(map[string]any); ok {
			for key, value := range profile.Auth {
				authObj[key] = value
			}
		}
	}
	authData, err := opencodeMarshalJSON(auth)
	if err != nil {
		return fmt.Errorf("Failed to serialize config: %w", err)
	}
	if err := core.AtomicWrite(authPath, authData); err != nil {
		return err
	}

	activePath, err := opencodeActiveProfilePath()
	if err != nil {
		return err
	}
	return core.AtomicWrite(activePath, core.BytesOf(id))
}

// opencodeDecodeObject decodes raw JSON into an object; anything that is not
// an object yields an empty map (Rust's as_object() checks).
func opencodeDecodeObject(raw []byte) map[string]any {
	decoder := json.NewDecoder(bytes.NewReader(raw))
	decoder.UseNumber()
	var value any
	if err := decoder.Decode(&value); err != nil {
		return map[string]any{}
	}
	obj, ok := value.(map[string]any)
	if !ok {
		return map[string]any{}
	}
	return obj
}

// GetOpencodeConfigStatus reports whether the live config/auth files exist.
func (a *App) GetOpencodeConfigStatus() (OpenCodeConfigStatus, error) {
	configPath, err := opencodeConfigPath()
	if err != nil {
		return OpenCodeConfigStatus{}, err
	}
	authPath, err := opencodeAuthPath()
	if err != nil {
		return OpenCodeConfigStatus{}, err
	}
	return OpenCodeConfigStatus{
		ConfigExists: fileExists(configPath),
		AuthExists:   fileExists(authPath),
		ConfigPath:   configPath,
		AuthPath:     authPath,
	}, nil
}

// GetOpencodeProviderTemplates returns the quick-setup provider templates.
func (a *App) GetOpencodeProviderTemplates() ([]ProviderTemplate, error) {
	return []ProviderTemplate{
		{
			ID: "anthropic", Name: "Anthropic",
			NPM: core.StrPtr("@ai-sdk/anthropic"), DefaultBaseURL: core.StrPtr("https://api.anthropic.com/v1"),
			RequiresAPIKey: true,
		},
		{
			ID: "openai", Name: "OpenAI",
			NPM: core.StrPtr("@ai-sdk/openai"), DefaultBaseURL: core.StrPtr("https://api.openai.com/v1"),
			RequiresAPIKey: true,
		},
		{
			ID: "gemini", Name: "Gemini",
			NPM: core.StrPtr("@ai-sdk/google"), DefaultBaseURL: core.StrPtr("https://generativelanguage.googleapis.com/v1beta"),
			RequiresAPIKey: true,
		},
	}, nil
}

// opencodeHTTPClient bounds the provider probe (Rust's reqwest client has no
// explicit timeout; a GUI button must not hang forever).
var opencodeHTTPClient = &http.Client{Timeout: 30 * time.Second}

// TestOpencodeProviderConnection pings <baseUrl>/v1/models with the right
// auth headers; true means a 2xx response.
func (a *App) TestOpencodeProviderConnection(providerID string, baseUrl string, apiKey string) (bool, error) {
	url := strings.TrimRight(strings.TrimSpace(baseUrl), "/") + "/v1/models"
	request, err := http.NewRequest(http.MethodGet, url, nil) // #nosec G107 -- provider URL from user config
	if err != nil {
		return false, fmt.Errorf("Connection failed: %w", err)
	}
	if providerID == "anthropic" {
		request.Header.Set("x-api-key", apiKey)
		request.Header.Set("anthropic-version", "2023-06-01")
	} else {
		request.Header.Set("Authorization", "Bearer "+apiKey)
	}
	response, err := opencodeHTTPClient.Do(request)
	if err != nil {
		return false, fmt.Errorf("Connection failed: %w", err)
	}
	defer func() { _ = response.Body.Close() }()
	return response.StatusCode >= 200 && response.StatusCode < 300, nil
}

// opencodeNormalizeProviderOptions copies `options.baseUrl` to
// `options.baseURL` when only the lowercase spelling is present.
func opencodeNormalizeProviderOptions(value any) any {
	providers, ok := value.(map[string]any)
	if !ok {
		return value
	}
	for _, entry := range providers {
		provider, ok := entry.(map[string]any)
		if !ok {
			continue
		}
		options, ok := provider["options"].(map[string]any)
		if !ok {
			continue
		}
		if _, hasNew := options["baseURL"]; hasNew {
			continue
		}
		if baseURL, hasOld := options["baseUrl"]; hasOld {
			options["baseURL"] = baseURL
		}
	}
	return value
}

// ReadOpencodeCurrentConfig reads providers/auth from the live OpenCode
// files; provider `options.apiKey` values fill in missing auth entries.
func (a *App) ReadOpencodeCurrentConfig() (OpenCodeCurrentConfig, error) {
	configPath, err := opencodeConfigPath()
	if err != nil {
		return OpenCodeCurrentConfig{}, err
	}
	config := opencodeReadJSONValue(configPath)
	providerValue := any(map[string]any{})
	if cfg, ok := config.(map[string]any); ok {
		if raw, present := cfg["provider"]; present && raw != nil {
			providerValue = raw
		}
	}
	normalized := opencodeNormalizeProviderOptions(providerValue)

	providers := map[string]OpenCodeProviderConfig{}
	if raw, err := opencodeMarshalJSON(normalized); err == nil {
		if err := json.Unmarshal(raw, &providers); err != nil || providers == nil {
			providers = map[string]OpenCodeProviderConfig{}
		}
	}

	authPath, err := opencodeAuthPath()
	if err != nil {
		return OpenCodeCurrentConfig{}, err
	}
	auth := map[string]any{}
	if obj, ok := opencodeReadJSONValue(authPath).(map[string]any); ok {
		for key, value := range obj {
			auth[key] = value
		}
	}
	if providerObj, ok := normalized.(map[string]any); ok {
		for providerID, entry := range providerObj {
			if _, present := auth[providerID]; present {
				continue
			}
			provider, ok := entry.(map[string]any)
			if !ok {
				continue
			}
			options, ok := provider["options"].(map[string]any)
			if !ok {
				continue
			}
			apiKey, ok := options["apiKey"].(string)
			if !ok || apiKey == "" {
				continue
			}
			auth[providerID] = map[string]any{"type": "api", "key": apiKey}
		}
	}

	return OpenCodeCurrentConfig{Providers: providers, Auth: auth}, nil
}
