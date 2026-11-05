package main

import (
	"encoding/json"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"strings"

	"flag"
	"sort"

	"droidgear-wails/internal/service/dsh"
)

// dsh mirrors the Dsh section of the GUI: provider CRUD for the selected
// profile (desktop/web) plus config status. Writes honour --dry-run through
// the shared core.BeginDryRun recorder.
func (c *cli) dsh(args []string) error {
	if len(args) == 0 {
		return c.usage("dsh needs a subcommand: providers | status")
	}
	switch args[0] {
	case "providers":
		return c.dshProviders(args[1:])
	case "status":
		return c.dshStatus(args[1:])
	default:
		return c.usage("unknown dsh subcommand %q (want providers | status)", args[0])
	}
}

// dshProfileFlag registers the shared --profile flag (desktop | web); the
// empty default means "let the service resolve the default profile".
func (c *cli) dshProfileFlag(fs *flag.FlagSet) *string {
	return fs.String("profile", "", "dsh profile: desktop | web (default: auto)")
}

// dshProfileArg turns the flag into the service's profile parameter.
func dshProfileArg(name string) *string {
	if strings.TrimSpace(name) == "" {
		return nil
	}
	return &name
}

func (c *cli) dshProviders(args []string) error {
	if len(args) == 0 {
		return c.usage("dsh providers needs a subcommand: list | save | delete")
	}
	switch args[0] {
	case "list":
		return c.dshProvidersList(args[1:])
	case "save":
		return c.dshProvidersSave(args[1:])
	case "delete":
		return c.dshProvidersDelete(args[1:])
	default:
		return c.usage("unknown dsh providers subcommand %q (want list | save | delete)", args[0])
	}
}

func (c *cli) dshProvidersList(args []string) error {
	fs := c.newFlagSet("dsh providers list")
	profile := c.dshProfileFlag(fs)
	if _, err := parseMixed(fs, args); err != nil {
		return c.usage("%v", err)
	}
	if fs.NArg() != 0 {
		return c.usage("dsh providers list takes no positional arguments")
	}
	current, err := dsh.ReadCurrentConfig(dshProfileArg(*profile))
	if err != nil {
		return err
	}
	ids := make([]string, 0, len(current.Providers))
	for id := range current.Providers {
		ids = append(ids, id)
	}
	sort.Strings(ids)
	return emit(c.opts, c.stdout, current, func(w io.Writer) error {
		if len(ids) == 0 {
			_, err := fmt.Fprintf(w, "no providers in profile %s\n", *profile)
			return err
		}
		for _, id := range ids {
			provider := current.Providers[id]
			base := ""
			if provider.BaseURL != nil {
				base = *provider.BaseURL
			}
			name := ""
			if provider.DisplayName != nil {
				name = *provider.DisplayName
			}
			fmt.Fprintf(w, "%-28s %-24s %s\n", id, name, base)
		}
		return nil
	})
}

func (c *cli) dshProvidersSave(args []string) error {
	fs := c.newFlagSet("dsh providers save")
	profile := c.dshProfileFlag(fs)
	file := fs.String("file", "", "path to a provider document (JSON, same shape as the GUI)")
	rest, err := parseMixed(fs, args)
	if err != nil {
		return c.usage("%v", err)
	}
	if len(rest) != 1 {
		return c.usage("dsh providers save <id> --file <json> [--profile desktop|web]")
	}
	id := rest[0]
	if strings.TrimSpace(*file) == "" {
		return c.usage("dsh providers save needs --file <json>")
	}
	raw, err := os.ReadFile(filepath.Clean(*file)) // #nosec G304 -- user-supplied provider file
	if err != nil {
		return fmt.Errorf("Failed to read provider file: %w", err)
	}
	var provider dsh.DshProviderConfig
	if err := json.Unmarshal(raw, &provider); err != nil {
		return c.usage("invalid provider JSON: %v", err)
	}
	if err := dsh.SaveProvider(dshProfileArg(*profile), id, provider); err != nil {
		return err
	}
	return emit(c.opts, c.stdout, map[string]any{"saved": id, "profile": *profile}, func(w io.Writer) error {
		_, err := fmt.Fprintf(w, "saved dsh provider %q in profile %s\n", id, *profile)
		return err
	})
}

func (c *cli) dshProvidersDelete(args []string) error {
	fs := c.newFlagSet("dsh providers delete")
	profile := c.dshProfileFlag(fs)
	rest, err := parseMixed(fs, args)
	if err != nil {
		return c.usage("%v", err)
	}
	if len(rest) != 1 {
		return c.usage("dsh providers delete <id> [--profile desktop|web]")
	}
	id := rest[0]
	if err := dsh.DeleteProvider(dshProfileArg(*profile), id); err != nil {
		return err
	}
	return emit(c.opts, c.stdout, map[string]any{"deleted": id, "profile": *profile}, func(w io.Writer) error {
		_, err := fmt.Fprintf(w, "deleted dsh provider %q from profile %s\n", id, *profile)
		return err
	})
}

func (c *cli) dshStatus(args []string) error {
	fs := c.newFlagSet("dsh status")
	profile := c.dshProfileFlag(fs)
	if _, err := parseMixed(fs, args); err != nil {
		return c.usage("%v", err)
	}
	if fs.NArg() != 0 {
		return c.usage("dsh status takes no positional arguments")
	}
	status, err := dsh.GetConfigStatus(dshProfileArg(*profile))
	if err != nil {
		return err
	}
	return emit(c.opts, c.stdout, status, func(w io.Writer) error {
		name := *profile
		if status.ProfileName != nil {
			name = *status.ProfileName
		}
		fmt.Fprintf(w, "profile:     %s\n", name)
		fmt.Fprintf(w, "config:      %s (%s)\n", status.ConfigPath, presentOr(status.ConfigExists))
		fmt.Fprintf(w, "credentials: %s (%s)\n", status.CredentialsPath, presentOr(status.CredentialsExists))
		if status.LegacyExists {
			fmt.Fprintf(w, "legacy:      %s\n", status.LegacyPath)
		}
		return nil
	})
}
