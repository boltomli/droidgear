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

// Actions wires 'a' = make the selected settings file active and 'd' = delete
// it (the live Global file is never deletable, like in the GUI).
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
