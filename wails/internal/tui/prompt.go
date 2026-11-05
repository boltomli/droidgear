package tui

import (
	"strings"

	"github.com/charmbracelet/bubbletea"
)

// promptSpec configures the one-line text input behind every input-class
// write (save an auth profile under a name, point at a JSON file, type a
// path …). Submitting hands control to the writeFlow, so even prompted
// operations keep the preview → confirm → conflict contract.
type promptSpec struct {
	// title is the pane heading ("save current auth as …").
	title string
	// hint is the dimmed guidance below the input (valid values, format).
	hint string
	// initial pre-fills the buffer (renames start from the current label).
	initial string
	// allowEmpty lets enter submit an empty value (resetting a font/shell
	// preference back to the terminal default).
	allowEmpty bool
	// submit builds the write action for the entered value.
	submit func(value string) writeAction
}

// promptScreen is the shared text input. Like the writeFlow it is a KeySink:
// it owns every key and hands the origin screen back on esc/q.
type promptScreen struct {
	back    Screen
	spec    promptSpec
	refresh func()

	buffer string
	status string
}

func newPromptScreen(back Screen, spec promptSpec, refresh func()) *promptScreen {
	return &promptScreen{
		back:    back,
		spec:    spec,
		refresh: refresh,
		buffer:  spec.initial,
		status:  spec.title + " · enter save (preview first) · esc cancel",
	}
}

// IsKeySink marks the screen as the key owner (see tui.go).
func (s *promptScreen) IsKeySink() bool { return true }

func (s *promptScreen) Title() string { return s.spec.title }

func (s *promptScreen) Status() string { return s.status }

func (s *promptScreen) Update(msg tea.Msg) (Screen, tea.Cmd) {
	key, ok := msg.(tea.KeyMsg)
	if !ok {
		return s, nil
	}
	switch key.Type {
	case tea.KeyEnter:
		value := strings.TrimSpace(s.buffer)
		if value == "" && !s.spec.allowEmpty {
			s.status = s.spec.title + " · a value is required · esc cancel"
			return s, nil
		}
		return newWriteFlow(s.back, s.spec.submit(value), s.refresh), nil
	case tea.KeyEsc, tea.KeyCtrlC:
		return s.back, nil
	case tea.KeyBackspace, tea.KeyDelete:
		if runes := []rune(s.buffer); len(runes) > 0 {
			s.buffer = string(runes[:len(runes)-1])
		}
	case tea.KeyRunes:
		// Plain typing (and paste — KeyRunes carries the whole run), so a
		// literal "q" is text here, never a quit key.
		s.buffer += string(key.Runes)
	case tea.KeySpace:
		if len(key.Runes) > 0 {
			s.buffer += string(key.Runes)
		} else {
			s.buffer += " "
		}
	}
	return s, nil
}

func (s *promptScreen) View(width, height int) string {
	var b strings.Builder
	b.WriteString(titleStyle.Render(s.spec.title))
	b.WriteString("\n\n")
	b.WriteString(selectedStyle.Render(truncate(s.buffer+"│", width-2)))
	b.WriteString("\n\n")
	if s.spec.hint != "" {
		b.WriteString(dimStyle.Render(truncate(s.spec.hint, width)))
		b.WriteString("\n")
	}
	b.WriteString(dimStyle.Render("type a value · enter saves through a preview · esc cancels"))
	return b.String()
}

// splitFirstSpace splits "first rest-of-the-line": names/usernames never
// carry spaces, everything after the first space belongs to the value
// (labels and passwords may contain them).
func splitFirstSpace(value string) (first string, rest string) {
	if i := strings.IndexByte(value, ' '); i >= 0 {
		return value[:i], value[i+1:]
	}
	return value, ""
}
