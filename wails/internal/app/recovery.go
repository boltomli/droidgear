// Emergency data (crash recovery) commands. Mirrors src-tauri/src/commands/
// recovery.rs including the structured RecoveryError contract: the frontend
// matches on `error.type`, so errors are returned as JSON strings that the
// invoke shim forwards verbatim (see src/lib/recovery.ts).
package app

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"regexp"
	"strings"
	"time"

	"droidgear-wails/internal/core"
)

// maxRecoveryDataBytes mirrors types::MAX_RECOVERY_DATA_BYTES (10 MiB).
const maxRecoveryDataBytes = 10 << 20

// recoveryDir returns ~/.droidgear/recovery.
func recoveryDir() (string, error) {
	dir, err := core.DroidGearDir()
	if err != nil {
		return "", err
	}
	return filepath.Join(dir, "recovery"), nil
}

var recoveryFilenamePattern = regexp.MustCompile(`^[a-zA-Z0-9_-]+(\.[a-zA-Z0-9]+)?$`)

func validateRecoveryFilename(filename string) error {
	if filename == "" {
		return errString(`{"type":"ValidationError","message":"Filename cannot be empty"}`)
	}
	if len([]rune(filename)) > 100 {
		return errString(`{"type":"ValidationError","message":"Filename too long (max 100 characters)"}`)
	}
	if !recoveryFilenamePattern.MatchString(filename) {
		return errString(`{"type":"ValidationError","message":"Invalid filename: only alphanumeric characters, dashes, underscores, and dots allowed"}`)
	}
	return nil
}

// SaveEmergencyData writes data as pretty JSON to the recovery directory.
func (a *App) SaveEmergencyData(filename string, data any) error {
	if err := validateRecoveryFilename(filename); err != nil {
		return err
	}
	content, err := json.MarshalIndent(data, "", "  ")
	if err != nil {
		return errString(fmt.Sprintf(`{"type":"ParseError","message":%q}`, err.Error()))
	}
	if len(content) > maxRecoveryDataBytes {
		return errString(fmt.Sprintf(`{"type":"DataTooLarge","max_bytes":%d}`, maxRecoveryDataBytes))
	}
	dir, err := recoveryDir()
	if err != nil {
		return errString(fmt.Sprintf(`{"type":"IoError","message":%q}`, err.Error()))
	}
	if err := core.EnsureDir(dir); err != nil {
		return errString(fmt.Sprintf(`{"type":"IoError","message":%q}`, err.Error()))
	}
	target := filepath.Join(dir, filename+".json")
	if err := core.AtomicWrite(target, content); err != nil {
		return errString(fmt.Sprintf(`{"type":"IoError","message":%q}`, err.Error()))
	}
	return nil
}

// LoadEmergencyData reads previously saved emergency data. Returns the
// structured FileNotFound error when no file exists (expected case).
func (a *App) LoadEmergencyData(filename string) (any, error) {
	if err := validateRecoveryFilename(filename); err != nil {
		return nil, err
	}
	dir, err := recoveryDir()
	if err != nil {
		return nil, errString(fmt.Sprintf(`{"type":"IoError","message":%q}`, err.Error()))
	}
	target := filepath.Join(dir, filename+".json")
	raw, err := os.ReadFile(target) // #nosec G304 -- validated filename under app dir
	if err != nil {
		if os.IsNotExist(err) {
			return nil, errString(`{"type":"FileNotFound"}`)
		}
		return nil, errString(fmt.Sprintf(`{"type":"IoError","message":%q}`, err.Error()))
	}
	var out any
	if err := json.Unmarshal(raw, &out); err != nil {
		return nil, errString(fmt.Sprintf(`{"type":"ParseError","message":%q}`, err.Error()))
	}
	return out, nil
}

// CleanupOldRecoveryFiles removes recovery files older than seven days and
// returns the number of removed files.
func (a *App) CleanupOldRecoveryFiles() (int, error) {
	dir, err := recoveryDir()
	if err != nil {
		return 0, errString(fmt.Sprintf(`{"type":"IoError","message":%q}`, err.Error()))
	}
	entries, err := os.ReadDir(dir) // #nosec G304 -- app-managed directory
	if err != nil {
		if os.IsNotExist(err) {
			return 0, nil
		}
		return 0, errString(fmt.Sprintf(`{"type":"IoError","message":%q}`, err.Error()))
	}
	removed := 0
	cutoff := time.Now().Add(-7 * 24 * time.Hour)
	for _, entry := range entries {
		if entry.IsDir() || !strings.HasSuffix(entry.Name(), ".json") {
			continue
		}
		info, err := entry.Info()
		if err != nil {
			continue
		}
		if info.ModTime().Before(cutoff) {
			if os.Remove(filepath.Join(dir, entry.Name())) == nil {
				removed++
			}
		}
	}
	return removed, nil
}
