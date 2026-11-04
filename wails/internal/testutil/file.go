package testutil

import (
	"os"
	"testing"
)

// MustRead reads a test fixture and fails the test instead of returning an
// error, so call sites stay one line.
func MustRead(t *testing.T, path string) string {
	t.Helper()
	data, err := os.ReadFile(path) // #nosec G304 -- test fixture
	if err != nil {
		t.Fatalf("read %s: %v", path, err)
	}
	return string(data)
}
