package tui

import (
	"context"
	"fmt"

	"github.com/charmbracelet/bubbletea"
)

// screenSection is one tab of a sectioned screen — the TUI counterpart of a
// GUI tool's sub-views (Droid: settings/missions/trusted/auth/mcp/models/
// legacy; Dsh: desktop/web; …).
type screenSection struct {
	key   string
	title string
	open  func(ctx context.Context) (Screen, error)
}

// sectionedScreen hosts a screen per section: tab/shift+tab (or 1..N) swaps
// the section, every other key goes to the active section's screen.
type sectionedScreen struct {
	ctx      context.Context
	title    string
	sections []screenSection
	cur      int
	body     Screen
	err      error
}

func newSectionedScreen(ctx context.Context, title string, sections []screenSection) (Screen, error) {
	s := &sectionedScreen{ctx: ctx, title: title, sections: sections}
	s.openCurrent()
	return s, nil
}

func (s *sectionedScreen) openCurrent() {
	body, err := s.sections[s.cur].open(s.ctx)
	if err != nil {
		s.err = err
		s.body = nil
		return
	}
	s.err = nil
	s.body = body
}

func (s *sectionedScreen) Title() string { return s.title }

func (s *sectionedScreen) Status() string {
	section := s.sections[s.cur]
	if s.err != nil {
		return fmt.Sprintf("%s · %s · error: %v", s.title, section.title, s.err)
	}
	status := section.title
	if s.body != nil {
		if st := s.body.Status(); st != "" {
			status = st
		}
	}
	return fmt.Sprintf("%s · %d/%d tab sections", status, s.cur+1, len(s.sections))
}

func (s *sectionedScreen) Update(msg tea.Msg) (Screen, tea.Cmd) {
	key, ok := msg.(tea.KeyMsg)
	if !ok {
		if s.body != nil {
			var cmd tea.Cmd
			s.body, cmd = s.body.Update(msg)
			return s, cmd
		}
		return s, nil
	}

	name := key.String()
	switch {
	case name == "tab":
		s.cur = (s.cur + 1) % len(s.sections)
		s.openCurrent()
		return s, nil
	case name == "shift+tab":
		s.cur = (s.cur + len(s.sections) - 1) % len(s.sections)
		s.openCurrent()
		return s, nil
	default:
		if len(name) == 1 && name >= "1" && name <= "9" {
			idx := int(name[0] - '1')
			if idx < len(s.sections) {
				if idx != s.cur {
					s.cur = idx
					s.openCurrent()
				}
				return s, nil
			}
		}
	}

	if s.body != nil {
		var cmd tea.Cmd
		s.body, cmd = s.body.Update(msg)
		return s, cmd
	}
	return s, nil
}

func (s *sectionedScreen) View(width, height int) string {
	if s.err != nil {
		return errStyle.Render(fmt.Sprintf("%s %s: %v", s.title, s.sections[s.cur].title, s.err))
	}
	if s.body == nil {
		return dimStyle.Render("loading…")
	}
	return s.body.View(width, height)
}
