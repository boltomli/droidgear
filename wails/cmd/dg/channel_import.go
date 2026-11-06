package main

import (
	"fmt"
	"io"
	"sort"
	"strings"

	"droidgear-wails/internal/core"
	"droidgear-wails/internal/service/hermes"
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
	result, err := pi.DeleteLiveProvider(args[0])
	if err != nil {
		return err
	}
	fields, notes := piStartupReceipt(result)
	receipt := map[string]any{
		"deleted":          !c.opts.dryRun,
		"id":               args[0],
		"appliedProviders": result.AppliedProviders,
	}
	for key, value := range fields {
		receipt[key] = value
	}
	if len(notes) > 0 {
		receipt["startupReasons"] = result.StartupReasons
	}
	return emit(c.opts, c.stdout, receipt, func(w io.Writer) error {
		if _, err := fmt.Fprintf(w, "deleted pi provider %s\n", args[0]); err != nil {
			return err
		}
		for _, note := range notes {
			if _, err := fmt.Fprintf(w, "adjusted: %s\n", note); err != nil {
				return err
			}
		}
		return nil
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

// ---------------------------------------------------------------------------
// dg hermes providers
// ---------------------------------------------------------------------------

func (c *cli) hermesProviders(args []string) error {
	if len(args) == 0 {
		return c.usage("hermes providers needs a subcommand: list | show | import | delete")
	}
	switch args[0] {
	case "list":
		return c.hermesProvidersList(args[1:])
	case "show":
		return c.hermesProvidersShow(args[1:])
	case "import":
		return c.hermesProvidersImport(args[1:])
	case "delete":
		return c.hermesProvidersDelete(args[1:])
	default:
		return c.usage("unknown hermes providers subcommand %q (want list | show | import | delete)", args[0])
	}
}

func (c *cli) hermesLiveProviders() ([]hermes.HermesModelConfig, error) {
	current, err := hermes.ReadCurrentConfig()
	if err != nil {
		return nil, err
	}
	providers := make([]hermes.HermesModelConfig, 0, len(current.Models))
	for _, model := range current.Models {
		if model.Name != nil && strings.TrimSpace(*model.Name) != "" {
			providers = append(providers, model)
		}
	}
	sort.Slice(providers, func(i, j int) bool {
		return strings.ToLower(*providers[i].Name) < strings.ToLower(*providers[j].Name)
	})
	return providers, nil
}

func (c *cli) hermesProvidersList(args []string) error {
	if len(args) != 0 {
		return c.usage("hermes providers list takes no arguments")
	}
	providers, err := c.hermesLiveProviders()
	if err != nil {
		return err
	}
	if !c.opts.reveal {
		for i := range providers {
			if providers[i].APIKey != nil {
				masked := core.RedactSecret(*providers[i].APIKey)
				providers[i].APIKey = &masked
			}
		}
	}
	active := ""
	current, err := hermes.ReadCurrentConfig()
	if err == nil {
		for _, model := range current.Models {
			if model.IsDefault && model.Name != nil {
				active = *model.Name
			}
		}
	}
	return emit(c.opts, c.stdout, providers, func(w io.Writer) error {
		if len(providers) == 0 {
			_, err := fmt.Fprintln(w, "no hermes providers")
			return err
		}
		for _, provider := range providers {
			mark := "  "
			if provider.Name != nil && *provider.Name == active {
				mark = "* "
			}
			baseURL := ""
			if provider.BaseURL != nil {
				baseURL = *provider.BaseURL
			}
			fmt.Fprintf(w, "%s%-24s %s\n", mark, *provider.Name, baseURL)
		}
		return nil
	})
}

func (c *cli) hermesProvidersShow(args []string) error {
	if len(args) != 1 {
		return c.usage("hermes providers show needs <name>")
	}
	providers, err := c.hermesLiveProviders()
	if err != nil {
		return err
	}
	for _, provider := range providers {
		if *provider.Name != args[0] {
			continue
		}
		if !c.opts.reveal && provider.APIKey != nil {
			masked := core.RedactSecret(*provider.APIKey)
			provider.APIKey = &masked
		}
		return emit(c.opts, c.stdout, provider, func(w io.Writer) error {
			_, err := fmt.Fprintln(w, args[0])
			return err
		})
	}
	return fmt.Errorf("%w: hermes provider %q", errNotFound, args[0])
}

func (c *cli) hermesProvidersImport(args []string) error {
	fs := c.newFlagSet("hermes providers import")
	channelID := fs.String("channel", "", "channel id to import (see `dg channels list`)")
	tokenRef := fs.String("token", "", "token name or id (default: the channel's first token)")
	modelID := fs.String("model", "", "default model id (default: the first model the endpoint lists)")
	name := fs.String("name", "", "provider name (default: the channel name)")
	if err := fs.Parse(args); err != nil {
		return c.usage("%v", err)
	}
	if fs.NArg() != 0 {
		return c.usage("hermes providers import takes no positional arguments")
	}
	if strings.TrimSpace(*channelID) == "" {
		return c.usage("hermes providers import needs --channel <id>")
	}
	ctx, cancel := c.opts.commandContext()
	defer cancel()

	result, err := importflow.ImportHermes(ctx, *channelID, *tokenRef, *modelID)
	if err != nil {
		return err
	}
	if strings.TrimSpace(*name) != "" {
		// The service derives the provider name from the channel; a custom
		// name is a second import under that name.
		result, err = hermes.ImportChannelProvider(hermes.HermesChannelImportInput{
			ChannelName:  *name,
			ChannelType:  "",
			BaseURL:      result.BaseURL,
			APIKey:       result.APIKey,
			DefaultModel: derefString(result.DefaultModel),
		})
		if err != nil {
			return err
		}
	}
	if !c.opts.reveal {
		masked := core.RedactSecret(result.APIKey)
		result.APIKey = masked
	}
	return emit(c.opts, c.stdout, result, func(w io.Writer) error {
		_, err := fmt.Fprintf(w, "imported hermes provider %s (%s) from channel %s\n",
			result.Name, result.BaseURL, *channelID)
		return err
	})
}

func derefString(value *string) string {
	if value == nil {
		return ""
	}
	return *value
}

func (c *cli) hermesProvidersDelete(args []string) error {
	if len(args) != 1 {
		return c.usage("hermes providers delete needs <name>")
	}
	if err := hermes.DeleteLiveProvider(args[0]); err != nil {
		return err
	}
	return emit(c.opts, c.stdout, map[string]any{"deleted": true, "name": args[0]}, func(w io.Writer) error {
		_, err := fmt.Fprintf(w, "deleted hermes provider %s\n", args[0])
		return err
	})
}
