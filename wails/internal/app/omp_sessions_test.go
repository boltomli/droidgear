package app

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

// ompWriteSession drops the shared OMP fixture (title slot → session
// header → model_change → messages → title_change → branch) into
// `<home>/.omp/agent/sessions/project/session.jsonl`.
func ompWriteSession(t *testing.T, home string) string {
	t.Helper()
	dir := filepath.Join(home, ".omp", "agent", "sessions", "project")
	if err := os.MkdirAll(dir, 0o755); err != nil {
		t.Fatalf("mkdir: %v", err)
	}
	path := filepath.Join(dir, "session.jsonl")
	content := `{"type":"title","v":1,"title":"Rename the parser","source":"auto","updatedAt":"2026-01-01T00:00:09.000Z","pad":"   "}
{"type":"session","version":3,"id":"session-1","timestamp":"2026-01-01T00:00:00.000Z","cwd":"/work/demo"}
{"type":"model_change","id":"model-1","parentId":null,"timestamp":"2026-01-01T00:00:00.100Z","model":"openai/gpt-5"}
{"type":"message","id":"user-1","parentId":"model-1","timestamp":"2026-01-01T00:00:01.000Z","message":{"role":"user","content":[{"type":"text","text":"Investigate the parser"}],"timestamp":1767225601000}}
{"type":"message","id":"assistant-1","parentId":"user-1","timestamp":"2026-01-01T00:00:02.000Z","message":{"role":"assistant","provider":"openai","model":"gpt-5","content":[{"type":"thinking","thinking":"Read the file"},{"type":"toolCall","name":"bash","intent":"Checking the parser"},{"type":"text","text":"Done"}],"usage":{"input":10,"output":5,"cacheRead":2,"cacheWrite":1,"totalTokens":18,"cost":{"total":0.03}},"timestamp":1767225602000}}
{"type":"message","id":"result-1","parentId":"assistant-1","timestamp":"2026-01-01T00:00:03.000Z","message":{"role":"toolResult","content":[{"type":"text","text":"ok"}]}}
{"type":"title_change","id":"name-1","parentId":"result-1","timestamp":"2026-01-01T00:00:04.000Z","title":"Old title","source":"auto"}
{"type":"message","id":"branch-1","parentId":"user-1","timestamp":"2026-01-01T00:00:05.000Z","message":{"role":"assistant","provider":"openai","model":"gpt-5","content":"Alternate answer","usage":{"input":1,"output":1,"totalTokens":2}}}
`
	if err := os.WriteFile(path, []byte(content), 0o644); err != nil {
		t.Fatalf("write session: %v", err)
	}
	return path
}

// TestListOmpSessions covers the summary contract: the dedicated title
// slot wins over title_change history, the combined provider/model string
// splits, every message entry is counted and assistant usage is summed.
func TestListOmpSessions(t *testing.T) {
	home := isolatedHome(t)
	a := &App{}

	sessions, err := a.ListOmpSessions()
	if err != nil || len(sessions) != 0 {
		t.Fatalf("missing dir should be empty: %v %+v", err, sessions)
	}

	path := ompWriteSession(t, home)
	sessions, err = a.ListOmpSessions()
	if err != nil {
		t.Fatalf("ListOmpSessions: %v", err)
	}
	if len(sessions) != 1 {
		t.Fatalf("expected 1 session, got %d", len(sessions))
	}
	summary := sessions[0]
	if summary.ID != "session-1" {
		t.Fatalf("id = %q", summary.ID)
	}
	// The dedicated `title` slot wins over the `title_change` history.
	if summary.Title != "Rename the parser" {
		t.Fatalf("title = %q", summary.Title)
	}
	if summary.Project != "/work/demo" {
		t.Fatalf("project = %q", summary.Project)
	}
	if summary.Model != "gpt-5" || summary.ModelProvider != "openai" {
		t.Fatalf("model = %q/%q", summary.ModelProvider, summary.Model)
	}
	// Counts every `message` entry: user, assistant, toolResult, branch.
	if summary.MessageCount != 4 {
		t.Fatalf("messageCount = %d, want 4", summary.MessageCount)
	}
	usage := summary.TokenUsage
	if usage.InputTokens != 11 || usage.OutputTokens != 6 ||
		usage.TotalTokens != 20 || usage.Cost != 0.03 {
		t.Fatalf("usage = %+v", usage)
	}
	if summary.ModifiedAt <= 0 {
		t.Fatalf("mtime = %v", summary.ModifiedAt)
	}
	if summary.Path != path {
		t.Fatalf("path = %q, want %q", summary.Path, path)
	}
}

// TestGetOmpSessionDetailActiveBranch covers the active-branch marking and
// the assistant tool-call summary line.
func TestGetOmpSessionDetailActiveBranch(t *testing.T) {
	home := isolatedHome(t)
	a := &App{}
	path := ompWriteSession(t, home)

	detail, err := a.GetOmpSessionDetail(path)
	if err != nil {
		t.Fatalf("GetOmpSessionDetail: %v", err)
	}
	byID := map[string]OmpSessionMessage{}
	for _, message := range detail.Messages {
		byID[message.ID] = message
	}
	if _, ok := byID["user-1"]; !ok {
		t.Fatalf("user-1 missing: %+v", detail.Messages)
	}
	// The assistant on the main line is superseded by `branch-1`.
	if byID["assistant-1"].IsActiveBranch {
		t.Fatalf("assistant-1 must not be active: %+v", byID["assistant-1"])
	}
	if !byID["branch-1"].IsActiveBranch {
		t.Fatalf("branch-1 should be active: %+v", byID["branch-1"])
	}
	if !byID["user-1"].IsActiveBranch {
		t.Fatalf("user-1 should be active: %+v", byID["user-1"])
	}
	// Assistant tool calls are surfaced as a summary line.
	foundSummary := false
	for _, block := range byID["assistant-1"].Content {
		if block.Type == "toolCall" && block.Text != nil && *block.Text == "bash: Checking the parser" {
			foundSummary = true
		}
	}
	if !foundSummary {
		t.Fatalf("toolCall summary missing: %+v", byID["assistant-1"].Content)
	}
	if byID["result-1"].Role != "toolResult" {
		t.Fatalf("toolResult role = %q", byID["result-1"].Role)
	}
}

// TestDeleteOmpSession validates the containment guard and removal.
func TestDeleteOmpSession(t *testing.T) {
	home := isolatedHome(t)
	a := &App{}
	path := ompWriteSession(t, home)

	outside := filepath.Join(home, "outside.jsonl")
	if err := os.WriteFile(outside, []byte("{}"), 0o644); err != nil {
		t.Fatalf("write outside: %v", err)
	}
	err := a.DeleteOmpSession(outside)
	if err == nil || !strings.Contains(err.Error(), "inside the OMP sessions directory") {
		t.Fatalf("expected containment error, got %v", err)
	}
	if _, statErr := os.Stat(outside); statErr != nil {
		t.Fatalf("outside file must survive: %v", statErr)
	}

	if err := a.DeleteOmpSession(path); err != nil {
		t.Fatalf("DeleteOmpSession: %v", err)
	}
	if _, err := os.Stat(path); !os.IsNotExist(err) {
		t.Fatalf("session file should be gone (err=%v)", err)
	}
}

// TestOmpSessionIgnoresRecordsBeforeHeader covers the ordering rule: a
// message before the `session` header is ignored instead of failing, and
// the header itself may be preceded by the title slot.
func TestOmpSessionIgnoresRecordsBeforeHeader(t *testing.T) {
	home := isolatedHome(t)
	a := &App{}
	dir := filepath.Join(home, ".omp", "agent", "sessions", "project")
	if err := os.MkdirAll(dir, 0o755); err != nil {
		t.Fatalf("mkdir: %v", err)
	}
	path := filepath.Join(dir, "early.jsonl")
	content := `{"type":"message","id":"early-1","timestamp":"2026-01-01T00:00:00.000Z","message":{"role":"user","content":[{"type":"text","text":"before header"}]}}
{"type":"session","version":3,"id":"early-session","timestamp":"2026-01-01T00:00:01.000Z","cwd":"/work/early"}
`
	if err := os.WriteFile(path, []byte(content), 0o644); err != nil {
		t.Fatalf("write: %v", err)
	}

	detail, err := a.GetOmpSessionDetail(path)
	if err != nil {
		t.Fatalf("GetOmpSessionDetail: %v", err)
	}
	if detail.Summary.ID != "early-session" || detail.Summary.Project != "/work/early" {
		t.Fatalf("summary: %+v", detail.Summary)
	}
	// The pre-header message was ignored entirely.
	if detail.Summary.MessageCount != 0 || len(detail.Messages) != 0 {
		t.Fatalf("pre-header message leaked: %+v", detail.Messages)
	}

	// A JSONL file without any session header cannot be read or deleted.
	bad := filepath.Join(dir, "bad.jsonl")
	if err := os.WriteFile(bad, []byte(`{"type":"message","id":"m"}`+"\n"), 0o644); err != nil {
		t.Fatalf("write bad: %v", err)
	}
	if _, err := a.GetOmpSessionDetail(bad); err == nil || err.Error() != "Not an OMP session file" {
		t.Fatalf("expected header error, got %v", err)
	}
	if err := a.DeleteOmpSession(bad); err == nil || err.Error() != "Not an OMP session file" {
		t.Fatalf("expected header error on delete, got %v", err)
	}
}
