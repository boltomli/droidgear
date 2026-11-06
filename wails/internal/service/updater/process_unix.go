//go:build !windows

package updater

import (
	"errors"
	"os/exec"
	"syscall"
)

// processAlive reports whether pid still exists (signal 0 probes without
// delivering anything; EPERM still means "alive, not ours").
func processAlive(pid int) bool {
	if pid <= 0 {
		return false
	}
	err := syscall.Kill(pid, 0)
	return err == nil || errors.Is(err, syscall.EPERM)
}

// spawnRelaunch starts the freshly installed GUI binary in its own session so
// it survives this process exiting. The child waits for our PID before it
// initialises Wails (see RelaunchPID), because the single-instance lock is
// only released on exit.
func spawnRelaunch(exe string) error {
	cmd := exec.Command(exe, RelaunchArgs()...)
	cmd.SysProcAttr = &syscall.SysProcAttr{Setsid: true}
	if err := cmd.Start(); err != nil {
		return err
	}
	return cmd.Process.Release()
}
