package app

import (
	"strings"
	"testing"
)

// The dropdown only consumes Name, so the name-mangling helpers carry the
// whole contract — pin them against real registry value names and font
// file stems observed on Windows/macOS.

func TestFontEntryFamily(t *testing.T) {
	cases := map[string]string{
		// Windows registry value names (HKLM Fonts)
		"Arial (TrueType)":                  "Arial",
		"Arial Bold Italic (TrueType)":      "Arial",
		"Segoe UI (OpenType)":               "Segoe UI",
		"Cambria & Cambria Math (TrueType)": "Cambria & Cambria Math",
		"Bahnschrift (TrueType)":            "Bahnschrift",
		"Times New Roman Bold (TrueType)":   "Times New Roman",
		// Weight words that form standalone families must survive.
		"Calibri Light (TrueType)":     "Calibri Light",
		"Arial Black (TrueType)":       "Arial Black",
		"Segoe UI Semibold (TrueType)": "Segoe UI Semibold",
		// Per-user (HKCU) entries: file stems, hyphen/underscore separated.
		"NotoSans-Regular":                    "NotoSans",
		"SauceCodeProNerdFontMono-BoldItalic": "SauceCodeProNerdFontMono",
		"zh-hans (TrueType)":                  "zh-hans",
		// macOS font file stems.
		"PingFang.ttc":       "PingFang",
		"HelveticaNeue-Bold": "HelveticaNeue",
		// Non-font suffix must NOT be stripped (not a known file type).
		"My Custom Font (Web)": "My Custom Font (Web)",
		// Degenerate inputs.
		"":           "",
		"Bold":       "Bold", // nothing left to peel
		"(TrueType)": "",
	}
	for input, want := range cases {
		if got := fontEntryFamily(input); got != want {
			t.Errorf("fontEntryFamily(%q) = %q, want %q", input, got, want)
		}
	}
}

func TestLooksMonospaced(t *testing.T) {
	yes := []string{"Consolas", "DejaVu Sans Mono", "JetBrains Mono", "Liberation Mono", "Menlo", "Cascadia Code"}
	no := []string{"Arial", "Calibri", "Segoe UI", "Comic Sans MS"}
	for _, name := range yes {
		if !looksMonospaced(name) {
			t.Errorf("looksMonospaced(%q) = false, want true", name)
		}
	}
	for _, name := range no {
		if looksMonospaced(name) {
			t.Errorf("looksMonospaced(%q) = true, want false", name)
		}
	}
}

func TestBuildSystemFontsDedupesAndSorts(t *testing.T) {
	fonts := buildSystemFonts([]string{
		" Segoe UI ", "", "Arial", "segoe ui", "Calibri",
	})
	if len(fonts) != 3 {
		t.Fatalf("len = %d, want 3 (empty + case-duplicate removed)", len(fonts))
	}
	got := []string{fonts[0].Name, fonts[1].Name, fonts[2].Name}
	want := []string{"Arial", "Calibri", "Segoe UI"}
	for i := range want {
		if got[i] != want[i] {
			t.Fatalf("order = %v, want %v", got, want)
		}
	}
	if fonts[0].Weight != 400 || fonts[0].Style != "normal" {
		t.Errorf("defaults missing: %+v", fonts[0])
	}
	if fonts[0].ID == "" || fonts[0].Name != fonts[0].FontName {
		t.Errorf("id/fontName inconsistent: %+v", fonts[0])
	}
}

// TestGetSystemFontsSmoke exercises the real platform backend (registry on
// Windows, fc-list/dirs elsewhere). It only asserts the contract the shim
// relies on: no error, a non-empty deduped list, sorted names.
func TestGetSystemFontsSmoke(t *testing.T) {
	fonts, err := (&App{}).GetSystemFonts()
	if err != nil {
		t.Fatalf("GetSystemFonts: %v", err)
	}
	if len(fonts) == 0 {
		t.Fatal("GetSystemFonts returned no fonts")
	}
	seen := make(map[string]bool, len(fonts))
	for i, font := range fonts {
		if font.Name == "" {
			t.Fatalf("fonts[%d] has empty Name: %+v", i, font)
		}
		if seen[font.Name] {
			t.Fatalf("duplicate family %q", font.Name)
		}
		seen[font.Name] = true
		if i > 0 && strings.ToLower(fonts[i-1].Name) > strings.ToLower(font.Name) {
			t.Fatalf("not sorted: %q before %q", fonts[i-1].Name, font.Name)
		}
	}
}
