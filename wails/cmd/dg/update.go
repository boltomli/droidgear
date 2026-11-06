// `dg update` — the headless half of the single-file updater
// (internal/service/updater, ADR-001 rule 9: the GUI does not get an
// updater-only path).
//
// It updates the `dg` binary itself: the same verified swap the GUI performs,
// without the restart (the running process keeps the old image; the next
// invocation uses the new one).
package main

import (
	"fmt"
	"io"

	"droidgear-wails/internal/service/updater"
)

// updateReceipt is the write-receipt shape from the CLI contract
// (wails-tui-cli-design §4.4): `applied` plus the service result flattened.
type updateReceipt struct {
	Applied bool `json:"applied"`
	updater.ApplyResult
}

// updatePlan is the `--dry-run`/`--check` document: what would happen, without
// downloading anything.
type updatePlan struct {
	Applied         bool   `json:"applied"`
	CurrentVersion  string `json:"currentVersion"`
	LatestVersion   string `json:"latestVersion"`
	UpdateAvailable bool   `json:"updateAvailable"`
	DevBuild        bool   `json:"devBuild"`
	AssetName       string `json:"assetName,omitempty"`
	AssetSize       int64  `json:"assetSize,omitempty"`
	Executable      string `json:"executable,omitempty"`
	ReleaseURL      string `json:"releaseUrl,omitempty"`
}

func (c *cli) update(args []string) error {
	fs := c.newFlagSet("update")
	checkOnly := fs.Bool("check", false, "report whether a newer build exists, without downloading")
	force := fs.Bool("force", false, "allow updating a source build (version dev)")
	rest, err := parseMixed(fs, args)
	if err != nil {
		return c.usage("%v", err)
	}
	if len(rest) != 0 {
		return c.usage("update takes no arguments (flags: --check, --force)")
	}

	cfg := updater.DefaultConfig()
	displayVersion := cfg.CurrentVersion
	if updater.IsDevBuild(cfg.CurrentVersion) {
		if !*force {
			return updater.ErrDevBuild
		}
		// --force treats a source build as 0.0.0: any release is newer, and the
		// swap replaces the `go build` output in place. The receipt still shows
		// the version the binary actually reports ("dev").
		cfg.CurrentVersion = "0.0.0"
	}

	// A dry run stops after the plan: it must not touch the network beyond the
	// release list, and must not download.
	if *checkOnly || c.opts.dryRun {
		// The plan is this command's own preview: keep it instead of letting the
		// generic ChangeSet substitution replace it (design doc §4.2).
		c.previewedOwn = true
		ctx, cancel := c.opts.commandContext()
		defer cancel()
		plan, err := updater.Check(ctx, cfg, updater.KindCLI)
		if err != nil {
			return err
		}
		document := updatePlan{
			CurrentVersion:  plan.CurrentVersion,
			LatestVersion:   plan.LatestVersion,
			UpdateAvailable: plan.UpdateAvailable,
			DevBuild:        plan.DevBuild,
			AssetName:       plan.AssetName,
			AssetSize:       plan.AssetSize,
			Executable:      cfg.Executable,
			ReleaseURL:      plan.ReleaseURL,
		}
		return emit(c.opts, c.stdout, document, func(w io.Writer) error {
			switch {
			case plan.DevBuild:
				_, err := fmt.Fprintln(w, "development build — no update is offered")
				return err
			case plan.UpdateAvailable:
				_, err := fmt.Fprintf(w, "update available: %s → %s (%s, %s)\n",
					plan.CurrentVersion, plan.LatestVersion, plan.AssetName, humanSize(plan.AssetSize))
				return err
			default:
				_, err := fmt.Fprintf(w, "up to date: %s\n", plan.CurrentVersion)
				return err
			}
		})
	}

	ctx, cancel := c.opts.commandContext()
	defer cancel()
	// Download records the verified binary in state.json; Apply installs
	// exactly that record (and re-verifies it).
	if _, err := updater.Download(ctx, cfg, updater.KindCLI, nil); err != nil {
		return err
	}
	result, err := updater.Apply(cfg, updater.KindCLI, false, nil)
	if err != nil {
		return err
	}
	result.FromVersion = displayVersion

	receipt := updateReceipt{Applied: true, ApplyResult: result}
	return emit(c.opts, c.stdout, receipt, func(w io.Writer) error {
		_, err := fmt.Fprintf(w, "updated dg %s → %s (%s)\nre-run dg to use the new binary\n",
			result.FromVersion, result.ToVersion, result.Executable)
		return err
	})
}

// humanSize renders a byte count for the human (non-JSON) output.
func humanSize(bytes int64) string {
	const unit = 1024
	if bytes < unit {
		return fmt.Sprintf("%d B", bytes)
	}
	value := float64(bytes)
	for _, suffix := range []string{"KiB", "MiB", "GiB"} {
		value /= unit
		if value < unit {
			return fmt.Sprintf("%.1f %s", value, suffix)
		}
	}
	return fmt.Sprintf("%.1f TiB", value/unit)
}
