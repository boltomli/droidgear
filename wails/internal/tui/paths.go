package tui

import (
	"context"
	"fmt"
	"strings"

	"droidgear-wails/internal/service/paths"
	"github.com/charmbracelet/bubbletea"
)

// pathsScreen renders the effective configuration paths — the terminal
// counterpart of Preferences → Paths. Writing path overrides is part of the
// preview phase (tui-parity P2); for now the pane is read-only.
type pathsScreen struct {
	rows     []paths.EffectivePath
	wsl      paths.WslInfo
	selected int
	offset   int
	err      error
	status   string
}

func newPathsScreen(ctx context.Context) (Screen, error) {
	effective, err := paths.Effective(ctx)
	if err != nil {
		return &pathsScreen{err: err}, nil
	}
	// WSL info is supplementary (the GUI shows it in Preferences → Paths);
	// a failed probe must not blank the whole screen.
	wsl, _ := paths.Wsl(ctx)
	return &pathsScreen{
		rows: effective.List(),
		wsl:  wsl,
	}, nil
}

func (s *pathsScreen) Title() string { return "Paths" }

func (s *pathsScreen) Status() string { return s.status }

func (s *pathsScreen) Update(msg tea.Msg) (Screen, tea.Cmd) {
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

func (s *pathsScreen) View(width, height int) string {
	if s.err != nil {
		return errStyle.Render("failed to load paths: " + s.err.Error())
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
