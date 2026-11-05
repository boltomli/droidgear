package tui

import (
	"fmt"
	"path/filepath"
	"strings"

	"droidgear-wails/internal/core"
	"github.com/charmbracelet/bubbletea"
)

// writeAction is one previewable write: a title for the header and the real
// service call (never a second implementation — tui-parity §5 预览与实际写盘同源).
type writeAction struct {
	title string
	run   func() error
}

// writeFlow implements the P2 contract (tui-parity §2): Plan → Preview →
// Confirm → Apply.
//
//   - Preview runs the real code path under core.BeginDryRun, so what the
//     diff shows is byte-what a write would produce.
//   - Confirm dry-runs the action a second time and compares beforeHash/after
//     per file: if anything moved while the user read the diff (another
//     instance, an editor, the GUI), it stops with a conflict prompt —
//     optimistic concurrency per design doc §5.2.
//   - 'y' on a conflict is the `--force` equivalent: proceed anyway; the
//     write still goes through the normal atomic path.
type writeFlow struct {
	back Screen

	action writeAction
	// reload re-reads the back screen after a successful apply (optional —
	// screens that show the written state implement Refresh).
	reload func()

	preview  *core.ChangeSet
	selected int
	offset   int
	reveal   bool

	conflicts []string
	applied   bool
	err       error
	status    string
}

// newWriteFlow previews action immediately (local file IO, like every other
// screen open).
func newWriteFlow(back Screen, action writeAction, reload func()) *writeFlow {
	f := &writeFlow{back: back, action: action, reload: reload}
	cs, err := dryRun(action)
	if err != nil {
		f.err = err
		f.status = fmt.Sprintf("preview failed: %v · esc back", err)
	} else {
		f.preview = cs
		f.status = "preview · " + action.title + " · y apply · n cancel · ↑↓ select · r secrets"
	}
	return f
}

// dryRun executes action with the recorder installed and yields what it
// would have written.
func dryRun(action writeAction) (*core.ChangeSet, error) {
	end := core.BeginDryRun()
	err := action.run()
	cs := end()
	if err != nil {
		return nil, err
	}
	return cs, nil
}

// confirm re-plans the action and compares it with the previewed plan.
// Mismatched beforeHash/after/paths mean the inputs changed on disk since
// the preview: report the conflict instead of writing.
func (f *writeFlow) confirm(force bool) {
	if !force && f.preview != nil {
		second, err := dryRun(f.action)
		if err != nil {
			f.err = err
			f.status = fmt.Sprintf("plan failed: %v · esc back", err)
			return
		}
		if conflicts := diffPlans(f.preview, second); len(conflicts) > 0 {
			f.conflicts = conflicts
			f.status = fmt.Sprintf("conflict: %d file(s) changed since the preview · y force · n cancel", len(conflicts))
			return
		}
	}
	if err := f.action.run(); err != nil {
		f.err = err
		f.status = fmt.Sprintf("apply failed: %v · esc back", err)
		return
	}
	f.applied = true
	f.conflicts = nil
	count := 0
	if f.preview != nil {
		count = len(f.preview.Changes)
	}
	f.status = fmt.Sprintf("applied · %d file(s) · enter back", count)
	if f.reload != nil {
		f.reload()
	}
}

func changePathLabel(change core.Change) string {
	switch {
	case change.Removed:
		return "removed"
	case change.BeforeHash == "":
		return "new file"
	default:
		return "rewritten"
	}
}

// diffPlans returns the paths whose plan changed between preview and confirm
// (new/removed targets count as changed too).
func diffPlans(previewed, second *core.ChangeSet) []string {
	before := map[string]core.Change{}
	for _, change := range previewed.Changes {
		before[change.Path] = change
	}
	seen := map[string]bool{}
	var conflicts []string
	add := func(path string) {
		if !seen[path] {
			seen[path] = true
			conflicts = append(conflicts, path)
		}
	}
	for _, change := range second.Changes {
		old, ok := before[change.Path]
		if !ok || old.BeforeHash != change.BeforeHash || old.After != change.After {
			add(change.Path)
		}
	}
	for _, change := range previewed.Changes {
		found := false
		for _, next := range second.Changes {
			if next.Path == change.Path {
				found = true
				break
			}
		}
		if !found {
			add(change.Path)
		}
	}
	return conflicts
}

// IsKeySink marks the flow as the key owner: it consumes esc/q itself so it
// can hand control back to the screen it was opened from (tui.go).
func (f *writeFlow) IsKeySink() bool { return true }

func (f *writeFlow) Title() string { return "Preview" }

func (f *writeFlow) Status() string { return f.status }

func (f *writeFlow) Update(msg tea.Msg) (Screen, tea.Cmd) {
	key, ok := msg.(tea.KeyMsg)
	if !ok {
		return f, nil
	}
	name := key.String()

	// Terminal states: any of the usual keys returns to the origin screen.
	if f.err != nil && f.preview == nil {
		switch name {
		case "esc", "q", "enter", "ctrl+c":
			return f.back, nil
		}
		return f, nil
	}
	if f.applied {
		switch name {
		case "esc", "q", "enter", "y":
			return f.back, nil
		}
		return f, nil
	}

	if len(f.conflicts) > 0 {
		switch name {
		case "y":
			f.confirm(true)
		case "n", "esc", "q":
			return f.back, nil
		}
		return f, nil
	}

	switch name {
	case "y":
		f.confirm(false)
	case "n", "esc", "q":
		return f.back, nil
	case "up", "k":
		if f.selected > 0 {
			f.selected--
		}
	case "down", "j":
		if f.selected < len(f.preview.Changes)-1 {
			f.selected++
		}
	case "r":
		f.reveal = !f.reveal
	}
	return f, nil
}

func (f *writeFlow) View(width, height int) string {
	var b strings.Builder

	switch {
	case f.applied:
		b.WriteString(titleStyle.Render("Applied: " + f.action.title))
		b.WriteString("\n\n")
		b.WriteString(dimStyle.Render(fmt.Sprintf("%d file(s) written · enter back", len(f.preview.Changes))))
		return b.String()
	case f.err != nil && f.preview == nil:
		return errStyle.Render(f.action.title + ": " + f.err.Error())
	case len(f.conflicts) > 0:
		b.WriteString(errStyle.Render("Conflict: files changed since the preview"))
		b.WriteString("\n\n")
		for _, path := range f.conflicts {
			b.WriteString(truncate("  "+path, width) + "\n")
		}
		b.WriteString("\n" + dimStyle.Render("y force write · n cancel"))
		return b.String()
	}

	header := "Preview: " + f.action.title
	if f.reveal {
		header += " (secrets visible)"
	}
	b.WriteString(titleStyle.Render(header))
	b.WriteString("\n\n")

	if len(f.preview.Changes) == 0 {
		b.WriteString(dimStyle.Render("no file changes recorded — y applies anyway"))
		return b.String()
	}

	// Change list on the left, the selected document on the right.
	listWidth := sidebarWidth
	if width <= listWidth+20 {
		listWidth = width / 2
	}
	detailWidth := width - listWidth - 1
	if detailWidth < 20 {
		detailWidth = 20
	}
	visible := height - 3
	if visible < 1 {
		visible = 1
	}
	if f.selected < f.offset {
		f.offset = f.selected
	}
	if f.selected >= f.offset+visible {
		f.offset = f.selected - visible + 1
	}
	end := f.offset + visible
	if end > len(f.preview.Changes) {
		end = len(f.preview.Changes)
	}

	var left strings.Builder
	for i := f.offset; i < end; i++ {
		change := f.preview.Changes[i]
		mark := "~"
		switch {
		case change.Removed:
			mark = "-"
		case change.BeforeHash == "":
			mark = "+"
		}
		// The list column is narrow: show the file name (the part users
		// recognise) and put the full path on the detail header.
		line := mark + " " + filepath.Base(change.Path)
		if i == f.selected {
			left.WriteString(selectedStyle.Render(truncate(line, listWidth)))
		} else {
			left.WriteString(truncate(line, listWidth))
		}
		left.WriteString("\n")
	}

	change := f.preview.Changes[f.selected]
	detail := titleStyle.Render(change.Kind) + "\n\n"
	if change.Removed {
		detail += dimStyle.Render("<file removed>")
	} else {
		body := change.After
		if !f.reveal {
			body = core.RedactSensitiveText(body)
		}
		detail += strings.Join(strings.Split(body, "\n"), "\n")
	}

	leftLines := strings.Split(strings.TrimRight(left.String(), "\n"), "\n")
	detailLines := strings.Split(detail, "\n")
	rows := make([]string, 0, height)
	for i := 0; i < height; i++ {
		l, r := "", ""
		if i < len(leftLines) {
			l = leftLines[i]
		}
		if i < len(detailLines) {
			r = truncate(detailLines[i], detailWidth)
		}
		rows = append(rows, pad(l, listWidth)+"│"+r)
	}
	return strings.Join(rows, "\n")
}
