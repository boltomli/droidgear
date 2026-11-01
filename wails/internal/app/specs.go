// Specs surface: ~/.factory/specs CRUD plus the `specs-changed` watcher.
//
// Mirrors droidgear-core::specs and src-tauri/src/commands/specs.rs; the
// notify watcher is replaced by a poll loop per wails/PORTING.md.
//
// This file also hosts the small path/JSON helpers shared by the other Droid
// tool files in this package (mcp.go, droid_settings.go, droid_trusted.go,
// factory_auth_profiles.go) — they are droid-prefixed so they read as one
// spelling of the same concept across those files.
package app

import (
	"bytes"
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"sync"
	"sync/atomic"
	"time"
	"unicode/utf8"

	"droidgear-wails/internal/core"
	"github.com/wailsapp/wails/v3/pkg/application"
)

// ---------------------------------------------------------------------------
// Shared Droid-tool helpers
// ---------------------------------------------------------------------------

// droidTempCounter makes sibling temp names unique within one process
// (mirrors the atomic counters in droidgear-core's write helpers).
var droidTempCounter atomic.Uint64

// droidHomeDir resolves the user home directory with the Rust error string
// (dirs::home_dir().ok_or("Failed to get home directory")).
func droidHomeDir() (string, error) {
	home, err := core.HomeDir()
	if err != nil {
		return "", errString("Failed to get home directory")
	}
	return home, nil
}

// droidFactoryHomeForHome resolves the Factory home for home, honoring the
// configPaths.factory override in <home>/.droidgear/settings.json (mirrors
// paths::get_factory_home_for_home). A configPaths object whose known keys
// are not all strings fails to deserialize in Rust and yields the default.
func droidFactoryHomeForHome(home string) string {
	fallback := filepath.Join(home, ".factory")
	data, err := os.ReadFile(filepath.Join(home, ".droidgear", "settings.json"))
	if err != nil || strings.TrimSpace(string(data)) == "" {
		return fallback
	}
	settings, err := droidDecodeJSONValue(data)
	if err != nil {
		return fallback
	}
	root, ok := settings.(map[string]any)
	if !ok {
		return fallback
	}
	paths, ok := root["configPaths"].(map[string]any)
	if !ok {
		return fallback
	}
	for _, key := range []string{"factory", "opencode", "opencodeAuth", "codex", "claude", "openclaw", "hermes", "pi", "omp"} {
		if value, present := paths[key]; present {
			if _, isString := value.(string); !isString {
				return fallback
			}
		}
	}
	if override, ok := paths["factory"].(string); ok && override != "" {
		return override
	}
	return fallback
}

// droidWithExtension mirrors Rust Path::with_extension: the suffix after the
// final dot is replaced (dotfiles keep their whole name and gain the ext).
func droidWithExtension(path string, ext string) string {
	dir, base := filepath.Split(path)
	if i := strings.LastIndex(base, "."); i > 0 {
		base = base[:i]
	}
	return dir + base + "." + ext
}

// droidExtension mirrors Rust Path::extension() for a file *name*.
func droidExtension(name string) string {
	i := strings.LastIndex(name, ".")
	if i <= 0 {
		return ""
	}
	return name[i+1:]
}

// droidFileStem mirrors Rust Path::file_stem() for a file *name*.
func droidFileStem(name string) string {
	if i := strings.LastIndex(name, "."); i > 0 {
		return name[:i]
	}
	return name
}

// droidFileName mirrors Rust Path::file_name() (None → empty string).
func droidFileName(path string) string {
	if path == "" {
		return ""
	}
	base := filepath.Base(filepath.Clean(path))
	if base == "." || base == ".." || strings.HasSuffix(base, string(filepath.Separator)) {
		return ""
	}
	return base
}

// droidPathsEqual compares paths component-wise like Rust's Path PartialEq
// (separator-normalized, case-sensitive).
func droidPathsEqual(a string, b string) bool {
	return filepath.Clean(a) == filepath.Clean(b)
}

// droidPathHasPrefix mirrors Rust Path::starts_with (component-wise).
func droidPathHasPrefix(path string, prefix string) bool {
	p := filepath.Clean(path)
	pre := filepath.Clean(prefix)
	if p == pre {
		return true
	}
	if !strings.HasPrefix(p, pre) {
		return false
	}
	if strings.HasSuffix(pre, string(filepath.Separator)) {
		return true
	}
	rest := p[len(pre):]
	return len(rest) > 0 && rest[0] == filepath.Separator
}

// droidPathExists reports whether path exists (any file type).
func droidPathExists(path string) bool {
	_, err := os.Stat(path)
	return err == nil
}

// droidIsSymlink reports whether path is a symlink (mirrors Path::is_symlink).
func droidIsSymlink(path string) bool {
	info, err := os.Lstat(path)
	return err == nil && info.Mode()&os.ModeSymlink != 0
}

// droidResolvePath mirrors std::fs::canonicalize (absolute, symlinks
// resolved). On Windows the Rust side keeps the verbatim \\?\ prefix; the Go
// port stores the ordinary absolute path, which stays self-consistent across
// the Wails surface (see the porting report).
func droidResolvePath(path string) (string, error) {
	abs, err := filepath.Abs(path)
	if err != nil {
		return "", err
	}
	resolved, err := filepath.EvalSymlinks(abs)
	if err != nil {
		return "", err
	}
	return resolved, nil
}

// droidDecodeJSONValue parses JSON preserving number literals, mirroring
// serde_json::Value (integers stay integers; unknown structure is kept).
func droidDecodeJSONValue(data []byte) (any, error) {
	dec := json.NewDecoder(bytes.NewReader(data))
	dec.UseNumber()
	var value any
	if err := dec.Decode(&value); err != nil {
		return nil, err
	}
	if dec.More() {
		return nil, fmt.Errorf("trailing garbage after JSON value")
	}
	return value, nil
}

// droidMarshalPretty renders JSON the way serde_json::to_string_pretty does:
// two-space indent, no HTML escaping, no trailing newline, map keys sorted.
func droidMarshalPretty(value any) ([]byte, error) {
	var buf bytes.Buffer
	enc := json.NewEncoder(&buf)
	enc.SetEscapeHTML(false)
	enc.SetIndent("", "  ")
	if err := enc.Encode(value); err != nil {
		return nil, err
	}
	return bytes.TrimRight(buf.Bytes(), "\n"), nil
}

// droidAtomicWrite mirrors droidgear-core::storage::atomic_write: a unique
// sibling temp file renamed over the target, with byte-identical errors.
func droidAtomicWrite(path string, data []byte) error {
	parent := filepath.Dir(path)
	if !droidPathExists(parent) {
		if err := os.MkdirAll(parent, 0o755); err != nil { // #nosec G301 -- config directory
			return fmt.Errorf("Failed to create directory: %w", err)
		}
	}
	name := filepath.Base(path)
	nanos := time.Now().UnixNano()
	pid := os.Getpid()
	for counter := uint64(0); ; counter++ {
		suffix := fmt.Sprintf("%d-%d", pid, nanos)
		if counter > 0 {
			suffix = fmt.Sprintf("%s-%d", suffix, counter)
		}
		candidate := filepath.Join(parent, "."+name+"."+suffix+".tmp")
		if droidPathExists(candidate) {
			continue
		}
		if err := writeDroidTempFile(candidate, data, path); err != nil {
			return err
		}
		return nil
	}
}

func writeDroidTempFile(tempPath string, data []byte, target string) error {
	if err := os.WriteFile(tempPath, data, 0o644); err != nil { // #nosec G306 -- config files
		return fmt.Errorf("Failed to write file: %w", err)
	}
	if err := os.Rename(tempPath, target); err != nil {
		_ = os.Remove(tempPath)
		return fmt.Errorf("Failed to write file: %w", err)
	}
	return nil
}

// ---------------------------------------------------------------------------
// Types
// ---------------------------------------------------------------------------

// SpecFile mirrors specs::SpecFile (bindings.ts).
type SpecFile struct {
	Name       string `json:"name"`
	Path       string `json:"path"`
	Content    string `json:"content"`
	ModifiedAt int64  `json:"modifiedAt"`
}

// ---------------------------------------------------------------------------
// Path resolution
// ---------------------------------------------------------------------------

func specsDirForHome(home string) string {
	return filepath.Join(droidFactoryHomeForHome(home), "specs")
}

// ---------------------------------------------------------------------------
// CRUD (mirrors droidgear-core::specs)
// ---------------------------------------------------------------------------

func listSpecsForHome(home string) ([]SpecFile, error) {
	dir := specsDirForHome(home)
	entries, err := os.ReadDir(dir)
	if err != nil {
		if os.IsNotExist(err) {
			return []SpecFile{}, nil
		}
		return nil, fmt.Errorf("Failed to read specs directory: %w", err)
	}

	specs := []SpecFile{}
	for _, entry := range entries {
		name := entry.Name()
		if droidExtension(name) != "md" {
			continue
		}
		path := filepath.Join(dir, name)
		info, err := os.Stat(path) // follows symlinks, like fs::metadata
		if err != nil || !info.Mode().IsRegular() {
			continue
		}
		data, err := os.ReadFile(path) // #nosec G304 -- specs directory contents
		if err != nil || !utf8.Valid(data) {
			continue // Rust read_to_string skips unreadable/non-UTF-8 files
		}
		specs = append(specs, SpecFile{
			Name:       name,
			Path:       path,
			Content:    string(data),
			ModifiedAt: info.ModTime().UnixMilli(),
		})
	}

	sort.SliceStable(specs, func(i, j int) bool {
		return specs[i].ModifiedAt > specs[j].ModifiedAt
	})
	return specs, nil
}

func readSpecFile(path string) (SpecFile, error) {
	info, err := os.Stat(path)
	if err != nil {
		if os.IsNotExist(err) {
			return SpecFile{}, errString("Spec file not found")
		}
		return SpecFile{}, fmt.Errorf("Failed to read file metadata: %w", err)
	}
	data, err := os.ReadFile(path) // #nosec G304 -- user-picked spec path
	if err != nil {
		return SpecFile{}, fmt.Errorf("Failed to read file content: %w", err)
	}
	if !utf8.Valid(data) {
		return SpecFile{}, fmt.Errorf("Failed to read file content: %w", errString("stream did not contain valid UTF-8"))
	}
	return SpecFile{
		Name:       droidFileName(path),
		Path:       path,
		Content:    string(data),
		ModifiedAt: info.ModTime().UnixMilli(),
	}, nil
}

func renameSpecForHome(home string, oldPath string, newName string) (SpecFile, error) {
	specsDir := specsDirForHome(home)
	cleanOld := filepath.Clean(oldPath)
	if !droidPathHasPrefix(cleanOld, specsDir) {
		return SpecFile{}, errString("Invalid file path")
	}
	if !droidPathExists(cleanOld) {
		return SpecFile{}, errString("Spec file not found")
	}

	newName = strings.TrimSpace(newName)
	if newName == "" {
		return SpecFile{}, errString("File name cannot be empty")
	}
	if !strings.HasSuffix(newName, ".md") {
		newName += ".md"
	}
	if strings.ContainsAny(newName, `/\`) {
		return SpecFile{}, errString("Invalid file name")
	}

	newPath := filepath.Join(specsDir, newName)
	if droidPathExists(newPath) && !droidPathsEqual(newPath, cleanOld) {
		return SpecFile{}, errString("A file with this name already exists")
	}
	if err := os.Rename(cleanOld, newPath); err != nil {
		return SpecFile{}, fmt.Errorf("Failed to rename file: %w", err)
	}
	return readSpecFile(newPath)
}

func deleteSpecForHome(home string, path string) error {
	specsDir := specsDirForHome(home)
	cleanPath := filepath.Clean(path)
	if !droidPathHasPrefix(cleanPath, specsDir) {
		return errString("Invalid file path")
	}
	if !droidPathExists(cleanPath) {
		return errString("Spec file not found")
	}
	if err := os.Remove(cleanPath); err != nil {
		return fmt.Errorf("Failed to delete file: %w", err)
	}
	return nil
}

func updateSpecForHome(home string, path string, content string) (SpecFile, error) {
	specsDir := specsDirForHome(home)
	cleanPath := filepath.Clean(path)
	if !droidPathHasPrefix(cleanPath, specsDir) {
		return SpecFile{}, errString("Invalid file path")
	}
	if !droidPathExists(cleanPath) {
		return SpecFile{}, errString("Spec file not found")
	}
	if err := os.WriteFile(cleanPath, []byte(content), 0o644); err != nil { // #nosec G306 -- spec files
		return SpecFile{}, fmt.Errorf("Failed to write file: %w", err)
	}
	return readSpecFile(path)
}

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

// ---------------------------------------------------------------------------
// Commands (src-tauri/src/commands/specs.rs)
// ---------------------------------------------------------------------------

// ListSpecs lists all spec files from ~/.factory/specs.
func (a *App) ListSpecs() ([]SpecFile, error) {
	home, err := droidHomeDir()
	if err != nil {
		return []SpecFile{}, err
	}
	return listSpecsForHome(home)
}

// RenameSpec renames a spec file inside the specs directory.
func (a *App) RenameSpec(oldPath string, newName string) (SpecFile, error) {
	home, err := droidHomeDir()
	if err != nil {
		return SpecFile{}, err
	}
	return renameSpecForHome(home, oldPath, newName)
}

// DeleteSpec deletes a spec file inside the specs directory.
func (a *App) DeleteSpec(path string) error {
	home, err := droidHomeDir()
	if err != nil {
		return err
	}
	return deleteSpecForHome(home, path)
}

// UpdateSpec overwrites a spec file's content.
func (a *App) UpdateSpec(path string, content string) (SpecFile, error) {
	home, err := droidHomeDir()
	if err != nil {
		return SpecFile{}, err
	}
	return updateSpecForHome(home, path, content)
}

// StartSpecsWatcher creates the specs directory when missing and starts the
// change poll loop (replacing any watcher already running, like the Rust
// command does).
func (a *App) StartSpecsWatcher() error {
	home, err := droidHomeDir()
	if err != nil {
		return err
	}
	dir := specsDirForHome(home)
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
	if _, err := droidHomeDir(); err != nil {
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
