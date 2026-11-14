// Sessions viewer for the generic Factory/Droid session store, plus the
// shared poll-based watcher plumbing and JSON helpers used by all four
// sessions viewers (generic, Dsh, Pi, OMP).
//
// Mirrors droidgear-core/src/sessions.rs (parsing/listing) and
// src-tauri/src/commands/sessions.rs (commands + notify watcher). The Rust
// side uses notify file watchers; per wails/PORTING.md a poll loop that
// emits only on actual change is an accepted equivalent.
package read

import (
	"bufio"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"sort"
	"strings"

	"droidgear-wails/internal/core"
)

// ---------------------------------------------------------------------------
// Types — generic Factory sessions (droidgear-core::sessions)
// ---------------------------------------------------------------------------

// SessionProject mirrors sessions::SessionProject: a directory under
// ~/.factory/sessions that contains session files.
type SessionProject struct {
	// Name is the directory name (e.g. "-Users-sunshow-GIT-quickcast-api").
	Name string `json:"name"`
	// Path is the full path to the directory.
	Path string `json:"path"`
	// SessionCount is the number of .jsonl sessions in the directory.
	SessionCount uint32 `json:"sessionCount"`
	// ModifiedAt is the directory mtime in milliseconds.
	ModifiedAt float64 `json:"modifiedAt"`
}

// TokenUsage mirrors sessions::TokenUsage (generic session token counters).
type TokenUsage struct {
	InputTokens         float64 `json:"inputTokens"`
	OutputTokens        float64 `json:"outputTokens"`
	CacheCreationTokens float64 `json:"cacheCreationTokens"`
	CacheReadTokens     float64 `json:"cacheReadTokens"`
	ThinkingTokens      float64 `json:"thinkingTokens"`
}

// SessionSummary mirrors sessions::SessionSummary (list view row).
type SessionSummary struct {
	ID         string     `json:"id"`
	Title      string     `json:"title"`
	Project    string     `json:"project"`
	Model      string     `json:"model"`
	ModifiedAt float64    `json:"modifiedAt"`
	TokenUsage TokenUsage `json:"tokenUsage"`
	// Path is the full path to the session files without extension.
	Path string `json:"path"`
}

// ContentBlock mirrors sessions::ContentBlock — the shared transcript block
// type used by the generic, Dsh, Pi and OMP session messages.
type ContentBlock struct {
	Type     string  `json:"type"`
	Text     *string `json:"text,omitempty"`
	Thinking *string `json:"thinking,omitempty"`
}

// SessionMessage mirrors sessions::SessionMessage.
type SessionMessage struct {
	ID        string         `json:"id"`
	Role      string         `json:"role"`
	Content   []ContentBlock `json:"content"`
	Timestamp string         `json:"timestamp"`
}

// SessionDetail mirrors sessions::SessionDetail (detail view with messages).
type SessionDetail struct {
	ID         string           `json:"id"`
	Title      string           `json:"title"`
	Project    string           `json:"project"`
	Model      string           `json:"model"`
	Cwd        string           `json:"cwd"`
	ModifiedAt float64          `json:"modifiedAt"`
	TokenUsage TokenUsage       `json:"tokenUsage"`
	Messages   []SessionMessage `json:"messages"`
}

// ---------------------------------------------------------------------------
// Shared helpers for the four sessions viewers
// ---------------------------------------------------------------------------

// sessionModifiedMs returns the file mtime in milliseconds since the Unix
// epoch, or 0 when unavailable (mirrors the Rust modified_at_ms helpers).
func sessionModifiedMs(path string) float64 {
	info, err := os.Stat(path)
	if err != nil {
		return 0
	}
	ms := info.ModTime().UnixMilli()
	if ms < 0 {
		return 0
	}
	return float64(ms)
}

// sessionTitleFromText collapses whitespace and keeps at most max runes,
// appending an ellipsis when truncated (shared by Dsh/Pi/OMP title
// derivation; Rust has one copy per tool with 80/60-char limits).
func sessionTitleFromText(text string, max int) string {
	normalized := strings.Join(strings.Fields(text), " ")
	runes := []rune(normalized)
	if len(runes) > max {
		return string(runes[:max]) + "…"
	}
	return normalized
}

// sessionJSONMap parses one JSON document leniently: trailing data and
// non-object documents are rejected (serde_json::from_str parity), numbers
// stay json.Number so `as_i64` vs `as_f64` semantics can be reproduced.
func sessionJSONMap(raw string) (map[string]any, bool) {
	dec := json.NewDecoder(strings.NewReader(raw))
	dec.UseNumber()
	var out map[string]any
	if err := dec.Decode(&out); err != nil || out == nil {
		return nil, false
	}
	if _, err := dec.Token(); err != io.EOF {
		return nil, false
	}
	return out, true
}

// sessionJSONStr mirrors Value::as_str for one key.
func sessionJSONStr(m map[string]any, key string) (string, bool) {
	value, ok := m[key]
	if !ok {
		return "", false
	}
	str, ok := value.(string)
	return str, ok
}

// sessionJSONNum mirrors Value::as_f64 for one key (any JSON number).
func sessionJSONNum(m map[string]any, key string) (float64, bool) {
	value, ok := m[key]
	if !ok {
		return 0, false
	}
	switch number := value.(type) {
	case json.Number:
		f, err := number.Float64()
		return f, err == nil
	case float64:
		return number, true
	}
	return 0, false
}

// sessionJSONNumField is sessionJSONNum with a 0 default.
func sessionJSONNumField(m map[string]any, key string) float64 {
	f, _ := sessionJSONNum(m, key)
	return f
}

// sessionJSONInt mirrors Value::as_i64 for one key: only integer literals
// (no '.'/'e') in the int64 range count, matching serde_json's Number
// representation split.
func sessionJSONInt(m map[string]any, key string) (int64, bool) {
	value, ok := m[key]
	if !ok {
		return 0, false
	}
	number, ok := value.(json.Number)
	if !ok {
		return 0, false
	}
	i, err := number.Int64()
	return i, err == nil
}

// sessionModelPair is the (provider, model) pair tracked while walking a
// Pi/OMP session history along the active branch.
type sessionModelPair struct {
	provider string
	model    string
}

// unmarshalContentBlocks parses a `content` JSON array with serde
// ContentBlock strictness (mirrors the untagged Blocks arm of the Rust
// Content enum): every element must be an object carrying a string "type",
// and `text`/`thinking` must be strings or null.
func unmarshalContentBlocks(data []byte) ([]ContentBlock, error) {
	var items []map[string]any
	if err := json.Unmarshal(data, &items); err != nil {
		return nil, err
	}
	blocks := make([]ContentBlock, 0, len(items))
	for _, item := range items {
		rawType, ok := item["type"]
		if !ok {
			return nil, errors.New("content block missing type")
		}
		contentType, ok := rawType.(string)
		if !ok {
			return nil, errors.New("content block type must be a string")
		}
		block := ContentBlock{Type: contentType}
		if value, exists := item["text"]; exists && value != nil {
			text, ok := value.(string)
			if !ok {
				return nil, errors.New("content block text must be a string")
			}
			block.Text = &text
		}
		if value, exists := item["thinking"]; exists && value != nil {
			thinking, ok := value.(string)
			if !ok {
				return nil, errors.New("content block thinking must be a string")
			}
			block.Thinking = &thinking
		}
		blocks = append(blocks, block)
	}
	return blocks, nil
}

// sessionPathWithin reports whether path is root itself or lives under root
// (component-wise, like Rust Path::starts_with — no symlink resolution).
func sessionPathWithin(root, path string) bool {
	rel, err := filepath.Rel(root, path)
	if err != nil {
		return false
	}
	if rel == ".." || strings.HasPrefix(rel, ".."+string(filepath.Separator)) {
		return false
	}
	return !filepath.IsAbs(rel)
}

// ---------------------------------------------------------------------------
// Generic session listing / detail / delete
// ---------------------------------------------------------------------------

// droidSessionsDir resolves <factory home>/sessions, honouring the
// Preferences → Paths "factory" override (Rust sessions_dir_for_home).
// DroidSessionsDir returns the generic sessions root — exported for the
// GUI-side watcher in internal/app (watchers stay out of the service layer,
// tui-parity §3).
func DroidSessionsDir() (string, error) {
	return droidSessionsDir()
}

func droidSessionsDir() (string, error) {
	factory := core.FactoryHome()
	if factory == "" {
		return "", errString("Failed to get home directory")
	}
	return filepath.Join(factory, "sessions"), nil
}

// droidSessionSettings reads <session>.settings.json → model + token usage.
// Missing/unreadable/malformed files yield ("unknown", zero usage), exactly
// like the Rust `unwrap_or_default` paths.
func droidSessionSettings(settingsPath string) (string, TokenUsage) {
	usage := TokenUsage{}
	data, err := os.ReadFile(settingsPath) // #nosec G304 -- path derived from the sessions store
	if err != nil {
		return "unknown", usage
	}
	parsed, ok := sessionJSONMap(string(data))
	if !ok {
		return "unknown", usage
	}
	model := "unknown"
	if value, ok := sessionJSONStr(parsed, "model"); ok {
		model = value
	}
	if tu, ok := parsed["tokenUsage"].(map[string]any); ok {
		usage.InputTokens = sessionJSONNumField(tu, "inputTokens")
		usage.OutputTokens = sessionJSONNumField(tu, "outputTokens")
		usage.CacheCreationTokens = sessionJSONNumField(tu, "cacheCreationTokens")
		usage.CacheReadTokens = sessionJSONNumField(tu, "cacheReadTokens")
		usage.ThinkingTokens = sessionJSONNumField(tu, "thinkingTokens")
	}
	return model, usage
}

// droidSessionTitle reads the first JSONL line for the display title
// (sessionTitle → title → "Untitled").
func droidSessionTitle(jsonlPath string) string {
	file, err := os.Open(jsonlPath) // #nosec G304 -- path derived from the sessions store
	if err != nil {
		return "Untitled"
	}
	defer func() { _ = file.Close() }()
	line, err := bufio.NewReader(file).ReadBytes('\n')
	if err != nil && len(line) == 0 {
		return "Untitled"
	}
	parsed, ok := sessionJSONMap(string(line))
	if !ok {
		return "Untitled"
	}
	if value, ok := sessionJSONStr(parsed, "sessionTitle"); ok {
		return value
	}
	if value, ok := sessionJSONStr(parsed, "title"); ok {
		return value
	}
	return "Untitled"
}

func listDroidSessionProjects(root string) ([]SessionProject, error) {
	projects := []SessionProject{}
	if !core.DirExists(root) {
		return projects, nil
	}
	entries, err := os.ReadDir(root)
	if err != nil {
		return nil, fmt.Errorf("Failed to read sessions directory: %w", err)
	}
	for _, entry := range entries {
		if !entry.IsDir() {
			continue
		}
		path := filepath.Join(root, entry.Name())
		// Count .jsonl files (sessions); unreadable directories count 0.
		sessionCount := 0
		if children, err := os.ReadDir(path); err == nil {
			for _, child := range children {
				if !child.IsDir() && filepath.Ext(child.Name()) == ".jsonl" {
					sessionCount++
				}
			}
		}
		if sessionCount == 0 {
			continue
		}
		projects = append(projects, SessionProject{
			Name:         entry.Name(),
			Path:         path,
			SessionCount: uint32(sessionCount), // #nosec G115 -- directory listing bounds the count
			ModifiedAt:   sessionModifiedMs(path),
		})
	}
	sort.SliceStable(projects, func(i, j int) bool {
		return projects[i].ModifiedAt > projects[j].ModifiedAt
	})
	return projects, nil
}

func listDroidSessions(root string, project *string) ([]SessionSummary, error) {
	sessions := []SessionSummary{}
	if !core.DirExists(root) {
		return sessions, nil
	}

	var projectDirs []string
	if project != nil {
		projectDirs = []string{filepath.Join(root, *project)}
	} else {
		entries, err := os.ReadDir(root)
		if err != nil {
			return nil, fmt.Errorf("Failed to read sessions directory: %w", err)
		}
		for _, entry := range entries {
			if entry.IsDir() {
				projectDirs = append(projectDirs, filepath.Join(root, entry.Name()))
			}
		}
	}

	for _, projectDir := range projectDirs {
		info, err := os.Stat(projectDir)
		if err != nil || !info.IsDir() {
			continue
		}
		projectName := filepath.Base(projectDir)
		entries, err := os.ReadDir(projectDir)
		if err != nil {
			continue
		}
		for _, entry := range entries {
			if entry.IsDir() || filepath.Ext(entry.Name()) != ".jsonl" {
				continue
			}
			jsonlPath := filepath.Join(projectDir, entry.Name())
			sessionID := strings.TrimSuffix(entry.Name(), ".jsonl")
			model, usage := droidSessionSettings(filepath.Join(projectDir, sessionID+".settings.json"))
			sessions = append(sessions, SessionSummary{
				ID:         sessionID,
				Title:      droidSessionTitle(jsonlPath),
				Project:    projectName,
				Model:      model,
				ModifiedAt: sessionModifiedMs(jsonlPath),
				TokenUsage: usage,
				Path:       strings.TrimSuffix(jsonlPath, ".jsonl"),
			})
		}
	}

	sort.SliceStable(sessions, func(i, j int) bool {
		return sessions[i].ModifiedAt > sessions[j].ModifiedAt
	})
	return sessions, nil
}

func getDroidSessionDetail(sessionPath string) (SessionDetail, error) {
	jsonlPath := sessionPath + ".jsonl"
	settingsPath := sessionPath + ".settings.json"

	if _, err := os.Stat(jsonlPath); err != nil {
		return SessionDetail{}, errString("Session file not found")
	}

	detail := SessionDetail{
		Title:    "Untitled",
		Project:  filepath.Base(filepath.Dir(jsonlPath)),
		Messages: []SessionMessage{},
	}
	detail.Model, detail.TokenUsage = droidSessionSettings(settingsPath)
	detail.ModifiedAt = sessionModifiedMs(jsonlPath)

	file, err := os.Open(jsonlPath) // #nosec G304 -- path from the frontend session list
	if err != nil {
		return SessionDetail{}, fmt.Errorf("Failed to open session file: %w", err)
	}
	defer func() { _ = file.Close() }()

	reader := bufio.NewReader(file)
	for {
		line, err := reader.ReadBytes('\n')
		if len(line) > 0 {
			parsed, ok := sessionJSONMap(string(line))
			if ok {
				applyDroidSessionLine(&detail, parsed)
			}
		}
		if err != nil {
			break
		}
	}
	return detail, nil
}

// applyDroidSessionLine folds one JSONL record into the detail, mirroring
// the Rust match on `session_start` / `message`.
func applyDroidSessionLine(detail *SessionDetail, parsed map[string]any) {
	msgType, _ := sessionJSONStr(parsed, "type")
	switch msgType {
	case "session_start":
		if value, ok := sessionJSONStr(parsed, "id"); ok {
			detail.ID = value
		}
		title := "Untitled"
		if value, ok := sessionJSONStr(parsed, "sessionTitle"); ok {
			title = value
		} else if value, ok := sessionJSONStr(parsed, "title"); ok {
			title = value
		}
		detail.Title = title
		if value, ok := sessionJSONStr(parsed, "cwd"); ok {
			detail.Cwd = value
		}
	case "message":
		message, _ := parsed["message"].(map[string]any)
		blocks := []ContentBlock{}
		if content, ok := message["content"].([]any); ok {
			for _, raw := range content {
				item, ok := raw.(map[string]any)
				if !ok {
					continue
				}
				contentType := "text"
				if value, ok := sessionJSONStr(item, "type"); ok {
					contentType = value
				}
				// Skip tool_use and tool_result for cleaner display.
				if contentType == "tool_use" || contentType == "tool_result" {
					continue
				}
				block := ContentBlock{Type: contentType}
				if value, ok := sessionJSONStr(item, "text"); ok {
					block.Text = &value
				}
				if value, ok := sessionJSONStr(item, "thinking"); ok {
					block.Thinking = &value
				}
				blocks = append(blocks, block)
			}
		}
		if len(blocks) == 0 {
			return
		}
		id, _ := sessionJSONStr(parsed, "id")
		timestamp, _ := sessionJSONStr(parsed, "timestamp")
		role, _ := sessionJSONStr(message, "role")
		detail.Messages = append(detail.Messages, SessionMessage{
			ID:        id,
			Role:      role,
			Content:   blocks,
			Timestamp: timestamp,
		})
	}
}

func deleteDroidSession(sessionPath string) error {
	sessionsDir, err := droidSessionsDir()
	if err != nil {
		return err
	}
	// Safety guard: only delete files inside the Droid sessions directory.
	// Both sides are canonicalized, so `..` segments and symlinks cannot point
	// the deletion outside it.
	jsonlPath, err := core.CanonicalizeWithin(sessionsDir, sessionPath+".jsonl")
	if err != nil {
		return errString("Session file not found")
	}
	if info, statErr := os.Stat(jsonlPath); statErr != nil || !info.Mode().IsRegular() {
		return errString("Session file not found")
	}
	settingsPath := strings.TrimSuffix(jsonlPath, ".jsonl") + ".settings.json"
	if err := core.RemoveFile(jsonlPath); err != nil {
		return fmt.Errorf("Failed to delete session: %w", err)
	}
	if _, statErr := os.Stat(settingsPath); statErr == nil {
		_ = core.RemoveFile(settingsPath)
	}
	return nil
}

// ListSessionProjects lists the session project directories (Rust
// `list_session_projects`).
func ListSessionProjects() ([]SessionProject, error) {
	root, err := droidSessionsDir()
	if err != nil {
		return nil, err
	}
	return listDroidSessionProjects(root)
}

// ListSessions lists sessions for all projects, or only `project` when set
// (Rust `list_sessions`).
func ListSessions(project *string) ([]SessionSummary, error) {
	root, err := droidSessionsDir()
	if err != nil {
		return nil, err
	}
	return listDroidSessions(root, project)
}

// GetSessionDetail returns one session with its transcript (Rust
// `get_session_detail`).
func GetSessionDetail(sessionPath string) (SessionDetail, error) {
	return getDroidSessionDetail(sessionPath)
}

// DeleteSession removes a session's .jsonl and .settings.json files (Rust
// `delete_session`).
func DeleteSession(sessionPath string) error {
	return deleteDroidSession(sessionPath)
}
