// Event-driven directory watching with a slow fingerprint fallback — the
// fix for the former 500ms full-tree stat poll behind every sessions
// viewer (guidelines §4.5 / P1-3). fsnotify drives change detection
// (sub-1s latency, ~0 idle CPU); a low-frequency fingerprint pass catches
// anything the OS did not report (missed renames, watcher overflow,
// deleted-and-recreated directories).
package app

import (
	"io/fs"
	"os"
	"path/filepath"
	"time"

	"droidgear-wails/internal/core"

	"github.com/fsnotify/fsnotify"
)

const (
	// watchTreeDebounce coalesces event bursts (a single session write
	// fires Create + several Writes) into one fingerprint pass.
	watchTreeDebounce = 150 * time.Millisecond
	// watchTreePollFallback is the last-resort poll period used only when
	// fsnotify cannot be initialised at all.
	watchTreePollFallback = 500 * time.Millisecond
)

// watchTreeUntil watches dir until stop is closed and calls emit whenever
// the caller's `changed` fingerprint reports a difference. `changed` owns
// the baseline (it is only ever called from this goroutine).
//
// fsnotify watches are not recursive, so subdirectories discovered through
// Create events are added on the fly, and the fallback pass re-syncs the
// watch set so recreated directories are picked up again.
func watchTreeUntil(
	dir string,
	stop <-chan struct{},
	changed func() bool,
	fallbackInterval time.Duration,
	emit func(),
) {
	notify, err := fsnotify.NewWatcher()
	if err != nil {
		// Last resort: pure fingerprint polling.
		ticker := time.NewTicker(watchTreePollFallback)
		defer ticker.Stop()
		for {
			select {
			case <-stop:
				return
			case <-ticker.C:
				if changed() {
					emit()
				}
			}
		}
	}
	defer notify.Close()

	watched := map[string]bool{}
	watchTree := func(root string) {
		_ = filepath.WalkDir(root, func(path string, d fs.DirEntry, err error) error {
			if err != nil || d == nil || !d.IsDir() {
				return nil
			}
			if !watched[path] && notify.Add(path) == nil {
				watched[path] = true
			}
			return nil
		})
	}
	watchTree(dir)

	debounce := time.NewTimer(time.Hour)
	if !debounce.Stop() {
		<-debounce.C
	}
	debounceArmed := false
	// arm() schedules a flush watchTreeDebounce from the first event of a
	// burst; later events within the window do not extend the wait, so
	// latency stays bounded.
	arm := func() {
		if !debounceArmed {
			debounce.Reset(watchTreeDebounce)
			debounceArmed = true
		}
	}
	flush := func() {
		if debounceArmed {
			debounce.Stop()
			debounceArmed = false
		}
		if changed() {
			emit()
		}
	}

	fallback := time.NewTicker(fallbackInterval)
	defer fallback.Stop()

	for {
		var tick <-chan time.Time
		if debounceArmed {
			tick = debounce.C
		}
		select {
		case <-stop:
			return

		case event, ok := <-notify.Events:
			if !ok {
				return
			}
			if event.Op&(fsnotify.Create|fsnotify.Write|fsnotify.Remove|fsnotify.Rename) == 0 {
				continue // Chmod-only noise
			}
			switch {
			case event.Op&fsnotify.Create != 0:
				if info, statErr := os.Stat(event.Name); statErr == nil && info.IsDir() {
					watchTree(event.Name) // new project/session subdir
				}
			case event.Op&(fsnotify.Remove|fsnotify.Rename) != 0:
				delete(watched, event.Name)
			}
			arm()

		case <-notify.Errors:
			// Transient watcher errors: the fallback pass recovers state.

		case <-tick:
			flush()

		case <-fallback.C:
			// Self-heal the watch set (directories that vanished and came
			// back without a usable Create event), then re-check.
			for path := range watched {
				if !core.DirExists(path) {
					delete(watched, path)
				}
			}
			watchTree(dir)
			flush()
		}
	}
}
