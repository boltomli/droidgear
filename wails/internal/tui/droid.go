package tui

import (
	"context"
	"encoding/json"
	"fmt"
	"sort"
	"strconv"
	"strings"

	"droidgear-wails/internal/service/droid"
	"droidgear-wails/internal/service/legacy"
	"droidgear-wails/internal/service/specs"
)

// newDroidScreen builds the sectioned Droid pane — the TUI counterpart of
// the GUI's Droid sub-views (settings, missions, trusted-folders,
// auth-profiles, mcp, models, legacy-versions; sessions/specs follow with
// tui-parity P3). The shell (tab/number keys, status line) lives in
// sectionedScreen.
func newDroidScreen(ctx context.Context) (Screen, error) {
	return newSectionedScreen(ctx, "Droid", droidSections())
}

func droidSections() []screenSection {
	list := func(source func() ListSource) func(context.Context) (Screen, error) {
		return func(ctx context.Context) (Screen, error) {
			return newListScreen(ctx, source())
		}
	}
	return []screenSection{
		{key: "settings", title: "Settings", open: func(ctx context.Context) (Screen, error) {
			return newKVScreen("Droid Settings", droidSettingsRows()).
				withRebuild(droidSettingsRows).
				withActions(droidSettingsActions()...), nil
		}},
		{key: "missions", title: "Missions", open: func(ctx context.Context) (Screen, error) {
			return newKVScreen("Droid Missions", droidMissionRows()).
				withRebuild(droidMissionRows).
				withActions(droidMissionActions()...), nil
		}},
		{key: "trusted", title: "Trusted", open: list(DroidTrustedSource)},
		{key: "auth", title: "Auth", open: list(DroidAuthSource)},
		{key: "specs", title: "Specs", open: list(SpecsSource)},
		{key: "mcp", title: "MCP", open: list(McpSource)},
		{key: "models", title: "Models", open: list(FactorySource)},
		{key: "sessions", title: "Sessions", open: list(DroidSessionsSource)},
		{key: "legacy", title: "Legacy", open: func(ctx context.Context) (Screen, error) {
			return newKVScreen("Droid Legacy", droidLegacyRows()).
				withRebuild(droidLegacyRows).
				withActions(kvAction{
					key: "d", label: "delete legacy config",
					run: func(kvRow) error { return legacy.Delete() },
				}), nil
		}},
	}
}

// ---------------------------------------------------------------------------
// Settings section (kv)
// ---------------------------------------------------------------------------

// droidSettingsRows mirrors DroidSettingsPage: active profile + behavior +
// compaction, every value read through internal/service/droid.
func droidSettingsRows() []kvRow {
	rows := []kvRow{}

	// Files section: every managed settings file (use/delete act on the
	// selected "file:<name>" row).
	if files, err := droid.ListSettingsFiles(); err == nil {
		for _, file := range files {
			value := file.Path
			if file.IsActive {
				value += "  (active)"
			}
			if !file.Exists {
				value += "  (missing)"
			}
			rows = append(rows, kvRow{section: "Files", key: "file:" + file.Name, value: value})
		}
	}

	files, err := droid.ListSettingsFiles()
	active := "(unknown)"
	for _, file := range files {
		if file.IsActive {
			active = file.Name
			if !file.IsGlobal {
				active += " — " + file.Path
			}
			break
		}
	}
	rows = append(rows, kvRow{section: "Active profile", key: "settings file", value: kvText(active, err)})
	if command, err := droid.GetLaunchCommand(); err == nil {
		rows = append(rows, kvRow{section: "Active profile", key: "launch command", value: command[0]})
	}

	add := func(section, key, value string, err error) {
		rows = append(rows, kvRow{section: section, key: key, value: kvText(value, err)})
	}
	boolText := func(v bool) string {
		if v {
			return "on"
		}
		return "off"
	}

	if v, err := droid.GetDiffMode(); err == nil {
		add("Behavior", "diff mode", v, err)
	}
	if v, err := droid.GetTodoDisplayMode(); err == nil {
		add("Behavior", "todo display", v, err)
	}
	if v, err := droid.GetCloudSessionSync(); err == nil {
		add("Behavior", "cloud session sync", boolText(v), err)
	}
	if v, err := droid.GetIncludeCoAuthoredByDroid(); err == nil {
		add("Behavior", "co-authored-by Droid", boolText(v), err)
	}
	if v, err := droid.GetShowThinkingInMainView(); err == nil {
		add("Behavior", "show thinking", boolText(v), err)
	}
	if v, err := droid.GetReasoningEffort(); err == nil {
		add("Behavior", "reasoning effort", pointerValue(v, "(unset)"), err)
	}

	if v, err := droid.GetCompactionModelMode(); err == nil {
		add("Compaction", "model mode", v, err)
	}
	if v, err := droid.GetCompactionTokenLimit(); err == nil {
		add("Compaction", "token limit", fmt.Sprintf("%d", v), err)
	}
	if v, err := droid.GetCompactionTokenLimitPerModel(); err == nil {
		keys := make([]string, 0, len(v))
		for model := range v {
			keys = append(keys, model)
		}
		sort.Strings(keys)
		add("Compaction", "per-model overrides", fmt.Sprintf("%d", len(keys)), err)
	}
	return rows
}

// droidSettingsActions is the write surface of the Settings section: file
// use/delete (row-scoped), boolean toggles, enum cycles and the two
// numeric/per-model prompts — all through the preview flow.
func droidSettingsActions() []kvAction {
	toggleBool := func(label string, key string, get func() (bool, error), save func(bool) error) kvAction {
		return kvAction{key: key, label: label, run: func(kvRow) error {
			current, err := get()
			if err != nil {
				return err
			}
			return save(!current)
		}}
	}
	cycleEnum := func(label string, key string, values []string, get func() (string, error), save func(string) error) kvAction {
		return kvAction{key: key, label: label, run: func(kvRow) error {
			current, err := get()
			if err != nil {
				return err
			}
			return save(nextChoice(values, current))
		}}
	}

	return []kvAction{
		{key: "u", label: "use file", onPrefix: "file:", run: func(row kvRow) error {
			name := strings.TrimPrefix(row.key, "file:")
			_, err := droid.SetActiveSettingsFile(&name)
			return err
		}},
		{key: "D", label: "delete file", onPrefix: "file:", run: func(row kvRow) error {
			return droid.DeleteSettingsFile(strings.TrimPrefix(row.key, "file:"))
		}},
		toggleBool("toggle cloud session sync", "c",
			droid.GetCloudSessionSync, droid.SaveCloudSessionSync),
		toggleBool("toggle co-authored-by", "a",
			droid.GetIncludeCoAuthoredByDroid, droid.SaveIncludeCoAuthoredByDroid),
		toggleBool("toggle show thinking", "s",
			droid.GetShowThinkingInMainView, droid.SaveShowThinkingInMainView),
		cycleEnum("cycle diff mode", "d", []string{"github", "unified"},
			droid.GetDiffMode, droid.SaveDiffMode),
		cycleEnum("cycle todo display", "o", []string{"pinned", "inline"},
			droid.GetTodoDisplayMode, droid.SaveTodoDisplayMode),
		{key: "r", label: "cycle reasoning effort", run: func(kvRow) error {
			current, err := droid.GetReasoningEffort()
			if err != nil {
				return err
			}
			value := ""
			if current != nil {
				value = *current
			}
			return droid.SaveReasoningEffort(nextChoice(
				[]string{"", "off", "low", "medium", "high", "max"}, value))
		}},
		cycleEnum("cycle compaction mode", "m", []string{"current-model", "factory-default"},
			droid.GetCompactionModelMode, droid.SaveCompactionModelMode),
		promptAction("l", "set compaction limit", kvPrompt{
			title: "compaction token limit",
			hint:  "positive integer",
			initial: func(kvRow) string {
				value, err := droid.GetCompactionTokenLimit()
				if err != nil {
					return ""
				}
				return fmt.Sprintf("%d", value)
			},
			submit: func(_ kvRow, entered string) writeAction {
				return writeAction{title: "set compaction token limit", run: func() error {
					parsed, err := strconv.ParseInt(strings.TrimSpace(entered), 10, 32)
					if err != nil || parsed <= 0 {
						return fmt.Errorf("expected a positive integer, got %q", entered)
					}
					return droid.SaveCompactionTokenLimit(int32(parsed))
				}}
			},
		}),
		promptAction("p", "per-model compaction limit", kvPrompt{
			title: "per-model compaction limit",
			hint:  "modelId [limit] - leave the limit empty to remove that model's override",
			submit: func(_ kvRow, entered string) writeAction {
				return writeAction{title: "set per-model compaction limit", run: func() error {
					model, limit := splitFirstSpace(entered)
					if model == "" {
						return fmt.Errorf("expected: modelId [limit]")
					}
					current, err := droid.GetCompactionTokenLimitPerModel()
					if err != nil {
						return err
					}
					if current == nil {
						current = map[string]int32{}
					}
					if limit == "" {
						delete(current, model)
						return droid.SaveCompactionTokenLimitPerModel(current)
					}
					parsed, err := strconv.ParseInt(strings.TrimSpace(limit), 10, 32)
					if err != nil || parsed <= 0 {
						return fmt.Errorf("expected a positive integer for the limit, got %q", limit)
					}
					current[model] = int32(parsed)
					return droid.SaveCompactionTokenLimitPerModel(current)
				}}
			},
		}),
	}
}

// nextChoice cycles through values (the current value not being listed
// starts the cycle from the first entry, so an unset field joins its own
// cycle as "").
func nextChoice(values []string, current string) string {
	for i, value := range values {
		if value == current {
			return values[(i+1)%len(values)]
		}
	}
	return values[0]
}

// droidMissionRows mirrors MissionsPage (mission worker model settings).
func droidMissionRows() []kvRow {
	settings, err := droid.GetMissionModelSettings()
	text := func(v *string) string {
		if err != nil {
			return "error: " + err.Error()
		}
		return pointerValue(v, "(unset)")
	}
	return []kvRow{
		{section: "Worker", key: "worker model", value: text(settings.WorkerModel)},
		{section: "Worker", key: "worker reasoning effort", value: text(settings.WorkerReasoningEffort)},
		{section: "Validation", key: "validation model", value: text(settings.ValidationWorkerModel)},
		{section: "Validation", key: "validation reasoning effort", value: text(settings.ValidationWorkerReasoningEffort)},
	}
}

// droidLegacyRows mirrors LegacyVersionsPage's check (read-only here; the
// delete action lands with the P2 write path).
func droidLegacyRows() []kvRow {
	found, err := legacy.Check()
	value := "absent"
	switch {
	case err != nil:
		value = "error: " + err.Error()
	case found:
		value = "present (legacy config.json with customModels)"
	}
	return []kvRow{
		{section: "Legacy", key: "legacy config", value: value},
	}
}

// droidMissionActions edits the four mission model fields: free-text prompts
// for the model ids, enum cycles for the efforts (GUI option set: unset /
// none / low / medium / high).
func droidMissionActions() []kvAction {
	pickModel := func(label string, key string, get func(droid.MissionModelSettings) *string, set func(*droid.MissionModelSettings, *string)) kvAction {
		return promptAction(key, label, kvPrompt{
			title:      label,
			hint:       "model id - empty clears the field (GUI: not set)",
			allowEmpty: true,
			initial: func(kvRow) string {
				settings, err := droid.GetMissionModelSettings()
				if err != nil {
					return ""
				}
				return pointerValue(get(settings), "")
			},
			submit: func(_ kvRow, entered string) writeAction {
				return writeAction{title: label, run: func() error {
					settings, err := droid.GetMissionModelSettings()
					if err != nil {
						return err
					}
					set(&settings, strPtrOrNil(entered))
					return droid.SaveMissionModelSettings(settings)
				}}
			},
		})
	}
	cycleEffort := func(label string, key string, get func(droid.MissionModelSettings) *string, set func(*droid.MissionModelSettings, *string)) kvAction {
		return kvAction{key: key, label: label, run: func(kvRow) error {
			settings, err := droid.GetMissionModelSettings()
			if err != nil {
				return err
			}
			current := pointerValue(get(settings), "")
			set(&settings, strPtrOrNil(nextChoice([]string{"", "none", "low", "medium", "high"}, current)))
			return droid.SaveMissionModelSettings(settings)
		}}
	}

	return []kvAction{
		pickModel("worker model", "w",
			func(s droid.MissionModelSettings) *string { return s.WorkerModel },
			func(s *droid.MissionModelSettings, v *string) { s.WorkerModel = v }),
		pickModel("validation model", "v",
			func(s droid.MissionModelSettings) *string { return s.ValidationWorkerModel },
			func(s *droid.MissionModelSettings, v *string) { s.ValidationWorkerModel = v }),
		cycleEffort("cycle worker effort", "e",
			func(s droid.MissionModelSettings) *string { return s.WorkerReasoningEffort },
			func(s *droid.MissionModelSettings, v *string) { s.WorkerReasoningEffort = v }),
		cycleEffort("cycle validation effort", "r",
			func(s droid.MissionModelSettings) *string { return s.ValidationWorkerReasoningEffort },
			func(s *droid.MissionModelSettings, v *string) { s.ValidationWorkerReasoningEffort = v }),
	}
}

func kvText(value string, err error) string {
	if err != nil {
		return "error: " + err.Error()
	}
	return value
}

// ---------------------------------------------------------------------------
// List sections
// ---------------------------------------------------------------------------

// droidTrustedSource adapts internal/service/droid trusted folders (Droid →
// Trusted folders).
type droidTrustedSource struct{}

// DroidTrustedSource exposes the trusted folders to the generic screen.
func DroidTrustedSource() ListSource { return droidTrustedSource{} }

func (droidTrustedSource) Title() string { return "Trusted Folders" }

func (droidTrustedSource) Load(context.Context) ([]ListRow, error) {
	folders, err := droid.ListTrustedFolders()
	if err != nil {
		return nil, err
	}
	rows := make([]ListRow, 0, len(folders))
	for _, folder := range folders {
		rows = append(rows, ListRow{ID: folder.Path, Label: folder.Path, Mark: "● "})
	}
	return rows, nil
}

func (droidTrustedSource) Detail(_ context.Context, path string) ([]byte, error) {
	folders, err := droid.ListTrustedFolders()
	if err != nil {
		return nil, err
	}
	for _, folder := range folders {
		if folder.Path == path {
			return json.MarshalIndent(folder, "", "  ")
		}
	}
	return nil, fmt.Errorf("trusted folder %q not found", path)
}

func (droidTrustedSource) Summary(rows []ListRow) string {
	return fmt.Sprintf("Trusted · %d folders · r toggle secrets", len(rows))
}

// droidAuthSource adapts internal/service/droid Factory auth profiles
// (Droid → Auth profiles).
type droidAuthSource struct{}

// DroidAuthSource exposes the Factory auth profiles to the generic screen.
func DroidAuthSource() ListSource { return droidAuthSource{} }

func (droidAuthSource) Title() string { return "Droid Auth" }

func (droidAuthSource) Load(context.Context) ([]ListRow, error) {
	state, err := droid.ListAuthProfiles()
	if err != nil {
		return nil, err
	}
	rows := make([]ListRow, 0, len(state.Profiles))
	for _, profile := range state.Profiles {
		mark := "  "
		if state.Active != nil && profile.Name == *state.Active {
			mark = "● "
		}
		label := profile.Label
		if label == "" {
			label = profile.Name
		}
		rows = append(rows, ListRow{ID: profile.Name, Label: label, Mark: mark})
	}
	return rows, nil
}

func (droidAuthSource) Detail(_ context.Context, name string) ([]byte, error) {
	state, err := droid.ListAuthProfiles()
	if err != nil {
		return nil, err
	}
	for _, profile := range state.Profiles {
		if profile.Name == name {
			return json.MarshalIndent(profile, "", "  ")
		}
	}
	return nil, fmt.Errorf("Droid auth profile %q not found", name)
}

// Actions wires 'a' = switch the live Factory auth to the selected profile,
// 'd' = delete it.
func (droidAuthSource) Actions() []ListAction {
	return []ListAction{
		{
			Key: "a", Label: "switch",
			Run: func(_ context.Context, row ListRow) error {
				return droid.SwitchAuthProfile(row.ID)
			},
		},
		{
			Key: "d", Label: "delete",
			Run: func(_ context.Context, row ListRow) error {
				return droid.DeleteAuthProfile(row.ID)
			},
		},
	}
}

// Actions wires 'd' = drop the selected directory from the trusted list.
func (droidTrustedSource) Actions() []ListAction {
	return []ListAction{{
		Key: "d", Label: "remove",
		Run: func(_ context.Context, row ListRow) error {
			return droid.RemoveTrustedFolder(row.ID)
		},
	}}
}

// InputActions wires 's' = snapshot the live Factory auth under a new name
// and 'n' = rename the selected profile's label.
func (droidAuthSource) InputActions() []InputAction {
	return []InputAction{
		{
			Key: "s", Label: "save current as",
			Hint: "name [label] - snapshots the live ~/.factory auth files",
			Submit: func(_ context.Context, row ListRow, value string) writeAction {
				return writeAction{
					title: "save current Factory auth as " + value,
					run: func() error {
						name, label := splitFirstSpace(value)
						if label == "" {
							label = name
						}
						return droid.SaveCurrentAuthProfile(name, label)
					},
				}
			},
		},
		{
			Key: "n", Label: "rename",
			Hint:    "new display label",
			Initial: func(row ListRow) string { return row.Label },
			Submit: func(_ context.Context, row ListRow, value string) writeAction {
				return writeAction{
					title: "rename auth " + row.ID,
					run:   func() error { return droid.RenameAuthProfile(row.ID, value) },
				}
			},
		},
	}
}

func (droidAuthSource) Summary(rows []ListRow) string {
	active := "none"
	for _, row := range rows {
		if row.Mark == "● " {
			active = row.Label
			break
		}
	}
	return fmt.Sprintf("Droid Auth · %d profiles · active: %s · r toggle secrets", len(rows), active)
}

// ---------------------------------------------------------------------------
// Specs section (list)
// ---------------------------------------------------------------------------

// specsSource adapts internal/service/specs — the Droid → Specs sub-view of
// the GUI (browse only; rename/delete/update land with the P2 write path).
type specsSource struct{}

// SpecsSource exposes the spec files to the generic screen.
func SpecsSource() ListSource { return specsSource{} }

func (specsSource) Title() string { return "Specs" }

func (specsSource) Load(context.Context) ([]ListRow, error) {
	specList, err := specs.List()
	if err != nil {
		return nil, err
	}
	rows := make([]ListRow, 0, len(specList))
	for _, spec := range specList {
		rows = append(rows, ListRow{ID: spec.Path, Label: spec.Name})
	}
	return rows, nil
}

func (specsSource) Detail(_ context.Context, path string) ([]byte, error) {
	specList, err := specs.List()
	if err != nil {
		return nil, err
	}
	for _, spec := range specList {
		if spec.Path == path {
			return json.MarshalIndent(spec, "", "  ")
		}
	}
	return nil, fmt.Errorf("spec %q not found", path)
}

// Actions wires 'd' = delete the selected spec file.
func (specsSource) Actions() []ListAction {
	return []ListAction{{
		Key: "d", Label: "delete",
		Run: func(_ context.Context, row ListRow) error {
			return specs.Delete(row.ID)
		},
	}}
}

// InputActions wires 'n' = rename the selected spec (the .md suffix is
// appended by the service, like the GUI).
func (specsSource) InputActions() []InputAction {
	return []InputAction{{
		Key: "n", Label: "rename",
		Hint:    "new file name (.md optional)",
		Initial: func(row ListRow) string { return strings.TrimSuffix(row.Label, ".md") },
		Submit: func(_ context.Context, row ListRow, value string) writeAction {
			return writeAction{
				title: "rename spec " + row.Label,
				run: func() error {
					_, err := specs.Rename(row.ID, value)
					return err
				},
			}
		},
	}}
}

func (specsSource) Summary(rows []ListRow) string {
	return fmt.Sprintf("Specs · %d files · r toggle secrets", len(rows))
}
