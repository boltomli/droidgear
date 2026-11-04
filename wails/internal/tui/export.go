package tui

import (
	"context"
	"encoding/json"
	"fmt"

	"droidgear-wails/internal/service/export"
)

// exportSource adapts internal/service/export — the export-template half of
// the Channels view (`export-templates` sub-view in the GUI).
//
// Listing and inspecting templates is read-only; running one writes a file
// and joins the write phase (tui-parity P2, ChangeSet preview).
type exportSource struct{}

// ExportSource exposes the export templates to the generic screen.
func ExportSource() ListSource { return exportSource{} }

func (exportSource) Title() string { return "Export" }

func (exportSource) Load(ctx context.Context) ([]ListRow, error) {
	templates, err := export.LoadTemplates(ctx)
	if err != nil {
		return nil, err
	}
	rows := make([]ListRow, 0, len(templates))
	for _, template := range templates {
		rows = append(rows, ListRow{ID: template.Name, Label: template.Name})
	}
	return rows, nil
}

func (exportSource) Detail(ctx context.Context, name string) ([]byte, error) {
	templates, err := export.LoadTemplates(ctx)
	if err != nil {
		return nil, err
	}
	for _, template := range templates {
		if template.Name != name {
			continue
		}
		return json.MarshalIndent(template, "", "  ")
	}
	return nil, fmt.Errorf("export template %q not found", name)
}

func (exportSource) Summary(rows []ListRow) string {
	return fmt.Sprintf(
		"Export · %d templates · running one is a write (preview phase)",
		len(rows),
	)
}
