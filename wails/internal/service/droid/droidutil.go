package droid

import (
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"sync/atomic"
	"time"

	"droidgear-wails/internal/core"
	prefs "droidgear-wails/internal/service/preferences"
)

// DroidRunPreferences is the droid-run slice of preferences.json (single
// definition: internal/service/preferences).
type DroidRunPreferences = prefs.DroidRunPreferences

// ---------------------------------------------------------------------------
// Shared Droid-tool helpers (moved from internal/app/specs.go, which keeps
// its own copy until Specs migrates in tui-parity P3)
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

// droidAtomicWrite mirrors droidgear-core::storage::atomic_write: a unique
// sibling temp file renamed over the target, with byte-identical errors.
func droidAtomicWrite(path string, data []byte) error {
	parent := filepath.Dir(path)
	if !core.PathExists(parent) {
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
		if core.PathExists(candidate) {
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
