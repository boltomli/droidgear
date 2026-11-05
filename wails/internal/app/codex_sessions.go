// Codex sessions commands — the GUI-facing forward; the read path lives in
// internal/service/read (B4) and the poll watcher stays here because it
// owns the Wails event bus.
package app

import (
	"fmt"
	"os"
	"path/filepath"
	"sync"
	"time"

	"droidgear-wails/internal/core"
	"droidgear-wails/internal/service/read"
	"github.com/wailsapp/wails/v3/pkg/application"
)

const (
	codexSessionEventName      = "codex-sessions-changed"
	codexSessionsFallbackEvery = 10 * time.Second
)

type (
	CodexSessionSummary  = read.CodexSessionSummary
	CodexSessionProvider = read.CodexSessionProvider
	CodexSessionDetail   = read.CodexSessionDetail
)

// ListCodexSessions lists Codex sessions, newest first.
func (a *App) ListCodexSessions() ([]CodexSessionSummary, error) {
	return read.ListCodexSessions()
}

// ListCodexSessionProviders lists the profiles a Codex session can switch to.
func (a *App) ListCodexSessionProviders() ([]CodexSessionProvider, error) {
	return read.ListCodexSessionProviders()
}

// GetCodexSessionDetail returns one Codex session with its transcript.
func (a *App) GetCodexSessionDetail(sessionPath string) (CodexSessionDetail, error) {
	return read.GetCodexSessionDetail(sessionPath)
}

// SetCodexSessionProvider points a session at one of the provider profiles.
func (a *App) SetCodexSessionProvider(sessionPath string, providerID string) error {
	return read.SetCodexSessionProvider(sessionPath, providerID)
}

// DeleteCodexSession removes a validated Codex session file.
func (a *App) DeleteCodexSession(sessionPath string) error {
	return read.DeleteCodexSession(sessionPath)
}

// ---------------------------------------------------------------------------
// Watcher (poll-based mirror of the Rust notify watcher)
// ---------------------------------------------------------------------------

// codexSessionsWatcherState holds the single package-level poll loop.
type codexSessionsWatcherState struct {
	mu      sync.Mutex
	running bool
	stop    chan struct{}
}

var codexSessionsWatcher codexSessionsWatcherState

// emitCodexEvent broadcasts an event when a Wails app instance exists;
// outside one (unit tests, early startup) it falls back to the test sink
// when set, otherwise it is a no-op. Mirrors emitPtyEvent.
func emitCodexEvent(name string) {
	if app := application.Get(); app != nil {
		app.Event.Emit(name)
		return
	}
	if codexEventSink != nil {
		codexEventSink(name)
	}
}

// codexEventSink lets tests observe the event stream without a running app.
var codexEventSink func(name string)

// codexSessionsSignature fingerprints the sessions tree (file count, total
// size, newest mtime) so the poll loop only emits on actual change.
func codexSessionsSignature(dir string) string {
	count := 0
	var totalSize, maxMtime int64
	_ = filepath.WalkDir(dir, func(path string, entry os.DirEntry, err error) error {
		if err != nil || entry.IsDir() {
			return nil
		}
		info, err := entry.Info()
		if err != nil {
			return nil
		}
		count++
		totalSize += info.Size()
		if mtime := info.ModTime().UnixNano(); mtime > maxMtime {
			maxMtime = mtime
		}
		return nil
	})
	return fmt.Sprintf("%d:%d:%d", count, totalSize, maxMtime)
}

// codexWatchSessionsLoop emits `codex-sessions-changed` when the sessions
// tree fingerprint changes. Event-driven via fsnotify with a low-frequency
// fallback poll (watchtree.go); the baseline is taken at start, matching
// notify (no event for the initial state).
func codexWatchSessionsLoop(dir string, stop <-chan struct{}) {
	signature := codexSessionsSignature(dir)
	changed := func() bool {
		next := codexSessionsSignature(dir)
		if next == signature {
			return false
		}
		signature = next
		return true
	}
	watchTreeUntil(dir, stop, changed, codexSessionsFallbackEvery, func() {
		emitCodexEvent(codexSessionEventName)
	})
}

// StartCodexSessionsWatcher starts watching the codex sessions directory for
// changes (no-op when the directory does not exist yet, like Rust).
func (a *App) StartCodexSessionsWatcher() error {
	sessionsDir, err := read.CodexSessionsDir()
	if err != nil {
		return err
	}
	if !core.DirExists(sessionsDir) {
		return nil
	}
	codexSessionsWatcher.mu.Lock()
	defer codexSessionsWatcher.mu.Unlock()
	if codexSessionsWatcher.running {
		// Rust replaces the existing watcher on start.
		close(codexSessionsWatcher.stop)
	}
	stop := make(chan struct{})
	codexSessionsWatcher.stop = stop
	codexSessionsWatcher.running = true
	go codexWatchSessionsLoop(sessionsDir, stop)
	return nil
}

// StopCodexSessionsWatcher stops watching the codex sessions directory.
func (a *App) StopCodexSessionsWatcher() error {
	codexSessionsWatcher.mu.Lock()
	defer codexSessionsWatcher.mu.Unlock()
	if codexSessionsWatcher.running {
		close(codexSessionsWatcher.stop)
		codexSessionsWatcher.running = false
		codexSessionsWatcher.stop = nil
	}
	return nil
}
