package app

import (
	"bytes"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/klauspost/compress/zstd"
)

// Fixture records mirroring the Rust dsh_sessions tests.
const (
	dshFixtureHeader = `{"type":"session","version":4,"id":"session-test","createdAt":1700000000000,"cwd":"/work/repo","isSeeded":false,"delegationDepth":0,"agentPreset":"standard"}`
	// Injected runtime context: filtered from the transcript.
	dshFixtureRuntime = `{"type":"user/message","seq":1,"time":1700000001000,"data":{"content":[{"type":"text","text":"Injected runtime context"}],"source":{"kind":"runtime-context"},"role":"user","id":"ctx-1"}}`
	dshFixtureUser    = `{"type":"user/message","seq":2,"time":1700000002000,"data":{"content":[{"type":"text","text":"Fix the Dsh session parser"}],"source":{"kind":"user"},"role":"user","id":"user-1"}}`
	dshFixtureTitle   = `{"type":"session/title","seq":3,"time":1700000003000,"data":{"title":"Fix the Dsh session parser","messageSeqs":[2],"source":{"kind":"fallback"}}}`
	dshFixtureRequest = `{"type":"request/header","seq":4,"time":1700000004000,"data":{"header":{"config":{"provider":"deepseek-official","model":"deepseek-flash"}}}}`
	dshFixtureAssist  = `{"type":"assistant/message","seq":5,"time":1700000005000,"data":{"turn":1,"step":1,"message":{"role":"assistant","content":[{"type":"reasoning","text":"Let me inspect the files."},{"type":"text","text":"Done."},{"type":"tool-call","id":"call-1","name":"bash","arguments":"{}"}],"source":{"kind":"model","provider":"deepseek-official","model":"deepseek-flash"},"id":"asst-1"},"usage":{"inputTokens":100,"outputTokens":20,"cacheReadTokens":30,"cacheWriteTokens":5,"totalTokens":155}}}`
)

var dshFixtureFull = []string{
	dshFixtureHeader,
	dshFixtureRuntime,
	dshFixtureRequest,
	dshFixtureUser,
	dshFixtureTitle,
	dshFixtureAssist,
}

// dshWriteRawSession creates
// `<home>/.dsh/sessions/--work-repo--/session-test/session.jsonl`.
func dshWriteRawSession(t *testing.T, home string, lines []string) (string, string) {
	t.Helper()
	sessionDir := filepath.Join(home, ".dsh", "sessions", "--work-repo--", "session-test")
	if err := os.MkdirAll(sessionDir, 0o755); err != nil {
		t.Fatalf("mkdir: %v", err)
	}
	logPath := filepath.Join(sessionDir, "session.jsonl")
	content := strings.Join(lines, "\n") + "\n"
	if err := os.WriteFile(logPath, []byte(content), 0o644); err != nil {
		t.Fatalf("write log: %v", err)
	}
	return sessionDir, logPath
}

// dshEncodeFrames compresses each record as its own Zstandard frame and
// concatenates them — the physical encoding Dsh's JSONL persistence writes
// (header frame + one frame per durable append batch).
func dshEncodeFrames(t *testing.T, lines []string) []byte {
	t.Helper()
	var buffer bytes.Buffer
	for _, line := range lines {
		writer, err := zstd.NewWriter(&buffer)
		if err != nil {
			t.Fatalf("zstd writer: %v", err)
		}
		if _, err := writer.Write([]byte(line + "\n")); err != nil {
			t.Fatalf("zstd write: %v", err)
		}
		if err := writer.Close(); err != nil {
			t.Fatalf("zstd close: %v", err)
		}
	}
	return buffer.Bytes()
}

func TestParseDshLogName(t *testing.T) {
	cases := []struct {
		name       string
		version    uint32
		compressed bool
		ok         bool
	}{
		{"session.jsonl", 0, false, true},
		{"session.jsonl.zstd", 0, true, true},
		{"session.v4.jsonl", 4, false, true},
		{"session.v4.jsonl.zstd", 4, true, true},
		{"session.v12.jsonl", 12, false, true},
		{"session.vX.jsonl", 0, false, false},
		{"session.v", 0, false, false},
		{"other.jsonl", 0, false, false},
		{"session.txt", 0, false, false},
		{"session", 0, false, false},
	}
	for _, c := range cases {
		version, compressed, ok := parseDshLogName(c.name)
		if version != c.version || compressed != c.compressed || ok != c.ok {
			t.Errorf("parseDshLogName(%q) = (%d,%v,%v), want (%d,%v,%v)",
				c.name, version, compressed, ok, c.version, c.compressed, c.ok)
		}
	}
}

// TestListDshSessionsRawLog covers the log-derived summary: id, title,
// cwd-as-project, model route and log path.
func TestListDshSessionsRawLog(t *testing.T) {
	home := isolatedHome(t)
	a := &App{}

	sessions, err := a.ListDshSessions()
	if err != nil || len(sessions) != 0 {
		t.Fatalf("missing dir should be empty: %v %+v", err, sessions)
	}

	_, logPath := dshWriteRawSession(t, home, dshFixtureFull)
	sessions, err = a.ListDshSessions()
	if err != nil {
		t.Fatalf("ListDshSessions: %v", err)
	}
	if len(sessions) != 1 {
		t.Fatalf("expected 1 session, got %d", len(sessions))
	}
	summary := sessions[0]
	if summary.ID != "session-test" {
		t.Fatalf("id = %q", summary.ID)
	}
	if summary.Title != "Fix the Dsh session parser" {
		t.Fatalf("title = %q", summary.Title)
	}
	if summary.Project != "/work/repo" {
		t.Fatalf("project = %q", summary.Project)
	}
	if summary.Model != "deepseek-flash" || summary.ModelProvider != "deepseek-official" {
		t.Fatalf("model route = %q/%q", summary.ModelProvider, summary.Model)
	}
	// No projection cache yet → zero usage.
	if summary.TokenUsage.InputTokens != 0 || summary.TokenUsage.TotalTokens != 0 {
		t.Fatalf("usage = %+v", summary.TokenUsage)
	}
	if summary.Path != logPath {
		t.Fatalf("path = %q, want %q", summary.Path, logPath)
	}
	if summary.ModifiedAt <= 0 {
		t.Fatalf("mtime = %v", summary.ModifiedAt)
	}
}

// TestListDshSessionsProjcacheOverride covers the projection-cache fast
// path: cached title/model/usage win over the log head.
func TestListDshSessionsProjcacheOverride(t *testing.T) {
	home := isolatedHome(t)
	a := &App{}
	dshWriteRawSession(t, home, []string{dshFixtureHeader, dshFixtureUser})

	cacheDir := filepath.Join(home, ".dsh", "storages", "session_projcache", "sessions")
	if err := os.MkdirAll(cacheDir, 0o755); err != nil {
		t.Fatalf("mkdir: %v", err)
	}
	cache := `{"version":7,"record":{"identity":{"formatVersion":4,"createdAt":1700000000000,"cwd":"/work/repo"},"rows":{"title":{"ver":1,"seq":1,"val":"Renamed session"},"titleInput":{"ver":3,"seq":1,"val":{"first":{"seq":0,"text":"first prompt"},"count":1,"lastSeq":0}},"modelSelection":{"ver":2,"seq":1,"val":{"lastUsed":{"provider":"deepseek-official","model":"deepseek-flash"},"pending":null}},"tokenUsage":{"ver":2,"seq":1,"val":{"totals":{"uncachedInputTokens":100,"outputTokens":20,"cacheReadTokens":30,"cacheWriteTokens":5},"last":null}}}}}`
	if err := os.WriteFile(filepath.Join(cacheDir, "session-test.json"), []byte(cache), 0o644); err != nil {
		t.Fatalf("write cache: %v", err)
	}

	sessions, err := a.ListDshSessions()
	if err != nil || len(sessions) != 1 {
		t.Fatalf("ListDshSessions: %v %+v", err, sessions)
	}
	summary := sessions[0]
	if summary.Title != "Renamed session" {
		t.Fatalf("title = %q", summary.Title)
	}
	if summary.Model != "deepseek-flash" || summary.ModelProvider != "deepseek-official" {
		t.Fatalf("model route = %q/%q", summary.ModelProvider, summary.Model)
	}
	usage := summary.TokenUsage
	if usage.InputTokens != 100 || usage.OutputTokens != 20 ||
		usage.CacheReadTokens != 30 || usage.CacheCreationTokens != 5 ||
		usage.TotalTokens != 155 {
		t.Fatalf("usage = %+v", usage)
	}
}

// TestListDshSessionsFirstPromptTitle covers the title fallback chain when
// no title record exists.
func TestListDshSessionsFirstPromptTitle(t *testing.T) {
	home := isolatedHome(t)
	a := &App{}
	dshWriteRawSession(t, home, []string{dshFixtureHeader, dshFixtureRuntime, dshFixtureUser})

	sessions, err := a.ListDshSessions()
	if err != nil || len(sessions) != 1 {
		t.Fatalf("ListDshSessions: %v %+v", err, sessions)
	}
	// The injected runtime-context message must not become the title.
	if sessions[0].Title != "Fix the Dsh session parser" {
		t.Fatalf("title = %q", sessions[0].Title)
	}
}

// TestListDshSessionsPrefersNewestGeneration covers the newest-generation
// selection (higher version wins, compressed wins at equal version).
func TestListDshSessionsPrefersNewestGeneration(t *testing.T) {
	home := isolatedHome(t)
	a := &App{}
	_, logPath := dshWriteRawSession(t, home, []string{dshFixtureHeader, dshFixtureUser})

	v4 := filepath.Join(filepath.Dir(logPath), "session.v4.jsonl")
	content := dshFixtureHeader + "\n" +
		`{"type":"session/title","seq":1,"time":1700000000000,"data":{"title":"Newest generation"}}` + "\n"
	if err := os.WriteFile(v4, []byte(content), 0o644); err != nil {
		t.Fatalf("write v4: %v", err)
	}

	sessions, err := a.ListDshSessions()
	if err != nil || len(sessions) != 1 {
		t.Fatalf("ListDshSessions: %v %+v", err, sessions)
	}
	if sessions[0].Title != "Newest generation" {
		t.Fatalf("title = %q", sessions[0].Title)
	}
	if !strings.HasSuffix(sessions[0].Path, "session.v4.jsonl") {
		t.Fatalf("path = %q", sessions[0].Path)
	}
}

// TestListDshSessionsDecodesConcatenatedZstdFrames proves the compressed
// default encoding (concatenated frames per append batch) decodes for both
// the list and the detail view.
func TestListDshSessionsDecodesConcatenatedZstdFrames(t *testing.T) {
	home := isolatedHome(t)
	a := &App{}

	sessionDir := filepath.Join(home, ".dsh", "sessions", "--work-repo--", "session-test")
	if err := os.MkdirAll(sessionDir, 0o755); err != nil {
		t.Fatalf("mkdir: %v", err)
	}
	zstdPath := filepath.Join(sessionDir, "session.v4.jsonl.zstd")
	if err := os.WriteFile(zstdPath, dshEncodeFrames(t, dshFixtureFull), 0o644); err != nil {
		t.Fatalf("write zstd: %v", err)
	}

	sessions, err := a.ListDshSessions()
	if err != nil {
		t.Fatalf("ListDshSessions: %v", err)
	}
	if len(sessions) != 1 {
		t.Fatalf("expected 1 session, got %d", len(sessions))
	}
	summary := sessions[0]
	if summary.Title != "Fix the Dsh session parser" || summary.Model != "deepseek-flash" {
		t.Fatalf("zstd summary: %+v", summary)
	}
	if summary.Path != zstdPath {
		t.Fatalf("path = %q, want %q", summary.Path, zstdPath)
	}

	detail, err := a.GetDshSessionDetail(zstdPath)
	if err != nil {
		t.Fatalf("GetDshSessionDetail: %v", err)
	}
	if len(detail.Messages) != 2 {
		t.Fatalf("expected 2 messages, got %d: %+v", len(detail.Messages), detail.Messages)
	}
	if detail.Messages[1].Content[0].Type != "thinking" {
		t.Fatalf("expected thinking block: %+v", detail.Messages[1].Content)
	}
}

// TestGetDshSessionDetail covers the transcript filters and usage
// accumulation across assistant messages.
func TestGetDshSessionDetail(t *testing.T) {
	home := isolatedHome(t)
	a := &App{}
	_, logPath := dshWriteRawSession(t, home, dshFixtureFull)

	detail, err := a.GetDshSessionDetail(logPath)
	if err != nil {
		t.Fatalf("GetDshSessionDetail: %v", err)
	}
	summary := detail.Summary
	if summary.ID != "session-test" || summary.Project != "/work/repo" ||
		summary.Title != "Fix the Dsh session parser" || summary.Model != "deepseek-flash" {
		t.Fatalf("summary: %+v", summary)
	}
	if summary.TokenUsage.InputTokens != 100 || summary.TokenUsage.CacheCreationTokens != 5 {
		t.Fatalf("usage: %+v", summary.TokenUsage)
	}
	// Injected runtime-context message filtered; tool-call block skipped;
	// reasoning mapped to a thinking block.
	if len(detail.Messages) != 2 {
		t.Fatalf("expected 2 messages, got %d: %+v", len(detail.Messages), detail.Messages)
	}
	user := detail.Messages[0]
	if user.Role != "user" || len(user.Content) != 1 || user.Content[0].Text == nil ||
		*user.Content[0].Text != "Fix the Dsh session parser" {
		t.Fatalf("user message: %+v", user)
	}
	if user.Timestamp != "2023-11-14T22:13:22Z" {
		t.Fatalf("user timestamp = %q", user.Timestamp)
	}
	assistant := detail.Messages[1]
	if assistant.Role != "assistant" || len(assistant.Content) != 2 {
		t.Fatalf("assistant message: %+v", assistant)
	}
	if assistant.Content[0].Type != "thinking" ||
		assistant.Content[0].Thinking == nil ||
		*assistant.Content[0].Thinking != "Let me inspect the files." {
		t.Fatalf("thinking block: %+v", assistant.Content[0])
	}
	if assistant.Content[1].Text == nil || *assistant.Content[1].Text != "Done." {
		t.Fatalf("text block: %+v", assistant.Content[1])
	}
	if assistant.ID != "asst-1" {
		t.Fatalf("assistant id = %q", assistant.ID)
	}

	// A second assistant message accumulates usage.
	second := `{"type":"assistant/message","seq":6,"time":1700000006000,"data":{"message":{"role":"assistant","content":[{"type":"text","text":"More."}],"id":"asst-2"},"usage":{"inputTokens":10,"outputTokens":2,"cacheReadTokens":3,"cacheWriteTokens":0}}}`
	if err := os.WriteFile(logPath, []byte(strings.Join(append(dshFixtureFull, second), "\n")+"\n"), 0o644); err != nil {
		t.Fatalf("append: %v", err)
	}
	detail, err = a.GetDshSessionDetail(logPath)
	if err != nil {
		t.Fatalf("GetDshSessionDetail (2): %v", err)
	}
	usage := detail.Summary.TokenUsage
	if usage.InputTokens != 110 || usage.OutputTokens != 22 || usage.CacheReadTokens != 33 {
		t.Fatalf("accumulated usage: %+v", usage)
	}
	if len(detail.Messages) != 3 {
		t.Fatalf("expected 3 messages, got %d", len(detail.Messages))
	}

	// Missing file → byte-identical Rust error.
	if _, err := a.GetDshSessionDetail(filepath.Join(filepath.Dir(logPath), "missing.jsonl")); err == nil ||
		err.Error() != "Dsh session file not found" {
		t.Fatalf("expected not-found error, got %v", err)
	}
}

// TestDeleteDshSession covers the safety guard (only logs inside the Dsh
// sessions tree), session-dir removal and project-dir pruning.
func TestDeleteDshSession(t *testing.T) {
	home := isolatedHome(t)
	a := &App{}
	sessionDir, logPath := dshWriteRawSession(t, home, dshFixtureFull)

	// A file outside the sessions directory must be rejected.
	outside := filepath.Join(home, "settings.yaml")
	if err := os.WriteFile(outside, []byte("x"), 0o644); err != nil {
		t.Fatalf("write outside: %v", err)
	}
	err := a.DeleteDshSession(outside)
	if err == nil || !strings.Contains(err.Error(), "not found") {
		t.Fatalf("expected not-found error, got %v", err)
	}
	if _, statErr := os.Stat(outside); statErr != nil {
		t.Fatalf("outside file must survive: %v", statErr)
	}

	if err := a.DeleteDshSession(logPath); err != nil {
		t.Fatalf("DeleteDshSession: %v", err)
	}
	if _, err := os.Stat(sessionDir); !os.IsNotExist(err) {
		t.Fatalf("session dir should be gone (err=%v)", err)
	}
	projectDir := filepath.Dir(sessionDir)
	if _, err := os.Stat(projectDir); !os.IsNotExist(err) {
		t.Fatalf("empty project dir should be pruned (err=%v)", err)
	}
	sessionsDir := filepath.Join(home, ".dsh", "sessions")
	if _, err := os.Stat(sessionsDir); err != nil {
		t.Fatalf("sessions root must survive: %v", err)
	}
}
