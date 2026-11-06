package main

import (
	"embed"
	"log"
	"os"

	"droidgear-wails/internal/app"

	"github.com/wailsapp/wails/v3/pkg/application"
	"github.com/wailsapp/wails/v3/pkg/services/notifications"
)

// The frontend bundle (produced by `wails3 task build:frontend`) is embedded
// into the binary and served from the asset server.
//
//go:embed all:frontend/dist
var assets embed.FS

func main() {
	// A binary installed by a previous self-update arrives with
	// `--apply-update-relaunch <pid>`: wait for the old process (which still
	// holds the single-instance lock) to exit and clear its leftovers. This
	// has to run before application.New.
	app.HandleUpdateRelaunch(os.Args[1:])
	// Background housekeeping: previous `.old` executable and staged downloads
	// that are no longer pending (internal/service/updater).
	go app.StartupMaintenance()

	// Cross-platform notifications (SendNativeNotification forwards here).
	notificationService := notifications.New()
	app.SetNotificationService(notificationService)

	appInstance := application.New(application.Options{
		Name:        "DroidGear Minimal",
		Description: "Minimal GUI-only Dsh / Pi / Hermes / OMP configurator (Wails v3 spike)",
		Services: []application.Service{
			application.NewService(&app.App{}),
			application.NewService(notificationService),
		},
		Assets: application.AssetOptions{
			Handler: application.AssetFileServerFS(assets),
		},
		Mac: application.MacOptions{
			ApplicationShouldTerminateAfterLastWindowClosed: true,
		},
		// Single instance: a second launch focuses the existing window
		// instead of writing the same ~/.droidgear config files from two
		// processes (parity with tauri-plugin-single-instance; the shells
		// use different lock identities so both can still run side by side).
		SingleInstance: &application.SingleInstanceOptions{
			UniqueID: "com.droidgear.wailsminimal", // matches build/config.yml productIdentifier
			OnSecondInstanceLaunch: func(application.SecondInstanceData) {
				if running := application.Get(); running != nil {
					if win, ok := running.Window.Get("main"); ok {
						win.Show()
						win.Focus()
					}
				}
			},
		},
		// Stop watchers / kill PTY child trees / flush window geometry
		// before the process exits (internal/app/shutdown.go).
		OnShutdown: app.CleanupOnShutdown,
	})

	// Restore the previous geometry (~/.droidgear/window-state.json) when
	// available; otherwise the window contract applies: 1150×700 default =
	// minimum (mirroring src-tauri/src/lib.rs), centred, created hidden and
	// revealed by the frontend once the shell has mounted (Show + Focus in
	// src/wails/AppShell.tsx). The contract table lives in wails/PORTING.md.
	state := app.LoadWindowState()
	windowOptions := application.WebviewWindowOptions{
		Title: "DroidGear Minimal",
		// "main" lets Go commands (ResetWindowState) address this window by
		// name through the WindowManager.
		Name: "main",
		// The window is centred by default (WindowCentered is the zero
		// value); on mount the frontend shrinks it to fit smaller work
		// areas (see src/wails/AppShell.tsx).
		Width:     1150,
		Height:    700,
		MinWidth:  1150,
		MinHeight: 700,
		// Frameless: the shared TitleBar renders the window controls. Drag
		// and edge-resize come from the Wails runtime's `--wails-draggable:
		// drag` CSS hook (see src/wails/wails.css). UseApplicationMenu makes
		// Windows/Linux inherit the native menu built by BuildAppMenu
		// (internal/app/menu.go); the frontend installs it once the language
		// is known (src/wails/menu-bridge.ts).
		Frameless:          true,
		UseApplicationMenu: true,
		// Hidden until the frontend calls Show() — no white/black flash at
		// startup, matching Tauri's visible:false → show() sequence.
		Hidden:           true,
		BackgroundColour: startupBackgroundColour(),
		URL:              "/",
	}
	if state != nil {
		windowOptions.X = state.X
		windowOptions.Y = state.Y
		windowOptions.Width = state.Width
		windowOptions.Height = state.Height
		windowOptions.InitialPosition = application.WindowXY
	}
	mainWindow := appInstance.Window.NewWithOptions(windowOptions)
	app.AttachWindowState(mainWindow, state)

	if err := appInstance.Run(); err != nil {
		log.Fatal(err)
	}
}

// startupBackgroundColour picks the native window background from the
// persisted theme preference (light → white, dark/system → #0f0f0f, the
// same value as the hardcoded `html` background in src/App.css) so the
// first frame and window edges do not flash the wrong colour. The frontend
// keeps it in sync on every theme change (src/wails/theme-window.ts).
func startupBackgroundColour() application.RGBA {
	if prefs, err := (&app.App{}).LoadPreferences(); err == nil && prefs.Theme == "light" {
		return application.NewRGB(255, 255, 255)
	}
	return application.NewRGB(15, 15, 15)
}
