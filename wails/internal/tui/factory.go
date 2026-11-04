package tui

import (
	"context"
	"encoding/json"
	"fmt"
	"strconv"

	"droidgear-wails/internal/service/factory"
)

// factorySource adapts internal/service/factory — the Models view of the GUI
// (BYOK custom models + favorites). Rows are addressed by index so two
// entries that share a model id still open the right detail.
type factorySource struct{}

// FactorySource exposes the BYOK custom models to the generic screen.
func FactorySource() ListSource { return factorySource{} }

func (factorySource) Title() string { return "Models" }

func (factorySource) Load(context.Context) ([]ListRow, error) {
	models, err := factory.LoadCustomModels()
	if err != nil {
		return nil, err
	}
	favorites, err := factory.GetModelFavorites()
	if err != nil {
		favorites = nil
	}
	isFavorite := map[string]bool{}
	for _, id := range favorites {
		isFavorite[id] = true
	}

	rows := make([]ListRow, 0, len(models))
	for i, model := range models {
		label := model.DisplayName
		if label == nil || *label == "" {
			label = &model.Model
		}
		mark := "  "
		id := model.ID
		if (id != nil && isFavorite[*id]) || isFavorite[model.Model] {
			mark = "★ "
		}
		rows = append(rows, ListRow{ID: strconv.Itoa(i), Label: *label, Mark: mark})
	}
	return rows, nil
}

func (factorySource) Detail(_ context.Context, id string) ([]byte, error) {
	index, err := strconv.Atoi(id)
	if err != nil {
		return nil, fmt.Errorf("invalid model row %q", id)
	}
	models, err := factory.LoadCustomModels()
	if err != nil {
		return nil, err
	}
	if index < 0 || index >= len(models) {
		return nil, fmt.Errorf("model row %d out of range", index)
	}
	return json.MarshalIndent(models[index], "", "  ")
}

func (factorySource) Summary(rows []ListRow) string {
	favorites := 0
	for _, row := range rows {
		if row.Mark == "★ " {
			favorites++
		}
	}
	return fmt.Sprintf(
		"Models · %d custom · %d favorites · r toggle secrets",
		len(rows), favorites,
	)
}
