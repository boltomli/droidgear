package main

import (
	"context"

	"droidgear-wails/internal/tui"
)

// tui starts the interactive terminal UI.
//
// The TUI owns the terminal for its whole lifetime, so the JSON / dry-run
// plumbing that every other command uses does not apply to it — passing
// either flag is a usage error rather than a silently ignored option.
//
// It is the parity track's entry point (docs/developer/tui-parity.zh.md):
// screens read internal/service directly, exactly like the subcommands do.
func (c *cli) tui(args []string) error {
	if len(args) > 0 {
		return usageError{message: "usage: dg tui"}
	}
	if c.opts.jsonOut || c.opts.dryRun {
		return usageError{
			message: "`dg tui` is interactive: --json and --dry-run do not apply",
		}
	}
	return tui.Run(context.Background())
}
