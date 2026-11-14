package tui

import (
	"context"
	"strings"

	"droidgear-wails/internal/service/preferences"
)

// settingsRows flattens the preferences document into GUI-aligned sections
// (General + Appearance); unset values render as the GUI's placeholder.
func settingsRows(prefs preferences.AppPreferences) []kvRow {
	rows := []kvRow{
		{section: "General", key: "preferred terminal", value: pointerValue(prefs.PreferredTerminal, "auto detect")},
		{section: "General", key: "disable auto update", value: boolValue(prefs.DisableAutoUpdate, false)},
		{section: "General", key: "update channel", value: prefs.UpdateChannel()},
		{section: "Appearance", key: "theme", value: prefs.Theme},
		{section: "Appearance", key: "language", value: pointerValue(prefs.Language, "system default")},
		{section: "Appearance", key: "terminal font", value: pointerValue(prefs.TerminalFontFamily, "terminal default")},
		{section: "Appearance", key: "terminal shell", value: pointerValue(prefs.TerminalShellCommand, "terminal default")},
	}
	if run := prefs.DroidRun; run != nil {
		rows = append(rows,
			kvRow{section: "Droid run", key: "unset ANTHROPIC_AUTH_TOKEN", value: boolValue(run.UnsetAnthropicAuthToken, false)},
			kvRow{section: "Droid run", key: "disable auto-update env", value: boolValue(run.DisableAutoUpdateEnv, false)},
		)
	}
	return rows
}

// newSettingsScreen renders the app preferences — Preferences → General /
// Appearance (theme, language, terminal, updates), every value editable
// through the preview flow.
func newSettingsScreen(context.Context) (Screen, error) {
	prefs, err := preferences.Load()
	if err != nil {
		return &kvScreen{title: "Preferences", err: err}, nil
	}
	path, _ := preferences.Path()
	rowsFn := func() []kvRow {
		current, err := preferences.Load()
		if err != nil {
			return settingsRows(prefs)
		}
		return settingsRows(current)
	}
	return newKVScreen("Preferences", settingsRows(prefs)).
		withRebuild(rowsFn).
		withActions(
			kvAction{
				key: "t", label: "cycle theme",
				run: func(kvRow) error {
					current, err := preferences.Load()
					if err != nil {
						return err
					}
					switch current.Theme {
					case "system":
						current.Theme = "light"
					case "light":
						current.Theme = "dark"
					default:
						current.Theme = "system"
					}
					return preferences.Save(current)
				},
			},
			kvAction{
				key: "l", label: "cycle language",
				run: func(kvRow) error {
					current, err := preferences.Load()
					if err != nil {
						return err
					}
					switch {
					case current.Language == nil:
						current.Language = strPtrOrNil("en")
					case *current.Language == "en":
						current.Language = strPtrOrNil("zh")
					default:
						current.Language = nil // system default, like the GUI
					}
					return preferences.Save(current)
				},
			},
			kvAction{
				key: "u", label: "toggle auto update",
				run: func(kvRow) error {
					current, err := preferences.Load()
					if err != nil {
						return err
					}
					disabled := current.DisableAutoUpdate != nil && *current.DisableAutoUpdate
					current.DisableAutoUpdate = boolPtr(!disabled)
					return preferences.Save(current)
				},
			},
			kvAction{
				key: "c", label: "cycle update channel",
				run: func(kvRow) error {
					current, err := preferences.Load()
					if err != nil {
						return err
					}
					// nil (unset) is the preview default, so toggling back to
					// preview clears the field instead of storing "preview".
					if current.UpdateChannel() == preferences.UpdateChannelPreview {
						current.UpdatesChannel = strPtrOrNil(preferences.UpdateChannelStable)
					} else {
						current.UpdatesChannel = nil
					}
					return preferences.Save(current)
				},
			},
			promptAction("f", "set terminal font", kvPrompt{
				title:      "terminal font",
				hint:       "font family name - empty resets to the terminal default",
				allowEmpty: true,
				initial: func(kvRow) string {
					value, _ := preferences.Load()
					return pointerValue(value.TerminalFontFamily, "")
				},
				submit: func(_ kvRow, entered string) writeAction {
					return writeAction{title: "set terminal font", run: func() error {
						current, err := preferences.Load()
						if err != nil {
							return err
						}
						current.TerminalFontFamily = strPtrOrNil(entered)
						return preferences.Save(current)
					}}
				},
			}),
			promptAction("h", "set terminal shell", kvPrompt{
				title:      "terminal shell command",
				hint:       "e.g. C:\\Windows\\System32\\bash.exe - empty resets to the terminal default",
				allowEmpty: true,
				initial: func(kvRow) string {
					value, _ := preferences.Load()
					return pointerValue(value.TerminalShellCommand, "")
				},
				submit: func(_ kvRow, entered string) writeAction {
					return writeAction{title: "set terminal shell", run: func() error {
						current, err := preferences.Load()
						if err != nil {
							return err
						}
						current.TerminalShellCommand = strPtrOrNil(entered)
						return preferences.Save(current)
					}}
				},
			}),
			promptAction("e", "set preferred terminal", kvPrompt{
				title:      "preferred terminal",
				hint:       "auto | system-default | terminal | iterm2 | ghostty | gnome-terminal | konsole | cmd | powershell - empty = auto detect",
				allowEmpty: true,
				initial: func(kvRow) string {
					value, _ := preferences.Load()
					return pointerValue(value.PreferredTerminal, "")
				},
				submit: func(_ kvRow, entered string) writeAction {
					return writeAction{title: "set preferred terminal", run: func() error {
						current, err := preferences.Load()
						if err != nil {
							return err
						}
						switch entered {
						case "auto", "system-default":
							current.PreferredTerminal = nil
						default:
							current.PreferredTerminal = strPtrOrNil(entered)
						}
						return preferences.Save(current)
					}}
				},
			}),
		).
		withTrailing("preferences file: " + path), nil
}

func pointerValue(v *string, fallback string) string {
	if v == nil || strings.TrimSpace(*v) == "" {
		return fallback
	}
	return *v
}

func boolValue(v *bool, fallback bool) string {
	value := fallback
	if v != nil {
		value = *v
	}
	if value {
		return "on"
	}
	return "off"
}

// strPtrOrNil maps "" to nil (the GUI stores absence, not empty strings).
func strPtrOrNil(value string) *string {
	if strings.TrimSpace(value) == "" {
		return nil
	}
	return &value
}

func boolPtr(value bool) *bool { return &value }
