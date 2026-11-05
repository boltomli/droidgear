package testutil

import (
	"os"
	"path/filepath"
	"testing"
)

// WriteTestFile creates path (including parents) with the given content.
// It is shared by internal/app's tests and the service layer's tests, which
// is why it lives here instead of in either package.
func WriteTestFile(t *testing.T, path string, content string) {
	t.Helper()
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil { // #nosec G301 -- test fixture
		t.Fatalf("mkdir: %v", err)
	}
	if err := os.WriteFile(path, []byte(content), 0o644); err != nil { // #nosec G306 -- test fixture
		t.Fatalf("write %s: %v", path, err)
	}
}
