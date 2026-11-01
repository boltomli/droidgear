// Window geometry persistence for the Wails shell — there is no
// window-state plugin, so x/y/size/maximised are stored in
// ~/.droidgear/window-state.json. The file and schema are deliberately
// Wails-specific: the Tauri build keeps tauri-plugin-window-state data
// under %APPDATA%\com.droidgear.app, and sharing one file between two
// different schemas would corrupt both.
//
// Restoring happens in main.go (geometry is applied to the window options)
// and AttachWindowState persists subsequent moves/resizes (debounced) plus
// a final flush on close and shutdown (guidelines §3.8).
package app

import (
	"encoding/json"
	"os"
	"path/filepath"
	"sync"
	"time"

	"droidgear-wails/internal/core"

	"github.com/wailsapp/wails/v3/pkg/application"
	"github.com/wailsapp/wails/v3/pkg/events"
)

// WindowState mirrors the geometry fields persisted between runs.
type WindowState struct {
	X         int  `json:"x"`
	Y         int  `json:"y"`
	Width     int  `json:"width"`
	Height    int  `json:"height"`
	Maximized bool `json:"maximized"`
}

// windowStatePath returns ~/.droidgear/window-state.json.
func windowStatePath() (string, error) {
	dir, err := core.DroidGearDir()
	if err != nil {
		return "", err
	}
	return filepath.Join(dir, "window-state.json"), nil
}

// LoadWindowState reads the persisted geometry. It returns nil when the
// file is absent, unreadable or nonsensical — the caller then keeps the
// default (centred 1280×840) geometry.
func LoadWindowState() *WindowState {
	path, err := windowStatePath()
	if err != nil {
		return nil
	}
	data, err := os.ReadFile(path) // #nosec G304 -- fixed path under ~/.droidgear
	if err != nil {
		return nil
	}
	var state WindowState
	if err := json.Unmarshal(data, &state); err != nil {
		return nil
	}
	if state.Width <= 0 || state.Height <= 0 {
		return nil
	}
	return &state
}

// saveWindowState persists the geometry best-effort: a failing write must
// never break a move, close or shutdown.
func saveWindowState(state WindowState) {
	path, err := windowStatePath()
	if err != nil {
		return
	}
	data, err := json.MarshalIndent(state, "", "  ")
	if err != nil {
		return
	}
	_ = core.AtomicWrite(path, data)
}

var (
	windowStateMu  sync.Mutex
	attachedWindow *application.WebviewWindow
	attachedFlush  *time.Timer
	debounceWindow = 400 * time.Millisecond
)

// AttachWindowState maximises the window on first show when the saved
// state says so, and persists subsequent moves/resizes with a debounced
// write plus a synchronous flush when the window closes. The window is
// also recorded so CleanupOnShutdown can flush the final geometry.
func AttachWindowState(win *application.WebviewWindow, state *WindowState) {
	windowStateMu.Lock()
	attachedWindow = win
	windowStateMu.Unlock()

	if state != nil && state.Maximized {
		// One-shot: maximise on first show, then unsubscribe so later
		// un-minimises do not re-maximise a window the user restored.
		var unsubscribe func()
		unsubscribe = win.OnWindowEvent(events.Common.WindowShow, func(*application.WindowEvent) {
			win.Maximise()
			if unsubscribe != nil {
				unsubscribe()
			}
		})
	}

	flush := func() {
		rect := win.Bounds()
		saveWindowState(WindowState{
			X:         rect.X,
			Y:         rect.Y,
			Width:     rect.Width,
			Height:    rect.Height,
			Maximized: win.IsMaximised(),
		})
	}
	schedule := func() {
		windowStateMu.Lock()
		defer windowStateMu.Unlock()
		if attachedFlush == nil {
			attachedFlush = time.AfterFunc(debounceWindow, flush)
		} else {
			attachedFlush.Reset(debounceWindow)
		}
	}

	win.OnWindowEvent(events.Common.WindowDidMove, func(*application.WindowEvent) { schedule() })
	win.OnWindowEvent(events.Common.WindowDidResize, func(*application.WindowEvent) { schedule() })
	win.OnWindowEvent(events.Common.WindowClosing, func(*application.WindowEvent) {
		windowStateMu.Lock()
		if attachedFlush != nil {
			attachedFlush.Stop()
			attachedFlush = nil
		}
		windowStateMu.Unlock()
		flush()
	})
}

// flushAttachedWindowState writes the current geometry on the shutdown
// path (e.g. Quit from the menu, where no WindowClosing event fires first).
func flushAttachedWindowState() {
	windowStateMu.Lock()
	win := attachedWindow
	if attachedFlush != nil {
		attachedFlush.Stop()
		attachedFlush = nil
	}
	windowStateMu.Unlock()
	if win == nil {
		return
	}
	rect := win.Bounds()
	saveWindowState(WindowState{
		X:         rect.X,
		Y:         rect.Y,
		Width:     rect.Width,
		Height:    rect.Height,
		Maximized: win.IsMaximised(),
	})
}
