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

// DefaultModules is the P0 module set: paths plus the four profile domains
// already extracted into internal/service. Parity adds the rest as their
// service packages land (docs/developer/tui-parity.zh.md §2).
func DefaultModules() []Module {
	return []Module{
		{
			Key:   "paths",
			Title: "Paths",
			Open: func(ctx context.Context) (Screen, error) {
				return newPathsScreen(ctx)
			},
		},
		{
			Key:   "codex",
			Title: "Codex",
			Open: func(ctx context.Context) (Screen, error) {
				return newProfilesScreen(ctx, CodexDomain())
			},
		},
		{
			Key:   "hermes",
			Title: "Hermes",
			Open: func(ctx context.Context) (Screen, error) {
				return newProfilesScreen(ctx, HermesDomain())
			},
		},
		{
			Key:   "pi",
			Title: "Pi",
			Open: func(ctx context.Context) (Screen, error) {
				return newProfilesScreen(ctx, PiDomain())
			},
		},
		{
			Key:   "omp",
			Title: "OMP",
			Open: func(ctx context.Context) (Screen, error) {
				return newProfilesScreen(ctx, OmpDomain())
			},
		},
	}
}
