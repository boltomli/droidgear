package core

import (
	"crypto/sha256"
	"encoding/hex"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"sync/atomic"
)

// Change is one file a --dry-run command would have written or removed
// (design doc §3.3). `after` holds the full document that would have been
// written, `removed` marks a deletion instead, `beforeHash` is the sha256 of
// the file as it was when the write was recorded — the TUI preview flow
// compares it between preview and apply to detect external edits (§5.2).
type Change struct {
	Path       string `json:"path"`
	Kind       string `json:"kind"`
	BeforeHash string `json:"beforeHash,omitempty"`
	After      string `json:"after,omitempty"`
	Removed    bool   `json:"removed,omitempty"`
	Redacted   bool   `json:"redacted,omitempty"`
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
		if change.BeforeHash == "" {
			change.BeforeHash = HashFile(change.Path)
		}
		cs.record(change)
	}
}

// HashFile returns the `sha256:` digest of path's current content, or ""
// when the file does not exist or is not a regular file (a fresh create has
// no "before" state).
func HashFile(path string) string {
	info, err := os.Stat(path)
	if err != nil || !info.Mode().IsRegular() {
		return ""
	}
	data, err := os.ReadFile(path) // #nosec G304 -- hashing whatever the write path would replace
	if err != nil {
		return ""
	}
	sum := sha256.Sum256(data)
	return "sha256:" + hex.EncodeToString(sum[:])
}

// RecordWrite is the dry-run hook for write paths that cannot delegate to
// AtomicWrite byte for byte (custom error strings, rename semantics): it
// records the would-be write — beforeHash included — and reports whether a
// recorder is installed, i.e. whether the caller must skip the real write.
func RecordWrite(path string, data []byte) bool {
	if !InDryRun() {
		return false
	}
	recordIfDryRun(Change{Path: path, Kind: KindOf(path), After: string(data)})
	return true
}

// RecordRemove mirrors RecordWrite for deletions (files and directories).
func RecordRemove(path string) bool {
	if !InDryRun() {
		return false
	}
	recordIfDryRun(Change{Path: path, Kind: KindOf(path), Removed: true})
	return true
}

// RecordRename previews an os.Rename as the pair the contract expects: the
// old path removed, the new path written with newData. It reports whether
// the real rename must be skipped.
func RecordRename(oldPath string, newPath string, newData []byte) bool {
	if !InDryRun() {
		return false
	}
	recordIfDryRun(Change{Path: oldPath, Kind: KindOf(oldPath), Removed: true})
	recordIfDryRun(Change{Path: newPath, Kind: KindOf(newPath), After: string(newData)})
	return true
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
