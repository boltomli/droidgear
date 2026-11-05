package main

import (
	"flag"
	"fmt"
	"io"

	"droidgear-wails/internal/service/connectivity"
)

// connectivity mirrors Preferences → Models: probe a BYOK model (or every
// configured model) from the shell. Everything here is a read-only network
// call — no --dry-run semantics apply, failures still map to the exit-code
// table (provider rejections surface through the diagnostics payload).
func (c *cli) connectivity(args []string) error {
	if len(args) == 0 {
		return c.usage("connectivity needs a subcommand: test | all")
	}
	switch args[0] {
	case "test":
		return c.connectivityTest(args[1:])
	case "all":
		return c.connectivityAll(args[1:])
	default:
		return c.usage("unknown connectivity subcommand %q (want test | all)", args[0])
	}
}

// connectivityFlags registers the shared --mode/--prompt pair.
func (c *cli) connectivityFlags(name string) (*flag.FlagSet, *string, *string) {
	fs := c.newFlagSet(name)
	mode := fs.String("mode", "ping", "ping | inference")
	prompt := fs.String("prompt", "", "inference prompt (inference mode only)")
	return fs, mode, prompt
}

// connectivityPromptArg maps the empty flag to the service's nullable arg.
func connectivityPromptArg(prompt *string) *string {
	if *prompt == "" {
		return nil
	}
	return prompt
}

func (c *cli) connectivityTest(args []string) error {
	fs, mode, prompt := c.connectivityFlags("connectivity test")
	rest, err := parseMixed(fs, args)
	if err != nil {
		return c.usage("%v", err)
	}
	if len(rest) != 1 {
		return c.usage("connectivity test <modelId> [--mode ping|inference] [--prompt <text>]")
	}
	result, err := connectivity.TestModelConnectionWithMode(rest[0], *mode, connectivityPromptArg(prompt))
	if err != nil {
		return err
	}
	return emit(c.opts, c.stdout, result, func(w io.Writer) error {
		state := "unavailable"
		if result.IsAvailable {
			state = "available"
		}
		fmt.Fprintf(w, "%s\t%s\t%s\t%dms\n",
			result.ModelID, result.ModelName, state, result.Diagnostics.LatencyMS)
		if result.Diagnostics.Error != nil {
			fmt.Fprintf(w, "error: %s\n", *result.Diagnostics.Error)
		}
		return nil
	})
}

func (c *cli) connectivityAll(args []string) error {
	fs, mode, prompt := c.connectivityFlags("connectivity all")
	rest, err := parseMixed(fs, args)
	if err != nil {
		return c.usage("%v", err)
	}
	if len(rest) != 0 {
		return c.usage("connectivity all [--mode ping|inference] [--prompt <text>]")
	}
	results, err := connectivity.TestAllModelConnectionsWithMode(*mode, connectivityPromptArg(prompt))
	if err != nil {
		return err
	}
	return emit(c.opts, c.stdout, results, func(w io.Writer) error {
		if len(results) == 0 {
			_, err := fmt.Fprintln(w, "no configured models")
			return err
		}
		for _, result := range results {
			state := "unavailable"
			if result.IsAvailable {
				state = "available"
			}
			fmt.Fprintf(w, "%s\t%s\t%s\n", result.ModelID, result.ModelName, state)
		}
		return nil
	})
}
