package read

import (
	"droidgear-wails/internal/testutil"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

// piIsolatedHome isolates the home plus the Pi-specific environment
// overrides so a developer's real PI_CODING_AGENT_* settings cannot leak
// into the fixtures.
func piIsolatedHome(t *testing.T) string {
	t.Helper()
	home := testutil.IsolatedHome(t)
	t.Setenv("PI_CODING_AGENT_DIR", "")
	t.Setenv("PI_CODING_AGENT_SESSION_DIR", "")
	return home
}

// piWriteSession drops the shared Pi fixture (header → model_change →
// messages → cleared session_info name → branch → compaction) into
// `<home>/.pi/agent/sessions/project/session.jsonl`.
func piWriteSession(t *testing.T, home string) string {
	t.Helper()
	dir := filepath.Join(home, ".pi", "agent", "sessions", "project")
	if err := os.MkdirAll(dir, 0o755); err != nil {
		t.Fatalf("mkdir: %v", err)
	}
	path := filepath.Join(dir, "session.jsonl")
	content := `{"type":"session","version":3,"id":"session-1","timestamp":"2026-01-01T00:00:00.000Z","cwd":"/work/demo"}
{"type":"model_change","id":"model-1","parentId":null,"timestamp":"2026-01-01T00:00:00.100Z","provider":"openai","modelId":"gpt-5"}
{"type":"message","id":"user-1","parentId":"model-1","timestamp":"2026-01-01T00:00:01.000Z","message":{"role":"user","content":"Investigate the parser","timestamp":1767225601000}}
{"type":"message","id":"assistant-1","parentId":"user-1","timestamp":"2026-01-01T00:00:02.000Z","message":{"role":"assistant","provider":"openai","model":"gpt-5","content":[{"type":"thinking","thinking":"Read the file"},{"type":"text","text":"Done"}],"usage":{"input":10,"output":5,"cacheRead":2,"cacheWrite":1,"totalTokens":18,"cost":{"total":0.03}},"timestamp":1767225602000}}
{"type":"session_info","id":"name-1","parentId":"assistant-1","timestamp":"2026-01-01T00:00:03.000Z","name":"Parser review"}
{"type":"session_info","id":"name-2","parentId":"name-1","timestamp":"2026-01-01T00:00:04.000Z","name":"  "}
{"type":"message","id":"branch-1","parentId":"user-1","timestamp":"2026-01-01T00:00:05.000Z","message":{"role":"assistant","provider":"openai","model":"gpt-5","content":"Alternate answer","usage":{"input":1,"output":1,"totalTokens":2}}}
{"type":"compaction","id":"compact-1","parentId":"branch-1","timestamp":"2026-01-01T00:00:06.000Z","summary":"Older context summarized","tokensBefore":100,"usage":{"input":3,"output":1,"totalTokens":4,"cost":{"total":0.01}}}
`
	if err := os.WriteFile(path, []byte(content), 0o644); err != nil {
		t.Fatalf("write session: %v", err)
	}
	return path
}

// TestListPiSessions covers the summary contract: first-prompt title
// fallback (the cleared session_info name), header cwd as project, model
// from the active branch, message counts and accumulated usage.
func TestListPiSessions(t *testing.T) {
	home := piIsolatedHome(t)

	sessions, err := ListPiSessions()
	if err != nil || len(sessions) != 0 {
		t.Fatalf("missing dir should be empty: %v %+v", err, sessions)
	}

	path := piWriteSession(t, home)
	sessions, err = ListPiSessions()
	if err != nil {
		t.Fatalf("ListPiSessions: %v", err)
	}
	if len(sessions) != 1 {
		t.Fatalf("expected 1 session, got %d", len(sessions))
	}
	summary := sessions[0]
	if summary.ID != "session-1" {
		t.Fatalf("id = %q", summary.ID)
	}
	// An explicitly cleared session_info falls back to the first prompt.
	if summary.Title != "Investigate the parser" {
		t.Fatalf("title = %q", summary.Title)
	}
	if summary.Project != "/work/demo" {
		t.Fatalf("project = %q", summary.Project)
	}
	if summary.Model != "gpt-5" || summary.ModelProvider != "openai" {
		t.Fatalf("model = %q/%q", summary.ModelProvider, summary.Model)
	}
	if summary.MessageCount != 3 {
		t.Fatalf("messageCount = %d, want 3", summary.MessageCount)
	}
	usage := summary.TokenUsage
	if usage.InputTokens != 14 || usage.OutputTokens != 7 ||
		usage.TotalTokens != 24 || usage.Cost != 0.04 {
		t.Fatalf("usage = %+v", usage)
	}
	if summary.ModifiedAt <= 0 {
		t.Fatalf("mtime = %v", summary.ModifiedAt)
	}
	if summary.Path != path {
		t.Fatalf("path = %q, want %q", summary.Path, path)
	}
}

// TestGetPiSessionDetailActiveBranch covers the transcript render set and
// the active-branch marking (ancestry of the last entry).
func TestGetPiSessionDetailActiveBranch(t *testing.T) {
	home := piIsolatedHome(t)
	path := piWriteSession(t, home)

	detail, err := GetPiSessionDetail(path)
	if err != nil {
		t.Fatalf("GetPiSessionDetail: %v", err)
	}
	if len(detail.Messages) != 4 {
		t.Fatalf("expected 4 messages, got %d: %+v", len(detail.Messages), detail.Messages)
	}
	byID := map[string]PiSessionMessage{}
	for _, message := range detail.Messages {
		byID[message.ID] = message
	}
	if !byID["user-1"].IsActiveBranch {
		t.Fatalf("user-1 should be on the active branch: %+v", byID["user-1"])
	}
	if byID["assistant-1"].IsActiveBranch {
		t.Fatalf("superseded assistant-1 must not be active: %+v", byID["assistant-1"])
	}
	if !byID["compact-1"].IsActiveBranch {
		t.Fatalf("compact-1 should be on the active branch: %+v", byID["compact-1"])
	}
	if !byID["branch-1"].IsActiveBranch {
		t.Fatalf("branch-1 should be on the active branch: %+v", byID["branch-1"])
	}
	// String content maps to a text block; block content keeps thinking.
	if byID["user-1"].Role != "user" || len(byID["user-1"].Content) != 1 ||
		byID["user-1"].Content[0].Type != "text" {
		t.Fatalf("user content: %+v", byID["user-1"])
	}
	if byID["compact-1"].Role != "system" || byID["compact-1"].Content[0].Type != "compaction" {
		t.Fatalf("compaction block: %+v", byID["compact-1"])
	}
}

// TestDeletePiSession validates the containment guard and the header
// check before removal.
func TestDeletePiSession(t *testing.T) {
	home := piIsolatedHome(t)
	path := piWriteSession(t, home)

	// Unrelated JSONL outside the sessions tree is rejected.
	outside := filepath.Join(home, "outside.jsonl")
	if err := os.WriteFile(outside, []byte("{}"), 0o644); err != nil {
		t.Fatalf("write outside: %v", err)
	}
	err := DeletePiSession(outside)
	if err == nil || !strings.Contains(err.Error(), "inside the Pi sessions directory") {
		t.Fatalf("expected containment error, got %v", err)
	}
	if _, statErr := os.Stat(outside); statErr != nil {
		t.Fatalf("outside file must survive: %v", statErr)
	}

	if err := DeletePiSession(path); err != nil {
		t.Fatalf("DeletePiSession: %v", err)
	}
	if _, err := os.Stat(path); !os.IsNotExist(err) {
		t.Fatalf("session file should be gone (err=%v)", err)
	}
}

// TestPiSessionValidationCoversHeaderAndExtension covers the two guards:
// a JSONL file without a session header cannot be read or deleted, and a
// non-.jsonl path is rejected outright.
func TestPiSessionValidationCoversHeaderAndExtension(t *testing.T) {
	home := piIsolatedHome(t)
	dir := filepath.Join(home, ".pi", "agent", "sessions", "project")
	if err := os.MkdirAll(dir, 0o755); err != nil {
		t.Fatalf("mkdir: %v", err)
	}

	bad := filepath.Join(dir, "bad.jsonl")
	if err := os.WriteFile(bad, []byte(`{"type":"message","id":"m"}`+"\n"), 0o644); err != nil {
		t.Fatalf("write bad: %v", err)
	}
	if _, err := GetPiSessionDetail(bad); err == nil || err.Error() != "Not a Pi session file" {
		t.Fatalf("expected header error, got %v", err)
	}
	if err := DeletePiSession(bad); err == nil || err.Error() != "Not a Pi session file" {
		t.Fatalf("expected header error on delete, got %v", err)
	}

	txt := filepath.Join(dir, "notes.txt")
	if err := os.WriteFile(txt, []byte("x"), 0o644); err != nil {
		t.Fatalf("write txt: %v", err)
	}
	if err := DeletePiSession(txt); err == nil ||
		!strings.Contains(err.Error(), "inside the Pi sessions directory") {
		t.Fatalf("expected extension error, got %v", err)
	}
}

// TestPiSessionsDirEnvironmentOverride covers the env precedence:
// PI_CODING_AGENT_SESSION_DIR wins over everything.
func TestPiSessionsDirEnvironmentOverride(t *testing.T) {
	home := piIsolatedHome(t)

	alt := filepath.Join(home, "alt-sessions", "project")
	if err := os.MkdirAll(alt, 0o755); err != nil {
		t.Fatalf("mkdir: %v", err)
	}
	altPath := filepath.Join(alt, "env.jsonl")
	if err := os.WriteFile(altPath, []byte(
		`{"type":"session","version":3,"id":"env-1","timestamp":"2026-01-01T00:00:00.000Z","cwd":"/work/env"}`+"\n",
	), 0o644); err != nil {
		t.Fatalf("write: %v", err)
	}
	// A default-layout session that must be invisible while the override
	// points elsewhere.
	piWriteSession(t, home)

	t.Setenv("PI_CODING_AGENT_SESSION_DIR", filepath.Join(home, "alt-sessions"))
	sessions, err := ListPiSessions()
	if err != nil {
		t.Fatalf("ListPiSessions: %v", err)
	}
	if len(sessions) != 1 || sessions[0].ID != "env-1" {
		t.Fatalf("override not honoured: %+v", sessions)
	}
}
