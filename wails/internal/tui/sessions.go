package tui

import (
	"context"
	"encoding/json"
	"fmt"
	"path/filepath"

	"droidgear-wails/internal/service/read"
)

// sessionsSource adapts one tool's session list to the generic list screen
// (Droid → Sessions, Codex → Sessions, Pi/OMP/Dsh → Sessions — all three
// summary shapes share title/project/model/path, so only the three closures
// differ).
type sessionsSource struct {
	title  string
	load   func() ([]ListRow, error)
	detail func(path string) ([]byte, error)
	// delete wires the 'd' action through the preview flow (nil = read-only).
	delete func(path string) error
}

// Actions exposes session deletion through the writeFlow (P2).
func (s *sessionsSource) Actions() []ListAction {
	if s.delete == nil {
		return nil
	}
	del := s.delete
	return []ListAction{{
		Key: "d", Label: "delete",
		Run: func(_ context.Context, row ListRow) error { return del(row.ID) },
	}}
}

func (s *sessionsSource) Title() string { return s.title }

func (s *sessionsSource) Load(context.Context) ([]ListRow, error) {
	return s.load()
}

func (s *sessionsSource) Detail(_ context.Context, path string) ([]byte, error) {
	return s.detail(path)
}

func (s *sessionsSource) Summary(rows []ListRow) string {
	return fmt.Sprintf("%s · %d sessions · r toggle secrets", s.title, len(rows))
}

// sessionRows normalises any summary shape: the row label is the session
// title (falling back to the id) and the id is the session path the detail
// call needs.
func sessionRows(count int, get func(i int) (path string, title string)) []ListRow {
	rows := make([]ListRow, 0, count)
	for i := 0; i < count; i++ {
		path, title := get(i)
		if title == "" {
			title = filepath.Base(path)
		}
		rows = append(rows, ListRow{ID: path, Label: title})
	}
	return rows
}

// marshalDetail pretty-prints any detail struct for the detail pane (the
// screen redacts and truncates it).
func marshalSessionDetail(value any) ([]byte, error) {
	return json.MarshalIndent(value, "", "  ")
}

// DroidSessionsSource exposes the generic Factory/Droid sessions (Droid →
// Sessions).
func DroidSessionsSource() ListSource {
	return &sessionsSource{
		title:  "Sessions",
		delete: read.DeleteSession,
		load: func() ([]ListRow, error) {
			sessions, err := read.ListSessions(nil)
			if err != nil {
				return nil, err
			}
			return sessionRows(len(sessions), func(i int) (string, string) {
				return sessions[i].Path, sessions[i].Title
			}), nil
		},
		detail: func(path string) ([]byte, error) {
			detail, err := read.GetSessionDetail(path)
			if err != nil {
				return nil, err
			}
			return marshalSessionDetail(detail)
		},
	}
}

// CodexSessionsSource exposes Codex sessions (Codex → Sessions).
func CodexSessionsSource() ListSource {
	return &sessionsSource{
		title:  "Codex Sessions",
		delete: read.DeleteCodexSession,
		load: func() ([]ListRow, error) {
			sessions, err := read.ListCodexSessions()
			if err != nil {
				return nil, err
			}
			return sessionRows(len(sessions), func(i int) (string, string) {
				return sessions[i].Path, sessions[i].Title
			}), nil
		},
		detail: func(path string) ([]byte, error) {
			detail, err := read.GetCodexSessionDetail(path)
			if err != nil {
				return nil, err
			}
			return marshalSessionDetail(detail)
		},
	}
}

// PiSessionsSource exposes Pi sessions (Pi → Sessions).
func PiSessionsSource() ListSource {
	return &sessionsSource{
		title:  "Pi Sessions",
		delete: read.DeletePiSession,
		load: func() ([]ListRow, error) {
			sessions, err := read.ListPiSessions()
			if err != nil {
				return nil, err
			}
			return sessionRows(len(sessions), func(i int) (string, string) {
				return sessions[i].Path, sessions[i].Title
			}), nil
		},
		detail: func(path string) ([]byte, error) {
			detail, err := read.GetPiSessionDetail(path)
			if err != nil {
				return nil, err
			}
			return marshalSessionDetail(detail)
		},
	}
}

// OmpSessionsSource exposes OMP sessions (OMP → Sessions).
func OmpSessionsSource() ListSource {
	return &sessionsSource{
		title:  "OMP Sessions",
		delete: read.DeleteOmpSession,
		load: func() ([]ListRow, error) {
			sessions, err := read.ListOmpSessions()
			if err != nil {
				return nil, err
			}
			return sessionRows(len(sessions), func(i int) (string, string) {
				return sessions[i].Path, sessions[i].Title
			}), nil
		},
		detail: func(path string) ([]byte, error) {
			detail, err := read.GetOmpSessionDetail(path)
			if err != nil {
				return nil, err
			}
			return marshalSessionDetail(detail)
		},
	}
}

// DshSessionsSource exposes Dsh sessions (Dsh → Sessions).
func DshSessionsSource() ListSource {
	return &sessionsSource{
		title:  "Dsh Sessions",
		delete: read.DeleteDshSession,
		load: func() ([]ListRow, error) {
			sessions, err := read.ListDshSessions()
			if err != nil {
				return nil, err
			}
			return sessionRows(len(sessions), func(i int) (string, string) {
				return sessions[i].Path, sessions[i].Title
			}), nil
		},
		detail: func(path string) ([]byte, error) {
			detail, err := read.GetDshSessionDetail(path)
			if err != nil {
				return nil, err
			}
			return marshalSessionDetail(detail)
		},
	}
}
