package omp

import (
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"time"

	"droidgear-wails/internal/core"

	_ "modernc.org/sqlite"

	"gopkg.in/yaml.v3"
)

// ---------------------------------------------------------------------------
// Types (mirroring droidgear-core::omp)
// ---------------------------------------------------------------------------

// OmpModelRoles mirrors omp::OmpModelRoles. Upstream groups chat roles
// (default, smol, slow, vision, plan, commit, tiny, memory, task, advisor)
// and model-kind roles (image, web, speech, dictation, judge).
type OmpModelRoles struct {
	Default   *string                    `json:"default"`
	Smol      *string                    `json:"smol"`
	Slow      *string                    `json:"slow"`
	Vision    *string                    `json:"vision"`
	Plan      *string                    `json:"plan"`
	Commit    *string                    `json:"commit"`
	Tiny      *string                    `json:"tiny"`
	Memory    *string                    `json:"memory"`
	Task      *string                    `json:"task"`
	Advisor   *string                    `json:"advisor"`
	Image     *string                    `json:"image"`
	Web       *string                    `json:"web"`
	Speech    *string                    `json:"speech"`
	Dictation *string                    `json:"dictation"`
	Judge     *string                    `json:"judge"`
	Extra     map[string]json.RawMessage `json:"-"`
}

// OmpTheme mirrors omp::OmpTheme.
type OmpTheme struct {
	Dark  *string `json:"dark"`
	Light *string `json:"light"`
}

// OmpAgentConfig mirrors omp::OmpAgentConfig (config.yml).
type OmpAgentConfig struct {
	ModelRoles           *OmpModelRoles             `json:"modelRoles"`
	ModelPresets         map[string]any             `json:"modelPresets,omitempty"`
	DefaultThinkingLevel *string                    `json:"defaultThinkingLevel,omitempty"`
	Theme                *OmpTheme                  `json:"theme"`
	SteeringMode         *string                    `json:"steeringMode"`
	FollowUpMode         *string                    `json:"followUpMode"`
	InterruptMode        *string                    `json:"interruptMode"`
	SymbolPreset         *string                    `json:"symbolPreset"`
	SetupVersion         *uint32                    `json:"setupVersion"`
	Extra                map[string]json.RawMessage `json:"-"`
}

// OmpModelCost mirrors omp::OmpModelCost.
type OmpModelCost struct {
	Input      float64 `json:"input"`
	Output     float64 `json:"output"`
	CacheRead  float64 `json:"cacheRead"`
	CacheWrite float64 `json:"cacheWrite"`
}

// OmpThinkingConfig mirrors omp::OmpThinkingConfig.
type OmpThinkingConfig struct {
	Mode         *string                    `json:"mode"`
	Efforts      *[]string                  `json:"efforts"`
	DefaultLevel *string                    `json:"defaultLevel"`
	Extra        map[string]json.RawMessage `json:"-"`
}

// OmpCustomModel mirrors omp::OmpCustomModel.
type OmpCustomModel struct {
	ID                  string                     `json:"id"`
	Name                *string                    `json:"name"`
	API                 *string                    `json:"api"`
	BaseURL             *string                    `json:"baseUrl"`
	Reasoning           *bool                      `json:"reasoning"`
	Thinking            *OmpThinkingConfig         `json:"thinking"`
	Input               *[]string                  `json:"input"`
	ImageInputDecoder   *string                    `json:"imageInputDecoder"`
	Cost                *OmpModelCost              `json:"cost"`
	ContextWindow       *uint32                    `json:"contextWindow"`
	MaxContextWindow    *uint32                    `json:"maxContextWindow"`
	MaxTokens           *uint32                    `json:"maxTokens"`
	OmitMaxOutputTokens *bool                      `json:"omitMaxOutputTokens"`
	PreferWebsockets    *bool                      `json:"preferWebsockets"`
	Headers             map[string]string          `json:"headers"`
	Compat              any                        `json:"compat"`
	RemoteCompaction    any                        `json:"remoteCompaction"`
	Extra               map[string]json.RawMessage `json:"-"`
}

// OmpCustomProvider mirrors omp::OmpCustomProvider.
type OmpCustomProvider struct {
	BaseURL             *string                    `json:"baseUrl"`
	API                 *string                    `json:"api"`
	APIKey              *string                    `json:"apiKey"`
	Auth                *string                    `json:"auth"`
	Headers             map[string]string          `json:"headers"`
	AuthHeader          *bool                      `json:"authHeader"`
	Models              []OmpCustomModel           `json:"models"`
	Discovery           any                        `json:"discovery"`
	ModelOverrides      map[string]any             `json:"modelOverrides"`
	Compat              any                        `json:"compat"`
	DisableStrictTools  *bool                      `json:"disableStrictTools"`
	RemoteCompaction    any                        `json:"remoteCompaction"`
	GuardrailIdentifier *string                    `json:"guardrailIdentifier"`
	GuardrailVersion    *string                    `json:"guardrailVersion"`
	GuardrailTrace      *string                    `json:"guardrailTrace"`
	Transport           *string                    `json:"transport"`
	Extra               map[string]json.RawMessage `json:"-"`
}

// OmpCustomProviderEntry mirrors omp::OmpCustomProviderEntry.
type OmpCustomProviderEntry struct {
	ProviderID string            `json:"providerId"`
	Config     OmpCustomProvider `json:"config"`
}

// OmpCustomProviders mirrors omp::OmpCustomProviders.
type OmpCustomProviders struct {
	Providers []OmpCustomProviderEntry `json:"providers"`
}

// OmpCachedModel mirrors omp::OmpCachedModel (models.db, read-only).
type OmpCachedModel struct {
	ID               string                     `json:"id"`
	Name             *string                    `json:"name"`
	API              *string                    `json:"api"`
	Provider         *string                    `json:"provider"`
	BaseURL          *string                    `json:"baseUrl"`
	Reasoning        bool                       `json:"reasoning"`
	Input            []string                   `json:"input"`
	Cost             *OmpModelCost              `json:"cost"`
	ContextWindow    uint32                     `json:"contextWindow"`
	MaxContextWindow uint32                     `json:"maxContextWindow,omitempty"`
	MaxTokens        uint32                     `json:"maxTokens"`
	Thinking         *OmpThinkingConfig         `json:"thinking"`
	Extra            map[string]json.RawMessage `json:"-"`
}

// OmpProviderModels mirrors omp::OmpProviderModels.
type OmpProviderModels struct {
	ProviderID string           `json:"providerId"`
	Models     []OmpCachedModel `json:"models"`
}

// OmpCredentialStatus mirrors omp::OmpCredentialStatus.
type OmpCredentialStatus struct {
	Provider       string `json:"provider"`
	CredentialType string `json:"credentialType"`
	HasKey         bool   `json:"hasKey"`
}

// OmpConfigStatus mirrors omp::OmpConfigStatus.
type OmpConfigStatus struct {
	ConfigExists   bool   `json:"configExists"`
	ConfigPath     string `json:"configPath"`
	ModelsDBExists bool   `json:"modelsDbExists"`
	AgentDBExists  bool   `json:"agentDbExists"`
}

// OmpCurrentConfig mirrors omp::OmpCurrentConfig.
type OmpCurrentConfig struct {
	AgentConfig    OmpAgentConfig        `json:"agentConfig"`
	ProviderModels []OmpProviderModels   `json:"providerModels"`
	Credentials    []OmpCredentialStatus `json:"credentials"`
}

// OmpProfile mirrors omp::OmpProfile.
type OmpProfile struct {
	ID          string        `json:"id"`
	Name        string        `json:"name"`
	Description *string       `json:"description"`
	CreatedAt   string        `json:"createdAt"`
	UpdatedAt   string        `json:"updatedAt"`
	ModelRoles  OmpModelRoles `json:"modelRoles"`
	// DefaultThinkingLevel is the fallback thinking level written to
	// config.yml. An empty string clears the key; nil leaves the live value
	// untouched (older profiles predate the field).
	DefaultThinkingLevel *string `json:"defaultThinkingLevel,omitempty"`
	// ModelPresets mirrors config.yml `modelPresets` (a named snapshot of
	// every role assignment plus its thinking level).
	ModelPresets map[string]any `json:"modelPresets,omitempty"`
}

// OmpProviderTestResult mirrors omp::OmpProviderTestResult.
type OmpProviderTestResult struct {
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

func (r OmpModelRoles) MarshalJSON() ([]byte, error) {
	type alias OmpModelRoles
	return core.MarshalJSONWithExtra(alias(r), r.Extra)
}

func (r *OmpModelRoles) UnmarshalJSON(data []byte) error {
	type plain OmpModelRoles
	var p plain
	if err := core.UnmarshalJSONWithExtra(data, &p); err != nil {
		return err
	}
	*r = OmpModelRoles(p)
	return nil
}

func (c OmpAgentConfig) MarshalJSON() ([]byte, error) {
	type alias OmpAgentConfig
	return core.MarshalJSONWithExtra(alias(c), c.Extra)
}

func (c *OmpAgentConfig) UnmarshalJSON(data []byte) error {
	type plain OmpAgentConfig
	var p plain
	if err := core.UnmarshalJSONWithExtra(data, &p); err != nil {
		return err
	}
	*c = OmpAgentConfig(p)
	return nil
}

func (t OmpThinkingConfig) MarshalJSON() ([]byte, error) {
	type alias OmpThinkingConfig
	return core.MarshalJSONWithExtra(alias(t), t.Extra)
}

func (t *OmpThinkingConfig) UnmarshalJSON(data []byte) error {
	type plain OmpThinkingConfig
	var p plain
	if err := core.UnmarshalJSONWithExtra(data, &p); err != nil {
		return err
	}
	*t = OmpThinkingConfig(p)
	return nil
}

func (m OmpCustomModel) MarshalJSON() ([]byte, error) {
	type alias OmpCustomModel
	return core.MarshalJSONWithExtra(alias(m), m.Extra)
}

func (m *OmpCustomModel) UnmarshalJSON(data []byte) error {
	type plain OmpCustomModel
	var p plain
	if err := core.UnmarshalJSONWithExtra(data, &p); err != nil {
		return err
	}
	*m = OmpCustomModel(p)
	return nil
}

func (p OmpCustomProvider) MarshalJSON() ([]byte, error) {
	type alias OmpCustomProvider
	return core.MarshalJSONWithExtra(alias(p), p.Extra)
}

func (p *OmpCustomProvider) UnmarshalJSON(data []byte) error {
	type plain OmpCustomProvider
	var v plain
	if err := core.UnmarshalJSONWithExtra(data, &v); err != nil {
		return err
	}
	*p = OmpCustomProvider(v)
	return nil
}

func (m OmpCachedModel) MarshalJSON() ([]byte, error) {
	type alias OmpCachedModel
	return core.MarshalJSONWithExtra(alias(m), m.Extra)
}

func (m *OmpCachedModel) UnmarshalJSON(data []byte) error {
	type plain OmpCachedModel
	var p plain
	if err := core.UnmarshalJSONWithExtra(data, &p); err != nil {
		return err
	}
	*m = OmpCachedModel(p)
	return nil
}

// ---------------------------------------------------------------------------
// Paths
// ---------------------------------------------------------------------------

func ompDroidGearDir() (string, error) {
	dir, err := core.DroidGearDir()
	if err != nil {
		return "", err
	}
	return filepath.Join(dir, "omp"), nil
}

func ompProfilesDir() (string, error) {
	dir, err := ompDroidGearDir()
	if err != nil {
		return "", err
	}
	return filepath.Join(dir, "profiles"), nil
}

func ompActiveProfilePath() (string, error) {
	dir, err := ompDroidGearDir()
	if err != nil {
		return "", err
	}
	return filepath.Join(dir, "active-profile.txt"), nil
}

func ompConfigDir() (string, error) {
	dir := core.OmpHome()
	if err := core.EnsureDir(dir); err != nil {
		return "", fmt.Errorf("Failed to create OMP config directory: %w", err)
	}
	return dir, nil
}

func ompConfigYMLPath() (string, error) {
	dir, err := ompConfigDir()
	if err != nil {
		return "", err
	}
	return filepath.Join(dir, "config.yml"), nil
}

func ompModelsDBPath() (string, error) {
	dir, err := ompConfigDir()
	if err != nil {
		return "", err
	}
	return filepath.Join(dir, "models.db"), nil
}

func ompAgentDBPath() (string, error) {
	dir, err := ompConfigDir()
	if err != nil {
		return "", err
	}
	return filepath.Join(dir, "agent.db"), nil
}

// ompModelsYMLPath prefers models.yml, falling back to models.yaml.
func ompModelsYMLPath() (string, error) {
	dir, err := ompConfigDir()
	if err != nil {
		return "", err
	}
	yml := filepath.Join(dir, "models.yml")
	yamlPath := filepath.Join(dir, "models.yaml")
	if !core.FileExists(yml) && core.FileExists(yamlPath) {
		return yamlPath, nil
	}
	return yml, nil
}

func ompProfilePath(id string) (string, error) {
	if err := core.ValidateProfileID(id); err != nil {
		return "", err
	}
	dir, err := ompProfilesDir()
	if err != nil {
		return "", err
	}
	return filepath.Join(dir, id+".json"), nil
}

// ---------------------------------------------------------------------------
// models.yml read/write (custom providers)
// ---------------------------------------------------------------------------

type ompModelsConfig struct {
	Providers map[string]any `yaml:"providers"`
	Extra     map[string]any `yaml:",inline"`
}

func readModelsYML() (ompModelsConfig, error) {
	path, err := ompModelsYMLPath()
	if err != nil {
		return ompModelsConfig{}, err
	}
	if !core.FileExists(path) {
		jsonPath := strings.TrimSuffix(path, filepath.Ext(path)) + ".json"
		if !core.FileExists(jsonPath) {
			return ompModelsConfig{Providers: map[string]any{}}, nil
		}
		path = jsonPath
	}
	data, err := os.ReadFile(path) // #nosec G304 -- tool config path
	if err != nil {
		return ompModelsConfig{}, fmt.Errorf("Failed to read models.yml: %w", err)
	}
	if strings.TrimSpace(string(data)) == "" {
		return ompModelsConfig{Providers: map[string]any{}}, nil
	}
	if strings.HasSuffix(path, ".json") {
		var config ompModelsConfig
		if err := json.Unmarshal(data, &config); err != nil {
			return ompModelsConfig{}, fmt.Errorf("Invalid models.json: %w", err)
		}
		if config.Providers == nil {
			config.Providers = map[string]any{}
		}
		return config, nil
	}
	root, err := core.ParseYAML(string(data))
	if err != nil {
		return ompModelsConfig{}, fmt.Errorf("Invalid models.yml YAML: %w", err)
	}
	config := ompModelsConfig{Providers: map[string]any{}, Extra: map[string]any{}}
	if root == nil {
		return config, nil
	}
	if !core.IsMapping(root) {
		return config, errors.New("models.yml root must be a mapping")
	}
	if providers := core.MappingGetMap(root, "providers"); providers != nil {
		for i := 0; i+1 < len(providers.Content); i += 2 {
			key := providers.Content[i]
			if key.Kind != yaml.ScalarNode {
				continue
			}
			value, err := core.NodeToAny(providers.Content[i+1])
			if err != nil {
				return ompModelsConfig{}, fmt.Errorf("Invalid models.yml entry '%s': %w", key.Value, err)
			}
			config.Providers[key.Value] = value
		}
	}
	// Preserve non-providers top-level keys.
	for i := 0; i+1 < len(root.Content); i += 2 {
		if root.Content[i].Value == "providers" {
			continue
		}
		value, err := core.NodeToAny(root.Content[i+1])
		if err != nil {
			return ompModelsConfig{}, fmt.Errorf("Invalid models.yml section '%s': %w", root.Content[i].Value, err)
		}
		config.Extra[root.Content[i].Value] = value
	}
	return config, nil
}

// pruneNullValues drops nil entries from nested maps so the generated
// models.yml only carries declared fields. OMP's `omptype` schema rejects an
// explicit `null` for every optional provider/model key (`headers: null` fails
// with "must be an object (was null)"), while a missing key is accepted — so
// nil Go fields must be omitted instead of serialised as null.
func pruneNullValues(value any) any {
	switch typed := value.(type) {
	case map[string]any:
		out := make(map[string]any, len(typed))
		for key, item := range typed {
			if item == nil {
				continue
			}
			out[key] = pruneNullValues(item)
		}
		return out
	case []any:
		out := make([]any, len(typed))
		for i, item := range typed {
			out[i] = pruneNullValues(item)
		}
		return out
	default:
		return value
	}
}

func writeModelsYML(config ompModelsConfig) error {
	path, err := ompModelsYMLPath()
	if err != nil {
		return err
	}
	root := &yaml.Node{Kind: yaml.MappingNode}
	for key, value := range config.Extra {
		node, err := core.AnyToNode(pruneNullValues(value))
		if err != nil {
			return fmt.Errorf("Failed to serialize models.yml section '%s': %w", key, err)
		}
		core.MappingSet(root, key, node)
	}
	providers := &yaml.Node{Kind: yaml.MappingNode}
	for id, value := range config.Providers {
		node, err := core.AnyToNode(pruneNullValues(value))
		if err != nil {
			return fmt.Errorf("Failed to serialize OMP provider '%s': %w", id, err)
		}
		core.MappingSet(providers, id, node)
	}
	core.MappingSet(root, "providers", providers)

	text, err := core.MarshalYAML(root)
	if err != nil {
		return fmt.Errorf("Failed to serialize models.yml: %w", err)
	}
	return core.AtomicWrite(path, core.BytesOf(text))
}

// ReadOmpCustomProviders lists providers from models.yml sorted by id.
func ReadCustomProviders() (OmpCustomProviders, error) {
	config, err := readModelsYML()
	if err != nil {
		return OmpCustomProviders{}, err
	}
	providers := []OmpCustomProviderEntry{}
	for providerID, value := range config.Providers {
		raw, err := json.Marshal(value)
		if err != nil {
			return OmpCustomProviders{}, fmt.Errorf("Invalid OMP provider '%s': %w", providerID, err)
		}
		var provider OmpCustomProvider
		if err := json.Unmarshal(raw, &provider); err != nil {
			return OmpCustomProviders{}, fmt.Errorf("Invalid OMP provider '%s': %w", providerID, err)
		}
		providers = append(providers, OmpCustomProviderEntry{ProviderID: providerID, Config: provider})
	}
	sort.Slice(providers, func(i, j int) bool {
		return providers[i].ProviderID < providers[j].ProviderID
	})
	return OmpCustomProviders{Providers: providers}, nil
}

func ValidateCustomProvider(provider OmpCustomProvider) error {
	present := func(value *string) bool { return value != nil && strings.TrimSpace(*value) != "" }
	if provider.Auth != nil {
		switch strings.TrimSpace(*provider.Auth) {
		case "apiKey", "none", "oauth":
		default:
			return errors.New("OMP auth must be apiKey, none, or oauth")
		}
	}
	if len(provider.Models) > 0 {
		if !present(provider.BaseURL) {
			return errors.New("OMP declared models require a baseUrl")
		}
		auth := ""
		if provider.Auth != nil {
			auth = strings.TrimSpace(*provider.Auth)
		}
		if auth != "none" && auth != "oauth" && !present(provider.APIKey) {
			return errors.New("OMP declared models require an apiKey or auth: none/oauth")
		}
	}
	ids := map[string]bool{}
	for _, model := range provider.Models {
		id := strings.TrimSpace(model.ID)
		if id == "" || ids[id] {
			return errors.New("OMP model IDs must be non-empty and unique within a provider")
		}
		ids[id] = true
		if !present(model.API) && !present(provider.API) {
			return fmt.Errorf("OMP model '%s' requires an API", model.ID)
		}
		if (model.ContextWindow != nil && *model.ContextWindow == 0) ||
			(model.MaxTokens != nil && *model.MaxTokens == 0) {
			return errors.New("OMP token limits must be positive")
		}
	}
	return nil
}

// SaveOmpCustomProvider upserts one provider in models.yml, preserving
// unknown fields carried over from the previous version of the entry. The
// returned reconcile pass repairs config.yml roles the upsert made
// unresolvable (a replaced model list is declared state, so removed models
// disappear).
func SaveCustomProvider(providerID string, provider OmpCustomProvider) (OmpRoleReconcile, error) {
	providerID = strings.TrimSpace(providerID)
	if providerID == "" {
		return OmpRoleReconcile{}, errors.New("OMP provider ID cannot be empty")
	}
	if err := ValidateCustomProvider(provider); err != nil {
		return OmpRoleReconcile{}, err
	}
	config, err := readModelsYML()
	if err != nil {
		return OmpRoleReconcile{}, err
	}
	before := cloneProviderMap(config.Providers)
	if previousRaw, ok := config.Providers[providerID]; ok {
		raw, err := json.Marshal(previousRaw)
		if err == nil {
			var previous OmpCustomProvider
			if err := json.Unmarshal(raw, &previous); err == nil {
				if provider.Extra == nil {
					provider.Extra = map[string]json.RawMessage{}
				}
				for key, value := range previous.Extra {
					if _, exists := provider.Extra[key]; !exists {
						provider.Extra[key] = value
					}
				}
				for i := range provider.Models {
					for _, previousModel := range previous.Models {
						if previousModel.ID != provider.Models[i].ID {
							continue
						}
						if provider.Models[i].Extra == nil {
							provider.Models[i].Extra = map[string]json.RawMessage{}
						}
						for key, value := range previousModel.Extra {
							if _, exists := provider.Models[i].Extra[key]; !exists {
								provider.Models[i].Extra[key] = value
							}
						}
					}
				}
			}
		}
	}
	value, err := core.ToMap(provider)
	if err != nil {
		return OmpRoleReconcile{}, fmt.Errorf("Failed to serialize OMP provider: %w", err)
	}
	config.Providers[providerID] = value
	if err := writeModelsYML(config); err != nil {
		return OmpRoleReconcile{}, err
	}
	return ReconcileModelRoles(before, config.Providers)
}

// DeleteOmpCustomProvider removes one provider from models.yml and repairs the
// config.yml roles that pointed at it: OMP refuses to start with
// "No default model selected" when the default role loses its provider.
func DeleteCustomProvider(providerID string) (OmpRoleReconcile, error) {
	config, err := readModelsYML()
	if err != nil {
		return OmpRoleReconcile{}, err
	}
	before := cloneProviderMap(config.Providers)
	delete(config.Providers, providerID)
	if err := writeModelsYML(config); err != nil {
		return OmpRoleReconcile{}, err
	}
	return ReconcileModelRoles(before, config.Providers)
}

// ---------------------------------------------------------------------------
// config.yml read/write
// ---------------------------------------------------------------------------

func readConfigYML() (OmpAgentConfig, error) {
	path, err := ompConfigYMLPath()
	if err != nil {
		return OmpAgentConfig{}, err
	}
	data, err := os.ReadFile(path) // #nosec G304 -- tool config path
	if err != nil || strings.TrimSpace(string(data)) == "" {
		return OmpAgentConfig{}, nil
	}
	raw, err := yamlToJSONStrict(string(data))
	if err != nil {
		return OmpAgentConfig{}, fmt.Errorf("Invalid config.yml YAML: %w", err)
	}
	var config OmpAgentConfig
	if err := json.Unmarshal(raw, &config); err != nil {
		return OmpAgentConfig{}, fmt.Errorf("Invalid config.yml YAML: %w", err)
	}
	return config, nil
}

// yamlToJSONStrict parses YAML text and re-encodes it as JSON bytes.
func yamlToJSONStrict(text string) ([]byte, error) {
	root, err := core.ParseYAML(text)
	if err != nil {
		return nil, err
	}
	return core.NodeToJSON(root)
}

func writeConfigYML(config OmpAgentConfig) error {
	path, err := ompConfigYMLPath()
	if err != nil {
		return err
	}
	dir, err := ompConfigDir()
	if err != nil {
		return err
	}
	if err := core.EnsureDir(dir); err != nil {
		return fmt.Errorf("Failed to create OMP config directory: %w", err)
	}
	raw, err := json.Marshal(config)
	if err != nil {
		return fmt.Errorf("Failed to serialize config.yml: %w", err)
	}
	node, err := core.JSONToNode(raw)
	if err != nil {
		return fmt.Errorf("Failed to serialize config.yml: %w", err)
	}
	text, err := core.MarshalYAML(node)
	if err != nil {
		return fmt.Errorf("Failed to serialize config.yml: %w", err)
	}
	return core.AtomicWrite(path, core.BytesOf(text))
}

// ---------------------------------------------------------------------------
// SQLite readers (read-only)
// ---------------------------------------------------------------------------

func readModelCacheFromDB(path string) ([]OmpProviderModels, error) {
	if !core.FileExists(path) {
		return []OmpProviderModels{}, nil
	}
	db, err := sqlOpen(path)
	if err != nil {
		return nil, err
	}
	defer func() { _ = db.Close() }()
	rows, err := db.Query("SELECT provider_id, models FROM model_cache")
	if err != nil {
		return nil, err
	}
	defer func() { _ = rows.Close() }()
	result := []OmpProviderModels{}
	for rows.Next() {
		var providerID, modelsJSON string
		if err := rows.Scan(&providerID, &modelsJSON); err != nil {
			return nil, err
		}
		var models []OmpCachedModel
		if err := json.Unmarshal([]byte(modelsJSON), &models); err != nil {
			return nil, fmt.Errorf("Failed to parse models for '%s': %w", providerID, err)
		}
		result = append(result, OmpProviderModels{ProviderID: providerID, Models: models})
	}
	return result, rows.Err()
}

func readCredentialsFromDB(path string) ([]OmpCredentialStatus, error) {
	if !core.FileExists(path) {
		return []OmpCredentialStatus{}, nil
	}
	db, err := sqlOpen(path)
	if err != nil {
		return nil, err
	}
	defer func() { _ = db.Close() }()
	rows, err := db.Query("SELECT provider, credential_type, data FROM auth_credentials")
	if err != nil {
		return nil, err
	}
	defer func() { _ = rows.Close() }()
	result := []OmpCredentialStatus{}
	for rows.Next() {
		var provider, credentialType, data string
		if err := rows.Scan(&provider, &credentialType, &data); err != nil {
			return nil, err
		}
		result = append(result, OmpCredentialStatus{
			Provider:       provider,
			CredentialType: credentialType,
			HasKey:         strings.TrimSpace(data) != "" && data != "null",
		})
	}
	return result, rows.Err()
}

func readAPIKeyForProvider(providerID string) (string, error) {
	if custom, err := a_readOmpCustomProviders(); err == nil {
		for _, entry := range custom {
			if entry.ProviderID != providerID {
				continue
			}
			if entry.Config.APIKey != nil && strings.TrimSpace(*entry.Config.APIKey) != "" {
				return *entry.Config.APIKey, nil
			}
			if entry.Config.Auth != nil {
				auth := strings.TrimSpace(*entry.Config.Auth)
				if auth == "none" || auth == "oauth" {
					return "", nil
				}
			}
		}
	}
	path, err := ompAgentDBPath()
	if err != nil {
		return "", err
	}
	if !core.FileExists(path) {
		return "", fmt.Errorf("agent.db not found at %s", path)
	}
	db, err := sqlOpen(path)
	if err != nil {
		return "", err
	}
	defer func() { _ = db.Close() }()
	var data string
	err = db.QueryRow("SELECT data FROM auth_credentials WHERE provider = ?", providerID).Scan(&data)
	if err != nil {
		return "", fmt.Errorf("No credentials found for provider '%s' in agent.db", providerID)
	}
	data = strings.TrimSpace(data)
	if data == "" || data == "null" {
		return "", fmt.Errorf("No API key configured for provider '%s'", providerID)
	}
	var parsed map[string]any
	if json.Unmarshal([]byte(data), &parsed) == nil {
		if key, ok := parsed["api_key"].(string); ok {
			return key, nil
		}
	}
	return data, nil
}

// a_readOmpCustomProviders is the internal list used by the test helper.
func a_readOmpCustomProviders() ([]OmpCustomProviderEntry, error) {
	result, err := ReadCustomProviders()
	if err != nil {
		return nil, err
	}
	return result.Providers, nil
}

// findOmpProviderInfo locates a base URL and model id for a provider from
// models.db (preferred) or models.yml.
func findOmpProviderInfo(providerID string) (string, string, error) {
	dbPath, err := ompModelsDBPath()
	if err != nil {
		return "", "", err
	}
	if cached, err := readModelCacheFromDB(dbPath); err == nil {
		for _, pm := range cached {
			if pm.ProviderID != providerID || len(pm.Models) == 0 {
				continue
			}
			model := pm.Models[0]
			baseURL := ""
			if model.BaseURL != nil {
				baseURL = *model.BaseURL
			}
			if baseURL == "" {
				baseURL = fmt.Sprintf("https://api.%s.com", providerID)
			}
			return baseURL, model.ID, nil
		}
	}
	custom, err := a_readOmpCustomProviders()
	if err != nil {
		return "", "", err
	}
	for _, entry := range custom {
		if entry.ProviderID != providerID {
			continue
		}
		if len(entry.Config.Models) == 0 {
			return "", "", fmt.Errorf("No models found for provider '%s'", providerID)
		}
		model := entry.Config.Models[0]
		baseURL := ""
		if model.BaseURL != nil {
			baseURL = *model.BaseURL
		} else if entry.Config.BaseURL != nil {
			baseURL = *entry.Config.BaseURL
		}
		if baseURL == "" {
			return "", "", fmt.Errorf("Provider '%s' has no baseUrl", providerID)
		}
		return baseURL, model.ID, nil
	}
	return "", "", fmt.Errorf("Provider '%s' not found in models.yml or models.db", providerID)
}

// ---------------------------------------------------------------------------
// Profile CRUD + apply + status
// ---------------------------------------------------------------------------

func readOmpProfileFile(path string) (OmpProfile, error) {
	data, err := os.ReadFile(path) // #nosec G304 -- profile path
	if err != nil {
		return OmpProfile{}, fmt.Errorf("Failed to read profile: %w", err)
	}
	var profile OmpProfile
	if err := json.Unmarshal(data, &profile); err != nil {
		return OmpProfile{}, fmt.Errorf("Invalid profile JSON: %w", err)
	}
	return profile, nil
}

func WriteProfileFile(profile OmpProfile) error {
	path, err := ompProfilePath(profile.ID)
	if err != nil {
		return err
	}
	data, err := json.MarshalIndent(profile, "", "  ")
	if err != nil {
		return fmt.Errorf("Failed to serialize profile JSON: %w", err)
	}
	return core.AtomicWrite(path, data)
}

func ListProfiles() ([]OmpProfile, error) {
	dir, err := ompProfilesDir()
	if err != nil {
		return nil, err
	}
	if !core.DirExists(dir) {
		return []OmpProfile{}, nil
	}
	entries, err := os.ReadDir(dir)
	if err != nil {
		return nil, fmt.Errorf("Failed to read profiles dir: %w", err)
	}
	profiles := []OmpProfile{}
	for _, entry := range entries {
		if entry.IsDir() || !strings.HasSuffix(entry.Name(), ".json") {
			continue
		}
		if profile, err := readOmpProfileFile(filepath.Join(dir, entry.Name())); err == nil {
			profiles = append(profiles, profile)
		}
	}
	sort.Slice(profiles, func(i, j int) bool {
		return strings.ToLower(profiles[i].Name) < strings.ToLower(profiles[j].Name)
	})
	return profiles, nil
}

// GetOmpProfile returns one profile by id.
func GetProfile(id string) (OmpProfile, error) {
	path, err := ompProfilePath(id)
	if err != nil {
		return OmpProfile{}, err
	}
	return readOmpProfileFile(path)
}

// SaveOmpProfile creates or updates a profile.
func SaveProfile(profile OmpProfile) error {
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
	return WriteProfileFile(profile)
}

// DeleteOmpProfile removes a profile and clears the active pointer.
func DeleteProfile(id string) error {
	path, err := ompProfilePath(id)
	if err != nil {
		return err
	}
	if core.FileExists(path) {
		if err := core.RemoveFile(path); err != nil {
			return fmt.Errorf("Failed to delete profile: %w", err)
		}
	}
	if active, err := GetActiveProfileID(); err == nil && active != nil && *active == id {
		if activePath, err := ompActiveProfilePath(); err == nil {
			_ = core.RemoveFile(activePath)
		}
	}
	return nil
}

// DuplicateOmpProfile copies a profile under a new name and id.
func DuplicateProfile(id string, newName string) (OmpProfile, error) {
	profile, err := GetProfile(id)
	if err != nil {
		return OmpProfile{}, err
	}
	profile.ID = core.NewUUID()
	profile.Name = newName
	profile.CreatedAt = core.NowRFC3339()
	profile.UpdatedAt = profile.CreatedAt
	if err := WriteProfileFile(profile); err != nil {
		return OmpProfile{}, err
	}
	return profile, nil
}

// GetActiveProfileID returns the active profile id (null when unset).
func GetActiveProfileID() (*string, error) {
	path, err := ompActiveProfilePath()
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
	path, err := ompActiveProfilePath()
	if err != nil {
		return err
	}
	return core.AtomicWrite(path, core.BytesOf(id))
}

// OmpApplyResult reports what one apply wrote plus the advisory notes for the
// roles OMP will not be able to resolve (a profile can pin a provider that
// models.yml no longer carries; the apply does not silently rewrite that
// choice, it reports it).
type OmpApplyResult struct {
	ID       string        `json:"id"`
	Roles    OmpModelRoles `json:"roles"`
	Warnings []string      `json:"warnings,omitempty"`
}

// ApplyOmpProfile writes the profile's model roles into config.yml.
func ApplyProfile(id string) (OmpApplyResult, error) {
	profile, err := GetProfile(id)
	if err != nil {
		return OmpApplyResult{}, err
	}
	config, err := readConfigYML()
	if err != nil {
		return OmpApplyResult{}, err
	}
	roles := mergeOmpModelRoles(config.ModelRoles, profile.ModelRoles)
	config.ModelRoles = &roles
	if profile.DefaultThinkingLevel != nil {
		if strings.TrimSpace(*profile.DefaultThinkingLevel) == "" {
			config.DefaultThinkingLevel = nil
		} else {
			value := strings.TrimSpace(*profile.DefaultThinkingLevel)
			config.DefaultThinkingLevel = &value
		}
	}
	if profile.ModelPresets != nil {
		config.ModelPresets = profile.ModelPresets
	}
	if err := writeConfigYML(config); err != nil {
		return OmpApplyResult{}, err
	}
	if err := SetActiveProfileID(id); err != nil {
		return OmpApplyResult{}, err
	}
	warnings, err := ValidateModelRoles(roles, config.DefaultThinkingLevel)
	if err != nil {
		return OmpApplyResult{}, err
	}
	return OmpApplyResult{ID: id, Roles: roles, Warnings: warnings}, nil
}

// GetOmpConfigStatus reports the OMP config files' state.
func GetConfigStatus() (OmpConfigStatus, error) {
	configPath, err := ompConfigYMLPath()
	if err != nil {
		return OmpConfigStatus{}, err
	}
	modelsDB, err := ompModelsDBPath()
	if err != nil {
		return OmpConfigStatus{}, err
	}
	agentDB, err := ompAgentDBPath()
	if err != nil {
		return OmpConfigStatus{}, err
	}
	return OmpConfigStatus{
		ConfigExists:   core.FileExists(configPath),
		ConfigPath:     configPath,
		ModelsDBExists: core.FileExists(modelsDB),
		AgentDBExists:  core.FileExists(agentDB),
	}, nil
}

// ReadOmpCurrentConfig combines config.yml, models.db and agent.db.
// DB read failures degrade to empty lists (files may be locked by a running
// OMP instance), mirroring the Rust behaviour.
func ReadCurrentConfig() (OmpCurrentConfig, error) {
	agentConfig, err := readConfigYML()
	if err != nil {
		return OmpCurrentConfig{}, err
	}
	modelsDB, _ := ompModelsDBPath()
	providerModels, err := readModelCacheFromDB(modelsDB)
	if err != nil {
		providerModels = []OmpProviderModels{}
	}
	agentDB, _ := ompAgentDBPath()
	credentials, err := readCredentialsFromDB(agentDB)
	if err != nil {
		credentials = []OmpCredentialStatus{}
	}
	return OmpCurrentConfig{
		AgentConfig:    agentConfig,
		ProviderModels: providerModels,
		Credentials:    credentials,
	}, nil
}

// ---------------------------------------------------------------------------
// Connection testing (HTTP probe on /v1/models)
// ---------------------------------------------------------------------------

const ompTestTimeout = 30 * time.Second

// TestOmpProviderConnection probes the provider endpoint with a lightweight
// /v1/models request.
func TestProviderConnection(providerID string) (OmpProviderTestResult, error) {
	providerID = strings.TrimSpace(providerID)
	if providerID == "" {
		return OmpProviderTestResult{}, errors.New("OMP provider ID cannot be empty")
	}
	baseURL, modelID, err := findOmpProviderInfo(providerID)
	if err != nil {
		return OmpProviderTestResult{}, err
	}
	apiKey, err := readAPIKeyForProvider(providerID)
	if err != nil {
		return OmpProviderTestResult{}, err
	}

	startedAt := time.Now()
	diagnostics := probeModelEndpoint(providerID, baseURL, apiKey, modelID, 10*time.Second)
	latency := uint64(time.Since(startedAt).Milliseconds())
	if time.Since(startedAt) > ompTestTimeout {
		message := fmt.Sprintf("OMP connection test timed out after %d seconds", uint64(ompTestTimeout/time.Second))
		return OmpProviderTestResult{
			Success: false, ProviderID: providerID, ModelID: modelID, LatencyMS: latency, Error: &message,
		}, nil
	}
	return OmpProviderTestResult{
		Success:      diagnostics.success,
		ProviderID:   providerID,
		ModelID:      diagnostics.modelID,
		LatencyMS:    latency,
		ResponseText: diagnostics.responseText,
		Error:        diagnostics.error,
	}, nil
}
