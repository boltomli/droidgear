// Pi session history viewer. Reads the documented JSONL format (v1–v3)
// directly — discovery never opens sessions through Pi's SDK, which could
// migrate/rewrite files (see earendil-works/pi coding-agent docs/sessions.md).
//
// Usage covers the entire file; the saved branch is the ancestry of the
// last entry. Mirrors droidgear-core/src/pi_sessions.rs and
// src-tauri/src/commands/pi_sessions.rs.
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
// Types (droidgear-core::pi_sessions)
// ---------------------------------------------------------------------------

// PiTokenUsage mirrors pi_sessions::PiTokenUsage.
type PiTokenUsage struct {
	InputTokens         float64 `json:"inputTokens"`
	OutputTokens        float64 `json:"outputTokens"`
	CacheReadTokens     float64 `json:"cacheReadTokens"`
	CacheCreationTokens float64 `json:"cacheCreationTokens"`
	ReasoningTokens     float64 `json:"reasoningTokens"`
	TotalTokens         float64 `json:"totalTokens"`
	Cost                float64 `json:"cost"`
}

// PiSessionSummary mirrors pi_sessions::PiSessionSummary.
type PiSessionSummary struct {
	ID            string       `json:"id"`
	Title         string       `json:"title"`
	Project       string       `json:"project"` // the header's cwd, not the directory encoding
	Model         string       `json:"model"`
	ModelProvider string       `json:"modelProvider"`
	ModifiedAt    float64      `json:"modifiedAt"`
	MessageCount  uint32       `json:"messageCount"`
	TokenUsage    PiTokenUsage `json:"tokenUsage"`
	Path          string       `json:"path"`
}

// PiSessionMessage mirrors pi_sessions::PiSessionMessage.
type PiSessionMessage struct {
	ID        string         `json:"id"`
	Role      string         `json:"role"`
	Content   []ContentBlock `json:"content"`
	Timestamp string         `json:"timestamp"`
	// IsActiveBranch marks the saved branch (ancestry of the last entry).
	IsActiveBranch bool `json:"isActiveBranch"`
}

// PiSessionDetail mirrors pi_sessions::PiSessionDetail.
type PiSessionDetail struct {
	Summary  PiSessionSummary   `json:"summary"`
	Messages []PiSessionMessage `json:"messages"`
}

// ---------------------------------------------------------------------------
// Wire format (deserialize only the fields history needs — tool arguments,
// images and provider signatures can be large and are not copied across
// the UI bridge)
// ---------------------------------------------------------------------------

// piWireContent mirrors the untagged Rust `Content` enum: a plain string or
// an array of typed blocks.
type piWireContent struct {
	isText bool
	text   string
	blocks []ContentBlock
}

func (c *piWireContent) UnmarshalJSON(data []byte) error {
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
		blocks, err := unmarshalContentBlocks(data)
		if err != nil {
			return err
		}
		c.blocks = blocks
		return nil
	}
	return fmt.Errorf("pi content must be a string or block array")
}

// piWireUsage mirrors pi_sessions::Usage (serde default on every field).
type piWireUsage struct {
	Input       float64    `json:"input"`
	Output      float64    `json:"output"`
	CacheRead   float64    `json:"cacheRead"`
	CacheWrite  float64    `json:"cacheWrite"`
	Reasoning   float64    `json:"reasoning"`
	TotalTokens *float64   `json:"totalTokens"`
	Cost        piWireCost `json:"cost"`
}

// piWireCost mirrors pi_sessions::Cost { total }.
type piWireCost struct {
	Total float64 `json:"total"`
}

// piWireMessage mirrors pi_sessions::Message.
type piWireMessage struct {
	Role         string         `json:"role"`
	Content      *piWireContent `json:"content"`
	Model        string         `json:"model"`
	Provider     string         `json:"provider"`
	Timestamp    *float64       `json:"timestamp"`
	Display      bool           `json:"display"`
	ErrorMessage *string        `json:"errorMessage"`
	Usage        *piWireUsage   `json:"usage"`
}

// piWireEntry mirrors pi_sessions::Entry.
type piWireEntry struct {
	Kind      string         `json:"type"`
	ID        string         `json:"id"`
	ParentID  *string        `json:"parentId"`
	Version   *uint32        `json:"version"`
	Timestamp string         `json:"timestamp"`
	Cwd       string         `json:"cwd"`
	Name      *string        `json:"name"`
	ModelID   string         `json:"modelId"`
	Provider  string         `json:"provider"`
	Message   *piWireMessage `json:"message"`
	Content   *piWireContent `json:"content"`
	Display   bool           `json:"display"`
	Summary   *string        `json:"summary"`
	Usage     *piWireUsage   `json:"usage"`
}

// piHistoryEntry is one parsed record reduced to what the history walk
// needs (id / parent / model / displayable message).
type piHistoryEntry struct {
	ID       string
	ParentID *string
	Model    *sessionModelPair
	Message  *PiSessionMessage
}

// ---------------------------------------------------------------------------
// Path helpers
// ---------------------------------------------------------------------------

// piExpandPath expands a leading "~" the way the Rust core does (used for
// the PI_CODING_AGENT_* environment overrides).
func piExpandPath(home, value string) string {
	if value == "~" {
		return home
	}
	if strings.HasPrefix(value, "~/") || strings.HasPrefix(value, `~\`) {
		return filepath.Join(home, value[2:])
	}
	return value
}

// piSessionsDir resolves the Pi sessions root: PI_CODING_AGENT_SESSION_DIR
// wins outright; otherwise the settings "pi" override beats
// PI_CODING_AGENT_DIR, which beats ~/.pi/agent (Rust resolve_sessions_dir).
// PiSessionsDir returns the sessions directory for the current home — exported for the GUI-side watcher in internal/app.
func PiSessionsDir() (string, error) {
	return piSessionsDir()
}

func piSessionsDir() (string, error) {
	home, err := core.HomeDir()
	if err != nil {
		return "", errString("Failed to get home directory")
	}
	path, isDefault, err := core.EffectivePathForKey("pi")
	if err != nil {
		return "", errString("Failed to get home directory")
	}
	agentDir := path
	if isDefault {
		// No settings override: environment variable, then the default.
		if env := os.Getenv("PI_CODING_AGENT_DIR"); env != "" {
			agentDir = piExpandPath(home, env)
		}
	} else {
		agentDir = piExpandPath(home, path)
	}
	if env := os.Getenv("PI_CODING_AGENT_SESSION_DIR"); env != "" {
		return piExpandPath(home, env), nil
	}
	return filepath.Join(agentDir, "sessions"), nil
}

// piNativePath normalises forward slashes on Windows (Rust stores paths
// with MAIN_SEPARATOR).
func piNativePath(path string) string {
	if runtime.GOOS == "windows" {
		return strings.ReplaceAll(path, "/", `\`)
	}
	return path
}

// ---------------------------------------------------------------------------
// Discovery
// ---------------------------------------------------------------------------

// collectPiFiles recurses for .jsonl files without following symlinks;
// nested read failures are ignored (Rust logs a warning), a root failure
// propagates.
func collectPiFiles(dir string, files *[]string) error {
	entries, err := os.ReadDir(dir)
	if err != nil {
		return fmt.Errorf("Failed to read Pi sessions: %w", err)
	}
	for _, entry := range entries {
		path := filepath.Join(dir, entry.Name())
		// DirEntry type checks do not follow symlinks: symlinked dirs are
		// not recursed (cycles / files outside the root) and symlinked
		// files are not collected.
		if entry.IsDir() {
			if err := collectPiFiles(path, files); err != nil {
				// Nested failures are logged and ignored.
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

// piTimestampMs parses an RFC3339 timestamp into epoch millis (Rust
// chrono parse_from_rfc3339 → timestamp_millis).
func piTimestampMs(value string) *float64 {
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

func (t *PiTokenUsage) add(usage *piWireUsage) {
	t.InputTokens += usage.Input
	t.OutputTokens += usage.Output
	t.CacheReadTokens += usage.CacheRead
	t.CacheCreationTokens += usage.CacheWrite
	t.ReasoningTokens += usage.Reasoning
	if usage.TotalTokens != nil {
		t.TotalTokens += *usage.TotalTokens
	} else {
		t.TotalTokens += usage.Input + usage.Output + usage.CacheRead + usage.CacheWrite
	}
	t.Cost += usage.Cost.Total
}

// piTextBlock builds a display text block.
func piTextBlock(kind, text string) ContentBlock {
	return ContentBlock{Type: kind, Text: &text}
}

// displayPiBlocks keeps only renderable blocks: non-empty text, non-empty
// thinking, and images (rendered as an empty-text marker).
func displayPiBlocks(content *piWireContent) []ContentBlock {
	if content == nil {
		return []ContentBlock{}
	}
	if content.isText {
		if content.text == "" {
			return []ContentBlock{}
		}
		text := content.text
		return []ContentBlock{piTextBlock("text", text)}
	}
	blocks := []ContentBlock{}
	for _, block := range content.blocks {
		switch block.Type {
		case "text":
			if block.Text != nil && *block.Text != "" {
				text := *block.Text
				blocks = append(blocks, piTextBlock("text", text))
			}
		case "thinking":
			if block.Thinking != nil && *block.Thinking != "" {
				thinking := *block.Thinking
				blocks = append(blocks, ContentBlock{Type: "thinking", Thinking: &thinking})
			}
		case "image":
			blocks = append(blocks, piTextBlock("image", ""))
		}
	}
	return blocks
}

// joinTextBlocks concatenates every text block (first-prompt derivation).
func joinTextBlocks(blocks []ContentBlock) string {
	parts := []string{}
	for _, block := range blocks {
		if block.Type == "text" && block.Text != nil {
			parts = append(parts, *block.Text)
		}
	}
	return strings.Join(parts, " ")
}

// ---------------------------------------------------------------------------
// Session reading
// ---------------------------------------------------------------------------

func readPiSession(path string, includeMessages bool) (PiSessionDetail, error) {
	file, err := os.Open(path) // #nosec G304 -- path from the frontend session list
	if err != nil {
		return PiSessionDetail{}, fmt.Errorf("Failed to open Pi session: %w", err)
	}
	defer func() { _ = file.Close() }()
	modified := sessionModifiedMs(path)

	var header *piWireEntry
	entries := []piHistoryEntry{}
	var name *string
	firstMessage := ""
	usage := PiTokenUsage{}
	var messageCount uint32
	var activity *float64

	// LF is the only record delimiter; malformed/incomplete trailing
	// records are ignored without repairing or rewriting the file.
	reader := bufio.NewReader(file)
	lineNumber := 0
	for {
		line, err := reader.ReadBytes('\n')
		if err != nil && err != io.EOF {
			return PiSessionDetail{}, fmt.Errorf("Failed to read Pi session: %w", err)
		}
		if len(line) > 0 {
			currentLine := lineNumber
			lineNumber++
			var entry piWireEntry
			if err := json.Unmarshal(line, &entry); err != nil {
				continue
			}
			if header == nil {
				// The first parsed record must be the session header.
				if entry.Kind != "session" || entry.ID == "" {
					return PiSessionDetail{}, errString("Not a Pi session file")
				}
				header = &entry
				continue
			}
			if entry.Kind == "session_info" {
				// An explicitly cleared name replaces (and clears) the
				// previous one — the list falls back to the first prompt.
				name = nil
				if entry.Name != nil {
					trimmed := strings.TrimSpace(*entry.Name)
					if trimmed != "" {
						name = &trimmed
					}
				}
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
				model = &sessionModelPair{provider: entry.Provider, model: entry.ModelID}
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
					if message.Role == "assistant" && message.Model != "" {
						model = &sessionModelPair{provider: message.Provider, model: message.Model}
					}
					if message.Role == "user" || message.Role == "assistant" {
						var moment *float64
						if message.Timestamp != nil {
							moment = message.Timestamp
						} else {
							moment = piTimestampMs(entry.Timestamp)
						}
						if moment != nil {
							previous := 0.0
							if activity != nil {
								previous = *activity
							}
							if *moment > previous {
								activity = moment
							} else {
								value := previous
								activity = &value
							}
						}
						blocks = displayPiBlocks(message.Content)
						if message.Role == "user" && firstMessage == "" {
							firstMessage = joinTextBlocks(blocks)
						}
						if message.ErrorMessage != nil && *message.ErrorMessage != "" {
							blocks = append(blocks, piTextBlock("error", *message.ErrorMessage))
						}
						role = message.Role
					} else if (message.Role == "custom" || message.Role == "hookMessage") && message.Display {
						blocks = displayPiBlocks(message.Content)
						role = "system"
					}
				}
			case "compaction", "branch_summary":
				if entry.Usage != nil {
					usage.add(entry.Usage)
				}
				if entry.Summary != nil {
					// retainedTail repeats existing messages; don't count it again.
					blocks = append(blocks, piTextBlock(entry.Kind, *entry.Summary))
					role = "system"
				}
			case "custom_message":
				if entry.Display {
					blocks = displayPiBlocks(entry.Content)
					role = "system"
				}
			}

			var message *PiSessionMessage
			if includeMessages && len(blocks) > 0 {
				message = &PiSessionMessage{
					ID:        entry.ID,
					Role:      role,
					Content:   blocks,
					Timestamp: entry.Timestamp,
				}
			}
			entries = append(entries, piHistoryEntry{
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
		return PiSessionDetail{}, errString("Not a Pi session file")
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
	if name != nil {
		title = *name
	} else {
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
	} else if headerTime := piTimestampMs(header.Timestamp); headerTime != nil {
		modifiedAt = *headerTime
	}

	messages := []PiSessionMessage{}
	for index, entry := range entries {
		if entry.Message != nil {
			message := *entry.Message
			message.IsActiveBranch = active[index]
			messages = append(messages, message)
		}
	}

	return PiSessionDetail{
		Summary: PiSessionSummary{
			ID:            header.ID,
			Title:         title,
			Project:       header.Cwd,
			Model:         modelName,
			ModelProvider: modelProvider,
			ModifiedAt:    modifiedAt,
			MessageCount:  messageCount,
			TokenUsage:    usage,
			Path:          piNativePath(path),
		},
		Messages: messages,
	}, nil
}

// validatePiSessionPath canonicalises root and path, requiring path to be
// a .jsonl file inside the sessions directory.
func validatePiSessionPath(root, sessionPath string) (string, error) {
	canonicalRoot, err := filepath.EvalSymlinks(root)
	if err != nil {
		return "", fmt.Errorf("Pi sessions directory unavailable: %w", err)
	}
	canonicalPath, err := filepath.EvalSymlinks(sessionPath)
	if err != nil {
		return "", fmt.Errorf("Pi session unavailable: %w", err)
	}
	info, err := os.Stat(canonicalPath)
	if err != nil || info.IsDir() ||
		filepath.Ext(canonicalPath) != ".jsonl" ||
		!sessionPathWithin(canonicalRoot, canonicalPath) {
		return "", errString("Session must be a JSONL file inside the Pi sessions directory")
	}
	return canonicalPath, nil
}

// ---------------------------------------------------------------------------
// Listing
// ---------------------------------------------------------------------------

func listPiSessions(root string) ([]PiSessionSummary, error) {
	sessions := []PiSessionSummary{}
	if !core.DirExists(root) {
		return sessions, nil
	}
	files := []string{}
	if err := collectPiFiles(root, &files); err != nil {
		return nil, err
	}
	for _, path := range files {
		detail, err := readPiSession(path, false)
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
// Commands (src-tauri/src/commands/pi_sessions.rs)
// ---------------------------------------------------------------------------

// ListPiSessions lists Pi sessions, newest first (Rust `list_pi_sessions`).
func ListPiSessions() ([]PiSessionSummary, error) {
	root, err := piSessionsDir()
	if err != nil {
		return nil, err
	}
	return listPiSessions(root)
}

// GetPiSessionDetail returns one Pi session with its transcript (Rust
// `get_pi_session_detail`).
func GetPiSessionDetail(sessionPath string) (PiSessionDetail, error) {
	root, err := piSessionsDir()
	if err != nil {
		return PiSessionDetail{}, err
	}
	path, err := validatePiSessionPath(root, sessionPath)
	if err != nil {
		return PiSessionDetail{}, err
	}
	return readPiSession(path, true)
}

// DeletePiSession removes a validated session file (Rust `delete_pi_session`);
// the header is validated as well so unrelated JSONL files cannot be removed.
func DeletePiSession(sessionPath string) error {
	root, err := piSessionsDir()
	if err != nil {
		return err
	}
	path, err := validatePiSessionPath(root, sessionPath)
	if err != nil {
		return err
	}
	if _, err := readPiSession(path, false); err != nil {
		return err
	}
	if err := os.Remove(path); err != nil {
		return fmt.Errorf("Failed to delete Pi session: %w", err)
	}
	return nil
}
