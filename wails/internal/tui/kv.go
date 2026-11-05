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

// kvAction is a previewable write bound to one rune key of a kv screen.
// Actions are row-aware: onPrefix restricts them to selected rows whose key
// starts with it ("" = any row), and run receives that row — that is how the
// Droid settings-file rows get use/delete while the value rows get cycles
// and prompts. kv screens without actions stay read-only.
type kvAction struct {
	key      string
	label    string
	onPrefix string
	// run executes the write directly; nil when prompt is set.
	run func(row kvRow) error
	// prompt asks for a value first (input-class writes).
	prompt *kvPrompt
}

// promptAction is the compact constructor for an input-class kvAction.
func promptAction(key string, label string, prompt kvPrompt) kvAction {
	return kvAction{key: key, label: label, prompt: &prompt}
}

// kvPrompt is the input-class variant of a kvAction (see promptSpec).
type kvPrompt struct {
	title      string
	hint       string
	allowEmpty bool
	initial    func(row kvRow) string
	submit     func(row kvRow, value string) writeAction
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
	actions  []kvAction
	// rebuild re-reads the rows after a writeFlow applied (nil = static).
	rebuild func() []kvRow

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

// withActions binds write actions to rune keys (shown as status hints).
func (s *kvScreen) withActions(actions ...kvAction) *kvScreen {
	s.actions = actions
	parts := make([]string, 0, len(actions))
	seen := map[string]bool{}
	for _, action := range actions {
		if seen[action.label] {
			continue // one hint per label (per-row actions repeat it)
		}
		seen[action.label] = true
		parts = append(parts, action.key+" "+action.label)
	}
	hint := strings.Join(parts, " · ")
	if s.status != "" {
		s.status += " · " + hint
	} else {
		s.status = hint
	}
	return s
}

// withRebuild stores the row builder so the screen can refresh itself after
// a write applied.
func (s *kvScreen) withRebuild(fn func() []kvRow) *kvScreen {
	s.rebuild = fn
	return s
}

// Refresh implements the post-write reload the writeFlow asks for.
func (s *kvScreen) Refresh() {
	if s.rebuild == nil {
		return
	}
	s.rows = s.rebuild()
	if s.selected >= len(s.rows) {
		s.selected = len(s.rows) - 1
	}
	if s.selected < 0 {
		s.selected = 0
	}
}

func (s *kvScreen) Title() string { return s.title }

func (s *kvScreen) Status() string { return s.status }

// selectedRow returns the highlighted row ("" when the pane is empty).
func (s *kvScreen) selectedRow() (kvRow, bool) {
	if s.selected < 0 || s.selected >= len(s.rows) {
		return kvRow{}, false
	}
	return s.rows[s.selected], true
}

func (s *kvScreen) Update(msg tea.Msg) (Screen, tea.Cmd) {
	key, ok := msg.(tea.KeyMsg)
	if !ok {
		return s, nil
	}
	name := key.String()
	if len(name) == 1 {
		for _, action := range s.actions {
			if action.key != name {
				continue
			}
			row, has := s.selectedRow()
			if action.onPrefix != "" && (!has || !strings.HasPrefix(row.key, action.onPrefix)) {
				continue
			}
			title := action.label
			if action.onPrefix != "" {
				title += " · " + strings.TrimPrefix(row.key, action.onPrefix)
			}
			if action.prompt != nil {
				initial := ""
				if action.prompt.initial != nil {
					initial = action.prompt.initial(row)
				}
				return newPromptScreen(s, promptSpec{
					title:      action.prompt.title,
					hint:       action.prompt.hint,
					allowEmpty: action.prompt.allowEmpty,
					initial:    initial,
					submit: func(value string) writeAction {
						return action.prompt.submit(row, value)
					},
				}, s.Refresh), nil
			}
			target := row
			return newWriteFlow(s, writeAction{
				title: title,
				run: func() error {
					return action.run(target)
				},
			}, s.Refresh), nil
		}
	}
	switch name {
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
