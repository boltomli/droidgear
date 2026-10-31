//go:build windows

package core

import (
	"os"
	"os/exec"
	"syscall"
)

// runWslCapture executes wsl.exe with the given arguments and returns its
// stdout. The CREATE_NO_WINDOW flag prevents a console window from flashing
// (mirrors the Rust core's creation_flags).
func runWslCapture(args ...string) (string, bool) {
	cmd := exec.Command("wsl", args...) // #nosec G204 -- fixed argument vector
	cmd.SysProcAttr = &syscall.SysProcAttr{HideWindow: true}
	output, err := cmd.Output()
	if err != nil {
		return "", false
	}
	return string(output), true
}

// localAppData returns %LOCALAPPDATA%.
func localAppData() string {
	return os.Getenv("LOCALAPPDATA")
}
