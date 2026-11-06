// Command dg is the headless CLI for DroidGear: every write the GUI can do
// has a `dg` counterpart that a script can run over SSH or inside a
// container, with `--json` as the machine contract
// (docs/developer/wails-tui-cli-design.zh.md §4).
//
// The binary links internal/service + internal/core only — never
// internal/app or the Wails runtime (layering rule R3, enforced by
// scripts/check-go-layering.mjs), so it builds and runs without WebKit or
// Cocoa present.
package main

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"time"

	"droidgear-wails/internal/core"
	"droidgear-wails/internal/service/channels"
)

// Exit codes (design doc §4.4). 0/1/2 are live; 3 (conflict) and 4
// (upstream/network) follow the batches that introduce conflict detection
// and error classification — see the status note in §4.4.
const (
	exitOK       = 0
	exitRuntime  = 1
	exitUsage    = 2
	exitConflict = 3
	exitUpstream = 4
)

// options are the global flags, recognised anywhere on the command line
// (`dg path show --json` and `dg --json path show` both work).
type options struct {
	jsonOut bool
	dryRun  bool
	apply   bool
	home    string
	timeout time.Duration
	reveal  bool
}

func main() {
	os.Exit(run(os.Args[1:], os.Stdout, os.Stderr))
}

// usageError marks a problem in how the command was invoked (exit 2).
type usageError struct{ message string }

func (e usageError) Error() string { return e.message }

// run executes one CLI invocation and returns the process exit code.
// It is separated from main so tests can drive it without forking.
func run(args []string, stdout, stderr io.Writer) (exit int) {
	opts, rest, err := splitGlobalFlags(args)
	if err != nil {
		return fail(stderr, opts, exitUsage, "usage", err.Error(), "")
	}
	if opts.home != "" {
		// Process-wide: internal/core resolves paths through os.UserHomeDir,
		// which reads the environment on every call (design doc §4.3).
		// dg is a short-lived process, so overriding here is enough.
		// Windows derives more roots from the environment — without redirecting
		// them too, %LOCALAPPDATA%/hermes (Hermes priority chain) would point
		// back at the real profile and break the sandbox the flag promises.
		_ = os.Setenv("HOME", opts.home)
		_ = os.Setenv("USERPROFILE", opts.home)
		_ = os.Setenv("APPDATA", filepath.Join(opts.home, "AppData", "Roaming"))
		_ = os.Setenv("LOCALAPPDATA", filepath.Join(opts.home, "AppData", "Local"))
	}

	if len(rest) == 0 {
		// Bare `dg` is a mistake, not a request for help (exit 2), while
		// `dg help` is an explicit request (exit 0).
		printUsage(stderr)
		return exitUsage
	}
	if rest[0] == "help" || rest[0] == "--help" || rest[0] == "-h" {
		printUsage(stdout)
		return exitOK
	}

	cmd, cmdArgs := rest[0], rest[1:]

	// A dry run executes the real code path with every write recorded instead
	// of performed (core.BeginDryRun), so the preview can never drift from
	// what `apply` would actually do. stdout is buffered for the duration: the
	// command's own receipt must not compete with the ChangeSet that answers
	// `--dry-run` (commands with a richer preview of their own — export run —
	// set cli.previewedOwn and keep theirs).
	var (
		endDryRun func() *core.ChangeSet
		buffered  bytes.Buffer
	)
	out := stdout
	if opts.dryRun {
		out = &buffered
		endDryRun = core.BeginDryRun()
	}
	c := &cli{opts: opts, stdout: out, stderr: stderr}
	if endDryRun != nil {
		defer func() {
			cs := endDryRun()
			switch {
			case exit != exitOK:
				// The command failed: its error already went to stderr, and any
				// partial output stays where the caller expects it.
				_, _ = io.Copy(stdout, &buffered)
			case len(cs.Changes) > 0 && !c.previewedOwn:
				// The receipt in the buffer claims the write happened; the
				// ChangeSet is the honest answer, so it replaces it.
				c.stdout = stdout
				exit = c.printChangeSet(cs)
			default:
				_, _ = io.Copy(stdout, &buffered)
			}
		}()
	}

	var runErr error
	switch cmd {
	case "version":
		runErr = c.version(cmdArgs)
	case "doctor":
		runErr = c.doctor(cmdArgs)
	case "path":
		runErr = c.path(cmdArgs)
	case "channels":
		runErr = c.channels(cmdArgs)
	case "codex":
		runErr = c.codex(cmdArgs)
	case "hermes":
		runErr = c.hermes(cmdArgs)
	case "pi":
		runErr = c.pi(cmdArgs)
	case "omp":
		runErr = c.omp(cmdArgs)
	case "export":
		runErr = c.export(cmdArgs)
	case "mcp":
		runErr = c.mcp(cmdArgs)
	case "claude":
		runErr = c.claude(cmdArgs)
	case "copilot":
		runErr = c.copilot(cmdArgs)
	case "droid":
		runErr = c.droid(cmdArgs)
	case "dsh":
		runErr = c.dsh(cmdArgs)
	case "sessions":
		runErr = c.sessions(cmdArgs)
	case "spec":
		runErr = c.spec(cmdArgs)
	case "connectivity":
		runErr = c.connectivity(cmdArgs)
	case "factory":
		runErr = c.factory(cmdArgs)
	case "tui":
		runErr = c.tui(cmdArgs)
	default:
		return fail(stderr, opts, exitUsage, "usage",
			fmt.Sprintf("unknown command %q", cmd), "run `dg help`")
	}

	if runErr == nil {
		return exitOK
	}
	var u usageError
	switch {
	case errors.As(runErr, &u):
		return fail(stderr, opts, exitUsage, "usage", u.Error(), "run `dg help`")
	case errors.Is(runErr, errNotFound):
		return fail(stderr, opts, exitRuntime, "not_found", runErr.Error(), "")
	case errors.Is(runErr, channels.ErrUpstream):
		return fail(stderr, opts, exitUpstream, "upstream", runErr.Error(),
			"provider endpoint unreachable or rejected the request")
	default:
		return fail(stderr, opts, exitRuntime, "runtime", runErr.Error(), "")
	}
}

// errNotFound marks "the target does not exist" (exit 1, code not_found).
var errNotFound = errors.New("not found")

// printChangeSet renders the recorded writes as the answer to --dry-run:
// one JSON document with --json, a `would write/remove` list otherwise.
// Secrets are masked here (unless --reveal) — the preview shows the very
// document that would land on disk, so it must not leak what it previews.
func (c *cli) printChangeSet(cs *core.ChangeSet) int {
	if !c.opts.reveal {
		for i := range cs.Changes {
			if cs.Changes[i].Removed {
				continue
			}
			redacted := core.RedactSensitiveText(cs.Changes[i].After)
			if redacted != cs.Changes[i].After {
				cs.Changes[i].After = redacted
				cs.Changes[i].Redacted = true
			}
		}
	}
	if c.opts.jsonOut {
		if err := writeJSON(c.stdout, cs); err != nil {
			return fail(c.stderr, c.opts, exitRuntime, "runtime", err.Error(), "")
		}
		return exitOK
	}
	for _, change := range cs.Changes {
		if change.Removed {
			fmt.Fprintf(c.stdout, "would remove %s\n", change.Path)
			continue
		}
		redacted := ""
		if change.Redacted {
			redacted = ", redacted"
		}
		fmt.Fprintf(c.stdout, "would write %s (%s, %d bytes%s)\n",
			change.Path, change.Kind, len(change.After), redacted)
	}
	return exitOK
}

// fail writes the error envelope (or a human line) and returns the code.
func fail(w io.Writer, opts options, code int, errCode string, message string, hint string) int {
	if opts.jsonOut {
		_ = writeJSON(w, map[string]any{
			"error": map[string]string{
				"code":    errCode,
				"message": message,
				"hint":    hint,
			},
		})
	} else {
		fmt.Fprintf(w, "error: %s\n", message)
		if hint != "" {
			fmt.Fprintf(w, "hint: %s\n", hint)
		}
	}
	return code
}

// ---------------------------------------------------------------------------
// Global flags
// ---------------------------------------------------------------------------

var globalValueFlags = map[string]bool{"home": true, "timeout": true}
var globalBoolFlags = map[string]bool{
	"json": true, "dry-run": true, "apply": true, "reveal": true,
}

// splitGlobalFlags separates global flags from the command line, preserving
// the order of everything else so the subcommand's own flag.FlagSet can parse
// what is left.
func splitGlobalFlags(args []string) (options, []string, error) {
	var opts options
	var rest []string
	for i := 0; i < len(args); i++ {
		arg := args[i]
		if arg == "--" {
			rest = append(rest, args[i:]...)
			break
		}
		if len(arg) < 2 || arg[0] != '-' {
			rest = append(rest, arg)
			continue
		}
		name := strings.TrimLeft(arg, "-")
		value, hasValue := name, false
		if eq := strings.Index(name, "="); eq >= 0 {
			name, value, hasValue = name[:eq], name[eq+1:], true
		}
		switch {
		case globalBoolFlags[name]:
			if !hasValue {
				value = "true"
			}
			parsed, err := strconv.ParseBool(value)
			if err != nil {
				return opts, nil, usageError{fmt.Sprintf("--%s expects true or false", name)}
			}
			setGlobalBool(&opts, name, parsed)
		case globalValueFlags[name]:
			if !hasValue {
				i++
				if i >= len(args) {
					return opts, nil, usageError{fmt.Sprintf("--%s needs a value", name)}
				}
				value = args[i]
			}
			if err := setGlobalValue(&opts, name, value); err != nil {
				return opts, nil, err
			}
		default:
			// Not ours: hand it to the subcommand untouched.
			rest = append(rest, arg)
		}
		if opts.dryRun && opts.apply {
			return opts, nil, usageError{"--apply and --dry-run are mutually exclusive"}
		}
	}
	return opts, rest, nil
}

func setGlobalBool(opts *options, name string, value bool) {
	switch name {
	case "json":
		opts.jsonOut = value
	case "dry-run":
		opts.dryRun = value
	case "apply":
		opts.apply = value
	case "reveal":
		opts.reveal = value
	}
}

func setGlobalValue(opts *options, name string, value string) error {
	switch name {
	case "home":
		if strings.TrimSpace(value) == "" {
			return usageError{"--home needs a directory"}
		}
		opts.home = value
	case "timeout":
		parsed, err := time.ParseDuration(value)
		if err != nil || parsed < 0 {
			return usageError{fmt.Sprintf("--timeout expects a duration like 30s, got %q", value)}
		}
		opts.timeout = parsed
	}
	return nil
}

// commandContext returns the context for one invocation, honouring --timeout.
// A zero timeout means "no extra deadline": every provider request already
// carries its own client timeout, and an export run may legitimately take
// minutes across many channels.
func (opts options) commandContext() (context.Context, context.CancelFunc) {
	if opts.timeout <= 0 {
		return context.WithCancel(context.Background())
	}
	return context.WithTimeout(context.Background(), opts.timeout)
}

// ---------------------------------------------------------------------------
// Output helpers
// ---------------------------------------------------------------------------

func writeJSON(w io.Writer, value any) error {
	encoder := json.NewEncoder(w)
	encoder.SetIndent("", "  ")
	return encoder.Encode(value)
}

// emit writes the machine document for --json, or the human rendering
// produced by the command itself.
func emit(opts options, stdout io.Writer, machine any, human func(io.Writer) error) error {
	if opts.jsonOut {
		return writeJSON(stdout, machine)
	}
	if human == nil {
		return nil
	}
	return human(stdout)
}

func printUsage(w io.Writer) {
	fmt.Fprint(w, `dg — headless DroidGear CLI

Usage:
  dg [global flags] <command> [args]

Commands:
  version                       print the build version
  doctor                        report effective paths, WSL and config files (read-only)
  path show                     print all nine effective config paths
  path set <key> <path>         override one config path
  path reset <key>              remove one config path override
  channels list                 list configured channels
  channels show <id>            show one channel plus its auth kind (redacted)
  channels tokens <id>          list a channel's tokens (keys redacted unless --reveal)
  codex profiles list           list codex profiles (API keys redacted unless --reveal)
  codex profiles show <id>      show one profile
  codex profiles active         print the active profile id
  codex profiles apply <id>     write a profile into the live ~/.codex config
  codex profiles save --file <json>  create or replace a profile from JSON
  codex profiles delete <id>    delete a profile
  codex status                  report live config/auth paths and the active profile
  hermes profiles list|show|active|apply|save|delete  manage hermes profiles
  hermes providers list|show|delete|import --channel <id> [--token <t>] [--model <m>] [--name <n>]  live ~/.hermes/config.yaml providers
  hermes status                 report the live config path and active profile
  pi profiles list|show|active|apply|save|delete  manage pi profiles
  pi providers list|show|delete            live ~/.pi/agent/models.json providers
  pi providers import --channel <id> [--token <t>] [--models a,b] [--provider <id>]  import a channel as a pi provider
  pi settings show              startup model selection from ~/.pi/agent/settings.json
  pi settings set [--provider <id>] [--model <id>] [--thinking <level>]  pin the startup model
  pi settings reset             clear the startup model (pi picks automatically)
  pi status                     report the live config path and active profile
  omp profiles list|show|active|apply|save|delete  manage omp profiles
  omp providers list|save --file <json>|delete <id>  manage omp custom providers
  omp providers import --channel <id> [--token <t>] [--models a,b] [--provider <id>]  import a channel as an omp provider
  omp status                    report config/DB presence and the active profile
  export list                   list export templates
  export save --file <json>     create or replace a template from a JSON document
  export delete <name>          delete a template
  export run <name>             run a template and write its output file
  mcp list|show <name>|toggle <name>|save --file <json>|delete <name>|preset
  factory path|reset|models list|save --file <json>|favorites list|save|defaults show|save
  claude settings list|show|active|use|delete|save --file <json>
  copilot profiles list|show|active|apply|save --file <json>|delete + status
  droid settings list|use|delete|values | trusted list|add|remove | auth list|save-current|switch|delete|rename | missions show | legacy check|delete
  dsh providers list|save --file <json>|delete [--profile desktop|web] + status
  sessions projects|list [project]|show <path>   read-only session store
  spec list|cat <name>|delete <name>             specs (edit via $EDITOR)
  connectivity test <modelId>|all [--mode ping|inference] [--prompt <text>]
  tui                           open the interactive terminal UI (needs a TTY)

Global flags (accepted before or after the command):
  --json            print one JSON document on stdout (errors go to stderr)
  --dry-run         compute without writing
  --home <dir>      resolve ~ and config paths under <dir> (tests, containers)
  --timeout <dur>  overall deadline for one command (default: none)
  --reveal          include secrets in output (default: redacted)

Exit codes:
  0 success   1 runtime/not-found   2 usage   3 conflict   4 upstream/provider

See docs/developer/wails-tui-cli-design.zh.md for the full contract.
`)
}
