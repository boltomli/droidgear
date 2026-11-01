package app

import (
	"database/sql"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"testing"
	"time"

	"droidgear-wails/internal/core"
)

// ---------------------------------------------------------------------------
// Fixture (mirrors the Rust codex_sessions test constants)
// ---------------------------------------------------------------------------

const (
	codexSessMETA      = `{"timestamp":"2026-09-08T00:00:00Z","type":"session_meta","payload":{"id":"sess-1","cwd":"/work/repo","model_provider":"openai"}}`
	codexSessTURN      = `{"timestamp":"2026-09-08T00:00:01Z","type":"turn_context","payload":{"cwd":"/work/repo","model":"gpt-5"}}`
	codexSessUserInstr = `{"timestamp":"2026-09-08T00:00:02Z","type":"response_item","payload":{"type":"message","role":"user","content":[{"type":"input_text","text":"<user_instructions>\n# Guidelines\n"}]}}`
	codexSessUserEnv   = `{"timestamp":"2026-09-08T00:00:03Z","type":"response_item","payload":{"type":"message","role":"user","content":[{"type":"input_text","text":"<environment_context>\n  <cwd>/work/repo</cwd>\n</environment_context>"}]}}`
	codexSessUserReal  = `{"timestamp":"2026-09-08T00:00:04Z","type":"response_item","payload":{"type":"message","role":"user","content":[{"type":"input_text","text":"Fix the bug"}]}}`
	codexSessThinking  = `{"timestamp":"2026-09-08T00:00:05Z","type":"event_msg","payload":{"type":"agent_reasoning","text":"I should check the parser."}}`
	codexSessSummary   = `{"timestamp":"2026-09-08T00:00:06Z","type":"response_item","payload":{"type":"reasoning","summary":[{"type":"summary_text","text":"summary text"}],"encrypted_content":"enc"}}`
	codexSessAssistant = `{"timestamp":"2026-09-08T00:00:07Z","type":"response_item","payload":{"type":"message","role":"assistant","content":[{"type":"output_text","text":"Done"}]}}`
	codexSessTokens    = `{"timestamp":"2026-09-08T00:00:08Z","type":"event_msg","payload":{"type":"token_count","info":{"total_token_usage":{"input_tokens":100,"cached_input_tokens":10,"cache_write_input_tokens":5,"output_tokens":20,"reasoning_output_tokens":30,"total_tokens":165}}}}`
)

var codexSessFull = []string{
	codexSessMETA, codexSessTURN, codexSessUserInstr, codexSessUserEnv,
	codexSessUserReal, codexSessThinking, codexSessSummary, codexSessAssistant,
	codexSessTokens,
}

// codexSessFixture creates <home>/.codex/sessions/2026/09/08/rollout-test.jsonl.
func codexSessFixture(t *testing.T, lines []string) string {
	t.Helper()
	home, err := coreHomeForTest()
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

// codexSessWithIndex appends a session_index.jsonl entry (newest wins).
func codexSessWithIndex(t *testing.T, id string, threadName string) {
	t.Helper()
	home, err := coreHomeForTest()
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
func TestCodexSessionListReadsSummaryAndIndexTitle(t *testing.T) {
	isolatedHome(t)
	a := &App{}

	// Missing sessions dir → empty list, no error.
	sessions, err := a.ListCodexSessions()
	if err != nil || len(sessions) != 0 {
		t.Fatalf("empty list: %v %d", err, len(sessions))
	}

	path := codexSessFixture(t, codexSessFull)
	codexSessWithIndex(t, "sess-1", "Fix the bug")

	sessions, err = a.ListCodexSessions()
	if err != nil {
		t.Fatalf("ListCodexSessions: %v", err)
	}
	if len(sessions) != 1 {
		t.Fatalf("sessions: %d", len(sessions))
	}
	s := sessions[0]
	if s.ID != "sess-1" || s.Title != "Fix the bug" || s.Project != "/work/repo" {
		t.Fatalf("summary: %+v", s)
	}
	if s.Model != "gpt-5" || s.ModelProvider != "openai" {
		t.Fatalf("model/provider: %+v", s)
	}
	if s.Path != path {
		t.Fatalf("path: %q want %q", s.Path, path)
	}
	if s.ModifiedAt <= 0 {
		t.Fatalf("modifiedAt: %v", s.ModifiedAt)
	}
	usage := s.TokenUsage
	if usage.InputTokens != 100 || usage.OutputTokens != 20 ||
		usage.CacheReadTokens != 10 || usage.CacheCreationTokens != 5 ||
		usage.ReasoningTokens != 30 || usage.TotalTokens != 165 {
		t.Fatalf("token usage: %+v", usage)
	}
}

// TestCodexSessionListTitleFallbacks: first real user message when the index
// has no entry; "New Session" when neither source exists.
func TestCodexSessionListTitleFallbacks(t *testing.T) {
	isolatedHome(t)
	a := &App{}

	codexSessFixture(t, codexSessFull)
	sessions, err := a.ListCodexSessions()
	if err != nil || len(sessions) != 1 {
		t.Fatalf("list: %v %d", err, len(sessions))
	}
	// Injected <user_instructions>/<environment_context> blocks are skipped —
	// the title comes from "Fix the bug".
	if sessions[0].Title != "Fix the bug" {
		t.Fatalf("title: %q", sessions[0].Title)
	}

	codexSessFixtureWithID(t, "rollout-bare", []string{codexSessMETA, codexSessTURN})
	sessions, err = a.ListCodexSessions()
	if err != nil {
		t.Fatalf("list: %v", err)
	}
	var bare *CodexSessionSummary
	for i := range sessions {
		if sessions[i].ID == "bare" || strings.Contains(sessions[i].Path, "rollout-bare") {
			bare = &sessions[i]
		}
	}
	if bare == nil {
		t.Fatalf("bare session missing: %+v", sessions)
	}
	if bare.Title != "New Session" || bare.Model != "gpt-5" || bare.TokenUsage.TotalTokens != 0 {
		t.Fatalf("bare: %+v", bare)
	}
}

// codexSessFixtureWithID writes a session file with a custom file name.
func codexSessFixtureWithID(t *testing.T, name string, lines []string) string {
	t.Helper()
	home, err := coreHomeForTest()
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

// TestCodexSessionListTokenUsageFromTailWindow pads a file past the tail
// window so the final token_count event is only visible via the tail read.
func TestCodexSessionListTokenUsageFromTailWindow(t *testing.T) {
	isolatedHome(t)
	a := &App{}

	path := codexSessFixture(t, []string{codexSessMETA})
	padding := `{"timestamp":"2026-09-08T00:00:00Z","type":"event_msg","payload":{"type":"agent_reasoning","text":"xxxxxxxxxxxxxxxxxxxxxxxxxxxxxxxxxxxxxxxxxxxxxxxxxxxxxxxxxxxxxxxx"}}` + "\n"
	f, err := os.OpenFile(path, os.O_APPEND|os.O_WRONLY, 0o644)
	if err != nil {
		t.Fatalf("open: %v", err)
	}
	for i := 0; i < 6000; i++ { // > HEAD+TAIL (384KiB)
		if _, err := f.WriteString(padding); err != nil {
			t.Fatalf("pad: %v", err)
		}
	}
	if _, err := f.WriteString(codexSessTokens + "\n"); err != nil {
		t.Fatalf("append tokens: %v", err)
	}
	_ = f.Close()

	sessions, err := a.ListCodexSessions()
	if err != nil || len(sessions) != 1 {
		t.Fatalf("list: %v %d", err, len(sessions))
	}
	if sessions[0].TokenUsage.TotalTokens != 165 {
		t.Fatalf("tail usage: %+v", sessions[0].TokenUsage)
	}
}

// ---------------------------------------------------------------------------
// Detail
// ---------------------------------------------------------------------------

// TestCodexSessionDetailFiltersInjectedBlocksAndAttachesThinking covers the
// chat rendering contract: injected user blocks are dropped, agent_reasoning
// becomes the leading thinking block (winning over reasoning summaries), and
// token usage is tracked.
func TestCodexSessionDetailFiltersInjectedBlocksAndAttachesThinking(t *testing.T) {
	isolatedHome(t)
	a := &App{}

	path := codexSessFixture(t, codexSessFull)
	detail, err := a.GetCodexSessionDetail(path)
	if err != nil {
		t.Fatalf("GetCodexSessionDetail: %v", err)
	}
	if detail.ID != "sess-1" || detail.Cwd != "/work/repo" ||
		detail.Model != "gpt-5" || detail.ModelProvider != "openai" {
		t.Fatalf("detail meta: %+v", detail)
	}
	if detail.TokenUsage.TotalTokens != 165 {
		t.Fatalf("usage: %+v", detail.TokenUsage)
	}
	// 1 user message (injected instruction/env messages filtered out)
	// + 1 assistant message carrying thinking + text.
	if len(detail.Messages) != 2 {
		t.Fatalf("messages: %d %+v", len(detail.Messages), detail.Messages)
	}
	user := detail.Messages[0]
	if user.Role != "user" || len(user.Content) != 1 ||
		user.Content[0].Text == nil || *user.Content[0].Text != "Fix the bug" {
		t.Fatalf("user message: %+v", user)
	}
	assistant := detail.Messages[1]
	if assistant.Role != "assistant" || len(assistant.Content) != 2 {
		t.Fatalf("assistant: %+v", assistant)
	}
	if assistant.Content[0].Type != "thinking" ||
		assistant.Content[0].Thinking == nil ||
		*assistant.Content[0].Thinking != "I should check the parser." {
		t.Fatalf("thinking block: %+v", assistant.Content[0])
	}
	if assistant.Content[1].Type != "text" ||
		assistant.Content[1].Text == nil || *assistant.Content[1].Text != "Done" {
		t.Fatalf("text block: %+v", assistant.Content[1])
	}
	if detail.Title != "Fix the bug" {
		t.Fatalf("title: %q", detail.Title)
	}
}

// TestCodexSessionDetailReasoningSummaryFallback: without agent_reasoning the
// reasoning summary feeds the thinking block.
func TestCodexSessionDetailReasoningSummaryFallback(t *testing.T) {
	isolatedHome(t)
	a := &App{}

	path := codexSessFixture(t, []string{codexSessMETA, codexSessUserReal, codexSessSummary, codexSessAssistant})
	detail, err := a.GetCodexSessionDetail(path)
	if err != nil {
		t.Fatalf("GetCodexSessionDetail: %v", err)
	}
	var thinking *string
	for _, message := range detail.Messages {
		for _, block := range message.Content {
			if block.Thinking != nil {
				thinking = block.Thinking
			}
		}
	}
	if thinking == nil || *thinking != "summary text" {
		t.Fatalf("thinking: %v", thinking)
	}
}

// TestCodexSessionDetailToleratesInvalidUTF8: raw tool output that is not
// valid UTF-8 must not break listing or detail parsing.
func TestCodexSessionDetailToleratesInvalidUTF8(t *testing.T) {
	isolatedHome(t)
	a := &App{}

	path := codexSessFixture(t, []string{codexSessMETA, codexSessUserReal})
	f, err := os.OpenFile(path, os.O_APPEND|os.O_WRONLY, 0o644)
	if err != nil {
		t.Fatalf("open: %v", err)
	}
	// Raw tool output containing invalid UTF-8 bytes.
	if _, err := f.WriteString(`{"timestamp":"2026-09-08T00:00:05Z","type":"response_item","payload":{"type":"function_call_output","output":"` + "\xff\xfe" + `"}}` + "\n"); err != nil {
		t.Fatalf("write binary: %v", err)
	}
	if _, err := f.WriteString(codexSessThinking + "\n" + codexSessAssistant + "\n" + codexSessTokens + "\n"); err != nil {
		t.Fatalf("append: %v", err)
	}
	_ = f.Close()

	sessions, err := a.ListCodexSessions()
	if err != nil || len(sessions) != 1 {
		t.Fatalf("list: %v %d", err, len(sessions))
	}
	if sessions[0].Title != "Fix the bug" || sessions[0].TokenUsage.TotalTokens != 165 {
		t.Fatalf("summary: %+v", sessions[0])
	}

	detail, err := a.GetCodexSessionDetail(path)
	if err != nil {
		t.Fatalf("detail: %v", err)
	}
	if len(detail.Messages) != 2 {
		t.Fatalf("valid lines around the binary line are kept: %d", len(detail.Messages))
	}
	if detail.TokenUsage.TotalTokens != 165 {
		t.Fatalf("usage: %+v", detail.TokenUsage)
	}
}

// ---------------------------------------------------------------------------
// Providers
// ---------------------------------------------------------------------------

// TestCodexSessionProvidersAggregateAcrossProfiles: openai pinned first with
// its display name, named providers keep their name, unnamed fall back to id.
func TestCodexSessionProvidersAggregateAcrossProfiles(t *testing.T) {
	isolatedHome(t)
	a := &App{}

	openaiProfile := CodexProfile{ID: "p-openai", Name: "Official", ModelProvider: "openai", Model: "gpt-5"}
	if err := a.SaveCodexProfile(openaiProfile); err != nil {
		t.Fatalf("save openai: %v", err)
	}
	custom := codexTestCustomProfile("p-custom", "Custom")
	custom.ModelProvider = "deepseek"
	custom.Providers = map[string]CodexProviderConfig{
		"deepseek": {Name: strPtr("DeepSeek")},
		"mimo":     {Name: nil},
	}
	if err := a.SaveCodexProfile(custom); err != nil {
		t.Fatalf("save custom: %v", err)
	}

	providers, err := a.ListCodexSessionProviders()
	if err != nil {
		t.Fatalf("ListCodexSessionProviders: %v", err)
	}
	if len(providers) != 3 {
		t.Fatalf("providers: %+v", providers)
	}
	if providers[0].ID != "openai" || providers[0].Name != "OpenAI" {
		t.Fatalf("openai must be first: %+v", providers[0])
	}
	byID := map[string]string{}
	for _, provider := range providers {
		byID[provider.ID] = provider.Name
	}
	if byID["deepseek"] != "DeepSeek" {
		t.Fatalf("named provider: %+v", byID)
	}
	if byID["mimo"] != "mimo" {
		t.Fatalf("unnamed provider falls back to id: %+v", byID)
	}
}

// ---------------------------------------------------------------------------
// Provider switch
// ---------------------------------------------------------------------------

// TestCodexSessionSetProviderRewritesHeader: only the session_meta line
// changes; the rest of the file is preserved byte-for-byte, and list/detail
// see the new provider.
func TestCodexSessionSetProviderRewritesHeader(t *testing.T) {
	isolatedHome(t)
	a := &App{}

	path := codexSessFixture(t, codexSessFull)
	original, err := os.ReadFile(path)
	if err != nil {
		t.Fatalf("read: %v", err)
	}

	if err := a.SetCodexSessionProvider(path, "deepseek"); err != nil {
		t.Fatalf("SetCodexSessionProvider: %v", err)
	}

	updated, err := os.ReadFile(path)
	if err != nil {
		t.Fatalf("read updated: %v", err)
	}
	firstLine := string(updated[:strings.IndexByte(string(updated), '\n')])
	if !strings.Contains(firstLine, `"model_provider":"deepseek"`) {
		t.Fatalf("first line: %q", firstLine)
	}
	// Rest of the file preserved byte-for-byte.
	origRest := original[strings.IndexByte(string(original), '\n')+1:]
	updatedRest := updated[strings.IndexByte(string(updated), '\n')+1:]
	if string(origRest) != string(updatedRest) {
		t.Fatal("file tail changed")
	}

	sessions, err := a.ListCodexSessions()
	if err != nil || len(sessions) != 1 || sessions[0].ModelProvider != "deepseek" {
		t.Fatalf("list after switch: %v %+v", err, sessions)
	}
	detail, err := a.GetCodexSessionDetail(path)
	if err != nil || detail.ModelProvider != "deepseek" || detail.ID != "sess-1" || len(detail.Messages) != 2 {
		t.Fatalf("detail after switch: %v %+v", err, detail)
	}
}

// TestCodexSessionSetProviderRejectsInvalidTargets: paths outside the
// sessions dir and files without a session_meta header are refused; the
// original file stays untouched.
func TestCodexSessionSetProviderRejectsInvalidTargets(t *testing.T) {
	isolatedHome(t)
	a := &App{}

	// Fixture creates the sessions dir; the target under test is `outside`.
	codexSessFixture(t, codexSessFull)
	home, err := coreHomeForTest()
	if err != nil {
		t.Fatalf("home: %v", err)
	}
	outside := filepath.Join(home, ".codex", "config.toml")
	if err := os.WriteFile(outside, []byte("x"), 0o644); err != nil {
		t.Fatalf("write outside: %v", err)
	}

	err = a.SetCodexSessionProvider(outside, "deepseek")
	if err == nil || !strings.Contains(err.Error(), "not found") {
		t.Fatalf("outside path: %v", err)
	}
	data, _ := os.ReadFile(outside)
	if string(data) != "x" {
		t.Fatalf("outside file modified: %q", data)
	}

	// First line is not session_meta.
	badPath := codexSessFixtureWithID(t, "rollout-bad", []string{codexSessTURN})
	err = a.SetCodexSessionProvider(badPath, "deepseek")
	if err == nil || !strings.Contains(err.Error(), "session_meta") {
		t.Fatalf("non-meta header: %v", err)
	}
}

// TestCodexSessionSetProviderUpdatesThreadIndex: the codex state database's
// threads table follows the switch so `codex resume` regroups the session.
func TestCodexSessionSetProviderUpdatesThreadIndex(t *testing.T) {
	isolatedHome(t)
	a := &App{}

	path := codexSessFixture(t, codexSessFull)
	home, err := coreHomeForTest()
	if err != nil {
		t.Fatalf("home: %v", err)
	}
	stateDB := filepath.Join(home, ".codex", "state_5.sqlite")
	codexTestCreateThreadsDB(t, stateDB, "openai")

	if err := a.SetCodexSessionProvider(path, "deepseek"); err != nil {
		t.Fatalf("SetCodexSessionProvider: %v", err)
	}

	db, err := sql.Open("sqlite", stateDB)
	if err != nil {
		t.Fatalf("open db: %v", err)
	}
	defer func() { _ = db.Close() }()
	var provider string
	if err := db.QueryRow("SELECT model_provider FROM threads WHERE id = 'sess-1'").Scan(&provider); err != nil {
		t.Fatalf("query: %v", err)
	}
	if provider != "deepseek" {
		t.Fatalf("thread provider: %q", provider)
	}
}

// TestCodexSessionSetProviderIgnoresDBWithoutThreads: a state db without a
// threads table (older Codex) is skipped without failing the switch.
func TestCodexSessionSetProviderIgnoresDBWithoutThreads(t *testing.T) {
	isolatedHome(t)
	a := &App{}

	path := codexSessFixture(t, codexSessFull)
	home, err := coreHomeForTest()
	if err != nil {
		t.Fatalf("home: %v", err)
	}
	stateDB := filepath.Join(home, ".codex", "state_5.sqlite")
	db, err := sql.Open("sqlite", stateDB)
	if err != nil {
		t.Fatalf("open: %v", err)
	}
	if _, err := db.Exec("CREATE TABLE other (id TEXT)"); err != nil {
		t.Fatalf("create: %v", err)
	}
	_ = db.Close()

	if err := a.SetCodexSessionProvider(path, "deepseek"); err != nil {
		t.Fatalf("SetCodexSessionProvider: %v", err)
	}
	sessions, err := a.ListCodexSessions()
	if err != nil || len(sessions) != 1 || sessions[0].ModelProvider != "deepseek" {
		t.Fatalf("list: %v %+v", err, sessions)
	}
}

// codexTestCreateThreadsDB creates a state db with a populated threads table.
func codexTestCreateThreadsDB(t *testing.T, path string, provider string) {
	t.Helper()
	db, err := sql.Open("sqlite", path)
	if err != nil {
		t.Fatalf("open: %v", err)
	}
	defer func() { _ = db.Close() }()
	if _, err := db.Exec("CREATE TABLE threads (id TEXT PRIMARY KEY, model_provider TEXT NOT NULL)"); err != nil {
		t.Fatalf("create table: %v", err)
	}
	if _, err := db.Exec("INSERT INTO threads (id, model_provider) VALUES ('sess-1', ?)", provider); err != nil {
		t.Fatalf("insert: %v", err)
	}
}

// ---------------------------------------------------------------------------
// Delete
// ---------------------------------------------------------------------------

// TestCodexSessionDeletePrunesDateDirs: removing the last session prunes the
// empty YYYY/MM/DD directories up to (but excluding) the sessions dir.
func TestCodexSessionDeletePrunesDateDirs(t *testing.T) {
	isolatedHome(t)
	a := &App{}

	path := codexSessFixture(t, codexSessFull)
	home, err := coreHomeForTest()
	if err != nil {
		t.Fatalf("home: %v", err)
	}
	sessionsDir := filepath.Join(home, ".codex", "sessions")

	if err := a.DeleteCodexSession(path); err != nil {
		t.Fatalf("DeleteCodexSession: %v", err)
	}
	for _, dir := range []string{
		filepath.Join(sessionsDir, "2026", "09", "08"),
		filepath.Join(sessionsDir, "2026", "09"),
		filepath.Join(sessionsDir, "2026"),
	} {
		if _, err := os.Stat(dir); !os.IsNotExist(err) {
			t.Fatalf("%s must be pruned, err=%v", dir, err)
		}
	}
	if _, err := os.Stat(sessionsDir); err != nil {
		t.Fatalf("sessions dir must stay: %v", err)
	}
}

// TestCodexSessionDeleteRejectsPathsOutside: only files inside the sessions
// directory can be deleted.
func TestCodexSessionDeleteRejectsPathsOutside(t *testing.T) {
	isolatedHome(t)
	a := &App{}

	codexSessFixture(t, codexSessFull)
	home, err := coreHomeForTest()
	if err != nil {
		t.Fatalf("home: %v", err)
	}
	outside := filepath.Join(home, ".codex", "config.toml")
	if err := os.WriteFile(outside, []byte("x"), 0o644); err != nil {
		t.Fatalf("write: %v", err)
	}

	err = a.DeleteCodexSession(outside)
	if err == nil || !strings.Contains(err.Error(), "not found") {
		t.Fatalf("outside delete: %v", err)
	}
	if _, statErr := os.Stat(outside); statErr != nil {
		t.Fatalf("outside file must survive: %v", statErr)
	}
}

// ---------------------------------------------------------------------------
// Watcher
// ---------------------------------------------------------------------------

// TestCodexSessionsWatcherEmitsOnChange covers the poll-based mirror of the
// Rust notify watcher: start → change → `codex-sessions-changed`; stop ends
// the stream; the baseline state emits nothing.
func TestCodexSessionsWatcherEmitsOnChange(t *testing.T) {
	isolatedHome(t)
	a := &App{}

	// Missing sessions dir → start is a no-op (Rust parity), stop is safe.
	if err := a.StartCodexSessionsWatcher(); err != nil {
		t.Fatalf("start (no dir): %v", err)
	}
	if err := a.StopCodexSessionsWatcher(); err != nil {
		t.Fatalf("stop (no dir): %v", err)
	}

	path := codexSessFixture(t, codexSessFull)

	var mu sync.Mutex
	events := []string{}
	codexEventSink = func(name string) {
		mu.Lock()
		defer mu.Unlock()
		events = append(events, name)
	}
	defer func() { codexEventSink = nil }()

	if err := a.StartCodexSessionsWatcher(); err != nil {
		t.Fatalf("start: %v", err)
	}
	// Let the baseline signature settle; the pre-existing state must not emit.
	time.Sleep(1200 * time.Millisecond)
	mu.Lock()
	if len(events) != 0 {
		mu.Unlock()
		t.Fatalf("baseline must not emit: %+v", events)
	}
	mu.Unlock()

	// A modification triggers exactly the change event.
	if err := os.WriteFile(path, []byte(strings.Join(append(codexSessFull, codexSessTokens), "\n")+"\n"), 0o644); err != nil {
		t.Fatalf("modify: %v", err)
	}
	deadline := time.After(5 * time.Second)
	for {
		mu.Lock()
		got := len(events) > 0
		mu.Unlock()
		if got {
			break
		}
		select {
		case <-deadline:
			t.Fatal("no codex-sessions-changed event in time")
		case <-time.After(50 * time.Millisecond):
		}
	}
	mu.Lock()
	first := events[0]
	mu.Unlock()
	if first != "codex-sessions-changed" {
		t.Fatalf("event name: %q", first)
	}

	if err := a.StopCodexSessionsWatcher(); err != nil {
		t.Fatalf("stop: %v", err)
	}
	mu.Lock()
	before := len(events)
	mu.Unlock()

	// After stop, further changes emit nothing.
	time.Sleep(300 * time.Millisecond)
	if err := os.Remove(path); err != nil {
		t.Fatalf("remove: %v", err)
	}
	time.Sleep(1200 * time.Millisecond)
	mu.Lock()
	defer mu.Unlock()
	if len(events) != before {
		t.Fatalf("events after stop: %+v", events)
	}
}

// TestCodexSessionsWatcherRestartReplacesLoop: starting twice must not stack
// poll loops (the Rust command replaces the watcher).
func TestCodexSessionsWatcherRestartReplacesLoop(t *testing.T) {
	isolatedHome(t)
	a := &App{}

	path := codexSessFixture(t, codexSessFull)

	var mu sync.Mutex
	count := 0
	codexEventSink = func(name string) {
		mu.Lock()
		defer mu.Unlock()
		count++
	}
	defer func() { codexEventSink = nil }()

	if err := a.StartCodexSessionsWatcher(); err != nil {
		t.Fatalf("start 1: %v", err)
	}
	if err := a.StartCodexSessionsWatcher(); err != nil {
		t.Fatalf("start 2: %v", err)
	}
	time.Sleep(1200 * time.Millisecond)

	if err := os.WriteFile(path, []byte(codexSessMETA+"\n"), 0o644); err != nil {
		t.Fatalf("modify: %v", err)
	}
	deadline := time.After(5 * time.Second)
	for {
		mu.Lock()
		got := count
		mu.Unlock()
		if got > 0 {
			break
		}
		select {
		case <-deadline:
			t.Fatal("no event after restart")
		case <-time.After(50 * time.Millisecond):
		}
	}
	if err := a.StopCodexSessionsWatcher(); err != nil {
		t.Fatalf("stop: %v", err)
	}

	// Exactly one loop may survive the restart: stop must silence everything.
	mu.Lock()
	before := count
	mu.Unlock()
	time.Sleep(300 * time.Millisecond)
	if err := os.WriteFile(path, []byte(codexSessMETA+"\n"+codexSessTURN+"\n"), 0o644); err != nil {
		t.Fatalf("modify 2: %v", err)
	}
	time.Sleep(1200 * time.Millisecond)
	mu.Lock()
	defer mu.Unlock()
	if count != before {
		t.Fatalf("duplicate watcher still running: %d -> %d", before, count)
	}
}

// coreHomeForTest resolves the isolated home the tests run against.
func coreHomeForTest() (string, error) {
	return core.HomeDir()
}
