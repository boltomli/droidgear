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

// DefaultModules is the module set shipped so far, in the GUI left栏 order:
// Channels (the GUI's standalone first button) then the tool batches —
// batch 1 (codex, droid, hermes, omp, pi), batch 2 (claude, copilot, dsh) —
// followed by the Preferences panes that have their own TUI screen. Batch
// membership and the alphabetical order inside a batch are shared with
// src/components/layout/LeftSideBar.tsx (TOOL_BATCHES) and pinned by
// TestDefaultModulesMatchParityMatrix. opencode / openclaw are deprecated
// (GUI renders them last, behind a divider) and never get a TUI screen.
// Parity adds a row per GUI view as its service package lands — the source of
// truth for what is still missing is the matrix in
// docs/developer/tui-parity.zh.md §2.
func DefaultModules() []Module {
	specs := []struct {
		key   string
		title string
		open  func(context.Context) (Screen, error)
	}{
		{"channels", "Channels", newChannelsScreen},
		// Batch 1: the daily drivers.
		{"codex", "Codex", newCodexScreen},
		{"droid", "Droid", newDroidScreen},
		{"hermes", "Hermes", newHermesScreen},
		{"omp", "OMP", newOmpScreen},
		{"pi", "Pi", newPiScreen},
		// Batch 2.
		{"claude", "Claude", list(ClaudeSource)},
		{"copilot", "Copilot", list(CopilotSource)},
		{"dsh", "Dsh", newDshScreen},
		// Preferences panes.
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
