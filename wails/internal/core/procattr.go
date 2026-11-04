// Cross-platform child-process launch flags.
//
// The GUI shells spawn console tools (PowerShell registry probes, `cmd /c
// start` launchers, taskkill) that must not flash a console window. Windows
// exposes that through syscall.SysProcAttr{HideWindow, CreationFlags}, but
// those fields only exist on Windows: a struct literal naming them makes this
// package fail to compile on macOS/Linux, which the single-binary release
// workflow builds (three platforms, one binary each). The flags are therefore
// applied reflectively — identical behaviour on Windows, a no-op elsewhere.
package core

import (
	"os/exec"
	"reflect"
	"syscall"
)

// Windows process-creation flags (winbase.h).
const (
	processCreateNoWindow   = 0x08000000 // CREATE_NO_WINDOW
	processCreateNewConsole = 0x00000010 // CREATE_NEW_CONSOLE
)

// setProcessCreationFlags attaches the Windows process-creation flags to cmd.
// syscall.SysProcAttr only carries those fields on Windows, and this package
// must compile everywhere, so they are set reflectively; on other platforms
// the helper is a no-op.
func setProcessCreationFlags(cmd *exec.Cmd, hide bool, creationFlags uint32) {
	if !hide && creationFlags == 0 {
		return
	}
	attr := &syscall.SysProcAttr{}
	value := reflect.ValueOf(attr).Elem()
	applied := false
	if hide {
		if field := value.FieldByName("HideWindow"); field.IsValid() && field.CanSet() && field.Kind() == reflect.Bool {
			field.SetBool(true)
			applied = true
		}
	}
	if creationFlags != 0 {
		if field := value.FieldByName("CreationFlags"); field.IsValid() && field.CanSet() && field.Kind() == reflect.Uint32 {
			field.SetUint(uint64(creationFlags))
			applied = true
		}
	}
	if applied {
		cmd.SysProcAttr = attr
	}
}

// hideConsoleWindow runs cmd without a console window (CREATE_NO_WINDOW).
// Used for short-lived helper probes that must stay invisible in a GUI app.
func HideConsoleWindow(cmd *exec.Cmd) {
	setProcessCreationFlags(cmd, true, processCreateNoWindow)
}

// useNewConsole gives cmd its own console window (CREATE_NEW_CONSOLE), used
// when a GUI process launches an interactive shell the user must see.
func UseNewConsole(cmd *exec.Cmd) {
	setProcessCreationFlags(cmd, false, processCreateNewConsole)
}
