package testutil

import (
	"os"
	"path/filepath"
	"strings"
	"testing"

	"droidgear-wails/internal/core"
)

// Codex sessions fixtures shared by internal/app's watcher tests and
// internal/service/read's read tests — they were one file before the B4
// split, so the constants live here (testutil imports no domain package).
// ---------------------------------------------------------------------------
// Fixture (mirrors the Rust codex_sessions test constants)
// ---------------------------------------------------------------------------

const (
	CodexSessMETA      = `{"timestamp":"2026-09-08T00:00:00Z","type":"session_meta","payload":{"id":"sess-1","cwd":"/work/repo","model_provider":"openai"}}`
	CodexSessTURN      = `{"timestamp":"2026-09-08T00:00:01Z","type":"turn_context","payload":{"cwd":"/work/repo","model":"gpt-5"}}`
	CodexSessUserInstr = `{"timestamp":"2026-09-08T00:00:02Z","type":"response_item","payload":{"type":"message","role":"user","content":[{"type":"input_text","text":"<user_instructions>\n# Guidelines\n"}]}}`
	CodexSessUserEnv   = `{"timestamp":"2026-09-08T00:00:03Z","type":"response_item","payload":{"type":"message","role":"user","content":[{"type":"input_text","text":"<environment_context>\n  <cwd>/work/repo</cwd>\n</environment_context>"}]}}`
	CodexSessUserReal  = `{"timestamp":"2026-09-08T00:00:04Z","type":"response_item","payload":{"type":"message","role":"user","content":[{"type":"input_text","text":"Fix the bug"}]}}`
	CodexSessThinking  = `{"timestamp":"2026-09-08T00:00:05Z","type":"event_msg","payload":{"type":"agent_reasoning","text":"I should check the parser."}}`
	CodexSessSummary   = `{"timestamp":"2026-09-08T00:00:06Z","type":"response_item","payload":{"type":"reasoning","summary":[{"type":"summary_text","text":"summary text"}],"encrypted_content":"enc"}}`
	CodexSessAssistant = `{"timestamp":"2026-09-08T00:00:07Z","type":"response_item","payload":{"type":"message","role":"assistant","content":[{"type":"output_text","text":"Done"}]}}`
	CodexSessTokens    = `{"timestamp":"2026-09-08T00:00:08Z","type":"event_msg","payload":{"type":"token_count","info":{"total_token_usage":{"input_tokens":100,"cached_input_tokens":10,"cache_write_input_tokens":5,"output_tokens":20,"reasoning_output_tokens":30,"total_tokens":165}}}}`
)

var CodexSessFull = []string{
	CodexSessMETA, CodexSessTURN, CodexSessUserInstr, CodexSessUserEnv,
	CodexSessUserReal, CodexSessThinking, CodexSessSummary, CodexSessAssistant,
	CodexSessTokens,
}

// CodexSessFixture creates <home>/.codex/sessions/2026/09/08/rollout-test.jsonl.
func CodexSessFixture(t *testing.T, lines []string) string {
	t.Helper()
	home, err := CoreHomeForTest()
	if err != nil {
		t.Fatalf("home: %v", err)
	}
	dir := filepath.Join(home, ".codex", "sessions", "2026", "09", "08")
	if err := os.MkdirAll(dir, 0o755); err != nil {
		t.Fatalf("mkdir: %v", err)
	}
	path := filepath.Join(dir, "rollout-test.jsonl")
	content := strings.Join(lines, "\n") + "\n"
	if err := os.WriteFile(path, []byte(content), 0o644); err != nil {
		t.Fatalf("write session: %v", err)
	}
	return path
}

// CodexSessWithIndex appends a session_index.jsonl entry (newest wins).
func CodexSessWithIndex(t *testing.T, id string, threadName string) {
	t.Helper()
	home, err := CoreHomeForTest()
	if err != nil {
		t.Fatalf("home: %v", err)
	}
	path := filepath.Join(home, ".codex", "session_index.jsonl")
	line := `{"id":"` + id + `","thread_name":"` + threadName + `","updated_at":"2026-09-08T00:00:00Z"}` + "\n"
	if err := os.WriteFile(path, []byte(line), 0o644); err != nil {
		t.Fatalf("write index: %v", err)
	}
}

// ---------------------------------------------------------------------------
// Listing
// ---------------------------------------------------------------------------

// TestCodexSessionListReadsSummaryAndIndexTitle covers the list view: index
// title wins, head-window meta supplies id/cwd/model/provider, tail-window
// token usage, and newest-first ordering.

// CoreHomeForTest resolves the isolated home the tests run against.
func CoreHomeForTest() (string, error) {
	return core.HomeDir()
}

// CodexSessFixtureWithID writes a session file with a custom file name.
func CodexSessFixtureWithID(t *testing.T, name string, lines []string) string {
	t.Helper()
	home, err := CoreHomeForTest()
	if err != nil {
		t.Fatalf("home: %v", err)
	}
	dir := filepath.Join(home, ".codex", "sessions", "2026", "09", "07")
	if err := os.MkdirAll(dir, 0o755); err != nil {
		t.Fatalf("mkdir: %v", err)
	}
	path := filepath.Join(dir, name+".jsonl")
	if err := os.WriteFile(path, []byte(strings.Join(lines, "\n")+"\n"), 0o644); err != nil {
		t.Fatalf("write session: %v", err)
	}
	return path
}
