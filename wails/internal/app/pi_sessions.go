// Pi sessions commands — the GUI-facing forward; the read path lives in
// internal/service/read (B4) and the poll watcher stays here because it
// owns the Wails event bus.
package app

import (
	"droidgear-wails/internal/service/read"
)

type (
	PiSessionSummary = read.PiSessionSummary
	PiSessionDetail  = read.PiSessionDetail
)

// ListPiSessions lists Pi sessions, newest first.
func (a *App) ListPiSessions() ([]PiSessionSummary, error) {
	return read.ListPiSessions()
}

// GetPiSessionDetail returns one Pi session with its transcript.
func (a *App) GetPiSessionDetail(sessionPath string) (PiSessionDetail, error) {
	return read.GetPiSessionDetail(sessionPath)
}

// DeletePiSession removes a validated session file.
func (a *App) DeletePiSession(sessionPath string) error {
	return read.DeletePiSession(sessionPath)
}

// ---------------------------------------------------------------------------
// Watcher (stays in app: Wails event bus)
// ---------------------------------------------------------------------------

// StartPiSessionsWatcher starts the Pi sessions poll watcher; the frontend
// listens for `pi-sessions-changed` (Rust `start_pi_sessions_watcher`).
func (a *App) StartPiSessionsWatcher() error {
	dir, err := read.PiSessionsDir()
	if err != nil {
		return err
	}
	return startSessionsPollWatcher("pi-sessions-changed", dir)
}

// StopPiSessionsWatcher stops the Pi sessions watcher (Rust
// `stop_pi_sessions_watcher`).
func (a *App) StopPiSessionsWatcher() error {
	stopSessionsPollWatcher("pi-sessions-changed")
	return nil
}
