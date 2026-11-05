// Dsh sessions commands — the GUI-facing forward; the read path lives in
// internal/service/read (B4) and the poll watcher stays here because it
// owns the Wails event bus.
package app

import (
	"droidgear-wails/internal/service/read"
)

type (
	DshSessionSummary = read.DshSessionSummary
	DshSessionDetail  = read.DshSessionDetail
)

// ListDshSessions lists Dsh sessions, newest first (Rust `list_dsh_sessions`).
func (a *App) ListDshSessions() ([]DshSessionSummary, error) {
	return read.ListDshSessions()
}

// GetDshSessionDetail returns one Dsh session with its transcript.
func (a *App) GetDshSessionDetail(sessionPath string) (DshSessionDetail, error) {
	return read.GetDshSessionDetail(sessionPath)
}

// DeleteDshSession removes a session directory and prunes the empty project.
func (a *App) DeleteDshSession(sessionPath string) error {
	return read.DeleteDshSession(sessionPath)
}

// ---------------------------------------------------------------------------
// Watcher (stays in app: Wails event bus)
// ---------------------------------------------------------------------------

// StartDshSessionsWatcher starts the Dsh sessions poll watcher; the
// frontend listens for `dsh-sessions-changed` (Rust
// `start_dsh_sessions_watcher`).
func (a *App) StartDshSessionsWatcher() error {
	dir, err := read.DshSessionsDir()
	if err != nil {
		return err
	}
	return startSessionsPollWatcher("dsh-sessions-changed", dir)
}

// StopDshSessionsWatcher stops the Dsh sessions watcher (Rust
// `stop_dsh_sessions_watcher`).
func (a *App) StopDshSessionsWatcher() error {
	stopSessionsPollWatcher("dsh-sessions-changed")
	return nil
}
