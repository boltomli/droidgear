// OMP (Oh My Pi) session history viewer. Reads its JSONL format directly —
// discovery never opens sessions through OMP itself, which could migrate or
// rewrite files.
//
// Each session is a `.jsonl` file under `<omp home>/sessions/<project>/`,
// starting with an optional `title` slot (the current auto-generated title),
// then the `session` header, followed by `model_change` / `model_usage` /
// `message` / `custom_message` / `title_change` entries. Token usage covers
// the assistant messages; the saved branch is the ancestry of the last
// entry. Mirrors droidgear-core/src/omp_sessions.rs and
// src-tauri/src/commands/omp_sessions.rs.
package read

import (
	"bufio"
	"encoding/json"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"runtime"
	"sort"
	"strings"
	"time"

	"droidgear-wails/internal/core"
)

// ---------------------------------------------------------------------------
// Types (droidgear-core::omp_sessions)
// ---------------------------------------------------------------------------

// OmpTokenUsage mirrors omp_sessions::OmpTokenUsage.
type OmpTokenUsage struct {
	InputTokens         float64 `json:"inputTokens"`
	OutputTokens        float64 `json:"outputTokens"`
	CacheReadTokens     float64 `json:"cacheReadTokens"`
	CacheCreationTokens float64 `json:"cacheCreationTokens"`
	ReasoningTokens     float64 `json:"reasoningTokens"`
	TotalTokens         float64 `json:"totalTokens"`
	Cost                float64 `json:"cost"`
}

// OmpSessionSummary mirrors omp_sessions::OmpSessionSummary.
type OmpSessionSummary struct {
	ID            string        `json:"id"`
	Title         string        `json:"title"`
	Project       string        `json:"project"` // the header's cwd, not the directory encoding
	Model         string        `json:"model"`
	ModelProvider string        `json:"modelProvider"`
	ModifiedAt    float64       `json:"modifiedAt"`
	MessageCount  uint32        `json:"messageCount"`
	TokenUsage    OmpTokenUsage `json:"tokenUsage"`
	Path          string        `json:"path"`
}

// OmpSessionMessage mirrors omp_sessions::OmpSessionMessage.
type OmpSessionMessage struct {
	ID        string         `json:"id"`
	Role      string         `json:"role"`
	Content   []ContentBlock `json:"content"`
	Timestamp string         `json:"timestamp"`
	// IsActiveBranch marks the saved branch (ancestry of the last entry).
	IsActiveBranch bool `json:"isActiveBranch"`
}

// OmpSessionDetail mirrors omp_sessions::OmpSessionDetail.
type OmpSessionDetail struct {
	Summary  OmpSessionSummary   `json:"summary"`
	Messages []OmpSessionMessage `json:"messages"`
}

// ---------------------------------------------------------------------------
// Wire format
// ---------------------------------------------------------------------------

// ompWireBlock mirrors omp_sessions::OmpBlock — OMP blocks carry tool
// metadata (name, intent, partialArgs) in addition to text/thinking.
type ompWireBlock struct {
	Type     string  `json:"type"`
	Text     *string `json:"text"`
	Thinking *string `json:"thinking"`
	Name     *string `json:"name"`
	Intent   *string `json:"intent"`
}

// ompWireContent mirrors the untagged Rust `Content` enum: a plain string
// or an array of typed blocks.
type ompWireContent struct {
	isText bool
	text   string
	blocks []ompWireBlock
}

func (c *ompWireContent) UnmarshalJSON(data []byte) error {
	trimmed := strings.TrimSpace(string(data))
	switch {
	case strings.HasPrefix(trimmed, "\""):
		var text string
		if err := json.Unmarshal(data, &text); err != nil {
			return err
		}
		c.isText = true
		c.text = text
		return nil
	case strings.HasPrefix(trimmed, "["):
		blocks, err := unmarshalOmpBlocks(data)
		if err != nil {
			return err
		}
		c.blocks = blocks
		return nil
	}
	return fmt.Errorf("omp content must be a string or block array")
}

// unmarshalOmpBlocks parses a block array with serde OmpBlock strictness:
// every element must be an object with a string "type"; text/thinking/
// name/intent must be strings or null.
func unmarshalOmpBlocks(data []byte) ([]ompWireBlock, error) {
	var items []map[string]any
	if err := json.Unmarshal(data, &items); err != nil {
		return nil, err
	}
	blocks := make([]ompWireBlock, 0, len(items))
	for _, item := range items {
		rawType, ok := item["type"].(string)
		if !ok {
			return nil, fmt.Errorf("omp block type must be a string")
		}
		block := ompWireBlock{Type: rawType}
		if value, exists := item["text"]; exists && value != nil {
			text, ok := value.(string)
			if !ok {
				return nil, fmt.Errorf("omp block text must be a string")
			}
			block.Text = &text
		}
		if value, exists := item["thinking"]; exists && value != nil {
			thinking, ok := value.(string)
			if !ok {
				return nil, fmt.Errorf("omp block thinking must be a string")
			}
			block.Thinking = &thinking
		}
		if value, exists := item["name"]; exists && value != nil {
			name, ok := value.(string)
			if !ok {
				return nil, fmt.Errorf("omp block name must be a string")
			}
			block.Name = &name
		}
		if value, exists := item["intent"]; exists && value != nil {
			intent, ok := value.(string)
			if !ok {
				return nil, fmt.Errorf("omp block intent must be a string")
			}
			block.Intent = &intent
		}
		blocks = append(blocks, block)
	}
	return blocks, nil
}

// ompWireUsage mirrors omp_sessions::Usage (serde default on every field).
type ompWireUsage struct {
	Input           float64     `json:"input"`
	Output          float64     `json:"output"`
	CacheRead       float64     `json:"cacheRead"`
	CacheWrite      float64     `json:"cacheWrite"`
	ReasoningTokens float64     `json:"reasoningTokens"`
	TotalTokens     *float64    `json:"totalTokens"`
	Cost            ompWireCost `json:"cost"`
}

// ompWireCost mirrors omp_sessions::Cost { total }.
type ompWireCost struct {
	Total float64 `json:"total"`
}

// ompWireMessage mirrors omp_sessions::Message.
type ompWireMessage struct {
	Role      string          `json:"role"`
	Content   *ompWireContent `json:"content"`
	Model     *string         `json:"model"`
	Provider  *string         `json:"provider"`
	Timestamp *float64        `json:"timestamp"`
	Usage     *ompWireUsage   `json:"usage"`
}

// ompWireEntry mirrors omp_sessions::Entry.
type ompWireEntry struct {
	Kind      string          `json:"type"`
	ID        string          `json:"id"`
	ParentID  *string         `json:"parentId"`
	Version   *uint32         `json:"version"`
	Timestamp string          `json:"timestamp"`
	Cwd       string          `json:"cwd"`
	Title     *string         `json:"title"`
	Model     *string         `json:"model"`
	Message   *ompWireMessage `json:"message"`
	Content   *ompWireContent `json:"content"`
	Display   bool            `json:"display"`
	Usage     *ompWireUsage   `json:"usage"`
}

// ompHistoryEntry is one parsed record reduced to what the history walk
// needs (id / parent / model / displayable message).
type ompHistoryEntry struct {
	ID       string
	ParentID *string
	Model    *sessionModelPair
	Message  *OmpSessionMessage
}

// ---------------------------------------------------------------------------
// Path helpers
// ---------------------------------------------------------------------------

// ompSessionsDir resolves `<omp home>/sessions`, honouring the Preferences
// → Paths "omp" override (Rust omp_sessions_dir).
// OmpSessionsDir returns the sessions directory for the current home — exported for the GUI-side watcher in internal/app.
func OmpSessionsDir() (string, error) {
	return ompSessionsDir()
}

func ompSessionsDir() (string, error) {
	home := core.OmpHome()
	if home == "" {
		return "", errString("Failed to get home directory")
	}
	return filepath.Join(home, "sessions"), nil
}

// ompNativePath normalises forward slashes on Windows (Rust stores paths
// with MAIN_SEPARATOR).
func ompNativePath(path string) string {
	if runtime.GOOS == "windows" {
		return strings.ReplaceAll(path, "/", `\`)
	}
	return path
}

// splitOmpModel splits OMP's combined `provider/model` string
// (e.g. `litellm/azure-gpt-5.6-sol`).
func splitOmpModel(value string) sessionModelPair {
	if index := strings.Index(value, "/"); index >= 0 {
		return sessionModelPair{provider: value[:index], model: value[index+1:]}
	}
	return sessionModelPair{provider: "", model: value}
}

// ---------------------------------------------------------------------------
// Discovery
// ---------------------------------------------------------------------------

// collectOmpFiles recurses for .jsonl files without following symlinks;
// nested read failures are ignored (Rust logs a warning), a root failure
// propagates.
func collectOmpFiles(dir string, files *[]string) error {
	entries, err := os.ReadDir(dir)
	if err != nil {
		return fmt.Errorf("Failed to read OMP sessions: %w", err)
	}
	for _, entry := range entries {
		path := filepath.Join(dir, entry.Name())
		// DirEntry type checks do not follow symlinks: symlinked dirs are
		// not recursed and symlinked files are not collected.
		if entry.IsDir() {
			if err := collectOmpFiles(path, files); err != nil {
				continue
			}
		} else if entry.Type().IsRegular() && filepath.Ext(entry.Name()) == ".jsonl" {
			*files = append(*files, path)
		}
	}
	return nil
}

// ---------------------------------------------------------------------------
// Parsing helpers
// ---------------------------------------------------------------------------

// ompTimestampMs parses an RFC3339 timestamp into epoch millis.
func ompTimestampMs(value string) *float64 {
	if value == "" {
		return nil
	}
	parsed, err := time.Parse(time.RFC3339, value)
	if err != nil {
		return nil
	}
	ms := float64(parsed.UnixMilli())
	return &ms
}

func (t *OmpTokenUsage) add(usage *ompWireUsage) {
	t.InputTokens += usage.Input
	t.OutputTokens += usage.Output
	t.CacheReadTokens += usage.CacheRead
	t.CacheCreationTokens += usage.CacheWrite
	t.ReasoningTokens += usage.ReasoningTokens
	if usage.TotalTokens != nil {
		t.TotalTokens += *usage.TotalTokens
	} else {
		t.TotalTokens += usage.Input + usage.Output + usage.CacheRead + usage.CacheWrite
	}
	t.Cost += usage.Cost.Total
}

func ompTextBlock(kind, text string) ContentBlock {
	return ContentBlock{Type: kind, Text: &text}
}

// ompToolCallSummary summarizes a tool call into a compact display line
// (`name: intent`, or just the name).
func ompToolCallSummary(block ompWireBlock) string {
	name := "tool"
	if block.Name != nil {
		name = *block.Name
	}
	if block.Intent != nil {
		if intent := strings.TrimSpace(*block.Intent); intent != "" {
			return name + ": " + intent
		}
	}
	return name
}

// displayOmpBlocks keeps only renderable blocks: non-empty text, non-empty
// thinking, assistant tool calls (as a summary line) and images (rendered
// without a text payload).
func displayOmpBlocks(content *ompWireContent, assistant bool) []ContentBlock {
	if content == nil {
		return []ContentBlock{}
	}
	if content.isText {
		if content.text == "" {
			return []ContentBlock{}
		}
		text := content.text
		return []ContentBlock{ompTextBlock("text", text)}
	}
	blocks := []ContentBlock{}
	for _, block := range content.blocks {
		switch block.Type {
		case "text":
			if block.Text != nil && *block.Text != "" {
				text := *block.Text
				blocks = append(blocks, ompTextBlock("text", text))
			}
		case "thinking":
			if block.Thinking != nil && *block.Thinking != "" {
				thinking := *block.Thinking
				blocks = append(blocks, ContentBlock{Type: "thinking", Thinking: &thinking})
			}
		case "toolCall":
			// Assistant tool calls are shown as a summary line; tool
			// results arrive as their own `toolResult` messages.
			if assistant {
				blocks = append(blocks, ompTextBlock("toolCall", ompToolCallSummary(block)))
			}
		case "image":
			blocks = append(blocks, ContentBlock{Type: "image"})
		}
	}
	return blocks
}

// ---------------------------------------------------------------------------
// Session reading
// ---------------------------------------------------------------------------

func readOmpSession(path string, includeMessages bool) (OmpSessionDetail, error) {
	file, err := os.Open(path) // #nosec G304 -- path from the frontend session list
	if err != nil {
		return OmpSessionDetail{}, fmt.Errorf("Failed to open OMP session: %w", err)
	}
	defer func() { _ = file.Close() }()
	modified := sessionModifiedMs(path)

	var header *ompWireEntry
	entries := []ompHistoryEntry{}
	// The dedicated `title` slot reflects the current title; `title_change`
	// entries are the change history used only as a fallback.
	var currentTitle *string
	var lastTitleChange *string
	firstMessage := ""
	usage := OmpTokenUsage{}
	var messageCount uint32
	var activity *float64

	// LF is the only record delimiter; malformed/incomplete trailing
	// records are ignored without repairing or rewriting the file.
	reader := bufio.NewReader(file)
	lineNumber := 0
	for {
		line, err := reader.ReadBytes('\n')
		if err != nil && err != io.EOF {
			return OmpSessionDetail{}, fmt.Errorf("Failed to read OMP session: %w", err)
		}
		if len(line) > 0 {
			currentLine := lineNumber
			lineNumber++
			var entry ompWireEntry
			if err := json.Unmarshal(line, &entry); err != nil {
				continue
			}
			// The `session` header identifies the file; it may be preceded
			// by a `title` slot, so it is not required to be the first
			// record.
			switch entry.Kind {
			case "session":
				if header == nil {
					if entry.ID == "" {
						return OmpSessionDetail{}, errString("Not an OMP session file")
					}
					header = &entry
				}
				continue
			case "title":
				currentTitle = ompNonEmptyTitle(entry.Title)
				continue
			case "title_change":
				lastTitleChange = ompNonEmptyTitle(entry.Title)
				continue
			}
			if header == nil {
				// Ignore records that appear before the session header.
				continue
			}
			if entry.ID == "" {
				entry.ID = fmt.Sprintf("line-%d", currentLine)
			}
			if header.Version == nil || *header.Version < 2 {
				// v1 sessions have no parent ids: chain to the previous entry.
				var parent *string
				if len(entries) > 0 {
					previous := entries[len(entries)-1].ID
					parent = &previous
				}
				entry.ParentID = parent
			}

			var model *sessionModelPair
			role := ""
			var blocks []ContentBlock
			switch entry.Kind {
			case "model_change":
				if entry.Model != nil {
					pair := splitOmpModel(*entry.Model)
					model = &pair
				}
			case "message":
				// The picker counts every message entry; the detail view
				// renders the conversational roles it can show.
				messageCount++
				if entry.Message != nil {
					message := entry.Message
					if message.Role == "assistant" || message.Role == "toolResult" {
						if message.Usage != nil {
							usage.add(message.Usage)
						}
					}
					switch message.Role {
					case "assistant":
						if message.Model != nil && *message.Model != "" {
							provider := ""
							if message.Provider != nil {
								provider = *message.Provider
							}
							model = &sessionModelPair{provider: provider, model: *message.Model}
						}
						blocks = displayOmpBlocks(message.Content, true)
						ompAdvanceActivity(&activity, message.Timestamp, entry.Timestamp)
						role = message.Role
					case "user":
						blocks = displayOmpBlocks(message.Content, false)
						ompAdvanceActivity(&activity, message.Timestamp, entry.Timestamp)
						if firstMessage == "" {
							firstMessage = joinTextBlocks(blocks)
						}
						role = message.Role
					case "toolResult":
						blocks = displayOmpBlocks(message.Content, false)
						role = message.Role
					}
				}
			case "model_usage":
				// Auxiliary (e.g. judge) usage entries are intentionally not
				// summed; assistant message usage is the session's headline.
			case "custom_message":
				if entry.Display {
					blocks = displayOmpBlocks(entry.Content, false)
					role = "system"
				}
			}

			var message *OmpSessionMessage
			if includeMessages && len(blocks) > 0 {
				message = &OmpSessionMessage{
					ID:        entry.ID,
					Role:      role,
					Content:   blocks,
					Timestamp: entry.Timestamp,
				}
			}
			entries = append(entries, ompHistoryEntry{
				ID:       entry.ID,
				ParentID: entry.ParentID,
				Model:    model,
				Message:  message,
			})
		}
		if err == io.EOF {
			break
		}
	}
	if header == nil {
		return OmpSessionDetail{}, errString("Not an OMP session file")
	}

	// The saved branch is the ancestry of the last entry; the first model
	// encountered on it wins.
	byID := map[string]int{}
	for index, entry := range entries {
		byID[entry.ID] = index
	}
	active := map[int]bool{}
	model := (*sessionModelPair)(nil)
	current := len(entries) - 1
	for current >= 0 {
		if active[current] {
			break
		}
		active[current] = true
		entry := entries[current]
		if model == nil {
			model = entry.Model
		}
		if entry.ParentID == nil {
			break
		}
		next, ok := byID[*entry.ParentID]
		if !ok {
			break
		}
		current = next
	}

	modelProvider := ""
	modelName := "unknown"
	if model != nil {
		modelProvider = model.provider
		modelName = model.model
	}

	title := ""
	if chosen := ompFirstNonNil(currentTitle, lastTitleChange); chosen != nil {
		title = sessionTitleFromText(*chosen, 60)
	}
	if title == "" {
		derived := sessionTitleFromText(firstMessage, 60)
		if derived == "" {
			title = "New Session"
		} else {
			title = derived
		}
	}

	modifiedAt := modified
	if activity != nil && *activity > 0 {
		modifiedAt = *activity
	} else if headerTime := ompTimestampMs(header.Timestamp); headerTime != nil {
		modifiedAt = *headerTime
	}

	messages := []OmpSessionMessage{}
	for index, entry := range entries {
		if entry.Message != nil {
			message := *entry.Message
			message.IsActiveBranch = active[index]
			messages = append(messages, message)
		}
	}

	return OmpSessionDetail{
		Summary: OmpSessionSummary{
			ID:            header.ID,
			Title:         title,
			Project:       header.Cwd,
			Model:         modelName,
			ModelProvider: modelProvider,
			ModifiedAt:    modifiedAt,
			MessageCount:  messageCount,
			TokenUsage:    usage,
			Path:          ompNativePath(path),
		},
		Messages: messages,
	}, nil
}

// ompNonEmptyTitle trims a title slot value; empty slots clear the stored
// title (Rust `.take().map(trim).filter(non-empty)`).
func ompNonEmptyTitle(value *string) *string {
	if value == nil {
		return nil
	}
	trimmed := strings.TrimSpace(*value)
	if trimmed == "" {
		return nil
	}
	return &trimmed
}

func ompFirstNonNil(values ...*string) *string {
	for _, value := range values {
		if value != nil {
			return value
		}
	}
	return nil
}

// ompAdvanceActivity keeps the newest user/assistant activity timestamp
// (message timestamp, falling back to the entry's RFC3339 timestamp).
func ompAdvanceActivity(activity **float64, messageTime *float64, entryTimestamp string) {
	moment := messageTime
	if moment == nil {
		moment = ompTimestampMs(entryTimestamp)
	}
	if moment == nil {
		return
	}
	previous := 0.0
	if *activity != nil {
		previous = **activity
	}
	if *moment > previous {
		*activity = moment
	} else {
		value := previous
		*activity = &value
	}
}

// validateOmpSessionPath canonicalises root and path, requiring path to be
// a .jsonl file inside the sessions directory.
func validateOmpSessionPath(root, sessionPath string) (string, error) {
	canonicalRoot, err := filepath.EvalSymlinks(root)
	if err != nil {
		return "", fmt.Errorf("OMP sessions directory unavailable: %w", err)
	}
	canonicalPath, err := filepath.EvalSymlinks(sessionPath)
	if err != nil {
		return "", fmt.Errorf("OMP session unavailable: %w", err)
	}
	info, err := os.Stat(canonicalPath)
	if err != nil || info.IsDir() ||
		filepath.Ext(canonicalPath) != ".jsonl" ||
		!sessionPathWithin(canonicalRoot, canonicalPath) {
		return "", errString("Session must be a JSONL file inside the OMP sessions directory")
	}
	return canonicalPath, nil
}

// ---------------------------------------------------------------------------
// Listing
// ---------------------------------------------------------------------------

func listOmpSessions(root string) ([]OmpSessionSummary, error) {
	sessions := []OmpSessionSummary{}
	if !core.DirExists(root) {
		return sessions, nil
	}
	files := []string{}
	if err := collectOmpFiles(root, &files); err != nil {
		return nil, err
	}
	for _, path := range files {
		detail, err := readOmpSession(path, false)
		if err != nil {
			// Unreadable sessions are skipped (Rust logs a warning).
			continue
		}
		sessions = append(sessions, detail.Summary)
	}
	sort.SliceStable(sessions, func(i, j int) bool {
		if sessions[i].ModifiedAt != sessions[j].ModifiedAt {
			return sessions[i].ModifiedAt > sessions[j].ModifiedAt
		}
		return sessions[i].Path < sessions[j].Path
	})
	return sessions, nil
}

// ---------------------------------------------------------------------------
// Commands (src-tauri/src/commands/omp_sessions.rs)
// ---------------------------------------------------------------------------

// ListOmpSessions lists OMP sessions, newest first (Rust `list_omp_sessions`).
func ListOmpSessions() ([]OmpSessionSummary, error) {
	root, err := ompSessionsDir()
	if err != nil {
		return nil, err
	}
	return listOmpSessions(root)
}

// GetOmpSessionDetail returns one OMP session with its transcript (Rust
// `get_omp_session_detail`).
func GetOmpSessionDetail(sessionPath string) (OmpSessionDetail, error) {
	root, err := ompSessionsDir()
	if err != nil {
		return OmpSessionDetail{}, err
	}
	path, err := validateOmpSessionPath(root, sessionPath)
	if err != nil {
		return OmpSessionDetail{}, err
	}
	return readOmpSession(path, true)
}

// DeleteOmpSession removes a validated session file (Rust
// `delete_omp_session`); the header is validated as well so unrelated
// JSONL files cannot be removed.
func DeleteOmpSession(sessionPath string) error {
	root, err := ompSessionsDir()
	if err != nil {
		return err
	}
	path, err := validateOmpSessionPath(root, sessionPath)
	if err != nil {
		return err
	}
	if _, err := readOmpSession(path, false); err != nil {
		return err
	}
	if err := core.RemoveFile(path); err != nil {
		return fmt.Errorf("Failed to delete OMP session: %w", err)
	}
	return nil
}
