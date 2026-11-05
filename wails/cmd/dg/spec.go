package main

import (
	"fmt"
	"io"

	"droidgear-wails/internal/core"
	"droidgear-wails/internal/service/specs"
)

// spec mirrors Droid → Specs: a read-first surface (list | cat) plus delete,
// which honours --dry-run like every other write command. Editing stays on
// the `$EDITOR` manual path (design §4.2).
func (c *cli) spec(args []string) error {
	if len(args) == 0 {
		return c.usage("spec needs a subcommand: list | cat | delete")
	}
	switch args[0] {
	case "list":
		return c.specList(args[1:])
	case "cat":
		return c.specCat(args[1:])
	case "delete":
		return c.specDelete(args[1:])
	default:
		return c.usage("unknown spec subcommand %q (want list | cat | delete)", args[0])
	}
}

func (c *cli) specList(args []string) error {
	if len(args) != 0 {
		return c.usage("spec list takes no arguments")
	}
	files, err := specs.List()
	if err != nil {
		return err
	}
	return emit(c.opts, c.stdout, files, func(w io.Writer) error {
		if len(files) == 0 {
			_, err := fmt.Fprintln(w, "no specs")
			return err
		}
		for _, spec := range files {
			fmt.Fprintf(w, "%-32s %8d bytes  %s\n", spec.Name, len(spec.Content), spec.Path)
		}
		return nil
	})
}

// requireSpec resolves <name> against the managed list by file name first,
// then by full path (both forms the TUI rows expose).
func requireSpec(name string) (specs.SpecFile, error) {
	files, err := specs.List()
	if err != nil {
		return specs.SpecFile{}, err
	}
	for _, spec := range files {
		if spec.Name == name || spec.Path == name {
			return spec, nil
		}
	}
	return specs.SpecFile{}, fmt.Errorf("%w: spec %q", errNotFound, name)
}

func (c *cli) specCat(args []string) error {
	if len(args) != 1 {
		return c.usage("spec cat <name>")
	}
	spec, err := requireSpec(args[0])
	if err != nil {
		return err
	}
	content := spec.Content
	if !c.opts.reveal {
		content = core.RedactSensitiveText(content)
	}
	return emit(c.opts, c.stdout, map[string]any{
		"name": spec.Name, "path": spec.Path, "content": content,
	}, func(w io.Writer) error {
		_, err := fmt.Fprintln(w, content)
		return err
	})
}

func (c *cli) specDelete(args []string) error {
	if len(args) != 1 {
		return c.usage("spec delete <name>")
	}
	spec, err := requireSpec(args[0])
	if err != nil {
		return err
	}
	if err := specs.Delete(spec.Path); err != nil {
		return err
	}
	return emit(c.opts, c.stdout, map[string]any{"deleted": spec.Name}, func(w io.Writer) error {
		_, err := fmt.Fprintf(w, "deleted spec %s\n", spec.Name)
		return err
	})
}
