package tui

import (
	"context"
	"strings"
	"testing"

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

	if got := m.SelectedModule(); got != "paths" {
		t.Fatalf("initial selection = %q, want paths", got)
	}
	if m.Focused() != focusSidebar {
		t.Fatalf("initial focus = %v, want sidebar", m.Focused())
	}

	m = step(t, m, keyMsg("down"))
	if got := m.SelectedModule(); got != "codex" {
		t.Fatalf("after down selection = %q, want codex", got)
	}

	// Bottom of the list clamps instead of wrapping.
	m = step(t, m, keyMsg("down"))
	m = step(t, m, keyMsg("down"))
	m = step(t, m, keyMsg("down"))
	m = step(t, m, keyMsg("down"))
	if got := m.SelectedModule(); got != "omp" {
		t.Fatalf("clamped selection = %q, want omp", got)
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
	domain := &fakeDomain{
		rows: []ProfileRow{
			{ID: "a", Name: "Alpha"},
			{ID: "b", Name: "Beta"},
		},
		active: "b",
		detail: `{"id":"b","name":"Beta","apiKey":"sk-super-secret-value"}`,
	}

	screen, err := newProfilesScreen(context.Background(), domain)
	if err != nil {
		t.Fatalf("newProfilesScreen: %v", err)
	}

	if got := screen.Status(); !strings.Contains(got, "active: b") {
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
	domain := &fakeDomain{
		rows:   []ProfileRow{{ID: "a", Name: "Alpha"}},
		detail: `{"id":"a","apiKey":"sk-super-secret-value"}`,
	}
	screen, err := newProfilesScreen(context.Background(), domain)
	if err != nil {
		t.Fatalf("newProfilesScreen: %v", err)
	}

	next, _ := screen.Update(keyMsg("r"))
	screen = next
	view := screen.View(100, 24)
	if !strings.Contains(view, "sk-super-secret-value") {
		t.Fatalf("reveal did not show the secret:\n%s", view)
	}
}

func TestDefaultModulesMatchParityMatrix(t *testing.T) {
	// The P0 set is deliberately small; this pins the module keys so the
	// parity matrix (docs/developer/tui-parity.zh.md) and the sidebar cannot
	// drift apart silently.
	want := []string{"paths", "codex", "hermes", "pi", "omp"}
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

type fakeDomain struct {
	rows   []ProfileRow
	active string
	detail string
	err    error
}

func (f *fakeDomain) Key() string   { return "fake" }
func (f *fakeDomain) Title() string { return "Fake" }

func (f *fakeDomain) List(context.Context) ([]ProfileRow, error) {
	return f.rows, f.err
}

func (f *fakeDomain) ActiveID(context.Context) (*string, error) {
	if f.active == "" {
		return nil, nil
	}
	active := f.active
	return &active, nil
}

func (f *fakeDomain) Detail(context.Context, string) ([]byte, error) {
	return []byte(f.detail), f.err
}

// fakeScreen is a no-op pane used to exercise the root model's focus rules.
type fakeScreen struct{}

func (*fakeScreen) Title() string                              { return "Fake" }
func (s *fakeScreen) Update(msg tea.Msg) (Screen, tea.Cmd)     { return s, nil }
func (*fakeScreen) View(width, height int) string              { return "fake" }
func (*fakeScreen) Status() string                             { return "" }
