package tui

import (
	"context"

	"github.com/charmbracelet/bubbletea"
)

// Screen is one right-hand pane of the TUI (the equivalent of a GUI
// sub-view). Screens are dumb renderers: every value they show comes from
// internal/service, so a screen never touches the file system directly.
type Screen interface {
	// Title is the pane heading (also used by tests for identification).
	Title() string
	// Update handles a bubbletea message and returns the next state.
	Update(msg tea.Msg) (Screen, tea.Cmd)
	// View renders the pane for the given content box.
	View(width, height int) string
	// Status is the one-line status shown in the bottom bar ("" = keep the
	// caller's default).
	Status() string
}

// Module is one entry of the left-hand list.
type Module struct {
	// Key is stable and machine-readable (parity matrix row id).
	Key string
	// Title is the human label shown in the sidebar.
	Title string
	// Open builds the screen. It runs on selection, so it must stay cheap:
	// local file reads only, no network.
	Open func(ctx context.Context) (Screen, error)
}

// DefaultModules is the module set shipped so far. Parity adds a row per
// GUI view as its service package lands — the source of truth for what is
// still missing is the matrix in docs/developer/tui-parity.zh.md §2.
func DefaultModules() []Module {
	sources := []struct {
		key    string
		title  string
		source func() ListSource
	}{
		{"channels", "Channels", ChannelsSource},
		{"export", "Export", ExportSource},
		{"codex", "Codex", CodexSource},
		{"hermes", "Hermes", HermesSource},
		{"pi", "Pi", PiSource},
		{"omp", "OMP", OmpSource},
		{"mcp", "MCP", McpSource},
	}

	modules := []Module{
		{
			Key:   "paths",
			Title: "Paths",
			Open: func(ctx context.Context) (Screen, error) {
				return newPathsScreen(ctx)
			},
		},
	}
	for _, entry := range sources {
		entry := entry
		modules = append(modules, Module{
			Key:   entry.key,
			Title: entry.title,
			Open: func(ctx context.Context) (Screen, error) {
				return newListScreen(ctx, entry.source())
			},
		})
	}
	return modules
}
