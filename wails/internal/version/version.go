// Package version holds the build version shared by every binary in the
// repo: the Wails GUI (App.GetAppVersion, the About dialog) and the `dg`
// CLI (`dg version`). One ldflags target means the GUI and the CLI can
// never report different versions for the same build.
//
// Injected at link time via
// `-ldflags "-X droidgear-wails/internal/version.Version=<version>"` in the
// Taskfiles (wails/build/Taskfile.yml, wails/build/windows/Taskfile.yml);
// the version itself comes from the repo-root package.json through
// wails/scripts/sync-version.mjs (single source of truth shared with the
// Tauri build).
package version

// Version is the application version. The "dev" fallback covers
// `wails3 dev`, `go test` and plain `go run` builds, which build without
// ldflags.
var Version = "dev"
