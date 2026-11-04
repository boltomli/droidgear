package main

import (
	"encoding/json"
	"flag"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"strings"

	"droidgear-wails/internal/core"
	"droidgear-wails/internal/service/mcp"
)

// mcp mirrors the MCP section of the GUI (list / detail / toggle / save /
// delete / preset). Every write honours --dry-run through core.BeginDryRun,
// the recording hook the other write commands share.
func (c *cli) mcp(args []string) error {
	if len(args) == 0 {
		return c.usage("mcp needs a subcommand: list | show | toggle | save | delete | preset")
	}
	switch args[0] {
	case "list":
		return c.mcpList(args[1:])
	case "show":
		return c.mcpShow(args[1:])
	case "toggle":
		return c.mcpToggle(args[1:])
	case "save":
		return c.mcpSave(args[1:])
	case "delete":
		return c.mcpDelete(args[1:])
	case "preset":
		return c.mcpPreset(args[1:])
	default:
		return c.usage(
			"unknown mcp subcommand %q (want list | show | toggle | save | delete | preset)",
			args[0],
		)
	}
}

func (c *cli) mcpList(args []string) error {
	if len(args) != 0 {
		return c.usage("mcp list takes no arguments")
	}
	servers, err := mcp.LoadServers()
	if err != nil {
		return err
	}
	return emit(c.opts, c.stdout, servers, func(w io.Writer) error {
		if len(servers) == 0 {
			_, err := fmt.Fprintln(w, "no MCP servers")
			return err
		}
		for _, server := range servers {
			state := "enabled"
			if server.Config.Disabled {
				state = "disabled"
			}
			fmt.Fprintf(w, "%-24s %-6s %s\n", server.Name, server.Config.Type, state)
		}
		return nil
	})
}

func (c *cli) mcpShow(args []string) error {
	if len(args) != 1 {
		return c.usage("mcp show <name>")
	}
	servers, err := mcp.LoadServers()
	if err != nil {
		return err
	}
	for _, server := range servers {
		if server.Name != args[0] {
			continue
		}
		document, err := json.MarshalIndent(server, "", "  ")
		if err != nil {
			return err
		}
		body := string(document)
		if !c.opts.reveal {
			// Env values and headers are credentials (parity §5.3).
			body = core.RedactSensitiveText(body)
		}
		return emit(c.opts, c.stdout, json.RawMessage(body), func(w io.Writer) error {
			_, err := fmt.Fprintln(w, body)
			return err
		})
	}
	return fmt.Errorf("MCP server %q not found", args[0])
}

func (c *cli) mcpToggle(args []string) error {
	fs := c.newFlagSet("mcp toggle")
	disabled := fs.Bool("disabled", false, "set the state explicitly instead of flipping it")
	if err := fs.Parse(args); err != nil {
		return c.usage("%v", err)
	}
	if fs.NArg() != 1 {
		return c.usage("mcp toggle <name> [--disabled=<bool>]")
	}
	name := fs.Arg(0)

	servers, err := mcp.LoadServers()
	if err != nil {
		return err
	}
	var target *mcp.McpServer
	for i := range servers {
		if servers[i].Name == name {
			target = &servers[i]
			break
		}
	}
	if target == nil {
		return fmt.Errorf("MCP server %q not found", name)
	}

	// stdlib flag has no Changed(): Visit only walks the flags that were set.
	next := !target.Config.Disabled
	fs.Visit(func(f *flag.Flag) {
		if f.Name == "disabled" {
			next = *disabled
		}
	})
	if err := mcp.ToggleServer(name, next); err != nil {
		return err
	}
	state := "enabled"
	if next {
		state = "disabled"
	}
	return emit(c.opts, c.stdout,
		map[string]any{"applied": true, "name": name, "disabled": next},
		func(w io.Writer) error {
			_, err := fmt.Fprintf(w, "%s is now %s\n", name, state)
			return err
		})
}

func (c *cli) mcpSave(args []string) error {
	fs := c.newFlagSet("mcp save")
	file := fs.String("file", "", "path to a server document (JSON, same shape as the GUI)")
	if err := fs.Parse(args); err != nil {
		return c.usage("%v", err)
	}
	if fs.NArg() != 0 {
		return c.usage("mcp save takes no positional arguments")
	}
	if strings.TrimSpace(*file) == "" {
		return c.usage("mcp save needs --file <json>")
	}

	raw, err := os.ReadFile(filepath.Clean(*file)) // #nosec G304 -- user-supplied file
	if err != nil {
		return fmt.Errorf("Failed to read server file: %w", err)
	}
	var server mcp.McpServer
	if err := json.Unmarshal(raw, &server); err != nil {
		return c.usage("invalid server JSON: %v", err)
	}
	if strings.TrimSpace(server.Name) == "" {
		return c.usage("server needs a non-empty \"name\"")
	}

	if err := mcp.SaveServer(server); err != nil {
		return err
	}
	return emit(c.opts, c.stdout, map[string]any{"applied": true, "name": server.Name},
		func(w io.Writer) error {
			_, err := fmt.Fprintf(w, "saved MCP server %q\n", server.Name)
			return err
		})
}

func (c *cli) mcpDelete(args []string) error {
	if len(args) != 1 {
		return c.usage("mcp delete <name>")
	}
	if err := mcp.DeleteServer(args[0]); err != nil {
		return err
	}
	return emit(c.opts, c.stdout, map[string]any{"applied": true, "name": args[0]},
		func(w io.Writer) error {
			_, err := fmt.Fprintf(w, "deleted MCP server %q\n", args[0])
			return err
		})
}

func (c *cli) mcpPreset(args []string) error {
	if len(args) != 0 {
		return c.usage("mcp preset takes no arguments")
	}
	if err := mcp.SaveBaizhiPreset(); err != nil {
		return err
	}
	return emit(c.opts, c.stdout, map[string]any{"applied": true, "name": "baizhi"},
		func(w io.Writer) error {
			_, err := fmt.Fprintln(w, "baizhi preset stored (disabled)")
			return err
		})
}
