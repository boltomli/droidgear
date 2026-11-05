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

// KeySink marks a screen that consumes navigation keys itself (the write
// preview flow): the root model forwards everything except ctrl+c, and the
// screen decides when to hand control back.
type KeySink interface {
	IsKeySink() bool
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

// DefaultModules is the module set shipped so far: the GUI's left栏 in the
// same order (droid, channels, codex, claude, hermes, pi, omp, copilot, dsh)
// followed by the Preferences panes that have their own TUI screen. Parity
// adds a row per GUI view as its service package lands — the source of truth
// for what is still missing is the matrix in docs/developer/tui-parity.zh.md §2.
func DefaultModules() []Module {
	specs := []struct {
		key   string
		title string
		open  func(context.Context) (Screen, error)
	}{
		{"droid", "Droid", newDroidScreen},
		{"channels", "Channels", newChannelsScreen},
		{"codex", "Codex", newCodexScreen},
		{"claude", "Claude", list(ClaudeSource)},
		{"hermes", "Hermes", list(HermesSource)},
		{"pi", "Pi", newPiScreen},
		{"omp", "OMP", newOmpScreen},
		{"copilot", "Copilot", list(CopilotSource)},
		{"dsh", "Dsh", newDshScreen},
		{"settings", "Settings", newSettingsScreen},
		{"registry", "Registry", list(RegistrySource)},
		{"paths", "Paths", newPathsScreen},
	}

	modules := make([]Module, 0, len(specs))
	for _, spec := range specs {
		spec := spec
		modules = append(modules, Module{
			Key:   spec.key,
			Title: spec.title,
			Open:  spec.open,
		})
	}
	return modules
}
