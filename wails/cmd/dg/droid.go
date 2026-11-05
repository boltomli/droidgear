package main

import (
	"encoding/json"
	"fmt"
	"io"

	"droidgear-wails/internal/service/droid"
	"droidgear-wails/internal/service/legacy"
)

// droid mirrors the Droid section of the GUI: settings-file profiles and
// values, trusted folders, Factory auth profiles, mission models and the
// legacy-config cleanup. (Droid → Models is covered by `dg factory models`.)
// Writes honour --dry-run through the shared core.BeginDryRun recorder.
func (c *cli) droid(args []string) error {
	if len(args) == 0 {
		return c.usage("droid needs a subcommand: settings | trusted | auth | missions | legacy")
	}
	switch args[0] {
	case "settings":
		return c.droidSettings(args[1:])
	case "trusted":
		return c.droidTrusted(args[1:])
	case "auth":
		return c.droidAuth(args[1:])
	case "missions":
		return c.droidMissions(args[1:])
	case "legacy":
		return c.droidLegacy(args[1:])
	default:
		return c.usage("unknown droid subcommand %q (want settings | trusted | auth | missions | legacy)", args[0])
	}
}

// ---------------------------------------------------------------------------
// settings
// ---------------------------------------------------------------------------

func (c *cli) droidSettings(args []string) error {
	if len(args) == 0 {
		return c.usage("droid settings needs a subcommand: list | use | delete | values")
	}
	switch args[0] {
	case "list":
		return c.droidSettingsList(args[1:])
	case "use":
		return c.droidSettingsUse(args[1:])
	case "delete":
		return c.droidSettingsDelete(args[1:])
	case "values":
		return c.droidSettingsValues(args[1:])
	default:
		return c.usage("unknown droid settings subcommand %q (want list | use | delete | values)", args[0])
	}
}

func requireDroidSettingsFile(name string) error {
	files, err := droid.ListSettingsFiles()
	if err != nil {
		return err
	}
	for _, file := range files {
		if file.Name == name {
			return nil
		}
	}
	return fmt.Errorf("%w: droid settings file %q", errNotFound, name)
}

func (c *cli) droidSettingsList(args []string) error {
	if len(args) != 0 {
		return c.usage("droid settings list takes no arguments")
	}
	files, err := droid.ListSettingsFiles()
	if err != nil {
		return err
	}
	return emit(c.opts, c.stdout, files, func(w io.Writer) error {
		if len(files) == 0 {
			_, err := fmt.Fprintln(w, "no droid settings files")
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
			kind := "managed"
			switch {
			case file.IsGlobal:
				kind = "global"
			case file.IsExternal:
				kind = "external"
			}
			fmt.Fprintf(w, "%-24s %-16s %-9s %s\n", file.Name, state, kind, file.Path)
		}
		return nil
	})
}

func (c *cli) droidSettingsUse(args []string) error {
	if len(args) != 1 {
		return c.usage("droid settings use needs <name>")
	}
	if err := requireDroidSettingsFile(args[0]); err != nil {
		return err
	}
	name := args[0]
	info, err := droid.SetActiveSettingsFile(&name)
	if err != nil {
		return err
	}
	return emit(c.opts, c.stdout, info, func(w io.Writer) error {
		_, err := fmt.Fprintf(w, "active settings file: %s\n", info.Name)
		return err
	})
}

func (c *cli) droidSettingsDelete(args []string) error {
	if len(args) != 1 {
		return c.usage("droid settings delete needs <name>")
	}
	name := args[0]
	if name == "global" || name == "Global" {
		return c.usage("the global settings file cannot be deleted")
	}
	if err := requireDroidSettingsFile(name); err != nil {
		return err
	}
	if err := droid.DeleteSettingsFile(name); err != nil {
		return err
	}
	return emit(c.opts, c.stdout, map[string]any{"deleted": name}, func(w io.Writer) error {
		_, err := fmt.Fprintf(w, "deleted droid settings file %q\n", name)
		return err
	})
}

// droidSettingsValues dumps the active-file settings the GUI's Droid →
// Settings pane reads (read-only: the typed setters stay GUI/TUI-side until
// the input-class write actions land).
func (c *cli) droidSettingsValues(args []string) error {
	if len(args) != 0 {
		return c.usage("droid settings values takes no arguments")
	}
	diffMode, err := droid.GetDiffMode()
	if err != nil {
		return err
	}
	todoMode, err := droid.GetTodoDisplayMode()
	if err != nil {
		return err
	}
	cloudSync, err := droid.GetCloudSessionSync()
	if err != nil {
		return err
	}
	coAuthored, err := droid.GetIncludeCoAuthoredByDroid()
	if err != nil {
		return err
	}
	showThinking, err := droid.GetShowThinkingInMainView()
	if err != nil {
		return err
	}
	reasoning, err := droid.GetReasoningEffort()
	if err != nil {
		return err
	}
	compactionMode, err := droid.GetCompactionModelMode()
	if err != nil {
		return err
	}
	compactionLimit, err := droid.GetCompactionTokenLimit()
	if err != nil {
		return err
	}
	compactionPerModel, err := droid.GetCompactionTokenLimitPerModel()
	if err != nil {
		return err
	}
	values := map[string]any{
		"diffMode":                     diffMode,
		"todoDisplayMode":              todoMode,
		"cloudSessionSync":             cloudSync,
		"includeCoAuthoredByDroid":     coAuthored,
		"showThinkingInMainView":       showThinking,
		"reasoningEffort":              reasoning,
		"compactionModelMode":          compactionMode,
		"compactionTokenLimit":         compactionLimit,
		"compactionTokenLimitPerModel": compactionPerModel,
	}
	return emit(c.opts, c.stdout, values, func(w io.Writer) error {
		document, err := json.MarshalIndent(values, "", "  ")
		if err != nil {
			return err
		}
		_, err = fmt.Fprintln(w, string(document))
		return err
	})
}

// ---------------------------------------------------------------------------
// trusted folders
// ---------------------------------------------------------------------------

func (c *cli) droidTrusted(args []string) error {
	if len(args) == 0 {
		return c.usage("droid trusted needs a subcommand: list | add | remove")
	}
	switch args[0] {
	case "list":
		return c.droidTrustedList(args[1:])
	case "add":
		return c.droidTrustedAdd(args[1:])
	case "remove":
		return c.droidTrustedRemove(args[1:])
	default:
		return c.usage("unknown droid trusted subcommand %q (want list | add | remove)", args[0])
	}
}

func (c *cli) droidTrustedList(args []string) error {
	if len(args) != 0 {
		return c.usage("droid trusted list takes no arguments")
	}
	folders, err := droid.ListTrustedFolders()
	if err != nil {
		return err
	}
	return emit(c.opts, c.stdout, folders, func(w io.Writer) error {
		if len(folders) == 0 {
			_, err := fmt.Fprintln(w, "no trusted folders")
			return err
		}
		for _, folder := range folders {
			fmt.Fprintf(w, "%s\t%s\n", folder.Path, folder.TrustedAt)
		}
		return nil
	})
}

func (c *cli) droidTrustedAdd(args []string) error {
	if len(args) != 1 {
		return c.usage("droid trusted add <path>")
	}
	folder, err := droid.AddTrustedFolder(args[0])
	if err != nil {
		return err
	}
	return emit(c.opts, c.stdout, folder, func(w io.Writer) error {
		_, err := fmt.Fprintf(w, "trusted: %s\n", folder.Path)
		return err
	})
}

func (c *cli) droidTrustedRemove(args []string) error {
	if len(args) != 1 {
		return c.usage("droid trusted remove <path>")
	}
	if err := droid.RemoveTrustedFolder(args[0]); err != nil {
		return err
	}
	return emit(c.opts, c.stdout, map[string]any{"removed": args[0]}, func(w io.Writer) error {
		_, err := fmt.Fprintf(w, "removed trusted folder %s\n", args[0])
		return err
	})
}

// ---------------------------------------------------------------------------
// Factory auth profiles
// ---------------------------------------------------------------------------

func (c *cli) droidAuth(args []string) error {
	if len(args) == 0 {
		return c.usage("droid auth needs a subcommand: list | save-current | switch | delete | rename")
	}
	switch args[0] {
	case "list":
		return c.droidAuthList(args[1:])
	case "save-current":
		return c.droidAuthSaveCurrent(args[1:])
	case "switch":
		return c.droidAuthSwitch(args[1:])
	case "delete":
		return c.droidAuthDelete(args[1:])
	case "rename":
		return c.droidAuthRename(args[1:])
	default:
		return c.usage("unknown droid auth subcommand %q (want list | save-current | switch | delete | rename)", args[0])
	}
}

func requireDroidAuthProfile(state droid.AuthProfileState, name string) error {
	for _, profile := range state.Profiles {
		if profile.Name == name {
			return nil
		}
	}
	return fmt.Errorf("%w: droid auth profile %q", errNotFound, name)
}

func (c *cli) droidAuthList(args []string) error {
	if len(args) != 0 {
		return c.usage("droid auth list takes no arguments")
	}
	state, err := droid.ListAuthProfiles()
	if err != nil {
		return err
	}
	return emit(c.opts, c.stdout, state, func(w io.Writer) error {
		if len(state.Profiles) == 0 {
			_, err := fmt.Fprintln(w, "no droid auth profiles")
			return err
		}
		for _, profile := range state.Profiles {
			mark := " "
			if state.Active != nil && profile.Name == *state.Active {
				mark = "*"
			}
			fmt.Fprintf(w, "%s %-20s %-24s %s\n", mark, profile.Name, profile.Label, profile.CreatedAt)
		}
		return nil
	})
}

func (c *cli) droidAuthSaveCurrent(args []string) error {
	if len(args) < 1 || len(args) > 2 {
		return c.usage("droid auth save-current <name> [label]")
	}
	label := args[0]
	if len(args) == 2 {
		label = args[1]
	}
	if err := droid.SaveCurrentAuthProfile(args[0], label); err != nil {
		return err
	}
	return emit(c.opts, c.stdout, map[string]any{"saved": args[0]}, func(w io.Writer) error {
		_, err := fmt.Fprintf(w, "saved droid auth profile %q\n", args[0])
		return err
	})
}

func (c *cli) droidAuthSwitch(args []string) error {
	if len(args) != 1 {
		return c.usage("droid auth switch <name>")
	}
	state, err := droid.ListAuthProfiles()
	if err != nil {
		return err
	}
	if err := requireDroidAuthProfile(state, args[0]); err != nil {
		return err
	}
	if err := droid.SwitchAuthProfile(args[0]); err != nil {
		return err
	}
	return emit(c.opts, c.stdout, map[string]any{"switched": args[0]}, func(w io.Writer) error {
		_, err := fmt.Fprintf(w, "switched droid auth to %q\n", args[0])
		return err
	})
}

func (c *cli) droidAuthDelete(args []string) error {
	if len(args) != 1 {
		return c.usage("droid auth delete <name>")
	}
	state, err := droid.ListAuthProfiles()
	if err != nil {
		return err
	}
	if err := requireDroidAuthProfile(state, args[0]); err != nil {
		return err
	}
	if err := droid.DeleteAuthProfile(args[0]); err != nil {
		return err
	}
	return emit(c.opts, c.stdout, map[string]any{"deleted": args[0]}, func(w io.Writer) error {
		_, err := fmt.Fprintf(w, "deleted droid auth profile %q\n", args[0])
		return err
	})
}

func (c *cli) droidAuthRename(args []string) error {
	if len(args) != 2 {
		return c.usage("droid auth rename <name> <label>")
	}
	state, err := droid.ListAuthProfiles()
	if err != nil {
		return err
	}
	if err := requireDroidAuthProfile(state, args[0]); err != nil {
		return err
	}
	if err := droid.RenameAuthProfile(args[0], args[1]); err != nil {
		return err
	}
	return emit(c.opts, c.stdout, map[string]any{"renamed": args[0], "label": args[1]}, func(w io.Writer) error {
		_, err := fmt.Fprintf(w, "renamed droid auth profile %q to %q\n", args[0], args[1])
		return err
	})
}

// ---------------------------------------------------------------------------
// missions + legacy versions
// ---------------------------------------------------------------------------

func (c *cli) droidMissions(args []string) error {
	if len(args) == 0 {
		return c.usage("droid missions needs a subcommand: show")
	}
	switch args[0] {
	case "show":
		if len(args) != 1 {
			return c.usage("droid missions show takes no arguments")
		}
		settings, err := droid.GetMissionModelSettings()
		if err != nil {
			return err
		}
		return emit(c.opts, c.stdout, settings, func(w io.Writer) error {
			document, err := json.MarshalIndent(settings, "", "  ")
			if err != nil {
				return err
			}
			_, err = fmt.Fprintln(w, string(document))
			return err
		})
	default:
		return c.usage("unknown droid missions subcommand %q (want show)", args[0])
	}
}

func (c *cli) droidLegacy(args []string) error {
	if len(args) == 0 {
		return c.usage("droid legacy needs a subcommand: check | delete")
	}
	switch args[0] {
	case "check":
		if len(args) != 1 {
			return c.usage("droid legacy check takes no arguments")
		}
		found, err := legacy.Check()
		if err != nil {
			return err
		}
		return emit(c.opts, c.stdout, map[string]any{"legacyExists": found}, func(w io.Writer) error {
			line := "no legacy config"
			if found {
				line = "legacy config.json present"
			}
			_, err := fmt.Fprintln(w, line)
			return err
		})
	case "delete":
		if len(args) != 1 {
			return c.usage("droid legacy delete takes no arguments")
		}
		if err := legacy.Delete(); err != nil {
			return err
		}
		return emit(c.opts, c.stdout, map[string]any{"deleted": true}, func(w io.Writer) error {
			_, err := fmt.Fprintln(w, "legacy config deleted")
			return err
		})
	default:
		return c.usage("unknown droid legacy subcommand %q (want check | delete)", args[0])
	}
}
