// Specs surface: ~/.factory/specs CRUD (mirrors droidgear-core::specs and
// src-tauri/src/commands/specs.rs). The `specs-changed` poll watcher stays
// in internal/app (tui-parity §3: watcher 留在 GUI 适配层) — it needs the
// Wails event bus; this package only reads and writes files.
package specs

import (
	"fmt"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"unicode/utf8"

	"droidgear-wails/internal/core"
)

// ---------------------------------------------------------------------------
// Path resolution + the small path helpers specs mirrors from the Rust core
// ---------------------------------------------------------------------------

// homeDir resolves the user home directory with the Rust error string
// (dirs::home_dir().ok_or("Failed to get home directory")).
func homeDir() (string, error) {
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

func specsDirForHome(home string) string {
	return filepath.Join(core.FactoryHomeForHome(home), "specs")
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
	if !core.PathExists(cleanOld) {
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
	if core.PathExists(newPath) && !droidPathsEqual(newPath, cleanOld) {
		return SpecFile{}, errString("A file with this name already exists")
	}
	if core.InDryRun() {
		data, err := os.ReadFile(cleanOld) // #nosec G304 -- specs directory contents
		if err != nil {
			return SpecFile{}, err
		}
		core.RecordRename(cleanOld, newPath, data)
		return readSpecFile(cleanOld) // preview only: the rename did not happen
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
	if !core.PathExists(cleanPath) {
		return errString("Spec file not found")
	}
	if err := core.RemoveFile(cleanPath); err != nil {
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
	if !core.PathExists(cleanPath) {
		return SpecFile{}, errString("Spec file not found")
	}
	if core.RecordWrite(cleanPath, []byte(content)) {
		return readSpecFile(cleanPath) // preview only: nothing was written
	}
	if err := os.WriteFile(cleanPath, []byte(content), 0o644); err != nil { // #nosec G306 -- spec files
		return SpecFile{}, fmt.Errorf("Failed to write file: %w", err)
	}
	return readSpecFile(path)
}

// ---------------------------------------------------------------------------
// Commands (src-tauri/src/commands/specs.rs)
// ---------------------------------------------------------------------------

// ListSpecs lists all spec files from ~/.factory/specs.
func List() ([]SpecFile, error) {
	home, err := homeDir()
	if err != nil {
		return []SpecFile{}, err
	}
	return listSpecsForHome(home)
}

// RenameSpec renames a spec file inside the specs directory.
func Rename(oldPath string, newName string) (SpecFile, error) {
	home, err := homeDir()
	if err != nil {
		return SpecFile{}, err
	}
	return renameSpecForHome(home, oldPath, newName)
}

// DeleteSpec deletes a spec file inside the specs directory.
func Delete(path string) error {
	home, err := homeDir()
	if err != nil {
		return err
	}
	return deleteSpecForHome(home, path)
}

// UpdateSpec overwrites a spec file's content.
func Update(path string, content string) (SpecFile, error) {
	home, err := homeDir()
	if err != nil {
		return SpecFile{}, err
	}
	return updateSpecForHome(home, path, content)
}

// Dir returns the specs directory for the current home without creating it.
// The GUI's watcher (internal/app) resolves it to install its poll loop.
func Dir() (string, error) {
	home, err := homeDir()
	if err != nil {
		return "", err
	}
	return specsDirForHome(home), nil
}
