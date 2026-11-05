// OMP sessions commands — the GUI-facing forward; the read path lives in
// internal/service/read (B4) and the poll watcher stays here because it
// owns the Wails event bus.
package app

import (
	"droidgear-wails/internal/service/read"
)

type (
	OmpSessionSummary = read.OmpSessionSummary
	OmpSessionDetail  = read.OmpSessionDetail
)

// ListOmpSessions lists OMP sessions, newest first.
func (a *App) ListOmpSessions() ([]OmpSessionSummary, error) {
	return read.ListOmpSessions()
}

// GetOmpSessionDetail returns one OMP session with its transcript.
func (a *App) GetOmpSessionDetail(sessionPath string) (OmpSessionDetail, error) {
	return read.GetOmpSessionDetail(sessionPath)
}

// DeleteOmpSession removes a validated OMP session file.
func (a *App) DeleteOmpSession(sessionPath string) error {
	return read.DeleteOmpSession(sessionPath)
}

// ---------------------------------------------------------------------------
// Watcher (stays in app: Wails event bus)
// ---------------------------------------------------------------------------

// StartOmpSessionsWatcher starts the OMP sessions poll watcher; the
// frontend listens for `omp-sessions-changed` (Rust
// `start_omp_sessions_watcher`).
func (a *App) StartOmpSessionsWatcher() error {
	dir, err := read.OmpSessionsDir()
	if err != nil {
		return err
	}
	return startSessionsPollWatcher("omp-sessions-changed", dir)
}

// StopOmpSessionsWatcher stops the OMP sessions watcher (Rust
// `stop_omp_sessions_watcher`).
func (a *App) StopOmpSessionsWatcher() error {
	stopSessionsPollWatcher("omp-sessions-changed")
	return nil
}
