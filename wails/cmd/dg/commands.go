package main

import (
	"context"
	"encoding/json"
	"flag"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"strings"

	"droidgear-wails/internal/core"
	"droidgear-wails/internal/service/channels"
	"droidgear-wails/internal/service/export"
	"droidgear-wails/internal/service/paths"
	"droidgear-wails/internal/version"
)

// cli carries the parsed global flags plus the streams of one invocation.
type cli struct {
	opts   options
	stdout io.Writer
	stderr io.Writer
}

// newFlagSet parses a subcommand's own flags. Global flags were already
// removed by splitGlobalFlags, and the package's flag output is discarded so
// every diagnostic goes through the JSON envelope / stderr of run().
func (c *cli) newFlagSet(name string) *flag.FlagSet {
	fs := flag.NewFlagSet(name, flag.ContinueOnError)
	fs.SetOutput(io.Discard)
	return fs
}

func (c *cli) usage(format string, args ...any) error {
	return usageError{fmt.Sprintf(format, args...)}
}

// ---------------------------------------------------------------------------
// dg version
// ---------------------------------------------------------------------------

func (c *cli) version(args []string) error {
	if len(args) != 0 {
		return c.usage("version takes no arguments")
	}
	return emit(c.opts, c.stdout, map[string]string{"version": version.Version}, func(w io.Writer) error {
		_, err := fmt.Fprintln(w, version.Version)
		return err
	})
}

// ---------------------------------------------------------------------------
// dg doctor
// ---------------------------------------------------------------------------

type fileCheck struct {
	Path   string `json:"path"`
	Exists bool   `json:"exists"`
}

type doctorReport struct {
	Version string               `json:"version"`
	Home    string               `json:"home"`
	Paths   paths.EffectivePaths `json:"paths"`
	Wsl     paths.WslInfo        `json:"wsl"`
	Files   map[string]fileCheck `json:"files"`
}

// doctor is strictly read-only: it reports which config files exist instead
// of loading them, because loading channels can trigger the one-time Factory
// migration write (design doc §5.1).
func (c *cli) doctor(args []string) error {
	if len(args) != 0 {
		return c.usage("doctor takes no arguments")
	}
	ctx, cancel := c.opts.commandContext()
	defer cancel()

	home, err := core.HomeDir()
	if err != nil {
		return err
	}
	effective, err := paths.Effective(ctx)
	if err != nil {
		return err
	}
	wsl, err := paths.Wsl(ctx)
	if err != nil {
		return err
	}

	droidGearDir, err := core.DroidGearDir()
	if err != nil {
		return err
	}
	files := map[string]fileCheck{}
	addFile := func(name string, path string, err error) {
		if err != nil {
			files[name] = fileCheck{Path: "", Exists: false}
			return
		}
		files[name] = fileCheck{Path: path, Exists: core.FileExists(path)}
	}
	settingsPath, err := core.DroidGearSettingsPath()
	addFile("droidgearSettings", settingsPath, err)
	addFile("channels", filepath.Join(droidGearDir, "channels.json"), nil)
	addFile("exportTemplates", filepath.Join(droidGearDir, "export-templates.yaml"), nil)

	report := doctorReport{
		Version: version.Version,
		Home:    home,
		Paths:   effective,
		Wsl:     wsl,
		Files:   files,
	}
	return emit(c.opts, c.stdout, report, func(w io.Writer) error {
		fmt.Fprintf(w, "dg %s\n", report.Version)
		fmt.Fprintf(w, "home: %s\n", report.Home)
		for _, path := range report.Paths.List() {
			suffix := "default"
			if !path.IsDefault {
				suffix = "override"
			}
			fmt.Fprintf(w, "  %-12s %s (%s)\n", path.Key, path.Path, suffix)
		}
		for name, check := range report.Files {
			state := "missing"
			if check.Exists {
				state = "present"
			}
			fmt.Fprintf(w, "  file %-18s %-8s %s\n", name, state, check.Path)
		}
		if report.Wsl.Available {
			fmt.Fprintf(w, "wsl: available (%d distro(s))\n", len(report.Wsl.Distros))
		} else {
			fmt.Fprintf(w, "wsl: unavailable\n")
		}
		return nil
	})
}

// ---------------------------------------------------------------------------
// dg path
// ---------------------------------------------------------------------------

func (c *cli) path(args []string) error {
	if len(args) == 0 {
		return c.usage("path needs a subcommand: show | set | reset")
	}
	switch args[0] {
	case "show":
		return c.pathShow(args[1:])
	case "set":
		return c.pathSet(args[1:])
	case "reset":
		return c.pathReset(args[1:])
	default:
		return c.usage("unknown path subcommand %q (want show | set | reset)", args[0])
	}
}

func (c *cli) pathShow(args []string) error {
	if len(args) != 0 {
		return c.usage("path show takes no arguments")
	}
	ctx, cancel := c.opts.commandContext()
	defer cancel()

	effective, err := paths.Effective(ctx)
	if err != nil {
		return err
	}
	return emit(c.opts, c.stdout, effective, func(w io.Writer) error {
		for _, path := range effective.List() {
			suffix := ""
			if !path.IsDefault {
				suffix = "  (override)"
			}
			fmt.Fprintf(w, "%-12s %s%s\n", path.Key, path.Path, suffix)
		}
		return nil
	})
}

func (c *cli) pathSet(args []string) error {
	if len(args) != 2 {
		return c.usage("path set needs <key> <path>")
	}
	key, value := args[0], args[1]
	if err := c.requirePathKey(key); err != nil {
		return err
	}
	return c.applyPathChange(key, value, func(ctx context.Context) error {
		return paths.SaveOverride(ctx, key, value)
	})
}

func (c *cli) pathReset(args []string) error {
	if len(args) != 1 {
		return c.usage("path reset needs <key>")
	}
	key := args[0]
	if err := c.requirePathKey(key); err != nil {
		return err
	}
	return c.applyPathChange(key, "", func(ctx context.Context) error {
		return paths.ResetOverride(ctx, key)
	})
}

func (c *cli) requirePathKey(key string) error {
	for _, known := range paths.Keys() {
		if known == key {
			return nil
		}
	}
	return c.usage("unknown path key %q (want one of: %s)", key, strings.Join(paths.Keys(), ", "))
}

// applyPathChange shares the --dry-run / receipt handling of path set|reset.
func (c *cli) applyPathChange(key string, value string, write func(context.Context) error) error {
	receipt := func(applied bool) map[string]any {
		return map[string]any{"applied": applied, "key": key, "path": value}
	}
	human := func(applied bool) func(io.Writer) error {
		return func(w io.Writer) error {
			verb := "set"
			if !applied {
				verb = "would set"
			}
			if value == "" {
				verb = strings.Replace(verb, "set", "reset", 1)
				_, err := fmt.Fprintf(w, "%s %s\n", verb, key)
				return err
			}
			_, err := fmt.Fprintf(w, "%s %s = %s\n", verb, key, value)
			return err
		}
	}

	if c.opts.dryRun {
		return emit(c.opts, c.stdout, receipt(false), human(false))
	}
	ctx, cancel := c.opts.commandContext()
	defer cancel()
	if err := write(ctx); err != nil {
		return err
	}
	return emit(c.opts, c.stdout, receipt(true), human(true))
}

// ---------------------------------------------------------------------------
// dg channels
// ---------------------------------------------------------------------------

func (c *cli) channels(args []string) error {
	if len(args) == 0 {
		return c.usage("channels needs a subcommand: list | show | tokens")
	}
	switch args[0] {
	case "list":
		return c.channelsList(args[1:])
	case "show":
		return c.channelsShow(args[1:])
	case "tokens":
		return c.channelsTokens(args[1:])
	default:
		return c.usage("unknown channels subcommand %q (want list | show | tokens)", args[0])
	}
}

func (c *cli) loadChannel(id string) (channels.Channel, error) {
	ctx, cancel := c.opts.commandContext()
	defer cancel()

	list, err := channels.Load(ctx)
	if err != nil {
		return channels.Channel{}, err
	}
	for _, channel := range list {
		if channel.ID == id {
			return channel, nil
		}
	}
	return channels.Channel{}, fmt.Errorf("%w: channel %q", errNotFound, id)
}

func (c *cli) channelsList(args []string) error {
	if len(args) != 0 {
		return c.usage("channels list takes no arguments")
	}
	ctx, cancel := c.opts.commandContext()
	defer cancel()

	list, err := channels.Load(ctx)
	if err != nil {
		return err
	}
	return emit(c.opts, c.stdout, list, func(w io.Writer) error {
		if len(list) == 0 {
			_, err := fmt.Fprintln(w, "no channels configured")
			return err
		}
		for _, channel := range list {
			state := "disabled"
			if channel.Enabled {
				state = "enabled"
			}
			fmt.Fprintf(w, "%-20s %-14s %-8s %s\n", channel.ID, channel.Type, state, channel.BaseURL)
		}
		return nil
	})
}

type channelDetail struct {
	Channel  channels.Channel `json:"channel"`
	AuthKind string           `json:"authKind"` // api_key | credentials | none
}

func (c *cli) channelsShow(args []string) error {
	if len(args) != 1 {
		return c.usage("channels show needs <id>")
	}
	channel, err := c.loadChannel(args[0])
	if err != nil {
		return err
	}

	ctx, cancel := c.opts.commandContext()
	defer cancel()

	kind, err := channelAuthKind(ctx, channel.ID)
	if err != nil {
		return err
	}
	detail := channelDetail{Channel: channel, AuthKind: kind}
	return emit(c.opts, c.stdout, detail, func(w io.Writer) error {
		fmt.Fprintf(w, "id:       %s\n", channel.ID)
		fmt.Fprintf(w, "name:     %s\n", channel.Name)
		fmt.Fprintf(w, "type:     %s\n", channel.Type)
		fmt.Fprintf(w, "baseUrl:  %s\n", channel.BaseURL)
		fmt.Fprintf(w, "enabled:  %v\n", channel.Enabled)
		fmt.Fprintf(w, "auth:     %s\n", kind)
		return nil
	})
}

func (c *cli) channelsTokens(args []string) error {
	if len(args) != 1 {
		return c.usage("channels tokens needs <id>")
	}
	channel, err := c.loadChannel(args[0])
	if err != nil {
		return err
	}

	ctx, cancel := c.opts.commandContext()
	defer cancel()

	tokens, err := channels.TokensFor(ctx, channel)
	if err != nil {
		return err
	}
	if !c.opts.reveal {
		// Secrets never reach stdout without --reveal (design doc §5.3);
		// the shape stays identical so scripts keep working.
		masked := make([]channels.ChannelToken, len(tokens))
		for i, token := range tokens {
			token.Key = core.RedactSecret(token.Key)
			masked[i] = token
		}
		tokens = masked
	}
	return emit(c.opts, c.stdout, tokens, func(w io.Writer) error {
		if len(tokens) == 0 {
			_, err := fmt.Fprintln(w, "no tokens")
			return err
		}
		for _, token := range tokens {
			fmt.Fprintf(w, "%-6d %-24s %s\n", token.Status, token.Name, token.Key)
		}
		return nil
	})
}

func channelAuthKind(ctx context.Context, id string) (string, error) {
	apiKey, err := channels.ApiKey(ctx, id)
	if err != nil {
		return "", err
	}
	if apiKey != nil {
		return "api_key", nil
	}
	credentials, err := channels.Credentials(ctx, id)
	if err != nil {
		return "", err
	}
	if credentials != nil {
		return "credentials", nil
	}
	return "none", nil
}

// ---------------------------------------------------------------------------
// dg export
// ---------------------------------------------------------------------------

func (c *cli) export(args []string) error {
	if len(args) == 0 {
		return c.usage("export needs a subcommand: list | save | delete | run")
	}
	switch args[0] {
	case "list":
		return c.exportList(args[1:])
	case "save":
		return c.exportSave(args[1:])
	case "delete":
		return c.exportDelete(args[1:])
	case "run":
		return c.exportRun(args[1:])
	default:
		return c.usage("unknown export subcommand %q (want list | save | delete | run)", args[0])
	}
}

func (c *cli) exportList(args []string) error {
	if len(args) != 0 {
		return c.usage("export list takes no arguments")
	}
	ctx, cancel := c.opts.commandContext()
	defer cancel()

	templates, err := export.LoadTemplates(ctx)
	if err != nil {
		return err
	}
	return emit(c.opts, c.stdout, templates, func(w io.Writer) error {
		if len(templates) == 0 {
			_, err := fmt.Fprintln(w, "no export templates")
			return err
		}
		for _, template := range templates {
			fmt.Fprintf(w, "%-24s %-6s %s\n", template.Name, template.Format, template.OutputPath)
		}
		return nil
	})
}

func (c *cli) exportSave(args []string) error {
	fs := c.newFlagSet("export save")
	file := fs.String("file", "", "path to a template document (JSON, same shape as the GUI)")
	if err := fs.Parse(args); err != nil {
		return c.usage("%v", err)
	}
	if fs.NArg() != 0 {
		return c.usage("export save takes no positional arguments")
	}
	if strings.TrimSpace(*file) == "" {
		return c.usage("export save needs --file <json>")
	}

	raw, err := os.ReadFile(filepath.Clean(*file)) // #nosec G304 -- user-supplied template file
	if err != nil {
		return fmt.Errorf("Failed to read template file: %w", err)
	}
	var template export.ExportTemplate
	if err := json.Unmarshal(raw, &template); err != nil {
		return c.usage("invalid template JSON: %v", err)
	}
	if strings.TrimSpace(template.Name) == "" {
		return c.usage("template needs a non-empty \"name\"")
	}

	ctx, cancel := c.opts.commandContext()
	defer cancel()
	if err := export.SaveTemplate(ctx, template); err != nil {
		return err
	}
	return emit(c.opts, c.stdout, map[string]any{"applied": true, "name": template.Name},
		func(w io.Writer) error {
			_, err := fmt.Fprintf(w, "saved template %q\n", template.Name)
			return err
		})
}

func (c *cli) exportDelete(args []string) error {
	if len(args) != 1 {
		return c.usage("export delete needs <name>")
	}
	name := args[0]

	ctx, cancel := c.opts.commandContext()
	defer cancel()

	if err := requireTemplate(ctx, name); err != nil {
		return err
	}
	if err := export.DeleteTemplate(ctx, name); err != nil {
		return err
	}
	return emit(c.opts, c.stdout, map[string]any{"applied": true, "name": name},
		func(w io.Writer) error {
			_, err := fmt.Fprintf(w, "deleted template %q\n", name)
			return err
		})
}

func (c *cli) exportRun(args []string) error {
	if len(args) != 1 {
		return c.usage("export run needs <name>")
	}
	name := args[0]

	ctx, cancel := c.opts.commandContext()
	defer cancel()

	if err := requireTemplate(ctx, name); err != nil {
		return err
	}
	outcome, err := export.Run(ctx, name, export.RunOptions{
		DryRun: c.opts.dryRun,
		Reveal: c.opts.reveal,
	})
	if err != nil {
		return err
	}

	if err := emit(c.opts, c.stdout, outcome, nil); err != nil {
		return err
	}
	if !c.opts.jsonOut {
		verb := "wrote"
		if !outcome.Applied {
			verb = "would write"
		}
		fmt.Fprintf(c.stdout, "%s %d record(s) from %d channel(s) to %s\n",
			verb, outcome.Result.RecordCount, outcome.Result.ChannelsCount, outcome.Result.OutputPath)
		// Warnings are diagnostics, not the answer: keep them on stderr so
		// stdout stays a single parseable document.
		for _, warning := range outcome.Result.Warnings {
			fmt.Fprintf(c.stderr, "warning: %s\n", warning)
		}
	}
	return nil
}

// requireTemplate turns "template missing" into the not_found exit path with
// a stable code, instead of leaving the caller to parse an error string.
func requireTemplate(ctx context.Context, name string) error {
	templates, err := export.LoadTemplates(ctx)
	if err != nil {
		return err
	}
	for _, template := range templates {
		if template.Name == name {
			return nil
		}
	}
	return fmt.Errorf("%w: export template %q", errNotFound, name)
}
