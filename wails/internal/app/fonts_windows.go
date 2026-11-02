//go:build windows

// Windows system font enumeration: the per-machine and per-user Fonts
// registry keys hold one value per installed face; the value NAME is the
// display name ("Arial Bold (TrueType)") and the value DATA is the font
// file. Family names are approximated from the value name (fontEntryFamily
// in fonts.go) — parsing the actual name tables would need a font library,
// which PORTING.md forbids as a new dependency.

package app

import (
	"os"
	"path/filepath"
	"strings"

	"golang.org/x/sys/windows/registry"
)

// registryFontKeys are the two locations Windows registers fonts in: the
// machine-wide HKLM key (C:\Windows\Fonts) and the per-user key (%LOCALAPPDATA%\Microsoft\Windows\Fonts).
var registryFontKeys = []struct {
	root registry.Key
	path string
}{
	{registry.LOCAL_MACHINE, `SOFTWARE\Microsoft\Windows NT\CurrentVersion\Fonts`},
	{registry.CURRENT_USER, `SOFTWARE\Microsoft\Windows NT\CurrentVersion\Fonts`},
}

// enumerateSystemFonts collects installed font families from the registry.
func enumerateSystemFonts() ([]SystemFont, error) {
	seen := make(map[string]bool)
	var fonts []SystemFont
	var lastErr error
	opened := false

	for _, source := range registryFontKeys {
		key, err := registry.OpenKey(source.root, source.path, registry.QUERY_VALUE)
		if err != nil {
			lastErr = err
			continue
		}
		opened = true
		values, err := key.ReadValueNames(0)
		if err != nil {
			lastErr = err
			key.Close()
			continue
		}
		for _, valueName := range values {
			if valueName == "" {
				continue // default value, not a font
			}
			data, _, err := key.GetStringValue(valueName)
			if err != nil {
				continue // non-string value; not a font entry we can use
			}
			family := fontEntryFamily(valueName)
			if family == "" || seen[strings.ToLower(family)] {
				continue
			}
			seen[strings.ToLower(family)] = true
			fonts = append(fonts, newSystemFont(family, resolveFontPath(data)))
		}
		key.Close()
	}

	if !opened {
		// Both keys unreadable — the dropdown would be silently empty, so
		// surface it like the Tauri plugin does (the frontend logs and
		// keeps the manual-input fallback).
		if lastErr == nil {
			lastErr = errString("Fonts registry keys are not readable")
		}
		return nil, lastErr
	}
	sortSystemFonts(fonts)
	return fonts, nil
}

// resolveFontPath turns the registry value data into a usable path: HKLM
// stores bare file names (relative to %WINDIR%\Fonts), HKCU stores absolute
// paths into the user's font directory.
func resolveFontPath(data string) string {
	if data == "" || filepath.IsAbs(data) {
		return data
	}
	windir := os.Getenv("WINDIR")
	if windir == "" {
		windir = `C:\Windows`
	}
	return filepath.Join(windir, "Fonts", data)
}
