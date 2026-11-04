// Package testutil holds test-only helpers shared by internal/app's tests and
// the service layer's tests. Nothing in production code imports it — it exists
// because both sides of the app/service split need the same isolated home and
// the same canned fixtures, and duplicating those silently is how a test
// starts writing to a developer's real profile.
package testutil

import (
	"path/filepath"
	"testing"
)

// IsolatedHome points path resolution at a throwaway home directory so tests
// never touch the real ~/.droidgear, ~/.pi, etc.
//
// LOCALAPPDATA and PATH are isolated as well: HermesHome's Windows priority
// chain reads %LOCALAPPDATA% directly (bypassing USERPROFILE), and the final
// fallback probes the default WSL distro via `wsl.exe`. Without these two,
// ApplyHermesProfile rewrote the developer's REAL %LOCALAPPDATA%/hermes/
// config.yaml — or created \\wsl$\...\.hermes inside the real distro — on
// every `go test`.
func IsolatedHome(t *testing.T) string {
	t.Helper()
	home := t.TempDir()
	t.Setenv("USERPROFILE", home)
	t.Setenv("HOME", home)
	t.Setenv("LOCALAPPDATA", filepath.Join(home, "AppData", "Local"))
	// Empty PATH: `wsl.exe` can't be resolved, so defaultHermesHomeWSL
	// skips the WSL branch and falls back to the (isolated) ~/.hermes.
	t.Setenv("PATH", filepath.Join(home, "empty-bin"))
	return home
}
