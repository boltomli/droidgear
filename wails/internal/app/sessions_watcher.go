// Shared sessions watcher plumbing (all four sessions viewers) — the GUI
// adapter half of the B4 split: the poll loop needs the Wails event bus, so
// it stays in internal/app while the read paths live in internal/service/read
// (tui-parity §3: watcher 留在 GUI 适配层).
package app

import (
	"fmt"
	"io/fs"
	"path/filepath"
	"sync"
	"time"

	"droidgear-wails/internal/core"
	"droidgear-wails/internal/service/read"
	"github.com/wailsapp/wails/v3/pkg/application"
)

// ---------------------------------------------------------------------------
// Generic (Droid) session commands — GUI-facing forwards
// ---------------------------------------------------------------------------

// ListSessionProjects lists the session project directories.
func (a *App) ListSessionProjects() ([]SessionProject, error) {
	return read.ListSessionProjects()
}

// ListSessions lists sessions for all projects, or only `project` when set.
func (a *App) ListSessions(project *string) ([]SessionSummary, error) {
	return read.ListSessions(project)
}

// GetSessionDetail returns one session with its transcript.
func (a *App) GetSessionDetail(sessionPath string) (SessionDetail, error) {
	return read.GetSessionDetail(sessionPath)
}

// DeleteSession removes a session's .jsonl and .settings.json files.
func (a *App) DeleteSession(sessionPath string) error {
	return read.DeleteSession(sessionPath)
}

type (
	SessionProject = read.SessionProject
	TokenUsage     = read.TokenUsage
	SessionSummary = read.SessionSummary
	ContentBlock   = read.ContentBlock
	SessionMessage = read.SessionMessage
	SessionDetail  = read.SessionDetail
)

// ---------------------------------------------------------------------------
// Shared change watcher plumbing (all four sessions viewers)
// ---------------------------------------------------------------------------

// sessionsFallbackInterval is the safety-net fingerprint poll behind the
// event-driven watcher (watchtree.go); fsnotify carries the normal path so
// idle cost is ~0 and change latency stays sub-second (guidelines §4.5).
const sessionsFallbackInterval = 10 * time.Second

// sessionsPollRun is one running watcher for a single event name.
type sessionsPollRun struct {
	dir  string
	stop chan struct{}
	done chan struct{}
}

// sessionsPollRegistry keeps at most one watcher per event name (Rust keeps
// one notify watcher per command state).
var sessionsPollRegistry = struct {
	mu       sync.Mutex
	watchers map[string]*sessionsPollRun
}{watchers: map[string]*sessionsPollRun{}}

// sessionToolEventSink lets tests observe events without a running Wails
// application (mirrors ptyEventSink).
var sessionToolEventSink func(event string)

// emitSessionToolEvent broadcasts a sessions change event when a Wails app
// instance exists; outside one (unit tests, early startup) it falls back to
// the test sink when set, otherwise it is a no-op.
func emitSessionToolEvent(event string) {
	if app := application.Get(); app != nil {
		app.Event.Emit(event)
		return
	}
	if sessionToolEventSink != nil {
		sessionToolEventSink(event)
	}
}

// sessionsPollSnapshot records every entry under root with its type, mtime
// and size. Unreadable subtrees contribute nothing, so their disappearance
// still registers as a change.
func sessionsPollSnapshot(root string) map[string]string {
	state := map[string]string{}
	_ = filepath.WalkDir(root, func(path string, d fs.DirEntry, err error) error {
		if err != nil || d == nil {
			return nil
		}
		rel, relErr := filepath.Rel(root, path)
		if relErr != nil || rel == "." {
			return nil
		}
		info, infoErr := d.Info()
		if infoErr != nil {
			state[rel] = "entry"
			return nil
		}
		kind := "f"
		if d.IsDir() {
			kind = "d"
		}
		state[rel] = fmt.Sprintf("%s:%d:%d", kind, info.ModTime().UnixMilli(), info.Size())
		return nil
	})
	return state
}

func sessionsPollEqual(a, b map[string]string) bool {
	if len(a) != len(b) {
		return false
	}
	for key, value := range a {
		if b[key] != value {
			return false
		}
	}
	return true
}

// startSessionsPollWatcher starts watching dir and emits event only when the
// tree actually changes. A missing directory is not an error — the Rust
// command returns without watching (mirrors start_*_sessions_watcher).
func startSessionsPollWatcher(event string, dir string) error {
	if !core.DirExists(dir) {
		return nil
	}
	sessionsPollRegistry.mu.Lock()
	defer sessionsPollRegistry.mu.Unlock()
	if current, ok := sessionsPollRegistry.watchers[event]; ok {
		if current.dir == dir {
			return nil // already watching this directory
		}
		close(current.stop)
		<-current.done
		delete(sessionsPollRegistry.watchers, event)
	}
	run := &sessionsPollRun{dir: dir, stop: make(chan struct{}), done: make(chan struct{})}
	sessionsPollRegistry.watchers[event] = run
	previous := sessionsPollSnapshot(dir)
	go func() {
		defer close(run.done)
		changed := func() bool {
			current := sessionsPollSnapshot(dir)
			if sessionsPollEqual(previous, current) {
				return false
			}
			previous = current
			return true
		}
		watchTreeUntil(dir, run.stop, changed, sessionsFallbackInterval, func() {
			emitSessionToolEvent(event)
		})
	}()
	return nil
}

// stopSessionsPollWatcher stops the watcher for event, if any (Rust
// stop_*_sessions_watcher is a no-op when nothing is watching).
func stopSessionsPollWatcher(event string) {
	sessionsPollRegistry.mu.Lock()
	defer sessionsPollRegistry.mu.Unlock()
	if current, ok := sessionsPollRegistry.watchers[event]; ok {
		close(current.stop)
		<-current.done
		delete(sessionsPollRegistry.watchers, event)
	}
}

// StartSessionsWatcher starts the generic sessions directory watcher; the
// frontend listens for the `sessions-changed` event (Rust
// `start_sessions_watcher`).
func (a *App) StartSessionsWatcher() error {
	dir, err := read.DroidSessionsDir()
	if err != nil {
		return err
	}
	return startSessionsPollWatcher("sessions-changed", dir)
}

// StopSessionsWatcher stops the generic sessions watcher (Rust
// `stop_sessions_watcher`).
func (a *App) StopSessionsWatcher() error {
	stopSessionsPollWatcher("sessions-changed")
	return nil
}
