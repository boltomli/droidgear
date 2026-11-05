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

// newSettingsScreen renders the app preferences — the terminal counterpart
// of Preferences → General / Appearance (theme, language, terminal).
// Writing is part of the preview phase (tui-parity P2); read-only for now.
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
		withActions(kvAction{
			key: "t", label: "cycle theme",
			run: func() error {
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
		}).
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
