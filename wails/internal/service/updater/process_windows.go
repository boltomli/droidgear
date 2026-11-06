//go:build windows

package updater

import (
	"errors"
	"os/exec"
	"syscall"

	"golang.org/x/sys/windows"
)

// processAlive reports whether pid still exists. OpenProcess fails with
// ERROR_INVALID_PARAMETER once the process is gone; ERROR_ACCESS_DENIED means
// it exists but belongs to another user.
func processAlive(pid int) bool {
	if pid <= 0 {
		return false
	}
	handle, err := windows.OpenProcess(windows.SYNCHRONIZE|windows.PROCESS_QUERY_LIMITED_INFORMATION, false, uint32(pid))
	if err != nil {
		return errors.Is(err, windows.ERROR_ACCESS_DENIED)
	}
	defer func() { _ = windows.CloseHandle(handle) }()
	status, err := windows.WaitForSingleObject(handle, 0)
	return err == nil && status == uint32(windows.WAIT_TIMEOUT)
}

// spawnRelaunch starts the freshly installed GUI binary detached (no console
// window, own process group) so it outlives this process. The child waits for
// our PID before it initialises Wails (see RelaunchPID), because the
// single-instance lock is only released on exit.
func spawnRelaunch(exe string) error {
	cmd := exec.Command(exe, RelaunchArgs()...)
	cmd.SysProcAttr = &syscall.SysProcAttr{
		HideWindow:    true,
		CreationFlags: windows.CREATE_NEW_PROCESS_GROUP | windows.DETACHED_PROCESS,
	}
	if err := cmd.Start(); err != nil {
		return err
	}
	return cmd.Process.Release()
}
