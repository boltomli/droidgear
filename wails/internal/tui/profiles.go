package tui

import (
	"context"
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"

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
	// apply/delete wire the P2 write actions ('a' / 'd'); nil keeps the
	// source read-only.
	apply  func(id string) error
	delete func(id string) error
	// createFrom implements the "save a new profile from a JSON document"
	// input action ('n'), the TUI twin of `… profiles save --file`.
	createFrom func(raw []byte) error
	// hint is appended to the summary line, e.g. where the role ids come from.
	hint string
}

// InputActions exposes profile creation from a JSON file.
func (p *profileSource) InputActions() []InputAction {
	if p.createFrom == nil {
		return nil
	}
	create := p.createFrom
	return []InputAction{{
		Key: "n", Label: "new from file",
		Hint: "path to a profile JSON document (same shape as `dg … profiles save --file`)",
		Submit: func(_ context.Context, row ListRow, value string) writeAction {
			return writeAction{
				title: "create profile from " + value,
				run: func() error {
					raw, err := os.ReadFile(filepath.Clean(value)) // #nosec G304 -- user-supplied profile file
					if err != nil {
						return fmt.Errorf("read profile file: %w", err)
					}
					return create(raw)
				},
			}
		},
	}}
}

// Actions exposes apply/delete through the preview flow.
func (p *profileSource) Actions() []ListAction {
	actions := []ListAction{}
	if p.apply != nil {
		apply := p.apply
		actions = append(actions, ListAction{
			Key: "a", Label: "apply",
			Run: func(_ context.Context, row ListRow) error { return apply(row.ID) },
		})
	}
	if p.delete != nil {
		del := p.delete
		actions = append(actions, ListAction{
			Key: "d", Label: "delete",
			Run: func(_ context.Context, row ListRow) error { return del(row.ID) },
		})
	}
	return actions
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
	summary := fmt.Sprintf(
		"%s · %d profiles · active: %s · r toggle secrets",
		p.title, len(rows), active,
	)
	if p.hint != "" {
		summary += " · " + p.hint
	}
	return summary
}

// CodexSource adapts internal/service/codex.
func CodexSource() ListSource {
	return &profileSource{
		title:  "Codex",
		active: codex.GetActiveProfileID,
		apply:  codex.ApplyProfile,
		delete: codex.DeleteProfile,
		createFrom: func(raw []byte) error {
			var profile codex.CodexProfile
			if err := json.Unmarshal(raw, &profile); err != nil {
				return err
			}
			return codex.SaveProfile(profile)
		},
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
		apply:  hermes.ApplyProfile,
		delete: hermes.DeleteProfile,
		createFrom: func(raw []byte) error {
			var profile hermes.HermesProfile
			if err := json.Unmarshal(raw, &profile); err != nil {
				return err
			}
			return hermes.SaveProfile(profile)
		},
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
		apply:  func(id string) error { _, err := pi.ApplyProfile(id); return err },
		delete: pi.DeleteProfile,
		createFrom: func(raw []byte) error {
			var profile pi.PiProfile
			if err := json.Unmarshal(raw, &profile); err != nil {
				return err
			}
			return pi.SaveProfile(profile)
		},
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
		hint:   "role ids: dg omp roles candidates",
		active: omp.GetActiveProfileID,
		apply:  func(id string) error { _, err := omp.ApplyProfile(id); return err },
		delete: omp.DeleteProfile,
		createFrom: func(raw []byte) error {
			var profile omp.OmpProfile
			if err := json.Unmarshal(raw, &profile); err != nil {
				return err
			}
			return omp.SaveProfile(profile)
		},
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
		apply:  copilot.ApplyCopilotProfile,
		delete: copilot.DeleteCopilotProfile,
		createFrom: func(raw []byte) error {
			var profile copilot.CopilotProfile
			if err := json.Unmarshal(raw, &profile); err != nil {
				return err
			}
			return copilot.SaveCopilotProfile(profile)
		},
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
