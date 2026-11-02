// Window state safety commands. Wails spike has no window-state plugin, so
// "reset" restores a safe default geometry (mirrors the intent of
// commands/window.rs::reset_window_state: clear fullscreen, sane size,
// centre, focus).
package app

import (
	"github.com/wailsapp/wails/v3/pkg/application"
)

// resetWidth/resetHeight match the window contract (default = minimum,
// 1150×700 — see wails/PORTING.md and wails/main.go).
const (
	resetWidth  = 1150
	resetHeight = 700
)

// ResetWindowState restores the main window to a safe, visible state.
func (a *App) ResetWindowState() error {
	win, ok := application.Get().Window.Get("main")
	if !ok {
		return errString("Main window not found")
	}
	if win.IsFullscreen() {
		win.UnFullscreen()
	}
	win.SetSize(resetWidth, resetHeight)
	win.Center()
	win.Focus()
	return nil
}
