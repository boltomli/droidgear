package app

import (
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"strings"

	"droidgear-wails/internal/core"

	"gopkg.in/yaml.v3"
)

// ---------------------------------------------------------------------------
// Constants (mirroring droidgear-core::dsh)
// ---------------------------------------------------------------------------

const (
	dshLLMSection   = "llm-pi-ai"
	dshLLMEntryName = "@deepseek-ai/dsh-llm-pi-ai"
	dshProvidersKey = "providers"

	dshPatchFilename       = "cordis.patch.yml"
	dshLegacySettingsName  = "settings.yaml"
	dshCredentialsFilename = ".credentials.yaml"

	credentialsVersion = 1
)

var dshOfficialProfiles = []string{"desktop", "web"}

// dshReasoningLevels are the pi-ai thinking levels a route may declare as its
// default (`providers.<id>.reasoning`).
var dshReasoningLevels = []string{"off", "minimal", "low", "medium", "high", "xhigh", "max"}

// dshInputModalities are the request modalities pi-ai accepts.
var dshInputModalities = []string{"text", "image"}

// dshDevRoleProtocols accept `compat.supportsDeveloperRole`.
var dshDevRoleProtocols = map[string]bool{
	"openai-completions":     true,
	"openai-responses":       true,
	"azure-openai-responses": true,
	"openai-codex-responses": true,
}

// ---------------------------------------------------------------------------
// Types (JSON field names mirror the tauri-specta bindings)
// ---------------------------------------------------------------------------

// DshCompatConfig mirrors dsh::DshCompatConfig.
type DshCompatConfig struct {
	SupportsDeveloperRole *bool                      `json:"supportsDeveloperRole"`
	Extra                 map[string]json.RawMessage `json:"-"`
}

// DshModel mirrors dsh::DshModel.
type DshModel struct {
	ID               string                     `json:"id"`
	Name             *string                    `json:"name"`
	ContextWindow    *uint32                    `json:"contextWindow"`
	MaxTokens        *uint32                    `json:"maxTokens"`
	ReasoningEfforts map[string]*string         `json:"reasoningEfforts"`
	Input            *[]string                  `json:"input"`
	Extra            map[string]json.RawMessage `json:"-"`
}

// DshProviderConfig mirrors dsh::DshProviderConfig (note `baseURL`).
type DshProviderConfig struct {
	DisplayName *string                    `json:"displayName"`
	BaseURL     *string                    `json:"baseURL"`
	APIKeyEnv   *string                    `json:"apiKeyEnv"`
	API         *string                    `json:"api"`
	Reasoning   *string                    `json:"reasoning"`
	Compat      *DshCompatConfig           `json:"compat"`
	Models      []DshModel                 `json:"models"`
	Extra       map[string]json.RawMessage `json:"-"`
}

// DshCurrentConfig mirrors dsh::DshCurrentConfig.
type DshCurrentConfig struct {
	Providers map[string]DshProviderConfig `json:"providers"`
}

// DshProfile mirrors dsh::DshProfile.
type DshProfile struct {
	Name      string `json:"name"`
	Dir       string `json:"dir"`
	PatchPath string `json:"patchPath"`
}

// DshConfigStatus mirrors dsh::DshConfigStatus.
type DshConfigStatus struct {
	ProfileName       *string `json:"profileName"`
	ConfigExists      bool    `json:"configExists"`
	ConfigPath        string  `json:"configPath"`
	LegacyExists      bool    `json:"legacyExists"`
	LegacyPath        string  `json:"legacyPath"`
	CredentialsExists bool    `json:"credentialsExists"`
	CredentialsPath   string  `json:"credentialsPath"`
}

// DshCredentials mirrors dsh::DshCredentials.
type DshCredentials struct {
	Version uint32            `json:"version"`
	Refs    map[string]string `json:"refs"`
}

// ---------------------------------------------------------------------------
// JSON (un)marshal with extra preservation
// ---------------------------------------------------------------------------

func (c DshCompatConfig) MarshalJSON() ([]byte, error) {
	type alias DshCompatConfig
	return core.MarshalJSONWithExtra(alias(c), c.Extra)
}

func (c *DshCompatConfig) UnmarshalJSON(data []byte) error {
	type plain DshCompatConfig
	var p plain
	if err := core.UnmarshalJSONWithExtra(data, &p); err != nil {
		return err
	}
	*c = DshCompatConfig(p)
	return nil
}

func (m DshModel) MarshalJSON() ([]byte, error) {
	type alias DshModel
	return core.MarshalJSONWithExtra(alias(m), m.Extra)
}

func (m *DshModel) UnmarshalJSON(data []byte) error {
	type plain DshModel
	var p plain
	if err := core.UnmarshalJSONWithExtra(data, &p); err != nil {
		return err
	}
	*m = DshModel(p)
	return nil
}

func (p DshProviderConfig) MarshalJSON() ([]byte, error) {
	type alias DshProviderConfig
	return core.MarshalJSONWithExtra(alias(p), p.Extra)
}

func (p *DshProviderConfig) UnmarshalJSON(data []byte) error {
	type plain DshProviderConfig
	var v plain
	if err := core.UnmarshalJSONWithExtra(data, &v); err != nil {
		return err
	}
	*p = DshProviderConfig(v)
	return nil
}

// ---------------------------------------------------------------------------
// Paths
// ---------------------------------------------------------------------------

func dshSettingsPath() (string, error) {
	home, err := core.DshHome()
	if err != nil {
		return "", err
	}
	return filepath.Join(home, dshLegacySettingsName), nil
}

func dshProfilesDir() (string, error) {
	home, err := core.DshHome()
	if err != nil {
		return "", err
	}
	return filepath.Join(home, "profiles"), nil
}

func dshProfilePatchPath(name string) (string, error) {
	dir, err := dshProfilesDir()
	if err != nil {
		return "", err
	}
	return filepath.Join(dir, name, dshPatchFilename), nil
}

func dshHomePatchPath() (string, error) {
	home, err := core.DshHome()
	if err != nil {
		return "", err
	}
	return filepath.Join(home, dshPatchFilename), nil
}

func dshCredentialsPath() (string, error) {
	home, err := core.DshHome()
	if err != nil {
		return "", err
	}
	return filepath.Join(home, dshCredentialsFilename), nil
}

// ---------------------------------------------------------------------------
// Profile discovery / resolution
// ---------------------------------------------------------------------------

// ListDshProfiles lists the official Dsh runtime profiles (desktop, web)
// under ~/.dsh/profiles/, in that order.
func (a *App) ListDshProfiles() ([]DshProfile, error) {
	dir, err := dshProfilesDir()
	if err != nil {
		return nil, err
	}
	if !core.DirExists(dir) {
		return []DshProfile{}, nil
	}
	var profiles []DshProfile
	for _, name := range dshOfficialProfiles {
		profileDir := filepath.Join(dir, name)
		patchPath := filepath.Join(profileDir, dshPatchFilename)
		isProfile := core.DirExists(profileDir) &&
			(fileExists(filepath.Join(profileDir, "package.json")) || fileExists(patchPath))
		if isProfile {
			profiles = append(profiles, DshProfile{
				Name:      name,
				Dir:       profileDir,
				PatchPath: patchPath,
			})
		}
	}
	if profiles == nil {
		profiles = []DshProfile{}
	}
	return profiles, nil
}

func fileExists(path string) bool {
	info, err := os.Stat(path)
	return err == nil && info.Mode().IsRegular()
}

// resolveDshProfile resolves the official profile DroidGear manages.
func resolveDshProfile(requested *string) (*string, error) {
	if requested != nil {
		name := *requested
		found := false
		for _, official := range dshOfficialProfiles {
			if official == name {
				found = true
				break
			}
		}
		if !found {
			return nil, fmt.Errorf("Dsh profile '%s' is not an official runtime profile (%s)",
				name, strings.Join(dshOfficialProfiles, " / "))
		}
		dir, err := dshProfilesDir()
		if err != nil {
			return nil, err
		}
		profileDir := filepath.Join(dir, name)
		if !fileExists(filepath.Join(profileDir, "package.json")) &&
			!fileExists(filepath.Join(profileDir, dshPatchFilename)) {
			return nil, fmt.Errorf("Dsh profile '%s' does not exist", name)
		}
		return &name, nil
	}
	profiles, err := (&App{}).ListDshProfiles()
	if err != nil {
		return nil, err
	}
	if len(profiles) == 0 {
		return nil, nil
	}
	name := profiles[0].Name
	return &name, nil
}

// ---------------------------------------------------------------------------
// Patch document read/write
// ---------------------------------------------------------------------------

type patchDocument struct {
	entries []*yaml.Node
	raw     string
	root    *yaml.Node
}

// readPatchDocument reads a patch document: a YAML sequence of loader patch
// entry mappings. A missing or empty file yields an empty document; `null`
// roots are accepted as empty too.
func readPatchDocument(path string) (*patchDocument, error) {
	doc := &patchDocument{raw: ""}
	data, err := os.ReadFile(path) // #nosec G304 -- tool config path
	if err != nil {
		if os.IsNotExist(err) {
			return doc, nil
		}
		return nil, fmt.Errorf("Failed to read Dsh patch file: %w", err)
	}
	doc.raw = string(data)
	if strings.TrimSpace(doc.raw) == "" {
		return doc, nil
	}
	root, err := core.ParseYAML(doc.raw)
	if err != nil {
		return nil, fmt.Errorf("Invalid Dsh patch YAML: %w", err)
	}
	switch {
	case root == nil:
		return doc, nil
	case core.IsSequence(root):
		doc.root = root
		doc.entries = root.Content
		return doc, nil
	case root.Kind == yaml.ScalarNode && root.Tag == "!!null":
		return doc, nil
	default:
		return nil, errors.New("Dsh patch file root must be a sequence")
	}
}

func patchEntryID(entry *yaml.Node) string {
	if !core.IsMapping(entry) {
		return ""
	}
	id := core.MappingGet(entry, "id")
	if id != nil && id.Kind == yaml.ScalarNode {
		return id.Value
	}
	return ""
}

func findLLMEntry(entries []*yaml.Node) int {
	for i, entry := range entries {
		if patchEntryID(entry) == dshLLMSection {
			return i
		}
	}
	return -1
}

func newLLMEntry() *yaml.Node {
	entry := &yaml.Node{Kind: yaml.MappingNode}
	core.MappingSet(entry, "id", core.StringScalar(dshLLMSection))
	core.MappingSet(entry, "name", core.StringScalar(dshLLMEntryName))
	core.MappingSet(entry, "config", &yaml.Node{Kind: yaml.MappingNode})
	return entry
}

// writePatchDocument rewrites the patch file while preserving untouched entry
// blocks verbatim (so `!!js` expressions survive); only the llm-pi-ai entry
// is re-serialized.
func writePatchDocument(path string, doc *patchDocument) error {
	_, blocks := core.SplitPatchDocument(doc.raw)
	appendedLLM := len(doc.entries) == len(blocks)+1 &&
		len(doc.entries) > 0 && patchEntryID(doc.entries[len(doc.entries)-1]) == dshLLMSection
	if !appendedLLM && len(doc.entries) != len(blocks) {
		return errors.New("Dsh patch layout could not be matched for safe rewriting; edit the file manually")
	}

	header := core.LeadingCommentBlock(doc.raw)
	if len(blocks) > 0 {
		header = ""
		rawHeader, _ := core.SplitPatchDocument(doc.raw)
		header = rawHeader
	}
	if appendedLLM {
		blocks = append(blocks, "")
	}

	index := findLLMEntry(doc.entries)
	if index < 0 {
		return nil
	}
	if blocks[index] != "" && strings.Contains(blocks[index], "!!") {
		return fmt.Errorf("Dsh patch entry '%s' contains tagged values (`!!...`) that cannot be rewritten safely; edit the file manually", dshLLMSection)
	}
	serialized, err := core.SerializePatchEntry(doc.entries[index])
	if err != nil {
		return err
	}
	blocks[index] = serialized

	if parent := filepath.Dir(path); !core.DirExists(parent) {
		if err := core.EnsureDir(parent); err != nil {
			return fmt.Errorf("Failed to create Dsh profile directory: %w", err)
		}
	}
	var out strings.Builder
	out.WriteString(header)
	for _, block := range blocks {
		out.WriteString(block)
	}
	return core.AtomicWrite(path, core.BytesOf(out.String()))
}

// ensureLLMEntryRewritable refuses writes that would re-serialize an
// llm-pi-ai entry whose raw block contains `!!` tag tokens.
func ensureLLMEntryRewritable(doc *patchDocument) error {
	index := findLLMEntry(doc.entries)
	if index < 0 {
		return nil
	}
	_, blocks := core.SplitPatchDocument(doc.raw)
	if index < len(blocks) && strings.Contains(blocks[index], "!!") {
		return fmt.Errorf("Dsh patch entry '%s' contains tagged values (`!!...`) that cannot be rewritten safely; edit the file manually", dshLLMSection)
	}
	return nil
}

// ---------------------------------------------------------------------------
// Provider map parsing / serialization
// ---------------------------------------------------------------------------

func parseProviderMap(node *yaml.Node) (map[string]DshProviderConfig, error) {
	result := map[string]DshProviderConfig{}
	if !core.IsMapping(node) {
		return result, nil
	}
	for i := 0; i+1 < len(node.Content); i += 2 {
		key := node.Content[i]
		if key.Kind != yaml.ScalarNode {
			continue
		}
		valueJSON, err := core.NodeToJSON(node.Content[i+1])
		if err != nil {
			return nil, fmt.Errorf("Invalid Dsh provider '%s': %w", key.Value, err)
		}
		var config DshProviderConfig
		if err := json.Unmarshal(valueJSON, &config); err != nil {
			return nil, fmt.Errorf("Invalid Dsh provider '%s': %w", key.Value, err)
		}
		result[key.Value] = config
	}
	return result, nil
}

func readProvidersFromPatch(path string) (map[string]DshProviderConfig, error) {
	doc, err := readPatchDocument(path)
	if err != nil {
		return nil, err
	}
	index := findLLMEntry(doc.entries)
	if index < 0 {
		return map[string]DshProviderConfig{}, nil
	}
	config := core.MappingGet(doc.entries[index], "config")
	if config == nil {
		return map[string]DshProviderConfig{}, nil
	}
	providers := core.MappingGet(config, dshProvidersKey)
	if providers == nil {
		return map[string]DshProviderConfig{}, nil
	}
	if !core.IsMapping(providers) {
		return nil, fmt.Errorf("%s.config.%s must be a mapping", dshLLMSection, dshProvidersKey)
	}
	return parseProviderMap(providers)
}

func readSettingsDoc() (*yaml.Node, error) {
	path, err := dshSettingsPath()
	if err != nil {
		return nil, err
	}
	data, err := os.ReadFile(path) // #nosec G304 -- tool config path
	if err != nil {
		if os.IsNotExist(err) {
			return &yaml.Node{Kind: yaml.MappingNode}, nil
		}
		return nil, fmt.Errorf("Failed to read Dsh settings file: %w", err)
	}
	if strings.TrimSpace(string(data)) == "" {
		return &yaml.Node{Kind: yaml.MappingNode}, nil
	}
	root, err := core.ParseYAML(string(data))
	if err != nil {
		return nil, fmt.Errorf("Invalid Dsh settings YAML: %w", err)
	}
	if root == nil {
		return &yaml.Node{Kind: yaml.MappingNode}, nil
	}
	if !core.IsMapping(root) {
		return nil, errors.New("Dsh settings.yaml root must be a mapping")
	}
	return root, nil
}

func readProvidersFromLegacy() (map[string]DshProviderConfig, error) {
	path, err := dshSettingsPath()
	if err != nil {
		return nil, err
	}
	if !fileExists(path) {
		return map[string]DshProviderConfig{}, nil
	}
	root, err := readSettingsDoc()
	if err != nil {
		return nil, err
	}
	section := core.MappingGet(root, dshLLMSection)
	if section == nil {
		return map[string]DshProviderConfig{}, nil
	}
	providers := core.MappingGet(section, dshProvidersKey)
	if providers == nil {
		return map[string]DshProviderConfig{}, nil
	}
	if !core.IsMapping(providers) {
		return nil, fmt.Errorf("%s.%s must be a mapping", dshLLMSection, dshProvidersKey)
	}
	return parseProviderMap(providers)
}

// ---------------------------------------------------------------------------
// Commands
// ---------------------------------------------------------------------------

// ReadDshCurrentConfig reads the effective merged provider map.
func (a *App) ReadDshCurrentConfig(profile *string) (DshCurrentConfig, error) {
	providers, err := readProvidersFromLegacy()
	if err != nil {
		return DshCurrentConfig{}, err
	}
	resolved := profile
	if resolved == nil {
		resolved, err = resolveDshProfile(nil)
		if err != nil {
			return DshCurrentConfig{}, err
		}
	}
	if resolved != nil {
		patch, err := dshProfilePatchPath(*resolved)
		if err != nil {
			return DshCurrentConfig{}, err
		}
		fromPatch, err := readProvidersFromPatch(patch)
		if err != nil {
			return DshCurrentConfig{}, err
		}
		for id, config := range fromPatch {
			providers[id] = config
		}
	}
	homePatch, err := dshHomePatchPath()
	if err != nil {
		return DshCurrentConfig{}, err
	}
	if fileExists(homePatch) {
		fromHome, err := readProvidersFromPatch(homePatch)
		if err != nil {
			return DshCurrentConfig{}, err
		}
		for id, config := range fromHome {
			providers[id] = config
		}
	}
	if providers == nil {
		providers = map[string]DshProviderConfig{}
	}
	return DshCurrentConfig{Providers: providers}, nil
}

// validateDshProviderID mirrors the Rust provider id validation.
func validateDshProviderID(id string) error {
	if strings.TrimSpace(id) == "" {
		return errors.New("Invalid provider id")
	}
	for _, r := range id {
		if (r >= 'a' && r <= 'z') || (r >= 'A' && r <= 'Z') || (r >= '0' && r <= '9') || r == '-' || r == '_' {
			continue
		}
		return errors.New("Invalid provider id")
	}
	return nil
}

// prepareProviderConfig enriches models from the registry and validates the
// route before writing.
func prepareProviderConfig(config DshProviderConfig) (DshProviderConfig, error) {
	for i := range config.Models {
		model := &config.Models[i]
		enrichDshModel(model)
		if model.Input != nil {
			normalized := core.InputModalities(*model.Input)
			for _, value := range *model.Input {
				valid := false
				for _, allowed := range dshInputModalities {
					if strings.ToLower(strings.TrimSpace(value)) == allowed {
						valid = true
						break
					}
				}
				if !valid {
					return config, fmt.Errorf("Invalid Dsh model '%s' input modality; expected one of: %s",
						model.ID, strings.Join(dshInputModalities, ", "))
				}
			}
			if len(normalized) == 0 {
				model.Input = nil
			} else {
				model.Input = &normalized
			}
		}
	}

	if config.Reasoning != nil {
		normalized := strings.ToLower(strings.TrimSpace(*config.Reasoning))
		if normalized == "" {
			config.Reasoning = nil
		} else {
			valid := false
			for _, level := range dshReasoningLevels {
				if level == normalized {
					valid = true
					break
				}
			}
			if !valid {
				return config, fmt.Errorf("Invalid Dsh provider default reasoning level '%s'; expected one of: %s",
					*config.Reasoning, strings.Join(dshReasoningLevels, ", "))
			}
			config.Reasoning = &normalized
		}
	}

	// `compat.supportsDeveloperRole` only exists on OpenAI-style routes.
	var api string
	if config.API != nil {
		api = *config.API
	}
	if !dshDevRoleProtocols[api] && config.Compat != nil {
		config.Compat.SupportsDeveloperRole = nil
		if len(config.Compat.Extra) == 0 {
			config.Compat = nil
		}
	}
	return config, nil
}

// enrichDshModel fills metadata from the DroidGear model registry.
func enrichDshModel(model *DshModel) bool {
	metadata := core.FindRegistryModel(model.ID)
	if metadata == nil {
		return false
	}
	if model.Name == nil {
		name := metadata.Name
		model.Name = &name
	}
	if model.ContextWindow == nil {
		window := metadata.ContextWindow
		model.ContextWindow = &window
	}
	if model.MaxTokens == nil && metadata.MaxOutputTokens != nil {
		tokens := *metadata.MaxOutputTokens
		model.MaxTokens = &tokens
	}
	sameAsRegistry := mapsEqualStringPtr(model.ReasoningEfforts, metadata.ThinkingLevelMap)
	if model.ReasoningEfforts == nil || sameAsRegistry {
		model.ReasoningEfforts = core.RegistryReasoningEfforts(metadata)
	}
	if model.Input == nil && len(metadata.Input) > 0 {
		normalized := core.InputModalities(metadata.Input)
		if len(normalized) > 0 {
			model.Input = &normalized
		}
	}
	return true
}

func mapsEqualStringPtr(a, b map[string]*string) bool {
	if a == nil || b == nil {
		return a == nil && b == nil
	}
	if len(a) != len(b) {
		return false
	}
	for key, value := range a {
		other, ok := b[key]
		if !ok {
			return false
		}
		if value == nil || other == nil {
			if value != other {
				return false
			}
			continue
		}
		if *value != *other {
			return false
		}
	}
	return true
}

// SaveDshProvider inserts or updates one provider in the resolved target
// (profile cordis.patch.yml, or the legacy settings.yaml).
func (a *App) SaveDshProvider(profile *string, providerID string, config DshProviderConfig) error {
	providerID = strings.TrimSpace(providerID)
	if err := validateDshProviderID(providerID); err != nil {
		return err
	}
	config, err := prepareProviderConfig(config)
	if err != nil {
		return err
	}

	resolved, err := resolveDshProfile(profile)
	if err != nil {
		return err
	}
	if resolved == nil {
		return saveDshProviderLegacy(providerID, config)
	}

	patchPath, err := dshProfilePatchPath(*resolved)
	if err != nil {
		return err
	}
	doc, err := readPatchDocument(patchPath)
	if err != nil {
		return err
	}
	if err := ensureLLMEntryRewritable(doc); err != nil {
		return err
	}
	index := findLLMEntry(doc.entries)
	if index < 0 {
		doc.entries = append(doc.entries, newLLMEntry())
		doc.root.Content = append(doc.root.Content, doc.entries[len(doc.entries)-1])
		index = len(doc.entries) - 1
	}

	entry := doc.entries[index]
	configNode := core.MappingGet(entry, "config")
	if configNode == nil || !core.IsMapping(configNode) {
		if configNode != nil && !core.IsMapping(configNode) {
			return fmt.Errorf("%s config must be a mapping", dshLLMSection)
		}
		configNode = &yaml.Node{Kind: yaml.MappingNode}
		core.MappingSet(entry, "config", configNode)
	}
	providersNode := core.MappingGet(configNode, dshProvidersKey)
	if providersNode == nil || !core.IsMapping(providersNode) {
		if providersNode != nil && !core.IsMapping(providersNode) {
			return fmt.Errorf("%s.config.%s must be a mapping", dshLLMSection, dshProvidersKey)
		}
		providersNode = &yaml.Node{Kind: yaml.MappingNode}
		core.MappingSet(configNode, dshProvidersKey, providersNode)
	}

	valueNode, err := providerToYAMLNode(config)
	if err != nil {
		return err
	}
	core.MappingSet(providersNode, providerID, valueNode)
	return writePatchDocument(patchPath, doc)
}

// providerToYAMLNode converts a typed provider config (with extras) into a
// YAML node using the same key names as the JSON contract.
func providerToYAMLNode(config DshProviderConfig) (*yaml.Node, error) {
	data, err := config.MarshalJSON()
	if err != nil {
		return nil, err
	}
	return core.JSONToNode(data)
}

func saveDshProviderLegacy(providerID string, config DshProviderConfig) error {
	root, err := readSettingsDoc()
	if err != nil {
		return err
	}
	section := core.MappingGetMap(root, dshLLMSection)
	if section == nil {
		section = &yaml.Node{Kind: yaml.MappingNode}
		core.MappingSet(root, dshLLMSection, section)
	}
	providers := core.MappingGetMap(section, dshProvidersKey)
	if providers == nil {
		providers = &yaml.Node{Kind: yaml.MappingNode}
		core.MappingSet(section, dshProvidersKey, providers)
	}
	valueNode, err := providerToYAMLNode(config)
	if err != nil {
		return err
	}
	core.MappingSet(providers, providerID, valueNode)

	text, err := core.MarshalYAML(root)
	if err != nil {
		return fmt.Errorf("Failed to serialize Dsh settings: %w", err)
	}
	home, err := core.DshHome()
	if err != nil {
		return err
	}
	if err := core.EnsureDir(home); err != nil {
		return fmt.Errorf("Failed to create Dsh directory: %w", err)
	}
	path, err := dshSettingsPath()
	if err != nil {
		return err
	}
	return core.AtomicWrite(path, core.BytesOf(text))
}

// DeleteDshProvider removes one provider from the resolved target document.
func (a *App) DeleteDshProvider(profile *string, providerID string) error {
	providerID = strings.TrimSpace(providerID)
	if err := validateDshProviderID(providerID); err != nil {
		return err
	}
	resolved, err := resolveDshProfile(profile)
	if err != nil {
		return err
	}
	if resolved == nil {
		return deleteDshProviderLegacy(providerID)
	}

	patchPath, err := dshProfilePatchPath(*resolved)
	if err != nil {
		return err
	}
	doc, err := readPatchDocument(patchPath)
	if err != nil {
		return err
	}
	index := findLLMEntry(doc.entries)
	if index < 0 {
		return nil
	}
	if err := ensureLLMEntryRewritable(doc); err != nil {
		return err
	}
	entry := doc.entries[index]
	if configNode := core.MappingGet(entry, "config"); core.IsMapping(configNode) {
		if providersNode := core.MappingGet(configNode, dshProvidersKey); core.IsMapping(providersNode) {
			core.MappingRemove(providersNode, providerID)
		} else if providersNode != nil {
			return fmt.Errorf("%s.config.%s must be a mapping", dshLLMSection, dshProvidersKey)
		}
	} else if configNode != nil {
		return fmt.Errorf("%s config must be a mapping", dshLLMSection)
	}
	return writePatchDocument(patchPath, doc)
}

func deleteDshProviderLegacy(providerID string) error {
	root, err := readSettingsDoc()
	if err != nil {
		return err
	}
	if section := core.MappingGet(root, dshLLMSection); core.IsMapping(section) {
		if providers := core.MappingGet(section, dshProvidersKey); core.IsMapping(providers) {
			core.MappingRemove(providers, providerID)
		} else if providers != nil {
			return fmt.Errorf("%s.%s must be a mapping", dshLLMSection, dshProvidersKey)
		}
	} else if section != nil {
		return fmt.Errorf("%s must be a mapping", dshLLMSection)
	}
	text, err := core.MarshalYAML(root)
	if err != nil {
		return fmt.Errorf("Failed to serialize Dsh settings: %w", err)
	}
	path, err := dshSettingsPath()
	if err != nil {
		return err
	}
	return core.AtomicWrite(path, core.BytesOf(text))
}

// GetDshConfigStatus reports which files exist for the resolved profile.
func (a *App) GetDshConfigStatus(profile *string) (DshConfigStatus, error) {
	resolved, err := resolveDshProfile(profile)
	if err != nil {
		return DshConfigStatus{}, err
	}
	legacyPath, err := dshSettingsPath()
	if err != nil {
		return DshConfigStatus{}, err
	}
	credentialsPath, err := dshCredentialsPath()
	if err != nil {
		return DshConfigStatus{}, err
	}
	configPath := legacyPath
	if resolved != nil {
		if configPath, err = dshProfilePatchPath(*resolved); err != nil {
			return DshConfigStatus{}, err
		}
	}
	return DshConfigStatus{
		ProfileName:       resolved,
		ConfigExists:      fileExists(configPath),
		ConfigPath:        configPath,
		LegacyExists:      fileExists(legacyPath),
		LegacyPath:        legacyPath,
		CredentialsExists: fileExists(credentialsPath),
		CredentialsPath:   credentialsPath,
	}, nil
}

// ---------------------------------------------------------------------------
// Credentials (~/.dsh/.credentials.yaml)
// ---------------------------------------------------------------------------

func readCredentialsDoc() (*yaml.Node, error) {
	path, err := dshCredentialsPath()
	if err != nil {
		return nil, err
	}
	data, err := os.ReadFile(path) // #nosec G304 -- tool config path
	if err != nil {
		if os.IsNotExist(err) {
			return &yaml.Node{Kind: yaml.MappingNode}, nil
		}
		return nil, fmt.Errorf("Failed to read Dsh credentials file: %w", err)
	}
	if strings.TrimSpace(string(data)) == "" {
		return &yaml.Node{Kind: yaml.MappingNode}, nil
	}
	root, err := core.ParseYAML(string(data))
	if err != nil {
		return nil, fmt.Errorf("Invalid Dsh credentials YAML: %w", err)
	}
	if !core.IsMapping(root) {
		return nil, errors.New("Dsh credentials root must be a mapping")
	}
	if core.MappingGet(root, "version") == nil {
		return nil, errors.New("Dsh credentials file uses the pre-release flat layout; add `version: 1` and nest entries under `refs:`")
	}
	for i := 0; i+1 < len(root.Content); i += 2 {
		switch root.Content[i].Value {
		case "version", "refs", "records":
		default:
			return nil, fmt.Errorf("Dsh credentials file has unknown top-level key: %s", root.Content[i].Value)
		}
	}
	return root, nil
}

func writeCredentialsDoc(root *yaml.Node) error {
	text, err := core.MarshalYAML(root)
	if err != nil {
		return fmt.Errorf("Failed to serialize Dsh credentials: %w", err)
	}
	path, err := dshCredentialsPath()
	if err != nil {
		return err
	}
	home, err := core.DshHome()
	if err != nil {
		return err
	}
	if err := core.EnsureDir(home); err != nil {
		return fmt.Errorf("Failed to create Dsh directory: %w", err)
	}
	return core.AtomicWritePrivate(path, core.BytesOf(text))
}

func validateCredentialName(name string) error {
	if strings.TrimSpace(name) == "" {
		return errors.New("Invalid credential env var name")
	}
	for _, r := range name {
		if (r >= 'a' && r <= 'z') || (r >= 'A' && r <= 'Z') || (r >= '0' && r <= '9') || r == '_' || r == '-' {
			continue
		}
		return errors.New("Invalid credential env var name")
	}
	return nil
}

func checkCredentialsVersion(root *yaml.Node) error {
	version := core.MappingGet(root, "version")
	if version == nil || version.Kind != yaml.ScalarNode {
		return nil
	}
	if version.Value != fmt.Sprint(credentialsVersion) {
		return fmt.Errorf("Unsupported Dsh credentials version: %s", version.Value)
	}
	return nil
}

// ReadDshCredentials reads ~/.dsh/.credentials.yaml (version + refs).
func (a *App) ReadDshCredentials() (DshCredentials, error) {
	root, err := readCredentialsDoc()
	if err != nil {
		return DshCredentials{}, err
	}
	if err := checkCredentialsVersion(root); err != nil {
		return DshCredentials{}, err
	}
	refs := map[string]string{}
	if refsNode := core.MappingGet(root, "refs"); core.IsMapping(refsNode) {
		for i := 0; i+1 < len(refsNode.Content); i += 2 {
			keyNode, valueNode := refsNode.Content[i], refsNode.Content[i+1]
			if keyNode.Kind != yaml.ScalarNode {
				continue
			}
			if valueNode.Kind != yaml.ScalarNode {
				return DshCredentials{}, fmt.Errorf("Dsh credentials ref '%s' must be a string", keyNode.Value)
			}
			refs[keyNode.Value] = valueNode.Value
		}
	} else if refsNode != nil {
		return DshCredentials{}, errors.New("Dsh credentials 'refs' must be a mapping")
	}
	return DshCredentials{Version: credentialsVersion, Refs: refs}, nil
}

// SaveDshCredentialRef upserts one refs entry; an empty value removes it.
func (a *App) SaveDshCredentialRef(name string, value string) error {
	name = strings.TrimSpace(name)
	if err := validateCredentialName(name); err != nil {
		return err
	}
	root, err := readCredentialsDoc()
	if err != nil {
		return err
	}
	if err := checkCredentialsVersion(root); err != nil {
		return err
	}
	core.MappingSet(root, "version", &yaml.Node{Kind: yaml.ScalarNode, Tag: "!!int", Value: fmt.Sprint(credentialsVersion)})
	refs := core.MappingGetMap(root, "refs")
	if refs == nil {
		refs = &yaml.Node{Kind: yaml.MappingNode}
		core.MappingSet(root, "refs", refs)
	}
	if strings.TrimSpace(value) == "" {
		core.MappingRemove(refs, name)
	} else {
		core.MappingSet(refs, name, core.StringScalar(value))
	}
	return writeCredentialsDoc(root)
}

// DeleteDshCredentialRef removes one refs entry.
func (a *App) DeleteDshCredentialRef(name string) error {
	name = strings.TrimSpace(name)
	if err := validateCredentialName(name); err != nil {
		return err
	}
	root, err := readCredentialsDoc()
	if err != nil {
		return err
	}
	if refs := core.MappingGetMap(root, "refs"); refs != nil {
		core.MappingRemove(refs, name)
	}
	return writeCredentialsDoc(root)
}

// ---------------------------------------------------------------------------
// Advanced snapshot commands (not ported in this minimal build)
// ---------------------------------------------------------------------------

// ReadDshConfigSnapshot is part of the advanced config editor whose schema
// engine (droidgear-core::dsh_config) is not ported to Go yet. The UI
// degrades gracefully: the Advanced tab renders an empty state.
func (a *App) ReadDshConfigSnapshot(profile *string) (map[string]any, error) {
	return nil, errors.New("Dsh config snapshot is not supported in this minimal Wails build")
}

// SaveDshConfigEntry is not supported in this minimal build.
func (a *App) SaveDshConfigEntry(profile *string, entryID string, config any, expectedHash *string) (map[string]any, error) {
	return nil, errors.New("Dsh config snapshot is not supported in this minimal Wails build")
}

// SetDshConfigEntryDisabled is not supported in this minimal build.
func (a *App) SetDshConfigEntryDisabled(profile *string, entryID string, disabled bool, expectedHash *string) (map[string]any, error) {
	return nil, errors.New("Dsh config snapshot is not supported in this minimal Wails build")
}
