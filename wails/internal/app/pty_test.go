package app

import (
	"runtime"
	"strings"
	"testing"
	"time"
)

// testShellCommand returns the platform shell for PTY round-trip tests.
func testShellCommand() (string, []string) {
	if runtime.GOOS == "windows" {
		return "cmd.exe", []string{"/c", "echo hello-pty"}
	}
	return "/bin/sh", []string{"-c", "printf 'hello-pty'"}
}

func TestPtyStartAndExit(t *testing.T) {
	file, args := testShellCommand()
	id, err := ptys.Start(PtyStartOptions{File: file, Args: args})
	if err != nil {
		t.Fatalf("Start failed: %v", err)
	}
	t.Cleanup(func() { _ = ptys.Kill(id) })

	session := ptys.get(id)
	if session == nil {
		t.Fatal("session not registered")
	}

	select {
	case <-session.done:
	case <-time.After(15 * time.Second):
		t.Fatal("session did not exit in time")
	}

	if code := session.exitCode.Load(); code != 0 {
		t.Fatalf("expected exit code 0, got %d", code)
	}
}

func TestPtyWriteAndResize(t *testing.T) {
	file := "/bin/sh"
	args := []string{"-i"}
	if runtime.GOOS == "windows" {
		file = "cmd.exe"
		args = nil
	}
	id, err := ptys.Start(PtyStartOptions{File: file, Args: args})
	if err != nil {
		t.Fatalf("Start failed: %v", err)
	}
	t.Cleanup(func() { _ = ptys.Kill(id) })

	if err := ptys.Resize(id, 120, 40); err != nil {
		t.Fatalf("Resize failed: %v", err)
	}
	if err := ptys.Write(id, "exit\r\n"); err != nil {
		t.Fatalf("Write failed: %v", err)
	}

	session := ptys.get(id)
	select {
	case <-session.done:
	case <-time.After(15 * time.Second):
		t.Fatal("shell did not exit after write")
	}
}

func TestPtyUnknownSession(t *testing.T) {
	if err := ptys.Write(99999, "x"); err == nil {
		t.Fatal("expected error writing to unknown session")
	} else if !strings.Contains(err.Error(), "EOF") {
		t.Fatalf("expected EOF-style error, got %v", err)
	}
	if err := ptys.Resize(99999, 80, 24); err == nil {
		t.Fatal("expected error resizing unknown session")
	}
	// Killing an unknown session is a no-op (mirrors tauri-plugin-pty).
	if err := ptys.Kill(99999); err != nil {
		t.Fatalf("Kill of unknown session should be a no-op, got %v", err)
	}
}
