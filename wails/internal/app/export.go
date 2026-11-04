// Channel export template commands and the export engine entry points. The
// implementation lives in internal/service/export (design doc §2 R2); this
// file only marshals arguments for the frontend bindings.
package app

import (
	"context"

	"droidgear-wails/internal/service/export"
)

// ChannelFilter mirrors channel_export::ChannelFilter.
type ChannelFilter = export.ChannelFilter

// TokenFilter mirrors channel_export::TokenFilter.
type TokenFilter = export.TokenFilter

// ExportTemplate mirrors channel_export::ExportTemplate.
type ExportTemplate = export.ExportTemplate

// ExportResult mirrors channel_export::ExportResult.
type ExportResult = export.ExportResult

// LoadExportTemplates returns all export templates.
func (a *App) LoadExportTemplates() ([]ExportTemplate, error) {
	return export.LoadTemplates(context.Background())
}

// SaveExportTemplate creates or replaces a template by name.
func (a *App) SaveExportTemplate(template ExportTemplate) error {
	return export.SaveTemplate(context.Background(), template)
}

// DeleteExportTemplate removes a template by name.
func (a *App) DeleteExportTemplate(name string) error {
	return export.DeleteTemplate(context.Background(), name)
}

// RunExportTemplate executes a template and writes the output file. The
// GUI has no dry-run surface yet, so Reveal is irrelevant here: redaction
// only applies to DryRun previews (export.RunOptions).
func (a *App) RunExportTemplate(name string) (ExportResult, error) {
	outcome, err := export.Run(context.Background(), name, export.RunOptions{})
	if err != nil {
		return ExportResult{}, err
	}
	return outcome.Result, nil
}
