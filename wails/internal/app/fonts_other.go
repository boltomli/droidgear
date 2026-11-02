//go:build !windows

// Non-Windows system font enumeration (POSIX shells):
//   - Linux: `fc-list : family` (fontconfig), the same source the Tauri
//     side's fontdb falls back to when no native API is used.
//   - macOS: scan the standard font directories and derive family names
//     from the file stems. `system_profiler SPFontsDataType` would give
//     exact families but takes seconds per call (guidelines §3.4), and the
//     dropdown only needs a unique, readable list.
//
// This file is never compiled by the Windows-only CI (wails.yml); keep it
// dependency-free stdlib so `gofmt` stays the only gate.

package app

import (
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"strings"
)

// enumerateSystemFonts dispatches to the platform backend.
func enumerateSystemFonts() ([]SystemFont, error) {
	if runtime.GOOS == "darwin" {
		return buildSystemFonts(scanFontDirs()), nil
	}
	return fontconfigFamilies()
}

// fontconfigFamilies runs `fc-list : family` and splits the comma-
// separated family lists fontconfig emits per line.
func fontconfigFamilies() ([]SystemFont, error) {
	// #nosec G204 -- fixed program, fixed arguments
	out, err := exec.Command("fc-list", ":", "family").Output()
	if err != nil {
		return nil, errString("fc-list failed: " + err.Error())
	}
	var families []string
	for _, line := range strings.Split(string(out), "\n") {
		for _, family := range strings.Split(line, ",") {
			if family = strings.TrimSpace(family); family != "" {
				families = append(families, family)
			}
		}
	}
	return buildSystemFonts(families), nil
}

// scanFontDirs walks the system and user font directories, deriving family
// names from the file stems ("Arial Bold.ttf" → "Arial Bold").
func scanFontDirs() []string {
	dirs := []string{"/System/Library/Fonts", "/Library/Fonts"}
	if home, err := os.UserHomeDir(); err == nil && home != "" {
		dirs = append(dirs, filepath.Join(home, "Library", "Fonts"))
	}
	var families []string
	for _, dir := range dirs {
		entries, err := os.ReadDir(dir)
		if err != nil {
			continue // absent on some installs; skip rather than fail
		}
		for _, entry := range entries {
			if entry.IsDir() {
				continue
			}
			ext := strings.ToLower(filepath.Ext(entry.Name()))
			if !fontFileExtensions[ext] {
				continue
			}
			stem := strings.TrimSuffix(entry.Name(), filepath.Ext(entry.Name()))
			if family := fontEntryFamily(stem); family != "" {
				families = append(families, family)
			}
		}
	}
	return families
}
