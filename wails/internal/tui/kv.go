package tui

import (
	"fmt"
	"strings"

	"github.com/charmbracelet/bubbletea"
)

// kvRow is one key/value line of a kvScreen.
type kvRow struct {
	section string
	key     string
	value   string
}

// kvScreen is the shared read-only key/value pane (Preferences, Droid
// settings, Missions, Legacy …): sections on top of plain rows, selectable
// like the list screen so one set of navigation keys works everywhere.
type kvScreen struct {
	title string
	rows  []kvRow
	// trailing renders dimmed footer lines (file paths, hints).
	trailing []string
	status   string

	selected int
	offset   int
	err      error
}

func newKVScreen(title string, rows []kvRow) *kvScreen {
	return &kvScreen{title: title, rows: rows}
}

// withTrailing appends dimmed footer lines (e.g. the document path).
func (s *kvScreen) withTrailing(lines ...string) *kvScreen {
	s.trailing = lines
	return s
}

// withStatus sets the bottom-bar line ("" = caller default).
func (s *kvScreen) withStatus(status string) *kvScreen {
	s.status = status
	return s
}

func (s *kvScreen) Title() string { return s.title }

func (s *kvScreen) Status() string { return s.status }

func (s *kvScreen) Update(msg tea.Msg) (Screen, tea.Cmd) {
	key, ok := msg.(tea.KeyMsg)
	if !ok {
		return s, nil
	}
	switch key.String() {
	case "up", "k":
		if s.selected > 0 {
			s.selected--
		}
	case "down", "j":
		if s.selected < len(s.rows)-1 {
			s.selected++
		}
	}
	return s, nil
}

func (s *kvScreen) View(width, height int) string {
	if s.err != nil {
		return errStyle.Render("failed to load " + s.title + ": " + s.err.Error())
	}

	var b strings.Builder
	b.WriteString(titleStyle.Render(s.title))
	b.WriteString("\n\n")

	footer := 0
	if len(s.trailing) > 0 {
		footer = 1 + len(s.trailing)
	}
	visible := height - 3 - footer
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

	lastSection := ""
	for i := s.offset; i < end; i++ {
		row := s.rows[i]
		if row.section != lastSection {
			if lastSection != "" {
				if visible < 2 {
					break
				}
				b.WriteString("\n")
			}
			b.WriteString(dimStyle.Render("["+row.section+"]") + "\n")
			lastSection = row.section
		}
		marker := " "
		if i == s.selected {
			marker = ">"
		}
		line := fmt.Sprintf("%s %-24s %s", marker, row.key, row.value)
		if i == s.selected {
			b.WriteString(selectedStyle.Render(truncate(line, width)) + "\n")
		} else {
			b.WriteString(truncate(line, width) + "\n")
		}
	}

	for _, line := range s.trailing {
		b.WriteString("\n" + dimStyle.Render(truncate(line, width)))
	}
	return b.String()
}
