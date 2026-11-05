package tui

import (
	"context"
	"encoding/json"
	"fmt"

	"droidgear-wails/internal/service/codex"
	"droidgear-wails/internal/service/copilot"
	"droidgear-wails/internal/service/hermes"
	"droidgear-wails/internal/service/omp"
	"droidgear-wails/internal/service/pi"
)

// profileSource adapts a tool whose service package exposes the standard
// profile shape (list / get / active id). The four extracted domains all
// match, so they differ only in the three closures below.
type profileSource struct {
	title  string
	list   func() ([]ListRow, error)
	active func() (*string, error)
	get    func(id string) ([]byte, error)
}

func (p *profileSource) Title() string { return p.title }

func (p *profileSource) Load(context.Context) ([]ListRow, error) {
	rows, err := p.list()
	if err != nil {
		return nil, err
	}
	if p.active != nil {
		if active, err := p.active(); err == nil && active != nil {
			for i := range rows {
				if rows[i].ID == *active {
					rows[i].Mark = "●"
					break
				}
			}
		}
	}
	return rows, nil
}

func (p *profileSource) Detail(_ context.Context, id string) ([]byte, error) {
	return p.get(id)
}

func (p *profileSource) Summary(rows []ListRow) string {
	active := "none"
	for _, row := range rows {
		if row.Mark == "●" {
			active = row.Label
			break
		}
	}
	return fmt.Sprintf(
		"%s · %d profiles · active: %s · r toggle secrets",
		p.title, len(rows), active,
	)
}

// CodexSource adapts internal/service/codex.
func CodexSource() ListSource {
	return &profileSource{
		title:  "Codex",
		active: codex.GetActiveProfileID,
		list: func() ([]ListRow, error) {
			profiles, err := codex.ListProfiles()
			if err != nil {
				return nil, err
			}
			rows := make([]ListRow, 0, len(profiles))
			for _, profile := range profiles {
				rows = append(rows, ListRow{ID: profile.ID, Label: profile.Name})
			}
			return rows, nil
		},
		get: func(id string) ([]byte, error) {
			return marshalDetail(id, codex.GetProfile)
		},
	}
}

// HermesSource adapts internal/service/hermes.
func HermesSource() ListSource {
	return &profileSource{
		title:  "Hermes",
		active: hermes.GetActiveProfileID,
		list: func() ([]ListRow, error) {
			profiles, err := hermes.ListProfiles()
			if err != nil {
				return nil, err
			}
			rows := make([]ListRow, 0, len(profiles))
			for _, profile := range profiles {
				rows = append(rows, ListRow{ID: profile.ID, Label: profile.Name})
			}
			return rows, nil
		},
		get: func(id string) ([]byte, error) {
			return marshalDetail(id, hermes.GetProfile)
		},
	}
}

// PiSource adapts internal/service/pi.
func PiSource() ListSource {
	return &profileSource{
		title:  "Pi",
		active: pi.GetActiveProfileID,
		list: func() ([]ListRow, error) {
			profiles, err := pi.ListProfiles()
			if err != nil {
				return nil, err
			}
			rows := make([]ListRow, 0, len(profiles))
			for _, profile := range profiles {
				rows = append(rows, ListRow{ID: profile.ID, Label: profile.Name})
			}
			return rows, nil
		},
		get: func(id string) ([]byte, error) {
			return marshalDetail(id, pi.GetProfile)
		},
	}
}

// OmpSource adapts internal/service/omp.
func OmpSource() ListSource {
	return &profileSource{
		title:  "OMP",
		active: omp.GetActiveProfileID,
		list: func() ([]ListRow, error) {
			profiles, err := omp.ListProfiles()
			if err != nil {
				return nil, err
			}
			rows := make([]ListRow, 0, len(profiles))
			for _, profile := range profiles {
				rows = append(rows, ListRow{ID: profile.ID, Label: profile.Name})
			}
			return rows, nil
		},
		get: func(id string) ([]byte, error) {
			return marshalDetail(id, omp.GetProfile)
		},
	}
}

// CopilotSource adapts internal/service/copilot (Copilot → Profiles).
func CopilotSource() ListSource {
	return &profileSource{
		title:  "Copilot",
		active: copilot.GetActiveCopilotProfileId,
		list: func() ([]ListRow, error) {
			profiles, err := copilot.ListCopilotProfiles()
			if err != nil {
				return nil, err
			}
			rows := make([]ListRow, 0, len(profiles))
			for _, profile := range profiles {
				rows = append(rows, ListRow{ID: profile.ID, Label: profile.Name})
			}
			return rows, nil
		},
		get: func(id string) ([]byte, error) {
			return marshalDetail(id, copilot.GetCopilotProfile)
		},
	}
}

// marshalDetail keeps the adapters down to one line each: fetch by id,
// pretty-print, let the screen handle redaction.
func marshalDetail[T any](id string, get func(string) (T, error)) ([]byte, error) {
	profile, err := get(id)
	if err != nil {
		return nil, err
	}
	return json.MarshalIndent(profile, "", "  ")
}
