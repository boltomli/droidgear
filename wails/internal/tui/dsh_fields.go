package tui

import (
	"context"
	"encoding/json"
	"fmt"
	"strconv"
	"strings"

	"droidgear-wails/internal/service/dsh"
	"github.com/charmbracelet/bubbletea"
)

// dshFieldsScreen is the field editor behind 'e' on a Dsh config entry — the
// TUI twin of the GUI's per-tab settings cards. Every change is one
// writeFlow: the action re-reads the entry for the current base config and
// patch hash, so SaveConfigEntry's expectedHash turns an external edit into
// a service conflict between preview and confirm.
type dshFieldsScreen struct {
	ctx     context.Context
	back    Screen
	profile string
	entryID string

	entry     dsh.DshConfigEntryView
	patchHash string
	selected  int
	offset    int
	err       error
	status    string

	// inner is the active prompt/flow; nil while the field list owns the keys.
	inner Screen
}

func newDshFieldsScreen(ctx context.Context, back Screen, profile string, entryID string) (Screen, error) {
	s := &dshFieldsScreen{ctx: ctx, back: back, profile: profile, entryID: entryID}
	if err := s.reload(); err != nil {
		return s, nil
	}
	s.status = "↑↓ field · enter edit · esc back"
	return s, nil
}

// reload re-reads the entry (open + post-write refresh).
func (s *dshFieldsScreen) reload() error {
	snapshot, err := dsh.ReadConfigSnapshot(&s.profile)
	if err != nil {
		s.err = err
		return err
	}
	for _, entry := range snapshot.Entries {
		if entry.ID != s.entryID {
			continue
		}
		s.entry = entry
		s.patchHash = snapshot.PatchHash
		s.err = nil
		if s.selected >= len(s.entry.Fields) {
			s.selected = len(s.entry.Fields) - 1
		}
		if s.selected < 0 {
			s.selected = 0
		}
		return nil
	}
	s.err = fmt.Errorf("Dsh entry %q not found in profile %s", s.entryID, s.profile)
	return s.err
}

// Refresh implements the post-write reload the writeFlow asks for.
func (s *dshFieldsScreen) Refresh() { _ = s.reload() }

// IsKeySink marks the screen as the key owner (see tui.go): it forwards
// everything to an active prompt/flow and otherwise drives the field list.
func (s *dshFieldsScreen) IsKeySink() bool { return true }

func (s *dshFieldsScreen) Title() string { return "Dsh Config" }

func (s *dshFieldsScreen) Status() string {
	if s.inner != nil {
		return s.inner.Status()
	}
	if s.err != nil {
		return fmt.Sprintf("error: %v · esc back", s.err)
	}
	return s.status
}

func (s *dshFieldsScreen) Update(msg tea.Msg) (Screen, tea.Cmd) {
	// An active prompt/flow owns every key; when it hands us back, it is
	// done (the refresh already happened through Reload).
	if s.inner != nil {
		next, cmd := s.inner.Update(msg)
		if next == Screen(s) {
			s.inner = nil
		} else {
			s.inner = next
		}
		return s, cmd
	}

	key, ok := msg.(tea.KeyMsg)
	if !ok {
		return s, nil
	}
	switch key.String() {
	case "esc", "q":
		return s.back, nil
	case "up", "k":
		if s.selected > 0 {
			s.selected--
		}
	case "down", "j":
		if s.selected < len(s.entry.Fields)-1 {
			s.selected++
		}
	case "enter", " ":
		s.editSelected()
	}
	return s, nil
}

// editSelected opens the write flow (booleans/enums) or the prompt (the
// other kinds) for the highlighted field.
func (s *dshFieldsScreen) editSelected() {
	if s.selected < 0 || s.selected >= len(s.entry.Fields) {
		return
	}
	field := s.entry.Fields[s.selected]
	if field.Secret {
		s.status = "secret field: stored in ~/.droidgear credentials / the DSH Models page, not in the patch"
		return
	}

	switch field.Kind {
	case "boolean":
		current, _ := field.Value.(bool)
		next := !current
		s.inner = newWriteFlow(s, s.saveField(field.Name, next, false), s.Refresh)
	case "enum":
		if len(field.EnumValues) == 0 {
			s.status = field.Name + ": enum without options"
			return
		}
		current, _ := field.Value.(string)
		values := append([]string{""}, field.EnumValues...)
		next := nextChoice(values, current)
		if next == "" {
			s.inner = newWriteFlow(s, s.saveField(field.Name, nil, true), s.Refresh)
			return
		}
		s.inner = newWriteFlow(s, s.saveField(field.Name, next, false), s.Refresh)
	default:
		// string / number / json: one-line prompt; an empty value removes
		// the override (the GUI's "restore inherited" semantics).
		hint := "current value · empty removes the override"
		if field.Kind == "number" {
			hint = "numeric value · empty removes the override"
		} else if field.Kind == "json" {
			hint = "JSON document · empty removes the override"
		}
		initial := ""
		if text, ok := field.Value.(string); ok {
			initial = text
		} else if field.Value != nil {
			initial = stringifyValue(field.Value)
		}
		s.inner = newPromptScreen(s, promptSpec{
			title:      field.Name,
			hint:       hint,
			initial:    initial,
			allowEmpty: true,
			submit: func(value string) writeAction {
				if strings.TrimSpace(value) == "" {
					return s.saveField(field.Name, nil, true)
				}
				parsed, err := parseFieldInput(field, value)
				if err != nil {
					return writeAction{
						title: "set " + field.Name,
						run:   func() error { return err },
					}
				}
				return s.saveField(field.Name, parsed, false)
			},
		}, s.Refresh)
	}
}

// parseFieldInput converts the typed text into the JSON value the schema
// validator expects for the field's kind.
func parseFieldInput(field dsh.DshConfigFieldView, value string) (any, error) {
	switch field.Kind {
	case "number":
		parsed, err := strconv.ParseFloat(strings.TrimSpace(value), 64)
		if err != nil {
			return nil, fmt.Errorf("%s must be a number, got %q", field.Name, value)
		}
		return parsed, nil
	case "json":
		var parsed any
		if err := json.Unmarshal([]byte(value), &parsed); err != nil {
			return nil, fmt.Errorf("%s must be valid JSON: %v", field.Name, err)
		}
		return parsed, nil
	default:
		return value, nil
	}
}

func stringifyValue(value any) string {
	data, err := json.Marshal(value)
	if err != nil {
		return fmt.Sprint(value)
	}
	return string(data)
}

// saveField builds the write action: fresh entry + patch hash, base config
// copied (nil → {}), the one field set or removed, then SaveConfigEntry.
func (s *dshFieldsScreen) saveField(name string, value any, remove bool) writeAction {
	return writeAction{
		title: fmt.Sprintf("set %s.%s", s.entryID, name),
		run: func() error {
			snapshot, err := dsh.ReadConfigSnapshot(&s.profile)
			if err != nil {
				return err
			}
			var entry *dsh.DshConfigEntryView
			for i := range snapshot.Entries {
				if snapshot.Entries[i].ID == s.entryID {
					entry = &snapshot.Entries[i]
					break
				}
			}
			if entry == nil {
				return fmt.Errorf("Dsh entry %q not found", s.entryID)
			}
			config, err := dshConfigMap(entry.Config)
			if err != nil {
				return err
			}
			if remove {
				delete(config, name)
			} else {
				config[name] = value
			}
			hash := snapshot.PatchHash
			_, err = dsh.SaveConfigEntry(&s.profile, s.entryID, config, &hash)
			return err
		},
	}
}

// dshConfigMap deep-copies an entry's profile-layer config (nil → empty).
func dshConfigMap(config any) (map[string]any, error) {
	if config == nil {
		return map[string]any{}, nil
	}
	data, err := json.Marshal(config)
	if err != nil {
		return nil, err
	}
	out := map[string]any{}
	if err := json.Unmarshal(data, &out); err != nil {
		return nil, err
	}
	return out, nil
}

func (s *dshFieldsScreen) View(width, height int) string {
	if s.err != nil && len(s.entry.Fields) == 0 {
		return errStyle.Render("Dsh config: " + s.err.Error())
	}
	if s.inner != nil {
		return s.inner.View(width, height)
	}

	var b strings.Builder
	header := fmt.Sprintf("config: %s (%s)", s.entry.ID, s.entry.PackageName)
	b.WriteString(titleStyle.Render(truncate(header, width)))
	b.WriteString("\n\n")

	visible := height - 3
	if visible < 1 {
		visible = 1
	}
	if s.selected < s.offset {
		s.offset = s.selected
	}
	if s.selected >= s.offset+visible {
		s.offset = s.selected - visible + 1
	}
	end := s.offset + visible
	if end > len(s.entry.Fields) {
		end = len(s.entry.Fields)
	}

	for i := s.offset; i < end; i++ {
		field := s.entry.Fields[i]
		mark := "  "
		if i == s.selected {
			mark = "> "
		}
		value := stringifyValue(field.Value)
		if field.Value == nil {
			value = "(inherited)"
		}
		if field.Secret {
			value = "(secret)"
		}
		badges := ""
		if field.Overridden {
			badges += " *"
		}
		if field.Volatile {
			badges += " ~"
		}
		line := fmt.Sprintf("%s%-28s %-8s %s%s", mark, field.Name, field.Kind, value, badges)
		if i == s.selected {
			b.WriteString(selectedStyle.Render(truncate(line, width)))
		} else {
			b.WriteString(truncate(line, width))
		}
		b.WriteString("\n")
	}
	return b.String()
}
