package tui

import (
	"context"
	"encoding/json"
	"fmt"
	"strings"

	"droidgear-wails/internal/service/codex"
)

// codexAuthSource adapts internal/service/codex auth profiles (named backups
// of ~/.codex/auth.json) to the generic list screen — the Codex → Auth
// profiles sub-view of the GUI.
//
// Read-only for now: save/switch/delete/rename land with the P2 write path
// (ChangeSet preview → confirm, tui-parity §2).
type codexAuthSource struct{}

// CodexAuthSource exposes the Codex auth profiles to the generic screen.
func CodexAuthSource() ListSource { return codexAuthSource{} }

func (codexAuthSource) Title() string { return "Codex Auth" }

func (codexAuthSource) Load(context.Context) ([]ListRow, error) {
	state, err := codex.ListAuthProfiles()
	if err != nil {
		return nil, err
	}
	rows := make([]ListRow, 0, len(state.Profiles))
	for _, profile := range state.Profiles {
		mark := "  "
		if state.Active != nil && profile.Name == *state.Active {
			mark = "● "
		}
		label := profile.Label
		if label == "" {
			label = profile.Name
		}
		if profile.IsOfficial {
			label += officialSuffix
		}
		rows = append(rows, ListRow{ID: profile.Name, Label: label, Mark: mark})
	}
	return rows, nil
}

func (codexAuthSource) Detail(_ context.Context, name string) ([]byte, error) {
	state, err := codex.ListAuthProfiles()
	if err != nil {
		return nil, err
	}
	for _, profile := range state.Profiles {
		if profile.Name != name {
			continue
		}
		return json.MarshalIndent(profile, "", "  ")
	}
	return nil, fmt.Errorf("Codex auth profile %q not found", name)
}

const officialSuffix = "  [official]"

func (codexAuthSource) Summary(rows []ListRow) string {
	active := "none"
	official := 0
	for _, row := range rows {
		if strings.HasSuffix(row.Label, officialSuffix) {
			official++
		}
		if row.Mark == "● " {
			active = strings.TrimSuffix(row.Label, officialSuffix)
		}
	}
	return fmt.Sprintf(
		"Codex Auth · %d profiles · %d official · active: %s · r toggle secrets",
		len(rows), official, active,
	)
}
