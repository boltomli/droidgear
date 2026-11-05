package testutil

import (
	"os"
	"path/filepath"
	"testing"
)

// Sessions fixtures shared by internal/app's watcher tests and
// internal/service/read's read tests (test files cannot cross package
// borders, and testutil imports no domain package).

// SessionsFixtureDir creates `<home>/.factory/sessions/<project>` and
// returns it.
func SessionsFixtureDir(t *testing.T, home, project string) string {
	t.Helper()
	dir := filepath.Join(home, ".factory", "sessions", project)
	if err := os.MkdirAll(dir, 0o755); err != nil {
		t.Fatalf("mkdir: %v", err)
	}
	return dir
}

// SessionsWriteSession drops a session pair (.jsonl + .settings.json) into
// the fixture project directory.
func SessionsWriteSession(t *testing.T, dir, id, jsonl, settings string) string {
	t.Helper()
	jsonlPath := filepath.Join(dir, id+".jsonl")
	if err := os.WriteFile(jsonlPath, []byte(jsonl), 0o644); err != nil {
		t.Fatalf("write session: %v", err)
	}
	if settings != "" {
		if err := os.WriteFile(filepath.Join(dir, id+".settings.json"), []byte(settings), 0o644); err != nil {
			t.Fatalf("write settings: %v", err)
		}
	}
	return jsonlPath
}

// SessionsFixtureJSONL is the canned transcript used by both sides.
const SessionsFixtureJSONL = `{"type":"session_start","id":"sess-1","sessionTitle":"Hello World","cwd":"/work/demo"}
{"type":"message","id":"m1","timestamp":"2026-01-01T00:00:01.000Z","message":{"role":"user","content":[{"type":"text","text":"Fix the parser"},{"type":"tool_use","name":"bash","input":{}}]}}
{"type":"message","id":"m2","timestamp":"2026-01-01T00:00:02.000Z","message":{"role":"assistant","content":[{"type":"thinking","thinking":"Inspect"},{"type":"text","text":"Done"}]}}
`

// SessionsFixtureSettings is the canned session settings sidecar.
const SessionsFixtureSettings = `{"model":"gpt-5","tokenUsage":{"inputTokens":10,"outputTokens":2,"cacheCreationTokens":1,"cacheReadTokens":3,"thinkingTokens":4}}`
