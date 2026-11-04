package main

import (
	"encoding/json"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"strings"

	"droidgear-wails/internal/core"
	"droidgear-wails/internal/service/factory"
)

// factory mirrors the Factory settings surface of the GUI (Droid → Models):
// custom BYOK models, model favorites and session defaults. Writes honour
// --dry-run through core.BeginDryRun.
func (c *cli) factory(args []string) error {
	if len(args) == 0 {
		return c.usage("factory needs a subcommand: path | reset | models | favorites | defaults")
	}
	switch args[0] {
	case "path":
		return c.factoryPath(args[1:])
	case "reset":
		return c.factoryReset(args[1:])
	case "models":
		return c.factoryModels(args[1:])
	case "favorites":
		return c.factoryFavorites(args[1:])
	case "defaults":
		return c.factoryDefaults(args[1:])
	default:
		return c.usage("unknown factory subcommand %q (want path | reset | models | favorites | defaults)", args[0])
	}
}

func (c *cli) factoryPath(args []string) error {
	if len(args) != 0 {
		return c.usage("factory path takes no arguments")
	}
	path, err := factory.GetConfigPath()
	if err != nil {
		return err
	}
	return emit(c.opts, c.stdout, map[string]string{"path": path}, func(w io.Writer) error {
		_, err := fmt.Fprintln(w, path)
		return err
	})
}

func (c *cli) factoryReset(args []string) error {
	if len(args) != 0 {
		return c.usage("factory reset takes no arguments")
	}
	if err := factory.ResetConfigFile(); err != nil {
		return err
	}
	return emit(c.opts, c.stdout, map[string]any{"applied": true}, func(w io.Writer) error {
		_, err := fmt.Fprintln(w, "factory settings reset")
		return err
	})
}

func (c *cli) factoryModels(args []string) error {
	if len(args) == 0 {
		return c.usage("factory models needs a subcommand: list | save")
	}
	switch args[0] {
	case "list":
		if len(args) != 1 {
			return c.usage("factory models list takes no arguments")
		}
		models, err := factory.LoadCustomModels()
		if err != nil {
			return err
		}
		body, err := json.MarshalIndent(models, "", "  ")
		if err != nil {
			return err
		}
		document := string(body)
		if !c.opts.reveal {
			// Every custom model carries its own apiKey (parity §5.3).
			document = core.RedactSensitiveText(document)
		}
		return emit(c.opts, c.stdout, json.RawMessage(document), func(w io.Writer) error {
			if len(models) == 0 {
				_, err := fmt.Fprintln(w, "no custom models")
				return err
			}
			for _, model := range models {
				fmt.Fprintf(w, "%-32s %s\n", model.Model, model.BaseURL)
			}
			return nil
		})
	case "save":
		return c.factoryModelsSave(args[1:])
	default:
		return c.usage("unknown factory models subcommand %q (want list | save)", args[0])
	}
}

func (c *cli) factoryModelsSave(args []string) error {
	fs := c.newFlagSet("factory models save")
	file := fs.String("file", "", "path to a custom-model array (JSON, same shape as the GUI)")
	if err := fs.Parse(args); err != nil {
		return c.usage("%v", err)
	}
	if fs.NArg() != 0 {
		return c.usage("factory models save takes no positional arguments")
	}
	if strings.TrimSpace(*file) == "" {
		return c.usage("factory models save needs --file <json>")
	}
	models, err := readModelFile(*file)
	if err != nil {
		return err
	}
	if err := factory.SaveCustomModels(models); err != nil {
		return err
	}
	return emit(c.opts, c.stdout,
		map[string]any{"applied": true, "count": len(models)},
		func(w io.Writer) error {
			_, err := fmt.Fprintf(w, "saved %d custom models\n", len(models))
			return err
		})
}

func (c *cli) factoryFavorites(args []string) error {
	if len(args) == 0 {
		return c.usage("factory favorites needs a subcommand: list | save")
	}
	switch args[0] {
	case "list":
		if len(args) != 1 {
			return c.usage("factory favorites list takes no arguments")
		}
		favorites, err := factory.GetModelFavorites()
		if err != nil {
			return err
		}
		return emit(c.opts, c.stdout, favorites, func(w io.Writer) error {
			if len(favorites) == 0 {
				_, err := fmt.Fprintln(w, "no favorite models")
				return err
			}
			for _, id := range favorites {
				fmt.Fprintln(w, id)
			}
			return nil
		})
	case "save":
		fs := c.newFlagSet("factory favorites save")
		file := fs.String("file", "", "path to a JSON array of model ids")
		if err := fs.Parse(args[1:]); err != nil {
			return c.usage("%v", err)
		}
		if strings.TrimSpace(*file) == "" {
			return c.usage("factory favorites save needs --file <json>")
		}
		raw, err := os.ReadFile(filepath.Clean(*file)) // #nosec G304 -- user-supplied file
		if err != nil {
			return fmt.Errorf("Failed to read favorites file: %w", err)
		}
		var favorites []string
		if err := json.Unmarshal(raw, &favorites); err != nil {
			return c.usage("invalid favorites JSON: %v", err)
		}
		if err := factory.SaveModelFavorites(favorites); err != nil {
			return err
		}
		return emit(c.opts, c.stdout,
			map[string]any{"applied": true, "count": len(favorites)},
			func(w io.Writer) error {
				_, err := fmt.Fprintf(w, "saved %d favorites\n", len(favorites))
				return err
			})
	default:
		return c.usage("unknown factory favorites subcommand %q (want list | save)", args[0])
	}
}

func (c *cli) factoryDefaults(args []string) error {
	if len(args) == 0 {
		return c.usage("factory defaults needs a subcommand: show | save")
	}
	switch args[0] {
	case "show":
		if len(args) != 1 {
			return c.usage("factory defaults show takes no arguments")
		}
		settings, err := factory.GetSessionDefaultSettings()
		if err != nil {
			return err
		}
		body, err := json.MarshalIndent(settings, "", "  ")
		if err != nil {
			return err
		}
		document := string(body)
		if !c.opts.reveal {
			document = core.RedactSensitiveText(document)
		}
		return emit(c.opts, c.stdout, json.RawMessage(document), func(w io.Writer) error {
			_, err := fmt.Fprintln(w, document)
			return err
		})
	case "save":
		fs := c.newFlagSet("factory defaults save")
		file := fs.String("file", "", "path to a sessionDefaultSettings document (JSON)")
		if err := fs.Parse(args[1:]); err != nil {
			return c.usage("%v", err)
		}
		if strings.TrimSpace(*file) == "" {
			return c.usage("factory defaults save needs --file <json>")
		}
		raw, err := os.ReadFile(filepath.Clean(*file)) // #nosec G304 -- user-supplied file
		if err != nil {
			return fmt.Errorf("Failed to read defaults file: %w", err)
		}
		var settings factory.SessionDefaultSettings
		if err := json.Unmarshal(raw, &settings); err != nil {
			return c.usage("invalid defaults JSON: %v", err)
		}
		if err := factory.SaveSessionDefaultSettings(settings); err != nil {
			return err
		}
		return emit(c.opts, c.stdout, map[string]any{"applied": true},
			func(w io.Writer) error {
				_, err := fmt.Fprintln(w, "session defaults saved")
				return err
			})
	default:
		return c.usage("unknown factory defaults subcommand %q (want show | save)", args[0])
	}
}

// readModelFile parses the custom-model array the GUI saves. It round-trips
// through the same struct the GUI writes with, so both sides produce the
// identical document.
func readModelFile(path string) ([]factory.CustomModel, error) {
	raw, err := os.ReadFile(filepath.Clean(path)) // #nosec G304 -- user-supplied file
	if err != nil {
		return nil, fmt.Errorf("Failed to read models file: %w", err)
	}
	var models []factory.CustomModel
	if err := json.Unmarshal(raw, &models); err != nil {
		return nil, fmt.Errorf("invalid models JSON: %w", err)
	}
	return models, nil
}
