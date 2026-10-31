package app

// Generic DeepSeek Harness profile configuration (`cordis.patch.yml`) — Go
// port of droidgear-core::dsh_config.
//
// DSH 0.2.x has one user-writable layer per profile:
// `~/.dsh/profiles/<profile>/cordis.patch.yml`, a YAML sequence of loader
// patch rows (`- id: <entry>`, `name: <package>`, `config: {...}`,
// `disabled: true`). Layers compose lowest first:
//
//  1. the profile's bundle patches (declared by package.json
//     `dsh.profile.bundles`, baked into the embedded schema snapshot),
//  2. the profile's cordis.patch.yml (the only layer this module writes),
//  3. the home overlay `~/.dsh/cordis.patch.yml` (read-only for us).
//
// Writes mirror DSH's own config-editor semantics: a lock file next to
// package.json, a comment- and `!!js`-preserving document rewrite, schema
// validation, secret redaction, atomic replacement and a content hash for
// optimistic concurrency. Unlike the Rust core (serde_yaml resolves `!!js`
// away and needs a pre-parse text rewrite), yaml.v3 preserves both `!js`
// and `!!js` tags natively, so no tag normalization is required here.

import (
	_ "embed"
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"runtime"
	"sort"
	"strings"
	"sync"
	"time"

	"droidgear-wails/internal/core"

	"gopkg.in/yaml.v3"
)

// Prefix of a write refused because the patch changed since it was read
// (matched by the frontend store).
const dshConfigConflict = "DSH_CONFIG_CONFLICT"

// Prefix of a write refused because a higher layer overrides the entry.
const dshConfigShadowed = "DSH_CONFIG_SHADOWED"

const (
	profileLockFilename = "package.json.lock"
	staleLockAge        = 30 * time.Second
	lockWait            = 2 * time.Second
)

//go:embed schema/dsh-config-schema.json
var dshSchemaJSON []byte

// presetOnlyEntries: the web-app bundle disables these because the agent
// plane owns them; writing config for them at the host layer has no effect.
var presetOnlyEntries = map[string]bool{
	"agent-instructions": true, "command-compact": true, "command-goal": true,
	"compaction-basic": true, "plan-mode": true, "skill-filesystem": true,
	"tool-bash": true, "tool-fs": true, "tool-fs-search": true,
	"tool-goal": true, "tool-jobs": true, "tool-plugin-manager": true,
	"tool-pwsh": true, "tool-result-pruner": true, "tool-skill": true,
	"tool-subagent": true, "tool-subagent-control": true,
	"tool-subagent-fork": true, "tool-subagent-list-agents": true,
	"tool-todo": true, "tool-web": true, "tool-workflow": true,
	"workflow-ptc": true,
}

// ---------------------------------------------------------------------------
// Public types (JSON field names mirror the tauri-specta bindings)
// ---------------------------------------------------------------------------

// DshMigrationWarning mirrors dsh_config::DshMigrationWarning.
type DshMigrationWarning struct {
	Code     string  `json:"code"`
	Severity string  `json:"severity"`
	EntryID  *string `json:"entryId"`
	Message  string  `json:"message"`
	Hint     string  `json:"hint"`
}

// DshConfigFieldView mirrors dsh_config::DshConfigFieldView.
type DshConfigFieldView struct {
	Name        string   `json:"name"`
	Kind        string   `json:"kind"`
	Value       any      `json:"value"`
	Default     any      `json:"default"`
	EnumValues  []string `json:"enumValues"`
	Volatile    bool     `json:"volatile"`
	Secret      bool     `json:"secret"`
	Description string   `json:"description"`
	Overridden  bool     `json:"overridden"`
	Tier        string   `json:"tier"`
}

// DshConfigEntryView mirrors dsh_config::DshConfigEntryView.
type DshConfigEntryView struct {
	ID             string               `json:"id"`
	PackageName    string               `json:"packageName"`
	Layer          string               `json:"layer"`
	Tier           string               `json:"tier"`
	Description    string               `json:"description"`
	Disabled       bool                 `json:"disabled"`
	HasJS          bool                 `json:"hasJs"`
	Config         any                  `json:"config"`
	Overridden     bool                 `json:"overridden"`
	ShadowedByHome bool                 `json:"shadowedByHome"`
	Fields         []DshConfigFieldView `json:"fields"`
}

// DshConfigSnapshot mirrors dsh_config::DshConfigSnapshot.
type DshConfigSnapshot struct {
	SchemaVersion        string                `json:"schemaVersion"`
	InstalledVersion     *string               `json:"installedVersion"`
	Profile              *string               `json:"profile"`
	ProfilePatchPath     string                `json:"profilePatchPath"`
	HomePatchPath        string                `json:"homePatchPath"`
	HomePatchExists      bool                  `json:"homePatchExists"`
	LegacySettingsPath   string                `json:"legacySettingsPath"`
	LegacySettingsExists bool                  `json:"legacySettingsExists"`
	PatchHash            string                `json:"patchHash"`
	Entries              []DshConfigEntryView  `json:"entries"`
	Warnings             []DshMigrationWarning `json:"warnings"`
}

// DshConfigWriteResult mirrors dsh_config::DshConfigWriteResult.
type DshConfigWriteResult struct {
	PatchHash string `json:"patchHash"`
	Changed   bool   `json:"changed"`
}

// ---------------------------------------------------------------------------
// Schema snapshot
// ---------------------------------------------------------------------------

type schemaDoc struct {
	SchemaVersion int                     `json:"schemaVersion"`
	DshVersion    string                  `json:"dshVersion"`
	Bundles       map[string]schemaBundle `json:"bundles"`
	Entries       map[string]schemaEntry  `json:"entries"`
}

type schemaBundle struct {
	Entries []schemaRow `json:"entries"`
}

type schemaRow struct {
	ID                  string                     `json:"id"`
	Name                string                     `json:"name"`
	Config              any                        `json:"config"`
	DisabledByProfileOS map[string]map[string]bool `json:"disabledByProfileOS"`
}

type schemaEntry struct {
	Package     string        `json:"package"`
	Tier        string        `json:"tier"`
	Description string        `json:"description"`
	Fields      []schemaField `json:"fields"`
}

type schemaField struct {
	Name        string   `json:"name"`
	Kind        string   `json:"kind"`
	Default     any      `json:"default"`
	Nullable    bool     `json:"nullable"`
	EnumValues  []string `json:"enumValues"`
	Volatile    bool     `json:"volatile"`
	Secret      bool     `json:"secret"`
	Tier        string   `json:"tier"`
	Description string   `json:"description"`
}

// rawSchemaField decodes enumValues with nested-array flattening (the
// generator may emit nested arrays for a schemastery union).
type rawSchemaField struct {
	Name        string `json:"name"`
	Kind        string `json:"kind"`
	Default     any    `json:"default"`
	Nullable    bool   `json:"nullable"`
	EnumValues  []any  `json:"enumValues"`
	Volatile    bool   `json:"volatile"`
	Secret      bool   `json:"secret"`
	Tier        string `json:"tier"`
	Description string `json:"description"`
}

func (f *rawSchemaField) toField() schemaField {
	var enumValues []string
	var flatten func(value any)
	flatten = func(value any) {
		switch typed := value.(type) {
		case string:
			enumValues = append(enumValues, typed)
		case []any:
			for _, item := range typed {
				flatten(item)
			}
		}
	}
	for _, item := range f.EnumValues {
		flatten(item)
	}
	kind := f.Kind
	if kind == "" {
		kind = "string"
	}
	return schemaField{
		Name: f.Name, Kind: kind, Default: f.Default, Nullable: f.Nullable,
		EnumValues: enumValues, Volatile: f.Volatile, Secret: f.Secret,
		Tier: f.Tier, Description: f.Description,
	}
}

// wireSchemaEntry lets Entries decode through the raw field form.
type wireSchemaEntry struct {
	Package     string           `json:"package"`
	Tier        string           `json:"tier"`
	Description string           `json:"description"`
	Fields      []rawSchemaField `json:"fields"`
}

var (
	schemaOnce     sync.Once
	schemaDocValue *schemaDoc
	schemaErr      error
)

func dshSchema() (*schemaDoc, error) {
	schemaOnce.Do(func() {
		var wire struct {
			SchemaVersion int                        `json:"schemaVersion"`
			DshVersion    string                     `json:"dshVersion"`
			Bundles       map[string]schemaBundle    `json:"bundles"`
			Entries       map[string]wireSchemaEntry `json:"entries"`
		}
		if err := json.Unmarshal(dshSchemaJSON, &wire); err != nil {
			schemaErr = fmt.Errorf("invalid embedded dsh-config-schema.json: %w", err)
			return
		}
		if wire.SchemaVersion != 1 {
			schemaErr = fmt.Errorf("unsupported dsh-config-schema.json version: %d", wire.SchemaVersion)
			return
		}
		entries := make(map[string]schemaEntry, len(wire.Entries))
		for id, wireEntry := range wire.Entries {
			fields := make([]schemaField, 0, len(wireEntry.Fields))
			for _, raw := range wireEntry.Fields {
				fields = append(fields, raw.toField())
			}
			entries[id] = schemaEntry{
				Package: wireEntry.Package, Tier: wireEntry.Tier,
				Description: wireEntry.Description, Fields: fields,
			}
		}
		schemaDocValue = &schemaDoc{
			SchemaVersion: wire.SchemaVersion,
			DshVersion:    wire.DshVersion,
			Bundles:       wire.Bundles,
			Entries:       entries,
		}
	})
	return schemaDocValue, schemaErr
}

// ---------------------------------------------------------------------------
// YAML <-> JSON with `$js` marker round-trip
// ---------------------------------------------------------------------------

const jsTag = "!js"

func isJSNode(node *yaml.Node) bool {
	return node != nil && node.Kind == yaml.ScalarNode &&
		(node.Tag == jsTag || node.Tag == "!!js" || node.Tag == "tag:yaml.org,2002:js")
}

// yamlToJSON converts a YAML node tree into generic JSON-ready values,
// mapping tagged `!js` scalars to {"$js": "<expression>"} markers.
func yamlToJSON(node *yaml.Node) any {
	if node == nil {
		return nil
	}
	switch node.Kind {
	case yaml.AliasNode:
		return yamlToJSON(node.Alias)
	case yaml.ScalarNode:
		if isJSNode(node) {
			return map[string]any{"$js": node.Value}
		}
		if node.Tag == "!!null" || (node.Tag == "" && node.Value == "") {
			return nil
		}
		var value any
		if err := node.Decode(&value); err != nil {
			return node.Value
		}
		return value
	case yaml.SequenceNode:
		items := make([]any, 0, len(node.Content))
		for _, item := range node.Content {
			items = append(items, yamlToJSON(item))
		}
		return items
	case yaml.MappingNode:
		object := make(map[string]any, len(node.Content)/2)
		for i := 0; i+1 < len(node.Content); i += 2 {
			key, value := node.Content[i], node.Content[i+1]
			var name string
			if key.Kind == yaml.ScalarNode {
				name = key.Value
			} else {
				rendered, err := core.MarshalYAML(key)
				if err == nil {
					name = strings.TrimSpace(rendered)
				}
			}
			object[name] = yamlToJSON(value)
		}
		return object
	default:
		return nil
	}
}

// jsonToYAMLNode converts generic JSON-ready values back into YAML nodes,
// rendering {"$js": expr} markers as `!js` tagged scalars.
func jsonToYAMLNode(value any) *yaml.Node {
	switch typed := value.(type) {
	case nil:
		return &yaml.Node{Kind: yaml.ScalarNode, Tag: "!!null", Value: "null"}
	case bool:
		return &yaml.Node{Kind: yaml.ScalarNode, Tag: "!!bool", Value: fmt.Sprint(typed)}
	case string:
		return &yaml.Node{Kind: yaml.ScalarNode, Tag: "!!str", Value: typed}
	case float64:
		return &yaml.Node{Kind: yaml.ScalarNode, Tag: "!!float", Value: json.Number(fmt.Sprint(typed)).String()}
	case int:
		return &yaml.Node{Kind: yaml.ScalarNode, Tag: "!!int", Value: fmt.Sprint(typed)}
	case []any:
		node := &yaml.Node{Kind: yaml.SequenceNode}
		for _, item := range typed {
			node.Content = append(node.Content, jsonToYAMLNode(item))
		}
		return node
	case map[string]any:
		if expression, ok := jsMarkerExpression(typed); ok {
			return &yaml.Node{Kind: yaml.ScalarNode, Tag: jsTag, Value: expression}
		}
		keys := make([]string, 0, len(typed))
		for key := range typed {
			keys = append(keys, key)
		}
		sort.Strings(keys)
		node := &yaml.Node{Kind: yaml.MappingNode}
		for _, key := range keys {
			node.Content = append(node.Content,
				&yaml.Node{Kind: yaml.ScalarNode, Tag: "!!str", Value: key},
				jsonToYAMLNode(typed[key]),
			)
		}
		return node
	default:
		// Fallback for unexpected types (e.g. json.Number).
		rendered, err := json.Marshal(typed)
		if err != nil {
			return &yaml.Node{Kind: yaml.ScalarNode, Tag: "!!null", Value: "null"}
		}
		var normalized any
		if err := json.Unmarshal(rendered, &normalized); err != nil {
			return &yaml.Node{Kind: yaml.ScalarNode, Tag: "!!null", Value: "null"}
		}
		return jsonToYAMLNode(normalized)
	}
}

func jsMarkerExpression(object map[string]any) (string, bool) {
	if len(object) != 1 {
		return "", false
	}
	expression, ok := object["$js"].(string)
	return expression, ok
}

func isJSMarker(value any) bool {
	object, ok := value.(map[string]any)
	if !ok {
		return false
	}
	_, isJS := jsMarkerExpression(object)
	return isJS
}

func isSecretMarker(value any) bool {
	object, ok := value.(map[string]any)
	if !ok || len(object) != 1 {
		return false
	}
	secret, ok := object["$secret"].(bool)
	return ok && secret
}

func containsJS(value any) bool {
	switch typed := value.(type) {
	case map[string]any:
		if isJSMarker(value) {
			return true
		}
		for _, item := range typed {
			if containsJS(item) {
				return true
			}
		}
	case []any:
		for _, item := range typed {
			if containsJS(item) {
				return true
			}
		}
	}
	return false
}

// ---------------------------------------------------------------------------
// Composition
// ---------------------------------------------------------------------------

type composedEntry struct {
	ID            string
	PackageName   string
	Description   string
	Tier          string
	Config        any
	Disabled      bool
	Layer         string
	ProfileConfig any
	HomeConfig    any
}

func osKey() string {
	switch runtime.GOOS {
	case "windows":
		return "win32"
	case "darwin":
		return "darwin"
	default:
		return "linux"
	}
}

func rowDisabled(row schemaRow, profile string) (bool, bool) {
	if row.DisabledByProfileOS == nil {
		return false, false
	}
	byProfile := row.DisabledByProfileOS[profile]
	if byProfile == nil {
		byProfile = row.DisabledByProfileOS["desktop"]
	}
	if byProfile == nil {
		byProfile = row.DisabledByProfileOS["web"]
	}
	if byProfile == nil {
		return false, false
	}
	value, ok := byProfile[osKey()]
	return value, ok
}

func readProfileBundles(profile string) ([]string, error) {
	dir, err := dshProfileDir(profile)
	if err != nil {
		return nil, err
	}
	path := filepath.Join(dir, "package.json")
	data, err := os.ReadFile(path) // #nosec G304 -- profile manifest
	if err != nil {
		return nil, fmt.Errorf("Failed to read DSH profile manifest %s: %w", path, err)
	}
	var manifest map[string]any
	if err := json.Unmarshal(data, &manifest); err != nil {
		return nil, fmt.Errorf("Invalid DSH profile manifest: %w", err)
	}
	bundles := jsonPointerStrings(manifest, "dsh", "profile", "bundles")
	if len(bundles) == 0 {
		return nil, fmt.Errorf("DSH profile manifest %s does not declare dsh.profile.bundles", path)
	}
	return bundles, nil
}

func jsonPointerStrings(object map[string]any, path ...string) []string {
	current := any(object)
	for _, key := range path {
		mapping, ok := current.(map[string]any)
		if !ok {
			return nil
		}
		current = mapping[key]
	}
	items, ok := current.([]any)
	if !ok {
		return nil
	}
	var result []string
	for _, item := range items {
		if value, ok := item.(string); ok {
			result = append(result, value)
		}
	}
	return result
}

func dshProfileDir(profile string) (string, error) {
	dir, err := core.DshHome()
	if err != nil {
		return "", err
	}
	return filepath.Join(dir, "profiles", profile), nil
}

func composeEntries(profile string, schema *schemaDoc) ([]composedEntry, error) {
	bundles, err := readProfileBundles(profile)
	if err != nil {
		return nil, err
	}
	var entries []composedEntry

	for _, bundle := range bundles {
		schemaBundle, ok := schema.Bundles[bundle]
		if !ok {
			continue
		}
		layer := "bundle:" + bundle
		for _, row := range schemaBundle.Entries {
			disabled, disabledKnown := rowDisabled(row, profile)
			index := -1
			for i := range entries {
				if entries[i].ID == row.ID {
					index = i
					break
				}
			}
			if index >= 0 {
				entry := &entries[index]
				if row.Name != "" {
					entry.PackageName = row.Name
				}
				if row.Config != nil {
					entry.Config = row.Config
				}
				if disabledKnown {
					entry.Disabled = disabled
				}
				entry.Layer = layer
				continue
			}
			entryDisabled := false
			if disabledKnown {
				entryDisabled = disabled
			}
			entries = append(entries, composedEntry{
				ID: row.ID, PackageName: row.Name, Config: row.Config,
				Disabled: entryDisabled, Layer: layer,
			})
		}
	}

	profilePatch, err := dshProfilePatchPath(profile)
	if err != nil {
		return nil, err
	}
	homePatch, err := dshHomePatchPath()
	if err != nil {
		return nil, err
	}
	if err := applyPatchLayer(&entries, profilePatch, "profile", true); err != nil {
		return nil, err
	}
	if err := applyPatchLayer(&entries, homePatch, "home", false); err != nil {
		return nil, err
	}

	for i := range entries {
		entry := &entries[i]
		if meta, ok := schema.Entries[entry.ID]; ok {
			if meta.Package != "" {
				entry.PackageName = meta.Package
			}
			entry.Description = meta.Description
			if meta.Tier == "" {
				entry.Tier = "C"
			} else {
				entry.Tier = meta.Tier
			}
		} else if entry.Tier == "" {
			entry.Tier = "C"
		}
		if entry.PackageName == "" {
			entry.PackageName = entry.ID
		}
	}
	return entries, nil
}

func applyPatchLayer(entries *[]composedEntry, path string, layer string, isProfile bool) error {
	rows, _, err := readPatchDocumentNormalized(path)
	if err != nil {
		return err
	}
	for _, row := range rows {
		applyPatchRow(entries, row, layer, isProfile)
		if insert := core.MappingGet(row, "insert"); core.IsSequence(insert) {
			for _, item := range insert.Content {
				applyPatchRow(entries, item, layer, isProfile)
			}
		}
	}
	return nil
}

func applyPatchRow(entries *[]composedEntry, row *yaml.Node, layer string, isProfile bool) {
	if !core.IsMapping(row) {
		return
	}
	idNode := core.MappingGet(row, "id")
	if idNode == nil || idNode.Kind != yaml.ScalarNode {
		return
	}
	id := idNode.Value

	var config any
	if configNode := core.MappingGet(row, "config"); configNode != nil {
		config = yamlToJSON(configNode)
	}
	var disabled *bool
	if disabledNode := core.MappingGet(row, "disabled"); disabledNode != nil && disabledNode.Kind == yaml.ScalarNode {
		value := disabledNode.Value == "true"
		disabled = &value
	}
	var name string
	if nameNode := core.MappingGet(row, "name"); nameNode != nil && nameNode.Kind == yaml.ScalarNode {
		name = nameNode.Value
	}

	for i := range *entries {
		entry := &(*entries)[i]
		if entry.ID != id {
			continue
		}
		if name != "" {
			entry.PackageName = name
		}
		if config != nil {
			entry.Config = config
			if isProfile {
				entry.ProfileConfig = config
			} else {
				entry.HomeConfig = config
			}
		}
		if disabled != nil {
			entry.Disabled = *disabled
		}
		entry.Layer = layer
		return
	}

	if disabled == nil && config == nil {
		// A row without config/disabled that targets an unknown entry is a
		// no-op for the view.
		return
	}
	packageName := name
	if packageName == "" {
		packageName = id
	}
	var profileConfig, homeConfig any
	if isProfile {
		profileConfig = config
	} else {
		homeConfig = config
	}
	entryDisabled := false
	if disabled != nil {
		entryDisabled = *disabled
	}
	*entries = append(*entries, composedEntry{
		ID: id, PackageName: packageName, Tier: "C", Config: config,
		Disabled: entryDisabled, Layer: layer,
		ProfileConfig: profileConfig, HomeConfig: homeConfig,
	})
}
