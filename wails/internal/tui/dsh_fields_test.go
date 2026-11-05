package tui

import (
	"context"
	"strings"
	"testing"

	"droidgear-wails/internal/service/dsh"
	"droidgear-wails/internal/testutil"
)

// TestDshFieldEditorTogglesAndRestores drives the 'e' field editor end to
// end: open the editor for a schema-enabled entry, toggle a boolean field
// through the preview flow, then 'R' restores the entry's defaults.
func TestDshFieldEditorTogglesAndRestores(t *testing.T) {
	testutil.DshTestHome(t)
	ctx := context.Background()
	profile := "desktop"

	// Pick a schema-enabled entry that carries a boolean field.
	snapshot, err := dsh.ReadConfigSnapshot(&profile)
	if err != nil {
		t.Fatalf("ReadConfigSnapshot: %v", err)
	}
	entryIdx, fieldIdx := -1, -1
	var entryID, fieldName string
	var effective bool
	for i, entry := range snapshot.Entries {
		if entry.Disabled {
			continue
		}
		for j, field := range entry.Fields {
			if field.Kind == "boolean" && !field.Secret {
				entryIdx, fieldIdx = i, j
				entryID, fieldName = entry.ID, field.Name
				effective, _ = field.Value.(bool)
				break
			}
		}
		if entryIdx >= 0 {
			break
		}
	}
	if entryIdx < 0 {
		t.Fatal("fixture has no enabled entry with a boolean field")
	}
	want := !effective

	screen, err := newListScreen(ctx, DshDesktopSource())
	if err != nil {
		t.Fatalf("newListScreen: %v", err)
	}
	if !strings.Contains(screen.Status(), "e edit config") {
		t.Fatalf("status = %q, want the edit-config hint", screen.Status())
	}

	// Walk to the entry (rows follow the snapshot order) and open the editor.
	for i := 0; i < entryIdx; i++ {
		screen.Update(keyMsg("down"))
	}
	next, _ := screen.Update(keyMsg("e"))
	fields, ok := next.(*dshFieldsScreen)
	if !ok {
		t.Fatalf("e returned %T, want the field editor", next)
	}
	if view := fields.View(100, 40); !strings.Contains(view, "config: "+entryID) {
		t.Fatalf("editor header missing the entry id:\n%s", view)
	}

	// Walk to the boolean field and toggle it through the flow.
	for i := 0; i < fieldIdx; i++ {
		fields.Update(keyMsg("down"))
	}
	fields.Update(keyMsg("enter"))
	if got := fields.Status(); !strings.Contains(got, "preview") {
		t.Fatalf("status = %q, want the preview stage", got)
	}
	fields.Update(keyMsg("y"))
	if got := fields.Status(); !strings.Contains(got, "applied") {
		t.Fatalf("status = %q, want applied", got)
	}
	// Leave the flow: the editor comes back refreshed.
	next, _ = fields.Update(keyMsg("enter"))
	fields, ok = next.(*dshFieldsScreen)
	if !ok {
		t.Fatalf("enter returned %T, want the refreshed editor", next)
	}

	assertEntryConfigValue(t, profile, entryID, fieldName, want)

	// Back to the list, then 'R' drops the profile-layer config again.
	next, _ = fields.Update(keyMsg("esc"))
	list, ok := next.(*listScreen)
	if !ok {
		t.Fatalf("esc returned %T, want the list screen", next)
	}
	if !strings.Contains(list.Status(), "R restore defaults") {
		t.Fatalf("status = %q, want the restore hint", list.Status())
	}
	next, _ = list.Update(keyMsg("R"))
	flow, ok := next.(*writeFlow)
	if !ok {
		t.Fatalf("R returned %T, want the preview flow", next)
	}
	next, _ = flow.Update(keyMsg("y"))
	if !strings.Contains(next.Status(), "applied") {
		t.Fatalf("status = %q, want applied", next.Status())
	}

	assertEntryConfigAbsent(t, profile, entryID, fieldName)
}

func assertEntryConfigValue(t *testing.T, profile string, entryID string, fieldName string, want bool) {
	t.Helper()
	snapshot, err := dsh.ReadConfigSnapshot(&profile)
	if err != nil {
		t.Fatalf("ReadConfigSnapshot: %v", err)
	}
	for _, entry := range snapshot.Entries {
		if entry.ID != entryID {
			continue
		}
		config, err := dshConfigMap(entry.Config)
		if err != nil {
			t.Fatalf("dshConfigMap: %v", err)
		}
		got, _ := config[fieldName].(bool)
		if got != want {
			t.Fatalf("config[%s] = %v, want %v (config: %+v)", fieldName, got, want, config)
		}
		return
	}
	t.Fatalf("entry %s missing after toggle", entryID)
}

func assertEntryConfigAbsent(t *testing.T, profile string, entryID string, fieldName string) {
	t.Helper()
	snapshot, err := dsh.ReadConfigSnapshot(&profile)
	if err != nil {
		t.Fatalf("ReadConfigSnapshot: %v", err)
	}
	for _, entry := range snapshot.Entries {
		if entry.ID != entryID {
			continue
		}
		if entry.Config == nil {
			return // whole config dropped by the restore
		}
		config, err := dshConfigMap(entry.Config)
		if err != nil {
			t.Fatalf("dshConfigMap: %v", err)
		}
		if _, present := config[fieldName]; present {
			t.Fatalf("config[%s] survived the restore: %+v", fieldName, config)
		}
		return
	}
	t.Fatalf("entry %s missing after restore", entryID)
}
