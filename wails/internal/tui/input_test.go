package tui

import (
	"context"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"droidgear-wails/internal/service/codex"
	"droidgear-wails/internal/service/droid"
	"droidgear-wails/internal/service/mcp"
	"droidgear-wails/internal/service/preferences"
	"droidgear-wails/internal/testutil"
)

// --- input primitive: list screens -----------------------------------------

func TestSpecRenameThroughPromptAndPreview(t *testing.T) {
	home := testutil.IsolatedHome(t)
	specPath := filepath.Join(home, ".factory", "specs", "plan.md")
	testutil.WriteTestFile(t, specPath, "# plan\n")

	screen, err := newListScreen(context.Background(), SpecsSource())
	if err != nil {
		t.Fatalf("newListScreen: %v", err)
	}
	if !strings.Contains(screen.Status(), "n rename") {
		t.Fatalf("status = %q, want the rename hint", screen.Status())
	}

	next, _ := screen.Update(keyMsg("n"))
	prompt, ok := next.(*promptScreen)
	if !ok {
		t.Fatalf("n returned %T, want the prompt screen", next)
	}
	// The prompt starts prefilled with the current name: clear it first.
	for i := 0; i < len("plan"); i++ {
		next, _ = prompt.Update(keyMsg("backspace"))
		prompt = next.(*promptScreen)
	}
	for _, r := range "roadmap" {
		next, _ = prompt.Update(keyMsg(string(r)))
		prompt = next.(*promptScreen)
	}
	next, _ = prompt.Update(keyMsg("enter"))
	flow, ok := next.(*writeFlow)
	if !ok {
		t.Fatalf("enter returned %T, want the preview flow", next)
	}
	next, _ = flow.Update(keyMsg("y"))
	if !strings.Contains(next.Status(), "applied") {
		t.Fatalf("status = %q, want applied", next.Status())
	}
	next, _ = next.Update(keyMsg("enter"))
	refreshed, ok := next.(*listScreen)
	if !ok {
		t.Fatalf("enter returned %T, want the refreshed list", next)
	}
	if view := refreshed.View(100, 24); !strings.Contains(view, "roadmap.md") {
		t.Fatalf("list not refreshed after rename:\n%s", view)
	}
	if _, err := os.Stat(filepath.Join(home, ".factory", "specs", "roadmap.md")); err != nil {
		t.Fatalf("renamed file missing: %v", err)
	}
}

func TestProfileCreateFromFileThroughPrompt(t *testing.T) {
	home := testutil.IsolatedHome(t)
	profileFile := filepath.Join(home, "profile.json")
	testutil.WriteTestFile(t, profileFile,
		`{"id":"from-file","name":"From File","modelProvider":"openai"}`)

	screen, err := newListScreen(context.Background(), CodexSource())
	if err != nil {
		t.Fatalf("newListScreen: %v", err)
	}
	if !strings.Contains(screen.Status(), "n new from file") {
		t.Fatalf("status = %q, want the create hint", screen.Status())
	}

	next, _ := screen.Update(keyMsg("n"))
	prompt, ok := next.(*promptScreen)
	if !ok {
		t.Fatalf("n returned %T, want the prompt screen", next)
	}
	for _, r := range profileFile {
		next, _ = prompt.Update(keyMsg(string(r)))
		prompt = next.(*promptScreen)
	}
	next, _ = prompt.Update(keyMsg("enter"))
	flow, ok := next.(*writeFlow)
	if !ok {
		t.Fatalf("enter returned %T, want the preview flow", next)
	}
	next, _ = flow.Update(keyMsg("y"))
	if !strings.Contains(next.Status(), "applied") {
		t.Fatalf("status = %q, want applied", next.Status())
	}

	profiles, err := codex.ListProfiles()
	if err != nil {
		t.Fatalf("ListProfiles: %v", err)
	}
	found := false
	for _, profile := range profiles {
		if profile.ID == "from-file" {
			found = true
		}
	}
	if !found {
		t.Fatalf("profile not created: %+v", profiles)
	}
}

func TestMcpBaizhiPresetAction(t *testing.T) {
	testutil.IsolatedHome(t)
	// A row must exist for the key to dispatch (actions are row-scoped).
	if err := mcp.SaveServer(mcp.McpServer{
		Name: "s1",
		Config: mcp.McpServerConfig{
			Type: mcp.McpServerTypeHTTP,
			URL:  strPtrOrNil("https://example.test/mcp"),
		},
	}); err != nil {
		t.Fatalf("SaveServer: %v", err)
	}
	screen, err := newListScreen(context.Background(), McpSource())
	if err != nil {
		t.Fatalf("newListScreen: %v", err)
	}
	if !strings.Contains(screen.Status(), "b baizhi preset") {
		t.Fatalf("status = %q, want the preset hint", screen.Status())
	}
	next, _ := screen.Update(keyMsg("b"))
	flow, ok := next.(*writeFlow)
	if !ok {
		t.Fatalf("b returned %T, want the preview flow", next)
	}
	next, _ = flow.Update(keyMsg("y"))
	if !strings.Contains(next.Status(), "applied") {
		t.Fatalf("status = %q, want applied", next.Status())
	}
}

// --- input primitive: kv screens --------------------------------------------

func TestPreferencesLanguageCycleApplies(t *testing.T) {
	home := testutil.IsolatedHome(t)
	t.Setenv("APPDATA", home)
	t.Setenv("XDG_DATA_HOME", filepath.Join(home, "xdg-data"))

	screen, err := newSettingsScreen(context.Background())
	if err != nil {
		t.Fatalf("newSettingsScreen: %v", err)
	}
	next, _ := screen.Update(keyMsg("l"))
	flow, ok := next.(*writeFlow)
	if !ok {
		t.Fatalf("l returned %T, want the preview flow", next)
	}
	next, _ = flow.Update(keyMsg("y"))
	if !strings.Contains(next.Status(), "applied") {
		t.Fatalf("status = %q, want applied", next.Status())
	}

	prefs, err := preferences.Load()
	if err != nil {
		t.Fatalf("Load: %v", err)
	}
	if prefs.Language == nil || *prefs.Language != "en" {
		t.Fatalf("language = %v, want en", prefs.Language)
	}
}

func TestDroidSettingsFileUseAndValueCycle(t *testing.T) {
	testutil.IsolatedHome(t)
	ctx := context.Background()

	// Seed a managed settings file and make it active through the row action.
	if _, err := droid.CreateSettingsFile("work", false); err != nil {
		t.Fatalf("CreateSettingsFile: %v", err)
	}
	if _, err := droid.SetActiveSettingsFile(strPtrOrNil("work")); err != nil {
		t.Fatalf("SetActiveSettingsFile: %v", err)
	}

	screen, err := newDroidScreen(ctx)
	if err != nil {
		t.Fatalf("newDroidScreen: %v", err)
	}
	// Settings is the default section — its status carries the action hints.
	if got := screen.Status(); !strings.Contains(got, "u use file") || !strings.Contains(got, "d cycle diff mode") {
		t.Fatalf("status = %q, want the settings action hints", got)
	}
	if view := screen.View(100, 40); !strings.Contains(view, "work") {
		t.Fatalf("settings section missing the files rows:\n%s", view)
	}

	// 'd' cycles diff mode github → unified through the flow (cycles are
	// row-independent, unlike the file actions). Keys go through the
	// sectioned shell to the flow inside.
	before, err := droid.GetDiffMode()
	if err != nil {
		t.Fatalf("GetDiffMode: %v", err)
	}
	screen.Update(keyMsg("d"))
	if got := screen.Status(); !strings.Contains(got, "preview") {
		t.Fatalf("status = %q, want the preview stage", got)
	}
	screen.Update(keyMsg("y"))
	if got := screen.Status(); !strings.Contains(got, "applied") {
		t.Fatalf("status = %q, want applied", got)
	}
	after, err := droid.GetDiffMode()
	if err != nil {
		t.Fatalf("GetDiffMode after cycle: %v", err)
	}
	if after == before {
		t.Fatalf("diff mode did not cycle (%s)", before)
	}
}

func TestDroidMissionWorkerModelThroughPrompt(t *testing.T) {
	testutil.IsolatedHome(t)
	ctx := context.Background()

	screen, err := newDroidScreen(ctx)
	if err != nil {
		t.Fatalf("newDroidScreen: %v", err)
	}
	// Section 2 is Missions (Settings is the default section).
	screen.Update(keyMsg("tab"))
	if got := screen.Status(); !strings.Contains(got, "w worker model") {
		t.Fatalf("status = %q, want the mission hints", got)
	}

	// The prompt/flow live inside the sectioned shell: keep driving `screen`
	// and assert through the status line.
	screen.Update(keyMsg("w"))
	if got := screen.Status(); !strings.Contains(got, "enter save") {
		t.Fatalf("status = %q, want the prompt stage", got)
	}
	for _, r := range "worker-model-1" {
		screen.Update(keyMsg(string(r)))
	}
	screen.Update(keyMsg("enter"))
	if got := screen.Status(); !strings.Contains(got, "preview") {
		t.Fatalf("status = %q, want the preview stage", got)
	}
	screen.Update(keyMsg("y"))
	if got := screen.Status(); !strings.Contains(got, "applied") {
		t.Fatalf("status = %q, want applied", got)
	}

	settings, err := droid.GetMissionModelSettings()
	if err != nil {
		t.Fatalf("GetMissionModelSettings: %v", err)
	}
	if settings.WorkerModel == nil || *settings.WorkerModel != "worker-model-1" {
		t.Fatalf("worker model = %v", settings.WorkerModel)
	}
}
