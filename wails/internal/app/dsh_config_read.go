package app

// Snapshot reading side of the dsh_config port: patch document parsing,
// composition views, migration warnings and the content hash.

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"strings"

	"droidgear-wails/internal/core"

	"gopkg.in/yaml.v3"
)

// ---------------------------------------------------------------------------
// Patch document reading (js-tag tolerant)
// ---------------------------------------------------------------------------

// readPatchDocumentNormalized reads a patch document: a YAML sequence of
// loader patch entries. A missing or empty file yields an empty document;
// `null` roots are accepted as empty. Returns the parsed rows plus the raw
// text (for comment-preserving rewrites).
func readPatchDocumentNormalized(path string) ([]*yaml.Node, string, error) {
	data, err := os.ReadFile(path) // #nosec G304 -- tool config path
	if err != nil {
		if os.IsNotExist(err) {
			return nil, "", nil
		}
		return nil, "", fmt.Errorf("Failed to read DSH patch file: %w", err)
	}
	raw := string(data)
	if strings.TrimSpace(raw) == "" {
		return nil, raw, nil
	}
	root, err := core.ParseYAML(raw)
	if err != nil {
		return nil, raw, fmt.Errorf("Invalid DSH patch YAML: %w", err)
	}
	switch {
	case root == nil:
		return nil, raw, nil
	case core.IsSequence(root):
		return root.Content, raw, nil
	case root.Kind == yaml.ScalarNode && root.Tag == "!!null":
		return nil, raw, nil
	default:
		return nil, raw, errString("Dsh patch file root must be a sequence")
	}
}

// ---------------------------------------------------------------------------
// Snapshot reading
// ---------------------------------------------------------------------------

// ReadDshConfigSnapshot composes the full configuration snapshot for the
// resolved profile (bundles + profile patch + home overlay).
func (a *App) ReadDshConfigSnapshot(profile *string) (DshConfigSnapshot, error) {
	schema, err := dshSchema()
	if err != nil {
		return DshConfigSnapshot{}, err
	}

	legacyPath, err := dshSettingsPath()
	if err != nil {
		return DshConfigSnapshot{}, err
	}
	importedPath := strings.TrimSuffix(legacyPath, ".yaml") + ".yaml.imported"
	legacyExists := fileExists(legacyPath) || fileExists(importedPath)

	homePatch, err := dshHomePatchPath()
	if err != nil {
		return DshConfigSnapshot{}, err
	}

	resolved, err := resolveDshProfile(profile)
	if err != nil {
		return DshConfigSnapshot{}, err
	}

	snapshot := DshConfigSnapshot{
		SchemaVersion:        schema.DshVersion,
		InstalledVersion:     detectInstalledDshVersion(),
		Profile:              nil,
		ProfilePatchPath:     "",
		HomePatchPath:        homePatch,
		HomePatchExists:      fileExists(homePatch),
		LegacySettingsPath:   legacyPath,
		LegacySettingsExists: legacyExists,
		PatchHash:            "absent",
		Entries:              []DshConfigEntryView{},
		Warnings:             []DshMigrationWarning{},
	}

	if resolved == nil {
		snapshot.Warnings = append(snapshot.Warnings, DshMigrationWarning{
			Code:     "DSH_CONFIG_NO_PROFILE",
			Severity: "error",
			Message:  "No DSH runtime profile found under ~/.dsh/profiles",
			Hint:     "Start DeepSeek Harness once so it creates ~/.dsh/profiles/desktop.",
		})
		if legacyExists {
			snapshot.Warnings = append(snapshot.Warnings, DshMigrationWarning{
				Code:     "DSH_CONFIG_LEGACY_SETTINGS",
				Severity: "info",
				Message:  "A legacy settings.yaml document is present",
				Hint:     "DSH 0.2 imports it once and renames it to settings.yaml.imported; no profile was created.",
			})
		}
		return snapshot, nil
	}

	entries, err := composeEntries(*resolved, schema)
	if err != nil {
		return DshConfigSnapshot{}, err
	}
	profilePatch, err := dshProfilePatchPath(*resolved)
	if err != nil {
		return DshConfigSnapshot{}, err
	}

	views := make([]DshConfigEntryView, 0, len(entries))
	for i := range entries {
		views = append(views, entryView(&entries[i], schema))
	}

	snapshot.Profile = resolved
	snapshot.ProfilePatchPath = profilePatch
	snapshot.PatchHash = fileHash(profilePatch)
	snapshot.Entries = views
	snapshot.Warnings = buildWarnings(entries, homePatch, schema, snapshot.InstalledVersion, legacyExists)
	return snapshot, nil
}

func entryView(entry *composedEntry, schema *schemaDoc) DshConfigEntryView {
	meta := schema.Entries[entry.ID]
	effective := entry.Config

	// Redacted copy of the effective config for the `config` field.
	var redacted any
	if object, ok := effective.(map[string]any); ok {
		copy := make(map[string]any, len(object))
		for key, value := range object {
			copy[key] = value
		}
		for _, field := range meta.Fields {
			if !field.Secret {
				continue
			}
			if _, present := copy[field.Name]; present {
				copy[field.Name] = map[string]any{"$secret": true}
			}
		}
		redacted = copy
	} else {
		redacted = effective
	}

	effectiveObject, _ := effective.(map[string]any)
	profileObject, _ := entry.ProfileConfig.(map[string]any)

	fields := make([]DshConfigFieldView, 0, len(meta.Fields))
	for _, field := range meta.Fields {
		var value any
		overridden := false
		if profileObject != nil {
			_, overridden = profileObject[field.Name]
		}
		if effectiveObject != nil {
			if fieldValue, present := effectiveObject[field.Name]; present {
				if field.Secret {
					value = map[string]any{"$secret": true}
				} else {
					value = fieldValue
				}
			}
		}
		fields = append(fields, DshConfigFieldView{
			Name: field.Name, Kind: field.Kind, Value: value,
			Default: field.Default, EnumValues: withEmptySlice(field.EnumValues),
			Volatile: field.Volatile, Secret: field.Secret,
			Description: field.Description, Overridden: overridden, Tier: field.Tier,
		})
	}

	return DshConfigEntryView{
		ID:             entry.ID,
		PackageName:    entry.PackageName,
		Layer:          entry.Layer,
		Tier:           entry.Tier,
		Description:    entry.Description,
		Disabled:       entry.Disabled,
		HasJS:          containsJS(effective),
		Config:         redacted,
		Overridden:     entry.ProfileConfig != nil,
		ShadowedByHome: entry.HomeConfig != nil,
		Fields:         fields,
	}
}

func withEmptySlice(values []string) []string {
	if values == nil {
		return []string{}
	}
	return values
}

func buildWarnings(entries []composedEntry, homePatch string, schema *schemaDoc, installedVersion *string, legacyExists bool) []DshMigrationWarning {
	warnings := []DshMigrationWarning{}
	find := func(id string) *composedEntry {
		for i := range entries {
			if entries[i].ID == id {
				return &entries[i]
			}
		}
		return nil
	}

	if entry := find("tools"); entry != nil {
		if object, ok := entry.Config.(map[string]any); ok && object["mode"] == "both" {
			warnings = append(warnings, DshMigrationWarning{
				Code: "DSH_CONFIG_TOOLS_MODE_BOTH", Severity: "error", EntryID: strPtr(entry.ID),
				Message: `tools.mode = "both" is no longer accepted by DSH 0.2`,
				Hint:    "Use `native` or `ptc`.",
			})
		}
	}

	for i := range entries {
		entry := &entries[i]
		if !strings.HasPrefix(entry.ID, "subagent") {
			continue
		}
		if object, ok := entry.Config.(map[string]any); ok {
			if _, present := object["backgroundMode"]; present {
				warnings = append(warnings, DshMigrationWarning{
					Code: "DSH_CONFIG_SUBAGENT_BACKGROUND_MODE", Severity: "warning", EntryID: strPtr(entry.ID),
					Message: "subagent backgroundMode was removed in DSH 0.2",
					Hint:    "Remove the field; use the subagent tool's own background controls.",
				})
			}
		}
	}

	for i := range entries {
		entry := &entries[i]
		if !presetOnlyEntries[entry.ID] || entry.ProfileConfig == nil || !entry.Disabled {
			continue
		}
		warnings = append(warnings, DshMigrationWarning{
			Code: "DSH_CONFIG_PRESET_ONLY_ROW", Severity: "warning", EntryID: strPtr(entry.ID),
			Message: fmt.Sprintf("The profile patch configures `%s`, but the Web/desktop composition disables that host row", entry.ID),
			Hint:    "Agent-plane rows moved into the agent presets; edit preset-standard/ptc/minimal/cordis instead.",
		})
	}

	if entry := find("llm-pi-ai"); entry != nil {
		if object, ok := entry.Config.(map[string]any); ok {
			if providers, ok := object["providers"].(map[string]any); ok {
				if _, present := providers["azure-openai-responses"]; present {
					warnings = append(warnings, DshMigrationWarning{
						Code: "DSH_CONFIG_AZURE_ROUTE_KEY", Severity: "warning", EntryID: strPtr(entry.ID),
						Message: "The Azure pi-ai route key was renamed to `azure` in DSH 0.2.1",
						Hint:    "Rename providers.azure-openai-responses to providers.azure; the `api` identifier stays azure-openai-responses.",
					})
				}
			}
		}
	}

	for _, id := range []string{"llm-deepseek", "llm-deepseek-account"} {
		entry := find(id)
		if entry == nil {
			continue
		}
		object, ok := entry.Config.(map[string]any)
		if !ok {
			continue
		}
		models, ok := object["models"].([]any)
		if !ok {
			continue
		}
		for _, item := range models {
			model, ok := item.(map[string]any)
			if !ok {
				continue
			}
			if _, present := model["imageDetail"]; present {
				warnings = append(warnings, DshMigrationWarning{
					Code: "DSH_CONFIG_IMAGE_DETAIL", Severity: "warning", EntryID: strPtr(entry.ID),
					Message: "Model field `imageDetail` was removed in DSH 0.2",
					Hint:    "Replace it with `imagePixelBudget`.",
				})
				break
			}
		}
	}

	if installedVersion != nil && *installedVersion != schema.DshVersion {
		warnings = append(warnings, DshMigrationWarning{
			Code: "DSH_CONFIG_VERSION_MISMATCH", Severity: "warning",
			Message: fmt.Sprintf("Installed DeepSeek Harness is %s, but the bundled schema was generated from %s", *installedVersion, schema.DshVersion),
			Hint:    "Regenerate res/dsh-config-schema.json from the installed DSH source before editing unfamiliar fields.",
		})
	}

	for i := range entries {
		entry := &entries[i]
		if entry.HomeConfig != nil && entry.ProfileConfig != nil {
			warnings = append(warnings, DshMigrationWarning{
				Code: "DSH_CONFIG_HOME_SHADOW", Severity: "warning", EntryID: strPtr(entry.ID),
				Message: fmt.Sprintf("%s is configured in both the profile patch and the home overlay; the home overlay wins", entry.ID),
				Hint:    fmt.Sprintf("Move the setting into the profile patch, or clear it from %s first; DroidGear refuses to write a shadowed entry.", homePatch),
			})
		}
	}

	if legacyExists {
		warnings = append(warnings, DshMigrationWarning{
			Code: "DSH_CONFIG_LEGACY_SETTINGS", Severity: "info",
			Message: "Legacy settings.yaml (or .imported) is present",
			Hint:    "DSH 0.2 imports it once into the profile patch; DroidGear only needs cordis.patch.yml.",
		})
	}
	return warnings
}

// ---------------------------------------------------------------------------
// Hashing + installed version detection
// ---------------------------------------------------------------------------

// fileHash implements the optimistic-concurrency hash the UI round-trips
// (FNV-1a 64-bit, matching the Rust core byte for byte).
func fileHash(path string) string {
	data, err := os.ReadFile(path) // #nosec G304 -- tool config path
	if err != nil {
		return "absent"
	}
	return hashBytes(data)
}

func hashBytes(bytes []byte) string {
	hash := uint64(0xcbf29ce484222325)
	for _, b := range bytes {
		hash ^= uint64(b)
		hash *= 0x0100000001b3
	}
	return fmt.Sprintf("%016x", hash)
}

// detectInstalledDshVersion reads the installed desktop app's runtime
// manifest. The Rust core only probes macOS locations (plus the
// DSH_RUNTIME_MANIFEST override), so on Windows this is effectively
// environment-driven.
func detectInstalledDshVersion() *string {
	if path := os.Getenv("DSH_RUNTIME_MANIFEST"); path != "" {
		if version := readDesktopVersion(path); version != nil {
			return version
		}
	}
	candidates := []string{
		"/Applications/DeepSeek Harness.app/Contents/Resources/runtime/primary-runtime/runtime.json",
		"/Applications/DeepSeek Harness.app/Contents/Resources/runtime/versions.json",
	}
	if home, err := core.HomeDir(); err == nil {
		candidates = append(candidates,
			filepath.Join(home, "Applications/DeepSeek Harness.app/Contents/Resources/runtime/primary-runtime/runtime.json"))
	}
	for _, candidate := range candidates {
		if version := readDesktopVersion(candidate); version != nil {
			return version
		}
	}
	return nil
}

func readDesktopVersion(path string) *string {
	data, err := os.ReadFile(path) // #nosec G304 -- app manifest path
	if err != nil {
		return nil
	}
	var value map[string]any
	if json.Unmarshal(data, &value) != nil {
		return nil
	}
	if version, ok := value["desktopVersion"].(string); ok {
		return &version
	}
	if version, ok := value["version"].(string); ok {
		return &version
	}
	return nil
}
