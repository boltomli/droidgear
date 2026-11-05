package main

import (
	"encoding/json"
	"fmt"
	"io"

	"droidgear-wails/internal/core"
	"droidgear-wails/internal/service/read"
)

// sessions mirrors the Sessions viewers (generic Factory/Droid store): a
// read-only T2 surface — projects | list [project] | show <path>. The
// transcript may quote credentials, so output is redacted unless --reveal.
func (c *cli) sessions(args []string) error {
	if len(args) == 0 {
		return c.usage("sessions needs a subcommand: projects | list | show")
	}
	switch args[0] {
	case "projects":
		return c.sessionsProjects(args[1:])
	case "list":
		return c.sessionsList(args[1:])
	case "show":
		return c.sessionsShow(args[1:])
	default:
		return c.usage("unknown sessions subcommand %q (want projects | list | show)", args[0])
	}
}

func (c *cli) sessionsProjects(args []string) error {
	if len(args) != 0 {
		return c.usage("sessions projects takes no arguments")
	}
	projects, err := read.ListSessionProjects()
	if err != nil {
		return err
	}
	return emit(c.opts, c.stdout, projects, func(w io.Writer) error {
		if len(projects) == 0 {
			_, err := fmt.Fprintln(w, "no session projects")
			return err
		}
		for _, project := range projects {
			fmt.Fprintf(w, "%-32s %d session(s)\n", project.Name, project.SessionCount)
		}
		return nil
	})
}

func (c *cli) sessionsList(args []string) error {
	if len(args) > 1 {
		return c.usage("sessions list [project]")
	}
	var project *string
	if len(args) == 1 {
		project = &args[0]
	}
	sessions, err := read.ListSessions(project)
	if err != nil {
		return err
	}
	return emit(c.opts, c.stdout, sessions, func(w io.Writer) error {
		if len(sessions) == 0 {
			_, err := fmt.Fprintln(w, "no sessions")
			return err
		}
		for _, session := range sessions {
			fmt.Fprintf(w, "%-24s %-28s %-16s %s\n",
				session.ID, truncateCLI(session.Title, 28), session.Project, session.Path)
		}
		return nil
	})
}

func (c *cli) sessionsShow(args []string) error {
	if len(args) != 1 {
		return c.usage("sessions show <path>")
	}
	detail, err := read.GetSessionDetail(args[0])
	if err != nil {
		return err
	}
	document, err := json.MarshalIndent(detail, "", "  ")
	if err != nil {
		return err
	}
	if !c.opts.reveal {
		document = []byte(core.RedactSensitiveText(string(document)))
	}
	return emit(c.opts, c.stdout, json.RawMessage(document), func(w io.Writer) error {
		_, err := fmt.Fprintln(w, string(document))
		return err
	})
}

// truncateCLI shortens a title for the fixed-width human table.
func truncateCLI(value string, max int) string {
	runes := []rune(value)
	if len(runes) <= max {
		return value
	}
	return string(runes[:max-1]) + "…"
}
