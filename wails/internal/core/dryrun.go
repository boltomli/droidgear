package core

import (
	"os"
	"path/filepath"
	"strings"
	"sync"
	"sync/atomic"
)

// Change is one file a --dry-run command would have written or removed
// (design doc §3.3). `after` holds the full document that would have been
// written, `removed` marks a deletion instead.
type Change struct {
	Path     string `json:"path"`
	Kind     string `json:"kind"`
	After    string `json:"after,omitempty"`
	Removed  bool   `json:"removed,omitempty"`
	Redacted bool   `json:"redacted,omitempty"`
}

// ChangeSet is the preview `dg --dry-run` prints: the real code path runs,
// only the filesystem writes are recorded (see BeginDryRun).
type ChangeSet struct {
	mu      sync.Mutex
	Applied bool     `json:"applied"`
	Changes []Change `json:"changes"`
}

// dryRun holds the recorder for this process. `dg` runs one command per
// process and never concurrently; the GUI never installs one, so the hook is
// inert everywhere except a CLI preview.
var dryRun atomic.Pointer[ChangeSet]

// BeginDryRun redirects every write through AtomicWrite/AtomicWritePrivate,
// every directory creation through EnsureDir and every deletion through
// RemoveFile into a ChangeSet instead of touching the disk. The returned
// function ends the dry run and yields what was recorded.
//
// Callers must defer it: a leaked recorder would silently turn later writes
// into no-ops for the rest of the process.
func BeginDryRun() func() *ChangeSet {
	cs := &ChangeSet{}
	dryRun.Store(cs)
	return func() *ChangeSet {
		dryRun.Store(nil)
		return cs
	}
}

// InDryRun reports whether writes are currently being recorded.
func InDryRun() bool {
	return dryRun.Load() != nil
}

// record keeps the last change per path: a file written twice during one
// command previews as the document that would actually land on disk.
func (cs *ChangeSet) record(change Change) {
	cs.mu.Lock()
	defer cs.mu.Unlock()
	for i := range cs.Changes {
		if cs.Changes[i].Path == change.Path {
			cs.Changes[i] = change
			return
		}
	}
	cs.Changes = append(cs.Changes, change)
}

func recordIfDryRun(change Change) {
	if cs := dryRun.Load(); cs != nil {
		cs.record(change)
	}
}

// KindOf classifies a path the way the ChangeSet contract does
// (json | toml | yaml | text), so a preview renderer knows how to show it.
func KindOf(path string) string {
	switch strings.ToLower(filepath.Ext(path)) {
	case ".json":
		return "json"
	case ".toml":
		return "toml"
	case ".yaml", ".yml":
		return "yaml"
	default:
		return "text"
	}
}

// RemoveFile deletes a file, or records the deletion during a dry run.
// Service code must use it instead of os.Remove — a bare os.Remove would
// delete for real while the preview claims nothing happened.
func RemoveFile(path string) error {
	if InDryRun() {
		recordIfDryRun(Change{Path: path, Kind: KindOf(path), Removed: true})
		return nil
	}
	return os.Remove(path)
}
