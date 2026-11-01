// Plain-text file IO used by the channels list page (JSON import/export).
// Direct filesystem access is deliberately narrow: absolute paths only, no
// directory traversal from relative inputs.
package app

import (
	"fmt"
	"os"
	"path/filepath"
	"strings"
)

func validateIOPath(path string) error {
	if strings.TrimSpace(path) == "" {
		return errString("Path cannot be empty")
	}
	if !filepath.IsAbs(path) {
		return errString("Path must be absolute")
	}
	return nil
}

// ReadTextFile reads a UTF-8 text file (channel import).
func (a *App) ReadTextFile(path string) (string, error) {
	if err := validateIOPath(path); err != nil {
		return "", err
	}
	raw, err := os.ReadFile(filepath.Clean(path)) // #nosec G304 -- user-picked path via file dialog
	if err != nil {
		return "", fmt.Errorf("Failed to read file: %w", err)
	}
	return string(raw), nil
}

// WriteTextFile writes a UTF-8 text file atomically (channel export).
func (a *App) WriteTextFile(path string, contents string) error {
	if err := validateIOPath(path); err != nil {
		return err
	}
	if parent := filepath.Dir(path); parent != "" {
		if err := os.MkdirAll(parent, 0o755); err != nil { // #nosec G301 -- user-chosen export dir
			return fmt.Errorf("Failed to create directory: %w", err)
		}
	}
	return writeAtomicText(path, contents)
}
