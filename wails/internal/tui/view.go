package tui

import (
	"strings"

	"github.com/charmbracelet/bubbletea"
)

// viewScreen shows the result of a view action (channel tokens, later any
// fetch-and-display operation): two documents — the pre-masked one and the
// revealed one — so toggling secrets never refetches. Like the writeFlow it
// owns every key and hands the origin screen back on esc/q (KeySink).
type viewScreen struct {
	back   Screen
	title  string
	masked string
	full   string
	reveal bool
	status string
}

func newViewScreen(back Screen, title string, masked []byte, full []byte) *viewScreen {
	return &viewScreen{
		back:   back,
		title:  title,
		masked: string(masked),
		full:   string(full),
		status: "r reveal secrets · esc back",
	}
}

// IsKeySink marks the screen as the key owner (see tui.go).
func (s *viewScreen) IsKeySink() bool { return true }

func (s *viewScreen) Title() string { return s.title }

func (s *viewScreen) Status() string { return s.status }

func (s *viewScreen) Update(msg tea.Msg) (Screen, tea.Cmd) {
	key, ok := msg.(tea.KeyMsg)
	if !ok {
		return s, nil
	}
	switch key.String() {
	case "esc", "q", "enter":
		return s.back, nil
	case "r":
		s.reveal = !s.reveal
		if s.reveal {
			s.status = "r hide secrets · esc back"
		} else {
			s.status = "r reveal secrets · esc back"
		}
	}
	return s, nil
}

func (s *viewScreen) View(width, height int) string {
	header := s.title
	if s.reveal {
		header += " (secrets visible)"
	}
	body := s.masked
	if s.reveal {
		body = s.full
	}
	lines := strings.Split(body, "\n")
	visible := height - 3
	if visible < 1 {
		visible = 1
	}
	if len(lines) > visible {
		lines = lines[:visible]
	}
	for i, line := range lines {
		lines[i] = truncate(line, width)
	}
	return titleStyle.Render(header) + "\n\n" + strings.Join(lines, "\n")
}
