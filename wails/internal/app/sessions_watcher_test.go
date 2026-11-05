package app

import (
	"sync"
	"testing"
	"time"

	"droidgear-wails/internal/testutil"
)

// TestSessionsWatcherEmitsOnlyOnChange covers the poll watcher contract:
// no watcher for a missing directory, an event after a real change, and
// silence after stop.
func TestSessionsWatcherEmitsOnlyOnChange(t *testing.T) {
	home := testutil.IsolatedHome(t)
	a := &App{}

	var mu sync.Mutex
	events := []string{}
	sessionToolEventSink = func(event string) {
		mu.Lock()
		defer mu.Unlock()
		events = append(events, event)
	}
	defer func() { sessionToolEventSink = nil }()

	// Missing directory: start is a no-op (Rust returns Ok without watching).
	if err := a.StartSessionsWatcher(); err != nil {
		t.Fatalf("start (missing dir): %v", err)
	}

	dir := testutil.SessionsFixtureDir(t, home, "-work-demo")
	if err := a.StartSessionsWatcher(); err != nil {
		t.Fatalf("start: %v", err)
	}
	// Restart with the same directory keeps a single watcher.
	if err := a.StartSessionsWatcher(); err != nil {
		t.Fatalf("restart: %v", err)
	}

	// Baseline poll passes with no change → no event yet.
	time.Sleep(700 * time.Millisecond)
	mu.Lock()
	if len(events) != 0 {
		t.Fatalf("no change should not emit: %v", events)
	}
	mu.Unlock()

	testutil.SessionsWriteSession(t, dir, "sess-1", testutil.SessionsFixtureJSONL, "")

	deadline := time.After(5 * time.Second)
	for {
		mu.Lock()
		count := len(events)
		mu.Unlock()
		if count > 0 {
			break
		}
		select {
		case <-deadline:
			t.Fatal("expected sessions-changed event after a file write")
		case <-time.After(100 * time.Millisecond):
		}
	}
	mu.Lock()
	if events[0] != "sessions-changed" {
		t.Fatalf("unexpected event: %v", events)
	}
	mu.Unlock()

	if err := a.StopSessionsWatcher(); err != nil {
		t.Fatalf("stop: %v", err)
	}
	// Double stop is a no-op.
	if err := a.StopSessionsWatcher(); err != nil {
		t.Fatalf("second stop: %v", err)
	}
	before := func() int {
		mu.Lock()
		defer mu.Unlock()
		return len(events)
	}()
	testutil.SessionsWriteSession(t, dir, "sess-2", testutil.SessionsFixtureJSONL, "")
	time.Sleep(1200 * time.Millisecond)
	after := func() int {
		mu.Lock()
		defer mu.Unlock()
		return len(events)
	}()
	if after != before {
		t.Fatalf("stopped watcher emitted: %d -> %d", before, after)
	}
}

// TestSessionJSONContract pins the bindings.ts field names: camelCase keys,
// `type` for content blocks and omitted optional block fields.
