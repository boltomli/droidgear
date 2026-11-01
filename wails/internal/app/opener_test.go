package app

import (
	"testing"
)

func TestOpenerCommand(t *testing.T) {
	tests := []struct {
		goos   string
		target string
		name   string
		args   []string
	}{
		{"windows", "https://example.com", "rundll32", []string{"url.dll,FileProtocolHandler", "https://example.com"}},
		{"windows", `C:\Temp\file.md`, "rundll32", []string{"url.dll,FileProtocolHandler", `C:\Temp\file.md`}},
		{"darwin", "https://example.com", "open", []string{"https://example.com"}},
		{"linux", "https://example.com", "xdg-open", []string{"https://example.com"}},
	}
	for _, tt := range tests {
		name, args := openerCommand(tt.goos, tt.target)
		if name != tt.name || len(args) != len(tt.args) {
			t.Fatalf("openerCommand(%s, %q) = %q %v, want %q %v", tt.goos, tt.target, name, args, tt.name, tt.args)
		}
		for i := range args {
			if args[i] != tt.args[i] {
				t.Fatalf("openerCommand(%s, %q) arg[%d] = %q, want %q", tt.goos, tt.target, i, args[i], tt.args[i])
			}
		}
	}
}

func TestRevealCommand(t *testing.T) {
	name, args := revealCommand("windows", `C:\Temp\notes\file.md`)
	if name != "explorer" || len(args) != 1 || args[0] != `/select,C:\Temp\notes\file.md` {
		t.Fatalf("windows reveal = %q %v", name, args)
	}

	name, args = revealCommand("darwin", "/Users/me/notes/file.md")
	if name != "open" || len(args) != 2 || args[0] != "-R" || args[1] != "/Users/me/notes/file.md" {
		t.Fatalf("darwin reveal = %q %v", name, args)
	}

	name, args = revealCommand("linux", "/home/me/notes/file.md")
	if name != "xdg-open" || len(args) != 1 || args[0] != "/home/me/notes" {
		t.Fatalf("linux reveal = %q %v", name, args)
	}
}

func TestOpenerRejectsEmptyTargets(t *testing.T) {
	a := &App{}
	if err := a.OpenUrl("   "); err == nil {
		t.Fatal("OpenUrl(whitespace) should fail")
	}
	if err := a.OpenPath(""); err == nil {
		t.Fatal("OpenPath(empty) should fail")
	}
	if err := a.RevealItemInDir(""); err == nil {
		t.Fatal("RevealItemInDir(empty) should fail")
	}
}
