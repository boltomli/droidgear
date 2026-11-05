package app

import (
	"os"
	"path/filepath"
	"sync"
	"testing"
	"time"

	"droidgear-wails/internal/testutil"
)

// TestSpecsWatcherEmitsOnlyOnChange covers the poll loop behind
// start/stop_specs_watcher: quiet while idle, one `specs-changed` per actual
// change, silent again after stop.
func TestSpecsWatcherEmitsOnlyOnChange(t *testing.T) {
	home := testutil.IsolatedHome(t)
	a := &App{}

	var mu sync.Mutex
	events := 0
	specsEventSink = func(name string, payload map[string]any) {
		if name != "specs-changed" {
			t.Errorf("unexpected event %q", name)
		}
		mu.Lock()
		events++
		mu.Unlock()
	}
	defer func() { specsEventSink = nil }()

	if err := a.StartSpecsWatcher(); err != nil {
		t.Fatalf("StartSpecsWatcher: %v", err)
	}
	defer func() { _ = a.StopSpecsWatcher() }()

	count := func() int {
		mu.Lock()
		defer mu.Unlock()
		return events
	}

	// Baseline is captured before the loop starts: no event while idle.
	time.Sleep(700 * time.Millisecond)
	if count() != 0 {
		t.Fatalf("expected no idle events, got %d", count())
	}

	specsDir := filepath.Join(home, ".factory", "specs")
	if err := os.WriteFile(filepath.Join(specsDir, "watched.md"), []byte("x"), 0o644); err != nil {
		t.Fatalf("write spec: %v", err)
	}
	deadline := time.Now().Add(3 * time.Second)
	for count() == 0 && time.Now().Before(deadline) {
		time.Sleep(50 * time.Millisecond)
	}
	if count() == 0 {
		t.Fatal("expected specs-changed after creating a spec")
	}

	if err := a.StopSpecsWatcher(); err != nil {
		t.Fatalf("StopSpecsWatcher: %v", err)
	}
	before := count()
	if err := os.WriteFile(filepath.Join(specsDir, "after-stop.md"), []byte("y"), 0o644); err != nil {
		t.Fatalf("write spec: %v", err)
	}
	time.Sleep(800 * time.Millisecond)
	if count() != before {
		t.Fatalf("watcher still emitting after stop: %d → %d", before, count())
	}

	// Stopping twice is a no-op (Rust command behaves the same).
	if err := a.StopSpecsWatcher(); err != nil {
		t.Fatalf("second StopSpecsWatcher: %v", err)
	}
}

func TestSpecsWatcherStartCreatesDirectory(t *testing.T) {
	home := testutil.IsolatedHome(t)
	a := &App{}
	defer func() { _ = a.StopSpecsWatcher() }()

	if err := a.StartSpecsWatcher(); err != nil {
		t.Fatalf("StartSpecsWatcher: %v", err)
	}
	if info, err := os.Stat(filepath.Join(home, ".factory", "specs")); err != nil || !info.IsDir() {
		t.Fatalf("specs directory not created: %v", err)
	}
	if err := a.StopSpecsWatcher(); err != nil {
		t.Fatalf("StopSpecsWatcher: %v", err)
	}
}
