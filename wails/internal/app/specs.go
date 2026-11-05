// Droid Specs commands — the GUI-facing forward — plus the `specs-changed`
// poll watcher.
//
// The CRUD lives in internal/service/specs; the watcher stays here because
// it owns the Wails event bus (tui-parity §3: watcher 留在 GUI 适配层). The
// poll loop itself is the accepted equivalent of the Rust notify watcher
// (wails/PORTING.md).
package app

import (
	"fmt"
	"os"
	"path/filepath"
	"sync"
	"time"

	"droidgear-wails/internal/core"
	"droidgear-wails/internal/service/specs"
	"github.com/wailsapp/wails/v3/pkg/application"
)

// ---------------------------------------------------------------------------
// Commands (src-tauri/src/commands/specs.rs)
// ---------------------------------------------------------------------------

// ListSpecs lists all spec files from ~/.factory/specs.
func (a *App) ListSpecs() ([]specs.SpecFile, error) {
	return specs.List()
}

// RenameSpec renames a spec file inside the specs directory.
func (a *App) RenameSpec(oldPath string, newName string) (specs.SpecFile, error) {
	return specs.Rename(oldPath, newName)
}

// DeleteSpec deletes a spec file inside the specs directory.
func (a *App) DeleteSpec(path string) error {
	return specs.Delete(path)
}

// UpdateSpec overwrites a spec file's content.
func (a *App) UpdateSpec(path string, content string) (specs.SpecFile, error) {
	return specs.Update(path, content)
}

// ---------------------------------------------------------------------------
// Watcher (notify → 500ms poll, event name/payload mirrored)
// ---------------------------------------------------------------------------

// ---------------------------------------------------------------------------
// Watcher (notify → 500ms poll, event name/payload mirrored)
// ---------------------------------------------------------------------------

type specsFileStamp struct {
	mod  int64
	size int64
}

type specsWatcherState struct {
	mu      sync.Mutex
	running bool
	stop    chan struct{}
}

var specsWatcher specsWatcherState

// specsEventSink lets tests observe the watcher without a running Wails app.
var specsEventSink func(name string, payload map[string]any)

// emitSpecsChanged sends the `specs-changed` event SpecsPage listens to (the
// Tauri build emits `()`; the payload is ignored by the frontend).
func emitSpecsChanged() {
	payload := map[string]any{}
	if app := application.Get(); app != nil {
		app.Event.Emit("specs-changed", payload)
		return
	}
	if specsEventSink != nil {
		specsEventSink("specs-changed", payload)
	}
}

func specsSnapshot(dir string) map[string]specsFileStamp {
	entries, err := os.ReadDir(dir)
	if err != nil {
		return nil
	}
	snapshot := map[string]specsFileStamp{}
	for _, entry := range entries {
		info, err := os.Stat(filepath.Join(dir, entry.Name()))
		if err != nil {
			continue
		}
		if info.IsDir() {
			// A non-recursive watch sees a subdirectory only as an entry:
			// creation/removal matters, its contents' mtime does not.
			snapshot[entry.Name()] = specsFileStamp{}
			continue
		}
		snapshot[entry.Name()] = specsFileStamp{mod: info.ModTime().UnixNano(), size: info.Size()}
	}
	return snapshot
}

func specsSnapshotsEqual(left map[string]specsFileStamp, right map[string]specsFileStamp) bool {
	if len(left) != len(right) {
		return false
	}
	for name, stamp := range left {
		if other, ok := right[name]; !ok || other != stamp {
			return false
		}
	}
	return true
}

// StartSpecsWatcher creates the specs directory when missing and starts the
// change poll loop (replacing any watcher already running, like the Rust
// command does).
func (a *App) StartSpecsWatcher() error {
	dir, err := specs.Dir()
	if err != nil {
		return err
	}
	if err := core.EnsureDir(dir); err != nil {
		return fmt.Errorf("Failed to create specs directory: %w", err)
	}
	initial := specsSnapshot(dir)

	specsWatcher.mu.Lock()
	if specsWatcher.running {
		close(specsWatcher.stop)
	}
	stop := make(chan struct{})
	specsWatcher.running = true
	specsWatcher.stop = stop
	specsWatcher.mu.Unlock()

	go func() {
		ticker := time.NewTicker(500 * time.Millisecond)
		defer ticker.Stop()
		baseline := initial
		for {
			select {
			case <-stop:
				return
			case <-ticker.C:
			}
			snapshot := specsSnapshot(dir)
			if snapshot == nil {
				// The directory vanished (or became unreadable): notify once,
				// like notify's Remove event, and wait for it to come back.
				if baseline != nil {
					baseline = nil
					emitSpecsChanged()
				}
				continue
			}
			if baseline == nil {
				baseline = snapshot
				emitSpecsChanged()
				continue
			}
			if !specsSnapshotsEqual(baseline, snapshot) {
				baseline = snapshot
				emitSpecsChanged()
			}
		}
	}()
	return nil
}

// StopSpecsWatcher stops the change poll loop.
func (a *App) StopSpecsWatcher() error {
	if _, err := specs.Dir(); err != nil {
		return err
	}
	specsWatcher.mu.Lock()
	defer specsWatcher.mu.Unlock()
	if specsWatcher.running {
		close(specsWatcher.stop)
		specsWatcher.running = false
		specsWatcher.stop = nil
	}
	return nil
}
