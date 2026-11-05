package tui

import (
	"context"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"droidgear-wails/internal/core"
	"droidgear-wails/internal/service/claude"
	"droidgear-wails/internal/service/preferences"
	"droidgear-wails/internal/testutil"
	"github.com/charmbracelet/bubbletea"
)

func keyMsg(name string) tea.KeyMsg {
	switch name {
	case "up":
		return tea.KeyMsg{Type: tea.KeyUp}
	case "down":
		return tea.KeyMsg{Type: tea.KeyDown}
	case "enter":
		return tea.KeyMsg{Type: tea.KeyEnter}
	case "esc":
		return tea.KeyMsg{Type: tea.KeyEsc}
	case "tab":
		return tea.KeyMsg{Type: tea.KeyTab}
	case "ctrl+c":
		return tea.KeyMsg{Type: tea.KeyCtrlC}
	default:
		return tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune(name)}
	}
}

func step(t *testing.T, m Model, msg tea.Msg) Model {
	t.Helper()
	next, _ := m.Update(msg)
	model, ok := next.(Model)
	if !ok {
		t.Fatalf("Update returned %T, want Model", next)
	}
	return model
}

func TestSidebarNavigationAndFocus(t *testing.T) {
	m := New(context.Background())

	if got := m.SelectedModule(); got != "droid" {
		t.Fatalf("initial selection = %q, want droid", got)
	}
	if m.Focused() != focusSidebar {
		t.Fatalf("initial focus = %v, want sidebar", m.Focused())
	}

	m = step(t, m, keyMsg("down"))
	if got := m.SelectedModule(); got != "channels" {
		t.Fatalf("after down selection = %q, want channels", got)
	}

	// Bottom of the list clamps instead of wrapping.
	for i := 0; i < len(DefaultModules()); i++ {
		m = step(t, m, keyMsg("down"))
	}
	if got, last := m.SelectedModule(), DefaultModules()[len(DefaultModules())-1].Key; got != last {
		t.Fatalf("clamped selection = %q, want %q", got, last)
	}

	m = step(t, m, keyMsg("enter"))
	if m.Focused() != focusContent {
		t.Fatalf("after enter focus = %v, want content", m.Focused())
	}
	if m.CurrentScreen() == nil {
		t.Fatal("enter did not open a screen")
	}

	m = step(t, m, keyMsg("esc"))
	if m.Focused() != focusSidebar {
		t.Fatalf("after esc focus = %v, want sidebar", m.Focused())
	}
}

func TestQuitKeys(t *testing.T) {
	m := New(context.Background())

	_, cmd := m.Update(keyMsg("ctrl+c"))
	if cmd == nil {
		t.Fatal("ctrl+c in the sidebar produced no command")
	}
	if msg := cmd(); msg != tea.Quit() {
		t.Fatalf("ctrl+c command = %v, want tea.Quit", msg)
	}

	// q quits from the sidebar…
	_, cmd = m.Update(keyMsg("q"))
	if cmd == nil {
		t.Fatal("q in the sidebar produced no command")
	}

	// …but only goes back from a screen: content owns q.
	m.SetScreen(&fakeScreen{})
	content := step(t, m, keyMsg("q"))
	if content.Focused() != focusSidebar {
		t.Fatalf("q in content focus = %v, want sidebar", content.Focused())
	}
}

func TestTerminalTooSmall(t *testing.T) {
	m := New(context.Background())
	m.width, m.height = 40, 10
	view := m.View()
	if !strings.Contains(view, "terminal too small") {
		t.Fatalf("view = %q, want a too-small notice", view)
	}
}

func TestPathsScreenRendersEffectivePaths(t *testing.T) {
	testutil.IsolatedHome(t)

	screen, err := newPathsScreen(context.Background())
	if err != nil {
		t.Fatalf("newPathsScreen: %v", err)
	}
	view := screen.View(100, 24)

	for _, want := range []string{"Configuration paths", "KEY", "factory", "codex"} {
		if !strings.Contains(view, want) {
			t.Fatalf("paths view missing %q:\n%s", want, view)
		}
	}
}

func TestProfilesScreenListsAndSelects(t *testing.T) {
	source := &fakeSource{
		rows: []ListRow{
			{ID: "a", Label: "Alpha"},
			{ID: "b", Label: "Beta", Mark: "●"},
		},
		summary: "Fake · 2 profiles · active: Beta · r toggle secrets",
		detail:  `{"id":"b","name":"Beta","apiKey":"sk-super-secret-value"}`,
	}

	screen, err := newListScreen(context.Background(), source)
	if err != nil {
		t.Fatalf("newListScreen: %v", err)
	}

	if got := screen.Status(); !strings.Contains(got, "active: Beta") {
		t.Fatalf("status = %q, want the active profile", got)
	}
	view := screen.View(100, 24)
	for _, want := range []string{"Alpha", "Beta"} {
		if !strings.Contains(view, want) {
			t.Fatalf("profiles view missing %q:\n%s", want, view)
		}
	}
	// Secrets are redacted by default (parity §5.3).
	if strings.Contains(view, "sk-super-secret-value") {
		t.Fatalf("profile view leaked the api key:\n%s", view)
	}
	if !strings.Contains(view, "****") {
		t.Fatalf("profile view shows no redaction marker:\n%s", view)
	}
}

func TestProfilesScreenRevealToggle(t *testing.T) {
	source := &fakeSource{
		rows:   []ListRow{{ID: "a", Label: "Alpha"}},
		detail: `{"id":"a","apiKey":"sk-super-secret-value"}`,
	}
	screen, err := newListScreen(context.Background(), source)
	if err != nil {
		t.Fatalf("newListScreen: %v", err)
	}

	next, _ := screen.Update(keyMsg("r"))
	screen = next
	view := screen.View(100, 24)
	if !strings.Contains(view, "sk-super-secret-value") {
		t.Fatalf("reveal did not show the secret:\n%s", view)
	}
}

func TestProfileSourceMarksActiveProfile(t *testing.T) {
	active := "b"
	source := &profileSource{
		title: "Fake",
		list: func() ([]ListRow, error) {
			return []ListRow{
				{ID: "a", Label: "Alpha"},
				{ID: "b", Label: "Beta"},
			}, nil
		},
		active: func() (*string, error) { return &active, nil },
		get:    func(string) ([]byte, error) { return []byte("{}"), nil },
	}

	rows, err := source.Load(context.Background())
	if err != nil {
		t.Fatalf("Load: %v", err)
	}
	if rows[1].Mark != "●" {
		t.Fatalf("active row mark = %q, want ●", rows[1].Mark)
	}
	if rows[0].Mark != "" {
		t.Fatalf("inactive row mark = %q, want empty", rows[0].Mark)
	}
	if got := source.Summary(rows); !strings.Contains(got, "active: Beta") {
		t.Fatalf("summary = %q, want the active label", got)
	}
}

func TestChannelsAndExportScreensOpenOnAnEmptyHome(t *testing.T) {
	testutil.IsolatedHome(t)

	openers := map[string]func(context.Context) (Screen, error){
		"channels": func(ctx context.Context) (Screen, error) {
			return newListScreen(ctx, ChannelsSource())
		},
		"export": func(ctx context.Context) (Screen, error) {
			return newListScreen(ctx, ExportSource())
		},
		"copilot": func(ctx context.Context) (Screen, error) {
			return newListScreen(ctx, CopilotSource())
		},
		"codex-auth": func(ctx context.Context) (Screen, error) {
			return newListScreen(ctx, CodexAuthSource())
		},
		"claude": func(ctx context.Context) (Screen, error) {
			return newListScreen(ctx, ClaudeSource())
		},
	}
	for name, open := range openers {
		screen, err := open(context.Background())
		if err != nil {
			t.Fatalf("%s: %v", name, err)
		}
		view := screen.View(100, 24)
		if strings.Contains(view, "error") {
			t.Fatalf("%s screen reported an error on an empty home: %s", name, view)
		}
		if !strings.Contains(view, screen.Title()) {
			t.Fatalf("%s screen is missing its title: %s", name, view)
		}
	}
}

func TestCodexAuthScreenListsProfiles(t *testing.T) {
	testutil.IsolatedHome(t)

	dir, err := core.DroidGearDir()
	if err != nil {
		t.Fatalf("DroidGearDir: %v", err)
	}
	manifest := filepath.Join(dir, "auth-profiles", "codex", "profiles.json")
	if err := os.MkdirAll(filepath.Dir(manifest), 0o755); err != nil {
		t.Fatalf("mkdir: %v", err)
	}
	const body = `{"active":"prod","profiles":[` +
		`{"name":"prod","label":"Prod BYOK","createdAt":"2026-01-01T00:00:00Z","isOfficial":false},` +
		`{"name":"official","label":"Official","createdAt":"2026-01-02T00:00:00Z","isOfficial":true}]}`
	if err := os.WriteFile(manifest, []byte(body), 0o600); err != nil {
		t.Fatalf("write manifest: %v", err)
	}

	screen, err := newListScreen(context.Background(), CodexAuthSource())
	if err != nil {
		t.Fatalf("newListScreen: %v", err)
	}
	if got := screen.Status(); !strings.Contains(got, "active: Prod BYOK") || !strings.Contains(got, "1 official") {
		t.Fatalf("status = %q, want the active profile and the official count", got)
	}
	view := screen.View(100, 24)
	for _, want := range []string{"Prod BYOK", "Official  [official]"} {
		if !strings.Contains(view, want) {
			t.Fatalf("codex auth view missing %q:\n%s", want, view)
		}
	}
}

func TestSettingsScreenRendersPreferences(t *testing.T) {
	home := testutil.IsolatedHome(t)
	// preferences.json lives in the platform app-data dir, not HOME; point
	// APPDATA at the throwaway home so Save never touches the developer's
	// real profile.
	t.Setenv("APPDATA", home)
	t.Setenv("XDG_DATA_HOME", filepath.Join(home, "xdg-data"))

	lang := "zh-CN"
	if err := preferences.Save(preferences.AppPreferences{
		Theme:    "dark",
		Language: &lang,
	}); err != nil {
		t.Fatalf("Save: %v", err)
	}

	screen, err := newSettingsScreen(context.Background())
	if err != nil {
		t.Fatalf("newSettingsScreen: %v", err)
	}
	view := screen.View(100, 24)
	for _, want := range []string{"Preferences", "theme", "dark", "language", "zh-CN", "[Appearance]"} {
		if !strings.Contains(view, want) {
			t.Fatalf("settings view missing %q:\n%s", want, view)
		}
	}
}

func TestClaudeScreenListsSettingsFiles(t *testing.T) {
	testutil.IsolatedHome(t)

	if _, err := claude.CreateSettingsFile("work", false); err != nil {
		t.Fatalf("CreateSettingsFile: %v", err)
	}
	if _, err := claude.SetActiveSettingsFile(core.StrPtr("work")); err != nil {
		t.Fatalf("SetActiveSettingsFile: %v", err)
	}

	screen, err := newListScreen(context.Background(), ClaudeSource())
	if err != nil {
		t.Fatalf("newListScreen: %v", err)
	}
	if got := screen.Status(); !strings.Contains(got, "active: work") {
		t.Fatalf("status = %q, want the active settings file", got)
	}
	view := screen.View(100, 24)
	for _, want := range []string{"Claude Settings", "Global", "work"} {
		if !strings.Contains(view, want) {
			t.Fatalf("claude view missing %q:\n%s", want, view)
		}
	}
}

func TestDroidScreenSectionsSwitch(t *testing.T) {
	testutil.IsolatedHome(t)

	screen, err := newDroidScreen(context.Background())
	if err != nil {
		t.Fatalf("newDroidScreen: %v", err)
	}
	if got := screen.Title(); got != "Droid" {
		t.Fatalf("title = %q, want Droid", got)
	}
	view := screen.View(100, 30)
	for _, want := range []string{"Droid Settings", "diff mode"} {
		if !strings.Contains(view, want) {
			t.Fatalf("droid settings section missing %q:\n%s", want, view)
		}
	}

	next, _ := screen.Update(keyMsg("tab"))
	screen = next
	if view = screen.View(100, 30); !strings.Contains(view, "Droid Missions") {
		t.Fatalf("tab did not reach the Missions section:\n%s", view)
	}

	// Number keys jump straight to a section (3 = Trusted, 7 = Models).
	next, _ = screen.Update(keyMsg("3"))
	screen = next
	if view = screen.View(100, 30); !strings.Contains(view, "Trusted Folders") {
		t.Fatalf("key 3 did not reach the Trusted section:\n%s", view)
	}
	next, _ = screen.Update(keyMsg("7"))
	screen = next
	if view = screen.View(100, 30); !strings.Contains(view, "Models") {
		t.Fatalf("key 7 did not reach the Models section:\n%s", view)
	}
	if got := screen.Status(); !strings.Contains(got, "7/9") {
		t.Fatalf("status = %q, want the 7/9 section counter", got)
	}
}

func TestDshScreenSectionsSwitch(t *testing.T) {
	testutil.DshTestHome(t)

	screen, err := newDshScreen(context.Background())
	if err != nil {
		t.Fatalf("newDshScreen: %v", err)
	}
	view := screen.View(100, 30)
	if !strings.Contains(view, "Dsh Desktop") {
		t.Fatalf("desktop section missing:\n%s", view)
	}
	if got := screen.Status(); !strings.Contains(got, "1/3") {
		t.Fatalf("status = %q, want the 1/3 section counter", got)
	}

	next, _ := screen.Update(keyMsg("tab"))
	screen = next
	if got := screen.Status(); !strings.Contains(got, "2/3") {
		t.Fatalf("status after tab = %q, want the 2/3 section counter", got)
	}
}

func TestSessionsSourceListsGenericSessions(t *testing.T) {
	home := testutil.IsolatedHome(t)
	dir := testutil.SessionsFixtureDir(t, home, "-work-demo")
	testutil.SessionsWriteSession(t, dir, "sess-1", testutil.SessionsFixtureJSONL, "")

	screen, err := newListScreen(context.Background(), DroidSessionsSource())
	if err != nil {
		t.Fatalf("newListScreen: %v", err)
	}
	if got := screen.Status(); !strings.Contains(got, "1 sessions") {
		t.Fatalf("status = %q, want the session count", got)
	}
	view := screen.View(100, 24)
	if !strings.Contains(view, "Hello World") {
		t.Fatalf("sessions view missing the session title:\n%s", view)
	}
}

func TestCodexScreenExposesProvidersAuthSessions(t *testing.T) {
	testutil.IsolatedHome(t)

	screen, err := newCodexScreen(context.Background())
	if err != nil {
		t.Fatalf("newCodexScreen: %v", err)
	}
	if got := screen.Status(); !strings.Contains(got, "1/3") {
		t.Fatalf("status = %q, want the 1/3 section counter", got)
	}
	next, _ := screen.Update(keyMsg("tab"))
	screen = next
	next, _ = screen.Update(keyMsg("tab"))
	screen = next
	if got := screen.Status(); !strings.Contains(got, "3/3") || !strings.Contains(got, "Codex Sessions") {
		t.Fatalf("status = %q, want the sessions section", got)
	}
}

func TestSectionedScreenPassesKeysToNestedFlow(t *testing.T) {
	testutil.IsolatedHome(t)
	// One saved auth profile so the section has a row to act on.
	dir, err := core.DroidGearDir()
	if err != nil {
		t.Fatalf("DroidGearDir: %v", err)
	}
	manifest := filepath.Join(dir, "auth-profiles", "codex", "profiles.json")
	if err := os.MkdirAll(filepath.Dir(manifest), 0o755); err != nil {
		t.Fatalf("mkdir: %v", err)
	}
	body := `{"active":null,"profiles":[` +
		`{"name":"prod","label":"Prod","createdAt":"2026-01-01T00:00:00Z","isOfficial":false}]}`
	if err := os.WriteFile(manifest, []byte(body), 0o600); err != nil {
		t.Fatalf("write manifest: %v", err)
	}

	screen, err := newCodexScreen(context.Background())
	if err != nil {
		t.Fatalf("newCodexScreen: %v", err)
	}
	// Tab to the Auth section and trigger the switch action.
	next, _ := screen.Update(keyMsg("tab"))
	screen = next
	next, _ = screen.Update(keyMsg("a"))
	screen = next
	if got := screen.Status(); !strings.Contains(got, "preview") {
		t.Fatalf("status = %q, want the nested preview flow", got)
	}
	// esc is consumed by the flow, not by the root: it returns to the Auth
	// section and the sectioned shell keeps the keys afterwards.
	next, _ = screen.Update(keyMsg("esc"))
	screen = next
	if got := screen.Status(); !strings.Contains(got, "tab sections") {
		t.Fatalf("status = %q, want back in the section shell", got)
	}
}

func TestDefaultModulesMatchParityMatrix(t *testing.T) {
	// The P0 set is deliberately small; this pins the module keys so the
	// parity matrix (docs/developer/tui-parity.zh.md) and the sidebar cannot
	// drift apart silently.
	want := []string{"droid", "channels", "codex", "claude", "hermes", "pi", "omp", "copilot", "dsh", "paths", "settings"}
	modules := DefaultModules()
	if len(modules) != len(want) {
		t.Fatalf("got %d modules, want %d", len(modules), len(want))
	}
	for i, module := range modules {
		if module.Key != want[i] {
			t.Fatalf("module[%d] = %q, want %q", i, module.Key, want[i])
		}
		if module.Open == nil {
			t.Fatalf("module %q has no Open", module.Key)
		}
	}
}

// --- test doubles ----------------------------------------------------------

type fakeSource struct {
	rows    []ListRow
	detail  string
	err     error
	summary string
}

func (f *fakeSource) Title() string { return "Fake" }

func (f *fakeSource) Load(context.Context) ([]ListRow, error) {
	return f.rows, f.err
}

func (f *fakeSource) Detail(context.Context, string) ([]byte, error) {
	return []byte(f.detail), f.err
}

func (f *fakeSource) Summary(rows []ListRow) string {
	if f.summary != "" {
		return f.summary
	}
	return fmt.Sprintf("Fake · %d rows", len(rows))
}

// fakeScreen is a no-op pane used to exercise the root model's focus rules.
type fakeScreen struct{}

func (*fakeScreen) Title() string                          { return "Fake" }
func (s *fakeScreen) Update(msg tea.Msg) (Screen, tea.Cmd) { return s, nil }
func (*fakeScreen) View(width, height int) string          { return "fake" }
func (*fakeScreen) Status() string                         { return "" }
