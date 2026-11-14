// Self-update surface for the GUI — the Wails-facing half of
// internal/service/updater.
//
// The service does the work (release discovery, checksum verification, the
// executable swap); this file only adds the two things that are GUI-only by
// design (wails-tui-cli-design §2): progress events on the Wails bus and the
// quit that lets the relaunched binary take over. `dg update` calls the same
// service with no emitter and no restart.
//
// Design: docs/developer/self-update-single-file.zh.md.
package app

import (
	"context"
	"os"
	"time"

	"droidgear-wails/internal/service/updater"

	"github.com/wailsapp/wails/v3/pkg/application"
)

// Type aliases — one definition, in the service, so the generated bindings and
// `dg update --json` share the same fields.
type (
	UpdateCheckResult    = updater.CheckResult
	UpdateDownloadResult = updater.DownloadResult
	UpdateApplyResult    = updater.ApplyResult
	UpdatePending        = updater.PendingUpdate
	UpdateProgress       = updater.Progress
)

// updateProgressEmitter forwards service progress to the frontend
// (`update:progress` events consumed by src/services/updates.ts).
type updateProgressEmitter struct{}

func (updateProgressEmitter) UpdateProgress(progress updater.Progress) {
	if app := application.Get(); app != nil {
		app.Event.Emit("update:progress", progress)
	}
}

// CheckForUpdates asks GitHub for the newest verifiable single-file release on
// the configured channel. A dev build reports DevBuild instead of an offer.
//
// allowDowngrade additionally offers the channel's newest release when it is
// older than the running build — the only way back from a preview build to the
// stable line.
func (a *App) CheckForUpdates(allowDowngrade bool) (UpdateCheckResult, error) {
	ctx, cancel := context.WithTimeout(context.Background(), 60*time.Second)
	defer cancel()
	cfg := updater.DefaultConfig()
	cfg.AllowDowngrade = allowDowngrade
	return updater.Check(ctx, cfg, updater.KindGUI)
}

// DownloadUpdate downloads and verifies the GUI asset, staging it for
// ApplyUpdate. Emits `update:progress` while streaming.
func (a *App) DownloadUpdate(allowDowngrade bool) (UpdateDownloadResult, error) {
	ctx, cancel := context.WithTimeout(context.Background(), 15*time.Minute)
	defer cancel()
	cfg := updater.DefaultConfig()
	cfg.AllowDowngrade = allowDowngrade
	return updater.Download(ctx, cfg, updater.KindGUI, updateProgressEmitter{})
}

// ApplyUpdate swaps the staged binary in and restarts the app. The response is
// returned before the process quits so the frontend can show "restarting"
// instead of a spurious disconnect.
func (a *App) ApplyUpdate(allowDowngrade bool) (UpdateApplyResult, error) {
	cfg := updater.DefaultConfig()
	cfg.AllowDowngrade = allowDowngrade
	result, err := updater.Apply(cfg, updater.KindGUI, true, updateProgressEmitter{})
	if err != nil {
		return result, err
	}
	if result.Restart {
		go func() {
			time.Sleep(500 * time.Millisecond)
			if app := application.Get(); app != nil {
				app.Quit()
			}
		}()
	}
	return result, nil
}

// GetPendingUpdate reports a previously downloaded update (null when none).
func (a *App) GetPendingUpdate() (*UpdatePending, error) {
	return updater.Pending(updater.DefaultConfig()), nil
}

// DismissUpdate remembers that the user does not want the startup banner for
// this version again.
func (a *App) DismissUpdate(version string) error {
	return updater.Dismiss(updater.DefaultConfig(), version)
}

// HandleUpdateRelaunch blocks a freshly installed binary until the process
// that started it is gone, then cleans up the previous executable. It must run
// before application.New: the Wails single-instance lock is held by the old
// process until it exits, and a second instance would hand off and quit.
// Returns true when the invocation carried the internal relaunch flag.
func HandleUpdateRelaunch(args []string) bool {
	pid, ok := updater.RelaunchPID(args)
	if !ok {
		return false
	}
	updater.WaitForExit(pid, 60*time.Second)
	updater.CleanupStale(updater.DefaultConfig())
	// Keep the internal flag out of os.Args: Wails inspects argv (single
	// instance forwarding) and no user-facing option should see it.
	os.Args = append([]string{os.Args[0]}, args[2:]...)
	return true
}

// StartupMaintenance clears leftovers of a previous update (the `.old` backup
// Windows keeps locked, staged downloads that are no longer pending). It runs
// in the background so startup is not delayed.
func StartupMaintenance() {
	updater.CleanupStale(updater.DefaultConfig())
}
