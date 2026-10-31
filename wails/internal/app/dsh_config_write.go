package app

// Write side of the dsh_config port: schema-validated patch rewrites with
// DSH's own locking and optimistic-concurrency semantics.

import (
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"time"

	"droidgear-wails/internal/core"

	"gopkg.in/yaml.v3"
)

// ---------------------------------------------------------------------------
// Validation / write preparation
// ---------------------------------------------------------------------------

func prepareConfigForWrite(entryID string, config any, schema *schemaDoc) (any, error) {
	object, ok := config.(map[string]any)
	if !ok {
		return nil, errString("Configuration must be a JSON object")
	}

	// Drop any $secret markers the UI echoes back.
	for key, value := range object {
		if isSecretMarker(value) {
			delete(object, key)
		}
	}

	meta := schema.Entries[entryID]
	for _, field := range meta.Fields {
		if !field.Secret {
			continue
		}
		value, present := object[field.Name]
		if !present {
			continue
		}
		if isSecretMarker(value) {
			// The UI only knows whether a secret is set; dropping the key
			// keeps the inherited value instead of writing a placeholder.
			delete(object, field.Name)
		} else if value != nil {
			return nil, fmt.Errorf("Field '%s' is a secret; store it in ~/.dsh/.credentials.yaml (or the DSH Models page) instead of cordis.patch.yml", field.Name)
		}
	}

	if err := validateConfig(entryID, object, schema); err != nil {
		return nil, err
	}
	return object, nil
}

func validateConfig(entryID string, object map[string]any, schema *schemaDoc) error {
	meta, known := schema.Entries[entryID]
	if !known {
		return nil
	}
	for _, field := range meta.Fields {
		value, present := object[field.Name]
		if !present {
			continue
		}
		if field.Secret || isSecretMarker(value) {
			continue
		}
		if value == nil {
			if !field.Nullable {
				return fmt.Errorf("Field '%s' cannot be null", field.Name)
			}
			continue
		}
		if isJSMarker(value) {
			continue
		}
		switch field.Kind {
		case "number":
			switch value.(type) {
			case float64, int, int64, uint64:
			default:
				return fmt.Errorf("Field '%s' must be a number", field.Name)
			}
		case "boolean":
			if _, ok := value.(bool); !ok {
				return fmt.Errorf("Field '%s' must be a boolean", field.Name)
			}
		case "string":
			if _, ok := value.(string); !ok {
				return fmt.Errorf("Field '%s' must be a string", field.Name)
			}
		case "enum":
			text, ok := value.(string)
			if !ok {
				return fmt.Errorf("Field '%s' must be a string", field.Name)
			}
			if len(field.EnumValues) > 0 {
				allowed := false
				for _, candidate := range field.EnumValues {
					if candidate == text {
						allowed = true
						break
					}
				}
				if !allowed {
					return fmt.Errorf("Field '%s' must be one of: %s", field.Name, strings.Join(field.EnumValues, ", "))
				}
			}
		}
	}
	return nil
}

// ---------------------------------------------------------------------------
// Patch row rendering
// ---------------------------------------------------------------------------

func newPatchRow(id, name string, config *yaml.Node, disabled *bool) *yaml.Node {
	row := &yaml.Node{Kind: yaml.MappingNode}
	core.MappingSet(row, "id", core.StringScalar(id))
	if name != "" && name != id {
		core.MappingSet(row, "name", core.StringScalar(name))
	}
	if config != nil {
		core.MappingSet(row, "config", config)
	}
	if disabled != nil {
		value := &yaml.Node{Kind: yaml.ScalarNode, Tag: "!!bool"}
		if *disabled {
			value.Value = "true"
		} else {
			value.Value = "false"
		}
		core.MappingSet(row, "disabled", value)
	}
	return row
}

func mappingKeyCount(row *yaml.Node) int {
	if !core.IsMapping(row) {
		return 0
	}
	return len(row.Content) / 2
}

// hasUnsupportedTags reports whether a rewritten patch row carries a YAML
// tag we cannot round-trip. `!!js` / `!js` survive (yaml.v3 preserves them),
// but any other custom tag would be silently dropped.
func hasUnsupportedTags(block string) bool {
	inSingle, inDouble, inComment, escaped := false, false, false, false
	previousIsTagPosition := true
	runes := []rune(block)
	for index, ch := range runes {
		if inComment {
			if ch == '\n' {
				inComment = false
				previousIsTagPosition = true
			}
			continue
		}
		if inSingle || inDouble {
			switch {
			case escaped:
				escaped = false
			case ch == '\\':
				escaped = true
			case inSingle && ch == '\'':
				inSingle = false
			case inDouble && ch == '"':
				inDouble = false
			}
			continue
		}
		switch ch {
		case '\'':
			inSingle = true
			previousIsTagPosition = false
			continue
		case '"':
			inDouble = true
			previousIsTagPosition = false
			continue
		case '#':
			inComment = true
			continue
		case '!':
			if previousIsTagPosition {
				rest := string(runes[index:])
				allowed := strings.HasPrefix(rest, "!!js") ||
					strings.HasPrefix(rest, "!js") ||
					strings.HasPrefix(rest, "!<tag:yaml.org,2002:js>") ||
					strings.HasPrefix(rest, "!tag:yaml.org,2002:js")
				if !allowed {
					return true
				}
			}
		}
		previousIsTagPosition = ch == ' ' || ch == '\t' || ch == '\n' || ch == ':' || ch == '{' || ch == '[' || ch == ','
	}
	return false
}

func ensurePatchRowRewritable(block string, entryID string) error {
	if hasUnsupportedTags(block) {
		return fmt.Errorf("DSH patch entry '%s' contains YAML tags that cannot be rewritten safely; edit the file manually", entryID)
	}
	return nil
}

// renderDocument concatenates the header with the per-entry blocks; an empty
// document keeps only its leading comments plus `[]`.
func renderDocument(header string, rowCount int, blocks []string) string {
	if rowCount == 0 {
		return header + "[]\n"
	}
	return header + strings.Join(blocks, "")
}

func ensureHomePatchDoesNotShadow(entryID string) error {
	homePatch, err := dshHomePatchPath()
	if err != nil {
		return err
	}
	if !fileExists(homePatch) {
		return nil
	}
	rows, _, err := readPatchDocumentNormalized(homePatch)
	if err != nil {
		return err
	}
	for _, row := range rows {
		if patchEntryID(row) != entryID {
			continue
		}
		if core.MappingGet(row, "config") != nil {
			return fmt.Errorf("%s: '%s' is already configured by the home patch %s; edit that file or move the setting to the profile patch first", dshConfigShadowed, entryID, homePatch)
		}
	}
	return nil
}

// ---------------------------------------------------------------------------
// Profile lock (shared with DSH's own config editor)
// ---------------------------------------------------------------------------

type profileLock struct {
	path string
}

// acquireProfileLock takes the same package.json.lock DSH's config editor
// uses. Locks older than 30s are treated as abandoned.
func acquireProfileLock(profileDir string) (*profileLock, error) {
	path := filepath.Join(profileDir, profileLockFilename)
	deadline := time.Now().Add(lockWait)
	for {
		file, err := os.OpenFile(path, os.O_WRONLY|os.O_CREATE|os.O_EXCL, 0o644) // #nosec G304 -- profile lock file
		if err == nil {
			_, _ = file.Write([]byte(fmt.Sprint(os.Getpid())))
			_ = file.Close()
			return &profileLock{path: path}, nil
		}
		if !os.IsExist(err) {
			return nil, fmt.Errorf("Failed to lock DSH profile: %w", err)
		}
		if isStaleLock(path) {
			_ = os.Remove(path)
			continue
		}
		if time.Now().After(deadline) {
			return nil, fmt.Errorf("Another process is writing the DSH profile (lock %s); retry shortly", path)
		}
		time.Sleep(50 * time.Millisecond)
	}
}

func (lock *profileLock) release() {
	_ = os.Remove(lock.path)
}

func isStaleLock(path string) bool {
	info, err := os.Stat(path)
	if err != nil {
		return false
	}
	return time.Since(info.ModTime()) > staleLockAge
}

// ---------------------------------------------------------------------------
// Commands
// ---------------------------------------------------------------------------

// SaveDshConfigEntry saves one entry's complete configuration in the profile
// patch (or removes the override when config is null).
func (a *App) SaveDshConfigEntry(profile *string, entryID string, config any, expectedHash *string) (DshConfigWriteResult, error) {
	return saveOrToggleEntry(profile, entryID, config, nil, expectedHash)
}

// SetDshConfigEntryDisabled enables or disables one entry in the profile
// patch layer.
func (a *App) SetDshConfigEntryDisabled(profile *string, entryID string, disabled bool, expectedHash *string) (DshConfigWriteResult, error) {
	return saveOrToggleEntry(profile, entryID, nil, &disabled, expectedHash)
}

// saveOrToggleEntry implements the shared write flow of the Rust core's
// save_dsh_config_entry / set_dsh_config_entry_disabled.
func saveOrToggleEntry(profile *string, entryID string, config any, disabled *bool, expectedHash *string) (DshConfigWriteResult, error) {
	entryID = strings.TrimSpace(entryID)
	if entryID == "" {
		return DshConfigWriteResult{}, errString("Entry id is required")
	}
	schema, err := dshSchema()
	if err != nil {
		return DshConfigWriteResult{}, err
	}
	resolved, err := resolveDshProfile(profile)
	if err != nil {
		return DshConfigWriteResult{}, err
	}
	if resolved == nil {
		return DshConfigWriteResult{}, errString("No DSH profile found under ~/.dsh/profiles")
	}
	entries, err := composeEntries(*resolved, schema)
	if err != nil {
		return DshConfigWriteResult{}, err
	}
	found := false
	for i := range entries {
		if entries[i].ID == entryID {
			found = true
			break
		}
	}
	if !found {
		return DshConfigWriteResult{}, fmt.Errorf("Unknown DSH configuration entry: %s", entryID)
	}

	var prepared any
	if config != nil && disabled == nil {
		prepared, err = prepareConfigForWrite(entryID, config, schema)
		if err != nil {
			return DshConfigWriteResult{}, err
		}
	}

	profileDir, err := dshProfileDir(*resolved)
	if err != nil {
		return DshConfigWriteResult{}, err
	}
	if err := core.EnsureDir(profileDir); err != nil {
		return DshConfigWriteResult{}, fmt.Errorf("Failed to create DSH profile directory: %w", err)
	}
	lock, err := acquireProfileLock(profileDir)
	if err != nil {
		return DshConfigWriteResult{}, err
	}
	defer lock.release()

	path, err := dshProfilePatchPath(*resolved)
	if err != nil {
		return DshConfigWriteResult{}, err
	}
	originalBytes, readErr := os.ReadFile(path) // #nosec G304 -- tool config path
	original := string(originalBytes)
	if readErr != nil && !os.IsNotExist(readErr) {
		original = ""
	}

	if expectedHash != nil {
		current := "absent"
		if original != "" || fileExists(path) {
			current = hashBytes([]byte(original))
		}
		if *expectedHash != current {
			return DshConfigWriteResult{}, fmt.Errorf("%s: the DSH profile patch changed since it was read (expected %s, found %s)", dshConfigConflict, *expectedHash, current)
		}
	}

	if err := ensureHomePatchDoesNotShadow(entryID); err != nil {
		return DshConfigWriteResult{}, err
	}

	rows, readOriginal, err := readPatchDocumentNormalized(path)
	if err != nil {
		return DshConfigWriteResult{}, err
	}
	source := original
	if readOriginal != "" {
		source = readOriginal
	}
	rawHeader, blocks := core.SplitPatchDocument(source)
	header := rawHeader
	if len(blocks) == 0 {
		header = core.LeadingCommentBlock(source)
	}

	index := -1
	for i, row := range rows {
		if patchEntryID(row) == entryID {
			index = i
			break
		}
	}
	if index >= 0 {
		block := ""
		if index < len(blocks) {
			block = blocks[index]
		}
		if err := ensurePatchRowRewritable(block, entryID); err != nil {
			return DshConfigWriteResult{}, err
		}
	}

	defaultName := entryID
	if meta, ok := schema.Entries[entryID]; ok && meta.Package != "" {
		defaultName = meta.Package
	}

	switch {
	case index >= 0 && prepared != nil:
		row := rows[index]
		if !core.IsMapping(row) {
			return DshConfigWriteResult{}, fmt.Errorf("DSH patch entry '%s' is not a mapping", entryID)
		}
		core.MappingSet(row, "config", jsonToYAMLNode(prepared))
		block, err := core.SerializePatchEntry(row)
		if err != nil {
			return DshConfigWriteResult{}, err
		}
		blocks[index] = block

	case index >= 0 && prepared == nil && disabled == nil:
		row := rows[index]
		if !core.IsMapping(row) {
			return DshConfigWriteResult{}, fmt.Errorf("DSH patch entry '%s' is not a mapping", entryID)
		}
		core.MappingRemove(row, "config")
		if mappingKeyCount(row) <= 2 {
			rows = append(rows[:index], rows[index+1:]...)
			blocks = append(blocks[:index], blocks[index+1:]...)
		} else {
			block, err := core.SerializePatchEntry(row)
			if err != nil {
				return DshConfigWriteResult{}, err
			}
			blocks[index] = block
		}

	case index >= 0 && disabled != nil:
		row := rows[index]
		if !core.IsMapping(row) {
			return DshConfigWriteResult{}, fmt.Errorf("DSH patch entry '%s' is not a mapping", entryID)
		}
		if *disabled {
			value := &yaml.Node{Kind: yaml.ScalarNode, Tag: "!!bool", Value: "true"}
			core.MappingSet(row, "disabled", value)
		} else {
			core.MappingRemove(row, "disabled")
		}
		if *disabled || mappingKeyCount(row) > 2 {
			block, err := core.SerializePatchEntry(row)
			if err != nil {
				return DshConfigWriteResult{}, err
			}
			blocks[index] = block
		} else {
			rows = append(rows[:index], rows[index+1:]...)
			blocks = append(blocks[:index], blocks[index+1:]...)
		}

	case index < 0 && prepared != nil:
		row := newPatchRow(entryID, defaultName, jsonToYAMLNode(prepared), nil)
		block, err := core.SerializePatchEntry(row)
		if err != nil {
			return DshConfigWriteResult{}, err
		}
		rows = append(rows, row)
		blocks = append(blocks, block)

	case index < 0 && disabled != nil && *disabled:
		value := true
		row := newPatchRow(entryID, defaultName, nil, &value)
		block, err := core.SerializePatchEntry(row)
		if err != nil {
			return DshConfigWriteResult{}, err
		}
		rows = append(rows, row)
		blocks = append(blocks, block)

	case index < 0 && (prepared == nil || (disabled != nil && !*disabled)):
		// Nothing to write: removing a missing override or disabling nothing.
		return DshConfigWriteResult{PatchHash: hashBytes([]byte(source)), Changed: false}, nil
	}

	document := renderDocument(header, len(rows), blocks)
	if err := core.AtomicWritePrivate(path, []byte(document)); err != nil {
		return DshConfigWriteResult{}, err
	}
	return DshConfigWriteResult{PatchHash: hashBytes([]byte(document)), Changed: true}, nil
}
