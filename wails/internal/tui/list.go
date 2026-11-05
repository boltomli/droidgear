package tui

import (
	"context"
	"strings"

	"droidgear-wails/internal/core"
	"github.com/charmbracelet/bubbletea"
)

// ListRow is one entry of a screen's left column.
type ListRow struct {
	ID string
	// Label is what the user reads (profile name, channel name, …).
	Label string
	// Mark is an optional one-rune prefix: active profile, enabled channel.
	Mark string
}

// ListSource adapts one service package to the generic list screen.
//
// Almost every parity screen is "list on the left, detail on the right"
// (profiles, channels, export templates, MCP entries, trusted folders, …),
// so they all share this screen and differ only in the adapter — new domains
// implement ListSource instead of copying the rendering.
type ListSource interface {
	// Title is the pane heading.
	Title() string
	// Load fetches the rows. It runs on open, so it must stay cheap:
	// local file reads only, no network.
	Load(ctx context.Context) ([]ListRow, error)
	// Detail returns the selected row as indented JSON. Secrets are redacted
	// by the screen unless the user asks for them (parity §5.3).
	Detail(ctx context.Context, id string) ([]byte, error)
	// Summary renders the bottom-bar line for the rows Load returned. It is
	// derived from the rows (marks included) so no second read is needed.
	Summary(rows []ListRow) string
}

// ListAction is a previewable write bound to one rune key of the list
// screen (apply profile, delete session, toggle MCP …). Run receives the
// selected row; the screen wraps it in a writeFlow so every action goes
// through the same preview → confirm → apply contract.
type ListAction struct {
	// Key is a single lowercase rune ("a", "d", "t").
	Key string
	// Label is the status-hint word ("apply", "delete").
	Label string
	Run   func(ctx context.Context, row ListRow) error
}

// ActionSource is the optional ListSource extension carrying write actions;
// sources without it stay read-only.
type ActionSource interface {
	Actions() []ListAction
}

// listScreen is the shared list/detail pane.
type listScreen struct {
	ctx    context.Context
	source ListSource

	rows     []ListRow
	selected int
	offset   int

	raw    string
	reveal bool
	err    error
	status string
}

func newListScreen(ctx context.Context, source ListSource) (Screen, error) {
	s := &listScreen{ctx: ctx, source: source}
	s.reload()
	return s, nil
}

// reload re-reads the rows (called on open and after a writeFlow applied).
func (s *listScreen) reload() {
	rows, err := s.source.Load(s.ctx)
	if err != nil {
		s.err = err
		s.status = ""
		return
	}
	s.rows = rows
	if s.selected >= len(s.rows) {
		s.selected = len(s.rows) - 1
	}
	if s.selected < 0 {
		s.selected = 0
	}
	s.loadDetail()
	s.status = s.source.Summary(rows)
	if hints := s.actionHints(); hints != "" {
		s.status += " · " + hints
	}
}

// Refresh implements the post-write reload the writeFlow asks for.
func (s *listScreen) Refresh() { s.reload() }

func (s *listScreen) actions() []ListAction {
	if src, ok := s.source.(ActionSource); ok {
		return src.Actions()
	}
	return nil
}

func (s *listScreen) actionHints() string {
	parts := make([]string, 0, 2)
	for _, action := range s.actions() {
		parts = append(parts, action.Key+" "+action.Label)
	}
	return strings.Join(parts, " · ")
}

func (s *listScreen) Title() string { return s.source.Title() }

func (s *listScreen) Status() string { return s.status }

// revealSource is the optional extension for sources whose detail differs
// between the redacted and the revealed view: values that live under generic
// keys (Dsh `value` fields marked `secret: true`) cannot be recognised by
// render-time redaction, so the source masks them itself.
type revealSource interface {
	ListSource
	DetailFor(ctx context.Context, id string, reveal bool) ([]byte, error)
}

func (s *listScreen) loadDetail() {
	s.raw = ""
	if s.selected < 0 || s.selected >= len(s.rows) {
		return
	}
	var data []byte
	var err error
	if src, ok := s.source.(revealSource); ok {
		data, err = src.DetailFor(s.ctx, s.rows[s.selected].ID, s.reveal)
	} else {
		data, err = s.source.Detail(s.ctx, s.rows[s.selected].ID)
	}
	if err != nil {
		s.err = err
		return
	}
	s.err = nil
	s.raw = string(data)
}

func (s *listScreen) Update(msg tea.Msg) (Screen, tea.Cmd) {
	key, ok := msg.(tea.KeyMsg)
	if !ok {
		return s, nil
	}

	// Write actions open the preview flow (P2); only meaningful with a row.
	name := key.String()
	if len(name) == 1 {
		for _, action := range s.actions() {
			if action.Key != name || s.selected < 0 || s.selected >= len(s.rows) {
				continue
			}
			row := s.rows[s.selected]
			flow := newWriteFlow(s, writeAction{
				title: action.Label + " " + row.Label,
				run: func() error {
					return action.Run(s.ctx, row)
				},
			}, s.reload)
			return flow, nil
		}
	}

	switch name {
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
		if _, ok := s.source.(revealSource); ok {
			s.loadDetail()
		}
	}
	return s, nil
}

func (s *listScreen) View(width, height int) string {
	if s.err != nil && s.raw == "" {
		return errStyle.Render(s.source.Title() + ": " + s.err.Error())
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

func (s *listScreen) renderList(width, height int) string {
	var b strings.Builder
	b.WriteString(titleStyle.Render(s.source.Title()))
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
		b.WriteString(dimStyle.Render("nothing here yet"))
	}
	for i := s.offset; i < end; i++ {
		row := s.rows[i]
		prefix := row.Mark
		if prefix == "" {
			prefix = "  "
		}
		line := prefix + row.Label
		if i == s.selected {
			b.WriteString(selectedStyle.Render(truncate(line, width)))
		} else {
			b.WriteString(truncate(line, width))
		}
		b.WriteString("\n")
	}
	return b.String()
}

func (s *listScreen) renderDetail(width, height int) string {
	if s.raw == "" {
		return dimStyle.Render("no selection")
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
	header := "detail"
	if s.reveal {
		header = "detail (secrets visible)"
	}
	return titleStyle.Render(header) + "\n\n" + strings.Join(lines, "\n")
}
