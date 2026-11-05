package tui

import (
	"context"
	"encoding/json"
	"fmt"

	"droidgear-wails/internal/core"
	"droidgear-wails/internal/service/dsh"
)

// dshSource adapts one Dsh profile (desktop / web) to the generic list
// screen — the rows are the composed config entries of the profile's
// snapshot (Dsh → Desktop / Web).
//
// Read-only for now: provider save/delete and the entry toggle land with the
// P2 write path (the write needs expectedHash, tui-parity §2).
type dshSource struct {
	profile string
	// last snapshot kept so Summary can report hash/warnings without a
	// second read.
	snapshot *dsh.DshConfigSnapshot
}

// DshDesktopSource exposes the desktop profile's config entries.
func DshDesktopSource() ListSource { return &dshSource{profile: "desktop"} }

// DshWebSource exposes the web profile's config entries.
func DshWebSource() ListSource { return &dshSource{profile: "web"} }

func (s *dshSource) Title() string {
	return "Dsh " + capitalize(s.profile)
}

func (s *dshSource) Load(context.Context) ([]ListRow, error) {
	snapshot, err := dsh.ReadConfigSnapshot(&s.profile)
	if err != nil {
		return nil, err
	}
	s.snapshot = &snapshot
	rows := make([]ListRow, 0, len(snapshot.Entries))
	for _, entry := range snapshot.Entries {
		mark := "● "
		if entry.Disabled {
			mark = "○ "
		}
		label := entry.PackageName
		if label == "" {
			label = entry.ID
		}
		if entry.ShadowedByHome {
			label += "  [shadowed]"
		}
		rows = append(rows, ListRow{ID: entry.ID, Label: label, Mark: mark})
	}
	return rows, nil
}

func (s *dshSource) Detail(ctx context.Context, id string) ([]byte, error) {
	return s.detail(ctx, id, false)
}

// Actions wires 't' = flip the entry's disabled flag through the preview
// flow; expectedHash comes from the snapshot the row was listed from, so an
// external edit between preview and confirm fails as a service conflict.
func (s *dshSource) Actions() []ListAction {
	return []ListAction{
		{
			Key: "t", Label: "toggle",
			Run: func(_ context.Context, row ListRow) error {
				snapshot, err := dsh.ReadConfigSnapshot(&s.profile)
				if err != nil {
					return err
				}
				for _, entry := range snapshot.Entries {
					if entry.ID != row.ID {
						continue
					}
					hash := snapshot.PatchHash
					_, err := dsh.SetConfigEntryDisabled(&s.profile, row.ID, !entry.Disabled, &hash)
					return err
				}
				return fmt.Errorf("Dsh entry %q not found in profile %s", row.ID, s.profile)
			},
		},
		s.restoreAction(),
	}
}

// ScreenActions wires 'e' = the field editor (per-field writes with the
// entry's expectedHash, the GUI settings-card equivalent).
func (s *dshSource) ScreenActions() []ScreenAction {
	return []ScreenAction{{
		Key: "e", Label: "edit config",
		Open: func(ctx context.Context, back Screen, row ListRow) (Screen, error) {
			return newDshFieldsScreen(ctx, back, s.profile, row.ID)
		},
	}}
}

// Actions also carries 'R' = restore the entry's defaults (drops the
// profile-layer config so the schema values show through again).
func (s *dshSource) restoreAction() ListAction {
	return ListAction{
		Key: "R", Label: "restore defaults",
		Run: func(_ context.Context, row ListRow) error {
			snapshot, err := dsh.ReadConfigSnapshot(&s.profile)
			if err != nil {
				return err
			}
			hash := snapshot.PatchHash
			_, err = dsh.SaveConfigEntry(&s.profile, row.ID, nil, &hash)
			return err
		},
	}
}

// DetailFor is the reveal-aware variant: Dsh secrets sit under generic
// `value` keys (the field itself carries `secret: true`), so render-time
// redaction cannot recognise them — the masking happens here instead and
// only the reveal view gets the real value.
func (s *dshSource) DetailFor(ctx context.Context, id string, reveal bool) ([]byte, error) {
	return s.detail(ctx, id, reveal)
}

func (s *dshSource) detail(_ context.Context, id string, reveal bool) ([]byte, error) {
	snapshot, err := dsh.ReadConfigSnapshot(&s.profile)
	if err != nil {
		return nil, err
	}
	for _, entry := range snapshot.Entries {
		if entry.ID != id {
			continue
		}
		if !reveal {
			for i := range entry.Fields {
				if entry.Fields[i].Secret {
					masked, _ := entry.Fields[i].Value.(string)
					entry.Fields[i].Value = core.RedactSecret(masked)
				}
			}
		}
		return json.MarshalIndent(entry, "", "  ")
	}
	return nil, fmt.Errorf("Dsh entry %q not found in profile %s", id, s.profile)
}

func (s *dshSource) Summary(rows []ListRow) string {
	disabled := 0
	for _, row := range rows {
		if row.Mark == "○ " {
			disabled++
		}
	}
	summary := fmt.Sprintf(
		"Dsh %s · %d entries · %d disabled · r toggle secrets",
		s.profile, len(rows), disabled,
	)
	if s.snapshot != nil {
		if s.snapshot.PatchHash != "" {
			summary += fmt.Sprintf(" · patch %.8s", s.snapshot.PatchHash)
		}
		if len(s.snapshot.Warnings) > 0 {
			summary += fmt.Sprintf(" · %d warnings", len(s.snapshot.Warnings))
		}
	}
	return summary
}

func capitalize(v string) string {
	if v == "" {
		return v
	}
	return string(v[0]-32) + v[1:]
}

// newDshScreen builds the sectioned Dsh pane — the Dsh → Desktop / Web
// sub-views of the GUI (sessions follow with tui-parity P3).
func newDshScreen(ctx context.Context) (Screen, error) {
	list := func(source func() ListSource) func(context.Context) (Screen, error) {
		return func(ctx context.Context) (Screen, error) {
			return newListScreen(ctx, source())
		}
	}
	return newSectionedScreen(ctx, "Dsh", []screenSection{
		{key: "desktop", title: "Desktop", open: list(DshDesktopSource)},
		{key: "web", title: "Web", open: list(DshWebSource)},
		{key: "sessions", title: "Sessions", open: list(DshSessionsSource)},
	})
}
