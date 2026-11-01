package app

import (
	"encoding/base64"
	"strings"
	"sync"
	"testing"
	"time"
)

// TestPtyDataEvents verifies the streaming contract the JS shim relies on:
// `pty:data` carries base64 chunks keyed by session id, and `pty:exit`
// carries the exit code exactly once.
func TestPtyDataEvents(t *testing.T) {
	var mu sync.Mutex
	var data strings.Builder
	var exits []map[string]any
	ptyEventSink = func(name string, payload map[string]any) {
		mu.Lock()
		defer mu.Unlock()
		switch name {
		case "pty:data":
			chunk, err := base64.StdEncoding.DecodeString(payload["data"].(string))
			if err == nil {
				data.Write(chunk)
			}
		case "pty:exit":
			exits = append(exits, payload)
		}
	}
	defer func() { ptyEventSink = nil }()

	file, args := testShellCommand()
	id, err := ptys.Start(PtyStartOptions{File: file, Args: args})
	if err != nil {
		t.Fatalf("Start failed: %v", err)
	}

	deadline := time.After(15 * time.Second)
	var output string
	for {
		mu.Lock()
		done := len(exits) > 0
		output = data.String()
		mu.Unlock()
		if done {
			break
		}
		select {
		case <-deadline:
			t.Fatal("no pty:exit event in time")
		case <-time.After(50 * time.Millisecond):
		}
	}

	if !strings.Contains(output, "hello-pty") {
		t.Fatalf("expected streamed output in pty:data events, got %q", output)
	}
	mu.Lock()
	defer mu.Unlock()
	if len(exits) != 1 {
		t.Fatalf("expected exactly one pty:exit event, got %d", len(exits))
	}
	if exits[0]["id"] != id {
		t.Fatalf("pty:exit id mismatch: got %v want %d", exits[0]["id"], id)
	}
	if code, _ := exits[0]["exitCode"].(int32); code != 0 {
		t.Fatalf("expected exit code 0, got %v", exits[0]["exitCode"])
	}
}
