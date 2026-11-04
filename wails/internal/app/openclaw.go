package app

// OpenClaw backend: profile CRUD, profile application and live-config reads.
// Port of `droidgear-core::openclaw` (src-tauri/crates/droidgear-core/src/openclaw.rs).
//
// OpenClaw is the tool configured through ~/.openclaw/openclaw.json. DroidGear
// owns only a subset of that document:
//
//	agents.defaults.model.{primary,fallbacks}
//	agents.defaults.blockStreamingDefault / blockStreamingBreak / blockStreamingChunk / blockStreamingCoalesce
//	agents.defaults.models[<refs owned by the profile>]
//	channels.telegram.{blockStreaming,chunkMode}
//	models.providers.<id>.{baseUrl,apiKey,api,models[]}
//
// Everything else — unknown keys, key order, user-added settings — must
// survive a read → modify → write cycle, so openclaw.json is decoded into an
// order-preserving tree (`openclawObject`) instead of a plain map. The Rust
// core gets this from serde_json's preserve_order feature; Go has no ordered
// map in the standard library, so the tree and its marshaler live here.

import (
	"bytes"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"sort"
	"strconv"
	"strings"

	"droidgear-wails/internal/core"
)

// ---------------------------------------------------------------------------
// Types (mirroring droidgear-core::openclaw)
// ---------------------------------------------------------------------------

// OpenClawModel mirrors openclaw::OpenClawModel.
type OpenClawModel struct {
	ID            string   `json:"id"`
	Name          *string  `json:"name,omitempty"`
	Reasoning     bool     `json:"reasoning"`
	Input         []string `json:"input"`
	ContextWindow *uint32  `json:"contextWindow,omitempty"`
	MaxTokens     *uint32  `json:"maxTokens,omitempty"`
}

// OpenClawProviderConfig mirrors openclaw::OpenClawProviderConfig.
type OpenClawProviderConfig struct {
	BaseURL *string         `json:"baseUrl,omitempty"`
	APIKey  *string         `json:"apiKey,omitempty"`
	API     *string         `json:"api,omitempty"`
	Models  []OpenClawModel `json:"models"`
}

// BlockStreamingChunk mirrors openclaw::BlockStreamingChunk.
type BlockStreamingChunk struct {
	MinChars *uint32 `json:"minChars,omitempty"`
	MaxChars *uint32 `json:"maxChars,omitempty"`
}

// BlockStreamingCoalesce mirrors openclaw::BlockStreamingCoalesce.
type BlockStreamingCoalesce struct {
	IdleMs *uint32 `json:"idleMs,omitempty"`
}

// TelegramChannelConfig mirrors openclaw::TelegramChannelConfig.
type TelegramChannelConfig struct {
	BlockStreaming *bool   `json:"blockStreaming,omitempty"`
	ChunkMode      *string `json:"chunkMode,omitempty"`
}

// BlockStreamingConfig mirrors openclaw::BlockStreamingConfig.
type BlockStreamingConfig struct {
	BlockStreamingDefault  *string                 `json:"blockStreamingDefault,omitempty"`
	BlockStreamingBreak    *string                 `json:"blockStreamingBreak,omitempty"`
	BlockStreamingChunk    *BlockStreamingChunk    `json:"blockStreamingChunk,omitempty"`
	BlockStreamingCoalesce *BlockStreamingCoalesce `json:"blockStreamingCoalesce,omitempty"`
	TelegramChannel        *TelegramChannelConfig  `json:"telegramChannel,omitempty"`
}

// OpenClawProfile mirrors openclaw::OpenClawProfile (stored in ~/.droidgear/openclaw/profiles).
type OpenClawProfile struct {
	ID                   string                            `json:"id"`
	Name                 string                            `json:"name"`
	Description          *string                           `json:"description,omitempty"`
	CreatedAt            string                            `json:"createdAt"`
	UpdatedAt            string                            `json:"updatedAt"`
	DefaultModel         *string                           `json:"defaultModel,omitempty"`
	FailoverModels       *[]string                         `json:"failoverModels,omitempty"`
	Providers            map[string]OpenClawProviderConfig `json:"providers"`
	BlockStreamingConfig *BlockStreamingConfig             `json:"blockStreamingConfig,omitempty"`
}

// OpenClawConfigStatus mirrors openclaw::OpenClawConfigStatus.
type OpenClawConfigStatus struct {
	ConfigExists bool   `json:"configExists"`
	ConfigPath   string `json:"configPath"`
}

// OpenClawCurrentConfig mirrors openclaw::OpenClawCurrentConfig.
type OpenClawCurrentConfig struct {
	DefaultModel *string                           `json:"defaultModel,omitempty"`
	Providers    map[string]OpenClawProviderConfig `json:"providers"`
}

// ---------------------------------------------------------------------------
// Order-preserving JSON document
// ---------------------------------------------------------------------------

// openclawObject is a JSON object that keeps insertion order (mirrors
// serde_json's Map with the preserve_order feature). Values are *openclawObject,
// []any, json.Number, string, bool or nil — the same shape
// openclawParseDocument produces.
type openclawObject struct {
	keys   []string
	values map[string]any
}

func newOpenclawObject() *openclawObject {
	return &openclawObject{values: map[string]any{}}
}

// set inserts key or replaces its value while keeping the original position
// (IndexMap semantics, same as serde_json's Map::insert).
func (o *openclawObject) set(key string, value any) {
	if _, ok := o.values[key]; !ok {
		o.keys = append(o.keys, key)
	}
	o.values[key] = value
}

func (o *openclawObject) remove(key string) {
	if _, ok := o.values[key]; !ok {
		return
	}
	delete(o.values, key)
	for i, existing := range o.keys {
		if existing == key {
			o.keys = append(o.keys[:i], o.keys[i+1:]...)
			return
		}
	}
}

func (o *openclawObject) has(key string) bool {
	_, ok := o.values[key]
	return ok
}

func (o *openclawObject) get(key string) any {
	return o.values[key]
}

// snapshotKeys copies the key list so callers can delete while iterating.
func (o *openclawObject) snapshotKeys() []string {
	keys := make([]string, len(o.keys))
	copy(keys, o.keys)
	return keys
}

// entryObject mirrors `entry(key).or_insert_with(Object)` followed by
// `if let Value::Object(...)`: an absent key is created as an empty object,
// an existing non-object value is left untouched and reported as nil.
func (o *openclawObject) entryObject(key string) *openclawObject {
	if existing, ok := o.values[key]; ok {
		object, ok := existing.(*openclawObject)
		if !ok {
			return nil
		}
		return object
	}
	object := newOpenclawObject()
	o.set(key, object)
	return object
}

// entryObjectReplacing is entryObject plus the one place Rust converts a
// non-object in place (`agents.defaults.model` may arrive as a plain string).
func (o *openclawObject) entryObjectReplacing(key string) *openclawObject {
	if existing, ok := o.values[key]; ok {
		if object, ok := existing.(*openclawObject); ok {
			return object
		}
	}
	object := newOpenclawObject()
	o.set(key, object)
	return object
}

// openclawParseDocument decodes a JSON document into the ordered tree,
// keeping object key order and number literals (mirrors
// serde_json::from_str with preserve_order). Upstream OpenClaw allows JSON5
// (comments / trailing commas / single quotes), so the input is normalised
// to strict JSON first — see openclaw_json5.go. Trailing content after the
// root value is rejected like serde_json does.
func openclawParseDocument(data []byte) (any, error) {
	data = openclawNormalizeJSON5(data)
	decoder := json.NewDecoder(bytes.NewReader(data))
	decoder.UseNumber()
	value, err := openclawDecodeValue(decoder)
	if err != nil {
		return nil, err
	}
	if _, err := decoder.Token(); err == nil {
		return nil, errors.New("trailing characters")
	} else if !errors.Is(err, io.EOF) {
		return nil, err
	}
	return value, nil
}

func openclawDecodeValue(decoder *json.Decoder) (any, error) {
	token, err := decoder.Token()
	if err != nil {
		return nil, err
	}
	return openclawDecodeToken(token, decoder)
}

func openclawDecodeToken(token json.Token, decoder *json.Decoder) (any, error) {
	if token == nil { // JSON null
		return nil, nil
	}
	switch value := token.(type) {
	case json.Delim:
		switch value {
		case '{':
			object := newOpenclawObject()
			for decoder.More() {
				keyToken, err := decoder.Token()
				if err != nil {
					return nil, err
				}
				key, ok := keyToken.(string)
				if !ok {
					return nil, fmt.Errorf("expected object key, got %v", keyToken)
				}
				child, err := openclawDecodeValue(decoder)
				if err != nil {
					return nil, err
				}
				object.set(key, child) // duplicate keys: last value wins, first position kept
			}
			if _, err := decoder.Token(); err != nil { // consume '}'
				return nil, err
			}
			return object, nil
		case '[':
			array := []any{}
			for decoder.More() {
				child, err := openclawDecodeValue(decoder)
				if err != nil {
					return nil, err
				}
				array = append(array, child)
			}
			if _, err := decoder.Token(); err != nil { // consume ']'
				return nil, err
			}
			return array, nil
		default:
			return nil, fmt.Errorf("unexpected delimiter %q", value)
		}
	case json.Number, string, bool:
		return value, nil
	}
	return nil, fmt.Errorf("unexpected token %v", token)
}

// MarshalJSON emits the object with keys in insertion order. Strings are
// written without HTML escaping so `&`, `<` and `>` stay byte-identical to
// serde_json output.
func (o *openclawObject) MarshalJSON() ([]byte, error) {
	var buffer bytes.Buffer
	buffer.WriteByte('{')
	for i, key := range o.keys {
		if i > 0 {
			buffer.WriteByte(',')
		}
		if err := openclawWriteJSONString(&buffer, key); err != nil {
			return nil, err
		}
		buffer.WriteByte(':')
		data, err := openclawMarshalValue(o.values[key])
		if err != nil {
			return nil, err
		}
		buffer.Write(data)
	}
	buffer.WriteByte('}')
	return buffer.Bytes(), nil
}

func openclawMarshalValue(value any) ([]byte, error) {
	switch typed := value.(type) {
	case *openclawObject:
		return typed.MarshalJSON()
	case []any:
		var buffer bytes.Buffer
		buffer.WriteByte('[')
		for i, item := range typed {
			if i > 0 {
				buffer.WriteByte(',')
			}
			data, err := openclawMarshalValue(item)
			if err != nil {
				return nil, err
			}
			buffer.Write(data)
		}
		buffer.WriteByte(']')
		return buffer.Bytes(), nil
	case json.Number:
		return []byte(typed.String()), nil
	case string:
		var buffer bytes.Buffer
		if err := openclawWriteJSONString(&buffer, typed); err != nil {
			return nil, err
		}
		return buffer.Bytes(), nil
	case bool:
		if typed {
			return []byte("true"), nil
		}
		return []byte("false"), nil
	case nil:
		return []byte("null"), nil
	default:
		return json.Marshal(typed)
	}
}

func openclawWriteJSONString(buffer *bytes.Buffer, value string) error {
	var encoded bytes.Buffer
	encoder := json.NewEncoder(&encoded)
	encoder.SetEscapeHTML(false)
	if err := encoder.Encode(value); err != nil {
		return err
	}
	buffer.Write(bytes.TrimRight(encoded.Bytes(), "\n"))
	return nil
}

// openclawMarshalPretty renders the tree the way serde_json's
// to_string_pretty does (two-space indent, no trailing newline).
func openclawMarshalPretty(value any) ([]byte, error) {
	compact, err := openclawMarshalValue(value)
	if err != nil {
		return nil, err
	}
	var pretty bytes.Buffer
	if err := json.Indent(&pretty, compact, "", "  "); err != nil {
		return nil, err
	}
	return pretty.Bytes(), nil
}

// ---------------------------------------------------------------------------
// Paths
// ---------------------------------------------------------------------------

func openclawDroidGearDir() (string, error) {
	dir, err := core.DroidGearDir()
	if err != nil {
		return "", err
	}
	return filepath.Join(dir, "openclaw"), nil
}

func openclawProfilesDir() (string, error) {
	dir, err := openclawDroidGearDir()
	if err != nil {
		return "", err
	}
	dir = filepath.Join(dir, "profiles")
	if err := core.EnsureDir(dir); err != nil {
		return "", fmt.Errorf("Failed to create openclaw profiles directory: %w", err)
	}
	return dir, nil
}

func openclawActiveProfilePath() (string, error) {
	dir, err := openclawDroidGearDir()
	if err != nil {
		return "", err
	}
	if err := core.EnsureDir(dir); err != nil {
		return "", fmt.Errorf("Failed to create openclaw directory: %w", err)
	}
	return filepath.Join(dir, "active-profile.txt"), nil
}

// openclawConfigDir resolves the OpenClaw config directory (Preferences →
// Paths override honoured) and creates it, mirroring the Rust core's
// openclaw_config_dir_for_home.
func openclawConfigDir() (string, error) {
	path, _, err := core.EffectivePathForKey("openclaw")
	if err != nil {
		return "", err
	}
	if err := core.EnsureDir(path); err != nil {
		return "", fmt.Errorf("Failed to create openclaw config directory: %w", err)
	}
	return path, nil
}

func openclawConfigPath() (string, error) {
	dir, err := openclawConfigDir()
	if err != nil {
		return "", err
	}
	return filepath.Join(dir, "openclaw.json"), nil
}

func openclawProfilePath(id string) (string, error) {
	if err := core.ValidateProfileID(id); err != nil {
		return "", err
	}
	dir, err := openclawProfilesDir()
	if err != nil {
		return "", err
	}
	return filepath.Join(dir, id+".json"), nil
}

// ---------------------------------------------------------------------------
// Profile files
// ---------------------------------------------------------------------------

// openclawIsProfileFileName mirrors Rust's `path.extension() == Some("json")`:
// the name needs a real extension (a bare ".json" has none) and it is
// compared case-sensitively.
func openclawIsProfileFileName(name string) bool {
	ext := filepath.Ext(name)
	if ext == "" || ext == name {
		return false
	}
	return ext == ".json"
}

// openclawRequireJSONString reproduces serde's required `String` fields: the
// key must be present and hold a string (missing field → serde's exact
// "missing field `x`" wording, which is what `Invalid profile JSON:` wraps).
func openclawRequireJSONString(fields map[string]json.RawMessage, name string) error {
	value, ok := fields[name]
	if !ok {
		return fmt.Errorf("missing field `%s`", name)
	}
	if string(value) == "null" {
		return errors.New("invalid type: null, expected a string")
	}
	var text string
	if err := json.Unmarshal(value, &text); err != nil {
		return err
	}
	return nil
}

// openclawCheckProfileShape validates the parts of a profile document serde
// rejects but encoding/json happily accepts (required top-level strings,
// object-shaped providers, string model ids), so corrupt profile files are
// skipped by List instead of loading as zero values.
func openclawCheckProfileShape(data []byte) error {
	var fields map[string]json.RawMessage
	if err := json.Unmarshal(data, &fields); err != nil {
		return err
	}
	for _, name := range []string{"id", "name", "createdAt", "updatedAt"} {
		if err := openclawRequireJSONString(fields, name); err != nil {
			return err
		}
	}
	rawProviders, ok := fields["providers"]
	if !ok || string(rawProviders) == "null" {
		return nil
	}
	var providers map[string]json.RawMessage
	if err := json.Unmarshal(rawProviders, &providers); err != nil {
		return fmt.Errorf("invalid type: expected a map, got %s", string(rawProviders))
	}
	for _, rawProvider := range providers {
		var provider map[string]json.RawMessage
		if err := json.Unmarshal(rawProvider, &provider); err != nil {
			continue // serde also skips non-object providers while parsing models
		}
		rawModels, ok := provider["models"]
		if !ok || string(rawModels) == "null" {
			continue
		}
		var models []json.RawMessage
		if err := json.Unmarshal(rawModels, &models); err != nil {
			return fmt.Errorf("invalid type: expected a sequence, got %s", string(rawModels))
		}
		for _, rawModel := range models {
			var model map[string]json.RawMessage
			if err := json.Unmarshal(rawModel, &model); err != nil {
				return fmt.Errorf("invalid type: expected a map, got %s", string(rawModel))
			}
			if err := openclawRequireJSONString(model, "id"); err != nil {
				return err
			}
		}
	}
	return nil
}

// openclawNormalizeProfile applies the serde defaults Go's decoder leaves as
// nil: `providers`, `models` and `input` are plain Rust collections that
// always serialize as `{}` / `[]`, never null.
func openclawNormalizeProfile(profile *OpenClawProfile) {
	if profile.Providers == nil {
		profile.Providers = map[string]OpenClawProviderConfig{}
	}
	for id, provider := range profile.Providers {
		if provider.Models == nil {
			provider.Models = []OpenClawModel{}
		}
		for i := range provider.Models {
			if provider.Models[i].Input == nil {
				provider.Models[i].Input = []string{}
			}
		}
		profile.Providers[id] = provider
	}
}

func openclawReadProfileFile(path string) (OpenClawProfile, error) {
	data, err := os.ReadFile(path) // #nosec G304 -- profile path
	if err != nil {
		return OpenClawProfile{}, fmt.Errorf("Failed to read profile: %w", err)
	}
	if err := openclawCheckProfileShape(data); err != nil {
		return OpenClawProfile{}, fmt.Errorf("Invalid profile JSON: %w", err)
	}
	var profile OpenClawProfile
	if err := json.Unmarshal(data, &profile); err != nil {
		return OpenClawProfile{}, fmt.Errorf("Invalid profile JSON: %w", err)
	}
	openclawNormalizeProfile(&profile)
	return profile, nil
}

func openclawWriteProfileFile(profile OpenClawProfile) error {
	path, err := openclawProfilePath(profile.ID)
	if err != nil {
		return err
	}
	openclawNormalizeProfile(&profile)
	data, err := json.MarshalIndent(profile, "", "  ")
	if err != nil {
		return fmt.Errorf("Failed to serialize profile JSON: %w", err)
	}
	return core.AtomicWrite(path, data)
}

// ---------------------------------------------------------------------------
// openclaw.json read / parse / write
// ---------------------------------------------------------------------------

// openclawReadConfigRaw loads ~/.openclaw/openclaw.json as the ordered tree.
// A missing file (or one that cannot be stat'ed) yields an empty object —
// exactly what Rust's `!config_path.exists()` path does.
func openclawReadConfigRaw() (any, error) {
	path, err := openclawConfigPath()
	if err != nil {
		return nil, err
	}
	if _, err := os.Stat(path); err != nil {
		return newOpenclawObject(), nil
	}
	data, err := os.ReadFile(path) // #nosec G304 -- openclaw config path
	if err != nil {
		return nil, fmt.Errorf("Failed to read config file: %w", err)
	}
	document, err := openclawParseDocument(data)
	if err != nil {
		return nil, fmt.Errorf("Invalid config JSON: %w", err)
	}
	return document, nil
}

// openclawParseConfig extracts the DroidGear-owned fields of an openclaw.json
// document (mirrors Rust parse_openclaw_config): default model, failover
// models and the provider map.
func openclawParseConfig(document any) (*string, *[]string, map[string]OpenClawProviderConfig) {
	var defaultModel *string
	var failoverModels *[]string
	providers := map[string]OpenClawProviderConfig{}

	root, ok := document.(*openclawObject)
	if !ok {
		return nil, nil, providers
	}

	if agents, ok := root.get("agents").(*openclawObject); ok {
		if defaults, ok := agents.get("defaults").(*openclawObject); ok {
			if model, ok := defaults.get("model").(*openclawObject); ok {
				if primary, ok := model.get("primary").(string); ok {
					defaultModel = &primary
				}
				if fallbacks, ok := model.get("fallbacks").([]any); ok {
					list := []string{}
					for _, item := range fallbacks {
						if text, ok := item.(string); ok {
							list = append(list, text)
						}
					}
					if len(list) > 0 {
						failoverModels = &list
					}
				}
			}
		}
	}

	if models, ok := root.get("models").(*openclawObject); ok {
		if providersObject, ok := models.get("providers").(*openclawObject); ok {
			for _, id := range providersObject.keys {
				providerObject, ok := providersObject.get(id).(*openclawObject)
				if !ok {
					continue // non-object provider entries are ignored
				}
				provider := OpenClawProviderConfig{
					BaseURL: openclawString(providerObject.get("baseUrl")),
					APIKey:  openclawString(providerObject.get("apiKey")),
					API:     openclawString(providerObject.get("api")),
					Models:  []OpenClawModel{},
				}
				if modelList, ok := providerObject.get("models").([]any); ok {
					for _, modelValue := range modelList {
						modelObject, ok := modelValue.(*openclawObject)
						if !ok {
							continue
						}
						model := OpenClawModel{
							Name:          openclawString(modelObject.get("name")),
							Input:         []string{},
							ContextWindow: openclawUint32(modelObject.get("contextWindow")),
							MaxTokens:     openclawUint32(modelObject.get("maxTokens")),
						}
						if id, ok := modelObject.get("id").(string); ok {
							model.ID = id
						}
						if reasoning, ok := modelObject.get("reasoning").(bool); ok {
							model.Reasoning = reasoning
						}
						if input, ok := modelObject.get("input").([]any); ok {
							for _, item := range input {
								if text, ok := item.(string); ok {
									model.Input = append(model.Input, text)
								}
							}
						}
						provider.Models = append(provider.Models, model)
					}
				}
				providers[id] = provider
			}
		}
	}

	return defaultModel, failoverModels, providers
}

// openclawString mirrors Value::as_str for optional string fields.
func openclawString(value any) *string {
	text, ok := value.(string)
	if !ok {
		return nil
	}
	return &text
}

// openclawUint32 mirrors Value::as_u64() → u32: only integral number literals
// that fit the range are accepted (1.5, -1 and 1e6 all yield nil).
func openclawUint32(value any) *uint32 {
	number, ok := value.(json.Number)
	if !ok {
		return nil
	}
	parsed, err := strconv.ParseUint(number.String(), 10, 32)
	if err != nil {
		return nil
	}
	result := uint32(parsed)
	return &result
}

func openclawNumber(value uint32) json.Number {
	return json.Number(strconv.FormatUint(uint64(value), 10))
}

// openclawWriteConfig applies profile onto the existing openclaw.json and
// writes it back atomically (mirrors write_openclaw_config_for_home).
func openclawWriteConfig(profile *OpenClawProfile) error {
	path, err := openclawConfigPath()
	if err != nil {
		return err
	}
	document, err := openclawReadConfigRaw()
	if err != nil {
		return err
	}
	config, ok := document.(*openclawObject)
	if !ok {
		config = newOpenclawObject()
	}
	openclawApplyProfileInPlace(config, profile)
	data, err := openclawMarshalPretty(config)
	if err != nil {
		return fmt.Errorf("Failed to serialize config: %w", err)
	}
	return core.AtomicWrite(path, data)
}

// ---------------------------------------------------------------------------
// Profile → config merge (mirrors Rust apply_profile_in_place)
// ---------------------------------------------------------------------------

// openclawApplyProfileInPlace writes every DroidGear-owned field of profile
// into config. Unknown top-level keys, sibling keys inside partially owned
// objects and existing key order are left untouched; empty blocks DroidGear
// created but did not populate are removed at the end.
func openclawApplyProfileInPlace(config *openclawObject, profile *OpenClawProfile) {
	owned := make(map[string]bool, len(profile.Providers))
	for id := range profile.Providers {
		owned[id] = true
	}

	needsAgentsBlock := profile.DefaultModel != nil ||
		profile.FailoverModels != nil ||
		profile.BlockStreamingConfig != nil ||
		openclawProfileHasModels(profile) ||
		openclawDefaultsModelsHasOwnedRef(config, owned)

	if needsAgentsBlock {
		if agents := config.entryObject("agents"); agents != nil {
			if defaults := agents.entryObject("defaults"); defaults != nil {
				openclawApplyDefaultModel(defaults, profile)
				openclawApplyBlockStreaming(defaults, profile)
				openclawApplyDefaultsModelsRefs(defaults, profile, owned)
			}
		}
	}

	if streaming := profile.BlockStreamingConfig; streaming != nil && streaming.TelegramChannel != nil {
		if channels := config.entryObject("channels"); channels != nil {
			// channels.telegram is only ever inserted into, never dropped.
			if telegram := channels.entryObject("telegram"); telegram != nil {
				if streaming.TelegramChannel.BlockStreaming != nil {
					telegram.set("blockStreaming", *streaming.TelegramChannel.BlockStreaming)
				}
				if streaming.TelegramChannel.ChunkMode != nil {
					telegram.set("chunkMode", *streaming.TelegramChannel.ChunkMode)
				}
			}
		}
	}

	openclawApplyModelsProviders(config, profile, owned)
	openclawCleanupEmptyDGBlocks(config)
}

func openclawProfileHasModels(profile *OpenClawProfile) bool {
	for _, provider := range profile.Providers {
		if len(provider.Models) > 0 {
			return true
		}
	}
	return false
}

// openclawDefaultsModelsHasOwnedRef reports whether agents.defaults.models
// already references a provider the profile owns (so stale refs may need
// pruning even when the profile currently carries no models).
func openclawDefaultsModelsHasOwnedRef(config *openclawObject, owned map[string]bool) bool {
	agents, ok := config.get("agents").(*openclawObject)
	if !ok {
		return false
	}
	defaults, ok := agents.get("defaults").(*openclawObject)
	if !ok {
		return false
	}
	models, ok := defaults.get("models").(*openclawObject)
	if !ok {
		return false
	}
	for _, key := range models.keys {
		if owned[strings.Split(key, "/")[0]] {
			return true
		}
	}
	return false
}

// openclawApplyDefaultModel updates agents.defaults.model; only `primary` and
// `fallbacks` are owned, unknown siblings (routing metadata, weights, …)
// survive.
func openclawApplyDefaultModel(defaults *openclawObject, profile *OpenClawProfile) {
	if profile.DefaultModel == nil && profile.FailoverModels == nil {
		return
	}
	model := defaults.entryObjectReplacing("model")
	if profile.DefaultModel != nil {
		model.set("primary", *profile.DefaultModel)
	}
	switch {
	case profile.FailoverModels == nil:
		// Unset → leave whatever is already there alone.
	case len(*profile.FailoverModels) > 0:
		fallbacks := make([]any, 0, len(*profile.FailoverModels))
		for _, item := range *profile.FailoverModels {
			fallbacks = append(fallbacks, item)
		}
		model.set("fallbacks", fallbacks)
	default:
		// Empty list → the user explicitly cleared fallbacks.
		model.remove("fallbacks")
	}
}

// openclawApplyBlockStreaming updates the agents.defaults.blockStreaming* keys.
func openclawApplyBlockStreaming(defaults *openclawObject, profile *OpenClawProfile) {
	streaming := profile.BlockStreamingConfig
	if streaming == nil {
		return
	}
	if streaming.BlockStreamingDefault != nil {
		defaults.set("blockStreamingDefault", *streaming.BlockStreamingDefault)
	}
	if streaming.BlockStreamingBreak != nil {
		defaults.set("blockStreamingBreak", *streaming.BlockStreamingBreak)
	}
	if streaming.BlockStreamingChunk != nil {
		if chunk := defaults.entryObject("blockStreamingChunk"); chunk != nil {
			if streaming.BlockStreamingChunk.MinChars != nil {
				chunk.set("minChars", openclawNumber(*streaming.BlockStreamingChunk.MinChars))
			}
			if streaming.BlockStreamingChunk.MaxChars != nil {
				chunk.set("maxChars", openclawNumber(*streaming.BlockStreamingChunk.MaxChars))
			}
		}
	}
	if streaming.BlockStreamingCoalesce != nil && streaming.BlockStreamingCoalesce.IdleMs != nil {
		if coalesce := defaults.entryObject("blockStreamingCoalesce"); coalesce != nil {
			coalesce.set("idleMs", openclawNumber(*streaming.BlockStreamingCoalesce.IdleMs))
		}
	}
}

// openclawApplyDefaultsModelsRefs reconciles agents.defaults.models with the
// refs implied by profile.providers[*].models[]. Only refs whose provider
// prefix is owned are added/removed; other providers' refs and the contents
// of surviving entries (alias, params, streaming, …) are preserved.
func openclawApplyDefaultsModelsRefs(defaults *openclawObject, profile *OpenClawProfile, owned map[string]bool) {
	targetRefs := []string{}
	targetSet := map[string]bool{}
	for _, id := range openclawSortedProviderIDs(profile) {
		provider := profile.Providers[id]
		for _, model := range provider.Models {
			ref := id + "/" + model.ID
			if targetSet[ref] {
				continue
			}
			targetSet[ref] = true
			targetRefs = append(targetRefs, ref)
		}
	}
	if len(targetRefs) == 0 && !defaults.has("models") {
		return
	}
	models := defaults.entryObject("models")
	if models == nil {
		return
	}
	for _, key := range models.snapshotKeys() {
		if owned[strings.Split(key, "/")[0]] && !targetSet[key] {
			models.remove(key)
		}
	}
	for _, ref := range targetRefs {
		if !models.has(ref) {
			models.set(ref, newOpenclawObject())
		}
	}
}

// openclawApplyModelsProviders reconciles models.providers with the profile:
// providers missing from the profile are dropped (treated as user-deleted),
// owned providers get only their typed fields written.
func openclawApplyModelsProviders(config *openclawObject, profile *OpenClawProfile, owned map[string]bool) {
	hasExistingProviders := false
	if models, ok := config.get("models").(*openclawObject); ok {
		hasExistingProviders = models.has("providers")
	}
	if len(profile.Providers) == 0 && !hasExistingProviders {
		return
	}
	models := config.entryObject("models")
	if models == nil {
		return
	}
	// Default `mode` to "merge" if absent (matches the OpenClaw default).
	if !models.has("mode") {
		models.set("mode", "merge")
	}
	providers := models.entryObject("providers")
	if providers == nil {
		return
	}
	for _, id := range providers.snapshotKeys() {
		if !owned[id] {
			providers.remove(id)
		}
	}
	// Sorted iteration keeps the output deterministic (Rust walks a HashMap,
	// so its insertion order for new providers is arbitrary).
	for _, id := range openclawSortedProviderIDs(profile) {
		provider := profile.Providers[id]
		if target := providers.entryObject(id); target != nil {
			openclawUpdateProviderInPlace(target, &provider)
		}
	}
}

// openclawSortedProviderIDs returns the profile's provider ids in a stable
// order.
func openclawSortedProviderIDs(profile *OpenClawProfile) []string {
	ids := make([]string, 0, len(profile.Providers))
	for id := range profile.Providers {
		ids = append(ids, id)
	}
	sort.Strings(ids)
	return ids
}

// openclawUpdateProviderInPlace writes a provider's typed fields, merging
// models[] entry by id so unknown per-model keys survive.
func openclawUpdateProviderInPlace(providerObject *openclawObject, provider *OpenClawProviderConfig) {
	openclawSetOrRemoveString(providerObject, "baseUrl", provider.BaseURL)
	openclawSetOrRemoveString(providerObject, "apiKey", provider.APIKey)
	openclawSetOrRemoveString(providerObject, "api", provider.API)

	existingModels, _ := providerObject.get("models").([]any)
	existingByID := map[string]any{}
	for _, entry := range existingModels {
		object, ok := entry.(*openclawObject)
		if !ok {
			continue
		}
		id, ok := object.get("id").(string)
		if !ok {
			continue
		}
		existingByID[id] = entry // last write wins, like OpenClaw itself
	}

	newModels := make([]any, 0, len(provider.Models))
	for _, model := range provider.Models {
		entry, ok := existingByID[model.ID]
		if ok {
			delete(existingByID, model.ID)
		} else {
			entry = newOpenclawObject()
		}
		object, ok := entry.(*openclawObject)
		if !ok {
			object = newOpenclawObject()
			entry = object
		}
		object.set("id", model.ID)
		name := model.ID
		if model.Name != nil {
			name = *model.Name
		}
		object.set("name", name)
		object.set("reasoning", model.Reasoning)
		if len(model.Input) == 0 {
			object.remove("input")
		} else {
			input := make([]any, 0, len(model.Input))
			for _, item := range model.Input {
				input = append(input, item)
			}
			object.set("input", input)
		}
		if model.ContextWindow != nil {
			object.set("contextWindow", openclawNumber(*model.ContextWindow))
		} else {
			object.remove("contextWindow")
		}
		if model.MaxTokens != nil {
			object.set("maxTokens", openclawNumber(*model.MaxTokens))
		} else {
			object.remove("maxTokens")
		}
		newModels = append(newModels, entry)
	}

	if len(newModels) == 0 {
		providerObject.remove("models")
	} else {
		providerObject.set("models", newModels)
	}
}

func openclawSetOrRemoveString(object *openclawObject, key string, value *string) {
	if value == nil {
		object.remove(key)
		return
	}
	object.set(key, *value)
}

// openclawCleanupEmptyDGBlocks drops top-level blocks DroidGear created but
// did not populate; existing user blocks are left alone.
func openclawCleanupEmptyDGBlocks(config *openclawObject) {
	if agents, ok := config.get("agents").(*openclawObject); ok {
		if defaults, ok := agents.get("defaults").(*openclawObject); ok {
			for _, key := range []string{"models", "blockStreamingChunk", "blockStreamingCoalesce", "model"} {
				if value, ok := defaults.get(key).(*openclawObject); ok && len(value.keys) == 0 {
					defaults.remove(key)
				}
			}
		}
		if defaults, ok := agents.get("defaults").(*openclawObject); ok && len(defaults.keys) == 0 {
			agents.remove("defaults")
		}
	}
	if agents, ok := config.get("agents").(*openclawObject); ok && len(agents.keys) == 0 {
		config.remove("agents")
	}
}

// ---------------------------------------------------------------------------
// Commands — profile CRUD
// ---------------------------------------------------------------------------

// ListOpenclawProfiles lists all OpenClaw profiles sorted by name.
func (a *App) ListOpenclawProfiles() ([]OpenClawProfile, error) {
	dir, err := openclawProfilesDir()
	if err != nil {
		return nil, err
	}
	entries, err := os.ReadDir(dir)
	if err != nil {
		return nil, fmt.Errorf("Failed to read profiles dir: %w", err)
	}
	profiles := []OpenClawProfile{}
	for _, entry := range entries {
		name := entry.Name()
		if !openclawIsProfileFileName(name) {
			continue
		}
		if profile, err := openclawReadProfileFile(filepath.Join(dir, name)); err == nil {
			profiles = append(profiles, profile)
		}
	}
	sort.SliceStable(profiles, func(i, j int) bool {
		return strings.ToLower(profiles[i].Name) < strings.ToLower(profiles[j].Name)
	})
	return profiles, nil
}

// SaveOpenclawProfile creates or updates a profile.
func (a *App) SaveOpenclawProfile(profile OpenClawProfile) error {
	if strings.TrimSpace(profile.ID) == "" {
		profile.ID = core.NewUUID()
		profile.CreatedAt = core.NowRFC3339()
	} else {
		path, err := openclawProfilePath(profile.ID)
		if err != nil {
			return err
		}
		if _, statErr := os.Stat(path); statErr == nil {
			if existing, readErr := openclawReadProfileFile(path); readErr == nil {
				profile.CreatedAt = existing.CreatedAt
			}
		} else if strings.TrimSpace(profile.CreatedAt) == "" {
			profile.CreatedAt = core.NowRFC3339()
		}
	}
	profile.UpdatedAt = core.NowRFC3339()
	return openclawWriteProfileFile(profile)
}

// DeleteOpenclawProfile removes a profile and clears the active pointer when
// it pointed at the deleted profile.
func (a *App) DeleteOpenclawProfile(id string) error {
	path, err := openclawProfilePath(id)
	if err != nil {
		return err
	}
	if _, statErr := os.Stat(path); statErr == nil {
		if err := os.Remove(path); err != nil {
			return fmt.Errorf("Failed to delete profile: %w", err)
		}
	}
	if active, err := a.GetActiveOpenclawProfileId(); err == nil && active != nil && *active == id {
		if activePath, err := openclawActiveProfilePath(); err == nil {
			_ = os.Remove(activePath)
		}
	}
	return nil
}

// DuplicateOpenclawProfile copies a profile under a new id and name.
func (a *App) DuplicateOpenclawProfile(id string, newName string) (OpenClawProfile, error) {
	path, err := openclawProfilePath(id)
	if err != nil {
		return OpenClawProfile{}, err
	}
	profile, err := openclawReadProfileFile(path)
	if err != nil {
		return OpenClawProfile{}, err
	}
	profile.ID = core.NewUUID()
	profile.Name = newName
	profile.CreatedAt = core.NowRFC3339()
	profile.UpdatedAt = profile.CreatedAt
	if err := openclawWriteProfileFile(profile); err != nil {
		return OpenClawProfile{}, err
	}
	return profile, nil
}

// CreateDefaultOpenclawProfile seeds the first profile. When openclaw.json
// already exists the profile is initialised from its current content,
// otherwise from the documented fresh-install defaults.
func (a *App) CreateDefaultOpenclawProfile() (OpenClawProfile, error) {
	configPath, err := openclawConfigPath()
	if err != nil {
		return OpenClawProfile{}, err
	}
	var defaultModel *string
	var failoverModels *[]string
	providers := map[string]OpenClawProviderConfig{}
	if _, statErr := os.Stat(configPath); statErr == nil {
		data, err := os.ReadFile(configPath) // #nosec G304 -- openclaw config path
		if err != nil {
			return OpenClawProfile{}, fmt.Errorf("Failed to read config file: %w", err)
		}
		document, err := openclawParseDocument(data)
		if err != nil {
			return OpenClawProfile{}, fmt.Errorf("Invalid config JSON: %w", err)
		}
		defaultModel, failoverModels, providers = openclawParseConfig(document)
	} else {
		fresh := "anthropic/claude-sonnet-4-20250514"
		defaultModel = &fresh
	}

	now := core.NowRFC3339()
	profile := OpenClawProfile{
		ID:             core.NewUUID(),
		Name:           "Default",
		Description:    core.StrPtr("Default OpenClaw profile"),
		CreatedAt:      now,
		UpdatedAt:      now,
		DefaultModel:   defaultModel,
		FailoverModels: failoverModels,
		Providers:      providers,
	}
	if err := openclawWriteProfileFile(profile); err != nil {
		return OpenClawProfile{}, err
	}
	return profile, nil
}

// GetActiveOpenclawProfileId returns the active profile id (null when unset).
func (a *App) GetActiveOpenclawProfileId() (*string, error) {
	path, err := openclawActiveProfilePath()
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

// ---------------------------------------------------------------------------
// Commands — apply / status / current config
// ---------------------------------------------------------------------------

// ApplyOpenclawProfile merges the profile into ~/.openclaw/openclaw.json and
// records it as the active profile.
func (a *App) ApplyOpenclawProfile(profile OpenClawProfile) error {
	if err := openclawWriteConfig(&profile); err != nil {
		return err
	}
	path, err := openclawActiveProfilePath()
	if err != nil {
		return err
	}
	return core.AtomicWrite(path, []byte(profile.ID))
}

// GetOpenclawConfigStatus reports whether openclaw.json exists and where it lives.
func (a *App) GetOpenclawConfigStatus() (OpenClawConfigStatus, error) {
	path, err := openclawConfigPath()
	if err != nil {
		return OpenClawConfigStatus{}, err
	}
	_, statErr := os.Stat(path)
	return OpenClawConfigStatus{
		ConfigExists: statErr == nil,
		ConfigPath:   path,
	}, nil
}

// ReadOpenclawCurrentConfig reads the live OpenClaw configuration.
func (a *App) ReadOpenclawCurrentConfig() (OpenClawCurrentConfig, error) {
	path, err := openclawConfigPath()
	if err != nil {
		return OpenClawCurrentConfig{}, err
	}
	if _, statErr := os.Stat(path); statErr != nil {
		return OpenClawCurrentConfig{Providers: map[string]OpenClawProviderConfig{}}, nil
	}
	data, err := os.ReadFile(path) // #nosec G304 -- openclaw config path
	if err != nil {
		return OpenClawCurrentConfig{}, fmt.Errorf("Failed to read config file: %w", err)
	}
	document, err := openclawParseDocument(data)
	if err != nil {
		return OpenClawCurrentConfig{}, fmt.Errorf("Invalid config JSON: %w", err)
	}
	defaultModel, _, providers := openclawParseConfig(document)
	return OpenClawCurrentConfig{DefaultModel: defaultModel, Providers: providers}, nil
}
