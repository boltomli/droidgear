// Dsh (DeepSeek Harness) session history viewer. Reads the on-disk JSONL
// format (versions 0–4, optionally Zstandard-compressed) directly:
//
//	~/.dsh/sessions/
//	  --<normalized-cwd>--/          # readable project directory
//	    <session-id>/                # session-owned directory
//	      session.v4.jsonl.zstd      # newest generation, compressed
//	      session.jsonl              # same generations, uncompressed
//
// Mirrors droidgear-core/src/dsh_sessions.rs and
// src-tauri/src/commands/dsh_sessions.rs. Compressed generations are a
// concatenation of independent Zstandard frames; the klauspost decoder
// streams all frames instead of assuming a single one.
package read

import (
	"bufio"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"sort"
	"strconv"
	"strings"
	"time"

	"droidgear-wails/internal/core"
	"github.com/klauspost/compress/zstd"
)

// How many decompressed bytes of a log to scan for header / title / model.
const dshHeadScanBytes = 1024 * 1024

// Display title fallback when no title and no human message exist.
const dshUntitled = "New Session"

// Display model fallback.
const dshUnknownModel = "unknown"

// ---------------------------------------------------------------------------
// Types (droidgear-core::dsh_sessions)
// ---------------------------------------------------------------------------

// DshTokenUsage mirrors dsh_sessions::DshTokenUsage.
type DshTokenUsage struct {
	InputTokens         float64 `json:"inputTokens"`
	OutputTokens        float64 `json:"outputTokens"`
	CacheReadTokens     float64 `json:"cacheReadTokens"`
	CacheCreationTokens float64 `json:"cacheCreationTokens"`
	TotalTokens         float64 `json:"totalTokens"`
}

// DshSessionSummary mirrors dsh_sessions::DshSessionSummary.
type DshSessionSummary struct {
	ID            string        `json:"id"`
	Title         string        `json:"title"`
	Project       string        `json:"project"` // the header's cwd, not the directory encoding
	Model         string        `json:"model"`
	ModelProvider string        `json:"modelProvider"`
	ModifiedAt    float64       `json:"modifiedAt"`
	TokenUsage    DshTokenUsage `json:"tokenUsage"`
	Path          string        `json:"path"`
}

// DshSessionMessage mirrors dsh_sessions::DshSessionMessage.
type DshSessionMessage struct {
	ID        string         `json:"id"`
	Role      string         `json:"role"`
	Content   []ContentBlock `json:"content"`
	Timestamp string         `json:"timestamp"`
}

// DshSessionDetail mirrors dsh_sessions::DshSessionDetail.
type DshSessionDetail struct {
	Summary  DshSessionSummary   `json:"summary"`
	Messages []DshSessionMessage `json:"messages"`
}

// ---------------------------------------------------------------------------
// Path helpers
// ---------------------------------------------------------------------------

// dshSessionsDir resolves `<dsh home>/sessions` (Rust dsh_sessions_dir).
// DshSessionsDir returns the sessions directory for the current home — exported for the GUI-side watcher in internal/app.
func DshSessionsDir() (string, error) {
	return dshSessionsDir()
}

func dshSessionsDir() (string, error) {
	home, err := core.DshHome()
	if err != nil {
		return "", errString("Failed to get home directory")
	}
	return filepath.Join(home, "sessions"), nil
}

// dshStoragesDir resolves `<dsh home>/storages`.
func dshStoragesDir() (string, error) {
	home, err := core.DshHome()
	if err != nil {
		return "", errString("Failed to get home directory")
	}
	return filepath.Join(home, "storages"), nil
}

// ---------------------------------------------------------------------------
// Log discovery / decoding
// ---------------------------------------------------------------------------

// parseDshLogName parses a canonical session log filename into
// (format version, compressed). Canonical names are `session.jsonl[.zstd]`
// for v0 and `session.v<N>.jsonl[.zstd]` for later versions.
func parseDshLogName(name string) (version uint32, compressed bool, ok bool) {
	stem := name
	if strings.HasSuffix(stem, ".zstd") {
		compressed = true
		stem = strings.TrimSuffix(stem, ".zstd")
	}
	if !strings.HasSuffix(stem, ".jsonl") {
		return 0, false, false
	}
	stem = strings.TrimSuffix(stem, ".jsonl")
	if stem == "session" {
		return 0, compressed, true
	}
	if !strings.HasPrefix(stem, "session.v") {
		return 0, false, false
	}
	parsed, err := strconv.ParseUint(strings.TrimPrefix(stem, "session.v"), 10, 32)
	if err != nil {
		return 0, false, false
	}
	return uint32(parsed), compressed, true // #nosec G115 -- bounded by 32-bit parse
}

// newestDshLogInSessionDir selects the newest canonical generation: higher
// version wins, compressed wins over uncompressed at equal version.
func newestDshLogInSessionDir(dir string) (string, bool) {
	entries, err := os.ReadDir(dir)
	if err != nil {
		return "", false
	}
	bestPath := ""
	var bestVersion uint32
	var bestCompressed bool
	found := false
	for _, entry := range entries {
		if entry.IsDir() {
			continue
		}
		version, compressed, ok := parseDshLogName(entry.Name())
		if !ok {
			continue
		}
		path := filepath.Join(dir, entry.Name())
		replace := !found ||
			version > bestVersion ||
			(version == bestVersion && compressed && !bestCompressed)
		if replace {
			bestPath = path
			bestVersion = version
			bestCompressed = compressed
			found = true
		}
	}
	return bestPath, found
}

// dshLogReader wraps a session log stream; Close releases the decoder and
// the underlying file.
type dshLogReader struct {
	*bufio.Reader
	closeFn func() error
}

func (r *dshLogReader) Close() error { return r.closeFn() }

// openDshLogReader opens a session log as a buffered line reader,
// transparently decompressing Zstandard generations.
func openDshLogReader(path string) (*dshLogReader, error) {
	file, err := os.Open(path) // #nosec G304 -- path derived from the sessions store
	if err != nil {
		return nil, fmt.Errorf("Failed to open Dsh session: %w", err)
	}
	if filepath.Ext(path) == ".zstd" {
		decoder, err := zstd.NewReader(file)
		if err != nil {
			_ = file.Close()
			return nil, fmt.Errorf("zstd error: %w", err)
		}
		return &dshLogReader{
			Reader: bufio.NewReader(decoder),
			closeFn: func() error {
				decoder.Close()
				return file.Close()
			},
		}, nil
	}
	return &dshLogReader{
		Reader:  bufio.NewReader(file),
		closeFn: file.Close,
	}, nil
}

// ---------------------------------------------------------------------------
// Small JSON helpers (serde Value semantics)
// ---------------------------------------------------------------------------

// dshEventKind returns the string `type` field of an event envelope.
func dshEventKind(m map[string]any) string {
	kind, _ := sessionJSONStr(m, "type")
	return kind
}

// dshNonEmptyStr mirrors dsh_sessions::non_empty_str: trimmed, non-empty
// string values only.
func dshNonEmptyStr(m map[string]any, key string) (string, bool) {
	value, ok := sessionJSONStr(m, key)
	if !ok {
		return "", false
	}
	value = strings.TrimSpace(value)
	if value == "" {
		return "", false
	}
	return value, true
}

// dshPointer walks a JSON pointer path (Rust serde_json::Pointer).
func dshPointer(m map[string]any, keys ...string) any {
	var current any = m
	for _, key := range keys {
		object, ok := current.(map[string]any)
		if !ok {
			return nil
		}
		current = object[key]
	}
	return current
}

// dshFirstText returns the first non-empty trimmed `text` block of a
// content array (Rust first_text_of_content).
func dshFirstText(content any) *string {
	blocks, ok := content.([]any)
	if !ok {
		return nil
	}
	for _, raw := range blocks {
		block, ok := raw.(map[string]any)
		if !ok {
			continue
		}
		if kind, ok := dshNonEmptyStr(block, "type"); !ok || kind != "text" {
			continue
		}
		if text, ok := sessionJSONStr(block, "text"); ok {
			text = strings.TrimSpace(text)
			if text != "" {
				return &text
			}
		}
	}
	return nil
}

// ---------------------------------------------------------------------------
// Token usage
// ---------------------------------------------------------------------------

// addUsage accumulates one assistant usage record (Rust add_usage).
func (t *DshTokenUsage) addUsage(usage map[string]any) {
	input, ok := sessionJSONNum(usage, "inputTokens")
	if !ok {
		input, _ = sessionJSONNum(usage, "uncachedInputTokens")
	}
	output, _ := sessionJSONNum(usage, "outputTokens")
	cacheRead, _ := sessionJSONNum(usage, "cacheReadTokens")
	cacheCreation, _ := sessionJSONNum(usage, "cacheWriteTokens")
	t.InputTokens += input
	t.OutputTokens += output
	t.CacheReadTokens += cacheRead
	t.CacheCreationTokens += cacheCreation
	t.TotalTokens = t.InputTokens + t.OutputTokens + t.CacheReadTokens + t.CacheCreationTokens
}

// fromDshProjcacheTotals builds usage from a projection-cache totals row.
func fromDshProjcacheTotals(totals map[string]any) DshTokenUsage {
	input, ok := sessionJSONNum(totals, "uncachedInputTokens")
	if !ok {
		input, _ = sessionJSONNum(totals, "inputTokens")
	}
	output, _ := sessionJSONNum(totals, "outputTokens")
	cacheRead, _ := sessionJSONNum(totals, "cacheReadTokens")
	cacheCreation, _ := sessionJSONNum(totals, "cacheWriteTokens")
	return DshTokenUsage{
		InputTokens:         input,
		OutputTokens:        output,
		CacheReadTokens:     cacheRead,
		CacheCreationTokens: cacheCreation,
		TotalTokens:         input + output + cacheRead + cacheCreation,
	}
}

// ---------------------------------------------------------------------------
// Projection-cache metadata (optional fast path for the list view)
// ---------------------------------------------------------------------------

type dshProjcacheEntry struct {
	title         *string
	firstPrompt   *string
	cwd           *string
	model         *string
	modelProvider *string
	tokenUsage    DshTokenUsage
	hasUsage      bool
}

// dshRowValue returns rows[key].val.
func dshRowValue(rows map[string]any, key string) (map[string]any, bool) {
	row, ok := rows[key].(map[string]any)
	if !ok {
		return nil, false
	}
	value, ok := row["val"].(map[string]any)
	return value, ok
}

func dshProjcacheEntryFromRecord(record any) (*dshProjcacheEntry, bool) {
	object, ok := record.(map[string]any)
	if !ok {
		return nil, false
	}
	rows, ok := object["rows"].(map[string]any)
	if !ok {
		return nil, false
	}
	entry := &dshProjcacheEntry{}
	// title: rows.title.val (string, trimmed, non-empty)
	if value, ok := rows["title"].(map[string]any); ok {
		if raw, ok := value["val"].(string); ok {
			raw = strings.TrimSpace(raw)
			if raw != "" {
				entry.title = &raw
			}
		}
	}
	// first prompt: rows.titleInput.val.first.text
	if value, ok := dshRowValue(rows, "titleInput"); ok {
		if first, ok := value["first"].(map[string]any); ok {
			if text, ok := sessionJSONStr(first, "text"); ok {
				text = strings.TrimSpace(text)
				if text != "" {
					entry.firstPrompt = &text
				}
			}
		}
	}
	// cwd: identity.cwd
	if identity, ok := object["identity"].(map[string]any); ok {
		if cwd, ok := dshNonEmptyStr(identity, "cwd"); ok {
			entry.cwd = &cwd
		}
	}
	// model / provider: rows.modelSelection.val.lastUsed
	if value, ok := dshRowValue(rows, "modelSelection"); ok {
		if lastUsed, ok := value["lastUsed"].(map[string]any); ok {
			if model, ok := dshNonEmptyStr(lastUsed, "model"); ok {
				entry.model = &model
			}
			if provider, ok := dshNonEmptyStr(lastUsed, "provider"); ok {
				entry.modelProvider = &provider
			}
		}
	}
	// token usage: rows.tokenUsage.val.totals
	if value, ok := dshRowValue(rows, "tokenUsage"); ok {
		if totals, ok := value["totals"].(map[string]any); ok {
			entry.tokenUsage = fromDshProjcacheTotals(totals)
			entry.hasUsage = true
		}
	}
	return entry, true
}

// readDshProjcacheEntry prefers the per-session cache file and falls back
// to the legacy single-table layout (only consulted when the per-session
// file is missing or unusable).
func readDshProjcacheEntry(id string) *dshProjcacheEntry {
	storages, err := dshStoragesDir()
	if err != nil {
		return nil
	}
	perSession := filepath.Join(storages, "session_projcache", "sessions", id+".json")
	if data, err := os.ReadFile(perSession); err == nil { // #nosec G304 -- cache path
		if parsed, ok := core.DecodeJSONObject(string(data)); ok {
			record := parsed
			if value, exists := parsed["record"]; exists {
				record = nil
				if object, isObject := value.(map[string]any); isObject {
					record = object
				}
			}
			if record != nil {
				if entry, ok := dshProjcacheEntryFromRecord(record); ok {
					return entry
				}
			}
		}
	}

	data, err := os.ReadFile(filepath.Join(storages, "session_projcache.json")) // #nosec G304 -- cache path
	if err != nil {
		return nil
	}
	parsed, ok := core.DecodeJSONObject(string(data))
	if !ok {
		return nil
	}
	sessions, ok := dshPointer(parsed, "tables", "sessions").(map[string]any)
	if !ok {
		return nil
	}
	record, ok := sessions[id]
	if !ok {
		return nil
	}
	entry, ok := dshProjcacheEntryFromRecord(record)
	if !ok {
		return nil
	}
	return entry
}

// ---------------------------------------------------------------------------
// Log scanning (head: id / cwd / title / model / first prompt)
// ---------------------------------------------------------------------------

type dshLogHead struct {
	id            *string
	cwd           *string
	createdAt     *int64
	title         *string
	model         *string
	modelProvider *string
	firstPrompt   *string
}

func (h *dshLogHead) apply(json map[string]any) {
	switch dshEventKind(json) {
	case "session":
		if id, ok := dshNonEmptyStr(json, "id"); ok {
			h.id = &id
		}
		if cwd, ok := dshNonEmptyStr(json, "cwd"); ok {
			h.cwd = &cwd
		}
		if h.createdAt == nil {
			if createdAt, ok := sessionJSONInt(json, "createdAt"); ok {
				h.createdAt = &createdAt
			}
		}
	case "session/title":
		if raw, ok := dshPointer(json, "data", "title").(string); ok {
			raw = strings.TrimSpace(raw)
			if raw != "" {
				h.title = &raw
			}
		}
	case "request/header":
		config, _ := dshPointer(json, "data", "header", "config").(map[string]any)
		if model, ok := dshNonEmptyStr(config, "model"); ok {
			h.model = &model
		}
		if provider, ok := dshNonEmptyStr(config, "provider"); ok {
			h.modelProvider = &provider
		}
	case "user/message":
		kind, _ := dshPointer(json, "data", "source", "kind").(string)
		if kind != "user" {
			return
		}
		if h.firstPrompt == nil {
			h.firstPrompt = dshFirstText(dshPointer(json, "data", "content"))
		}
	}
}

// scanDshLogHead scans the head of a log, bounded by dshHeadScanBytes of
// decoded output. A missing `session` header is an error.
func scanDshLogHead(path string) (dshLogHead, error) {
	reader, err := openDshLogReader(path)
	if err != nil {
		return dshLogHead{}, err
	}
	defer func() { _ = reader.Close() }()

	head := dshLogHead{}
	limited := io.LimitReader(reader, dshHeadScanBytes)
	buffered := bufio.NewReader(limited)
	for {
		line, err := buffered.ReadBytes('\n')
		if len(line) > 0 {
			trimmed := strings.TrimSpace(string(line))
			if trimmed != "" {
				if parsed, ok := core.DecodeJSONObject(trimmed); ok {
					head.apply(parsed)
				}
			}
		}
		if err != nil {
			// A torn trailing frame only loses the suffix; keep the head.
			break
		}
	}
	if head.id == nil {
		return dshLogHead{}, errString("Dsh session header not found")
	}
	return head, nil
}

// ---------------------------------------------------------------------------
// Title / summary assembly
// ---------------------------------------------------------------------------

// displayDshTitle prefers the accepted title, then derives one from the
// first human prompt, then falls back to "New Session".
func displayDshTitle(title, firstPrompt *string) string {
	if title != nil {
		if trimmed := strings.TrimSpace(*title); trimmed != "" {
			return trimmed
		}
	}
	if firstPrompt != nil {
		if trimmed := strings.TrimSpace(*firstPrompt); trimmed != "" {
			if derived := sessionTitleFromText(trimmed, 80); derived != "" {
				return derived
			}
		}
	}
	return dshUntitled
}

func buildDshSummary(logPath string) (DshSessionSummary, error) {
	head, err := scanDshLogHead(logPath)
	if err != nil {
		return DshSessionSummary{}, err
	}
	sessionDir := filepath.Dir(logPath)
	id := ""
	if head.id != nil {
		id = *head.id
	} else {
		id = filepath.Base(sessionDir)
	}

	cache := readDshProjcacheEntry(id)

	title := displayDshTitle(
		pickDshString(cache, func(e *dshProjcacheEntry) *string { return e.title }, head.title),
		pickDshString(cache, func(e *dshProjcacheEntry) *string { return e.firstPrompt }, head.firstPrompt),
	)

	project := ""
	if cache != nil && cache.cwd != nil {
		project = *cache.cwd
	} else if head.cwd != nil {
		project = *head.cwd
	}

	model := dshUnknownModel
	if cache != nil && cache.model != nil {
		model = *cache.model
	} else if head.model != nil {
		model = *head.model
	}

	modelProvider := ""
	if cache != nil && cache.modelProvider != nil {
		modelProvider = *cache.modelProvider
	} else if head.modelProvider != nil {
		modelProvider = *head.modelProvider
	}

	usage := DshTokenUsage{}
	if cache != nil && cache.hasUsage {
		usage = cache.tokenUsage
	}

	return DshSessionSummary{
		ID:            id,
		Title:         title,
		Project:       project,
		Model:         model,
		ModelProvider: modelProvider,
		ModifiedAt:    sessionModifiedMs(logPath),
		TokenUsage:    usage,
		Path:          logPath,
	}, nil
}

// pickDshString returns the cache field when the cache has one, otherwise
// the head field (Rust `.or_else` chains).
func pickDshString(cache *dshProjcacheEntry, cacheField func(*dshProjcacheEntry) *string, headValue *string) *string {
	if cache != nil {
		if value := cacheField(cache); value != nil {
			return value
		}
	}
	return headValue
}

// ---------------------------------------------------------------------------
// Listing
// ---------------------------------------------------------------------------

func listDshSessions(sessionsDir string) ([]DshSessionSummary, error) {
	sessions := []DshSessionSummary{}
	if !core.DirExists(sessionsDir) {
		return sessions, nil
	}
	projectDirs, err := os.ReadDir(sessionsDir)
	if err != nil {
		return nil, fmt.Errorf("Failed to read Dsh sessions directory: %w", err)
	}
	for _, project := range projectDirs {
		if !project.IsDir() {
			continue
		}
		projectPath := filepath.Join(sessionsDir, project.Name())
		sessionDirs, err := os.ReadDir(projectPath)
		if err != nil {
			continue
		}
		for _, session := range sessionDirs {
			sessionPath := filepath.Join(projectPath, session.Name())
			if !session.IsDir() {
				continue
			}
			logPath, ok := newestDshLogInSessionDir(sessionPath)
			if !ok {
				continue
			}
			summary, err := buildDshSummary(logPath)
			if err != nil {
				// Unreadable sessions are skipped (Rust logs a warning).
				continue
			}
			sessions = append(sessions, summary)
		}
	}
	sort.SliceStable(sessions, func(i, j int) bool {
		return sessions[i].ModifiedAt > sessions[j].ModifiedAt
	})
	return sessions, nil
}

// ---------------------------------------------------------------------------
// Detail
// ---------------------------------------------------------------------------

// dshContentBlocks maps a content array into display blocks: text blocks
// for both roles, reasoning blocks only for the assistant (tool calls stay
// raw in the log — same display contract as Codex/Droid/Pi).
func dshContentBlocks(content any, role string) []ContentBlock {
	blocks, ok := content.([]any)
	if !ok {
		return []ContentBlock{}
	}
	out := []ContentBlock{}
	for _, raw := range blocks {
		block, ok := raw.(map[string]any)
		if !ok {
			continue
		}
		kind, _ := sessionJSONStr(block, "type")
		switch kind {
		case "text":
			if text, ok := sessionJSONStr(block, "text"); ok {
				text = strings.TrimSpace(text)
				if text != "" {
					value := text
					out = append(out, ContentBlock{Type: "text", Text: &value})
				}
			}
		case "reasoning":
			if role == "assistant" {
				if text, ok := sessionJSONStr(block, "text"); ok {
					text = strings.TrimSpace(text)
					if text != "" {
						value := text
						out = append(out, ContentBlock{Type: "thinking", Thinking: &value})
					}
				}
			}
		}
	}
	return out
}

// dshTimestampFromMs formats an epoch-millis timestamp like
// chrono `to_rfc3339_opts(Secs, true)`; invalid/absent → "".
func dshTimestampFromMs(ms *int64) string {
	if ms == nil {
		return ""
	}
	return time.UnixMilli(*ms).UTC().Format("2006-01-02T15:04:05Z07:00")
}

func getDshSessionDetail(sessionPath string) (DshSessionDetail, error) {
	logPath := sessionPath
	info, err := os.Stat(logPath)
	if err != nil || info.IsDir() {
		return DshSessionDetail{}, errString("Dsh session file not found")
	}

	modifiedAt := sessionModifiedMs(logPath)
	reader, err := openDshLogReader(logPath)
	if err != nil {
		return DshSessionDetail{}, err
	}
	defer func() { _ = reader.Close() }()

	id := filepath.Base(filepath.Dir(logPath))
	var cwd string
	var title *string
	var firstPrompt *string
	var model *string
	var modelProvider *string
	usage := DshTokenUsage{}
	messages := []DshSessionMessage{}
	counter := 0

	for {
		line, err := reader.ReadBytes('\n')
		if len(line) > 0 {
			trimmed := strings.TrimSpace(string(line))
			if trimmed != "" {
				if parsed, ok := core.DecodeJSONObject(trimmed); ok {
					applyDshDetailLine(parsed, &id, &cwd, &title, &firstPrompt, &model, &modelProvider, &usage, &messages, &counter)
				}
			}
		}
		if err != nil {
			// Torn final frame: keep the complete decoded prefix.
			break
		}
	}

	summary := DshSessionSummary{
		ID:            id,
		Title:         displayDshTitle(title, firstPrompt),
		Project:       cwd,
		Model:         dshUnknownModel,
		ModelProvider: "",
		ModifiedAt:    modifiedAt,
		TokenUsage:    usage,
		Path:          logPath,
	}
	if model != nil {
		summary.Model = *model
	}
	if modelProvider != nil {
		summary.ModelProvider = *modelProvider
	}
	return DshSessionDetail{Summary: summary, Messages: messages}, nil
}

// applyDshDetailLine folds one JSONL record into the running detail state.
func applyDshDetailLine(
	parsed map[string]any,
	id *string,
	cwd *string,
	title **string,
	firstPrompt **string,
	model **string,
	modelProvider **string,
	usage *DshTokenUsage,
	messages *[]DshSessionMessage,
	counter *int,
) {
	switch dshEventKind(parsed) {
	case "session":
		if value, ok := dshNonEmptyStr(parsed, "id"); ok {
			*id = value
		}
		if value, ok := dshNonEmptyStr(parsed, "cwd"); ok {
			*cwd = value
		}
	case "session/title":
		if raw, ok := dshPointer(parsed, "data", "title").(string); ok {
			raw = strings.TrimSpace(raw)
			if raw != "" {
				*title = &raw
			}
		}
	case "request/header":
		config, _ := dshPointer(parsed, "data", "header", "config").(map[string]any)
		if value, ok := dshNonEmptyStr(config, "model"); ok {
			*model = &value
		}
		if value, ok := dshNonEmptyStr(config, "provider"); ok {
			*modelProvider = &value
		}
	case "user/message":
		kind, _ := dshPointer(parsed, "data", "source", "kind").(string)
		if kind != "user" {
			return
		}
		blocks := dshContentBlocks(dshPointer(parsed, "data", "content"), "user")
		if len(blocks) == 0 {
			return
		}
		if *firstPrompt == nil {
			for _, block := range blocks {
				if block.Type == "text" && block.Text != nil {
					value := *block.Text
					*firstPrompt = &value
					break
				}
			}
		}
		*counter++
		messageID, ok := dshNonEmptyStr(parsed, "id")
		if !ok {
			messageID = fmt.Sprintf("user-%d", *counter)
		}
		*messages = append(*messages, DshSessionMessage{
			ID:        messageID,
			Role:      "user",
			Content:   blocks,
			Timestamp: dshTimestampFromMs(dshIntField(parsed, "time")),
		})
	case "assistant/message":
		data, _ := parsed["data"].(map[string]any)
		if data == nil {
			data = map[string]any{}
		}
		if usageValue, ok := data["usage"].(map[string]any); ok {
			usage.addUsage(usageValue)
		}
		blocks := dshContentBlocks(dshPointer(data, "message", "content"), "assistant")
		if len(blocks) == 0 {
			return
		}
		*counter++
		messageID, ok := dshPointer(data, "message", "id").(string)
		if !ok {
			messageID = fmt.Sprintf("assistant-%d", *counter)
		}
		*messages = append(*messages, DshSessionMessage{
			ID:        messageID,
			Role:      "assistant",
			Content:   blocks,
			Timestamp: dshTimestampFromMs(dshIntField(parsed, "time")),
		})
	}
}

// dshIntField returns an integer JSON number field (serde as_i64 parity).
func dshIntField(m map[string]any, key string) *int64 {
	if value, ok := sessionJSONInt(m, key); ok {
		return &value
	}
	return nil
}

// ---------------------------------------------------------------------------
// Delete
// ---------------------------------------------------------------------------

func deleteDshSession(logPath string) error {
	sessionsDir, err := dshSessionsDir()
	if err != nil {
		return err
	}
	info, err := os.Stat(logPath)
	if err != nil || info.IsDir() || !sessionPathWithin(sessionsDir, logPath) {
		return errString("Dsh session file not found")
	}
	sessionDir := filepath.Dir(logPath)
	projectDir := filepath.Dir(sessionDir)
	if filepath.Clean(filepath.Dir(projectDir)) != filepath.Clean(sessionsDir) {
		return errString("Dsh session file not found")
	}
	if err := os.RemoveAll(sessionDir); err != nil {
		return fmt.Errorf("Failed to delete Dsh session: %w", err)
	}
	// Prune the project directory when it becomes empty.
	if entries, err := os.ReadDir(projectDir); err == nil && len(entries) == 0 {
		_ = os.Remove(projectDir)
	}
	return nil
}

// ---------------------------------------------------------------------------
// Commands (src-tauri/src/commands/dsh_sessions.rs)
// ---------------------------------------------------------------------------

// ListDshSessions lists Dsh sessions, newest first (Rust `list_dsh_sessions`).
func ListDshSessions() ([]DshSessionSummary, error) {
	sessionsDir, err := dshSessionsDir()
	if err != nil {
		return nil, err
	}
	return listDshSessions(sessionsDir)
}

// GetDshSessionDetail returns one Dsh session with its transcript (Rust
// `get_dsh_session_detail`).
func GetDshSessionDetail(sessionPath string) (DshSessionDetail, error) {
	return getDshSessionDetail(sessionPath)
}

// DeleteDshSession removes a session directory and prunes the empty
// project directory (Rust `delete_dsh_session`).
func DeleteDshSession(sessionPath string) error {
	return deleteDshSession(sessionPath)
}
