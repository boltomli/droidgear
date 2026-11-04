package tui

import (
	"context"
	"encoding/json"
	"fmt"
	"strings"

	"droidgear-wails/internal/core"
	"droidgear-wails/internal/service/codex"
	"droidgear-wails/internal/service/hermes"
	"droidgear-wails/internal/service/omp"
	"droidgear-wails/internal/service/pi"
	"github.com/charmbracelet/bubbletea"
)

// ProfileRow is one entry of a tool's profile list.
type ProfileRow struct {
	ID   string
	Name string
}

// ProfileDomain adapts one tool's service package to the generic profile
// screen. The four extracted domains (codex/pi/hermes/omp) expose the same
// shape — list, active id, get one profile — so a single screen serves all
// of them instead of four near-identical implementations.
//
// New domains join by writing an adapter, never by copying the screen.
type ProfileDomain interface {
	Key() string
	Title() string
	List(ctx context.Context) ([]ProfileRow, error)
	ActiveID(ctx context.Context) (*string, error)
	// Detail returns the profile as indented JSON. Secrets are redacted by
	// the caller unless the user asks for them (parity §5.3).
	Detail(ctx context.Context, id string) ([]byte, error)
}

type profilesScreen struct {
	ctx      context.Context
	domain   ProfileDomain
	rows     []ProfileRow
	active   string
	selected int
	offset   int

	raw      string
	reveal   bool
	err      error
	status   string
}

func newProfilesScreen(ctx context.Context, domain ProfileDomain) (Screen, error) {
	s := &profilesScreen{ctx: ctx, domain: domain}

	rows, err := domain.List(ctx)
	if err != nil {
		s.err = err
		return s, nil
	}
	s.rows = rows

	if active, err := domain.ActiveID(ctx); err == nil && active != nil {
		s.active = *active
		// Start on the active profile — that is what the GUI opens too.
		for i, row := range rows {
			if row.ID == *active {
				s.selected = i
				break
			}
		}
	}

	s.loadDetail()
	s.updateStatus()
	return s, nil
}

func (s *profilesScreen) Title() string { return s.domain.Title() }

func (s *profilesScreen) Status() string { return s.status }

func (s *profilesScreen) loadDetail() {
	s.raw = ""
	if s.selected < 0 || s.selected >= len(s.rows) {
		return
	}
	data, err := s.domain.Detail(s.ctx, s.rows[s.selected].ID)
	if err != nil {
		s.err = err
		return
	}
	s.err = nil
	s.raw = string(data)
}

func (s *profilesScreen) updateStatus() {
	active := "none"
	if s.active != "" {
		active = s.active
	}
	s.status = fmt.Sprintf(
		"%s · %d profiles · active: %s · r toggle secrets",
		s.domain.Title(), len(s.rows), active,
	)
}

func (s *profilesScreen) Update(msg tea.Msg) (Screen, tea.Cmd) {
	key, ok := msg.(tea.KeyMsg)
	if !ok {
		return s, nil
	}
	switch key.String() {
	case "up", "k":
		if s.selected > 0 {
			s.selected--
			s.loadDetail()
		}
	case "down", "j":
		if s.selected < len(s.rows)-1 {
			s.selected++
			s.loadDetail()
		}
	case "r":
		s.reveal = !s.reveal
	}
	return s, nil
}

func (s *profilesScreen) View(width, height int) string {
	if s.err != nil && s.raw == "" {
		return errStyle.Render(s.domain.Title() + ": " + s.err.Error())
	}

	left := sidebarWidth
	if width <= left+20 {
		left = width / 3
	}
	listWidth := left
	detailWidth := width - listWidth - 1
	if detailWidth < 20 {
		detailWidth = 20
	}

	list := s.renderList(listWidth, height)
	detail := s.renderDetail(detailWidth, height)

	listLines := strings.Split(list, "\n")
	detailLines := strings.Split(detail, "\n")
	rows := make([]string, 0, height)
	for i := 0; i < height; i++ {
		l, r := "", ""
		if i < len(listLines) {
			l = listLines[i]
		}
		if i < len(detailLines) {
			r = detailLines[i]
		}
		rows = append(rows, pad(l, listWidth)+"│"+r)
	}
	return strings.Join(rows, "\n")
}

func (s *profilesScreen) renderList(width, height int) string {
	var b strings.Builder
	b.WriteString(titleStyle.Render(s.domain.Title()))
	b.WriteString("\n\n")

	visible := height - 3
	if visible < 1 {
		visible = 1
	}
	if s.selected < s.offset {
		s.offset = s.selected
	}
	if s.selected >= s.offset+visible {
		s.offset = s.selected - visible + 1
	}
	end := s.offset + visible
	if end > len(s.rows) {
		end = len(s.rows)
	}

	if len(s.rows) == 0 {
		b.WriteString(dimStyle.Render("no profiles"))
	}
	for i := s.offset; i < end; i++ {
		row := s.rows[i]
		marker := "  "
		label := row.Name
		if row.ID == s.active {
			marker = "● "
		}
		if i == s.selected {
			line := marker + label
			b.WriteString(selectedStyle.Render(truncate(line, width)))
		} else {
			b.WriteString(truncate(marker+label, width))
		}
		b.WriteString("\n")
	}
	return b.String()
}

func (s *profilesScreen) renderDetail(width, height int) string {
	if s.raw == "" {
		return dimStyle.Render("no profile selected")
	}
	body := s.raw
	if !s.reveal {
		body = core.RedactSensitiveText(body)
	}
	lines := strings.Split(body, "\n")
	if len(lines) > height {
		lines = lines[:height]
	}
	for i, line := range lines {
		lines[i] = truncate(line, width)
	}
	header := "profile"
	if s.reveal {
		header = "profile (secrets visible)"
	}
	return titleStyle.Render(header) + "\n\n" + strings.Join(lines, "\n")
}

// --- domain adapters -------------------------------------------------------

// codexDomain adapts internal/service/codex.
type codexDomain struct{}

func CodexDomain() ProfileDomain { return codexDomain{} }

func (codexDomain) Key() string   { return "codex" }
func (codexDomain) Title() string { return "Codex" }

func (codexDomain) List(ctx context.Context) ([]ProfileRow, error) {
	profiles, err := codex.ListProfiles()
	if err != nil {
		return nil, err
	}
	rows := make([]ProfileRow, 0, len(profiles))
	for _, p := range profiles {
		rows = append(rows, ProfileRow{ID: p.ID, Name: p.Name})
	}
	return rows, nil
}

func (codexDomain) ActiveID(context.Context) (*string, error) {
	return codex.GetActiveProfileID()
}

func (codexDomain) Detail(_ context.Context, id string) ([]byte, error) {
	return marshalDetail(id, codex.GetProfile)
}

// hermesDomain adapts internal/service/hermes.
type hermesDomain struct{}

func HermesDomain() ProfileDomain { return hermesDomain{} }

func (hermesDomain) Key() string   { return "hermes" }
func (hermesDomain) Title() string { return "Hermes" }

func (hermesDomain) List(context.Context) ([]ProfileRow, error) {
	profiles, err := hermes.ListProfiles()
	if err != nil {
		return nil, err
	}
	rows := make([]ProfileRow, 0, len(profiles))
	for _, p := range profiles {
		rows = append(rows, ProfileRow{ID: p.ID, Name: p.Name})
	}
	return rows, nil
}

func (hermesDomain) ActiveID(context.Context) (*string, error) {
	return hermes.GetActiveProfileID()
}

func (hermesDomain) Detail(_ context.Context, id string) ([]byte, error) {
	return marshalDetail(id, hermes.GetProfile)
}

// piDomain adapts internal/service/pi.
type piDomain struct{}

func PiDomain() ProfileDomain { return piDomain{} }

func (piDomain) Key() string   { return "pi" }
func (piDomain) Title() string { return "Pi" }

func (piDomain) List(context.Context) ([]ProfileRow, error) {
	profiles, err := pi.ListProfiles()
	if err != nil {
		return nil, err
	}
	rows := make([]ProfileRow, 0, len(profiles))
	for _, p := range profiles {
		rows = append(rows, ProfileRow{ID: p.ID, Name: p.Name})
	}
	return rows, nil
}

func (piDomain) ActiveID(context.Context) (*string, error) {
	return pi.GetActiveProfileID()
}

func (piDomain) Detail(_ context.Context, id string) ([]byte, error) {
	return marshalDetail(id, pi.GetProfile)
}

// ompDomain adapts internal/service/omp.
type ompDomain struct{}

func OmpDomain() ProfileDomain { return ompDomain{} }

func (ompDomain) Key() string   { return "omp" }
func (ompDomain) Title() string { return "OMP" }

func (ompDomain) List(context.Context) ([]ProfileRow, error) {
	profiles, err := omp.ListProfiles()
	if err != nil {
		return nil, err
	}
	rows := make([]ProfileRow, 0, len(profiles))
	for _, p := range profiles {
		rows = append(rows, ProfileRow{ID: p.ID, Name: p.Name})
	}
	return rows, nil
}

func (ompDomain) ActiveID(context.Context) (*string, error) {
	return omp.GetActiveProfileID()
}

func (ompDomain) Detail(_ context.Context, id string) ([]byte, error) {
	return marshalDetail(id, omp.GetProfile)
}

// marshalDetail keeps the four adapters down to one line each: fetch by id,
// pretty-print, let the screen handle redaction.
func marshalDetail[T any](id string, get func(string) (T, error)) ([]byte, error) {
	profile, err := get(id)
	if err != nil {
		return nil, err
	}
	return json.MarshalIndent(profile, "", "  ")
}
