// External links and file-manager integration — the Go side of
// `@tauri-apps/plugin-opener`. The Wails build aliases that plugin to
// src/wails/shims/tauri-opener.ts, which routes openUrl / openPath /
// revealItemInDir to these methods (used by AboutPane, McpPage,
// LegacyVersionsPage and ExportTemplatesPage).
package app

import (
	"fmt"
	"os/exec"
	"path"
	"runtime"
	"strings"
)

// OpenUrl opens a URL in the user's default handler (browser / mail client).
// Mirrors the plugin-opener `open_url` command.
func (a *App) OpenUrl(url string) error {
	if strings.TrimSpace(url) == "" {
		return errString("Failed to open URL: URL is empty")
	}
	name, args := openerCommand(runtime.GOOS, url)
	if err := startDetached(name, args...); err != nil {
		return fmt.Errorf("Failed to open URL: %w", err)
	}
	return nil
}

// OpenPath opens a file or directory with the default handler. Mirrors the
// plugin-opener `open_path` command.
func (a *App) OpenPath(path string) error {
	if strings.TrimSpace(path) == "" {
		return errString("Failed to open path: path is empty")
	}
	name, args := openerCommand(runtime.GOOS, path)
	if err := startDetached(name, args...); err != nil {
		return fmt.Errorf("Failed to open path: %w", err)
	}
	return nil
}

// RevealItemInDir shows a file in its parent folder (Windows: explorer
// /select, macOS: open -R, Linux: open the containing directory). Mirrors
// the plugin-opener `reveal_item_in_dir` command.
func (a *App) RevealItemInDir(item string) error {
	if strings.TrimSpace(item) == "" {
		return errString("Failed to reveal item: path is empty")
	}
	name, args := revealCommand(runtime.GOOS, item)
	if err := startDetached(name, args...); err != nil {
		return fmt.Errorf("Failed to reveal item: %w", err)
	}
	return nil
}

// openerCommand maps a target (URL or filesystem path) to the platform's
// "open with default handler" invocation. goos is a parameter so the table
// is testable from any host.
func openerCommand(goos, target string) (string, []string) {
	switch goos {
	case "windows":
		// url.dll's FileProtocolHandler handles http(s) URLs, local files
		// and folders alike, without flashing a console window.
		return "rundll32", []string{"url.dll,FileProtocolHandler", target}
	case "darwin":
		return "open", []string{target}
	default:
		return "xdg-open", []string{target}
	}
}

// revealCommand maps an item path to the platform's "reveal in file manager"
// invocation.
func revealCommand(goos, item string) (string, []string) {
	switch goos {
	case "windows":
		return "explorer", []string{"/select," + item}
	case "darwin":
		return "open", []string{"-R", item}
	default:
		// Linux file managers have no cross-desktop "select" primitive;
		// open the containing directory instead. Slash-based path.Dir keeps
		// the result host-independent (POSIX targets only).
		return "xdg-open", []string{path.Dir(item)}
	}
}

// startDetached launches a launcher process (rundll32/explorer/open/xdg-open)
// with a hidden console on Windows and does not wait for it — the opener
// outlives the call, like the Tauri plugin's implementation.
func startDetached(name string, args ...string) error {
	// #nosec G204 -- fixed launcher binaries; the target is passed as a
	// single argument, never evaluated by a shell.
	cmd := exec.Command(name, args...)
	hideConsoleWindow(cmd)
	if err := cmd.Start(); err != nil {
		return err
	}
	go func() { _ = cmd.Wait() }()
	return nil
}
