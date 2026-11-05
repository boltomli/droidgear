package tui

import (
	"context"
	"fmt"
	"strings"

	"droidgear-wails/internal/service/paths"
	"github.com/charmbracelet/bubbletea"
)

// pathsScreen renders the effective configuration paths — the terminal
// counterpart of Preferences → Paths, including the P2 write path: 'e'
// edits an override and 'x' resets one, both through the shared writeFlow
// preview (tui-parity §2).
type pathsScreen struct {
	ctx      context.Context
	rows     []paths.EffectivePath
	wsl      paths.WslInfo
	selected int
	offset   int
	err      error
	status   string

	// edit mode ('e' / enter): a one-line path input.
	editing bool
	buffer  string
}

func newPathsScreen(ctx context.Context) (Screen, error) {
	s := &pathsScreen{ctx: ctx}
	if err := s.reload(); err != nil {
		s.err = err
	}
	return s, nil
}

// reload re-reads the effective paths (open + post-write refresh).
func (s *pathsScreen) reload() error {
	effective, err := paths.Effective(s.ctx)
	if err != nil {
		return err
	}
	// WSL info is supplementary (the GUI shows it in Preferences → Paths);
	// a failed probe must not blank the whole screen.
	wsl, _ := paths.Wsl(s.ctx)
	s.rows = effective.List()
	s.wsl = wsl
	s.err = nil
	if s.selected >= len(s.rows) {
		s.selected = len(s.rows) - 1
	}
	if s.selected < 0 {
		s.selected = 0
	}
	s.status = fmt.Sprintf("%d paths · e set · x reset", len(s.rows))
	return nil
}

// Refresh implements the post-write reload the writeFlow asks for.
func (s *pathsScreen) Refresh() { _ = s.reload() }

func (s *pathsScreen) Title() string { return "Paths" }

func (s *pathsScreen) Status() string { return s.status }

func (s *pathsScreen) selectedRow() (paths.EffectivePath, bool) {
	if s.selected < 0 || s.selected >= len(s.rows) {
		return paths.EffectivePath{}, false
	}
	return s.rows[s.selected], true
}

// selectedRowKey is the key of the highlighted row (tests).
func (s *pathsScreen) selectedRowKey() string {
	if row, ok := s.selectedRow(); ok {
		return row.Key
	}
	return ""
}

func (s *pathsScreen) Update(msg tea.Msg) (Screen, tea.Cmd) {
	key, ok := msg.(tea.KeyMsg)
	if !ok {
		return s, nil
	}
	name := key.String()

	if s.editing {
		return s.updateEdit(name), nil
	}

	row, has := s.selectedRow()
	switch {
	case name == "e" || (name == "enter" && has):
		s.editing = true
		s.buffer = ""
		if has && !row.IsDefault {
			s.buffer = row.Path
		}
		s.status = fmt.Sprintf("set %s · enter save · esc cancel", row.Key)
	case name == "x" && has:
		if row.IsDefault {
			s.status = row.Key + " is already the default"
			return s, nil
		}
		keyID := row.Key
		flow := newWriteFlow(s, writeAction{
			title: "reset " + keyID,
			run: func() error {
				return paths.ResetOverride(s.ctx, keyID)
			},
		}, s.Refresh)
		return flow, nil
	case name == "up", name == "k":
		if s.selected > 0 {
			s.selected--
		}
	case name == "down", name == "j":
		if s.selected < len(s.rows)-1 {
			s.selected++
		}
	}
	return s, nil
}

// updateEdit handles one key of the one-line path input.
func (s *pathsScreen) updateEdit(name string) Screen {
	row, _ := s.selectedRow()
	switch name {
	case "esc":
		s.editing = false
		s.status = fmt.Sprintf("%d paths · e set · x reset", len(s.rows))
	case "enter":
		value := strings.TrimSpace(s.buffer)
		if value == "" {
			s.status = "path required · esc cancel"
			return s
		}
		keyID := row.Key
		s.editing = false
		return newWriteFlow(s, writeAction{
			title: "set " + keyID,
			run: func() error {
				return paths.SaveOverride(s.ctx, keyID, value)
			},
		}, s.Refresh)
	case "backspace":
		if len(s.buffer) > 0 {
			runes := []rune(s.buffer)
			s.buffer = string(runes[:len(runes)-1])
		}
	default:
		if len([]rune(name)) == 1 && name != "enter" && name != "esc" {
			s.buffer += name
		}
	}
	return s
}

func (s *pathsScreen) View(width, height int) string {
	if s.err != nil {
		return errStyle.Render("failed to load paths: " + s.err.Error())
	}

	if s.editing {
		row, _ := s.selectedRow()
		var b strings.Builder
		b.WriteString(titleStyle.Render("Configuration paths"))
		b.WriteString("\n\n")
		b.WriteString(fmt.Sprintf("set %s =\n\n", row.Key))
		b.WriteString(selectedStyle.Render(s.buffer + "│"))
		b.WriteString("\n\n")
		b.WriteString(dimStyle.Render("type the new path · enter save (preview first) · esc cancel"))
		return b.String()
	}

	var b strings.Builder
	b.WriteString(titleStyle.Render("Configuration paths"))
	b.WriteString("\n\n")

	header := fmt.Sprintf("%-14s %-8s %s", "KEY", "DEFAULT", "PATH")
	b.WriteString(dimStyle.Render(header))
	b.WriteString("\n")

	visible := height - 4
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

	for i := s.offset; i < end; i++ {
		row := s.rows[i]
		marker := " "
		if i == s.selected {
			marker = ">"
		}
		defaults := "yes"
		if !row.IsDefault {
			defaults = "override"
		}
		line := fmt.Sprintf("%s %-13s %-8s %s", marker, row.Key, defaults, row.Path)
		if i == s.selected {
			b.WriteString(selectedStyle.Render(truncate(line, width)))
		} else {
			b.WriteString(truncate(line, width))
		}
		b.WriteString("\n")
	}

	if s.wsl.Available {
		b.WriteString("\n" + dimStyle.Render("WSL: "+wslSummary(s.wsl)))
	}
	return b.String()
}

func wslSummary(info paths.WslInfo) string {
	if len(info.Distros) == 0 {
		return "available, no distributions found"
	}
	names := make([]string, 0, len(info.Distros))
	for _, d := range info.Distros {
		names = append(names, d.Name)
	}
	return strings.Join(names, ", ")
}
