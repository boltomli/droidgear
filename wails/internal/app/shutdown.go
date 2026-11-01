// Exit cleanup: nothing long-lived may outlive the process (guidelines
// §3.8 / P1-1). OnShutdown stops the filesystem watchers and kills every
// PTY session (including its process tree on Windows), then flushes the
// final window geometry.
package app

// CleanupOnShutdown releases all long-lived resources. Every stop is
// idempotent and safe when the corresponding watcher was never started.
func CleanupOnShutdown() {
	a := &App{}

	// Filesystem watchers (sessions viewers, specs).
	_ = a.StopSpecsWatcher()
	_ = a.StopSessionsWatcher()
	_ = a.StopCodexSessionsWatcher()
	_ = a.StopDshSessionsWatcher()
	_ = a.StopPiSessionsWatcher()
	_ = a.StopOmpSessionsWatcher()

	// Terminal sessions: close the PTY master and kill the child tree so
	// no shell survives the GUI (mirrors `taskkill /T` in PtyKill).
	ptys.killAll()

	// Persist the last known window geometry (moves may still be pending
	// in the debounce timer).
	flushAttachedWindowState()
}
