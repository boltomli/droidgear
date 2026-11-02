// System font enumeration for the Appearance pane's terminal-font dropdown.
// Mirrors the Tauri `get_system_fonts` command from tauri-plugin-system-fonts
// (frontend contract: tauri-plugin-system-fonts-api →
// src/wails/shims/tauri-system-fonts.ts → this method).
//
// Platform enumeration lives in fonts_windows.go (registry, //go:build
// windows) and fonts_other.go (fc-list / font-directory scan); the shared
// name-mangling helpers here are pure and unit-tested.
package app

import (
	"path/filepath"
	"sort"
	"strings"
)

// fontFileExtensions are the font file extensions recognised when deriving
// family names from file stems (macOS scan, user-font registry values).
var fontFileExtensions = map[string]bool{
	".ttf": true, ".otf": true, ".ttc": true, ".dfont": true, ".otc": true,
}

// SystemFont mirrors the SystemFont interface exported by
// tauri-plugin-system-fonts-api. The frontend only reads Name; Weight/Style
// are not derivable from a registry listing, so the platform backends fill
// the normal/400 defaults and Monospaced comes from a name heuristic —
// documented divergence from the Tauri side (fontdb parses the real name
// tables there).
type SystemFont struct {
	ID         string `json:"id"`
	Name       string `json:"name"`
	FontName   string `json:"fontName"`
	Path       string `json:"path"`
	Weight     int    `json:"weight"`
	Style      string `json:"style"`
	Monospaced bool   `json:"monospaced"`
}

// GetSystemFonts returns the fonts installed on this machine (mirrors the
// Tauri get_system_fonts command).
func (a *App) GetSystemFonts() ([]SystemFont, error) {
	return enumerateSystemFonts()
}

// styleSuffixes are the parenthetical file-type markers Windows appends to
// Fonts registry value names ("Arial (TrueType)", "Segoe UI (OpenType)").
var styleSuffixes = map[string]bool{
	"truetype": true,
	"opentype": true,
	"ttf":      true,
	"otf":      true,
	"cff":      true,
}

// subfamilyTokens are trailing name tokens that are subfamily (name ID 2)
// markers rather than part of a family name: "Arial Bold" → "Arial".
// Deliberately conservative — weight/width words that DO form standalone
// families (Light, Black, Narrow, Semibold, Condensed …) are kept, so
// "Calibri Light" and "Arial Black" survive as their own entries.
var subfamilyTokens = map[string]bool{
	"regular":      true,
	"bold":         true,
	"italic":       true,
	"oblique":      true,
	"bolditalic":   true,
	"boldoblique":  true,
	"bold italic":  true,
	"bold oblique": true,
}

// fontEntryFamily approximates a font family name from a Windows Fonts
// registry value name ("Arial Bold Italic (TrueType)" → "Arial"). It also
// accepts macOS/Linux font file stems ("NotoSans-Regular" → "NotoSans").
//
// Divergence from Tauri: fontdb reads the real name table, so patched fonts
// whose family name differs from the file name (e.g. "Noto Sans" vs
// "NotoSans-Regular") can differ between the two shells. The dropdown only
// needs a unique, human-readable list, so this is accepted.
func fontEntryFamily(name string) string {
	name = strings.TrimSpace(name)
	// Drop a known font-file extension so both registry value names and
	// file stems feed the same path ("PingFang.ttc" → "PingFang").
	if ext := strings.ToLower(filepath.Ext(name)); fontFileExtensions[ext] {
		name = strings.TrimSpace(strings.TrimSuffix(name, filepath.Ext(name)))
	}
	if i := strings.LastIndex(name, "("); i >= 0 && strings.HasSuffix(name, ")") {
		if inner := strings.ToLower(strings.TrimSpace(name[i+1 : len(name)-1])); styleSuffixes[inner] {
			name = strings.TrimSpace(name[:i])
		}
	}
	// Peel trailing subfamily tokens ("Arial Bold Italic" → "Arial",
	// "SauceCodeProNerdFontMono-BoldItalic" → "SauceCodeProNerdFontMono").
	for {
		sep := strings.LastIndexAny(name, " -_")
		if sep <= 0 {
			break
		}
		token := name[sep+1:]
		if !subfamilyTokens[strings.ToLower(strings.ReplaceAll(token, "_", " "))] &&
			!subfamilyTokens[strings.ToLower(strings.ReplaceAll(token, "-", " "))] &&
			!subfamilyTokens[strings.ToLower(token)] {
			break
		}
		name = strings.TrimSpace(name[:sep])
	}
	return name
}

// looksMonospaced is a name heuristic for the fields the registry cannot
// provide (fontdb computes this from the font's post table on Tauri).
func looksMonospaced(name string) bool {
	lower := strings.ToLower(name)
	for _, hint := range []string{
		"mono", "consolas", "courier", "cascadia", "menlo",
		"lucida console", "fixedsys",
	} {
		if strings.Contains(lower, hint) {
			return true
		}
	}
	return false
}

// newSystemFont wraps a family name into the plugin's SystemFont shape.
func newSystemFont(family, path string) SystemFont {
	id := path
	if id == "" {
		id = family
	}
	return SystemFont{
		ID:         id,
		Name:       family,
		FontName:   family,
		Path:       path,
		Weight:     400,
		Style:      "normal",
		Monospaced: looksMonospaced(family),
	}
}

// buildSystemFonts dedupes, sorts and wraps plain family names (used by the
// fc-list / font-directory backends; the registry backend carries paths and
// builds its entries directly).
func buildSystemFonts(families []string) []SystemFont {
	seen := make(map[string]bool, len(families))
	fonts := make([]SystemFont, 0, len(families))
	for _, family := range families {
		family = strings.TrimSpace(family)
		if family == "" || seen[strings.ToLower(family)] {
			continue
		}
		seen[strings.ToLower(family)] = true
		fonts = append(fonts, newSystemFont(family, ""))
	}
	sortSystemFonts(fonts)
	return fonts
}

// sortSystemFonts orders by lowercase family name for stable output (the
// frontend re-sorts with localeCompare, this is for determinism/tests).
func sortSystemFonts(fonts []SystemFont) {
	sort.Slice(fonts, func(i, j int) bool {
		return strings.ToLower(fonts[i].Name) < strings.ToLower(fonts[j].Name)
	})
}
