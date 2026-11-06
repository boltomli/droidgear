package main

import (
	"fmt"
	"io"
	"sort"
	"strings"

	"droidgear-wails/internal/core"
	"droidgear-wails/internal/service/importflow"
	"droidgear-wails/internal/service/pi"
)

// `dg pi providers …` / `dg omp providers import`.
//
// Resolution (channel → token → models) and the inference both live in the
// service layer (service/importflow + service/{pi,omp} + service/channelimport),
// so the GUI dialog, this CLI and the TUI share one implementation.

func splitCSV(value string) []string {
	parts := strings.Split(value, ",")
	out := make([]string, 0, len(parts))
	for _, part := range parts {
		if trimmed := strings.TrimSpace(part); trimmed != "" {
			out = append(out, trimmed)
		}
	}
	return out
}

func sortedPiProviderIDs(providers map[string]pi.PiProviderConfig) []string {
	ids := make([]string, 0, len(providers))
	for id := range providers {
		ids = append(ids, id)
	}
	sort.Strings(ids)
	return ids
}

// ---------------------------------------------------------------------------
// dg pi providers
// ---------------------------------------------------------------------------

func (c *cli) piProviders(args []string) error {
	if len(args) == 0 {
		return c.usage("pi providers needs a subcommand: list | show | import | delete")
	}
	switch args[0] {
	case "list":
		return c.piProvidersList(args[1:])
	case "show":
		return c.piProvidersShow(args[1:])
	case "import":
		return c.piProvidersImport(args[1:])
	case "delete":
		return c.piProvidersDelete(args[1:])
	default:
		return c.usage("unknown pi providers subcommand %q (want list | show | import | delete)", args[0])
	}
}

func (c *cli) piProvidersList(args []string) error {
	if len(args) != 0 {
		return c.usage("pi providers list takes no arguments")
	}
	config, err := pi.ReadCurrentConfig()
	if err != nil {
		return err
	}
	providers := config.Providers
	if !c.opts.reveal {
		for id, provider := range providers {
			if provider.APIKey != nil {
				masked := core.RedactSecret(*provider.APIKey)
				provider.APIKey = &masked
			}
			providers[id] = provider
		}
	}
	return emit(c.opts, c.stdout, config, func(w io.Writer) error {
		if len(providers) == 0 {
			_, err := fmt.Fprintln(w, "no pi providers")
			return err
		}
		for _, id := range sortedPiProviderIDs(providers) {
			provider := providers[id]
			api := ""
			if provider.API != nil {
				api = *provider.API
			}
			fmt.Fprintf(w, "%-24s %-24s %d model(s)\n", id, api, len(provider.Models))
		}
		return nil
	})
}

func (c *cli) piProvidersShow(args []string) error {
	if len(args) != 1 {
		return c.usage("pi providers show needs <provider-id>")
	}
	config, err := pi.ReadCurrentConfig()
	if err != nil {
		return err
	}
	provider, ok := config.Providers[args[0]]
	if !ok {
		return fmt.Errorf("%w: pi provider %q", errNotFound, args[0])
	}
	if !c.opts.reveal && provider.APIKey != nil {
		masked := core.RedactSecret(*provider.APIKey)
		provider.APIKey = &masked
	}
	return emit(c.opts, c.stdout, provider, func(w io.Writer) error {
		_, err := fmt.Fprintf(w, "%s\n", args[0])
		return err
	})
}

func (c *cli) piProvidersImport(args []string) error {
	fs := c.newFlagSet("pi providers import")
	channelID := fs.String("channel", "", "channel id to import (see `dg channels list`)")
	tokenRef := fs.String("token", "", "token name or id (default: the channel's first token)")
	models := fs.String("models", "", "comma-separated model ids (default: every model the endpoint lists)")
	providerID := fs.String("provider", "", "provider id (default: derived from the channel/token name)")
	if err := fs.Parse(args); err != nil {
		return c.usage("%v", err)
	}
	if fs.NArg() != 0 {
		return c.usage("pi providers import takes no positional arguments")
	}
	if strings.TrimSpace(*channelID) == "" {
		return c.usage("pi providers import needs --channel <id>")
	}
	ctx, cancel := c.opts.commandContext()
	defer cancel()

	result, err := importflow.ImportPi(ctx, *channelID, *tokenRef, splitCSV(*models), *providerID)
	if err != nil {
		return err
	}
	return emit(c.opts, c.stdout, result, func(w io.Writer) error {
		_, err := fmt.Fprintf(w, "imported pi provider %s (%s, %d model(s)) from channel %s\n",
			result.ProviderID, result.API, len(result.Models), *channelID)
		return err
	})
}

func (c *cli) piProvidersDelete(args []string) error {
	if len(args) != 1 {
		return c.usage("pi providers delete needs <provider-id>")
	}
	if err := pi.DeleteLiveProvider(args[0]); err != nil {
		return err
	}
	return emit(c.opts, c.stdout, map[string]any{"deleted": true, "id": args[0]}, func(w io.Writer) error {
		_, err := fmt.Fprintf(w, "deleted pi provider %s\n", args[0])
		return err
	})
}

// ---------------------------------------------------------------------------
// dg omp providers import
// ---------------------------------------------------------------------------

func (c *cli) ompProvidersImport(args []string) error {
	fs := c.newFlagSet("omp providers import")
	channelID := fs.String("channel", "", "channel id to import (see `dg channels list`)")
	tokenRef := fs.String("token", "", "token name or id (default: the channel's first token)")
	models := fs.String("models", "", "comma-separated model ids (default: every model the endpoint lists)")
	providerID := fs.String("provider", "", "provider id (default: derived from the channel/token name)")
	if err := fs.Parse(args); err != nil {
		return c.usage("%v", err)
	}
	if fs.NArg() != 0 {
		return c.usage("omp providers import takes no positional arguments")
	}
	if strings.TrimSpace(*channelID) == "" {
		return c.usage("omp providers import needs --channel <id>")
	}
	ctx, cancel := c.opts.commandContext()
	defer cancel()

	result, err := importflow.ImportOmp(ctx, *channelID, *tokenRef, splitCSV(*models), *providerID)
	if err != nil {
		return err
	}
	if !c.opts.reveal && result.Provider.APIKey != nil {
		masked := core.RedactSecret(*result.Provider.APIKey)
		result.Provider.APIKey = &masked
	}
	return emit(c.opts, c.stdout, result, func(w io.Writer) error {
		api := ""
		if result.Provider.API != nil {
			api = *result.Provider.API
		}
		_, err := fmt.Fprintf(w, "imported omp provider %s (%s, %d model(s)) from channel %s\n",
			result.ProviderID, api, len(result.Provider.Models), *channelID)
		return err
	})
}
