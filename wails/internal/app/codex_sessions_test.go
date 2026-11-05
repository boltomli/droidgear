package app

import (
	"droidgear-wails/internal/testutil"
	"os"
	"strings"
	"sync"
	"testing"
	"time"
)

// Watcher
// ---------------------------------------------------------------------------

// TestCodexSessionsWatcherEmitsOnChange covers the poll-based mirror of the
// Rust notify watcher: start → change → `codex-sessions-changed`; stop ends
// the stream; the baseline state emits nothing.
func TestCodexSessionsWatcherEmitsOnChange(t *testing.T) {
	isolatedHome(t)
	a := &App{}

	// Missing sessions dir → start is a no-op (Rust parity), stop is safe.
	if err := a.StartCodexSessionsWatcher(); err != nil {
		t.Fatalf("start (no dir): %v", err)
	}
	if err := a.StopCodexSessionsWatcher(); err != nil {
		t.Fatalf("stop (no dir): %v", err)
	}

	path := testutil.CodexSessFixture(t, testutil.CodexSessFull)

	var mu sync.Mutex
	events := []string{}
	codexEventSink = func(name string) {
		mu.Lock()
		defer mu.Unlock()
		events = append(events, name)
	}
	defer func() { codexEventSink = nil }()

	if err := a.StartCodexSessionsWatcher(); err != nil {
		t.Fatalf("start: %v", err)
	}
	// Let the baseline signature settle; the pre-existing state must not emit.
	time.Sleep(1200 * time.Millisecond)
	mu.Lock()
	if len(events) != 0 {
		mu.Unlock()
		t.Fatalf("baseline must not emit: %+v", events)
	}
	mu.Unlock()

	// A modification triggers exactly the change event.
	if err := os.WriteFile(path, []byte(strings.Join(append(testutil.CodexSessFull, testutil.CodexSessTokens), "\n")+"\n"), 0o644); err != nil {
		t.Fatalf("modify: %v", err)
	}
	deadline := time.After(5 * time.Second)
	for {
		mu.Lock()
		got := len(events) > 0
		mu.Unlock()
		if got {
			break
		}
		select {
		case <-deadline:
			t.Fatal("no codex-sessions-changed event in time")
		case <-time.After(50 * time.Millisecond):
		}
	}
	mu.Lock()
	first := events[0]
	mu.Unlock()
	if first != "codex-sessions-changed" {
		t.Fatalf("event name: %q", first)
	}

	if err := a.StopCodexSessionsWatcher(); err != nil {
		t.Fatalf("stop: %v", err)
	}
	mu.Lock()
	before := len(events)
	mu.Unlock()

	// After stop, further changes emit nothing.
	time.Sleep(300 * time.Millisecond)
	if err := os.Remove(path); err != nil {
		t.Fatalf("remove: %v", err)
	}
	time.Sleep(1200 * time.Millisecond)
	mu.Lock()
	defer mu.Unlock()
	if len(events) != before {
		t.Fatalf("events after stop: %+v", events)
	}
}

// TestCodexSessionsWatcherRestartReplacesLoop: starting twice must not stack
// poll loops (the Rust command replaces the watcher).
func TestCodexSessionsWatcherRestartReplacesLoop(t *testing.T) {
	isolatedHome(t)
	a := &App{}

	path := testutil.CodexSessFixture(t, testutil.CodexSessFull)

	var mu sync.Mutex
	count := 0
	codexEventSink = func(name string) {
		mu.Lock()
		defer mu.Unlock()
		count++
	}
	defer func() { codexEventSink = nil }()

	if err := a.StartCodexSessionsWatcher(); err != nil {
		t.Fatalf("start 1: %v", err)
	}
	if err := a.StartCodexSessionsWatcher(); err != nil {
		t.Fatalf("start 2: %v", err)
	}
	time.Sleep(1200 * time.Millisecond)

	if err := os.WriteFile(path, []byte(testutil.CodexSessMETA+"\n"), 0o644); err != nil {
		t.Fatalf("modify: %v", err)
	}
	deadline := time.After(5 * time.Second)
	for {
		mu.Lock()
		got := count
		mu.Unlock()
		if got > 0 {
			break
		}
		select {
		case <-deadline:
			t.Fatal("no event after restart")
		case <-time.After(50 * time.Millisecond):
		}
	}
	if err := a.StopCodexSessionsWatcher(); err != nil {
		t.Fatalf("stop: %v", err)
	}

	// Exactly one loop may survive the restart: stop must silence everything.
	mu.Lock()
	before := count
	mu.Unlock()
	time.Sleep(300 * time.Millisecond)
	if err := os.WriteFile(path, []byte(testutil.CodexSessMETA+"\n"+testutil.CodexSessTURN+"\n"), 0o644); err != nil {
		t.Fatalf("modify 2: %v", err)
	}
	time.Sleep(1200 * time.Millisecond)
	mu.Lock()
	defer mu.Unlock()
	if count != before {
		t.Fatalf("duplicate watcher still running: %d -> %d", before, count)
	}
}
