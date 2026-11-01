// Codex sessions viewer: reads rollout files from <codex home>/sessions,
// aggregates providers from all profiles, rewrites session_meta on provider
// switch (keeping the state_*.sqlite thread index in sync), deletes sessions,
// and poll-watches the sessions directory, emitting `codex-sessions-changed`
// exactly like the Rust notify watcher. Mirrors droidgear-core::
// codex_sessions (logic) and src-tauri/src/commands/codex_sessions.rs
// (watcher); the frontend contract is src/components/codex/CodexSessionsPage.tsx.
package app

import (
	"bytes"
	"database/sql"
	"encoding/json"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"sort"
	"strconv"
	"strings"
	"sync"
	"time"
	"unicode"

	"droidgear-wails/internal/core"

	"github.com/wailsapp/wails/v3/pkg/application"
)

// ---------------------------------------------------------------------------
// Types (camelCase JSON, matching the specta-generated TS types)
// ---------------------------------------------------------------------------

// CodexTokenUsage mirrors codex_sessions::CodexTokenUsage (from the last
// event_msg/token_count event).
type CodexTokenUsage struct {
	InputTokens         float64 `json:"inputTokens"`
	OutputTokens        float64 `json:"outputTokens"`
	CacheCreationTokens float64 `json:"cacheCreationTokens"`
	CacheReadTokens     float64 `json:"cacheReadTokens"`
	ReasoningTokens     float64 `json:"reasoningTokens"`
	TotalTokens         float64 `json:"totalTokens"`
}

// CodexSessionProvider mirrors codex_sessions::CodexSessionProvider.
type CodexSessionProvider struct {
	ID   string `json:"id"`
	Name string `json:"name"`
}

// CodexSessionSummary mirrors codex_sessions::CodexSessionSummary.
type CodexSessionSummary struct {
	ID            string          `json:"id"`
	Title         string          `json:"title"`
	Project       string          `json:"project"`
	Model         string          `json:"model"`
	ModelProvider string          `json:"modelProvider"`
	ModifiedAt    float64         `json:"modifiedAt"`
	TokenUsage    CodexTokenUsage `json:"tokenUsage"`
	Path          string          `json:"path"`
}

// CodexContentBlock mirrors codex_sessions::CodexContentBlock (`type` is the
// serde rename of content_type).
type CodexContentBlock struct {
	Type     string  `json:"type"`
	Text     *string `json:"text,omitempty"`
	Thinking *string `json:"thinking,omitempty"`
}

// CodexSessionMessage mirrors codex_sessions::CodexSessionMessage.
type CodexSessionMessage struct {
	ID        string              `json:"id"`
	Role      string              `json:"role"`
	Content   []CodexContentBlock `json:"content"`
	Timestamp string              `json:"timestamp"`
}

// CodexSessionDetail mirrors codex_sessions::CodexSessionDetail.
type CodexSessionDetail struct {
	ID            string                `json:"id"`
	Title         string                `json:"title"`
	Cwd           string                `json:"cwd"`
	Model         string                `json:"model"`
	ModelProvider string                `json:"modelProvider"`
	ModifiedAt    float64               `json:"modifiedAt"`
	TokenUsage    CodexTokenUsage       `json:"tokenUsage"`
	Messages      []CodexSessionMessage `json:"messages"`
}

// ---------------------------------------------------------------------------
// Windowing constants (mirrors HEAD_BYTES / TAIL_BYTES / UNTITLED)
// ---------------------------------------------------------------------------

const (
	codexSessionHeadBytes      = 256 * 1024
	codexSessionTailBytes      = 128 * 1024
	codexSessionUntitled       = "New Session"
	codexSessionEventName      = "codex-sessions-changed"
	codexSessionsFallbackEvery = 10 * time.Second
)

// ---------------------------------------------------------------------------
// Path helpers
// ---------------------------------------------------------------------------

// codexSessionsDir returns <codex home>/sessions (no side effects).
func codexSessionsDir() (string, error) {
	dir, err := codexConfigDirNoCreate()
	if err != nil {
		return "", err
	}
	return filepath.Join(dir, "sessions"), nil
}

// codexSessionWithin reports whether path is inside dir (component-wise,
// like Rust Path::starts_with) and exists.
func codexSessionWithin(dir string, path string) bool {
	rel, err := filepath.Rel(dir, path)
	if err != nil {
		return false
	}
	if rel == ".." || strings.HasPrefix(rel, ".."+string(filepath.Separator)) {
		return false
	}
	return codexPathExists(path)
}

// ---------------------------------------------------------------------------
// Line windows + helpers
// ---------------------------------------------------------------------------

// codexSplitLines mimics Rust str::lines(): split on \n, drop one trailing
// \r, no trailing empty element after a final newline.
func codexSplitLines(s string) []string {
	if s == "" {
		return []string{}
	}
	raw := strings.Split(s, "\n")
	if raw[len(raw)-1] == "" {
		raw = raw[:len(raw)-1]
	}
	lines := make([]string, 0, len(raw))
	for _, line := range raw {
		lines = append(lines, strings.TrimSuffix(line, "\r"))
	}
	return lines
}

// codexReadPrefixLines reads up to maxBytes from the start of the file,
// keeping whole lines (a trailing partial line is dropped when truncated).
// Decoding is lossy — session files may carry raw non-UTF-8 tool output.
func codexReadPrefixLines(path string, maxBytes int64) ([]string, error) {
	f, err := os.Open(path) // #nosec G304 -- session file path
	if err != nil {
		return nil, fmt.Errorf("Failed to open %q: %w", path, err)
	}
	defer func() { _ = f.Close() }()
	buf, err := io.ReadAll(io.LimitReader(f, maxBytes))
	if err != nil {
		return nil, fmt.Errorf("Failed to read %q: %w", path, err)
	}
	if int64(len(buf)) == maxBytes {
		if pos := bytes.LastIndexByte(buf, '\n'); pos >= 0 {
			buf = buf[:pos+1]
		}
	}
	return codexSplitLines(string(buf)), nil
}

// codexReadTailLines reads up to maxBytes from the end of the file, dropping
// the leading partial line. The second result is false when the whole file
// already fits in the head window.
func codexReadTailLines(path string, maxBytes int64) ([]string, bool, error) {
	f, err := os.Open(path) // #nosec G304 -- session file path
	if err != nil {
		return nil, false, fmt.Errorf("Failed to open %q: %w", path, err)
	}
	defer func() { _ = f.Close() }()
	info, err := f.Stat()
	if err != nil {
		return nil, false, fmt.Errorf("Failed to stat %q: %w", path, err)
	}
	if info.Size() <= maxBytes {
		return nil, false, nil
	}
	if _, err := f.Seek(info.Size()-maxBytes, io.SeekStart); err != nil {
		return nil, false, fmt.Errorf("Failed to seek %q: %w", path, err)
	}
	buf, err := io.ReadAll(io.LimitReader(f, maxBytes))
	if err != nil {
		return nil, false, fmt.Errorf("Failed to read %q: %w", path, err)
	}
	lines := codexSplitLines(string(buf))
	if len(lines) > 0 {
		lines = lines[1:] // first segment starts mid-line
	}
	return lines, true, nil
}

// codexModifiedAtMs is the file mtime in milliseconds (0 when unavailable).
func codexModifiedAtMs(path string) float64 {
	info, err := os.Stat(path) // #nosec G304 -- session file path
	if err != nil {
		return 0
	}
	return float64(info.ModTime().UnixMilli())
}

// codexIsHiddenUserBlock skips injected user-message blocks.
func codexIsHiddenUserBlock(text string) bool {
	trimmed := strings.TrimLeftFunc(text, unicode.IsSpace)
	return strings.HasPrefix(trimmed, "<user_instructions>") || strings.HasPrefix(trimmed, "<environment_context>")
}

// codexExtractMessageBlocks keeps input_text/output_text blocks; injected
// instruction/environment blocks are skipped for user messages.
func codexExtractMessageBlocks(content any, role string) []CodexContentBlock {
	blocks := []CodexContentBlock{}
	items, ok := content.([]any)
	if !ok {
		return blocks
	}
	for _, item := range items {
		obj, ok := item.(map[string]any)
		if !ok {
			continue
		}
		blockType, _ := obj["type"].(string)
		if blockType != "input_text" && blockType != "output_text" {
			continue
		}
		text, ok := obj["text"].(string)
		if !ok {
			continue
		}
		if role == "user" && codexIsHiddenUserBlock(text) {
			continue
		}
		blocks = append(blocks, CodexContentBlock{Type: "text", Text: &text})
	}
	return blocks
}

// codexTitleFromText collapses whitespace and truncates to 60 characters.
func codexTitleFromText(text string) string {
	title := strings.Join(strings.Fields(text), " ")
	runes := []rune(title)
	if len(runes) > 60 {
		title = string(runes[:60]) + "…"
	}
	return title
}

// codexFirstUserText finds the first displayable user-message text (title
// fallback when the session index has no entry).
func codexFirstUserText(lines []string) *string {
	for _, line := range lines {
		var jsonValue map[string]any
		if err := json.Unmarshal([]byte(line), &jsonValue); err != nil {
			continue
		}
		if typ, _ := jsonValue["type"].(string); typ != "response_item" {
			continue
		}
		payload, _ := jsonValue["payload"].(map[string]any)
		if payload == nil {
			continue
		}
		if typ, _ := payload["type"].(string); typ != "message" {
			continue
		}
		if role, _ := payload["role"].(string); role != "user" {
			continue
		}
		blocks := codexExtractMessageBlocks(payload["content"], "user")
		if len(blocks) > 0 && blocks[0].Text != nil {
			return blocks[0].Text
		}
	}
	return nil
}

// codexParseTokenUsage maps the token_count info object onto the JSON model.
func codexParseTokenUsage(info map[string]any) CodexTokenUsage {
	num := func(key string) float64 {
		if f, ok := info[key].(float64); ok {
			return f
		}
		return 0
	}
	return CodexTokenUsage{
		InputTokens:         num("input_tokens"),
		OutputTokens:        num("output_tokens"),
		CacheCreationTokens: num("cache_write_input_tokens"),
		CacheReadTokens:     num("cached_input_tokens"),
		ReasoningTokens:     num("reasoning_output_tokens"),
		TotalTokens:         num("total_tokens"),
	}
}

// codexLastTokenUsage returns the last cumulative token usage in lines.
func codexLastTokenUsage(lines []string) CodexTokenUsage {
	usage := CodexTokenUsage{}
	for _, line := range lines {
		var jsonValue map[string]any
		if err := json.Unmarshal([]byte(line), &jsonValue); err != nil {
			continue
		}
		if typ, _ := jsonValue["type"].(string); typ != "event_msg" {
			continue
		}
		payload, _ := jsonValue["payload"].(map[string]any)
		if payload == nil {
			continue
		}
		if typ, _ := payload["type"].(string); typ != "token_count" {
			continue
		}
		info, _ := payload["info"].(map[string]any)
		if info == nil {
			continue
		}
		if total, ok := info["total_token_usage"].(map[string]any); ok && len(total) > 0 {
			usage = codexParseTokenUsage(total)
		}
	}
	return usage
}

// codexScanHeadMeta extracts the first session id / cwd / model / provider
// seen in the head window.
func codexScanHeadMeta(lines []string) (id string, cwd string, model string, modelProvider string) {
	for _, line := range lines {
		var jsonValue map[string]any
		if err := json.Unmarshal([]byte(line), &jsonValue); err != nil {
			continue
		}
		payload, _ := jsonValue["payload"].(map[string]any)
		if payload == nil {
			payload = map[string]any{}
		}
		switch typ, _ := jsonValue["type"].(string); typ {
		case "session_meta":
			if id == "" {
				id, _ = payload["id"].(string)
			}
			if cwd == "" {
				cwd, _ = payload["cwd"].(string)
			}
			if modelProvider == "" {
				modelProvider, _ = payload["model_provider"].(string)
			}
		case "turn_context":
			if model == "" {
				model, _ = payload["model"].(string)
			}
		}
	}
	return id, cwd, model, modelProvider
}

// codexCollectJSONL gathers all *.jsonl session files below dir (recursive).
func codexCollectJSONL(dir string, out *[]string) {
	entries, err := os.ReadDir(dir) // #nosec G304 -- sessions dir
	if err != nil {
		return
	}
	for _, entry := range entries {
		path := filepath.Join(dir, entry.Name())
		if entry.IsDir() {
			codexCollectJSONL(path, out)
		} else if strings.HasSuffix(entry.Name(), ".jsonl") {
			*out = append(*out, path)
		}
	}
}

// codexReadSessionIndexTitles reads <codex home>/session_index.jsonl; the
// newest entry per id wins (file append order).
func codexReadSessionIndexTitles(codexHome string) map[string]string {
	titles := map[string]string{}
	data, err := os.ReadFile(filepath.Join(codexHome, "session_index.jsonl")) // #nosec G304 -- codex home file
	if err != nil {
		return titles
	}
	for _, line := range codexSplitLines(string(data)) {
		var entry map[string]any
		if err := json.Unmarshal([]byte(line), &entry); err != nil {
			continue
		}
		id, ok := entry["id"].(string)
		if !ok {
			continue
		}
		if name, ok := entry["thread_name"].(string); ok {
			if trimmed := strings.TrimSpace(name); trimmed != "" {
				titles[id] = trimmed
			}
		}
	}
	return titles
}

// ---------------------------------------------------------------------------
// Listing
// ---------------------------------------------------------------------------

// ListCodexSessions lists all sessions (newest first) from
// <codex home>/sessions.
func (a *App) ListCodexSessions() ([]CodexSessionSummary, error) {
	sessionsDir, err := codexSessionsDir()
	if err != nil {
		return nil, err
	}
	if !core.DirExists(sessionsDir) {
		return []CodexSessionSummary{}, nil
	}
	codexHome, err := codexConfigDirNoCreate()
	if err != nil {
		return nil, err
	}
	titles := codexReadSessionIndexTitles(codexHome)

	files := []string{}
	codexCollectJSONL(sessionsDir, &files)

	sessions := []CodexSessionSummary{}
	for _, path := range files {
		head, err := codexReadPrefixLines(path, codexSessionHeadBytes)
		if err != nil {
			return nil, err
		}
		id, cwd, model, modelProvider := codexScanHeadMeta(head)
		if id == "" {
			base := filepath.Base(path)
			id = strings.TrimSuffix(base, filepath.Ext(base))
		}
		title, ok := titles[id]
		if !ok {
			if first := codexFirstUserText(head); first != nil {
				title = codexTitleFromText(*first)
			} else {
				title = codexSessionUntitled
			}
		}
		tail, hasTail, err := codexReadTailLines(path, codexSessionTailBytes)
		if err != nil {
			return nil, err
		}
		window := head
		if hasTail {
			window = tail
		}
		if model == "" {
			model = "unknown"
		}
		sessions = append(sessions, CodexSessionSummary{
			ID:            id,
			Title:         title,
			Project:       cwd,
			Model:         model,
			ModelProvider: modelProvider,
			ModifiedAt:    codexModifiedAtMs(path),
			TokenUsage:    codexLastTokenUsage(window),
			Path:          path,
		})
	}
	sort.SliceStable(sessions, func(i, j int) bool {
		return sessions[i].ModifiedAt > sessions[j].ModifiedAt
	})
	return sessions, nil
}

// ---------------------------------------------------------------------------
// Providers
// ---------------------------------------------------------------------------

// codexProviderDisplayName: `openai` is always "OpenAI"; otherwise the
// provider config's name, falling back to the id.
func codexProviderDisplayName(id string, name *string) string {
	if id == "openai" {
		return "OpenAI"
	}
	if name != nil {
		if trimmed := strings.TrimSpace(*name); trimmed != "" {
			return trimmed
		}
	}
	return id
}

// ListCodexSessionProviders aggregates every provider id across all profiles:
// each profile contributes its model_provider plus its providers map;
// openai lists first, then everything sorted by lowercased display name.
func (a *App) ListCodexSessionProviders() ([]CodexSessionProvider, error) {
	profiles, err := codexListProfiles()
	if err != nil {
		return nil, err
	}
	// id → best display name (a named entry upgrades a bare id-only entry).
	names := map[string]*string{}
	add := func(id string, name *string) {
		id = strings.TrimSpace(id)
		if id == "" {
			return
		}
		var cleaned *string
		if name != nil {
			if trimmed := strings.TrimSpace(*name); trimmed != "" {
				cleaned = &trimmed
			}
		}
		existing, present := names[id]
		if !present || (existing == nil && cleaned != nil) {
			names[id] = cleaned
		}
	}
	for _, profile := range profiles {
		add(profile.ModelProvider, nil)
		for id, config := range profile.Providers {
			add(id, config.Name)
		}
	}
	providers := make([]CodexSessionProvider, 0, len(names))
	for id, name := range names {
		providers = append(providers, CodexSessionProvider{
			ID:   id,
			Name: codexProviderDisplayName(id, name),
		})
	}
	sort.SliceStable(providers, func(i, j int) bool {
		if providers[i].ID == "openai" {
			return true
		}
		if providers[j].ID == "openai" {
			return false
		}
		return strings.ToLower(providers[i].Name) < strings.ToLower(providers[j].Name)
	})
	return providers, nil
}

// ---------------------------------------------------------------------------
// Session detail
// ---------------------------------------------------------------------------

// codexTakePendingThinking prefers the plain agent_reasoning stream over
// reasoning summaries, joining with a blank line; clears both buffers.
func codexTakePendingThinking(pending *[]string, summary *[]string) *string {
	var chosen []string
	if len(*pending) > 0 {
		chosen = *pending
	} else {
		chosen = *summary
	}
	*pending = []string{}
	*summary = []string{}
	if len(chosen) == 0 {
		return nil
	}
	joined := strings.Join(chosen, "\n\n")
	return &joined
}

// codexReasoningSummaryTexts collects summary_text blocks from a reasoning
// payload (summary or content array).
func codexReasoningSummaryTexts(payload map[string]any) []string {
	texts := []string{}
	items, ok := payload["summary"].([]any)
	if !ok {
		items, ok = payload["content"].([]any)
	}
	if !ok {
		return texts
	}
	for _, item := range items {
		obj, ok := item.(map[string]any)
		if !ok {
			continue
		}
		if typ, _ := obj["type"].(string); typ != "summary_text" {
			continue
		}
		if text, ok := obj["text"].(string); ok {
			texts = append(texts, text)
		}
	}
	return texts
}

// GetCodexSessionDetail parses one rollout file into chat messages, token
// usage and title (first real user message, truncated).
func (a *App) GetCodexSessionDetail(sessionPath string) (CodexSessionDetail, error) {
	if !codexPathExists(sessionPath) {
		return CodexSessionDetail{}, errString("Session file not found")
	}
	modifiedAt := codexModifiedAtMs(sessionPath)

	f, err := os.Open(sessionPath) // #nosec G304 -- session file path
	if err != nil {
		return CodexSessionDetail{}, fmt.Errorf("Failed to open session file: %w", err)
	}
	raw, err := io.ReadAll(f)
	_ = f.Close()
	if err != nil {
		return CodexSessionDetail{}, fmt.Errorf("Failed to read session file: %w", err)
	}

	base := filepath.Base(sessionPath)
	id := strings.TrimSuffix(base, filepath.Ext(base))
	var cwd, model, modelProvider string
	tokenUsage := CodexTokenUsage{}
	messages := []CodexSessionMessage{}
	pendingThinking := []string{}
	pendingSummary := []string{}
	var firstUserMessage *string
	msgCounter := 0

	for _, line := range bytes.Split(raw, []byte("\n")) {
		var jsonValue map[string]any
		if err := json.Unmarshal(line, &jsonValue); err != nil {
			continue
		}
		timestamp, _ := jsonValue["timestamp"].(string)
		payload, _ := jsonValue["payload"].(map[string]any)
		if payload == nil {
			payload = map[string]any{}
		}
		switch typ, _ := jsonValue["type"].(string); typ {
		case "session_meta":
			if metaID, ok := payload["id"].(string); ok {
				id = metaID
			}
			if metaCwd, ok := payload["cwd"].(string); ok {
				cwd = metaCwd
			}
			if modelProvider == "" {
				if p, ok := payload["model_provider"].(string); ok {
					modelProvider = p
				}
			}
		case "turn_context":
			if model == "" {
				model, _ = payload["model"].(string)
			}
		case "response_item":
			switch payloadType, _ := payload["type"].(string); payloadType {
			case "message":
				role, _ := payload["role"].(string)
				// System-ish developer messages are not part of the chat.
				if role != "user" && role != "assistant" {
					continue
				}
				blocks := codexExtractMessageBlocks(payload["content"], role)
				if role == "user" {
					if firstUserMessage == nil && len(blocks) > 0 && blocks[0].Text != nil {
						firstUserMessage = blocks[0].Text
					}
					if len(blocks) == 0 {
						continue
					}
				}
				if role == "assistant" {
					// Attach the accumulated thinking stream as the leading
					// block of the assistant message.
					if thinking := codexTakePendingThinking(&pendingThinking, &pendingSummary); thinking != nil {
						blocks = append([]CodexContentBlock{{Type: "thinking", Thinking: thinking}}, blocks...)
					}
				}
				msgCounter++
				msgID, ok := payload["id"].(string)
				if !ok {
					msgID = fmt.Sprintf("msg-%d", msgCounter)
				}
				messages = append(messages, CodexSessionMessage{
					ID:        msgID,
					Role:      role,
					Content:   blocks,
					Timestamp: timestamp,
				})
			case "reasoning":
				pendingSummary = append(pendingSummary, codexReasoningSummaryTexts(payload)...)
			}
		case "event_msg":
			switch payloadType, _ := payload["type"].(string); payloadType {
			case "agent_reasoning":
				if text, ok := payload["text"].(string); ok {
					if trimmed := strings.TrimSpace(text); trimmed != "" {
						pendingThinking = append(pendingThinking, trimmed)
					}
				}
			case "token_count":
				info, _ := payload["info"].(map[string]any)
				if info == nil {
					continue
				}
				if total, ok := info["total_token_usage"].(map[string]any); ok && len(total) > 0 {
					tokenUsage = codexParseTokenUsage(total)
				}
			}
		}
	}

	// A session that ended mid-turn keeps its trailing thinking stream.
	if thinking := codexTakePendingThinking(&pendingThinking, &pendingSummary); thinking != nil {
		msgCounter++
		messages = append(messages, CodexSessionMessage{
			ID:      fmt.Sprintf("thinking-%d", msgCounter),
			Role:    "assistant",
			Content: []CodexContentBlock{{Type: "thinking", Thinking: thinking}},
		})
	}

	title := codexSessionUntitled
	if firstUserMessage != nil {
		title = codexTitleFromText(*firstUserMessage)
	}
	if model == "" {
		model = "unknown"
	}
	return CodexSessionDetail{
		ID:            id,
		Title:         title,
		Cwd:           cwd,
		Model:         model,
		ModelProvider: modelProvider,
		ModifiedAt:    modifiedAt,
		TokenUsage:    tokenUsage,
		Messages:      messages,
	}, nil
}

// codexMarshalCompact serialises JSON without HTML escaping (serde_json
// semantics) and without a trailing newline.
func codexMarshalCompact(value any) ([]byte, error) {
	var buf bytes.Buffer
	enc := json.NewEncoder(&buf)
	enc.SetEscapeHTML(false)
	if err := enc.Encode(value); err != nil {
		return nil, err
	}
	return bytes.TrimRight(buf.Bytes(), "\n"), nil
}

// ---------------------------------------------------------------------------
// State database (codex resume thread index)
// ---------------------------------------------------------------------------

// codexStateDB returns the newest state_*.sqlite in the codex home, if any.
func codexStateDB(codexHome string) string {
	entries, err := os.ReadDir(codexHome) // #nosec G304 -- codex home
	if err != nil {
		return ""
	}
	bestNumber := int64(-1)
	bestPath := ""
	for _, entry := range entries {
		if entry.IsDir() {
			continue
		}
		name := entry.Name()
		if !strings.HasPrefix(name, "state_") || !strings.HasSuffix(name, ".sqlite") {
			continue
		}
		numberStr := strings.TrimSuffix(strings.TrimPrefix(name, "state_"), ".sqlite")
		number, err := strconv.ParseUint(numberStr, 10, 64)
		if err != nil {
			continue
		}
		if int64(number) > bestNumber {
			bestNumber = int64(number)
			bestPath = filepath.Join(codexHome, name)
		}
	}
	return bestPath
}

// codexUpdateThreadIndexProvider updates threads.model_provider in the codex
// state database. Best-effort: sessions that are not indexed (or Codex
// versions without the database) are left alone — a missing threads table
// skips silently, mirroring rusqlite's query_row().unwrap_or(false).
func codexUpdateThreadIndexProvider(stateDB string, sessionID string, providerID string) error {
	db, err := sql.Open("sqlite", stateDB) // #nosec G304 -- codex state db
	if err != nil {
		return fmt.Errorf("Failed to open codex state database: %w", err)
	}
	defer func() { _ = db.Close() }()
	if _, err := db.Exec("PRAGMA busy_timeout = 2000"); err != nil {
		return fmt.Errorf("Failed to set database busy timeout: %w", err)
	}
	var one int
	if err := db.QueryRow("SELECT 1 FROM sqlite_master WHERE type = 'table' AND name = 'threads'").Scan(&one); err != nil {
		return nil // no threads table → nothing to update
	}
	if _, err := db.Exec("UPDATE threads SET model_provider = ? WHERE id = ?", providerID, sessionID); err != nil {
		return fmt.Errorf("Failed to update thread provider: %w", err)
	}
	return nil
}

// ---------------------------------------------------------------------------
// Provider switch / delete
// ---------------------------------------------------------------------------

// SetCodexSessionProvider rewrites the model_provider of a session file's
// session_meta line (rest preserved byte-for-byte) and keeps the codex
// thread index in sync so the session moves to the new provider's
// `codex resume` list.
func (a *App) SetCodexSessionProvider(sessionPath string, providerID string) error {
	sessionsDir, err := codexSessionsDir()
	if err != nil {
		return err
	}
	// Safety guard: only rewrite files inside the codex sessions directory.
	if !codexSessionWithin(sessionsDir, sessionPath) {
		return errString("Session file not found")
	}
	raw, err := os.ReadFile(sessionPath) // #nosec G304 -- session file path
	if err != nil {
		return fmt.Errorf("Failed to read session file: %w", err)
	}
	newline := bytes.IndexByte(raw, '\n')
	if newline < 0 {
		return errString("Session file is empty")
	}
	firstLine := raw[:newline]
	rest := raw[newline+1:]

	var header map[string]any
	if err := json.Unmarshal(firstLine, &header); err != nil {
		return fmt.Errorf("Failed to parse session_meta: %w", err)
	}
	if typ, _ := header["type"].(string); typ != "session_meta" {
		return errString("First line of session file is not session_meta")
	}
	payload, _ := header["payload"].(map[string]any)
	if payload == nil {
		payload = map[string]any{}
		header["payload"] = payload
	}
	sessionID, _ := payload["id"].(string)

	// Update the authoritative thread index first; if it fails, the session
	// file stays untouched so the change is all-or-nothing.
	if sessionID != "" {
		codexHome, err := codexConfigDirNoCreate()
		if err != nil {
			return err
		}
		if stateDB := codexStateDB(codexHome); stateDB != "" {
			if err := codexUpdateThreadIndexProvider(stateDB, sessionID, providerID); err != nil {
				return err
			}
		}
	}

	payload["model_provider"] = providerID
	updated, err := codexMarshalCompact(header)
	if err != nil {
		return fmt.Errorf("Failed to serialize session_meta: %w", err)
	}
	if err := core.AtomicWrite(sessionPath, append(updated, append([]byte("\n"), rest...)...)); err != nil {
		return fmt.Errorf("Failed to write session file: %w", err)
	}
	return nil
}

// DeleteCodexSession removes a session's .jsonl file (only inside the
// sessions directory) and prunes the empty YYYY/MM/DD directories behind it.
func (a *App) DeleteCodexSession(sessionPath string) error {
	sessionsDir, err := codexSessionsDir()
	if err != nil {
		return err
	}
	if !codexSessionWithin(sessionsDir, sessionPath) {
		return errString("Session file not found")
	}
	if err := os.Remove(sessionPath); err != nil {
		return fmt.Errorf("Failed to delete session: %w", err)
	}
	// Prune empty day/month directories, up to the sessions dir.
	dir := filepath.Dir(sessionPath)
	for dir != sessionsDir && strings.HasPrefix(dir, sessionsDir+string(filepath.Separator)) {
		if err := os.Remove(dir); err != nil {
			break
		}
		parent := filepath.Dir(dir)
		if parent == dir {
			break
		}
		dir = parent
	}
	return nil
}

// ---------------------------------------------------------------------------
// Watcher (poll-based mirror of the Rust notify watcher)
// ---------------------------------------------------------------------------

// codexSessionsWatcherState holds the single package-level poll loop.
type codexSessionsWatcherState struct {
	mu      sync.Mutex
	running bool
	stop    chan struct{}
}

var codexSessionsWatcher codexSessionsWatcherState

// emitCodexEvent broadcasts an event when a Wails app instance exists;
// outside one (unit tests, early startup) it falls back to the test sink
// when set, otherwise it is a no-op. Mirrors emitPtyEvent.
func emitCodexEvent(name string) {
	if app := application.Get(); app != nil {
		app.Event.Emit(name)
		return
	}
	if codexEventSink != nil {
		codexEventSink(name)
	}
}

// codexEventSink lets tests observe the event stream without a running app.
var codexEventSink func(name string)

// codexSessionsSignature fingerprints the sessions tree (file count, total
// size, newest mtime) so the poll loop only emits on actual change.
func codexSessionsSignature(dir string) string {
	count := 0
	var totalSize, maxMtime int64
	_ = filepath.WalkDir(dir, func(path string, entry os.DirEntry, err error) error {
		if err != nil || entry.IsDir() {
			return nil
		}
		info, err := entry.Info()
		if err != nil {
			return nil
		}
		count++
		totalSize += info.Size()
		if mtime := info.ModTime().UnixNano(); mtime > maxMtime {
			maxMtime = mtime
		}
		return nil
	})
	return fmt.Sprintf("%d:%d:%d", count, totalSize, maxMtime)
}

// codexWatchSessionsLoop emits `codex-sessions-changed` when the sessions
// tree fingerprint changes. Event-driven via fsnotify with a low-frequency
// fallback poll (watchtree.go); the baseline is taken at start, matching
// notify (no event for the initial state).
func codexWatchSessionsLoop(dir string, stop <-chan struct{}) {
	signature := codexSessionsSignature(dir)
	changed := func() bool {
		next := codexSessionsSignature(dir)
		if next == signature {
			return false
		}
		signature = next
		return true
	}
	watchTreeUntil(dir, stop, changed, codexSessionsFallbackEvery, func() {
		emitCodexEvent(codexSessionEventName)
	})
}

// StartCodexSessionsWatcher starts watching the codex sessions directory for
// changes (no-op when the directory does not exist yet, like Rust).
func (a *App) StartCodexSessionsWatcher() error {
	sessionsDir, err := codexSessionsDir()
	if err != nil {
		return err
	}
	if !core.DirExists(sessionsDir) {
		return nil
	}
	codexSessionsWatcher.mu.Lock()
	defer codexSessionsWatcher.mu.Unlock()
	if codexSessionsWatcher.running {
		// Rust replaces the existing watcher on start.
		close(codexSessionsWatcher.stop)
	}
	stop := make(chan struct{})
	codexSessionsWatcher.stop = stop
	codexSessionsWatcher.running = true
	go codexWatchSessionsLoop(sessionsDir, stop)
	return nil
}

// StopCodexSessionsWatcher stops watching the codex sessions directory.
func (a *App) StopCodexSessionsWatcher() error {
	codexSessionsWatcher.mu.Lock()
	defer codexSessionsWatcher.mu.Unlock()
	if codexSessionsWatcher.running {
		close(codexSessionsWatcher.stop)
		codexSessionsWatcher.running = false
		codexSessionsWatcher.stop = nil
	}
	return nil
}
