package testutil

import (
	"os"
	"path/filepath"
	"testing"
)

// DshTestHome builds an isolated home with a `desktop` profile declaring the
// real base + web-app bundles, mirroring the Rust test fixture. Shared by
// internal/app's smoke tests and service/dsh's tests.
func DshTestHome(t *testing.T) string {
	t.Helper()
	home := IsolatedHome(t)
	profileDir := filepath.Join(home, ".dsh", "profiles", "desktop")
	if err := os.MkdirAll(profileDir, 0o755); err != nil {
		t.Fatalf("profile dir: %v", err)
	}
	manifest := `{
  "name": "dsh-profile-desktop",
  "private": true,
  "dsh": { "profile": { "bundles": ["@deepseek-ai/dsh-base", "@deepseek-ai/dsh-web-app"] } }
}`
	if err := os.WriteFile(filepath.Join(profileDir, "package.json"), []byte(manifest), 0o644); err != nil {
		t.Fatalf("manifest: %v", err)
	}
	if err := os.WriteFile(filepath.Join(profileDir, "cordis.yml"), []byte("[]\n"), 0o644); err != nil {
		t.Fatalf("cordis.yml: %v", err)
	}
	return home
}
