package tui

import (
	"context"
	"encoding/json"
	"fmt"
	"strconv"

	"droidgear-wails/internal/core"
	"droidgear-wails/internal/service/connectivity"
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
		"Models · %d custom · %d favorites · c test · i test inference · r secrets",
		len(rows), favorites,
	)
}

// ViewActions wire the connectivity tests (Preferences → Models): 'c' probes
// the selected BYOK model in ping mode, 'i' runs a full inference round trip.
// The result is a diagnostic document — masked by default like everything
// else (the response text can quote credentials from the endpoint).
func (factorySource) ViewActions() []ViewAction {
	test := func(key string, label string, run func(string) (connectivity.ModelTestResult, error)) ViewAction {
		return ViewAction{
			Key: key, Label: label,
			Run: func(_ context.Context, row ListRow) (string, []byte, []byte, error) {
				result, err := run(row.ID)
				if err != nil {
					return "", nil, nil, err
				}
				full, err := json.MarshalIndent(result, "", "  ")
				if err != nil {
					return "", nil, nil, err
				}
				masked, err := json.MarshalIndent(result, "", "  ")
				if err != nil {
					return "", nil, nil, err
				}
				return "Connection test · " + row.Label,
					[]byte(core.RedactSensitiveText(string(masked))), full, nil
			},
		}
	}
	return []ViewAction{
		test("c", "test ping", func(id string) (connectivity.ModelTestResult, error) {
			return connectivity.TestModelConnection(id)
		}),
		test("i", "test inference", func(id string) (connectivity.ModelTestResult, error) {
			return connectivity.TestModelConnectionWithMode(id, "inference", nil)
		}),
	}
}
