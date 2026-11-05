package read

import (
	"database/sql"
	"droidgear-wails/internal/service/codex"
	"droidgear-wails/internal/testutil"
	_ "modernc.org/sqlite" // registers the pure-Go driver used by the thread-index fixtures
	"os"
	"path/filepath"
	"strings"
	"testing"

	"droidgear-wails/internal/core"
)

func TestCodexSessionListReadsSummaryAndIndexTitle(t *testing.T) {
	testutil.IsolatedHome(t)

	// Missing sessions dir → empty list, no error.
	sessions, err := ListCodexSessions()
	if err != nil || len(sessions) != 0 {
		t.Fatalf("empty list: %v %d", err, len(sessions))
	}

	path := testutil.CodexSessFixture(t, testutil.CodexSessFull)
	testutil.CodexSessWithIndex(t, "sess-1", "Fix the bug")

	sessions, err = ListCodexSessions()
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
	testutil.IsolatedHome(t)

	testutil.CodexSessFixture(t, testutil.CodexSessFull)
	sessions, err := ListCodexSessions()
	if err != nil || len(sessions) != 1 {
		t.Fatalf("list: %v %d", err, len(sessions))
	}
	// Injected <user_instructions>/<environment_context> blocks are skipped —
	// the title comes from "Fix the bug".
	if sessions[0].Title != "Fix the bug" {
		t.Fatalf("title: %q", sessions[0].Title)
	}

	testutil.CodexSessFixtureWithID(t, "rollout-bare", []string{testutil.CodexSessMETA, testutil.CodexSessTURN})
	sessions, err = ListCodexSessions()
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

// TestCodexSessionListTokenUsageFromTailWindow pads a file past the tail
// window so the final token_count event is only visible via the tail read.
func TestCodexSessionListTokenUsageFromTailWindow(t *testing.T) {
	testutil.IsolatedHome(t)

	path := testutil.CodexSessFixture(t, []string{testutil.CodexSessMETA})
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
	if _, err := f.WriteString(testutil.CodexSessTokens + "\n"); err != nil {
		t.Fatalf("append tokens: %v", err)
	}
	_ = f.Close()

	sessions, err := ListCodexSessions()
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
	testutil.IsolatedHome(t)

	path := testutil.CodexSessFixture(t, testutil.CodexSessFull)
	detail, err := GetCodexSessionDetail(path)
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
	testutil.IsolatedHome(t)

	path := testutil.CodexSessFixture(t, []string{testutil.CodexSessMETA, testutil.CodexSessUserReal, testutil.CodexSessSummary, testutil.CodexSessAssistant})
	detail, err := GetCodexSessionDetail(path)
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
	testutil.IsolatedHome(t)

	path := testutil.CodexSessFixture(t, []string{testutil.CodexSessMETA, testutil.CodexSessUserReal})
	f, err := os.OpenFile(path, os.O_APPEND|os.O_WRONLY, 0o644)
	if err != nil {
		t.Fatalf("open: %v", err)
	}
	// Raw tool output containing invalid UTF-8 bytes.
	if _, err := f.WriteString(`{"timestamp":"2026-09-08T00:00:05Z","type":"response_item","payload":{"type":"function_call_output","output":"` + "\xff\xfe" + `"}}` + "\n"); err != nil {
		t.Fatalf("write binary: %v", err)
	}
	if _, err := f.WriteString(testutil.CodexSessThinking + "\n" + testutil.CodexSessAssistant + "\n" + testutil.CodexSessTokens + "\n"); err != nil {
		t.Fatalf("append: %v", err)
	}
	_ = f.Close()

	sessions, err := ListCodexSessions()
	if err != nil || len(sessions) != 1 {
		t.Fatalf("list: %v %d", err, len(sessions))
	}
	if sessions[0].Title != "Fix the bug" || sessions[0].TokenUsage.TotalTokens != 165 {
		t.Fatalf("summary: %+v", sessions[0])
	}

	detail, err := GetCodexSessionDetail(path)
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
	testutil.IsolatedHome(t)

	openaiProfile := codex.CodexProfile{ID: "p-openai", Name: "Official", ModelProvider: "openai", Model: "gpt-5"}
	if err := codex.SaveProfile(openaiProfile); err != nil {
		t.Fatalf("save openai: %v", err)
	}
	// Built inline: the shared fixture lives in service/codex's tests, and a
	// helper package importing it would be an import cycle in tests.
	custom := codex.CodexProfile{
		ID:                   "p-custom",
		Name:                 "Custom",
		APIKey:               core.StrPtr("sk-provider"),
		ModelProvider:        "deepseek",
		Model:                "gpt-5.5",
		ModelReasoningEffort: core.StrPtr("medium"),
		APIKeyModelDiscovery: true,
		Providers: map[string]codex.CodexProviderConfig{
			"deepseek": {Name: core.StrPtr("DeepSeek")},
			"mimo":     {Name: nil},
		},
	}
	if err := codex.SaveProfile(custom); err != nil {
		t.Fatalf("save custom: %v", err)
	}

	providers, err := ListCodexSessionProviders()
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
	testutil.IsolatedHome(t)

	path := testutil.CodexSessFixture(t, testutil.CodexSessFull)
	original, err := os.ReadFile(path)
	if err != nil {
		t.Fatalf("read: %v", err)
	}

	if err := SetCodexSessionProvider(path, "deepseek"); err != nil {
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

	sessions, err := ListCodexSessions()
	if err != nil || len(sessions) != 1 || sessions[0].ModelProvider != "deepseek" {
		t.Fatalf("list after switch: %v %+v", err, sessions)
	}
	detail, err := GetCodexSessionDetail(path)
	if err != nil || detail.ModelProvider != "deepseek" || detail.ID != "sess-1" || len(detail.Messages) != 2 {
		t.Fatalf("detail after switch: %v %+v", err, detail)
	}
}

// TestCodexSessionSetProviderRejectsInvalidTargets: paths outside the
// sessions dir and files without a session_meta header are refused; the
// original file stays untouched.
func TestCodexSessionSetProviderRejectsInvalidTargets(t *testing.T) {
	testutil.IsolatedHome(t)

	// Fixture creates the sessions dir; the target under test is `outside`.
	testutil.CodexSessFixture(t, testutil.CodexSessFull)
	home, err := testutil.CoreHomeForTest()
	if err != nil {
		t.Fatalf("home: %v", err)
	}
	outside := filepath.Join(home, ".codex", "config.toml")
	if err := os.WriteFile(outside, []byte("x"), 0o644); err != nil {
		t.Fatalf("write outside: %v", err)
	}

	err = SetCodexSessionProvider(outside, "deepseek")
	if err == nil || !strings.Contains(err.Error(), "not found") {
		t.Fatalf("outside path: %v", err)
	}
	data, _ := os.ReadFile(outside)
	if string(data) != "x" {
		t.Fatalf("outside file modified: %q", data)
	}

	// First line is not session_meta.
	badPath := testutil.CodexSessFixtureWithID(t, "rollout-bad", []string{testutil.CodexSessTURN})
	err = SetCodexSessionProvider(badPath, "deepseek")
	if err == nil || !strings.Contains(err.Error(), "session_meta") {
		t.Fatalf("non-meta header: %v", err)
	}
}

// TestCodexSessionSetProviderUpdatesThreadIndex: the codex state database's
// threads table follows the switch so `codex resume` regroups the session.
func TestCodexSessionSetProviderUpdatesThreadIndex(t *testing.T) {
	testutil.IsolatedHome(t)

	path := testutil.CodexSessFixture(t, testutil.CodexSessFull)
	home, err := testutil.CoreHomeForTest()
	if err != nil {
		t.Fatalf("home: %v", err)
	}
	stateDB := filepath.Join(home, ".codex", "state_5.sqlite")
	codexTestCreateThreadsDB(t, stateDB, "openai")

	if err := SetCodexSessionProvider(path, "deepseek"); err != nil {
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
	testutil.IsolatedHome(t)

	path := testutil.CodexSessFixture(t, testutil.CodexSessFull)
	home, err := testutil.CoreHomeForTest()
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

	if err := SetCodexSessionProvider(path, "deepseek"); err != nil {
		t.Fatalf("SetCodexSessionProvider: %v", err)
	}
	sessions, err := ListCodexSessions()
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
	testutil.IsolatedHome(t)

	path := testutil.CodexSessFixture(t, testutil.CodexSessFull)
	home, err := testutil.CoreHomeForTest()
	if err != nil {
		t.Fatalf("home: %v", err)
	}
	sessionsDir := filepath.Join(home, ".codex", "sessions")

	if err := DeleteCodexSession(path); err != nil {
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
	testutil.IsolatedHome(t)

	testutil.CodexSessFixture(t, testutil.CodexSessFull)
	home, err := testutil.CoreHomeForTest()
	if err != nil {
		t.Fatalf("home: %v", err)
	}
	outside := filepath.Join(home, ".codex", "config.toml")
	if err := os.WriteFile(outside, []byte("x"), 0o644); err != nil {
		t.Fatalf("write: %v", err)
	}

	err = DeleteCodexSession(outside)
	if err == nil || !strings.Contains(err.Error(), "not found") {
		t.Fatalf("outside delete: %v", err)
	}
	if _, statErr := os.Stat(outside); statErr != nil {
		t.Fatalf("outside file must survive: %v", statErr)
	}
}

// ---------------------------------------------------------------------------
