package app

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

// --- path helper semantics --------------------------------------------------

func TestDroidWithExtensionMirrorsPathSemantics(t *testing.T) {
	cases := []struct {
		path string
		ext  string
		want string
	}{
		{"profile", "json", "profile.json"},
		{"my.profile", "json", "my.json"},     // extension replaces after last dot
		{".profile", "json", ".profile.json"}, // dotfiles gain the extension
		{".json", "json", ".json.json"},
		{filepath.Join("dir", "a.b.c"), "tmp", filepath.Join("dir", "a.b.tmp")},
		{filepath.Join("dir", "settings"), "tmp", filepath.Join("dir", "settings.tmp")},
	}
	for _, c := range cases {
		if got := droidWithExtension(c.path, c.ext); got != c.want {
			t.Errorf("droidWithExtension(%q, %q) = %q, want %q", c.path, c.ext, got, c.want)
		}
	}
	if got := droidFileStem("my.profile.json"); got != "my.profile" {
		t.Errorf("droidFileStem = %q", got)
	}
	if got := droidExtension("archive.JSON"); got != "JSON" {
		t.Errorf("droidExtension must be case-sensitive, got %q", got)
	}
	if got := droidExtension(".json"); got != "" {
		t.Errorf("dotfile has no extension, got %q", got)
	}
}

// --- settings files ---------------------------------------------------------

func droidSettingsTestHome(t *testing.T) string {
	t.Helper()
	home := t.TempDir()
	dir := filepath.Join(home, ".factory")
	if err := os.MkdirAll(dir, 0o755); err != nil {
		t.Fatalf("mkdir factory: %v", err)
	}
	if err := os.WriteFile(filepath.Join(dir, "settings.json"), []byte("{\"diffMode\":\"github\"}"), 0o644); err != nil {
		t.Fatalf("write global settings: %v", err)
	}
	return home
}

func TestDroidSettingsFilesCreateSwitchDelete(t *testing.T) {
	home := droidSettingsTestHome(t)

	files, err := droidListSettingsFiles(home)
	if err != nil {
		t.Fatalf("droidListSettingsFiles: %v", err)
	}
	if len(files) != 1 {
		t.Fatalf("expected only Global, got %+v", files)
	}
	global := files[0]
	if global.Name != "Global" || !global.IsGlobal || !global.IsActive || !global.Exists || global.IsExternal {
		t.Fatalf("unexpected global entry: %+v", global)
	}

	created, err := droidCreateSettingsFile(home, "profile", true)
	if err != nil {
		t.Fatalf("droidCreateSettingsFile: %v", err)
	}
	if created.Name != "profile" || created.IsGlobal || !created.IsActive || created.IsExternal {
		t.Fatalf("unexpected created entry: %+v", created)
	}
	profilePath := created.Path
	data, err := os.ReadFile(profilePath) // #nosec G304 -- test fixture
	if err != nil {
		t.Fatalf("read profile: %v", err)
	}
	if !strings.Contains(string(data), "diffMode") {
		t.Fatalf("copyFromActive lost content: %q", data)
	}

	// Errors mirror the Rust core byte-for-byte.
	if _, err := droidCreateSettingsFile(home, "profile", false); err == nil || err.Error() != "Settings file 'profile' already exists" {
		t.Fatalf("expected already-exists, got %v", err)
	}
	if _, err := droidCreateSettingsFile(home, "Global", false); err == nil || err.Error() != "Cannot use 'Global' as a custom file name" {
		t.Fatalf("expected Global rejection, got %v", err)
	}
	if _, err := droidCreateSettingsFile(home, "", false); err == nil || err.Error() != "File name cannot be empty" {
		t.Fatalf("expected empty-name rejection, got %v", err)
	}

	// Switch back to Global (nil and "" are equivalent in Rust).
	back, err := droidSetActiveSettingsFile(home, "")
	if err != nil {
		t.Fatalf("droidSetActiveSettingsFile: %v", err)
	}
	if !back.IsGlobal || back.Name != "Global" {
		t.Fatalf("expected Global active, got %+v", back)
	}
	if _, err := droidSetActiveSettingsFile(home, "missing"); err == nil || err.Error() != "Settings file 'missing' does not exist" {
		t.Fatalf("expected missing error, got %v", err)
	}
	if _, err := droidSetActiveSettingsFile(home, filepath.Join(home, "other.json")); err == nil || !strings.Contains(err.Error(), "not a linked settings file") {
		t.Fatalf("expected not-linked error, got %v", err)
	}

	switched, err := droidSetActiveSettingsFile(home, "profile")
	if err != nil {
		t.Fatalf("switch back: %v", err)
	}
	if switched.Name != "profile" || !switched.IsActive {
		t.Fatalf("unexpected switch result: %+v", switched)
	}

	// Deleting the active file resets the active selection to Global.
	if err := droidDeleteSettingsFile(home, "profile"); err != nil {
		t.Fatalf("droidDeleteSettingsFile: %v", err)
	}
	if _, err := os.Stat(profilePath); !os.IsNotExist(err) {
		t.Fatalf("profile not deleted: %v", err)
	}
	active, err := droidGetActiveSettingsFile(home)
	if err != nil {
		t.Fatalf("droidGetActiveSettingsFile: %v", err)
	}
	if !active.IsGlobal {
		t.Fatalf("expected Global after delete, got %+v", active)
	}
	if err := droidDeleteSettingsFile(home, "global"); err == nil || err.Error() != "Cannot delete the global settings file" {
		t.Fatalf("expected global-delete rejection, got %v", err)
	}
	if err := droidDeleteSettingsFile(home, "nope"); err == nil || err.Error() != "Settings file 'nope' does not exist" {
		t.Fatalf("expected missing-delete rejection, got %v", err)
	}
}

func TestDroidSettingsLinkUnlinkLifecycle(t *testing.T) {
	home := droidSettingsTestHome(t)
	external := filepath.Join(t.TempDir(), "team-settings.json")
	if err := os.WriteFile(external, []byte(`{"sessionDefaultSettings":{"model":"x"}}`), 0o644); err != nil {
		t.Fatalf("write external: %v", err)
	}

	linked, err := droidLinkSettingsFile(home, external)
	if err != nil {
		t.Fatalf("droidLinkSettingsFile: %v", err)
	}
	if linked.Name != "team-settings" || !linked.IsExternal || !linked.IsActive || !linked.Exists {
		t.Fatalf("unexpected linked entry: %+v", linked)
	}
	if !filepath.IsAbs(linked.Path) || filepath.Base(linked.Path) != "team-settings.json" {
		t.Fatalf("linked path not canonical: %q", linked.Path)
	}
	// The source is referenced, never copied.
	if _, err := os.Stat(external); err != nil {
		t.Fatalf("source file must survive linking: %v", err)
	}
	if _, err := os.Stat(filepath.Join(home, ".droidgear", "droid-settings", "team-settings.json")); !os.IsNotExist(err) {
		t.Fatal("link must not create a managed copy")
	}

	// Re-linking is idempotent (same file → same entry, activated again).
	again, err := droidLinkSettingsFile(home, external)
	if err != nil {
		t.Fatalf("re-link: %v", err)
	}
	if again.Path != linked.Path || again.Name != "team-settings" {
		t.Fatalf("re-link changed the entry: %+v", again)
	}
	files, err := droidListSettingsFiles(home)
	if err != nil {
		t.Fatalf("list: %v", err)
	}
	externals := 0
	for _, file := range files {
		if file.IsExternal {
			externals++
		}
	}
	if externals != 1 {
		t.Fatalf("expected 1 external entry, got %d", externals)
	}

	// Linking rejects bad inputs.
	if _, err := droidLinkSettingsFile(home, filepath.Join(home, "missing.json")); err == nil || !strings.Contains(err.Error(), "does not exist or cannot be read") {
		t.Fatalf("expected missing error, got %v", err)
	}
	txt := filepath.Join(t.TempDir(), "settings.txt")
	if err := os.WriteFile(txt, []byte("{}"), 0o644); err != nil {
		t.Fatalf("write txt: %v", err)
	}
	if _, err := droidLinkSettingsFile(home, txt); err == nil || err.Error() != "Settings file must have a .json extension" {
		t.Fatalf("expected extension error, got %v", err)
	}
	broken := filepath.Join(t.TempDir(), "broken.json")
	if err := os.WriteFile(broken, []byte("{not json"), 0o644); err != nil {
		t.Fatalf("write broken: %v", err)
	}
	if _, err := droidLinkSettingsFile(home, broken); err == nil || !strings.Contains(err.Error(), "is not valid JSON") {
		t.Fatalf("expected invalid-JSON error, got %v", err)
	}

	// The linked file resolves as the launch target until unlinked.
	activePath, err := droidActiveSettingsPath(home)
	if err != nil {
		t.Fatalf("active path: %v", err)
	}
	if !droidPathsEqual(activePath, linked.Path) {
		t.Fatalf("active path %q != linked %q", activePath, linked.Path)
	}

	// Unlinking accepts separator variants (component-wise comparison) and
	// resets the active file to Global without touching the source.
	slashed := strings.ReplaceAll(linked.Path, string(filepath.Separator), "/")
	if err := droidUnlinkSettingsFile(home, slashed); err != nil {
		t.Fatalf("unlink: %v", err)
	}
	if _, err := os.Stat(external); err != nil {
		t.Fatalf("unlink must not delete the source: %v", err)
	}
	active, err := droidGetActiveSettingsFile(home)
	if err != nil {
		t.Fatalf("active after unlink: %v", err)
	}
	if !active.IsGlobal {
		t.Fatalf("expected Global after unlink, got %+v", active)
	}
	if err := droidUnlinkSettingsFile(home, external); err == nil || !strings.Contains(err.Error(), "is not linked") {
		t.Fatalf("expected not-linked error, got %v", err)
	}
}

func TestDroidSettingsLinkUniquifiesDisplayName(t *testing.T) {
	home := droidSettingsTestHome(t)
	if err := os.MkdirAll(filepath.Join(home, ".droidgear", "droid-settings"), 0o755); err != nil {
		t.Fatalf("mkdir managed: %v", err)
	}
	managed := filepath.Join(home, ".droidgear", "droid-settings", "team-settings.json")
	if err := os.WriteFile(managed, []byte("{}"), 0o644); err != nil {
		t.Fatalf("write managed: %v", err)
	}
	external := filepath.Join(t.TempDir(), "team-settings.json")
	if err := os.WriteFile(external, []byte("{}"), 0o644); err != nil {
		t.Fatalf("write external: %v", err)
	}

	linked, err := droidLinkSettingsFile(home, external)
	if err != nil {
		t.Fatalf("link: %v", err)
	}
	if linked.Name != "team-settings-2" {
		t.Fatalf("expected uniquified name, got %q", linked.Name)
	}
}

func TestDroidSettingsActiveFallsBackWhenLinkedFileVanishes(t *testing.T) {
	home := droidSettingsTestHome(t)
	external := filepath.Join(t.TempDir(), "vanish.json")
	if err := os.WriteFile(external, []byte("{}"), 0o644); err != nil {
		t.Fatalf("write external: %v", err)
	}
	if _, err := droidLinkSettingsFile(home, external); err != nil {
		t.Fatalf("link: %v", err)
	}
	if err := os.Remove(external); err != nil {
		t.Fatalf("remove: %v", err)
	}
	active, err := droidGetActiveSettingsFile(home)
	if err != nil {
		t.Fatalf("get active: %v", err)
	}
	if !active.IsGlobal {
		t.Fatalf("expected Global fallback, got %+v", active)
	}
}

// --- settings values --------------------------------------------------------

func TestDroidSettingsValuesDefaultsAndRoundTrip(t *testing.T) {
	isolatedHome(t)
	a := &App{}

	// Defaults when the file is missing (mirrors the Rust getters).
	if v, _ := a.GetDiffMode(); v != "github" {
		t.Errorf("diffMode default = %q", v)
	}
	if v, _ := a.GetTodoDisplayMode(); v != "pinned" {
		t.Errorf("todoDisplayMode default = %q", v)
	}
	if v, _ := a.GetIncludeCoAuthoredByDroid(); v != true {
		t.Errorf("includeCoAuthoredByDroid default = %v", v)
	}
	if v, _ := a.GetShowThinkingInMainView(); v != false {
		t.Errorf("showThinkingInMainView default = %v", v)
	}
	if v, _ := a.GetCloudSessionSync(); v != true {
		t.Errorf("cloudSessionSync default = %v", v)
	}
	if v, _ := a.GetCompactionModelMode(); v != "current-model" {
		t.Errorf("compactionModelMode default = %q", v)
	}
	if v, _ := a.GetCompactionTokenLimit(); v != 200000 {
		t.Errorf("compactionTokenLimit default = %d", v)
	}
	if v, _ := a.GetCompactionTokenLimitPerModel(); len(v) != 0 {
		t.Errorf("per-model default = %#v", v)
	}
	if v, _ := a.GetReasoningEffort(); v != nil {
		t.Errorf("reasoningEffort default = %v", v)
	}
	if v, _ := a.GetMissionModelSettings(); v != (MissionModelSettings{}) {
		t.Errorf("mission settings default = %+v", v)
	}

	// Save → get round trips.
	steps := []struct {
		name  string
		save  func() error
		check func() bool
	}{
		{"diffMode", func() error { return a.SaveDiffMode("unified") }, func() bool { v, _ := a.GetDiffMode(); return v == "unified" }},
		{"todoDisplayMode", func() error { return a.SaveTodoDisplayMode("inline") }, func() bool { v, _ := a.GetTodoDisplayMode(); return v == "inline" }},
		{"includeCoAuthoredByDroid", func() error { return a.SaveIncludeCoAuthoredByDroid(false) }, func() bool { v, _ := a.GetIncludeCoAuthoredByDroid(); return v == false }},
		{"showThinkingInMainView", func() error { return a.SaveShowThinkingInMainView(true) }, func() bool { v, _ := a.GetShowThinkingInMainView(); return v == true }},
		{"cloudSessionSync", func() error { return a.SaveCloudSessionSync(false) }, func() bool { v, _ := a.GetCloudSessionSync(); return v == false }},
		{"reasoningEffort", func() error { return a.SaveReasoningEffort("high") }, func() bool { v, _ := a.GetReasoningEffort(); return v != nil && *v == "high" }},
		{"compactionModelMode", func() error { return a.SaveCompactionModelMode("separate-model") }, func() bool { v, _ := a.GetCompactionModelMode(); return v == "separate-model" }},
		{"compactionTokenLimit", func() error { return a.SaveCompactionTokenLimit(123456) }, func() bool { v, _ := a.GetCompactionTokenLimit(); return v == 123456 }},
		{"compactionTokenLimitPerModel", func() error {
			return a.SaveCompactionTokenLimitPerModel(map[string]int32{"gpt-5": 90000})
		}, func() bool {
			v, _ := a.GetCompactionTokenLimitPerModel()
			return v["gpt-5"] == 90000
		}},
	}
	for _, step := range steps {
		if err := step.save(); err != nil {
			t.Fatalf("%s save: %v", step.name, err)
		}
		if !step.check() {
			t.Fatalf("%s round trip failed", step.name)
		}
	}

	// Mission model settings round-trip; nil fields stay omitted on write.
	if err := a.SaveMissionModelSettings(MissionModelSettings{
		WorkerModel: strPtr("worker-1"),
	}); err != nil {
		t.Fatalf("save mission settings: %v", err)
	}
	mission, err := a.GetMissionModelSettings()
	if err != nil {
		t.Fatalf("get mission settings: %v", err)
	}
	if mission.WorkerModel == nil || *mission.WorkerModel != "worker-1" {
		t.Fatalf("mission round trip: %+v", mission)
	}
	if mission.ValidationWorkerModel != nil {
		t.Fatalf("nil field must stay nil: %+v", mission)
	}
	home, _ := droidHomeDir()
	raw, err := os.ReadFile(filepath.Join(home, ".factory", "settings.json")) // #nosec G304 -- isolated home
	if err != nil {
		t.Fatalf("read settings: %v", err)
	}
	decoded, err := droidDecodeJSONValue(raw)
	if err != nil {
		t.Fatalf("parse settings: %v", err)
	}
	missionRaw := decoded.(map[string]any)["missionModelSettings"].(map[string]any)
	if _, present := missionRaw["validationWorkerModel"]; present {
		t.Fatalf("nil field serialized: %#v", missionRaw)
	}
	if _, present := missionRaw["workerModel"]; !present {
		t.Fatalf("workerModel missing: %#v", missionRaw)
	}
}

func TestDroidSettingsValuesPreserveUnknownFields(t *testing.T) {
	isolatedHome(t)
	a := &App{}

	home, err := droidHomeDir()
	if err != nil {
		t.Fatalf("home: %v", err)
	}
	dir := filepath.Join(home, ".factory")
	if err := os.MkdirAll(dir, 0o755); err != nil {
		t.Fatalf("mkdir: %v", err)
	}
	seed := `{"futureFlag":{"keep":true},"customModels":[{"model":"x"}]}`
	if err := os.WriteFile(filepath.Join(dir, "settings.json"), []byte(seed), 0o644); err != nil {
		t.Fatalf("seed: %v", err)
	}

	if err := a.SaveDiffMode("unified"); err != nil {
		t.Fatalf("SaveDiffMode: %v", err)
	}
	raw, err := os.ReadFile(filepath.Join(dir, "settings.json")) // #nosec G304 -- isolated home
	if err != nil {
		t.Fatalf("read: %v", err)
	}
	decoded, err := droidDecodeJSONValue(raw)
	if err != nil {
		t.Fatalf("parse: %v", err)
	}
	obj := decoded.(map[string]any)
	keep, ok := obj["futureFlag"].(map[string]any)
	if !ok {
		t.Fatalf("futureFlag lost: %#v", obj)
	}
	if keep["keep"] != true {
		t.Fatalf("futureFlag value changed: %#v", keep)
	}
	models, ok := obj["customModels"].([]any)
	if !ok || len(models) != 1 {
		t.Fatalf("customModels lost: %#v", obj["customModels"])
	}
	if obj["diffMode"] != "unified" {
		t.Fatalf("diffMode not saved: %#v", obj["diffMode"])
	}
}

func TestDroidSettingsValuesParseErrorContract(t *testing.T) {
	isolatedHome(t)
	a := &App{}

	home, err := droidHomeDir()
	if err != nil {
		t.Fatalf("home: %v", err)
	}
	dir := filepath.Join(home, ".factory")
	if err := os.MkdirAll(dir, 0o755); err != nil {
		t.Fatalf("mkdir: %v", err)
	}
	broken := filepath.Join(dir, "settings.json")
	if err := os.WriteFile(broken, []byte("{not json"), 0o644); err != nil {
		t.Fatalf("seed: %v", err)
	}

	// Getters degrade to defaults on parse errors.
	if v, _ := a.GetDiffMode(); v != "github" {
		t.Fatalf("diffMode on broken config = %q", v)
	}
	if v, _ := a.GetCloudSessionSync(); v != true {
		t.Fatalf("cloudSessionSync on broken config = %v", v)
	}

	// Savers surface the CONFIG_PARSE_ERROR prefix the frontend matches on.
	err = a.SaveDiffMode("unified")
	if err == nil {
		t.Fatal("expected save error on broken config")
	}
	if !strings.HasPrefix(err.Error(), "CONFIG_PARSE_ERROR:") {
		t.Fatalf("missing CONFIG_PARSE_ERROR prefix: %v", err)
	}
	data, readErr := os.ReadFile(broken) // #nosec G304 -- isolated home
	if readErr != nil {
		t.Fatalf("read: %v", readErr)
	}
	if string(data) != "{not json" {
		t.Fatalf("failed save must not touch the file: %q", data)
	}
}

func TestDroidSettingsValuesReasoningEffortFallsBackToSessionDefaults(t *testing.T) {
	isolatedHome(t)
	a := &App{}

	home, err := droidHomeDir()
	if err != nil {
		t.Fatalf("home: %v", err)
	}
	dir := filepath.Join(home, ".factory")
	if err := os.MkdirAll(dir, 0o755); err != nil {
		t.Fatalf("mkdir: %v", err)
	}
	seed := `{"sessionDefaultSettings":{"reasoningEffort":"low"}}`
	if err := os.WriteFile(filepath.Join(dir, "settings.json"), []byte(seed), 0o644); err != nil {
		t.Fatalf("seed: %v", err)
	}

	value, err := a.GetReasoningEffort()
	if err != nil {
		t.Fatalf("GetReasoningEffort: %v", err)
	}
	if value == nil || *value != "low" {
		t.Fatalf("expected fallback to session defaults, got %v", value)
	}

	// A top-level value wins over the fallback.
	if err := a.SaveReasoningEffort("xhigh"); err != nil {
		t.Fatalf("SaveReasoningEffort: %v", err)
	}
	value, _ = a.GetReasoningEffort()
	if value == nil || *value != "xhigh" {
		t.Fatalf("top-level value not preferred: %v", value)
	}
}

// --- launch -----------------------------------------------------------------

func TestDroidLaunchCommandAndRunPlan(t *testing.T) {
	home := droidSettingsTestHome(t)
	global := filepath.Join(home, ".factory", "settings.json")

	// Global → plain `droid`.
	plan := droidBuildRunPlanFromSettingsPath(home, global, DroidRunPreferences{})
	if plan.program != "droid" || len(plan.args) != 0 || plan.settingsPath != nil {
		t.Fatalf("global plan wrong: %+v", plan)
	}
	if len(plan.env) != 1 || plan.env[0] != [2]string{"FACTORY_DROID_AUTO_UPDATE_ENABLED", "0"} {
		t.Fatalf("default env override missing: %#v", plan.env)
	}
	if len(plan.unsetEnv) != 1 || plan.unsetEnv[0] != "ANTHROPIC_AUTH_TOKEN" {
		t.Fatalf("default unset missing: %#v", plan.unsetEnv)
	}

	// Explicit run policy disables both overrides.
	off := false
	plan = droidBuildRunPlanFromSettingsPath(home, global, DroidRunPreferences{
		DisableAutoUpdateEnv:    &off,
		UnsetAnthropicAuthToken: &off,
	})
	if len(plan.env) != 0 || len(plan.unsetEnv) != 0 {
		t.Fatalf("policy overrides ignored: %+v", plan)
	}

	// A custom profile is passed to Droid's native --settings flag.
	custom := filepath.Join(home, ".droidgear", "droid-settings", "work.json")
	plan = droidBuildRunPlanFromSettingsPath(home, custom, DroidRunPreferences{})
	if len(plan.args) != 2 || plan.args[0] != "--settings" || plan.args[1] != custom {
		t.Fatalf("custom plan wrong: %+v", plan)
	}
	if plan.settingsPath == nil || *plan.settingsPath != custom {
		t.Fatalf("settingsPath not recorded: %+v", plan.settingsPath)
	}
}

func TestGetDroidLaunchCommand(t *testing.T) {
	home := isolatedHome(t)
	if err := os.MkdirAll(filepath.Join(home, ".factory"), 0o755); err != nil {
		t.Fatalf("mkdir: %v", err)
	}
	a := &App{}

	command, err := a.GetDroidLaunchCommand()
	if err != nil {
		t.Fatalf("GetDroidLaunchCommand: %v", err)
	}
	global := filepath.Join(home, ".factory", "settings.json")
	if command[0] != "droid" || command[1] != global {
		t.Fatalf("global command wrong: %q", command)
	}

	profile, err := a.CreateDroidSettingsFile("work", false)
	if err != nil {
		t.Fatalf("CreateDroidSettingsFile: %v", err)
	}
	command, err = a.GetDroidLaunchCommand()
	if err != nil {
		t.Fatalf("GetDroidLaunchCommand: %v", err)
	}
	want := `droid --settings "` + profile.Path + `"`
	if command[0] != want || command[1] != profile.Path {
		t.Fatalf("profile command wrong: got %q, want %q", command[0], want)
	}
}

func TestSetActiveDroidSettingsFileAcceptsNullPointer(t *testing.T) {
	isolatedHome(t)
	a := &App{}

	if _, err := a.CreateDroidSettingsFile("work", false); err != nil {
		t.Fatalf("create: %v", err)
	}
	back, err := a.SetActiveDroidSettingsFile(nil)
	if err != nil {
		t.Fatalf("set nil: %v", err)
	}
	if !back.IsGlobal {
		t.Fatalf("nil must switch to Global, got %+v", back)
	}
	name := "work"
	active, err := a.SetActiveDroidSettingsFile(&name)
	if err != nil {
		t.Fatalf("set name: %v", err)
	}
	if active.Name != "work" || !active.IsActive {
		t.Fatalf("unexpected active: %+v", active)
	}
}
