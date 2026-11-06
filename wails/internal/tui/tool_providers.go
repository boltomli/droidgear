package tui

import (
	"context"
	"encoding/json"
	"fmt"
	"sort"
	"strings"

	"droidgear-wails/internal/core"
	"droidgear-wails/internal/service/hermes"
	"droidgear-wails/internal/service/importflow"
	"droidgear-wails/internal/service/omp"
	"droidgear-wails/internal/service/pi"
)

// piProvidersSource is Pi → Providers over the LIVE models.json (the GUI's
// provider cards), as opposed to PiSource which lists saved profiles. 'i'
// imports a channel with service/importflow — the same code path as
// `dg pi providers import` and the GUI dialog.
type piProvidersSource struct{}

// PiProvidersSource exposes the live Pi providers to the generic screen.
func PiProvidersSource() ListSource { return piProvidersSource{} }

func (piProvidersSource) Title() string { return "Pi Providers" }

func (piProvidersSource) Load(context.Context) ([]ListRow, error) {
	config, err := pi.ReadCurrentConfig()
	if err != nil {
		return nil, err
	}
	rows := make([]ListRow, 0, len(config.Providers))
	for id, provider := range config.Providers {
		label := id
		if provider.Name != nil && strings.TrimSpace(*provider.Name) != "" {
			label = *provider.Name
		}
		rows = append(rows, ListRow{ID: id, Label: label})
	}
	sort.Slice(rows, func(i, j int) bool { return rows[i].ID < rows[j].ID })
	return rows, nil
}

func (piProvidersSource) Detail(_ context.Context, id string) ([]byte, error) {
	config, err := pi.ReadCurrentConfig()
	if err != nil {
		return nil, err
	}
	provider, ok := config.Providers[id]
	if !ok {
		return nil, fmt.Errorf("pi provider %q not found", id)
	}
	if provider.APIKey != nil {
		masked := core.RedactSecret(*provider.APIKey)
		provider.APIKey = &masked
	}
	return json.MarshalIndent(provider, "", "  ")
}

func (piProvidersSource) Actions() []ListAction {
	return []ListAction{{
		Key: "d", Label: "delete",
		Run: func(_ context.Context, row ListRow) error {
			// Deleting a provider can strand the startup model; the service
			// repairs settings.json and the preview shows both files.
			_, err := pi.DeleteLiveProvider(row.ID)
			return err
		},
	}}
}

func (piProvidersSource) InputActions() []InputAction {
	return []InputAction{{
		Key:   "i",
		Label: "import from channel",
		Hint:  "channel-id [token|-] [model1,model2] — omitted/- = first token; omitted models = every model the endpoint lists",
		Submit: func(ctx context.Context, _ ListRow, value string) writeAction {
			channelID, tokenRef, modelIDs := parseImportValue(value)
			return writeAction{
				title: "import pi provider from channel " + channelID,
				run: func() error {
					_, err := importflow.ImportPi(ctx, channelID, tokenRef, modelIDs, "")
					return err
				},
			}
		},
	}}
}

func (piProvidersSource) Summary(rows []ListRow) string {
	return fmt.Sprintf("Pi Providers · %d provider(s) · i import from channel · d delete", len(rows))
}

// ompProvidersSource is OMP → Providers over the LIVE models.yml. 'i' imports
// a channel with service/importflow (same path as the GUI dialog and
// `dg omp providers import`); 'd' deletes one provider.
type ompProvidersSource struct{}

// OmpProvidersSource exposes the live OMP custom providers.
func OmpProvidersSource() ListSource { return ompProvidersSource{} }

func (ompProvidersSource) Title() string { return "OMP Providers" }

func (ompProvidersSource) Load(context.Context) ([]ListRow, error) {
	list, err := omp.ReadCustomProviders()
	if err != nil {
		return nil, err
	}
	rows := make([]ListRow, 0, len(list.Providers))
	for _, entry := range list.Providers {
		rows = append(rows, ListRow{
			ID:    entry.ProviderID,
			Label: fmt.Sprintf("%s (%d models)", entry.ProviderID, len(entry.Config.Models)),
		})
	}
	return rows, nil
}

func (ompProvidersSource) Detail(_ context.Context, id string) ([]byte, error) {
	list, err := omp.ReadCustomProviders()
	if err != nil {
		return nil, err
	}
	for _, entry := range list.Providers {
		if entry.ProviderID != id {
			continue
		}
		if entry.Config.APIKey != nil {
			masked := core.RedactSecret(*entry.Config.APIKey)
			entry.Config.APIKey = &masked
		}
		return json.MarshalIndent(entry.Config, "", "  ")
	}
	return nil, fmt.Errorf("omp provider %q not found", id)
}

func (ompProvidersSource) Actions() []ListAction {
	return []ListAction{{
		Key: "d", Label: "delete",
		Run: func(_ context.Context, row ListRow) error {
			// Deleting a provider strands config.yml roles (OMP refuses to start
			// without a default role); the service repairs them and the preview
			// shows both files.
			_, err := omp.DeleteCustomProvider(row.ID)
			return err
		},
	}}
}

func (ompProvidersSource) InputActions() []InputAction {
	return []InputAction{{
		Key:   "i",
		Label: "import from channel",
		Hint:  "channel-id [token|-] [model1,model2] — omitted/- = first token; omitted models = every model the endpoint lists",
		Submit: func(ctx context.Context, _ ListRow, value string) writeAction {
			channelID, tokenRef, modelIDs := parseImportValue(value)
			return writeAction{
				title: "import omp provider from channel " + channelID,
				run: func() error {
					_, err := importflow.ImportOmp(ctx, channelID, tokenRef, modelIDs, "")
					return err
				},
			}
		},
	}}
}

func (ompProvidersSource) Summary(rows []ListRow) string {
	return fmt.Sprintf("OMP Providers · %d provider(s) · i import from channel · d delete", len(rows))
}

// parseImportValue splits the one-line TUI import input:
// `channel-id [token] [model1,model2]`.
func parseImportValue(value string) (channelID string, tokenRef string, modelIDs []string) {
	fields := strings.Fields(value)
	if len(fields) > 0 {
		channelID = fields[0]
	}
	if len(fields) > 1 {
		tokenRef = fields[1]
	}
	if len(fields) > 2 {
		for _, part := range strings.Split(fields[2], ",") {
			if trimmed := strings.TrimSpace(part); trimmed != "" {
				modelIDs = append(modelIDs, trimmed)
			}
		}
	}
	return channelID, tokenRef, modelIDs
}

// hermesProvidersSource is Hermes → Providers over the LIVE config.yaml
// (model entries backed by a `custom:<name>` provider). 'i' imports a channel
// through service/importflow — the same path as `dg hermes providers import`
// and the GUI dialog.
type hermesProvidersSource struct{}

// HermesProvidersSource exposes the live Hermes custom providers.
func HermesProvidersSource() ListSource { return hermesProvidersSource{} }

func (hermesProvidersSource) Title() string { return "Hermes Providers" }

func (hermesProvidersSource) Load(context.Context) ([]ListRow, error) {
	current, err := hermes.ReadCurrentConfig()
	if err != nil {
		return nil, err
	}
	rows := make([]ListRow, 0, len(current.Models))
	for _, model := range current.Models {
		if model.Name == nil || strings.TrimSpace(*model.Name) == "" {
			continue
		}
		mark := "  "
		if model.IsDefault {
			mark = "* "
		}
		rows = append(rows, ListRow{ID: *model.Name, Label: *model.Name, Mark: mark})
	}
	sort.Slice(rows, func(i, j int) bool { return rows[i].Label < rows[j].Label })
	return rows, nil
}

func (hermesProvidersSource) Detail(_ context.Context, id string) ([]byte, error) {
	current, err := hermes.ReadCurrentConfig()
	if err != nil {
		return nil, err
	}
	for _, model := range current.Models {
		if model.Name == nil || *model.Name != id {
			continue
		}
		if model.APIKey != nil {
			masked := core.RedactSecret(*model.APIKey)
			model.APIKey = &masked
		}
		return json.MarshalIndent(model, "", "  ")
	}
	return nil, fmt.Errorf("hermes provider %q not found", id)
}

func (hermesProvidersSource) Actions() []ListAction {
	return []ListAction{{
		Key: "d", Label: "delete",
		Run: func(_ context.Context, row ListRow) error {
			return hermes.DeleteLiveProvider(row.ID)
		},
	}}
}

func (hermesProvidersSource) InputActions() []InputAction {
	return []InputAction{{
		Key:   "i",
		Label: "import from channel",
		Hint:  "channel-id [token|-] [model] — omitted/- = first token; omitted model = first model the endpoint lists",
		Submit: func(ctx context.Context, _ ListRow, value string) writeAction {
			channelID, tokenRef, modelIDs := parseImportValue(value)
			modelID := ""
			if len(modelIDs) > 0 {
				modelID = modelIDs[0]
			}
			return writeAction{
				title: "import hermes provider from channel " + channelID,
				run: func() error {
					_, err := importflow.ImportHermes(ctx, channelID, tokenRef, modelID)
					return err
				},
			}
		},
	}}
}

func (hermesProvidersSource) Summary(rows []ListRow) string {
	return fmt.Sprintf("Hermes Providers · %d provider(s) · i import from channel · d delete", len(rows))
}
