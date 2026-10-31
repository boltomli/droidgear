package app

import (
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"sort"
	"strings"

	"droidgear-wails/internal/core"

	"gopkg.in/yaml.v3"
)

// ---------------------------------------------------------------------------
// Types (mirroring droidgear-core::hermes)
// ---------------------------------------------------------------------------

// HermesModelConfig mirrors hermes::HermesModelConfig.
type HermesModelConfig struct {
	Name      *string `json:"name"`
	Default   *string `json:"default"`
	Provider  *string `json:"provider"`
	BaseURL   *string `json:"baseUrl"`
	APIKey    *string `json:"apiKey"`
	IsDefault bool    `json:"isDefault"`
}

// HermesProfile mirrors hermes::HermesProfile.
type HermesProfile struct {
	ID              string              `json:"id"`
	Name            string              `json:"name"`
	Description     *string             `json:"description"`
	CreatedAt       string              `json:"createdAt"`
	UpdatedAt       string              `json:"updatedAt"`
	Models          []HermesModelConfig `json:"models"`
	ReasoningEffort *string             `json:"reasoningEffort"`
}

// Legacy profile files stored a single `model` object; the Rust deserializer
// migrated it to a one-element `models` list. legacyHermesProfile captures
// the old shape for reading.
type legacyHermesProfile struct {
	ID              string              `json:"id"`
	Name            string              `json:"name"`
	Description     *string             `json:"description"`
	CreatedAt       string              `json:"createdAt"`
	UpdatedAt       string              `json:"updatedAt"`
	Model           *HermesModelConfig  `json:"model"`
	Models          []HermesModelConfig `json:"models"`
	ReasoningEffort *string             `json:"reasoningEffort"`
}

// HermesConfigStatus mirrors hermes::HermesConfigStatus.
type HermesConfigStatus struct {
	ConfigExists bool   `json:"configExists"`
	ConfigPath   string `json:"configPath"`
}

// HermesCurrentConfig mirrors hermes::HermesCurrentConfig.
type HermesCurrentConfig struct {
	Models          []HermesModelConfig `json:"models"`
	ReasoningEffort *string             `json:"reasoningEffort"`
}

// ---------------------------------------------------------------------------
// Paths
// ---------------------------------------------------------------------------

func hermesDroidGearDir() (string, error) {
	dir, err := core.DroidGearDir()
	if err != nil {
		return "", err
	}
	return filepath.Join(dir, "hermes"), nil
}

func hermesProfilesDir() (string, error) {
	dir, err := hermesDroidGearDir()
	if err != nil {
		return "", err
	}
	return filepath.Join(dir, "profiles"), nil
}

func hermesActiveProfilePath() (string, error) {
	dir, err := hermesDroidGearDir()
	if err != nil {
		return "", err
	}
	return filepath.Join(dir, "active-profile.txt"), nil
}

// hermesConfigPath resolves config.yaml WITHOUT side effects. Read-only
// commands (GetHermesConfigStatus / ReadHermesCurrentConfig) must never
// create the directory: on the Windows priority chain a freshly created
// dir at a fallback location (or under \\wsl$\) silently changes what the
// next resolution picks, and nothing should touch disk before Apply.
// ApplyHermesProfile creates the parent directory via core.AtomicWrite.
func hermesConfigPath() (string, error) {
	return filepath.Join(core.HermesHome(), "config.yaml"), nil
}

func hermesProfilePath(id string) (string, error) {
	if err := validateProfileID(id); err != nil {
		return "", err
	}
	dir, err := hermesProfilesDir()
	if err != nil {
		return "", err
	}
	return filepath.Join(dir, id+".json"), nil
}

// ---------------------------------------------------------------------------
// Helpers
// ---------------------------------------------------------------------------

func nonEmpty(value *string) string {
	if value == nil {
		return ""
	}
	return strings.TrimSpace(*value)
}

// normalizeHermesModels guarantees at most one is_default entry; when none is
// marked, the first entry becomes the default.
func normalizeHermesModels(models []HermesModelConfig) []HermesModelConfig {
	if len(models) == 0 {
		return models
	}
	seenDefault := false
	for i := range models {
		if seenDefault {
			models[i].IsDefault = false
		} else if models[i].IsDefault {
			seenDefault = true
		}
	}
	if !seenDefault {
		models[0].IsDefault = true
	}
	return models
}

// resolveHermesProvider computes the value written to `model.provider`:
// explicit provider first; `custom` with a name upgrades to `custom:<name>`;
// a name alone yields `custom:<name>`; a bare base_url yields `custom`.
func resolveHermesProvider(entry HermesModelConfig) *string {
	provider := nonEmpty(entry.Provider)
	name := nonEmpty(entry.Name)
	switch provider {
	case "custom":
		if name != "" {
			value := "custom:" + name
			return &value
		}
		value := "custom"
		return &value
	case "":
		if name != "" {
			value := "custom:" + name
			return &value
		}
		if nonEmpty(entry.BaseURL) != "" {
			value := "custom"
			return &value
		}
		return nil
	default:
		return &provider
	}
}

func readHermesProfileFile(path string) (HermesProfile, error) {
	data, err := os.ReadFile(path) // #nosec G304 -- profile path
	if err != nil {
		return HermesProfile{}, fmt.Errorf("Failed to read profile: %w", err)
	}
	var legacy legacyHermesProfile
	if err := json.Unmarshal(data, &legacy); err != nil {
		return HermesProfile{}, fmt.Errorf("Invalid profile JSON: %w", err)
	}
	models := legacy.Models
	if len(models) == 0 && legacy.Model != nil {
		migrated := *legacy.Model
		migrated.IsDefault = true
		models = []HermesModelConfig{migrated}
	}
	return HermesProfile{
		ID:              legacy.ID,
		Name:            legacy.Name,
		Description:     legacy.Description,
		CreatedAt:       legacy.CreatedAt,
		UpdatedAt:       legacy.UpdatedAt,
		Models:          normalizeHermesModels(models),
		ReasoningEffort: legacy.ReasoningEffort,
	}, nil
}

func writeHermesProfileFile(profile HermesProfile) error {
	path, err := hermesProfilePath(profile.ID)
	if err != nil {
		return err
	}
	data, err := json.MarshalIndent(profile, "", "  ")
	if err != nil {
		return fmt.Errorf("Failed to serialize profile JSON: %w", err)
	}
	return core.AtomicWrite(path, data)
}

func listHermesProfiles() ([]HermesProfile, error) {
	dir, err := hermesProfilesDir()
	if err != nil {
		return nil, err
	}
	if !core.DirExists(dir) {
		return []HermesProfile{}, nil
	}
	entries, err := os.ReadDir(dir)
	if err != nil {
		return nil, fmt.Errorf("Failed to read profiles dir: %w", err)
	}
	profiles := []HermesProfile{}
	for _, entry := range entries {
		if entry.IsDir() || !strings.HasSuffix(entry.Name(), ".json") {
			continue
		}
		if profile, err := readHermesProfileFile(filepath.Join(dir, entry.Name())); err == nil {
			profiles = append(profiles, profile)
		}
	}
	sort.Slice(profiles, func(i, j int) bool {
		return strings.ToLower(profiles[i].Name) < strings.ToLower(profiles[j].Name)
	})
	return profiles, nil
}

// ---------------------------------------------------------------------------
// Commands — profile CRUD
// ---------------------------------------------------------------------------

// ListHermesProfiles lists all Hermes profiles sorted by name.
func (a *App) ListHermesProfiles() ([]HermesProfile, error) {
	return listHermesProfiles()
}

// GetHermesProfile returns one profile by id.
func (a *App) GetHermesProfile(id string) (HermesProfile, error) {
	path, err := hermesProfilePath(id)
	if err != nil {
		return HermesProfile{}, err
	}
	return readHermesProfileFile(path)
}

// SaveHermesProfile creates or updates a profile.
func (a *App) SaveHermesProfile(profile HermesProfile) error {
	if strings.TrimSpace(profile.ID) == "" {
		profile.ID = newUUID()
		profile.CreatedAt = nowRFC3339()
	} else {
		if existing, err := a.GetHermesProfile(profile.ID); err == nil {
			profile.CreatedAt = existing.CreatedAt
		} else if strings.TrimSpace(profile.CreatedAt) == "" {
			profile.CreatedAt = nowRFC3339()
		}
	}
	profile.UpdatedAt = nowRFC3339()
	profile.Models = normalizeHermesModels(profile.Models)
	return writeHermesProfileFile(profile)
}

// DeleteHermesProfile removes a profile and clears the active pointer.
func (a *App) DeleteHermesProfile(id string) error {
	path, err := hermesProfilePath(id)
	if err != nil {
		return err
	}
	if fileExists(path) {
		if err := os.Remove(path); err != nil {
			return fmt.Errorf("Failed to delete profile: %w", err)
		}
	}
	if active, err := a.GetActiveHermesProfileId(); err == nil && active != nil && *active == id {
		if activePath, err := hermesActiveProfilePath(); err == nil {
			_ = os.Remove(activePath)
		}
	}
	return nil
}

// DuplicateHermesProfile copies a profile under a new name and id.
func (a *App) DuplicateHermesProfile(id string, newName string) (HermesProfile, error) {
	profile, err := a.GetHermesProfile(id)
	if err != nil {
		return HermesProfile{}, err
	}
	profile.ID = newUUID()
	profile.Name = newName
	profile.CreatedAt = nowRFC3339()
	profile.UpdatedAt = profile.CreatedAt
	if err := writeHermesProfileFile(profile); err != nil {
		return HermesProfile{}, err
	}
	return profile, nil
}

// CreateDefaultHermesProfile seeds the first profile when none exist.
func (a *App) CreateDefaultHermesProfile() (HermesProfile, error) {
	profiles, err := listHermesProfiles()
	if err != nil {
		return HermesProfile{}, err
	}
	if len(profiles) > 0 {
		return HermesProfile{}, errors.New("Profiles already exist")
	}
	now := nowRFC3339()
	profile := HermesProfile{
		ID:        newUUID(),
		Name:      "默认",
		CreatedAt: now,
		UpdatedAt: now,
		Models:    []HermesModelConfig{{IsDefault: true}},
	}
	if err := writeHermesProfileFile(profile); err != nil {
		return HermesProfile{}, err
	}
	return profile, nil
}

// GetActiveHermesProfileId returns the active profile id (null when unset).
func (a *App) GetActiveHermesProfileId() (*string, error) {
	path, err := hermesActiveProfilePath()
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

// ---------------------------------------------------------------------------
// Apply / status / read current
// ---------------------------------------------------------------------------

// hermesProviderEntryBaseURL returns the endpoint of a providers-dict entry:
// `api` is the official v12 field name; `base_url` / `url` are accepted
// aliases.
func hermesProviderEntryBaseURL(item *yaml.Node) string {
	for _, key := range []string{"api", "base_url", "url"} {
		if value := core.MappingGetString(item, key); value != "" {
			return value
		}
	}
	return ""
}

// upsertHermesProviderEntry inserts or updates one named custom provider in
// the v12 `providers` dictionary. Only api / api_key / default_model are
// written; every other field (models, discover_models, transport, key_env,
// enabled, ...) keeps its existing value. Same-name entries in the legacy
// `custom_providers` list are migrated away — the same direction as
// `hermes update`'s v11→v12 migration — and the key itself is dropped once
// the list empties.
func upsertHermesProviderEntry(root *yaml.Node, name string, entry HermesModelConfig) error {
	providers := core.MappingGetMap(root, "providers")
	if providers == nil {
		if existing := core.MappingGet(root, "providers"); existing != nil {
			return errors.New("providers must be a YAML mapping")
		}
		providers = &yaml.Node{Kind: yaml.MappingNode}
		core.MappingSet(root, "providers", providers)
	}

	target := core.MappingGetMap(providers, name)
	if target == nil {
		if existing := core.MappingGet(providers, name); existing != nil {
			return errors.New("providers entry must be a YAML mapping")
		}
		target = &yaml.Node{Kind: yaml.MappingNode}
		core.MappingSet(providers, name, target)
	}

	if baseURL := nonEmpty(entry.BaseURL); baseURL != "" {
		core.MappingSet(target, "api", core.StringScalar(baseURL))
	}
	if apiKey := nonEmpty(entry.APIKey); apiKey != "" {
		core.MappingSet(target, "api_key", core.StringScalar(apiKey))
	}
	if model := nonEmpty(entry.Default); model != "" {
		core.MappingSet(target, "default_model", core.StringScalar(model))
	}

	if list := core.MappingGetSeq(root, "custom_providers"); list != nil {
		kept := list.Content[:0]
		for _, item := range list.Content {
			if core.MappingGetString(item, "name") == name {
				continue
			}
			kept = append(kept, item)
		}
		if len(kept) == 0 {
			core.MappingRemove(root, "custom_providers")
		} else {
			list.Content = kept
		}
	}
	return nil
}

// applyHermesProfileToConfig applies a profile to config.yaml using a
// read-modify-write cycle so unrelated sections survive.
func applyHermesProfileToConfig(profile HermesProfile, configPath string) error {
	root := &yaml.Node{Kind: yaml.MappingNode}
	if data, err := os.ReadFile(configPath); err == nil { // #nosec G304 -- tool config path
		if strings.TrimSpace(string(data)) != "" {
			parsed, err := core.ParseYAML(string(data))
			if err != nil {
				return fmt.Errorf("Failed to parse config.yaml: %w", err)
			}
			if parsed != nil {
				if !core.IsMapping(parsed) {
					return errors.New("config.yaml root must be a YAML mapping")
				}
				root = parsed
			}
		}
	} else if !os.IsNotExist(err) {
		return fmt.Errorf("Failed to read config.yaml: %w", err)
	}

	// 1. Named custom providers (custom:<name> with a base_url) → v12
	// providers dictionary; same-name legacy custom_providers entries are
	// migrated inside the upsert.
	for _, entry := range profile.Models {
		resolved := resolveHermesProvider(entry)
		if resolved == nil {
			continue
		}
		name, ok := strings.CutPrefix(*resolved, "custom:")
		if !ok || name == "" || nonEmpty(entry.BaseURL) == "" {
			continue
		}
		if err := upsertHermesProviderEntry(root, name, entry); err != nil {
			return err
		}
	}

	// 2. The model section (default entry drives model.default / model.provider).
	defaultEntry := pickHermesDefault(profile.Models)
	if defaultEntry != nil {
		resolved := resolveHermesProvider(*defaultEntry)
		hasModel := nonEmpty(defaultEntry.Default) != ""
		if hasModel || resolved != nil {
			modelSection := core.MappingGetMap(root, "model")
			if modelSection == nil {
				if existing := core.MappingGet(root, "model"); existing != nil {
					return errors.New("model section must be a YAML mapping")
				}
				modelSection = &yaml.Node{Kind: yaml.MappingNode}
				core.MappingSet(root, "model", modelSection)
			}
			if hasModel {
				core.MappingSet(modelSection, "default", core.StringScalar(nonEmpty(defaultEntry.Default)))
			}
			if resolved != nil {
				core.MappingSet(modelSection, "provider", core.StringScalar(*resolved))
				switch *resolved {
				case "custom":
					if url := nonEmpty(defaultEntry.BaseURL); url != "" {
						core.MappingSet(modelSection, "base_url", core.StringScalar(url))
					} else {
						core.MappingRemove(modelSection, "base_url")
					}
					if key := nonEmpty(defaultEntry.APIKey); key != "" {
						core.MappingSet(modelSection, "api_key", core.StringScalar(key))
					} else {
						core.MappingRemove(modelSection, "api_key")
					}
				default:
					if strings.HasPrefix(*resolved, "custom:") {
						core.MappingRemove(modelSection, "base_url")
						core.MappingRemove(modelSection, "api_key")
					}
				}
			}
		}
	}

	// 3. agent.reasoning_effort when set.
	if profile.ReasoningEffort != nil {
		agent := core.MappingGetMap(root, "agent")
		if agent == nil {
			if existing := core.MappingGet(root, "agent"); existing != nil {
				return errors.New("agent section must be a YAML mapping")
			}
			agent = &yaml.Node{Kind: yaml.MappingNode}
			core.MappingSet(root, "agent", agent)
		}
		core.MappingSet(agent, "reasoning_effort", core.StringScalar(*profile.ReasoningEffort))
	}

	text, err := core.MarshalYAML(root)
	if err != nil {
		return fmt.Errorf("Failed to serialize config.yaml: %w", err)
	}
	return core.AtomicWrite(configPath, core.BytesOf(text))
}

func pickHermesDefault(models []HermesModelConfig) *HermesModelConfig {
	for i := range models {
		if models[i].IsDefault {
			return &models[i]
		}
	}
	if len(models) > 0 {
		return &models[0]
	}
	return nil
}

// ApplyHermesProfile applies a profile to ~/.hermes/config.yaml.
func (a *App) ApplyHermesProfile(id string) error {
	profile, err := a.GetHermesProfile(id)
	if err != nil {
		return err
	}
	configPath, err := hermesConfigPath()
	if err != nil {
		return err
	}
	if err := applyHermesProfileToConfig(profile, configPath); err != nil {
		return err
	}
	activePath, err := hermesActiveProfilePath()
	if err != nil {
		return err
	}
	return core.AtomicWrite(activePath, core.BytesOf(id))
}

// GetHermesConfigStatus reports config.yaml state.
func (a *App) GetHermesConfigStatus() (HermesConfigStatus, error) {
	configPath, err := hermesConfigPath()
	if err != nil {
		return HermesConfigStatus{}, err
	}
	return HermesConfigStatus{ConfigExists: fileExists(configPath), ConfigPath: configPath}, nil
}

// ReadHermesCurrentConfig merges the live model section and custom_providers.
func (a *App) ReadHermesCurrentConfig() (HermesCurrentConfig, error) {
	configPath, err := hermesConfigPath()
	if err != nil {
		return HermesCurrentConfig{}, err
	}
	data, err := os.ReadFile(configPath) // #nosec G304 -- tool config path
	if err != nil || strings.TrimSpace(string(data)) == "" {
		return HermesCurrentConfig{Models: []HermesModelConfig{}}, nil
	}
	root, err := core.ParseYAML(string(data))
	if err != nil {
		return HermesCurrentConfig{}, fmt.Errorf("Failed to parse config.yaml: %w", err)
	}

	// v12 providers dictionary (primary): only entries carrying an endpoint
	// (api/base_url/url) are custom providers; endpoint-less entries (e.g.
	// catalog overrides for first-party providers) are skipped.
	models := []HermesModelConfig{}
	seen := map[string]bool{}
	if registry := core.MappingGetMap(root, "providers"); registry != nil {
		for i := 0; i+1 < len(registry.Content); i += 2 {
			keyNode, item := registry.Content[i], registry.Content[i+1]
			if keyNode.Kind != yaml.ScalarNode {
				continue
			}
			name := strings.TrimSpace(keyNode.Value)
			if name == "" || !core.IsMapping(item) {
				continue
			}
			baseURL := hermesProviderEntryBaseURL(item)
			if baseURL == "" {
				continue
			}
			def := core.MappingGetString(item, "default_model")
			if def == "" {
				def = core.MappingGetString(item, "model") // legacy field name
			}
			models = append(models, HermesModelConfig{
				Name:      ptrIfSet(name),
				Default:   ptrIfSet(def),
				BaseURL:   ptrIfSet(baseURL),
				APIKey:    ptrIfSet(core.MappingGetString(item, "api_key")),
				IsDefault: false,
			})
			seen[name] = true
		}
	}

	// Legacy custom_providers list (v11): still read; dict entries win on a
	// name collision.
	if list := core.MappingGetSeq(root, "custom_providers"); list != nil {
		for _, item := range list.Content {
			if !core.IsMapping(item) {
				continue
			}
			name := core.MappingGetString(item, "name")
			if name != "" && seen[name] {
				continue
			}
			models = append(models, HermesModelConfig{
				Name:      ptrIfSet(name),
				Default:   ptrIfSet(core.MappingGetString(item, "model")),
				BaseURL:   ptrIfSet(core.MappingGetString(item, "base_url")),
				APIKey:    ptrIfSet(core.MappingGetString(item, "api_key")),
				IsDefault: false,
			})
		}
	}

	modelSection := core.MappingGetMap(root, "model")
	getStr := func(key string) string {
		if modelSection == nil {
			return ""
		}
		child := core.MappingGet(modelSection, key)
		if child != nil && child.Kind == yaml.ScalarNode {
			return child.Value
		}
		return ""
	}
	liveDefault := getStr("default")
	if liveDefault == "" {
		liveDefault = getStr("model")
	}
	liveProvider := getStr("provider")
	liveBaseURL := getStr("base_url")
	liveAPIKey := getStr("api_key")

	switch {
	case strings.HasPrefix(liveProvider, "custom:") && len(liveProvider) > len("custom:"):
		name := liveProvider[len("custom:"):]
		matched := false
		for i := range models {
			if nonEmpty(models[i].Name) == name {
				models[i].IsDefault = true
				if models[i].Default == nil && liveDefault != "" {
					models[i].Default = &liveDefault
				}
				matched = true
				break
			}
		}
		if !matched {
			// The named provider is missing from custom_providers (e.g. a
			// hand-written config, or an older droidgear that only wrote the
			// model section). Carry the model section's base_url/api_key over
			// so a later Apply upserts them into custom_providers instead of
			// hitting the "custom:<name>" branch that strips both keys from
			// the model section — that combination used to destroy the live
			// endpoint/key.
			models = append(models, HermesModelConfig{
				Name:      &name,
				Default:   ptrIfSet(liveDefault),
				BaseURL:   ptrIfSet(liveBaseURL),
				APIKey:    ptrIfSet(liveAPIKey),
				IsDefault: true,
			})
		}
	case liveProvider == "custom":
		models = append(models, HermesModelConfig{
			Default: ptrIfSet(liveDefault), Provider: strPtr("custom"),
			BaseURL: ptrIfSet(liveBaseURL), APIKey: ptrIfSet(liveAPIKey), IsDefault: true,
		})
	case liveProvider != "":
		// Bare-name reference (what `hermes model` writes): match a
		// providers-dict key first; otherwise treat as a first-party
		// provider (openrouter / deepseek / ...).
		matched := false
		for i := range models {
			if nonEmpty(models[i].Name) == liveProvider {
				models[i].IsDefault = true
				if models[i].Default == nil && liveDefault != "" {
					models[i].Default = &liveDefault
				}
				matched = true
				break
			}
		}
		if !matched {
			models = append(models, HermesModelConfig{
				Default: ptrIfSet(liveDefault), Provider: &liveProvider,
				BaseURL: ptrIfSet(liveBaseURL), APIKey: ptrIfSet(liveAPIKey), IsDefault: true,
			})
		}
	default:
		if liveDefault != "" || liveBaseURL != "" || liveAPIKey != "" {
			models = append(models, HermesModelConfig{
				Default: ptrIfSet(liveDefault), BaseURL: ptrIfSet(liveBaseURL),
				APIKey: ptrIfSet(liveAPIKey), IsDefault: true,
			})
		}
	}

	if len(models) > 0 {
		anyDefault := false
		for _, entry := range models {
			if entry.IsDefault {
				anyDefault = true
				break
			}
		}
		if !anyDefault {
			models[0].IsDefault = true
		}
	}

	var reasoningEffort *string
	if agent := core.MappingGetMap(root, "agent"); agent != nil {
		child := core.MappingGet(agent, "reasoning_effort")
		if child != nil && child.Kind == yaml.ScalarNode && child.Value != "" {
			reasoningEffort = &child.Value
		}
	}

	return HermesCurrentConfig{Models: models, ReasoningEffort: reasoningEffort}, nil
}

func ptrIfSet(value string) *string {
	if value == "" {
		return nil
	}
	return &value
}

func strPtr(value string) *string {
	return &value
}
