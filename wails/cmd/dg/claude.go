package main

import (
	"encoding/json"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"strings"

	"droidgear-wails/internal/core"
	"droidgear-wails/internal/service/claude"
)

// claude mirrors the Claude section of the GUI (settings files): list /
// show / active / use / delete / save. Writes honour --dry-run through the
// shared core.BeginDryRun recorder.
func (c *cli) claude(args []string) error {
	if len(args) == 0 {
		return c.usage("claude needs a subcommand: settings")
	}
	switch args[0] {
	case "settings":
		return c.claudeSettings(args[1:])
	default:
		return c.usage("unknown claude subcommand %q (want settings)", args[0])
	}
}

func (c *cli) claudeSettings(args []string) error {
	if len(args) == 0 {
		return c.usage("claude settings needs a subcommand: list | show | active | use | create | delete | save")
	}
	switch args[0] {
	case "list":
		return c.claudeSettingsList(args[1:])
	case "show":
		return c.claudeSettingsShow(args[1:])
	case "active":
		return c.claudeSettingsActive(args[1:])
	case "use":
		return c.claudeSettingsUse(args[1:])
	case "create":
		return c.claudeSettingsCreate(args[1:])
	case "delete":
		return c.claudeSettingsDelete(args[1:])
	case "save":
		return c.claudeSettingsSave(args[1:])
	default:
		return c.usage("unknown claude settings subcommand %q (want list | show | active | use | create | delete | save)", args[0])
	}
}

// requireClaudeSettingsFile resolves <name> against the managed list so a
// missing file fails with exit 1 instead of creating one.
func requireClaudeSettingsFile(name string) error {
	files, err := claude.ListSettingsFiles()
	if err != nil {
		return err
	}
	for _, file := range files {
		if strings.EqualFold(file.Name, name) {
			return nil
		}
	}
	return fmt.Errorf("%w: claude settings file %q", errNotFound, name)
}

func (c *cli) claudeSettingsList(args []string) error {
	if len(args) != 0 {
		return c.usage("claude settings list takes no arguments")
	}
	files, err := claude.ListSettingsFiles()
	if err != nil {
		return err
	}
	return emit(c.opts, c.stdout, files, func(w io.Writer) error {
		if len(files) == 0 {
			_, err := fmt.Fprintln(w, "no claude settings files")
			return err
		}
		for _, file := range files {
			state := "-"
			if file.IsActive {
				state = "active"
			}
			if !file.Exists {
				state += "/missing"
			}
			fmt.Fprintf(w, "%-24s %-14s %s\n", file.Name, state, file.Path)
		}
		return nil
	})
}

func (c *cli) claudeSettingsShow(args []string) error {
	if len(args) != 1 {
		return c.usage("claude settings show needs <name>")
	}
	if err := requireClaudeSettingsFile(args[0]); err != nil {
		return err
	}
	contents, err := claude.ReadSettingsFile(args[0])
	if err != nil {
		return err
	}
	document, err := json.MarshalIndent(contents, "", "  ")
	if err != nil {
		return err
	}
	if !c.opts.reveal {
		document = []byte(core.RedactSensitiveText(string(document)))
	}
	return emit(c.opts, c.stdout, json.RawMessage(document), func(w io.Writer) error {
		_, err := fmt.Fprintln(w, string(document))
		return err
	})
}

func (c *cli) claudeSettingsActive(args []string) error {
	if len(args) != 0 {
		return c.usage("claude settings active takes no arguments")
	}
	active, err := claude.GetActiveSettingsFile()
	if err != nil {
		return err
	}
	return emit(c.opts, c.stdout, active, func(w io.Writer) error {
		_, err := fmt.Fprintf(w, "%s\t%s\n", active.Name, active.Path)
		return err
	})
}

func (c *cli) claudeSettingsUse(args []string) error {
	if len(args) != 1 {
		return c.usage("claude settings use needs <name>")
	}
	if err := requireClaudeSettingsFile(args[0]); err != nil {
		return err
	}
	name := args[0]
	info, err := claude.SetActiveSettingsFile(&name)
	if err != nil {
		return err
	}
	return emit(c.opts, c.stdout, info, func(w io.Writer) error {
		_, err := fmt.Fprintf(w, "active settings file: %s\n", info.Name)
		return err
	})
}

// claudeSettingsCreate adds a managed settings file (optionally seeded from
// the active one) — the name travels as an argument, so no input primitive
// is needed.
func (c *cli) claudeSettingsCreate(args []string) error {
	fs := c.newFlagSet("claude settings create")
	copyActive := fs.Bool("copy-active", false, "seed the new file from the active settings file")
	rest, err := parseMixed(fs, args)
	if err != nil {
		return c.usage("%v", err)
	}
	if len(rest) != 1 {
		return c.usage("claude settings create <name> [--copy-active]")
	}
	name := rest[0]
	files, err := claude.ListSettingsFiles()
	if err != nil {
		return err
	}
	for _, existing := range files {
		if strings.EqualFold(existing.Name, name) {
			return c.usage("claude settings file %q already exists", name)
		}
	}
	info, err := claude.CreateSettingsFile(name, *copyActive)
	if err != nil {
		return err
	}
	return emit(c.opts, c.stdout, info, func(w io.Writer) error {
		_, err := fmt.Fprintf(w, "created claude settings file %q\n", info.Name)
		return err
	})
}

func (c *cli) claudeSettingsDelete(args []string) error {
	if len(args) != 1 {
		return c.usage("claude settings delete needs <name>")
	}
	name := args[0]
	if strings.EqualFold(name, "global") {
		return c.usage("the global settings file cannot be deleted")
	}
	if err := requireClaudeSettingsFile(name); err != nil {
		return err
	}
	if err := claude.DeleteSettingsFile(name); err != nil {
		return err
	}
	return emit(c.opts, c.stdout, map[string]any{"deleted": name}, func(w io.Writer) error {
		_, err := fmt.Fprintf(w, "deleted claude settings file %q\n", name)
		return err
	})
}

func (c *cli) claudeSettingsSave(args []string) error {
	fs := c.newFlagSet("claude settings save")
	file := fs.String("file", "", "path to a settings document (JSON, same shape as the GUI)")
	rest, err := parseMixed(fs, args)
	if err != nil {
		return c.usage("%v", err)
	}
	if len(rest) != 1 {
		return c.usage("claude settings save <name> --file <json>")
	}
	name := rest[0]
	if err := requireClaudeSettingsFile(name); err != nil {
		return err
	}
	if strings.TrimSpace(*file) == "" {
		return c.usage("claude settings save needs --file <json>")
	}
	raw, err := os.ReadFile(filepath.Clean(*file)) // #nosec G304 -- user-supplied settings file
	if err != nil {
		return fmt.Errorf("Failed to read settings file: %w", err)
	}
	var contents any
	if err := json.Unmarshal(raw, &contents); err != nil {
		return c.usage("invalid settings JSON: %v", err)
	}
	if err := claude.SaveSettingsFile(name, contents); err != nil {
		return err
	}
	return emit(c.opts, c.stdout, map[string]any{"saved": name}, func(w io.Writer) error {
		_, err := fmt.Fprintf(w, "saved claude settings file %q\n", name)
		return err
	})
}
