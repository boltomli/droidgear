package read

import (
	"droidgear-wails/internal/testutil"
	"encoding/json"
	"os"
	"path/filepath"
	"testing"
)

// TestListSessionsAndProjects covers the generic Factory sessions listing:
// project discovery, summary rows (title/model/tokens/path) and the
// newest-first ordering.
func TestListSessionsAndProjects(t *testing.T) {
	home := testutil.IsolatedHome(t)

	// Missing sessions directory → empty results, not an error.
	projects, err := ListSessionProjects()
	if err != nil || len(projects) != 0 {
		t.Fatalf("empty projects: %v %+v", err, projects)
	}
	sessions, err := ListSessions(nil)
	if err != nil || len(sessions) != 0 {
		t.Fatalf("empty sessions: %v %+v", err, sessions)
	}

	dir := testutil.SessionsFixtureDir(t, home, "-work-demo")
	testutil.SessionsWriteSession(t, dir, "sess-1", testutil.SessionsFixtureJSONL, testutil.SessionsFixtureSettings)

	projects, err = ListSessionProjects()
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

	sessions, err = ListSessions(nil)
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
	filtered, err := ListSessions(&project)
	if err != nil || len(filtered) != 1 {
		t.Fatalf("filtered list: %v %+v", err, filtered)
	}
	missing := "nope"
	filtered, err = ListSessions(&missing)
	if err != nil || len(filtered) != 0 {
		t.Fatalf("missing project list: %v %+v", err, filtered)
	}
}

// TestGetSessionDetailGeneric covers the detail parse: session header,
// tool_use/tool_result filtering and content block mapping.
func TestGetSessionDetailGeneric(t *testing.T) {
	home := testutil.IsolatedHome(t)
	dir := testutil.SessionsFixtureDir(t, home, "-work-demo")
	jsonlPath := testutil.SessionsWriteSession(t, dir, "sess-1", testutil.SessionsFixtureJSONL, testutil.SessionsFixtureSettings)
	detailPath := filepath.Join(dir, "sess-1")

	detail, err := GetSessionDetail(detailPath)
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
	if _, err := GetSessionDetail(filepath.Join(dir, "missing")); err == nil || err.Error() != "Session file not found" {
		t.Fatalf("expected not-found error, got %v", err)
	}
}

// TestDeleteSessionGeneric covers removing the .jsonl + .settings.json pair
// and the not-found guard.
func TestDeleteSessionGeneric(t *testing.T) {
	home := testutil.IsolatedHome(t)
	dir := testutil.SessionsFixtureDir(t, home, "-work-demo")
	testutil.SessionsWriteSession(t, dir, "sess-1", testutil.SessionsFixtureJSONL, testutil.SessionsFixtureSettings)
	detailPath := filepath.Join(dir, "sess-1")

	if err := DeleteSession(detailPath); err != nil {
		t.Fatalf("DeleteSession: %v", err)
	}
	for _, suffix := range []string{".jsonl", ".settings.json"} {
		if _, err := os.Stat(detailPath + suffix); !os.IsNotExist(err) {
			t.Fatalf("%s should be gone (err=%v)", suffix, err)
		}
	}
	if err := DeleteSession(detailPath); err == nil || err.Error() != "Session file not found" {
		t.Fatalf("expected not-found error, got %v", err)
	}
}

// TestDeleteSessionRejectsPathsOutsideSessionsDir: the Droid deletion must stay
// inside <factory home>/sessions, so a path outside it — including one reached
// through `..` — cannot delete anything.
func TestDeleteSessionRejectsPathsOutsideSessionsDir(t *testing.T) {
	home := testutil.IsolatedHome(t)
	dir := testutil.SessionsFixtureDir(t, home, "-work-demo")
	testutil.SessionsWriteSession(t, dir, "sess-1", testutil.SessionsFixtureJSONL, testutil.SessionsFixtureSettings)

	outside := filepath.Join(home, "notes")
	if err := os.WriteFile(outside+".jsonl", []byte("{}"), 0o600); err != nil {
		t.Fatalf("write outside session: %v", err)
	}
	traversal := filepath.Join(home, "sessions", "..", "..", "notes")
	for _, path := range []string{outside, traversal} {
		if err := DeleteSession(path); err == nil || err.Error() != "Session file not found" {
			t.Fatalf("expected not-found for %q, got %v", path, err)
		}
	}
	if _, err := os.Stat(outside + ".jsonl"); err != nil {
		t.Fatalf("file outside the sessions dir was deleted: %v", err)
	}
}

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
