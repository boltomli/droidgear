package app

import (
	"encoding/json"
	"os"
	"path/filepath"
	"sync"
	"testing"
	"time"
)

// sessionsFixtureDir creates `<home>/.factory/sessions/<project>` and
// returns it.
func sessionsFixtureDir(t *testing.T, home, project string) string {
	t.Helper()
	dir := filepath.Join(home, ".factory", "sessions", project)
	if err := os.MkdirAll(dir, 0o755); err != nil {
		t.Fatalf("mkdir: %v", err)
	}
	return dir
}

// sessionsWriteSession drops a session pair (.jsonl + .settings.json) into
// the fixture project directory.
func sessionsWriteSession(t *testing.T, dir, id, jsonl, settings string) string {
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

const sessionsFixtureJSONL = `{"type":"session_start","id":"sess-1","sessionTitle":"Hello World","cwd":"/work/demo"}
{"type":"message","id":"m1","timestamp":"2026-01-01T00:00:01.000Z","message":{"role":"user","content":[{"type":"text","text":"Fix the parser"},{"type":"tool_use","name":"bash","input":{}}]}}
{"type":"message","id":"m2","timestamp":"2026-01-01T00:00:02.000Z","message":{"role":"assistant","content":[{"type":"thinking","thinking":"Inspect"},{"type":"text","text":"Done"}]}}
`

const sessionsFixtureSettings = `{"model":"gpt-5","tokenUsage":{"inputTokens":10,"outputTokens":2,"cacheCreationTokens":1,"cacheReadTokens":3,"thinkingTokens":4}}`

// TestListSessionsAndProjects covers the generic Factory sessions listing:
// project discovery, summary rows (title/model/tokens/path) and the
// newest-first ordering.
func TestListSessionsAndProjects(t *testing.T) {
	home := isolatedHome(t)
	a := &App{}

	// Missing sessions directory → empty results, not an error.
	projects, err := a.ListSessionProjects()
	if err != nil || len(projects) != 0 {
		t.Fatalf("empty projects: %v %+v", err, projects)
	}
	sessions, err := a.ListSessions(nil)
	if err != nil || len(sessions) != 0 {
		t.Fatalf("empty sessions: %v %+v", err, sessions)
	}

	dir := sessionsFixtureDir(t, home, "-work-demo")
	sessionsWriteSession(t, dir, "sess-1", sessionsFixtureJSONL, sessionsFixtureSettings)

	projects, err = a.ListSessionProjects()
	if err != nil {
		t.Fatalf("ListSessionProjects: %v", err)
	}
	if len(projects) != 1 {
		t.Fatalf("expected 1 project, got %d", len(projects))
	}
	if projects[0].Name != "-work-demo" || projects[0].SessionCount != 1 {
		t.Fatalf("unexpected project: %+v", projects[0])
	}
	if projects[0].Path != dir || projects[0].ModifiedAt <= 0 {
		t.Fatalf("unexpected project path/mtime: %+v", projects[0])
	}

	sessions, err = a.ListSessions(nil)
	if err != nil {
		t.Fatalf("ListSessions: %v", err)
	}
	if len(sessions) != 1 {
		t.Fatalf("expected 1 session, got %d", len(sessions))
	}
	summary := sessions[0]
	if summary.ID != "sess-1" || summary.Title != "Hello World" || summary.Project != "-work-demo" {
		t.Fatalf("unexpected summary identity: %+v", summary)
	}
	if summary.Model != "gpt-5" || summary.TokenUsage.InputTokens != 10 || summary.TokenUsage.ThinkingTokens != 4 {
		t.Fatalf("unexpected summary usage/model: %+v", summary)
	}
	if summary.ModifiedAt <= 0 {
		t.Fatalf("expected mtime > 0: %+v", summary)
	}
	// The list path drops the .jsonl extension (bindings: "without extension").
	wantPath := filepath.Join(dir, "sess-1")
	if summary.Path != wantPath {
		t.Fatalf("path = %q, want %q", summary.Path, wantPath)
	}

	// Project filter uses the directory name.
	project := "-work-demo"
	filtered, err := a.ListSessions(&project)
	if err != nil || len(filtered) != 1 {
		t.Fatalf("filtered list: %v %+v", err, filtered)
	}
	missing := "nope"
	filtered, err = a.ListSessions(&missing)
	if err != nil || len(filtered) != 0 {
		t.Fatalf("missing project list: %v %+v", err, filtered)
	}
}

// TestGetSessionDetailGeneric covers the detail parse: session header,
// tool_use/tool_result filtering and content block mapping.
func TestGetSessionDetailGeneric(t *testing.T) {
	home := isolatedHome(t)
	a := &App{}
	dir := sessionsFixtureDir(t, home, "-work-demo")
	jsonlPath := sessionsWriteSession(t, dir, "sess-1", sessionsFixtureJSONL, sessionsFixtureSettings)
	detailPath := filepath.Join(dir, "sess-1")

	detail, err := a.GetSessionDetail(detailPath)
	if err != nil {
		t.Fatalf("GetSessionDetail: %v", err)
	}
	if detail.ID != "sess-1" || detail.Title != "Hello World" || detail.Project != "-work-demo" {
		t.Fatalf("unexpected header: %+v", detail)
	}
	if detail.Cwd != "/work/demo" || detail.Model != "gpt-5" {
		t.Fatalf("unexpected cwd/model: %+v", detail)
	}
	if detail.TokenUsage.CacheReadTokens != 3 {
		t.Fatalf("unexpected usage: %+v", detail.TokenUsage)
	}
	// tool_use block dropped; two messages remain.
	if len(detail.Messages) != 2 {
		t.Fatalf("expected 2 messages, got %d: %+v", len(detail.Messages), detail.Messages)
	}
	user := detail.Messages[0]
	if user.Role != "user" || len(user.Content) != 1 || user.Content[0].Text == nil || *user.Content[0].Text != "Fix the parser" {
		t.Fatalf("unexpected user message: %+v", user)
	}
	assistant := detail.Messages[1]
	if assistant.Role != "assistant" || len(assistant.Content) != 2 {
		t.Fatalf("unexpected assistant message: %+v", assistant)
	}
	if assistant.Content[0].Type != "thinking" || assistant.Content[0].Thinking == nil {
		t.Fatalf("expected thinking block: %+v", assistant.Content[0])
	}
	if _, err := os.Stat(jsonlPath); err != nil {
		t.Fatalf("detail must not modify the file: %v", err)
	}

	// Unknown session path → byte-identical Rust error.
	if _, err := a.GetSessionDetail(filepath.Join(dir, "missing")); err == nil || err.Error() != "Session file not found" {
		t.Fatalf("expected not-found error, got %v", err)
	}
}

// TestDeleteSessionGeneric covers removing the .jsonl + .settings.json pair
// and the not-found guard.
func TestDeleteSessionGeneric(t *testing.T) {
	home := isolatedHome(t)
	a := &App{}
	dir := sessionsFixtureDir(t, home, "-work-demo")
	sessionsWriteSession(t, dir, "sess-1", sessionsFixtureJSONL, sessionsFixtureSettings)
	detailPath := filepath.Join(dir, "sess-1")

	if err := a.DeleteSession(detailPath); err != nil {
		t.Fatalf("DeleteSession: %v", err)
	}
	for _, suffix := range []string{".jsonl", ".settings.json"} {
		if _, err := os.Stat(detailPath + suffix); !os.IsNotExist(err) {
			t.Fatalf("%s should be gone (err=%v)", suffix, err)
		}
	}
	if err := a.DeleteSession(detailPath); err == nil || err.Error() != "Session file not found" {
		t.Fatalf("expected not-found error, got %v", err)
	}
}

// TestSessionsWatcherEmitsOnlyOnChange covers the poll watcher contract:
// no watcher for a missing directory, an event after a real change, and
// silence after stop.
func TestSessionsWatcherEmitsOnlyOnChange(t *testing.T) {
	home := isolatedHome(t)
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

	dir := sessionsFixtureDir(t, home, "-work-demo")
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

	sessionsWriteSession(t, dir, "sess-1", sessionsFixtureJSONL, "")

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
	sessionsWriteSession(t, dir, "sess-2", sessionsFixtureJSONL, "")
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
func TestSessionJSONContract(t *testing.T) {
	summary := SessionSummary{
		ID: "id", Title: "t", Project: "p", Model: "m", ModifiedAt: 1,
		TokenUsage: TokenUsage{InputTokens: 1}, Path: "x",
	}
	data, err := json.Marshal(summary)
	if err != nil {
		t.Fatalf("marshal: %v", err)
	}
	var decoded map[string]any
	if err := json.Unmarshal(data, &decoded); err != nil {
		t.Fatalf("unmarshal: %v", err)
	}
	for _, key := range []string{"id", "title", "project", "model", "modifiedAt", "tokenUsage", "path"} {
		if _, ok := decoded[key]; !ok {
			t.Fatalf("missing key %q in %s", key, data)
		}
	}
	usage := decoded["tokenUsage"].(map[string]any)
	for _, key := range []string{"inputTokens", "outputTokens", "cacheCreationTokens", "cacheReadTokens", "thinkingTokens"} {
		if _, ok := usage[key]; !ok {
			t.Fatalf("missing token key %q in %s", key, data)
		}
	}

	// Optional block fields are omitted when absent, present when empty.
	bare, _ := json.Marshal(ContentBlock{Type: "text"})
	if string(bare) != `{"type":"text"}` {
		t.Fatalf("bare block = %s", bare)
	}
	empty := ""
	filled, _ := json.Marshal(ContentBlock{Type: "text", Text: &empty})
	if string(filled) != `{"type":"text","text":""}` {
		t.Fatalf("filled block = %s", filled)
	}
}
