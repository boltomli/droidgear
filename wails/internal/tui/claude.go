package tui

import (
	"context"
	"encoding/json"
	"fmt"
	"strings"

	"droidgear-wails/internal/service/claude"
)

// claudeSource adapts internal/service/claude — the Claude → Settings
// sub-view of the GUI (managed settings files + the live global one).
//
// Read-only for now: set-active/create/delete/merge and the launch actions
// land with the P2 write path (tui-parity §2).
type claudeSource struct{}

// ClaudeSource exposes the Claude settings files to the generic screen.
func ClaudeSource() ListSource { return claudeSource{} }

func (claudeSource) Title() string { return "Claude Settings" }

func (claudeSource) Load(context.Context) ([]ListRow, error) {
	files, err := claude.ListSettingsFiles()
	if err != nil {
		return nil, err
	}
	rows := make([]ListRow, 0, len(files))
	for _, file := range files {
		mark := "  "
		if file.IsActive {
			mark = "● "
		}
		label := file.Name
		if !file.Exists {
			label += "  (missing)"
		}
		rows = append(rows, ListRow{ID: file.Name, Label: label, Mark: mark})
	}
	return rows, nil
}

func (claudeSource) Detail(_ context.Context, name string) ([]byte, error) {
	contents, err := claude.ReadSettingsFile(name)
	if err != nil {
		return nil, err
	}
	return json.MarshalIndent(contents, "", "  ")
}

// Actions wires 'a' = make the selected settings file active, 'd' = delete it
// (the live Global file is never deletable, like in the GUI) and 'm' = merge
// it into the global settings file.
func (claudeSource) Actions() []ListAction {
	return []ListAction{
		{
			Key: "a", Label: "activate",
			Run: func(_ context.Context, row ListRow) error {
				name := row.ID
				_, err := claude.SetActiveSettingsFile(&name)
				return err
			},
		},
		{
			Key: "d", Label: "delete",
			Run: func(_ context.Context, row ListRow) error {
				if strings.EqualFold(row.ID, "Global") {
					return fmt.Errorf("cannot delete the global settings file")
				}
				return claude.DeleteSettingsFile(row.ID)
			},
		},
		{
			Key: "m", Label: "merge to global",
			Run: func(_ context.Context, row ListRow) error {
				if strings.EqualFold(row.ID, "Global") {
					return fmt.Errorf("Global is already the global settings file")
				}
				return claude.MergeSettingsToGlobal(row.ID)
			},
		},
	}
}

// InputActions wire the name-taking writes: create ('n'), create seeded from
// the active file ('c') and duplicate the selected file ('u').
func (claudeSource) InputActions() []InputAction {
	newCreate := func(key string, copyActive bool) InputAction {
		label := "create"
		if copyActive {
			label = "create from active"
		}
		return InputAction{
			Key: key, Label: label,
			Hint: "new settings file name",
			Submit: func(_ context.Context, row ListRow, value string) writeAction {
				return writeAction{
					title: "create claude settings " + value,
					run:   func() error { _, err := claude.CreateSettingsFile(value, copyActive); return err },
				}
			},
		}
	}
	return []InputAction{
		newCreate("n", false),
		newCreate("c", true),
		{
			Key: "u", Label: "duplicate",
			Hint:    "new name for the copy",
			Initial: func(row ListRow) string { return row.ID + "-copy" },
			Submit: func(_ context.Context, row ListRow, value string) writeAction {
				return writeAction{
					title: "duplicate " + row.ID,
					run:   func() error { _, err := claude.DuplicateSettingsFile(row.ID, value); return err },
				}
			},
		},
	}
}

func (claudeSource) Summary(rows []ListRow) string {
	active := "none"
	for _, row := range rows {
		if row.Mark == "● " {
			active = row.Label
			break
		}
	}
	return fmt.Sprintf(
		"Claude Settings · %d files · active: %s · r toggle secrets",
		len(rows), active,
	)
}
