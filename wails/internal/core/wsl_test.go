package core

import "testing"

func TestParseWslListStripsUTF16AndFindsDefault(t *testing.T) {
	// Simulated `wsl -l -v` output as UTF-16LE (NUL bytes between chars).
	raw := "N\x00A\x00M\x00E\x00 \x00 \x00S\x00T\x00A\x00T\x00E\x00\r\x00\n\x00" +
		"*\x00 \x00U\x00b\x00u\x00n\x00t\x00u\x00 \x00 \x00R\x00u\x00n\x00n\x00i\x00n\x00g\x00 \x00 \x002\x00\r\x00\n\x00" +
		" \x00D\x00e\x00b\x00i\x00a\x00n\x00 \x00 \x00S\x00t\x00o\x00p\x00p\x00e\x00d\x00 \x00 \x002\x00\r\x00\n\x00"

	distros := parseWslList(raw)
	if len(distros) != 2 {
		t.Fatalf("expected 2 distros, got %d: %+v", len(distros), distros)
	}
	if distros[0].Name != "Ubuntu" || !distros[0].IsDefault || distros[0].State != "Running" || distros[0].Version != 2 {
		t.Errorf("ubuntu parse wrong: %+v", distros[0])
	}
	if distros[1].Name != "Debian" || distros[1].IsDefault || distros[1].State != "Stopped" {
		t.Errorf("debian parse wrong: %+v", distros[1])
	}
}

func TestParseWslListSupportsSpacesInName(t *testing.T) {
	raw := "NAME  STATE  VERSION\n* Ubuntu-22.04 LTS  Running  2\n"
	distros := parseWslList(raw)
	if len(distros) != 1 || distros[0].Name != "Ubuntu-22.04 LTS" {
		t.Fatalf("spaced name parse wrong: %+v", distros)
	}
}
