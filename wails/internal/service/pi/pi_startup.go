package pi

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"sort"
	"strings"
)

// ---------------------------------------------------------------------------
// Keeping the startup model resolvable across profile applies
// ---------------------------------------------------------------------------
//
// `ApplyProfile` replaces the providers models.json supplies, but the startup
// model lives in settings.json (see pi_settings.go) and pi resolves it against
// models.json **plus** its own catalog. Applying a profile that no longer
// carries the provider — or renames a model — therefore leaves settings.json
// pointing at something pi cannot start, and pi silently falls back to another
// model.
//
// ReconcileStartupModel is the one place that repairs that selection. It is
// deliberately conservative: it only rewrites a value that this apply made
// unresolvable. A selection served by pi's catalog (`~/.pi/agent/models-store.json`)
// is never touched, because a profile says nothing about catalog providers, and
// a provider that survives the apply keeps its model unless the model itself is
// gone.

// PiApplyResult reports what one apply wrote: the providers now in models.json
// and the startup selection that came out of the reconcile pass.
type PiApplyResult struct {
	ID               string     `json:"id"`
	AppliedProviders []string   `json:"appliedProviders"`
	Startup          PiSettings `json:"startup"`
	StartupAdjusted  bool       `json:"startupAdjusted"`
	StartupReasons   []string   `json:"startupReasons,omitempty"`
}

// startupCatalogModel is what the catalog says about one model: its thinking
// level map is the only field the repair pass needs (a catalog model must not
// keep a level it declares unsupported).
type startupCatalogModel struct {
	thinkingLevelMap map[string]*string
}

// startupCatalog is what pi's downloaded catalog knows: provider id → model id.
// An entry still means "pi knows this provider", which is what keeps
// catalog-only selections out of the provider repair path.
type startupCatalog map[string]map[string]startupCatalogModel

func (c startupCatalog) knowsProvider(id string) bool {
	_, ok := c[id]
	return ok
}

func (c startupCatalog) knowsModel(providerID string, modelID string) bool {
	_, ok := c[providerID][modelID]
	return ok
}

// thinkingLevelMap returns the model's declared map when the catalog knows it.
func (c startupCatalog) thinkingLevelMap(providerID string, modelID string) (map[string]*string, bool) {
	entry, ok := c[providerID][modelID]
	if !ok || len(entry.thinkingLevelMap) == 0 {
		return nil, false
	}
	return entry.thinkingLevelMap, true
}

// readStartupCatalog parses models-store.json (pi's cached catalog). A missing
// or malformed file yields an empty catalog: the repair pass then only trusts
// models.json, which keeps the historical behaviour for fresh installs.
func readStartupCatalog() startupCatalog {
	dir, err := piConfigDir()
	if err != nil {
		return startupCatalog{}
	}
	path := filepath.Join(dir, "models-store.json")
	data, err := os.ReadFile(path) // #nosec G304 -- pi agent directory
	if err != nil {
		return startupCatalog{}
	}
	var store map[string]struct {
		Models []struct {
			ID               string             `json:"id"`
			ThinkingLevelMap map[string]*string `json:"thinkingLevelMap"`
		} `json:"models"`
	}
	if err := json.Unmarshal(data, &store); err != nil {
		return startupCatalog{}
	}
	catalog := make(startupCatalog, len(store))
	for providerID, entry := range store {
		models := make(map[string]startupCatalogModel, len(entry.Models))
		for _, model := range entry.Models {
			if model.ID != "" {
				models[model.ID] = startupCatalogModel{thinkingLevelMap: model.ThinkingLevelMap}
			}
		}
		catalog[providerID] = models
	}
	return catalog
}

// providersFromDocument parses the providers object of a models.json document
// (empty when the document has none).
func providersFromDocument(data []byte) (map[string]PiProviderConfig, error) {
	providers := map[string]PiProviderConfig{}
	if len(strings.TrimSpace(string(data))) == 0 {
		return providers, nil
	}
	var root struct {
		Providers map[string]PiProviderConfig `json:"providers"`
	}
	if err := json.Unmarshal(data, &root); err != nil {
		return nil, fmt.Errorf("Invalid Pi providers object: %w", err)
	}
	if root.Providers != nil {
		providers = root.Providers
	}
	return providers, nil
}

// ReconcileStartupModel repairs settings.json after models.json changed from
// `before` to `after`. It returns the resulting selection (with the reasons a
// value was replaced) and only writes when something actually changed.
func ReconcileStartupModel(before map[string]PiProviderConfig, after map[string]PiProviderConfig) (PiApplyResult, error) {
	current, err := ReadSettings()
	if err != nil {
		return PiApplyResult{}, err
	}
	next := current.PiSettings
	catalog := readStartupCatalog()
	reasons := []string{}

	provider := optionalValue(next.DefaultProvider)
	model := optionalValue(next.DefaultModel)
	modelReplaced := false

	// 1. The provider itself disappeared. models.json no longer supplies it and
	//    pi's catalog never did, so pi has no models under that id at all.
	if provider != "" {
		_, suppliedBefore := before[provider]
		_, suppliedAfter := after[provider]
		if suppliedBefore && !suppliedAfter && !catalog.knowsProvider(provider) {
			if replacement := firstProviderID(after); replacement != "" {
				replacedModel := firstModelID(after[replacement])
				reasons = append(reasons, fmt.Sprintf(
					"provider %q is no longer supplied by models.json; using %q instead",
					provider, replacement,
				))
				provider = replacement
				next.DefaultProvider = &replacement
				model = replacedModel
				next.DefaultModel = optionalValuePtr(replacedModel)
				modelReplaced = true
			} else {
				reasons = append(reasons, fmt.Sprintf(
					"provider %q is no longer supplied by models.json and the profile has no providers; clearing the startup model",
					provider,
				))
				provider = ""
				model = ""
				next.DefaultProvider = nil
				next.DefaultModel = nil
				modelReplaced = true
			}
		}
	}

	// 2. The provider survived but the model id did not (renamed or dropped).
	if provider != "" && model != "" {
		if config, supplied := after[provider]; supplied {
			if !catalog.knowsModel(provider, model) && !providerHasModel(config, model) {
				if replacement := firstModelID(config); replacement != "" {
					reasons = append(reasons, fmt.Sprintf(
						"model %q is not in %q anymore; using %q instead",
						model, provider, replacement,
					))
					model = replacement
					next.DefaultModel = &replacement
					modelReplaced = true
				} else {
					reasons = append(reasons, fmt.Sprintf(
						"model %q is not in %q anymore and the provider lists no models; clearing the startup model",
						model, provider,
					))
					model = ""
					next.DefaultModel = nil
					modelReplaced = true
				}
			}
		}
	}

	// 3. The thinking level belongs to the model it was chosen for: drop a
	//    level the (possibly replaced) model cannot express, and never persist
	//    an unknown level (pi ignores it, so keeping it would only hide the
	//    mistake).
	if level := optionalValue(next.DefaultThinkingLevel); level != "" {
		cleared := ""
		switch {
		case !isThinkingLevel(level):
			cleared = fmt.Sprintf("thinking level %q is not one of %s; clearing it", level, strings.Join(ThinkingLevels, " | "))
		case provider != "" && model != "":
			if mapped, ok := thinkingLevelsFor(after, catalog, provider, model); ok && !thinkingLevelAllowed(mapped, level) {
				cleared = fmt.Sprintf("thinking level %q is not supported by %q; clearing it", level, model)
			} else if modelReplaced && !ok {
				// A different model is the startup model now: the level survives
				// only when the new model declares it.
				cleared = fmt.Sprintf("thinking level %q is not declared by %q; clearing it", level, model)
			}
		}
		if cleared != "" {
			reasons = append(reasons, cleared)
			next.DefaultThinkingLevel = nil
		}
	}

	result := PiApplyResult{
		Startup:         next,
		StartupAdjusted: len(reasons) > 0,
		StartupReasons:  reasons,
	}
	if !result.StartupAdjusted {
		return result, nil
	}
	if err := SaveSettings(next); err != nil {
		return result, err
	}
	return result, nil
}

// StartupOptions lists what the startup model fields can point at, so the GUI,
// `dg pi settings candidates` and the TUI Defaults screen offer the same
// choices instead of asking users to type ids from memory.
type StartupOptions struct {
	Providers []string            `json:"providers"`
	Models    map[string][]string `json:"models"`
}

// ListStartupOptions merges the providers models.json supplies with the cached
// catalog (models.json wins, its provider order is alphabetical like pi's
// first-provider fallback).
func ListStartupOptions() (StartupOptions, error) {
	options := StartupOptions{Models: map[string][]string{}}
	config, err := ReadCurrentConfig()
	if err != nil {
		return options, err
	}
	catalog := readStartupCatalog()
	for providerID, entry := range config.Providers {
		options.Providers = append(options.Providers, providerID)
		options.Models[providerID] = modelIDs(entry)
	}
	for providerID := range catalog {
		if _, seen := options.Models[providerID]; seen {
			continue
		}
		options.Providers = append(options.Providers, providerID)
		ids := make([]string, 0, len(catalog[providerID]))
		for modelID := range catalog[providerID] {
			ids = append(ids, modelID)
		}
		sort.Strings(ids)
		options.Models[providerID] = ids
	}
	sort.Strings(options.Providers)
	return options, nil
}

// StartupWarnings reports selections that pi will not be able to resolve. It
// is advisory (free text stays allowed) and shared by `dg pi settings set`,
// which prints the notes next to the receipt.
func StartupWarnings(settings PiSettings) ([]string, error) {
	options, err := ListStartupOptions()
	if err != nil {
		return nil, err
	}
	known := make(map[string]struct{}, len(options.Providers))
	for _, providerID := range options.Providers {
		known[providerID] = struct{}{}
	}
	warnings := []string{}
	provider := optionalValue(settings.DefaultProvider)
	model := optionalValue(settings.DefaultModel)
	if provider == "" {
		return warnings, nil
	}
	if _, ok := known[provider]; !ok {
		return append(warnings, fmt.Sprintf(
			"provider %q is neither in models.json nor in pi's catalog; pi will ignore the startup model",
			provider,
		)), nil
	}
	if model == "" {
		return warnings, nil
	}
	mapped := false
	for _, knownModel := range options.Models[provider] {
		if knownModel == model {
			mapped = true
			break
		}
	}
	if !mapped {
		return append(warnings, fmt.Sprintf(
			"model %q is not listed for provider %q; pi will ignore the startup model",
			model, provider,
		)), nil
	}
	level := optionalValue(settings.DefaultThinkingLevel)
	if level == "" {
		return warnings, nil
	}
	catalog := readStartupCatalog()
	if levels, ok := catalog.thinkingLevelMap(provider, model); ok && !thinkingLevelAllowed(levels, level) {
		warnings = append(warnings, fmt.Sprintf(
			"thinking level %q is not supported by %q; pi ignores it",
			level, model,
		))
	}
	return warnings, nil
}

// ---------------------------------------------------------------------------
// helpers
// ---------------------------------------------------------------------------

func optionalValue(value *string) string {
	if value == nil {
		return ""
	}
	return strings.TrimSpace(*value)
}

func optionalValuePtr(value string) *string {
	if value == "" {
		return nil
	}
	return &value
}

func isThinkingLevel(level string) bool {
	for _, known := range ThinkingLevels {
		if known == level {
			return true
		}
	}
	return false
}

// firstProviderID returns models.json's first provider id in alphabetical
// order (deterministic; independent of Go map iteration).
func firstProviderID(providers map[string]PiProviderConfig) string {
	ids := sortedProviderIDs(providers)
	if len(ids) == 0 {
		return ""
	}
	return ids[0]
}

// sortedProviderIDs lists provider ids alphabetically so every consumer (apply
// receipt, repair pass) agrees on the "first" provider.
func sortedProviderIDs(providers map[string]PiProviderConfig) []string {
	ids := make([]string, 0, len(providers))
	for id := range providers {
		ids = append(ids, id)
	}
	sort.Strings(ids)
	return ids
}

// firstModelID returns the provider's first declared model (upstream keeps the
// list ordered, so this is the model a human would pick first).
func firstModelID(provider PiProviderConfig) string {
	for _, model := range provider.Models {
		if strings.TrimSpace(model.ID) != "" {
			return model.ID
		}
	}
	return ""
}

func modelIDs(provider PiProviderConfig) []string {
	ids := make([]string, 0, len(provider.Models))
	for _, model := range provider.Models {
		if strings.TrimSpace(model.ID) != "" {
			ids = append(ids, model.ID)
		}
	}
	return ids
}

func providerHasModel(provider PiProviderConfig, modelID string) bool {
	_, ok := findProviderModel(provider, modelID)
	return ok
}

// thinkingLevelAllowed reports a supported level: the entry must exist and be
// non-nil (a nil entry is upstream's "not supported").
func thinkingLevelAllowed(mapped map[string]*string, level string) bool {
	value, ok := mapped[level]
	return ok && value != nil
}

// thinkingLevelsFor returns the thinking level map of a model, preferring the
// declared models.json entry over pi's cached catalog. ok == false means
// neither source declares a map, so nothing can be called unsupported.
func thinkingLevelsFor(
	providers map[string]PiProviderConfig,
	catalog startupCatalog,
	providerID string,
	modelID string,
) (map[string]*string, bool) {
	if model, ok := findProviderModel(providers[providerID], modelID); ok && len(model.ThinkingLevelMap) > 0 {
		return model.ThinkingLevelMap, true
	}
	return catalog.thinkingLevelMap(providerID, modelID)
}

func findProviderModel(provider PiProviderConfig, modelID string) (PiModel, bool) {
	for _, model := range provider.Models {
		if model.ID == modelID {
			return model, true
		}
	}
	return PiModel{}, false
}
