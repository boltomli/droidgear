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
	"droidgear-wails/internal/service/codex"
	"droidgear-wails/internal/service/export"
	"droidgear-wails/internal/service/hermes"
	"droidgear-wails/internal/service/omp"
	"droidgear-wails/internal/service/paths"
	"droidgear-wails/internal/service/pi"
	"droidgear-wails/internal/version"
)

// cli carries the parsed global flags plus the streams of one invocation.
type cli struct {
	opts   options
	stdout io.Writer
	stderr io.Writer
	// previewedOwn marks a command that produced its own dry-run preview
	// (export run carries counts and warnings a bare ChangeSet would lose),
	// so run() keeps that output instead of substituting the ChangeSet.
	previewedOwn bool
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

// parseMixed lets flags travel after positionals (`save <name> --file x`):
// Go's flag package stops parsing at the first positional, so the tokens are
// split here — flags first, positionals handed back in their original order.
func parseMixed(fs *flag.FlagSet, args []string) ([]string, error) {
	var flags, rest []string
	for i := 0; i < len(args); i++ {
		arg := args[i]
		if !strings.HasPrefix(arg, "--") {
			rest = append(rest, arg)
			continue
		}
		flags = append(flags, arg)
		name := strings.TrimPrefix(arg, "--")
		if strings.Contains(name, "=") {
			continue
		}
		def := fs.Lookup(name)
		if def == nil {
			continue // let fs.Parse report "flag provided but not defined"
		}
		if bf, ok := def.Value.(interface{ IsBoolFlag() bool }); ok && bf.IsBoolFlag() {
			continue
		}
		if i+1 >= len(args) {
			return nil, fmt.Errorf("flag --%s needs a value", name)
		}
		i++
		flags = append(flags, args[i])
	}
	if err := fs.Parse(flags); err != nil {
		return nil, err
	}
	return rest, nil
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

// applyPathChange shares the receipt handling of path set|reset. In a dry run
// the write is still issued — core records it instead of performing it — and
// run() replaces this receipt with the ChangeSet, so `applied` reflects what
// really happened rather than what was asked for.
func (c *cli) applyPathChange(key string, value string, write func(context.Context) error) error {
	ctx, cancel := c.opts.commandContext()
	defer cancel()
	if err := write(ctx); err != nil {
		return err
	}
	applied := !c.opts.dryRun
	receipt := map[string]any{"applied": applied, "key": key, "path": value}
	human := func(w io.Writer) error {
		verb := "set"
		if value == "" {
			verb = "reset"
		}
		if !applied {
			verb = "would " + verb
		}
		if value == "" {
			_, err := fmt.Fprintf(w, "%s %s\n", verb, key)
			return err
		}
		_, err := fmt.Fprintf(w, "%s %s = %s\n", verb, key, value)
		return err
	}
	return emit(c.opts, c.stdout, receipt, human)
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
// dg codex
// ---------------------------------------------------------------------------

func (c *cli) codex(args []string) error {
	if len(args) == 0 {
		return c.usage("codex needs a subcommand: profiles | status")
	}
	switch args[0] {
	case "profiles":
		return c.codexProfiles(args[1:])
	case "status":
		return c.codexStatus(args[1:])
	default:
		return c.usage("unknown codex subcommand %q (want profiles | status)", args[0])
	}
}

func (c *cli) codexProfiles(args []string) error {
	if len(args) == 0 {
		return c.usage("codex profiles needs a subcommand: list | show | active | apply | save | delete")
	}
	switch args[0] {
	case "list":
		return c.codexProfilesList(args[1:])
	case "show":
		return c.codexProfilesShow(args[1:])
	case "active":
		return c.codexProfilesActive(args[1:])
	case "apply":
		return c.codexProfilesApply(args[1:])
	case "save":
		return c.codexProfilesSave(args[1:])
	case "delete":
		return c.codexProfilesDelete(args[1:])
	default:
		return c.usage("unknown codex profiles subcommand %q (want list | show | active | apply | save | delete)", args[0])
	}
}

// requireCodexProfile turns "profile missing" into the not_found exit path
// instead of a generic runtime error.
func (c *cli) requireCodexProfile(id string) error {
	profiles, err := codex.ListProfiles()
	if err != nil {
		return err
	}
	for _, profile := range profiles {
		if profile.ID == id {
			return nil
		}
	}
	return fmt.Errorf("%w: codex profile %q", errNotFound, id)
}

// redactCodexProfile masks the API keys a profile carries. Profiles are the
// one codex surface where secrets travel in the document itself (top-level
// apiKey plus a per-provider apiKey), so both need masking (§5.3).
func redactCodexProfile(profile codex.CodexProfile) codex.CodexProfile {
	if profile.APIKey != nil {
		masked := core.RedactSecret(*profile.APIKey)
		profile.APIKey = &masked
	}
	if profile.Providers == nil {
		return profile
	}
	providers := make(map[string]codex.CodexProviderConfig, len(profile.Providers))
	for id, provider := range profile.Providers {
		if provider.APIKey != nil {
			masked := core.RedactSecret(*provider.APIKey)
			provider.APIKey = &masked
		}
		providers[id] = provider
	}
	profile.Providers = providers
	return profile
}

func (c *cli) codexProfilesList(args []string) error {
	if len(args) != 0 {
		return c.usage("codex profiles list takes no arguments")
	}
	profiles, err := codex.ListProfiles()
	if err != nil {
		return err
	}
	if !c.opts.reveal {
		for i := range profiles {
			profiles[i] = redactCodexProfile(profiles[i])
		}
	}
	return emit(c.opts, c.stdout, profiles, func(w io.Writer) error {
		if len(profiles) == 0 {
			_, err := fmt.Fprintln(w, "no codex profiles")
			return err
		}
		for _, profile := range profiles {
			fmt.Fprintf(w, "%-24s %-20s %s\n", profile.ID, profile.Name, profile.Model)
		}
		return nil
	})
}

func (c *cli) codexProfilesShow(args []string) error {
	if len(args) != 1 {
		return c.usage("codex profiles show needs <id>")
	}
	if err := c.requireCodexProfile(args[0]); err != nil {
		return err
	}
	profile, err := codex.GetProfile(args[0])
	if err != nil {
		return err
	}
	if !c.opts.reveal {
		profile = redactCodexProfile(profile)
	}
	return emit(c.opts, c.stdout, profile, func(w io.Writer) error {
		fmt.Fprintf(w, "id:        %s\n", profile.ID)
		fmt.Fprintf(w, "name:      %s\n", profile.Name)
		fmt.Fprintf(w, "provider:  %s\n", profile.ModelProvider)
		fmt.Fprintf(w, "model:     %s\n", profile.Model)
		fmt.Fprintf(w, "providers: %d\n", len(profile.Providers))
		if profile.APIKey != nil {
			fmt.Fprintf(w, "apiKey:    %s\n", *profile.APIKey)
		}
		return nil
	})
}

func (c *cli) codexProfilesActive(args []string) error {
	if len(args) != 0 {
		return c.usage("codex profiles active takes no arguments")
	}
	active, err := codex.GetActiveProfileID()
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

func (c *cli) codexProfilesApply(args []string) error {
	if len(args) != 1 {
		return c.usage("codex profiles apply needs <id>")
	}
	id := args[0]
	if err := c.requireCodexProfile(id); err != nil {
		return err
	}
	if err := codex.ApplyProfile(id); err != nil {
		return err
	}
	return emit(c.opts, c.stdout, map[string]any{"applied": true, "id": id}, func(w io.Writer) error {
		_, err := fmt.Fprintf(w, "applied codex profile %s\n", id)
		return err
	})
}

func (c *cli) codexProfilesSave(args []string) error {
	fs := c.newFlagSet("codex profiles save")
	file := fs.String("file", "", "path to a profile document (JSON, same shape as the GUI)")
	if err := fs.Parse(args); err != nil {
		return c.usage("%v", err)
	}
	if fs.NArg() != 0 {
		return c.usage("codex profiles save takes no positional arguments")
	}
	if strings.TrimSpace(*file) == "" {
		return c.usage("codex profiles save needs --file <json>")
	}

	raw, err := os.ReadFile(filepath.Clean(*file)) // #nosec G304 -- user-supplied profile file
	if err != nil {
		return fmt.Errorf("Failed to read profile file: %w", err)
	}
	var profile codex.CodexProfile
	if err := json.Unmarshal(raw, &profile); err != nil {
		return c.usage("invalid profile JSON: %v", err)
	}
	if strings.TrimSpace(profile.Name) == "" {
		return c.usage("profile needs a non-empty \"name\"")
	}
	if err := codex.SaveProfile(profile); err != nil {
		return err
	}
	return emit(c.opts, c.stdout, map[string]any{"applied": true, "name": profile.Name}, func(w io.Writer) error {
		_, err := fmt.Fprintf(w, "saved codex profile %q\n", profile.Name)
		return err
	})
}

func (c *cli) codexProfilesDelete(args []string) error {
	if len(args) != 1 {
		return c.usage("codex profiles delete needs <id>")
	}
	id := args[0]
	if err := c.requireCodexProfile(id); err != nil {
		return err
	}
	if err := codex.DeleteProfile(id); err != nil {
		return err
	}
	return emit(c.opts, c.stdout, map[string]any{"applied": true, "id": id}, func(w io.Writer) error {
		_, err := fmt.Fprintf(w, "deleted codex profile %s\n", id)
		return err
	})
}

func (c *cli) codexStatus(args []string) error {
	if len(args) != 0 {
		return c.usage("codex status takes no arguments")
	}
	status, err := codex.GetConfigStatus()
	if err != nil {
		return err
	}
	active, err := codex.GetActiveProfileID()
	if err != nil {
		return err
	}
	report := struct {
		codex.CodexConfigStatus
		ActiveProfileID *string `json:"activeProfileId"`
	}{CodexConfigStatus: status, ActiveProfileID: active}
	return emit(c.opts, c.stdout, report, func(w io.Writer) error {
		fmt.Fprintf(w, "config: %s (%s)\n", status.ConfigPath, presentOr(status.ConfigExists))
		fmt.Fprintf(w, "auth:   %s (%s)\n", status.AuthPath, presentOr(status.AuthExists))
		if active == nil {
			fmt.Fprintln(w, "active: (none)")
		} else {
			fmt.Fprintf(w, "active: %s\n", *active)
		}
		return nil
	})
}

func presentOr(exists bool) string {
	if exists {
		return "present"
	}
	return "missing"
}

// ---------------------------------------------------------------------------
// dg hermes
// ---------------------------------------------------------------------------

func (c *cli) hermes(args []string) error {
	if len(args) == 0 {
		return c.usage("hermes needs a subcommand: profiles | providers | status")
	}
	switch args[0] {
	case "profiles":
		return c.hermesProfiles(args[1:])
	case "providers":
		return c.hermesProviders(args[1:])
	case "status":
		return c.hermesStatus(args[1:])
	default:
		return c.usage("unknown hermes subcommand %q (want profiles | providers | status)", args[0])
	}
}

func (c *cli) hermesProfiles(args []string) error {
	if len(args) == 0 {
		return c.usage("hermes profiles needs a subcommand: list | show | active | apply | save | delete")
	}
	switch args[0] {
	case "list":
		return c.hermesProfilesList(args[1:])
	case "show":
		return c.hermesProfilesShow(args[1:])
	case "active":
		return c.hermesProfilesActive(args[1:])
	case "apply":
		return c.hermesProfilesApply(args[1:])
	case "save":
		return c.hermesProfilesSave(args[1:])
	case "delete":
		return c.hermesProfilesDelete(args[1:])
	default:
		return c.usage("unknown hermes profiles subcommand %q (want list | show | active | apply | save | delete)", args[0])
	}
}

func (c *cli) requireHermesProfile(id string) error {
	profiles, err := hermes.ListProfiles()
	if err != nil {
		return err
	}
	for _, profile := range profiles {
		if profile.ID == id {
			return nil
		}
	}
	return fmt.Errorf("%w: hermes profile %q", errNotFound, id)
}

// redactHermesProfile masks the per-model API keys a hermes profile carries
// (each entry of Models[] may hold one, §5.3).
func redactHermesProfile(profile hermes.HermesProfile) hermes.HermesProfile {
	if profile.Models == nil {
		return profile
	}
	models := make([]hermes.HermesModelConfig, len(profile.Models))
	for i, model := range profile.Models {
		if model.APIKey != nil {
			masked := core.RedactSecret(*model.APIKey)
			model.APIKey = &masked
		}
		models[i] = model
	}
	profile.Models = models
	return profile
}

func (c *cli) hermesProfilesList(args []string) error {
	if len(args) != 0 {
		return c.usage("hermes profiles list takes no arguments")
	}
	profiles, err := hermes.ListProfiles()
	if err != nil {
		return err
	}
	if !c.opts.reveal {
		for i := range profiles {
			profiles[i] = redactHermesProfile(profiles[i])
		}
	}
	return emit(c.opts, c.stdout, profiles, func(w io.Writer) error {
		if len(profiles) == 0 {
			_, err := fmt.Fprintln(w, "no hermes profiles")
			return err
		}
		for _, profile := range profiles {
			fmt.Fprintf(w, "%-24s %-20s %d model(s)\n", profile.ID, profile.Name, len(profile.Models))
		}
		return nil
	})
}

func (c *cli) hermesProfilesShow(args []string) error {
	if len(args) != 1 {
		return c.usage("hermes profiles show needs <id>")
	}
	if err := c.requireHermesProfile(args[0]); err != nil {
		return err
	}
	profile, err := hermes.GetProfile(args[0])
	if err != nil {
		return err
	}
	if !c.opts.reveal {
		profile = redactHermesProfile(profile)
	}
	return emit(c.opts, c.stdout, profile, func(w io.Writer) error {
		fmt.Fprintf(w, "id:     %s\n", profile.ID)
		fmt.Fprintf(w, "name:   %s\n", profile.Name)
		for _, model := range profile.Models {
			key := ""
			if model.APIKey != nil {
				key = " (api key set)"
			}
			name := ""
			if model.Name != nil {
				name = *model.Name
			}
			fmt.Fprintf(w, "model:  %s%s\n", name, key)
		}
		return nil
	})
}

func (c *cli) hermesProfilesActive(args []string) error {
	if len(args) != 0 {
		return c.usage("hermes profiles active takes no arguments")
	}
	active, err := hermes.GetActiveProfileID()
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

func (c *cli) hermesProfilesApply(args []string) error {
	if len(args) != 1 {
		return c.usage("hermes profiles apply needs <id>")
	}
	id := args[0]
	if err := c.requireHermesProfile(id); err != nil {
		return err
	}
	if err := hermes.ApplyProfile(id); err != nil {
		return err
	}
	return emit(c.opts, c.stdout, map[string]any{"applied": true, "id": id}, func(w io.Writer) error {
		_, err := fmt.Fprintf(w, "applied hermes profile %s\n", id)
		return err
	})
}

func (c *cli) hermesProfilesSave(args []string) error {
	fs := c.newFlagSet("hermes profiles save")
	file := fs.String("file", "", "path to a profile document (JSON, same shape as the GUI)")
	if err := fs.Parse(args); err != nil {
		return c.usage("%v", err)
	}
	if fs.NArg() != 0 {
		return c.usage("hermes profiles save takes no positional arguments")
	}
	if strings.TrimSpace(*file) == "" {
		return c.usage("hermes profiles save needs --file <json>")
	}

	raw, err := os.ReadFile(filepath.Clean(*file)) // #nosec G304 -- user-supplied profile file
	if err != nil {
		return fmt.Errorf("Failed to read profile file: %w", err)
	}
	var profile hermes.HermesProfile
	if err := json.Unmarshal(raw, &profile); err != nil {
		return c.usage("invalid profile JSON: %v", err)
	}
	if strings.TrimSpace(profile.Name) == "" {
		return c.usage("profile needs a non-empty \"name\"")
	}
	if err := hermes.SaveProfile(profile); err != nil {
		return err
	}
	return emit(c.opts, c.stdout, map[string]any{"applied": true, "name": profile.Name}, func(w io.Writer) error {
		_, err := fmt.Fprintf(w, "saved hermes profile %q\n", profile.Name)
		return err
	})
}

func (c *cli) hermesProfilesDelete(args []string) error {
	if len(args) != 1 {
		return c.usage("hermes profiles delete needs <id>")
	}
	id := args[0]
	if err := c.requireHermesProfile(id); err != nil {
		return err
	}
	if err := hermes.DeleteProfile(id); err != nil {
		return err
	}
	return emit(c.opts, c.stdout, map[string]any{"applied": true, "id": id}, func(w io.Writer) error {
		_, err := fmt.Fprintf(w, "deleted hermes profile %s\n", id)
		return err
	})
}

func (c *cli) hermesStatus(args []string) error {
	if len(args) != 0 {
		return c.usage("hermes status takes no arguments")
	}
	status, err := hermes.GetConfigStatus()
	if err != nil {
		return err
	}
	active, err := hermes.GetActiveProfileID()
	if err != nil {
		return err
	}
	report := struct {
		hermes.HermesConfigStatus
		ActiveProfileID *string `json:"activeProfileId"`
	}{HermesConfigStatus: status, ActiveProfileID: active}
	return emit(c.opts, c.stdout, report, func(w io.Writer) error {
		fmt.Fprintf(w, "config: %s (%s)\n", status.ConfigPath, presentOr(status.ConfigExists))
		if active == nil {
			fmt.Fprintln(w, "active: (none)")
		} else {
			fmt.Fprintf(w, "active: %s\n", *active)
		}
		return nil
	})
}

// ---------------------------------------------------------------------------
// dg pi
// ---------------------------------------------------------------------------

func (c *cli) pi(args []string) error {
	if len(args) == 0 {
		return c.usage("pi needs a subcommand: profiles | providers | settings | status")
	}
	switch args[0] {
	case "profiles":
		return c.piProfiles(args[1:])
	case "providers":
		return c.piProviders(args[1:])
	case "settings":
		return c.piSettings(args[1:])
	case "status":
		return c.piStatus(args[1:])
	default:
		return c.usage("unknown pi subcommand %q (want profiles | providers | settings | status)", args[0])
	}
}

// ---------------------------------------------------------------------------
// dg pi settings — the startup model selection in ~/.pi/agent/settings.json
// ---------------------------------------------------------------------------

func (c *cli) piSettings(args []string) error {
	if len(args) == 0 {
		return c.usage("pi settings needs a subcommand: show | set | reset | candidates")
	}
	switch args[0] {
	case "show":
		return c.piSettingsShow(args[1:])
	case "set":
		return c.piSettingsSet(args[1:])
	case "reset":
		return c.piSettingsReset(args[1:])
	case "candidates":
		return c.piSettingsCandidates(args[1:])
	default:
		return c.usage("unknown pi settings subcommand %q (want show | set | reset | candidates)", args[0])
	}
}

// piSettingsCandidates prints the provider/model pairs the startup model can
// point at (models.json plus pi's cached catalog) — the same list the GUI
// pickers and the TUI Defaults screen offer.
func (c *cli) piSettingsCandidates(args []string) error {
	if len(args) != 0 {
		return c.usage("pi settings candidates takes no arguments")
	}
	options, err := pi.ListStartupOptions()
	if err != nil {
		return err
	}
	return emit(c.opts, c.stdout, options, func(w io.Writer) error {
		if len(options.Providers) == 0 {
			_, err := fmt.Fprintln(w, "no providers known (models.json and pi's catalog are both empty)")
			return err
		}
		for _, providerID := range options.Providers {
			models := options.Models[providerID]
			if len(models) == 0 {
				fmt.Fprintf(w, "%s: (no models listed)\\n", providerID)
				continue
			}
			for _, modelID := range models {
				fmt.Fprintf(w, "%s/%s\\n", providerID, modelID)
			}
		}
		return nil
	})
}

// settingOrAuto renders an unset setting the way pi reads it: automatic.
func settingOrAuto(value *string) string {
	if value == nil || strings.TrimSpace(*value) == "" {
		return "(auto)"
	}
	return *value
}

func settingFlagValue(value string) *string {
	trimmed := strings.TrimSpace(value)
	if trimmed == "" {
		return nil
	}
	return &trimmed
}

// piSettingsShowResult is PiSettingsStatus plus the advisory notes that explain
// a selection pi cannot resolve (same wording as `pi settings set`).
type piSettingsShowResult struct {
	pi.PiSettingsStatus
	Warnings []string `json:"warnings,omitempty"`
}

func (c *cli) piSettingsShow(args []string) error {
	if len(args) != 0 {
		return c.usage("pi settings show takes no arguments")
	}
	status, err := pi.ReadSettings()
	if err != nil {
		return err
	}
	warnings, err := pi.StartupWarnings(status.PiSettings)
	if err != nil {
		return err
	}
	result := piSettingsShowResult{PiSettingsStatus: status, Warnings: warnings}
	return emit(c.opts, c.stdout, result, func(w io.Writer) error {
		state := "missing"
		if status.SettingsExists {
			state = "present"
		}
		fmt.Fprintf(w, "file:      %s (%s)\n", status.SettingsPath, state)
		fmt.Fprintf(w, "provider:  %s\n", settingOrAuto(status.DefaultProvider))
		fmt.Fprintf(w, "model:     %s\n", settingOrAuto(status.DefaultModel))
		fmt.Fprintf(w, "thinking:  %s\n", settingOrAuto(status.DefaultThinkingLevel))
		for _, warning := range warnings {
			fmt.Fprintf(w, "warning:   %s\n", warning)
		}
		return nil
	})
}

func (c *cli) piSettingsSet(args []string) error {
	fs := c.newFlagSet("pi settings set")
	provider := fs.String("provider", "", "default provider id (empty clears it)")
	model := fs.String("model", "", "default model id (empty clears it)")
	thinking := fs.String("thinking", "", "default thinking level: off|minimal|low|medium|high|xhigh|max (empty clears it)")
	if err := fs.Parse(args); err != nil {
		return c.usage("%v", err)
	}
	if fs.NArg() != 0 {
		return c.usage("pi settings set takes no positional arguments")
	}
	visited := map[string]bool{}
	fs.Visit(func(f *flag.Flag) { visited[f.Name] = true })
	if len(visited) == 0 {
		return c.usage("pi settings set needs at least one of --provider, --model, --thinking")
	}

	current, err := pi.ReadSettings()
	if err != nil {
		return err
	}
	next := current.PiSettings
	if visited["provider"] {
		next.DefaultProvider = settingFlagValue(*provider)
	}
	if visited["model"] {
		next.DefaultModel = settingFlagValue(*model)
	}
	if visited["thinking"] {
		next.DefaultThinkingLevel = settingFlagValue(*thinking)
	}
	if err := pi.SaveSettings(next); err != nil {
		return err
	}
	warnings, err := pi.StartupWarnings(next)
	if err != nil {
		return err
	}

	applied := !c.opts.dryRun
	receipt := map[string]any{
		"applied":              applied,
		"defaultProvider":      next.DefaultProvider,
		"defaultModel":         next.DefaultModel,
		"defaultThinkingLevel": next.DefaultThinkingLevel,
		"settingsPath":         current.SettingsPath,
	}
	if len(warnings) > 0 {
		receipt["warnings"] = warnings
	}
	return emit(c.opts, c.stdout, receipt, func(w io.Writer) error {
		verb := "set"
		if !applied {
			verb = "would set"
		}
		fmt.Fprintf(w, "%s pi default model: %s / %s (thinking %s)\n",
			verb, settingOrAuto(next.DefaultProvider), settingOrAuto(next.DefaultModel),
			settingOrAuto(next.DefaultThinkingLevel))
		for _, warning := range warnings {
			fmt.Fprintf(w, "warning: %s\n", warning)
		}
		return nil
	})
}

func (c *cli) piSettingsReset(args []string) error {
	if len(args) != 0 {
		return c.usage("pi settings reset takes no arguments")
	}
	if err := pi.ClearSettings(); err != nil {
		return err
	}
	applied := !c.opts.dryRun
	return emit(c.opts, c.stdout, map[string]any{"applied": applied, "cleared": true}, func(w io.Writer) error {
		if applied {
			_, err := fmt.Fprintln(w, "cleared pi default model (pi picks automatically)")
			return err
		}
		_, err := fmt.Fprintln(w, "would clear pi default model")
		return err
	})
}

func (c *cli) piProfiles(args []string) error {
	if len(args) == 0 {
		return c.usage("pi profiles needs a subcommand: list | show | active | apply | save | delete")
	}
	switch args[0] {
	case "list":
		return c.piProfilesList(args[1:])
	case "show":
		return c.piProfilesShow(args[1:])
	case "active":
		return c.piProfilesActive(args[1:])
	case "apply":
		return c.piProfilesApply(args[1:])
	case "save":
		return c.piProfilesSave(args[1:])
	case "delete":
		return c.piProfilesDelete(args[1:])
	default:
		return c.usage("unknown pi profiles subcommand %q (want list | show | active | apply | save | delete)", args[0])
	}
}

func (c *cli) requirePiProfile(id string) error {
	profiles, err := pi.ListProfiles()
	if err != nil {
		return err
	}
	for _, profile := range profiles {
		if profile.ID == id {
			return nil
		}
	}
	return fmt.Errorf("%w: pi profile %q", errNotFound, id)
}

// redactPiProfile masks the two secrets a pi provider carries: the API key
// and the OAuth token blob (§5.3).
func redactPiProfile(profile pi.PiProfile) pi.PiProfile {
	if profile.Providers == nil {
		return profile
	}
	providers := make(map[string]pi.PiProviderConfig, len(profile.Providers))
	for id, provider := range profile.Providers {
		if provider.APIKey != nil {
			masked := core.RedactSecret(*provider.APIKey)
			provider.APIKey = &masked
		}
		if provider.OAuth != nil {
			masked := core.RedactSecret(*provider.OAuth)
			provider.OAuth = &masked
		}
		providers[id] = provider
	}
	profile.Providers = providers
	return profile
}

func (c *cli) piProfilesList(args []string) error {
	if len(args) != 0 {
		return c.usage("pi profiles list takes no arguments")
	}
	profiles, err := pi.ListProfiles()
	if err != nil {
		return err
	}
	if !c.opts.reveal {
		for i := range profiles {
			profiles[i] = redactPiProfile(profiles[i])
		}
	}
	return emit(c.opts, c.stdout, profiles, func(w io.Writer) error {
		if len(profiles) == 0 {
			_, err := fmt.Fprintln(w, "no pi profiles")
			return err
		}
		for _, profile := range profiles {
			fmt.Fprintf(w, "%-24s %-20s %d provider(s)\n", profile.ID, profile.Name, len(profile.Providers))
		}
		return nil
	})
}

func (c *cli) piProfilesShow(args []string) error {
	if len(args) != 1 {
		return c.usage("pi profiles show needs <id>")
	}
	if err := c.requirePiProfile(args[0]); err != nil {
		return err
	}
	profile, err := pi.GetProfile(args[0])
	if err != nil {
		return err
	}
	if !c.opts.reveal {
		profile = redactPiProfile(profile)
	}
	return emit(c.opts, c.stdout, profile, func(w io.Writer) error {
		fmt.Fprintf(w, "id:        %s\n", profile.ID)
		fmt.Fprintf(w, "name:      %s\n", profile.Name)
		for id, provider := range profile.Providers {
			fmt.Fprintf(w, "provider:  %s\n", id)
			if provider.BaseURL != nil {
				fmt.Fprintf(w, "  baseUrl: %s\n", *provider.BaseURL)
			}
			if provider.APIKey != nil {
				fmt.Fprintf(w, "  apiKey:  %s\n", *provider.APIKey)
			}
		}
		return nil
	})
}

func (c *cli) piProfilesActive(args []string) error {
	if len(args) != 0 {
		return c.usage("pi profiles active takes no arguments")
	}
	active, err := pi.GetActiveProfileID()
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

func (c *cli) piProfilesApply(args []string) error {
	if len(args) != 1 {
		return c.usage("pi profiles apply needs <id>")
	}
	id := args[0]
	if err := c.requirePiProfile(id); err != nil {
		return err
	}
	result, err := pi.ApplyProfile(id)
	if err != nil {
		return err
	}
	fields, notes := piStartupReceipt(result)
	receipt := map[string]any{
		"applied":          !c.opts.dryRun,
		"id":               result.ID,
		"appliedProviders": result.AppliedProviders,
	}
	for key, value := range fields {
		receipt[key] = value
	}
	if len(notes) > 0 {
		receipt["startupReasons"] = result.StartupReasons
	}
	return emit(c.opts, c.stdout, receipt, func(w io.Writer) error {
		if _, err := fmt.Fprintf(w, "applied pi profile %s (%s)\n", id, strings.Join(result.AppliedProviders, ", ")); err != nil {
			return err
		}
		fmt.Fprintf(w, "startup model: %s / %s (thinking %s)\n",
			settingOrAuto(result.Startup.DefaultProvider),
			settingOrAuto(result.Startup.DefaultModel),
			settingOrAuto(result.Startup.DefaultThinkingLevel))
		for _, note := range notes {
			fmt.Fprintf(w, "adjusted: %s\n", note)
		}
		return nil
	})
}

// piStartupReceipt turns a pi apply/delete result into receipt fields plus the
// human-readable startup-model notes shared by `pi profiles apply` and
// `pi providers delete`.
func piStartupReceipt(result pi.PiApplyResult) (map[string]any, []string) {
	fields := map[string]any{
		"startup":         result.Startup,
		"startupAdjusted": result.StartupAdjusted,
	}
	return fields, result.StartupReasons
}

func (c *cli) piProfilesSave(args []string) error {
	fs := c.newFlagSet("pi profiles save")
	file := fs.String("file", "", "path to a profile document (JSON, same shape as the GUI)")
	if err := fs.Parse(args); err != nil {
		return c.usage("%v", err)
	}
	if fs.NArg() != 0 {
		return c.usage("pi profiles save takes no positional arguments")
	}
	if strings.TrimSpace(*file) == "" {
		return c.usage("pi profiles save needs --file <json>")
	}

	raw, err := os.ReadFile(filepath.Clean(*file)) // #nosec G304 -- user-supplied profile file
	if err != nil {
		return fmt.Errorf("Failed to read profile file: %w", err)
	}
	var profile pi.PiProfile
	if err := json.Unmarshal(raw, &profile); err != nil {
		return c.usage("invalid profile JSON: %v", err)
	}
	if strings.TrimSpace(profile.Name) == "" {
		return c.usage("profile needs a non-empty \"name\"")
	}
	if err := pi.SaveProfile(profile); err != nil {
		return err
	}
	return emit(c.opts, c.stdout, map[string]any{"applied": true, "name": profile.Name}, func(w io.Writer) error {
		_, err := fmt.Fprintf(w, "saved pi profile %q\n", profile.Name)
		return err
	})
}

func (c *cli) piProfilesDelete(args []string) error {
	if len(args) != 1 {
		return c.usage("pi profiles delete needs <id>")
	}
	id := args[0]
	if err := c.requirePiProfile(id); err != nil {
		return err
	}
	if err := pi.DeleteProfile(id); err != nil {
		return err
	}
	return emit(c.opts, c.stdout, map[string]any{"applied": true, "id": id}, func(w io.Writer) error {
		_, err := fmt.Fprintf(w, "deleted pi profile %s\n", id)
		return err
	})
}

func (c *cli) piStatus(args []string) error {
	if len(args) != 0 {
		return c.usage("pi status takes no arguments")
	}
	status, err := pi.GetConfigStatus()
	if err != nil {
		return err
	}
	active, err := pi.GetActiveProfileID()
	if err != nil {
		return err
	}
	report := struct {
		pi.PiConfigStatus
		ActiveProfileID *string `json:"activeProfileId"`
	}{PiConfigStatus: status, ActiveProfileID: active}
	return emit(c.opts, c.stdout, report, func(w io.Writer) error {
		fmt.Fprintf(w, "config: %s (%s)\n", status.ConfigPath, presentOr(status.ConfigExists))
		if active == nil {
			fmt.Fprintln(w, "active: (none)")
		} else {
			fmt.Fprintf(w, "active: %s\n", *active)
		}
		return nil
	})
}

// ---------------------------------------------------------------------------
// dg omp
// ---------------------------------------------------------------------------

func (c *cli) omp(args []string) error {
	if len(args) == 0 {
		return c.usage("omp needs a subcommand: profiles | providers | roles | status")
	}
	switch args[0] {
	case "profiles":
		return c.ompProfiles(args[1:])
	case "providers":
		return c.ompProviders(args[1:])
	case "roles":
		return c.ompRoles(args[1:])
	case "status":
		return c.ompStatus(args[1:])
	default:
		return c.usage("unknown omp subcommand %q (want profiles | providers | roles | status)", args[0])
	}
}

func (c *cli) ompRoles(args []string) error {
	if len(args) == 0 {
		return c.usage("omp roles needs a subcommand: candidates")
	}
	switch args[0] {
	case "candidates":
		return c.ompRolesCandidates(args[1:])
	default:
		return c.usage("unknown omp roles subcommand %q (want candidates)", args[0])
	}
}

// ompRolesCandidates prints every "provider/model-id" a model role can point
// at (models.yml plus OMP's cached catalog) — the same list the GUI pickers and
// the TUI hints use.
func (c *cli) ompRolesCandidates(args []string) error {
	if len(args) != 0 {
		return c.usage("omp roles candidates takes no arguments")
	}
	options, err := omp.ListRoleOptions()
	if err != nil {
		return err
	}
	return emit(c.opts, c.stdout, options, func(w io.Writer) error {
		values := omp.RoleValues(options)
		if len(values) == 0 {
			_, err := fmt.Fprintln(w, "no providers known (models.yml and OMP's catalog are both empty)")
			return err
		}
		for _, value := range values {
			if _, err := fmt.Fprintln(w, value); err != nil {
				return err
			}
		}
		return nil
	})
}

func (c *cli) ompProfiles(args []string) error {
	if len(args) == 0 {
		return c.usage("omp profiles needs a subcommand: list | show | active | apply | save | delete")
	}
	switch args[0] {
	case "list":
		return c.ompProfilesList(args[1:])
	case "show":
		return c.ompProfilesShow(args[1:])
	case "active":
		return c.ompProfilesActive(args[1:])
	case "apply":
		return c.ompProfilesApply(args[1:])
	case "save":
		return c.ompProfilesSave(args[1:])
	case "delete":
		return c.ompProfilesDelete(args[1:])
	default:
		return c.usage("unknown omp profiles subcommand %q (want list | show | active | apply | save | delete)", args[0])
	}
}

func (c *cli) ompProviders(args []string) error {
	if len(args) == 0 {
		return c.usage("omp providers needs a subcommand: list | save | delete | import")
	}
	switch args[0] {
	case "list":
		return c.ompProvidersList(args[1:])
	case "save":
		return c.ompProvidersSave(args[1:])
	case "delete":
		return c.ompProvidersDelete(args[1:])
	case "import":
		return c.ompProvidersImport(args[1:])
	default:
		return c.usage("unknown omp providers subcommand %q (want list | save | delete | import)", args[0])
	}
}

func (c *cli) requireOmpProfile(id string) error {
	profiles, err := omp.ListProfiles()
	if err != nil {
		return err
	}
	for _, profile := range profiles {
		if profile.ID == id {
			return nil
		}
	}
	return fmt.Errorf("%w: omp profile %q", errNotFound, id)
}

// redactOmpProviders masks the apiKey every custom provider may carry
// (§5.3). OMP profiles themselves hold no secrets — only model roles.
func redactOmpProviders(list omp.OmpCustomProviders) omp.OmpCustomProviders {
	for i := range list.Providers {
		provider := &list.Providers[i].Config
		if provider.APIKey != nil {
			masked := core.RedactSecret(*provider.APIKey)
			provider.APIKey = &masked
		}
	}
	return list
}

func (c *cli) ompProfilesList(args []string) error {
	if len(args) != 0 {
		return c.usage("omp profiles list takes no arguments")
	}
	profiles, err := omp.ListProfiles()
	if err != nil {
		return err
	}
	return emit(c.opts, c.stdout, profiles, func(w io.Writer) error {
		if len(profiles) == 0 {
			_, err := fmt.Fprintln(w, "no omp profiles")
			return err
		}
		for _, profile := range profiles {
			fmt.Fprintf(w, "%-24s %-20s\n", profile.ID, profile.Name)
		}
		return nil
	})
}

func (c *cli) ompProfilesShow(args []string) error {
	if len(args) != 1 {
		return c.usage("omp profiles show needs <id>")
	}
	if err := c.requireOmpProfile(args[0]); err != nil {
		return err
	}
	profile, err := omp.GetProfile(args[0])
	if err != nil {
		return err
	}
	return emit(c.opts, c.stdout, profile, func(w io.Writer) error {
		fmt.Fprintf(w, "id:         %s\n", profile.ID)
		fmt.Fprintf(w, "name:       %s\n", profile.Name)
		fmt.Fprintf(w, "modelRoles: %+v\n", profile.ModelRoles)
		return nil
	})
}

func (c *cli) ompProfilesActive(args []string) error {
	if len(args) != 0 {
		return c.usage("omp profiles active takes no arguments")
	}
	active, err := omp.GetActiveProfileID()
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

func (c *cli) ompProfilesApply(args []string) error {
	if len(args) != 1 {
		return c.usage("omp profiles apply needs <id>")
	}
	id := args[0]
	if err := c.requireOmpProfile(id); err != nil {
		return err
	}
	result, err := omp.ApplyProfile(id)
	if err != nil {
		return err
	}
	receipt := map[string]any{
		"applied":    !c.opts.dryRun,
		"id":         result.ID,
		"modelRoles": result.Roles,
	}
	if len(result.Warnings) > 0 {
		receipt["warnings"] = result.Warnings
	}
	return emit(c.opts, c.stdout, receipt, func(w io.Writer) error {
		if _, err := fmt.Fprintf(w, "applied omp profile %s\n", id); err != nil {
			return err
		}
		for _, warning := range result.Warnings {
			if _, err := fmt.Fprintf(w, "warning: %s\n", warning); err != nil {
				return err
			}
		}
		return nil
	})
}

func (c *cli) ompProfilesSave(args []string) error {
	return c.saveJSON("omp profiles", args, "path to a profile document (JSON, same shape as the GUI)",
		func(raw []byte) (string, error) {
			var profile omp.OmpProfile
			if err := json.Unmarshal(raw, &profile); err != nil {
				return "", c.usage("invalid profile JSON: %v", err)
			}
			if strings.TrimSpace(profile.Name) == "" {
				return "", c.usage("profile needs a non-empty \"name\"")
			}
			if err := omp.SaveProfile(profile); err != nil {
				return "", err
			}
			return profile.Name, nil
		})
}

func (c *cli) ompProfilesDelete(args []string) error {
	if len(args) != 1 {
		return c.usage("omp profiles delete needs <id>")
	}
	id := args[0]
	if err := c.requireOmpProfile(id); err != nil {
		return err
	}
	if err := omp.DeleteProfile(id); err != nil {
		return err
	}
	return emit(c.opts, c.stdout, map[string]any{"applied": true, "id": id}, func(w io.Writer) error {
		_, err := fmt.Fprintf(w, "deleted omp profile %s\n", id)
		return err
	})
}

func (c *cli) ompProvidersList(args []string) error {
	if len(args) != 0 {
		return c.usage("omp providers list takes no arguments")
	}
	list, err := omp.ReadCustomProviders()
	if err != nil {
		return err
	}
	if !c.opts.reveal {
		list = redactOmpProviders(list)
	}
	return emit(c.opts, c.stdout, list, func(w io.Writer) error {
		if len(list.Providers) == 0 {
			_, err := fmt.Fprintln(w, "no custom providers")
			return err
		}
		for _, entry := range list.Providers {
			key := ""
			if entry.Config.APIKey != nil {
				key = " (api key set)"
			}
			fmt.Fprintf(w, "%-24s%s\n", entry.ProviderID, key)
		}
		return nil
	})
}

func (c *cli) ompProvidersSave(args []string) error {
	var roles omp.OmpRoleReconcile
	return c.saveJSON("omp providers", args, "path to a provider document ({providerId, config})",
		func(raw []byte) (string, error) {
			var entry omp.OmpCustomProviderEntry
			if err := json.Unmarshal(raw, &entry); err != nil {
				return "", c.usage("invalid provider JSON: %v", err)
			}
			if strings.TrimSpace(entry.ProviderID) == "" {
				return "", c.usage("provider document needs a non-empty \"providerId\"")
			}
			result, err := omp.SaveCustomProvider(entry.ProviderID, entry.Config)
			if err != nil {
				return "", err
			}
			roles = result
			return entry.ProviderID, nil
		},
		func() (map[string]any, []string) { return roleReconcileReceipt(roles) },
	)
}

func (c *cli) ompProvidersDelete(args []string) error {
	if len(args) != 1 {
		return c.usage("omp providers delete needs <provider-id>")
	}
	id := args[0]
	list, err := omp.ReadCustomProviders()
	if err != nil {
		return err
	}
	found := false
	for _, entry := range list.Providers {
		if entry.ProviderID == id {
			found = true
			break
		}
	}
	if !found {
		return fmt.Errorf("%w: omp provider %q", errNotFound, id)
	}
	roles, err := omp.DeleteCustomProvider(id)
	if err != nil {
		return err
	}
	fields, notes := roleReconcileReceipt(roles)
	receipt := map[string]any{"applied": !c.opts.dryRun, "providerId": id}
	for key, value := range fields {
		receipt[key] = value
	}
	if len(notes) > 0 {
		receipt["notes"] = notes
	}
	return emit(c.opts, c.stdout, receipt, func(w io.Writer) error {
		if _, err := fmt.Fprintf(w, "deleted omp provider %s\n", id); err != nil {
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

// roleReconcileReceipt turns an OMP role repair pass into receipt fields plus
// the human-readable notes GUI/TUI/CLI all print.
func roleReconcileReceipt(result omp.OmpRoleReconcile) (map[string]any, []string) {
	fields := map[string]any{
		"rolesAdjusted": result.Adjusted,
		"modelRoles":    result.Roles,
	}
	if len(result.Adjustments) > 0 {
		fields["roleAdjustments"] = result.Adjustments
	}
	notes := make([]string, 0, len(result.Adjustments)+1)
	for _, adjustment := range result.Adjustments {
		target := "(cleared)"
		if adjustment.To != nil {
			target = *adjustment.To
		}
		notes = append(notes, fmt.Sprintf("role %s: %s -> %s (%s)",
			adjustment.Role, adjustment.From, target, adjustment.Reason))
	}
	if result.Thinking != nil {
		fields["thinkingAdjusted"] = true
		fields["defaultThinkingLevel"] = result.Thinking.To
		notes = append(notes, fmt.Sprintf("thinking level: %s -> %s (%s)",
			result.Thinking.From, result.Thinking.To, result.Thinking.Reason))
	}
	return fields, notes
}

func (c *cli) ompStatus(args []string) error {
	if len(args) != 0 {
		return c.usage("omp status takes no arguments")
	}
	status, err := omp.GetConfigStatus()
	if err != nil {
		return err
	}
	active, err := omp.GetActiveProfileID()
	if err != nil {
		return err
	}
	report := struct {
		omp.OmpConfigStatus
		ActiveProfileID *string `json:"activeProfileId"`
	}{OmpConfigStatus: status, ActiveProfileID: active}
	return emit(c.opts, c.stdout, report, func(w io.Writer) error {
		fmt.Fprintf(w, "config:   %s (%s)\n", status.ConfigPath, presentOr(status.ConfigExists))
		fmt.Fprintf(w, "modelsDb: %s\n", presentOr(status.ModelsDBExists))
		fmt.Fprintf(w, "agentDb:  %s\n", presentOr(status.AgentDBExists))
		if active == nil {
			fmt.Fprintln(w, "active:   (none)")
		} else {
			fmt.Fprintf(w, "active:   %s\n", *active)
		}
		return nil
	})
}

// saveJSON shares the "read a JSON document from --file and hand it to a
// domain writer" shape used by every `… save --file` subcommand.
// saveJSON writes one document from --file and emits the shared receipt. The
// optional detail hook adds service-reported extras (role repairs) to the JSON
// receipt and prints its notes after the human line.
func (c *cli) saveJSON(what string, args []string, help string, write func([]byte) (string, error), detail ...func() (map[string]any, []string)) error {
	fs := c.newFlagSet(what + " save")
	file := fs.String("file", "", help)
	if err := fs.Parse(args); err != nil {
		return c.usage("%v", err)
	}
	if fs.NArg() != 0 {
		return c.usage("%s save takes no positional arguments", what)
	}
	if strings.TrimSpace(*file) == "" {
		return c.usage("%s save needs --file <json>", what)
	}
	raw, err := os.ReadFile(filepath.Clean(*file)) // #nosec G304 -- user-supplied document
	if err != nil {
		return fmt.Errorf("Failed to read file: %w", err)
	}
	name, err := write(raw)
	if err != nil {
		return err
	}
	receipt := map[string]any{"applied": true, "name": name}
	notes := []string{}
	if len(detail) > 0 && detail[0] != nil {
		fields, extraNotes := detail[0]()
		for key, value := range fields {
			receipt[key] = value
		}
		notes = extraNotes
	}
	if len(notes) > 0 {
		receipt["notes"] = notes
	}
	return emit(c.opts, c.stdout, receipt, func(w io.Writer) error {
		if _, err := fmt.Fprintf(w, "saved %s %q\n", what, name); err != nil {
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
	if c.opts.dryRun {
		c.previewedOwn = true
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
