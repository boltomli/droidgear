package omp

import (
	"encoding/json"
	"fmt"
	"sort"
	"strings"

	"droidgear-wails/internal/core"
)

// ---------------------------------------------------------------------------
// Model roles vs models.yml: keeping role assignments resolvable
// ---------------------------------------------------------------------------
//
// config.yml's `modelRoles` map every role name to "provider/model-id" (OMP's
// own `--model` syntax). Deleting a custom provider — or shrinking its model
// list — in models.yml leaves those references dangling, and OMP does not
// recover from both cases the same way:
//
//   - provider gone:  "No default model selected. …"  (OMP refuses to start)
//   - model gone:     "Warning: Default model proxy/m2 not found. Using proxy/m1"
//
// ReconcileModelRoles is the single repair path, the OMP twin of pi's
// ReconcileStartupModel. It only rewrites a role that the models.yml write made
// unresolvable, and it never touches a role pointing at a provider OMP's own
// model cache (models.db `model_cache`) knows: those are catalog providers
// models.yml was merely overriding, so removing the override does not remove
// the provider.
//
// Every repair lands on the first available "provider/model-id": models.yml
// providers first (alphabetical id, declaration order for models — that is the
// order a human reads the file in), then the cached catalog. With nothing left
// to point at, the role is written as null, which is exactly how OMP records an
// unassigned role.

// OmpRoleAdjustment is one role a models.yml write made unresolvable, plus
// where it moved. To == nil means the role was cleared.
type OmpRoleAdjustment struct {
	Role   string  `json:"role"`
	From   string  `json:"from"`
	To     *string `json:"to"`
	Reason string  `json:"reason"`
}

// OmpThinkingAdjustment reports a defaultThinkingLevel reset: the model the
// default role now points at cannot express the level, so config.yml falls back
// to "auto" instead of silently sending an unsupported effort.
type OmpThinkingAdjustment struct {
	From   string `json:"from"`
	To     string `json:"to"`
	Reason string `json:"reason"`
}

// OmpRoleReconcile reports the repair pass. Adjusted is true when config.yml
// was rewritten.
type OmpRoleReconcile struct {
	Adjusted    bool                   `json:"adjusted"`
	Adjustments []OmpRoleAdjustment    `json:"adjustments,omitempty"`
	Thinking    *OmpThinkingAdjustment `json:"thinking,omitempty"`
	Roles       OmpModelRoles          `json:"roles"`
}

// ompRoleNames is the upstream role set in display order (chat roles first,
// then the model-kind roles). Unknown roles live in OmpModelRoles.Extra and are
// repaired by the same rules.
var ompRoleNames = []string{
	"default", "smol", "slow", "vision", "plan", "commit", "tiny", "memory",
	"task", "advisor", "image", "web", "speech", "dictation", "judge",
}

// ompRoleSlot returns the pointer to a modelled role field so one loop can read
// and rewrite every role.
func ompRoleSlot(roles *OmpModelRoles, name string) **string {
	switch name {
	case "default":
		return &roles.Default
	case "smol":
		return &roles.Smol
	case "slow":
		return &roles.Slow
	case "vision":
		return &roles.Vision
	case "plan":
		return &roles.Plan
	case "commit":
		return &roles.Commit
	case "tiny":
		return &roles.Tiny
	case "memory":
		return &roles.Memory
	case "task":
		return &roles.Task
	case "advisor":
		return &roles.Advisor
	case "image":
		return &roles.Image
	case "web":
		return &roles.Web
	case "speech":
		return &roles.Speech
	case "dictation":
		return &roles.Dictation
	case "judge":
		return &roles.Judge
	default:
		return nil
	}
}

// ReconcileModelRoles repairs config.yml's roles after models.yml changed from
// `before` to `after` (both are models.yml provider maps).
func ReconcileModelRoles(before map[string]any, after map[string]any) (OmpRoleReconcile, error) {
	config, err := readConfigYML()
	if err != nil {
		return OmpRoleReconcile{}, err
	}
	roles := OmpModelRoles{}
	if config.ModelRoles != nil {
		roles = *config.ModelRoles
	}
	targets := newRoleTargets(before, after)
	result := OmpRoleReconcile{Roles: roles}
	changed := false
	defaultRoleReplaced := false

	for _, name := range ompRoleNames {
		slot := ompRoleSlot(&roles, name)
		if slot == nil || *slot == nil {
			continue
		}
		from := *slot
		to, reason := targets.repair(*from)
		if reason == "" {
			continue
		}
		*slot = to
		changed = true
		if name == "default" {
			defaultRoleReplaced = true
		}
		result.Adjustments = append(result.Adjustments, OmpRoleAdjustment{
			Role: name, From: *from, To: to, Reason: reason,
		})
	}

	// Roles this build does not model (upstream additions) are string values in
	// Extra; repair them the same way instead of leaving a stale reference.
	extraNames := make([]string, 0, len(roles.Extra))
	for name := range roles.Extra {
		extraNames = append(extraNames, name)
	}
	sort.Strings(extraNames)
	for _, name := range extraNames {
		raw := roles.Extra[name]
		var value string
		if err := json.Unmarshal(raw, &value); err != nil || strings.TrimSpace(value) == "" {
			continue
		}
		to, reason := targets.repair(value)
		if reason == "" {
			continue
		}
		encoded, err := json.Marshal(to)
		if err != nil {
			return result, err
		}
		roles.Extra[name] = encoded
		changed = true
		result.Adjustments = append(result.Adjustments, OmpRoleAdjustment{
			Role: name, From: value, To: to, Reason: reason,
		})
	}

	// The thinking level belongs to the model the default role now points at.
	// A replaced default role is strict (the old level was chosen for the old
	// model); otherwise only an explicit exclusion resets it.
	if adjustment, changedThinking := targets.reconcileThinking(
		config.DefaultThinkingLevel, roles.Default, defaultRoleReplaced,
	); changedThinking {
		config.DefaultThinkingLevel = optionalRoleValue(ompAutoThinking)
		result.Thinking = adjustment
		changed = true
	}

	if !changed {
		return result, nil
	}
	config.ModelRoles = &roles
	if err := writeConfigYML(config); err != nil {
		return result, err
	}
	result.Roles = roles
	result.Adjusted = true
	return result, nil
}

// roleCatalog is OMP's cached provider catalog: provider id -> model id ->
// cached thinking metadata (nil when the cache entry listed none).
type roleCatalog map[string]map[string]*OmpThinkingConfig

// roleTargets describes what a role may point at once models.yml has been
// written.
type roleTargets struct {
	// before/after are the models.yml provider maps around the write. `after`
	// is authoritative for what exists now; `before` is what proves this write
	// removed the provider (an unrelated stale role is left untouched).
	before map[string]any
	after  map[string]any
	// catalog is OMP's cached provider catalog (models.db model_cache). A
	// provider listed there survives the removal of a models.yml override, so
	// its roles stay valid; cached models also carry the thinking metadata the
	// level check needs.
	catalog roleCatalog
	// catalogTrusted is false when models.db is missing/unreadable/empty: then
	// a provider missing from models.yml cannot be classified, and the pass
	// leaves every role alone rather than guessing.
	catalogTrusted bool
}

func newRoleTargets(before map[string]any, after map[string]any) roleTargets {
	catalog, ok := readRoleCatalog()
	if before == nil {
		before = map[string]any{}
	}
	if after == nil {
		after = map[string]any{}
	}
	return roleTargets{before: before, after: after, catalog: catalog, catalogTrusted: ok}
}

// repair returns the replacement value and a reason; an empty reason means the
// role is fine as it is.
func (t roleTargets) repair(value string) (*string, string) {
	provider, model, ok := splitRoleTarget(value)
	if !ok {
		// A bare model id carries no provider, so a models.yml write cannot be
		// blamed for it. Leave it to `dg omp roles check` to report.
		return nil, ""
	}
	if _, declared := t.after[provider]; declared {
		if model == "" || t.knowsModel(provider, model) {
			return nil, ""
		}
		reason := fmt.Sprintf("model %q is not in provider %q anymore", model, provider)
		if replacement := t.firstModelOf(provider); replacement != "" {
			return optionalRoleValue(provider + "/" + replacement), reason
		}
		if replacement := t.firstCandidate(); replacement != "" {
			return optionalRoleValue(replacement), reason
		}
		return nil, reason + " and no provider/model is left"
	}
	if _, declaredBefore := t.before[provider]; !declaredBefore {
		// The provider was never supplied by models.yml, so this write did not
		// remove it; a stale value here is a profile/typo problem that
		// ValidateModelRoles reports instead.
		return nil, ""
	}
	if !t.catalogTrusted || t.catalogKnowsProvider(provider) {
		return nil, ""
	}
	reason := fmt.Sprintf("provider %q is no longer in models.yml", provider)
	if replacement := t.firstCandidate(); replacement != "" {
		return optionalRoleValue(replacement), reason
	}
	return nil, reason + " and no provider/model is left"
}

// reconcileThinking resets defaultThinkingLevel to "auto" when the model the
// default role points at cannot express it. `strict` means this pass replaced
// the default role, so the level was chosen for a different model and must not
// survive unless the new model declares it; without `strict` only a model that
// explicitly lists its efforts can invalidate the level.
func (t roleTargets) reconcileThinking(level *string, defaultRole *string, strict bool) (*OmpThinkingAdjustment, bool) {
	if level == nil {
		return nil, false
	}
	current := strings.TrimSpace(*level)
	if current == "" || current == ompAutoThinking {
		return nil, false
	}
	reset := func(reason string) (*OmpThinkingAdjustment, bool) {
		return &OmpThinkingAdjustment{From: current, To: ompAutoThinking, Reason: reason}, true
	}
	if !isOmpThinkingLevel(current) {
		return reset(fmt.Sprintf("thinking level %q is not one of %s",
			current, strings.Join(ompThinkingLevels, " | ")))
	}
	provider, model, ok := splitRoleTarget(optionalRoleText(defaultRole))
	if !ok {
		if strict {
			return reset("the default role no longer points at a provider/model")
		}
		return nil, false
	}
	thinking, known := t.modelThinking(provider, model)
	if !known {
		if strict {
			return reset(fmt.Sprintf("provider %q does not list model %q anymore", provider, model))
		}
		return nil, false
	}
	if thinking == nil {
		if strict {
			return reset(fmt.Sprintf("%q does not declare any thinking levels", model))
		}
		return nil, false
	}
	if thinkingModeDisables(thinking) {
		return reset(fmt.Sprintf("%q has thinking disabled", model))
	}
	efforts := thinkingEfforts(thinking)
	if len(efforts) == 0 {
		if strict {
			return reset(fmt.Sprintf("%q does not declare its thinking levels", model))
		}
		return nil, false
	}
	for _, effort := range efforts {
		if effort == current {
			return nil, false
		}
	}
	return reset(fmt.Sprintf("%q supports only %s", model, strings.Join(efforts, " | ")))
}

// modelThinking returns the thinking metadata for a model, preferring
// models.yml (declared state) and falling back to the cached catalog. `known`
// reports whether the model exists at all; `nil, true` means it exists without
// thinking metadata.
func (t roleTargets) modelThinking(providerID string, modelID string) (*OmpThinkingConfig, bool) {
	if provider, declared := t.after[providerID]; declared {
		if thinking, found := providerModelThinking(provider, modelID); found {
			return thinking, true
		}
		for _, id := range providerModelIDs(provider) {
			if id == modelID {
				return nil, true
			}
		}
	}
	if models, ok := t.catalog[providerID]; ok {
		if thinking, found := models[modelID]; found {
			return thinking, true
		}
	}
	return nil, false
}

// knowsModel reports whether the provider still lists the model.
func (t roleTargets) knowsModel(providerID string, modelID string) bool {
	for _, id := range providerModelIDs(t.after[providerID]) {
		if id == modelID {
			return true
		}
	}
	_, cached := t.catalog[providerID][modelID]
	return cached
}

func (t roleTargets) catalogKnowsProvider(providerID string) bool {
	_, ok := t.catalog[providerID]
	return ok
}

// firstModelOf returns the provider's first declared model (models.yml order).
func (t roleTargets) firstModelOf(providerID string) string {
	for _, id := range providerModelIDs(t.after[providerID]) {
		return id
	}
	return ""
}

// firstCandidate picks models.yml's first model across providers (alphabetical
// provider id), then falls back to the cached catalog.
func (t roleTargets) firstCandidate() string {
	ids := make([]string, 0, len(t.after))
	for id := range t.after {
		ids = append(ids, id)
	}
	sort.Strings(ids)
	for _, id := range ids {
		if model := t.firstModelOf(id); model != "" {
			return id + "/" + model
		}
	}
	catalogIDs := make([]string, 0, len(t.catalog))
	for id := range t.catalog {
		catalogIDs = append(catalogIDs, id)
	}
	sort.Strings(catalogIDs)
	for _, id := range catalogIDs {
		models := make([]string, 0, len(t.catalog[id]))
		for model := range t.catalog[id] {
			models = append(models, model)
		}
		sort.Strings(models)
		if len(models) > 0 {
			return id + "/" + models[0]
		}
	}
	return ""
}

// OmpRoleOptions is the list of values a role input can pick from: models.yml
// merged with the cached catalog, provider ids alphabetical, models.yml model
// order preserved.
type OmpRoleOptions struct {
	Providers []string            `json:"providers"`
	Models    map[string][]string `json:"models"`
}

// ListRoleOptions merges models.yml with OMP's cached catalog so the GUI
// pickers, `dg omp roles candidates` and the TUI hints offer the same choices.
// models.yml wins for a provider both sources know (it is the declared state).
func ListRoleOptions() (OmpRoleOptions, error) {
	options := OmpRoleOptions{Models: map[string][]string{}}
	config, err := readModelsYML()
	if err != nil {
		return options, err
	}
	for providerID, value := range config.Providers {
		options.Providers = append(options.Providers, providerID)
		options.Models[providerID] = providerModelIDs(value)
	}
	sort.Strings(options.Providers)
	catalog, ok := readRoleCatalog()
	if !ok {
		return options, nil
	}
	for providerID, models := range catalog {
		if _, seen := options.Models[providerID]; seen {
			continue
		}
		ids := make([]string, 0, len(models))
		for modelID := range models {
			ids = append(ids, modelID)
		}
		sort.Strings(ids)
		options.Providers = append(options.Providers, providerID)
		options.Models[providerID] = ids
	}
	sort.Strings(options.Providers)
	return options, nil
}

// RoleValues flattens ListRoleOptions into "provider/model-id" strings — the
// value a role input holds.
func RoleValues(options OmpRoleOptions) []string {
	values := make([]string, 0, len(options.Providers))
	for _, providerID := range options.Providers {
		for _, modelID := range options.Models[providerID] {
			values = append(values, providerID+"/"+modelID)
		}
	}
	return values
}

// ValidateModelRoles explains every role that OMP cannot resolve. It never
// rewrites anything: `dg omp profiles apply` reports the notes next to its
// receipt so a profile that pins a deleted provider fails loudly instead of at
// the next OMP start.
func ValidateModelRoles(roles OmpModelRoles, thinkingLevel *string) ([]string, error) {
	config, err := readModelsYML()
	if err != nil {
		return nil, err
	}
	targets := roleTargets{after: config.Providers}
	if catalog, ok := readRoleCatalog(); ok {
		targets.catalog = catalog
		targets.catalogTrusted = true
	}
	warnings := []string{}
	check := func(name string, value string) {
		provider, model, ok := splitRoleTarget(value)
		if !ok {
			return
		}
		if _, declared := targets.after[provider]; declared {
			if model != "" && !targets.knowsModel(provider, model) {
				warnings = append(warnings, fmt.Sprintf(
					"modelRoles.%s points at %q; provider %q does not list that model (OMP falls back to %q)",
					name, value, provider, targets.firstModelOf(provider),
				))
			}
			return
		}
		if targets.catalogTrusted && targets.catalogKnowsProvider(provider) {
			return
		}
		warnings = append(warnings, fmt.Sprintf(
			"modelRoles.%s points at %q; provider %q is not in models.yml or OMP's catalog (OMP exits with \"No default model selected\")",
			name, value, provider,
		))
	}
	for _, name := range ompRoleNames {
		slot := ompRoleSlot(&roles, name)
		if slot == nil || *slot == nil {
			continue
		}
		check(name, strings.TrimSpace(**slot))
	}
	extraNames := make([]string, 0, len(roles.Extra))
	for name := range roles.Extra {
		extraNames = append(extraNames, name)
	}
	sort.Strings(extraNames)
	for _, name := range extraNames {
		var value string
		if err := json.Unmarshal(roles.Extra[name], &value); err != nil {
			continue
		}
		check(name, strings.TrimSpace(value))
	}
	if adjustment, changed := targets.reconcileThinking(thinkingLevel, roles.Default, false); changed {
		warnings = append(warnings, fmt.Sprintf(
			"defaultThinkingLevel %q cannot be expressed by the default model (%s); OMP falls back to auto",
			adjustment.From, adjustment.Reason,
		))
	}
	return warnings, nil
}

// ---------------------------------------------------------------------------
// helpers
// ---------------------------------------------------------------------------

// splitRoleTarget splits OMP's "provider/model-id" role value at the first
// slash: provider ids never contain one, while model ids can (e.g.
// "groq/compound"). A value without a provider is not a role target this pass
// can repair.
func splitRoleTarget(value string) (string, string, bool) {
	value = strings.TrimSpace(value)
	index := strings.Index(value, "/")
	if index <= 0 || index >= len(value)-1 {
		return "", "", false
	}
	return value[:index], value[index+1:], true
}

// optionalRoleText dereferences an optional string ("" when unset).
func optionalRoleText(value *string) string {
	if value == nil {
		return ""
	}
	return *value
}

func optionalRoleValue(value string) *string {
	if strings.TrimSpace(value) == "" {
		return nil
	}
	return &value
}

// providerModelIDs reads the model ids of one models.yml provider entry
// (untyped YAML/JSON tree).
func providerModelIDs(provider any) []string {
	entry, ok := provider.(map[string]any)
	if !ok {
		return nil
	}
	rawModels, ok := entry["models"].([]any)
	if !ok {
		return nil
	}
	ids := make([]string, 0, len(rawModels))
	for _, rawModel := range rawModels {
		model, ok := rawModel.(map[string]any)
		if !ok {
			continue
		}
		id, _ := model["id"].(string)
		if strings.TrimSpace(id) == "" {
			continue
		}
		ids = append(ids, id)
	}
	return ids
}

// readRoleCatalog loads OMP's cached provider catalog. The boolean reports
// whether it is safe to classify a provider as "not a catalog provider": a
// missing or unreadable models.db (locked by a running OMP, first run) gives no
// answer, so the repair pass stays out of the way.
func readRoleCatalog() (roleCatalog, bool) {
	path, err := ompModelsDBPath()
	if err != nil || !core.FileExists(path) {
		return nil, false
	}
	entries, err := readModelCacheFromDB(path)
	if err != nil {
		return nil, false
	}
	catalog := roleCatalog{}
	for _, entry := range entries {
		if strings.TrimSpace(entry.ProviderID) == "" {
			continue
		}
		models := catalog[entry.ProviderID]
		if models == nil {
			models = map[string]*OmpThinkingConfig{}
			catalog[entry.ProviderID] = models
		}
		for _, model := range entry.Models {
			if strings.TrimSpace(model.ID) != "" {
				models[model.ID] = model.Thinking
			}
		}
	}
	if len(catalog) == 0 {
		return nil, false
	}
	return catalog, true
}

// ompAutoThinking is OMP's "let the model decide" level (`--thinking` accepts
// auto alongside off…max), and the value config.yml falls back to.
const ompAutoThinking = "auto"

// ompThinkingLevels is OMP's `--thinking` enum (see `omp --help`).
var ompThinkingLevels = []string{"off", "minimal", "low", "medium", "high", "xhigh", "max", ompAutoThinking}

func isOmpThinkingLevel(level string) bool {
	for _, known := range ompThinkingLevels {
		if known == level {
			return true
		}
	}
	return false
}

// thinkingEfforts lists the levels a model's thinking config declares (empty
// when it declares none, i.e. the metadata cannot invalidate a level).
func thinkingEfforts(thinking *OmpThinkingConfig) []string {
	if thinking == nil || thinking.Efforts == nil {
		return nil
	}
	efforts := make([]string, 0, len(*thinking.Efforts))
	for _, effort := range *thinking.Efforts {
		if trimmed := strings.TrimSpace(effort); trimmed != "" {
			efforts = append(efforts, trimmed)
		}
	}
	return efforts
}

// thinkingModeDisables reports a model whose thinking mode is switched off, so
// every level except "auto"/"off" is meaningless.
func thinkingModeDisables(thinking *OmpThinkingConfig) bool {
	if thinking == nil || thinking.Mode == nil {
		return false
	}
	switch strings.TrimSpace(*thinking.Mode) {
	case "none", "off", "disabled":
		return true
	default:
		return false
	}
}

// providerModelThinking reads one models.yml model's `thinking` block (untyped
// YAML tree). `found` reports whether the model exists in the provider.
func providerModelThinking(provider any, modelID string) (*OmpThinkingConfig, bool) {
	entry, ok := provider.(map[string]any)
	if !ok {
		return nil, false
	}
	rawModels, ok := entry["models"].([]any)
	if !ok {
		return nil, false
	}
	for _, rawModel := range rawModels {
		model, ok := rawModel.(map[string]any)
		if !ok {
			continue
		}
		id, _ := model["id"].(string)
		if id != modelID {
			continue
		}
		raw, ok := model["thinking"]
		if !ok || raw == nil {
			return nil, true
		}
		thinking := &OmpThinkingConfig{}
		if err := core.FromMap(toAnyMap(raw), thinking); err != nil {
			return nil, true
		}
		return thinking, true
	}
	return nil, false
}

// toAnyMap narrows a YAML-derived value to the object shape FromMap expects.
func toAnyMap(value any) map[string]any {
	if mapped, ok := value.(map[string]any); ok {
		return mapped
	}
	return map[string]any{}
}

func cloneProviderMap(providers map[string]any) map[string]any {
	clone := make(map[string]any, len(providers))
	for id, value := range providers {
		clone[id] = value
	}
	return clone
}
