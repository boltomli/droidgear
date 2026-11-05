package main

import (
	"encoding/json"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"strings"

	"droidgear-wails/internal/core"
	"droidgear-wails/internal/service/copilot"
)

// copilot mirrors the Copilot section of the GUI: profiles
// list|show|active|apply|save|delete plus status. Writes honour --dry-run
// through the shared core.BeginDryRun recorder.
func (c *cli) copilot(args []string) error {
	if len(args) == 0 {
		return c.usage("copilot needs a subcommand: profiles | status")
	}
	switch args[0] {
	case "profiles":
		return c.copilotProfiles(args[1:])
	case "status":
		return c.copilotStatus(args[1:])
	default:
		return c.usage("unknown copilot subcommand %q (want profiles | status)", args[0])
	}
}

func (c *cli) copilotProfiles(args []string) error {
	if len(args) == 0 {
		return c.usage("copilot profiles needs a subcommand: list | show | active | apply | save | delete")
	}
	switch args[0] {
	case "list":
		return c.copilotProfilesList(args[1:])
	case "show":
		return c.copilotProfilesShow(args[1:])
	case "active":
		return c.copilotProfilesActive(args[1:])
	case "apply":
		return c.copilotProfilesApply(args[1:])
	case "save":
		return c.copilotProfilesSave(args[1:])
	case "delete":
		return c.copilotProfilesDelete(args[1:])
	default:
		return c.usage("unknown copilot profiles subcommand %q (want list | show | active | apply | save | delete)", args[0])
	}
}

func requireCopilotProfile(id string) error {
	profiles, err := copilot.ListCopilotProfiles()
	if err != nil {
		return err
	}
	for _, profile := range profiles {
		if profile.ID == id {
			return nil
		}
	}
	return fmt.Errorf("%w: copilot profile %q", errNotFound, id)
}

// redactCopilotProfile masks the BYOK api key a profile may carry (§5.3).
func redactCopilotProfile(profile copilot.CopilotProfile) copilot.CopilotProfile {
	if profile.APIKey != nil {
		masked := core.RedactSecret(*profile.APIKey)
		profile.APIKey = &masked
	}
	return profile
}

func (c *cli) copilotProfilesList(args []string) error {
	if len(args) != 0 {
		return c.usage("copilot profiles list takes no arguments")
	}
	profiles, err := copilot.ListCopilotProfiles()
	if err != nil {
		return err
	}
	if !c.opts.reveal {
		for i := range profiles {
			profiles[i] = redactCopilotProfile(profiles[i])
		}
	}
	return emit(c.opts, c.stdout, profiles, func(w io.Writer) error {
		if len(profiles) == 0 {
			_, err := fmt.Fprintln(w, "no copilot profiles")
			return err
		}
		for _, profile := range profiles {
			mode := "byok"
			if profile.UseOfficialAuth {
				mode = "official"
			}
			fmt.Fprintf(w, "%-24s %-20s %s\n", profile.ID, profile.Name, mode)
		}
		return nil
	})
}

func (c *cli) copilotProfilesShow(args []string) error {
	if len(args) != 1 {
		return c.usage("copilot profiles show needs <id>")
	}
	if err := requireCopilotProfile(args[0]); err != nil {
		return err
	}
	profile, err := copilot.GetCopilotProfile(args[0])
	if err != nil {
		return err
	}
	if !c.opts.reveal {
		profile = redactCopilotProfile(profile)
	}
	return emit(c.opts, c.stdout, profile, func(w io.Writer) error {
		document, err := json.MarshalIndent(profile, "", "  ")
		if err != nil {
			return err
		}
		_, err = fmt.Fprintln(w, string(document))
		return err
	})
}

func (c *cli) copilotProfilesActive(args []string) error {
	if len(args) != 0 {
		return c.usage("copilot profiles active takes no arguments")
	}
	active, err := copilot.GetActiveCopilotProfileId()
	if err != nil {
		return err
	}
	return emit(c.opts, c.stdout, active, func(w io.Writer) error {
		if active == nil {
			_, err := fmt.Fprintln(w, "(none)")
			return err
		}
		_, err := fmt.Fprintln(w, *active)
		return err
	})
}

func (c *cli) copilotProfilesApply(args []string) error {
	if len(args) != 1 {
		return c.usage("copilot profiles apply needs <id>")
	}
	id := args[0]
	if err := requireCopilotProfile(id); err != nil {
		return err
	}
	if err := copilot.ApplyCopilotProfile(id); err != nil {
		return err
	}
	return emit(c.opts, c.stdout, map[string]any{"applied": true, "id": id}, func(w io.Writer) error {
		_, err := fmt.Fprintf(w, "applied copilot profile %s\n", id)
		return err
	})
}

func (c *cli) copilotProfilesSave(args []string) error {
	fs := c.newFlagSet("copilot profiles save")
	file := fs.String("file", "", "path to a profile document (JSON, same shape as the GUI)")
	if err := fs.Parse(args); err != nil {
		return c.usage("%v", err)
	}
	if fs.NArg() != 0 {
		return c.usage("copilot profiles save takes no positional arguments")
	}
	if strings.TrimSpace(*file) == "" {
		return c.usage("copilot profiles save needs --file <json>")
	}
	raw, err := os.ReadFile(filepath.Clean(*file)) // #nosec G304 -- user-supplied profile file
	if err != nil {
		return fmt.Errorf("Failed to read profile file: %w", err)
	}
	var profile copilot.CopilotProfile
	if err := json.Unmarshal(raw, &profile); err != nil {
		return c.usage("invalid profile JSON: %v", err)
	}
	if strings.TrimSpace(profile.Name) == "" {
		return c.usage("profile needs a non-empty \"name\"")
	}
	if err := copilot.SaveCopilotProfile(profile); err != nil {
		return err
	}
	return emit(c.opts, c.stdout, map[string]any{"saved": profile.ID, "name": profile.Name}, func(w io.Writer) error {
		_, err := fmt.Fprintf(w, "saved copilot profile %q (%s)\n", profile.Name, profile.ID)
		return err
	})
}

func (c *cli) copilotProfilesDelete(args []string) error {
	if len(args) != 1 {
		return c.usage("copilot profiles delete needs <id>")
	}
	id := args[0]
	if err := requireCopilotProfile(id); err != nil {
		return err
	}
	if err := copilot.DeleteCopilotProfile(id); err != nil {
		return err
	}
	return emit(c.opts, c.stdout, map[string]any{"deleted": id}, func(w io.Writer) error {
		_, err := fmt.Fprintf(w, "deleted copilot profile %s\n", id)
		return err
	})
}

func (c *cli) copilotStatus(args []string) error {
	if len(args) != 0 {
		return c.usage("copilot status takes no arguments")
	}
	status, err := copilot.GetCopilotConfigStatus()
	if err != nil {
		return err
	}
	active, err := copilot.GetActiveCopilotProfileId()
	if err != nil {
		return err
	}
	payload := map[string]any{
		"configExists": status.ConfigExists,
		"configPath":   status.ConfigPath,
		"activeId":     active,
	}
	return emit(c.opts, c.stdout, payload, func(w io.Writer) error {
		fmt.Fprintf(w, "config:  %s (%s)\n", status.ConfigPath, presentOr(status.ConfigExists))
		if active == nil {
			fmt.Fprintln(w, "active:  (none)")
		} else {
			fmt.Fprintf(w, "active:  %s\n", *active)
		}
		return nil
	})
}
