package tui

import (
	"context"
	"encoding/json"
	"fmt"

	"droidgear-wails/internal/core"
)

// registrySource adapts the vendored model registry — the terminal
// counterpart of Preferences → Models (browse). Connectivity tests target
// BYOK models, so they live on the Droid → Models section instead.
type registrySource struct{}

// RegistrySource exposes the model registry to the generic screen.
func RegistrySource() ListSource { return registrySource{} }

func (registrySource) Title() string { return "Registry" }

func (registrySource) Load(context.Context) ([]ListRow, error) {
	models := core.RegistryModels()
	rows := make([]ListRow, 0, len(models))
	for _, model := range models {
		label := model.Name
		if label == "" {
			label = model.ID
		}
		mark := "  "
		if model.Reasoning {
			mark = "● "
		}
		rows = append(rows, ListRow{ID: model.ID, Label: label, Mark: mark})
	}
	return rows, nil
}

func (registrySource) Detail(_ context.Context, id string) ([]byte, error) {
	model := core.FindRegistryModel(id)
	if model == nil {
		return nil, fmt.Errorf("registry model %q not found", id)
	}
	return json.MarshalIndent(model, "", "  ")
}

func (registrySource) Summary(rows []ListRow) string {
	reasoning := 0
	for _, row := range rows {
		if row.Mark == "● " {
			reasoning++
		}
	}
	return fmt.Sprintf(
		"Registry · %d models · %d reasoning · static data (BYOK tests live in Droid → Models)",
		len(rows), reasoning,
	)
}
