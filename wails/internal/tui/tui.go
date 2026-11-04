// Package tui implements the DroidGear terminal UI (bubbletea).
//
// The TUI is a consumer of internal/service exactly like cmd/dg and the Wails
// app layer: it never reads or writes configuration files itself (layering
// rule R4 — see scripts/check-go-layering.mjs) and it renders the same data
// the GUI shows, minus the GUI-only surfaces listed in
// docs/developer/tui-parity.zh.md §1 (window, menu, notifications, PTY,
// updater, fonts).
//
// Layout mirrors the GUI: a module list on the left, the selected screen on
// the right, a status line and a help line at the bottom.
package tui

import (
	"context"
	"fmt"
	"strings"

	"github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/lipgloss"
)

const (
	sidebarWidth = 24
	minWidth     = 60
	minHeight    = 16
)

// focus is which pane receives arrow keys.
type focus int

const (
	focusSidebar focus = iota
	focusContent
)

// Model is the root bubbletea model.
type Model struct {
	ctx     context.Context
	modules []Module
	sel     int
	focus   focus
	screen  Screen
	loaded  map[string]bool

	width  int
	height int

	status string
	err    error
}

// New builds the root model with the P0 module set.
func New(ctx context.Context) Model {
	return Model{
		ctx:     ctx,
		modules: DefaultModules(),
		loaded:  map[string]bool{},
		width:   80,
		height:  24,
		status:  "↑↓ select · enter open · q quit",
	}
}

// Run starts the TUI and blocks until the user quits.
func Run(ctx context.Context) error {
	_, err := tea.NewProgram(New(ctx), tea.WithAltScreen()).Run()
	return err
}

func (m Model) Init() tea.Cmd {
	return nil
}

func (m Model) Update(msg tea.Msg) (tea.Model, tea.Cmd) {
	switch msg := msg.(type) {
	case tea.WindowSizeMsg:
		m.width, m.height = msg.Width, msg.Height
		if m.screen != nil {
			var cmd tea.Cmd
			m.screen, cmd = m.screen.Update(msg)
			return m, cmd
		}
		return m, nil

	case tea.KeyMsg:
		// Ctrl+C always quits, in either pane.
		if msg.String() == "ctrl+c" {
			return m, tea.Quit
		}
		if m.focus == focusSidebar {
			return m.updateSidebar(msg)
		}
		return m.updateContent(msg)
	}

	if m.screen != nil {
		var cmd tea.Cmd
		m.screen, cmd = m.screen.Update(msg)
		return m, cmd
	}
	return m, nil
}

func (m Model) updateSidebar(msg tea.KeyMsg) (tea.Model, tea.Cmd) {
	switch msg.String() {
	case "up", "k":
		if m.sel > 0 {
			m.sel--
		}
	case "down", "j":
		if m.sel < len(m.modules)-1 {
			m.sel++
		}
	case "enter", "tab", "l", "right":
		return m.openSelected()
	case "q":
		return m, tea.Quit
	}
	return m, nil
}

func (m Model) updateContent(msg tea.KeyMsg) (tea.Model, tea.Cmd) {
	switch msg.String() {
	case "esc", "h", "left", "q":
		m.focus = focusSidebar
		m.status = "↑↓ select · enter open · q quit"
		return m, nil
	}
	if m.screen == nil {
		m.focus = focusSidebar
		return m, nil
	}
	var cmd tea.Cmd
	m.screen, cmd = m.screen.Update(msg)
	if s := m.screen.Status(); s != "" {
		m.status = s
	}
	return m, cmd
}

// openSelected lazily builds the screen for the highlighted module.
func (m Model) openSelected() (tea.Model, tea.Cmd) {
	if m.sel < 0 || m.sel >= len(m.modules) {
		return m, nil
	}
	module := m.modules[m.sel]
	screen, err := module.Open(m.ctx)
	if err != nil {
		m.err = err
		m.status = fmt.Sprintf("error: %v", err)
		return m, nil
	}
	m.screen = screen
	m.loaded[module.Key] = true
	m.focus = focusContent
	if s := screen.Status(); s != "" {
		m.status = s
	} else {
		m.status = "esc back · q back · ctrl+c quit"
	}
	m.err = nil
	return m, nil
}

// SetScreen swaps the active screen (used by tests and by module reloads).
func (m *Model) SetScreen(screen Screen) {
	m.screen = screen
	m.focus = focusContent
}

// SelectedModule returns the highlighted module key (tests).
func (m Model) SelectedModule() string {
	if m.sel < 0 || m.sel >= len(m.modules) {
		return ""
	}
	return m.modules[m.sel].Key
}

// Focused reports which pane owns the arrow keys (tests).
func (m Model) Focused() focus { return m.focus }

// CurrentScreen returns the active screen, if any (tests).
func (m Model) CurrentScreen() Screen { return m.screen }

var (
	sidebarStyle = lipgloss.NewStyle().
			Width(sidebarWidth).
			Height(0).
			Padding(0, 1)
	selectedStyle = lipgloss.NewStyle().
			Bold(true).
			Foreground(lipgloss.Color("212")).
			Padding(0, 1)
	titleStyle = lipgloss.NewStyle().
			Bold(true).
			Padding(0, 1)
	statusStyle = lipgloss.NewStyle().
			Foreground(lipgloss.Color("244")).
			Padding(0, 1)
	helpStyle = lipgloss.NewStyle().
			Foreground(lipgloss.Color("240")).
			Padding(0, 1)
	dimStyle = lipgloss.NewStyle().
			Foreground(lipgloss.Color("240"))
	errStyle = lipgloss.NewStyle().
			Foreground(lipgloss.Color("196"))
)

// View renders sidebar + content + status/help bars.
func (m Model) View() string {
	if m.width < minWidth || m.height < minHeight {
		return fmt.Sprintf(
			"terminal too small (%d×%d) — need at least %d×%d",
			m.width, m.height, minWidth, minHeight,
		)
	}

	body := m.height - 2 // status + help
	if body < 4 {
		body = 4
	}

	contentWidth := m.width - sidebarWidth
	if m.screen == nil {
		contentWidth = m.width
	}

	var content string
	switch {
	case m.err != nil:
		content = errStyle.Render(m.err.Error())
	case m.screen == nil:
		content = dimStyle.Render(
			"select a module on the left and press enter\n\n" +
				"DroidGear terminal UI — data comes from internal/service,\n" +
				"the same layer the GUI and (until parity) dg use.",
		)
	default:
		content = m.screen.View(contentWidth, body)
	}

	var left string
	if m.screen == nil {
		left = m.renderSidebar(body)
	}

	var out string
	if left != "" {
		rows := make([]string, 0, body)
		leftLines := strings.Split(left, "\n")
		rightLines := strings.Split(content, "\n")
		for i := 0; i < body; i++ {
			l := ""
			if i < len(leftLines) {
				l = leftLines[i]
			}
			r := ""
			if i < len(rightLines) {
				r = rightLines[i]
			}
			rows = append(rows, pad(l, sidebarWidth)+r)
		}
		out = strings.Join(rows, "\n")
	} else {
		out = content
	}

	out += "\n" + statusStyle.Render(truncate(m.status, m.width))
	out += "\n" + helpStyle.Render(
		truncate("↑↓/jk move · enter open · esc back · ctrl+c quit", m.width),
	)
	return out
}

func (m Model) renderSidebar(height int) string {
	lines := make([]string, 0, height)
	lines = append(lines, titleStyle.Render("DroidGear"), "")
	for i, module := range m.modules {
		label := "  " + module.Title
		if i == m.sel {
			label = selectedStyle.Render("> " + module.Title)
		}
		lines = append(lines, label)
	}
	for len(lines) < height {
		lines = append(lines, "")
	}
	return sidebarStyle.Render(strings.Join(lines[:height], "\n"))
}

func pad(s string, width int) string {
	w := lipgloss.Width(s)
	if w >= width {
		return s
	}
	return s + strings.Repeat(" ", width-w)
}

func truncate(s string, width int) string {
	if width <= 2 || lipgloss.Width(s) <= width {
		return s
	}
	runes := []rune(s)
	for len(runes) > 0 && lipgloss.Width(string(runes)) > width-1 {
		runes = runes[:len(runes)-1]
	}
	return string(runes) + "…"
}
