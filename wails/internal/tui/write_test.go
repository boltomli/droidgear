package tui

import (
	"context"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"droidgear-wails/internal/core"
	"droidgear-wails/internal/service/dsh"
	"droidgear-wails/internal/service/export"
	"droidgear-wails/internal/service/paths"
	"droidgear-wails/internal/testutil"
)

// --- writeFlow contract -----------------------------------------------------

func TestWriteFlowPreviewsAppliesAndReturns(t *testing.T) {
	home := testutil.IsolatedHome(t)
	target := filepath.Join(home, "out.json")
	action := writeAction{title: "write out", run: func() error {
		return core.AtomicWrite(target, []byte(`{"ok":true}`))
	}}
	back := &fakeScreen{}
	flow := newWriteFlow(back, action, nil)

	view := flow.View(100, 24)
	if !strings.Contains(view, filepath.Base(target)) {
		t.Fatalf("preview does not list the target file:\n%s", view)
	}
	if !strings.Contains(flow.Status(), "y apply") {
		t.Fatalf("status = %q, want the apply hint", flow.Status())
	}
	if _, err := os.Stat(target); err == nil {
		t.Fatal("preview wrote to disk")
	}

	next, _ := flow.Update(keyMsg("y"))
	applied, ok := next.(*writeFlow)
	if !ok {
		t.Fatalf("update returned %T, want *writeFlow", next)
	}
	if !strings.Contains(applied.Status(), "applied") {
		t.Fatalf("status = %q, want applied", applied.Status())
	}
	data, err := os.ReadFile(target) // #nosec G304 -- isolated home
	if err != nil || string(data) != `{"ok":true}` {
		t.Fatalf("apply did not write the file: %q err=%v", data, err)
	}

	// Any of the usual keys hands control back to the origin screen.
	next, _ = applied.Update(keyMsg("enter"))
	if got, ok := next.(*fakeScreen); !ok || got != back {
		t.Fatalf("enter returned %T, want the origin screen", next)
	}
}

func TestWriteFlowConflictNeedsForce(t *testing.T) {
	home := testutil.IsolatedHome(t)
	src := filepath.Join(home, "src.txt")
	dst := filepath.Join(home, "dst.txt")
	if err := os.WriteFile(src, []byte("v1"), 0o644); err != nil { // #nosec G306 -- test fixture
		t.Fatalf("write src: %v", err)
	}
	action := writeAction{title: "copy src", run: func() error {
		data, err := os.ReadFile(src) // #nosec G304 -- isolated home
		if err != nil {
			return err
		}
		return core.AtomicWrite(dst, []byte("copy:"+string(data)))
	}}
	flow := newWriteFlow(&fakeScreen{}, action, nil)

	if view := flow.View(100, 24); !strings.Contains(view, "copy:v1") {
		t.Fatalf("preview should show the document that would land:\n%s", view)
	}

	// External writer wins the race between preview and confirm.
	if err := os.WriteFile(src, []byte("v2"), 0o644); err != nil { // #nosec G306 -- test fixture
		t.Fatalf("rewrite src: %v", err)
	}
	next, _ := flow.Update(keyMsg("y"))
	conflicted, _ := next.(*writeFlow)
	if !strings.Contains(conflicted.Status(), "conflict") {
		t.Fatalf("status = %q, want the conflict prompt", conflicted.Status())
	}
	if _, err := os.Stat(dst); err == nil {
		t.Fatal("conflict path wrote to disk")
	}

	// 'y' on the conflict is the --force equivalent.
	next, _ = conflicted.Update(keyMsg("y"))
	forced, _ := next.(*writeFlow)
	if !strings.Contains(forced.Status(), "applied") {
		t.Fatalf("status = %q, want applied after force", forced.Status())
	}
	data, err := os.ReadFile(dst) // #nosec G304 -- isolated home
	if err != nil || string(data) != "copy:v2" {
		t.Fatalf("forced apply = %q err=%v, want the fresh content", data, err)
	}
}

func TestWriteFlowRedactsSecretsUnlessRevealed(t *testing.T) {
	home := testutil.IsolatedHome(t)
	target := filepath.Join(home, "secret.json")
	action := writeAction{title: "write secret", run: func() error {
		return core.AtomicWrite(target, []byte(`{"apiKey":"sk-secret-123456"}`))
	}}
	flow := newWriteFlow(&fakeScreen{}, action, nil)

	view := flow.View(100, 24)
	if strings.Contains(view, "sk-secret-123456") {
		t.Fatalf("preview leaked the api key:\n%s", view)
	}
	if !strings.Contains(view, "****") {
		t.Fatalf("preview shows no redaction marker:\n%s", view)
	}

	next, _ := flow.Update(keyMsg("r"))
	if view = next.View(100, 24); !strings.Contains(view, "sk-secret-123456") {
		t.Fatalf("reveal did not show the key:\n%s", view)
	}
}

// --- list-screen actions ----------------------------------------------------

type actionFakeSource struct {
	fakeSource
	applied bool
}

func (s *actionFakeSource) Actions() []ListAction {
	return []ListAction{{
		Key: "a", Label: "apply",
		Run: func(context.Context, ListRow) error {
			s.applied = true
			return nil
		},
	}}
}

func TestListScreenActionOpensFlowThenRefreshes(t *testing.T) {
	source := &actionFakeSource{}
	source.rows = []ListRow{{ID: "alpha", Label: "Alpha"}}
	screen, err := newListScreen(context.Background(), source)
	if err != nil {
		t.Fatalf("newListScreen: %v", err)
	}
	if !strings.Contains(screen.Status(), "a apply") {
		t.Fatalf("status = %q, want the action hint", screen.Status())
	}

	next, _ := screen.Update(keyMsg("a"))
	flow, ok := next.(*writeFlow)
	if !ok {
		t.Fatalf("pressing a returned %T, want the preview flow", next)
	}
	if !strings.Contains(flow.Status(), "apply Alpha") {
		t.Fatalf("status = %q, want the action title", flow.Status())
	}

	next, _ = flow.Update(keyMsg("y"))
	if !source.applied {
		t.Fatal("confirm did not run the action")
	}
	next, _ = next.Update(keyMsg("enter"))
	if _, ok := next.(*listScreen); !ok {
		t.Fatalf("enter returned %T, want the list screen", next)
	}
}

// --- root model key sink ----------------------------------------------------

func TestRootForwardsAllKeysToKeySink(t *testing.T) {
	action := writeAction{title: "noop", run: func() error { return nil }}
	flow := newWriteFlow(&fakeScreen{}, action, nil)

	m := New(context.Background())
	m.SetScreen(flow)
	next := step(t, m, keyMsg("esc"))
	if next.Focused() != focusContent {
		t.Fatalf("focus = %v, want content while the sink is active", next.Focused())
	}
	if _, ok := next.CurrentScreen().(*fakeScreen); !ok {
		t.Fatalf("esc in the sink returned %T, want the origin screen", next.CurrentScreen())
	}
}

// --- Paths write (P2 row: Preferences → Paths 写) ---------------------------

func findPathRow(t *testing.T, rows []paths.EffectivePath, key string) paths.EffectivePath {
	t.Helper()
	for _, row := range rows {
		if row.Key == key {
			return row
		}
	}
	t.Fatalf("no path row %q", key)
	return paths.EffectivePath{}
}

func TestPathsScreenEditAndResetThroughPreview(t *testing.T) {
	home := testutil.IsolatedHome(t)
	override := filepath.Join(home, "custom-home")

	screen, err := newPathsScreen(context.Background())
	if err != nil {
		t.Fatalf("newPathsScreen: %v", err)
	}
	steps := screen

	// Select a row and enter edit mode.
	next, _ := screen.Update(keyMsg("down"))
	steps = next
	next, _ = steps.Update(keyMsg("e"))
	steps = next
	if got := steps.Status(); !strings.Contains(got, "enter save") {
		t.Fatalf("status = %q, want edit hints", got)
	}
	// Type the new path one key at a time (like a real keyboard).
	for _, r := range override {
		next, _ = steps.Update(keyMsg(string(r)))
		steps = next
	}
	next, _ = steps.Update(keyMsg("enter"))
	flow, ok := next.(*writeFlow)
	if !ok {
		t.Fatalf("enter returned %T, want the preview flow", next)
	}
	if !strings.Contains(flow.Status(), "y apply") {
		t.Fatalf("status = %q, want the preview stage", flow.Status())
	}
	next, _ = flow.Update(keyMsg("y"))
	if !strings.Contains(next.Status(), "applied") {
		t.Fatalf("status = %q, want applied", next.Status())
	}
	next, _ = next.Update(keyMsg("enter"))
	pane, ok := next.(*pathsScreen)
	if !ok {
		t.Fatalf("enter returned %T, want the paths screen", next)
	}

	effective, err := paths.Effective(context.Background())
	if err != nil {
		t.Fatalf("Effective: %v", err)
	}
	row := findPathRow(t, effective.List(), pane.selectedRowKey())
	if row.IsDefault || row.Path != override {
		t.Fatalf("override not applied: %+v", row)
	}
	if pane.View(100, 24) == "" {
		t.Fatal("paths screen blank after refresh")
	}

	// 'x' resets the override through the same preview flow.
	next, _ = pane.Update(keyMsg("x"))
	reset, ok := next.(*writeFlow)
	if !ok {
		t.Fatalf("x returned %T, want the preview flow", next)
	}
	next, _ = reset.Update(keyMsg("y"))
	if !strings.Contains(next.Status(), "applied") {
		t.Fatalf("status = %q, want applied", next.Status())
	}
	next, _ = next.Update(keyMsg("enter"))
	refreshed, _ := next.(*pathsScreen)

	effective, err = paths.Effective(context.Background())
	if err != nil {
		t.Fatalf("Effective after reset: %v", err)
	}
	row = findPathRow(t, effective.List(), refreshed.selectedRowKey())
	if !row.IsDefault {
		t.Fatalf("override not reset: %+v", row)
	}
}

// --- batch 1: previously unwired service writes ----------------------------

func TestExportRunThroughPreview(t *testing.T) {
	home := testutil.IsolatedHome(t)
	ctx := context.Background()
	out := filepath.Join(home, "export-out.json")
	if err := export.SaveTemplate(ctx, export.ExportTemplate{
		Name:       "t1",
		Format:     "json",
		OutputPath: out,
	}); err != nil {
		t.Fatalf("SaveTemplate: %v", err)
	}

	screen, err := newListScreen(ctx, ExportSource())
	if err != nil {
		t.Fatalf("newListScreen: %v", err)
	}
	if !strings.Contains(screen.Status(), "x run") {
		t.Fatalf("status = %q, want the run hint", screen.Status())
	}

	next, _ := screen.Update(keyMsg("x"))
	flow, ok := next.(*writeFlow)
	if !ok {
		t.Fatalf("x returned %T, want the preview flow", next)
	}
	view := flow.View(100, 24)
	if !strings.Contains(view, "export-out") {
		t.Fatalf("preview does not list the output file:\n%s", view)
	}
	if _, err := os.Stat(out); err == nil {
		t.Fatal("preview wrote the export output")
	}

	next, _ = flow.Update(keyMsg("y"))
	if !strings.Contains(next.Status(), "applied") {
		t.Fatalf("status = %q, want applied", next.Status())
	}
	if _, err := os.Stat(out); err != nil {
		t.Fatalf("output file missing after apply: %v", err)
	}
}

func TestDshToggleEntryThroughPreview(t *testing.T) {
	testutil.DshTestHome(t)
	ctx := context.Background()
	profile := "desktop"

	snapshot, err := dsh.ReadConfigSnapshot(&profile)
	if err != nil {
		t.Fatalf("ReadConfigSnapshot: %v", err)
	}
	if len(snapshot.Entries) == 0 {
		t.Fatal("fixture has no config entries")
	}
	// Toggle a schema-ENABLED entry: enabling a schema-disabled one is a
	// documented no-op (the patch can only add disables, dsh_config_write).
	idx := -1
	for i, entry := range snapshot.Entries {
		if !entry.Disabled {
			idx = i
			break
		}
	}
	if idx < 0 {
		t.Fatal("fixture has no enabled config entry to toggle")
	}
	entryID := snapshot.Entries[idx].ID

	screen, err := newListScreen(ctx, DshDesktopSource())
	if err != nil {
		t.Fatalf("newListScreen: %v", err)
	}
	if !strings.Contains(screen.Status(), "t toggle") {
		t.Fatalf("status = %q, want the toggle hint", screen.Status())
	}
	// Rows follow the snapshot order: walk down to the enabled entry.
	for i := 0; i < idx; i++ {
		next, _ := screen.Update(keyMsg("down"))
		screen = next
	}

	next, _ := screen.Update(keyMsg("t"))
	flow, ok := next.(*writeFlow)
	if !ok {
		t.Fatalf("t returned %T, want the preview flow", next)
	}
	next, _ = flow.Update(keyMsg("y"))
	if !strings.Contains(next.Status(), "applied") {
		t.Fatalf("status = %q, want applied", next.Status())
	}

	snapshot, err = dsh.ReadConfigSnapshot(&profile)
	if err != nil {
		t.Fatalf("ReadConfigSnapshot after toggle: %v", err)
	}
	for _, entry := range snapshot.Entries {
		if entry.ID != entryID {
			continue
		}
		if !entry.Disabled {
			t.Fatalf("toggle did not disable %s", entryID)
		}
		return
	}
	t.Fatalf("entry %s missing after toggle", entryID)
}

// --- view actions (masked fetch-and-display) --------------------------------

type viewFakeSource struct {
	fakeSource
}

func (s *viewFakeSource) ViewActions() []ViewAction {
	return []ViewAction{{
		Key: "f", Label: "tokens",
		Run: func(context.Context, ListRow) (string, []byte, []byte, error) {
			return "Tokens · Fake",
				[]byte(`[{"key":"****abcd"}]`),
				[]byte(`[{"key":"sk-real-token-5678"}]`),
				nil
		},
	}}
}

func TestViewActionShowsMaskedTokensThenReveal(t *testing.T) {
	source := &viewFakeSource{}
	source.rows = []ListRow{{ID: "ch1", Label: "Alpha"}}
	screen, err := newListScreen(context.Background(), source)
	if err != nil {
		t.Fatalf("newListScreen: %v", err)
	}
	if !strings.Contains(screen.Status(), "f tokens") {
		t.Fatalf("status = %q, want the tokens hint", screen.Status())
	}

	next, _ := screen.Update(keyMsg("f"))
	view, ok := next.(*viewScreen)
	if !ok {
		t.Fatalf("f returned %T, want the view screen", next)
	}
	rendered := view.View(100, 24)
	if strings.Contains(rendered, "sk-real-token-5678") {
		t.Fatalf("view leaked the token:\n%s", rendered)
	}
	if !strings.Contains(rendered, "****abcd") {
		t.Fatalf("view missing the masked key:\n%s", rendered)
	}

	next, _ = view.Update(keyMsg("r"))
	if rendered = next.View(100, 24); !strings.Contains(rendered, "sk-real-token-5678") {
		t.Fatalf("reveal did not show the token:\n%s", rendered)
	}

	next, _ = view.Update(keyMsg("esc"))
	if _, ok := next.(*listScreen); !ok {
		t.Fatalf("esc returned %T, want the list screen", next)
	}
}
