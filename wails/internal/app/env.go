// Environment commands. Mirrors src-tauri/src/commands/env.rs: on Windows the
// process env is already correct for a GUI app; on other platforms a login
// shell is queried.
package app

import (
	"fmt"
	"os"
	"os/exec"
	"runtime"
	"strings"
)

// GetEnvVar returns a process environment variable.
func (a *App) GetEnvVar(name string) *string {
	value := os.Getenv(name)
	if value == "" {
		if _, ok := os.LookupEnv(name); !ok {
			return nil
		}
	}
	return &value
}

// SetEnvVar sets a process environment variable.
func (a *App) SetEnvVar(name string, value string) {
	os.Setenv(name, value) // #nosec G104 -- mirror of the Tauri command's best-effort semantics
}

// RemoveEnvVar removes a process environment variable.
func (a *App) RemoveEnvVar(name string) {
	os.Unsetenv(name) // #nosec G104 -- best-effort
}

// GetShellEnv returns the environment as seen by a login shell (on Windows:
// the current process environment).
func (a *App) GetShellEnv() (map[string]string, error) {
	if runtime.GOOS != "windows" {
		shell := os.Getenv("SHELL")
		if shell == "" {
			shell = "/bin/zsh"
		}
		out, err := exec.Command(shell, "-l", "-c", "env").Output() // #nosec G204 -- $SHELL is the user's own shell
		if err != nil {
			return nil, fmt.Errorf("Failed to run shell: %w", err)
		}
		env := map[string]string{}
		for _, line := range strings.Split(string(out), "\n") {
			if key, value, ok := strings.Cut(line, "="); ok {
				env[key] = value
			}
		}
		return env, nil
	}
	env := map[string]string{}
	for _, kv := range os.Environ() {
		if key, value, ok := strings.Cut(kv, "="); ok {
			env[key] = value
		}
	}
	return env, nil
}
