package tui

import (
	"context"
	"encoding/json"
	"fmt"
	"sort"

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
			return newKVScreen("Droid Settings", droidSettingsRows()), nil
		}},
		{key: "missions", title: "Missions", open: func(ctx context.Context) (Screen, error) {
			return newKVScreen("Droid Missions", droidMissionRows()), nil
		}},
		{key: "trusted", title: "Trusted", open: list(DroidTrustedSource)},
		{key: "auth", title: "Auth", open: list(DroidAuthSource)},
		{key: "specs", title: "Specs", open: list(SpecsSource)},
		{key: "mcp", title: "MCP", open: list(McpSource)},
		{key: "models", title: "Models", open: list(FactorySource)},
		{key: "sessions", title: "Sessions", open: list(DroidSessionsSource)},
		{key: "legacy", title: "Legacy", open: func(ctx context.Context) (Screen, error) {
			return newKVScreen("Droid Legacy", droidLegacyRows()), nil
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

func (specsSource) Summary(rows []ListRow) string {
	return fmt.Sprintf("Specs · %d files · r toggle secrets", len(rows))
}
