package app

import (
	"droidgear-wails/internal/core"
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func trustedWriteConfig(t *testing.T, home string, contents string) string {
	t.Helper()
	dir := filepath.Join(home, ".factory")
	if err := os.MkdirAll(dir, 0o755); err != nil {
		t.Fatalf("mkdir factory: %v", err)
	}
	path := filepath.Join(dir, "settings.json")
	if err := os.WriteFile(path, []byte(contents), 0o644); err != nil {
		t.Fatalf("write config: %v", err)
	}
	return path
}

func trustedReadConfig(t *testing.T, path string) map[string]any {
	t.Helper()
	data, err := os.ReadFile(path) // #nosec G304 -- test fixture
	if err != nil {
		t.Fatalf("read config: %v", err)
	}
	value, err := core.DecodeJSONValue(data)
	if err != nil {
		t.Fatalf("parse config: %v", err)
	}
	obj, ok := value.(map[string]any)
	if !ok {
		t.Fatalf("config not an object: %T", value)
	}
	return obj
}

func TestTrustedFoldersListSortedPreservingTimestampsAndOtherSettings(t *testing.T) {
	home := t.TempDir()
	trustedWriteConfig(t, home, `{
	  "customModels": [{"id":"keep"}],
	  "trustedFolders": {
	    "/tmp/z": {"trustedAt": "2026-01-02T00:00:00.000Z"},
	    "/tmp/a": {"trustedAt": "2026-01-01T00:00:00.000Z"},
	    "/tmp/b": "2025-12-31T00:00:00.000Z"
	  }
	}`)

	folders, err := droidListTrustedFoldersForHome(home)
	if err != nil {
		t.Fatalf("list: %v", err)
	}
	if len(folders) != 3 {
		t.Fatalf("expected 3 folders, got %+v", folders)
	}
	if folders[0].Path != "/tmp/a" || folders[1].Path != "/tmp/b" || folders[2].Path != "/tmp/z" {
		t.Fatalf("not sorted by path: %+v", folders)
	}
	if folders[0].TrustedAt != "2026-01-01T00:00:00.000Z" {
		t.Fatalf("timestamp lost: %+v", folders[0])
	}
	// Bare-string entries (older builds) keep their timestamp.
	if folders[1].TrustedAt != "2025-12-31T00:00:00.000Z" {
		t.Fatalf("bare string timestamp lost: %+v", folders[1])
	}

	// Missing file → empty list.
	empty, err := droidListTrustedFoldersForHome(t.TempDir())
	if err != nil {
		t.Fatalf("list missing: %v", err)
	}
	if len(empty) != 0 {
		t.Fatalf("expected empty list, got %+v", empty)
	}
}

func TestTrustedFoldersAddIsIdempotentAndPreservesOtherSettings(t *testing.T) {
	home := t.TempDir()
	configPath := trustedWriteConfig(t, home, `{"customModels":[{"id":"keep"}],"trustedFolders":{}}`)
	project := filepath.Join(t.TempDir(), "project")
	if err := os.MkdirAll(project, 0o755); err != nil {
		t.Fatalf("mkdir project: %v", err)
	}

	added, err := droidAddTrustedFolderForHome(home, project)
	if err != nil {
		t.Fatalf("add: %v", err)
	}
	if added.Path != project || !strings.HasSuffix(added.TrustedAt, "Z") || len(added.TrustedAt) != 24 {
		t.Fatalf("unexpected entry: %+v", added)
	}
	again, err := droidAddTrustedFolderForHome(home, project)
	if err != nil {
		t.Fatalf("add again: %v", err)
	}
	if again.TrustedAt != added.TrustedAt {
		t.Fatalf("re-add must keep the original timestamp: %q → %q", added.TrustedAt, again.TrustedAt)
	}

	config := trustedReadConfig(t, configPath)
	models, ok := config["customModels"].([]any)
	if !ok || len(models) != 1 {
		t.Fatalf("customModels lost: %#v", config["customModels"])
	}
	entries := config["trustedFolders"].(map[string]any)
	entry, ok := entries[project].(map[string]any)
	if !ok || entry["trustedAt"] != added.TrustedAt {
		t.Fatalf("entry shape wrong: %#v", entries[project])
	}
}

func TestTrustedFoldersAddRejectsBadPaths(t *testing.T) {
	home := t.TempDir()
	trustedWriteConfig(t, home, "{}")

	if _, err := droidAddTrustedFolderForHome(home, "  "); err == nil || err.Error() != "Trusted folder path cannot be empty" {
		t.Fatalf("expected empty error, got %v", err)
	}
	if _, err := droidAddTrustedFolderForHome(home, "relative/path"); err == nil || err.Error() != "Trusted folder path must be absolute" {
		t.Fatalf("expected absolute error, got %v", err)
	}
	file := filepath.Join(t.TempDir(), "file.txt")
	if err := os.WriteFile(file, []byte("x"), 0o644); err != nil {
		t.Fatalf("write file: %v", err)
	}
	if _, err := droidAddTrustedFolderForHome(home, file); err == nil || err.Error() != "Trusted folder path must point to a directory" {
		t.Fatalf("expected directory error, got %v", err)
	}
	if _, err := droidAddTrustedFolderForHome(home, filepath.Join(t.TempDir(), "missing-dir")); err == nil || !strings.Contains(err.Error(), "Trusted folder does not exist") {
		t.Fatalf("expected does-not-exist error, got %v", err)
	}
}

func TestTrustedFoldersRemoveSingleAndBatch(t *testing.T) {
	home := t.TempDir()
	configPath := trustedWriteConfig(t, home, `{
	  "customModels": [{"id":"keep"}],
	  "trustedFolders": {
	    "/tmp/a": {"trustedAt": "a"},
	    "/tmp/b": {"trustedAt": "b"},
	    "/tmp/c": {"trustedAt": "c"}
	  }
	}`)

	if err := droidRemoveTrustedFoldersForHome(home, []string{" /tmp/a "}); err != nil {
		t.Fatalf("remove single: %v", err)
	}
	config := trustedReadConfig(t, configPath)
	entries := config["trustedFolders"].(map[string]any)
	if _, present := entries["/tmp/a"]; present {
		t.Fatalf("/tmp/a not removed: %#v", entries)
	}
	if entries["/tmp/b"] == nil || entries["/tmp/c"] == nil {
		t.Fatalf("other entries lost: %#v", entries)
	}

	// One read-modify-write for a batch, trimming each path.
	if err := droidRemoveTrustedFoldersForHome(home, []string{"/tmp/b", "/tmp/c"}); err != nil {
		t.Fatalf("remove batch: %v", err)
	}
	config = trustedReadConfig(t, configPath)
	entries = config["trustedFolders"].(map[string]any)
	if len(entries) != 0 {
		t.Fatalf("entries remain: %#v", entries)
	}
	if _, ok := config["customModels"].([]any); !ok {
		t.Fatalf("customModels lost: %#v", config["customModels"])
	}

	// Validation mirrors the Rust core.
	if err := droidRemoveTrustedFoldersForHome(home, []string{" "}); err == nil || err.Error() != "Trusted folder path cannot be empty" {
		t.Fatalf("expected empty error, got %v", err)
	}
	if err := droidRemoveTrustedFoldersForHome(home, []string{}); err != nil {
		t.Fatalf("empty batch must succeed: %v", err)
	}

	// Removing from a config without trustedFolders is a silent success.
	bare := t.TempDir()
	trustedWriteConfig(t, bare, `{"customModels":[]}`)
	if err := droidRemoveTrustedFoldersForHome(bare, []string{"/tmp/x"}); err != nil {
		t.Fatalf("remove without key: %v", err)
	}
}

func TestTrustedFoldersRejectsNonObjectShape(t *testing.T) {
	home := t.TempDir()
	trustedWriteConfig(t, home, `{"trustedFolders":["/tmp/a"]}`)

	if _, err := droidListTrustedFoldersForHome(home); err == nil || err.Error() != "Factory settings trustedFolders must be a JSON object" {
		t.Fatalf("expected object error on list, got %v", err)
	}
	if err := droidRemoveTrustedFoldersForHome(home, []string{"/tmp/a"}); err == nil || err.Error() != "Factory settings trustedFolders must be a JSON object" {
		t.Fatalf("expected object error on remove, got %v", err)
	}

	// Null trustedFolders lists as empty but fails on add (needs an object).
	trustedWriteConfig(t, home, `{"trustedFolders":null}`)
	folders, err := droidListTrustedFoldersForHome(home)
	if err != nil || len(folders) != 0 {
		t.Fatalf("null must list empty: %v %+v", err, folders)
	}
	nullDir := filepath.Join(t.TempDir(), "null-project")
	if err := os.MkdirAll(nullDir, 0o755); err != nil {
		t.Fatalf("mkdir: %v", err)
	}
	if _, err := droidAddTrustedFolderForHome(home, nullDir); err == nil || err.Error() != "Factory settings trustedFolders must be a JSON object" {
		t.Fatalf("expected object error on null add, got %v", err)
	}

	// Invalid entry shapes are rejected on list.
	trustedWriteConfig(t, home, `{"trustedFolders":{"\/tmp\/x":123}}`)
	if _, err := droidListTrustedFoldersForHome(home); err == nil || err.Error() != "Factory settings trustedFolders contains an invalid entry" {
		t.Fatalf("expected invalid-entry error, got %v", err)
	}

	// Non-object top level lists as empty and rejects add.
	trustedWriteConfig(t, home, `[1,2]`)
	folders, err = droidListTrustedFoldersForHome(home)
	if err != nil || len(folders) != 0 {
		t.Fatalf("array config must list empty: %v %+v", err, folders)
	}
	dir := filepath.Join(t.TempDir(), "d")
	if err := os.MkdirAll(dir, 0o755); err != nil {
		t.Fatalf("mkdir: %v", err)
	}
	if _, err := droidAddTrustedFolderForHome(home, dir); err == nil || err.Error() != "Factory settings must contain a JSON object" {
		t.Fatalf("expected must-be-object error, got %v", err)
	}
}

func TestTrustedFoldersCommandSurface(t *testing.T) {
	isolatedHome(t)
	a := &App{}
	dir := filepath.Join(t.TempDir(), "project")
	if err := os.MkdirAll(dir, 0o755); err != nil {
		t.Fatalf("mkdir: %v", err)
	}

	list, err := a.ListDroidTrustedFolders()
	if err != nil {
		t.Fatalf("ListDroidTrustedFolders: %v", err)
	}
	if len(list) != 0 {
		t.Fatalf("expected empty list, got %+v", list)
	}
	added, err := a.AddDroidTrustedFolder(dir)
	if err != nil {
		t.Fatalf("AddDroidTrustedFolder: %v", err)
	}
	if added.Path != dir {
		t.Fatalf("unexpected path: %+v", added)
	}
	if err := a.RemoveDroidTrustedFolder(dir); err != nil {
		t.Fatalf("RemoveDroidTrustedFolder: %v", err)
	}
	list, err = a.ListDroidTrustedFolders()
	if err != nil {
		t.Fatalf("list after remove: %v", err)
	}
	if len(list) != 0 {
		t.Fatalf("expected empty list, got %+v", list)
	}

	// The written document stays plain JSON (pretty, no HTML escaping).
	home, _ := droidHomeDir()
	data, err := os.ReadFile(filepath.Join(home, ".factory", "settings.json")) // #nosec G304 -- isolated home
	if err != nil {
		t.Fatalf("read: %v", err)
	}
	var probe map[string]json.RawMessage
	if err := json.Unmarshal(data, &probe); err != nil {
		t.Fatalf("written config not valid JSON: %v", err)
	}
}
