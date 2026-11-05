package specs

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"
)

func specsTestHome(t *testing.T) string {
	t.Helper()
	return t.TempDir()
}

func writeSpecFixture(t *testing.T, home string, name string, content string) string {
	t.Helper()
	dir := filepath.Join(home, ".factory", "specs")
	if err := os.MkdirAll(dir, 0o755); err != nil {
		t.Fatalf("mkdir specs: %v", err)
	}
	path := filepath.Join(dir, name)
	if err := os.WriteFile(path, []byte(content), 0o644); err != nil {
		t.Fatalf("write spec: %v", err)
	}
	return path
}

func TestSpecsListFiltersMarkdownAndSortsByMtime(t *testing.T) {
	home := specsTestHome(t)
	oldPath := writeSpecFixture(t, home, "old.md", "old content")
	writeSpecFixture(t, home, "new.md", "new content")
	writeSpecFixture(t, home, "notes.txt", "not a spec")
	// Non-UTF-8 content mirrors Rust read_to_string skipping the file.
	writeSpecFixture(t, home, "binary.md", "\xff\xfe")

	old := time.Now().Add(-2 * time.Hour)
	if err := os.Chtimes(oldPath, old, old); err != nil {
		t.Fatalf("chtimes: %v", err)
	}

	specs, err := listSpecsForHome(home)
	if err != nil {
		t.Fatalf("listSpecsForHome: %v", err)
	}
	if len(specs) != 2 {
		t.Fatalf("expected 2 specs, got %d: %+v", len(specs), specs)
	}
	if specs[0].Name != "new.md" || specs[1].Name != "old.md" {
		t.Fatalf("expected newest first, got %s then %s", specs[0].Name, specs[1].Name)
	}
	if specs[1].Content != "old content" {
		t.Fatalf("content not loaded: %q", specs[1].Content)
	}
	if specs[0].ModifiedAt <= specs[1].ModifiedAt {
		t.Fatalf("modifiedAt not descending: %d <= %d", specs[0].ModifiedAt, specs[1].ModifiedAt)
	}

	// Missing directory → empty list, not an error.
	empty := specsTestHome(t)
	got, err := listSpecsForHome(empty)
	if err != nil {
		t.Fatalf("list on missing dir: %v", err)
	}
	if len(got) != 0 {
		t.Fatalf("expected empty list, got %d", len(got))
	}
}

func TestSpecsReadUpdateRenameDelete(t *testing.T) {
	home := specsTestHome(t)
	path := writeSpecFixture(t, home, "feature.md", "v1")

	spec, err := readSpecFile(path)
	if err != nil {
		t.Fatalf("readSpecFile: %v", err)
	}
	if spec.Name != "feature.md" || spec.Content != "v1" || spec.Path != path {
		t.Fatalf("unexpected spec: %+v", spec)
	}
	if _, err := readSpecFile(filepath.Join(home, ".factory", "specs", "missing.md")); err == nil || err.Error() != "Spec file not found" {
		t.Fatalf("expected Spec file not found, got %v", err)
	}

	updated, err := updateSpecForHome(home, path, "v2")
	if err != nil {
		t.Fatalf("updateSpecForHome: %v", err)
	}
	if updated.Content != "v2" {
		t.Fatalf("update lost content: %+v", updated)
	}
	if data, _ := os.ReadFile(path); string(data) != "v2" {
		t.Fatalf("file not written: %q", data)
	}

	renamed, err := renameSpecForHome(home, path, "renamed")
	if err != nil {
		t.Fatalf("renameSpecForHome: %v", err)
	}
	if renamed.Name != "renamed.md" {
		t.Fatalf("expected .md suffix appended, got %q", renamed.Name)
	}
	if _, err := os.Stat(filepath.Join(home, ".factory", "specs", "renamed.md")); err != nil {
		t.Fatalf("renamed file missing: %v", err)
	}

	// Guards mirror the Rust core: renaming onto the same path is allowed,
	// renaming onto a different existing file is not.
	if _, err := renameSpecForHome(home, renamed.Path, "renamed"); err != nil {
		t.Fatalf("self-rename must succeed: %v", err)
	}
	writeSpecFixture(t, home, "other.md", "occupied")
	if _, err := renameSpecForHome(home, renamed.Path, "other"); err == nil || err.Error() != "A file with this name already exists" {
		t.Fatalf("expected duplicate error, got %v", err)
	}
	if _, err := renameSpecForHome(home, renamed.Path, "  "); err == nil || err.Error() != "File name cannot be empty" {
		t.Fatalf("expected empty-name error, got %v", err)
	}
	if _, err := renameSpecForHome(home, renamed.Path, "sub/name"); err == nil || err.Error() != "Invalid file name" {
		t.Fatalf("expected invalid-name error, got %v", err)
	}
	outside := filepath.Join(home, ".factory", "other.md")
	if err := os.WriteFile(outside, []byte("x"), 0o644); err != nil {
		t.Fatalf("write outside: %v", err)
	}
	if _, err := renameSpecForHome(home, outside, "x"); err == nil || err.Error() != "Invalid file path" {
		t.Fatalf("expected invalid-path error, got %v", err)
	}
	if _, err := updateSpecForHome(home, outside, "x"); err == nil || err.Error() != "Invalid file path" {
		t.Fatalf("expected invalid-path error on update, got %v", err)
	}
	if err := deleteSpecForHome(home, outside); err == nil || err.Error() != "Invalid file path" {
		t.Fatalf("expected invalid-path error on delete, got %v", err)
	}

	if err := deleteSpecForHome(home, renamed.Path); err != nil {
		t.Fatalf("deleteSpecForHome: %v", err)
	}
	if _, err := os.Stat(renamed.Path); !os.IsNotExist(err) {
		t.Fatalf("spec not deleted: %v", err)
	}
	if err := deleteSpecForHome(home, renamed.Path); err == nil || err.Error() != "Spec file not found" {
		t.Fatalf("expected Spec file not found, got %v", err)
	}
}

func TestSpecsHelpersMirrorPathSemantics(t *testing.T) {
	if !droidPathHasPrefix(filepath.Join("a", "b", "c.md"), filepath.Join("a", "b")) {
		t.Error("expected prefix match")
	}
	if droidPathHasPrefix(filepath.Join("a", "bc", "c.md"), filepath.Join("a", "b")) {
		t.Error("partial component must not match")
	}
	if droidPathHasPrefix(filepath.Join("a", "b"), filepath.Join("a", "b", "c")) {
		t.Error("child must not match longer prefix")
	}
	if !droidPathHasPrefix(string(filepath.Separator)+"foo", string(filepath.Separator)) {
		t.Error("root prefix must match children")
	}
	if !strings.HasSuffix(droidFileStem("my.profile.json"), "profile") {
		t.Error("file stem keeps intermediate dots")
	}
}
