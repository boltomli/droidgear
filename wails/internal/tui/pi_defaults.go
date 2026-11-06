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

// startupChoiceHints prints what the provider/model rows can point at, so the
// free-text prompt still offers the choices the GUI dropdowns list (the TUI
// twin of `dg pi settings candidates`).
func startupChoiceHints() []string {
	options, err := pi.ListStartupOptions()
	if err != nil {
		return []string{"choices: unavailable (" + err.Error() + ")"}
	}
	if len(options.Providers) == 0 {
		return []string{"choices: none (models.json and pi's catalog are both empty)"}
	}
	pairs := make([]string, 0, len(options.Providers))
	for _, providerID := range options.Providers {
		models := options.Models[providerID]
		if len(models) == 0 {
			pairs = append(pairs, providerID+"/-")
			continue
		}
		for _, modelID := range models {
			pairs = append(pairs, providerID+"/"+modelID)
		}
	}
	const limit = 8
	if len(pairs) > limit {
		pairs = append(pairs[:limit], fmt.Sprintf("… %d more", len(pairs)-limit))
	}
	return []string{"choices: " + strings.Join(pairs, " · ")}
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
	trailing := []string{
		"file " + status.SettingsPath,
		"unset keys are written as absent, so pi picks automatically",
		"thinking level: " + strings.Join(pi.ThinkingLevels, " | "),
		"applying a profile re-points a startup model the profile dropped",
	}
	trailing = append(trailing, startupChoiceHints()...)
	screen := newKVScreen("Pi defaults", rows).
		withRebuild(rowsFn).
		withTrailing(trailing...).
		withActions(
			promptAction("e", "edit", kvPrompt{
				title:      "set pi default · provider | model | thinking level",
				hint:       "empty clears the key (pi picks automatically) · thinking level: " + strings.Join(pi.ThinkingLevels, " | ") + " · provider/model choices: `dg pi settings candidates`",
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
