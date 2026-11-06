package tui

import (
	"context"
	"fmt"
	"strings"

	"droidgear-wails/internal/service/pi"
)

// autoLabel is how an unset key renders: pi picks the model itself.
const autoLabel = "(auto)"

// piDefaultRows flattens the startup model selection into GUI-aligned rows
// (the GUI's Pi → Default model card, i.e. ~/.pi/agent/settings.json).
func piDefaultRows(settings pi.PiSettings) []kvRow {
	value := func(v *string) string {
		if v == nil || strings.TrimSpace(*v) == "" {
			return autoLabel
		}
		return *v
	}
	return []kvRow{
		{section: "Startup model", key: "provider", value: value(settings.DefaultProvider)},
		{section: "Startup model", key: "model", value: value(settings.DefaultModel)},
		{section: "Startup model", key: "thinking level", value: value(settings.DefaultThinkingLevel)},
	}
}

// piDefaultAction applies one edited row to the live settings.json, keeping
// every other key (settings.json carries retry/theme/compaction …).
func piDefaultAction(row kvRow, raw string) writeAction {
	return writeAction{
		title: "set pi " + row.key,
		run: func() error {
			current, err := pi.ReadSettings()
			if err != nil {
				return err
			}
			next := current.PiSettings
			trimmed := strings.TrimSpace(raw)
			var value *string
			if trimmed != "" {
				value = &trimmed
			}
			switch row.key {
			case "provider":
				next.DefaultProvider = value
			case "model":
				next.DefaultModel = value
			case "thinking level":
				next.DefaultThinkingLevel = value
			default:
				return fmt.Errorf("unknown pi default row %q", row.key)
			}
			return pi.SaveSettings(next)
		},
	}
}

// newPiDefaultsScreen renders the startup model selection — the terminal
// counterpart of the GUI's default-model card. `e` edits a row, `x` clears
// the whole selection; both go through the shared preview → confirm contract,
// and `dg pi settings set|reset` writes the same keys through the same
// service functions.
func newPiDefaultsScreen(context.Context) (Screen, error) {
	status, err := pi.ReadSettings()
	if err != nil {
		return &kvScreen{title: "Pi defaults", err: err}, nil
	}
	rows := piDefaultRows(status.PiSettings)
	rowsFn := func() []kvRow {
		current, err := pi.ReadSettings()
		if err != nil {
			return rows
		}
		return piDefaultRows(current.PiSettings)
	}
	screen := newKVScreen("Pi defaults", rows).
		withRebuild(rowsFn).
		withTrailing(
			"file "+status.SettingsPath,
			"unset keys are written as absent, so pi picks automatically",
			"thinking level: "+strings.Join(pi.ThinkingLevels, " | "),
		).
		withActions(
			promptAction("e", "edit", kvPrompt{
				title:      "set pi default · provider | model | thinking level",
				hint:       "empty clears the key (pi picks automatically) · thinking level: " + strings.Join(pi.ThinkingLevels, " | "),
				allowEmpty: true,
				initial: func(row kvRow) string {
					if row.value == autoLabel {
						return ""
					}
					return row.value
				},
				submit: piDefaultAction,
			}),
			kvAction{
				key: "x", label: "reset selection",
				run: func(kvRow) error { return pi.ClearSettings() },
			},
		)
	return screen, nil
}
