package main

import (
	"embed"
	"log"

	"droidgear-wails/internal/app"

	"github.com/wailsapp/wails/v3/pkg/application"
)

// The frontend bundle (produced by `wails3 task build:frontend`) is embedded
// into the binary and served from the asset server.
//
//go:embed all:frontend/dist
var assets embed.FS

func main() {
	appInstance := application.New(application.Options{
		Name:        "DroidGear Minimal",
		Description: "Minimal GUI-only Dsh / Pi / Hermes / OMP configurator (Wails v3 spike)",
		Services: []application.Service{
			application.NewService(&app.App{}),
		},
		Assets: application.AssetOptions{
			Handler: application.AssetFileServerFS(assets),
		},
		Mac: application.MacOptions{
			ApplicationShouldTerminateAfterLastWindowClosed: true,
		},
	})

	appInstance.Window.NewWithOptions(application.WebviewWindowOptions{
		Title: "DroidGear Minimal",
		// "main" lets Go commands (ResetWindowState) address this window by
		// name through the WindowManager.
		Name: "main",
		// The window is centred by default (WindowCentered is the zero
		// value); on mount the frontend shrinks it to fit smaller work
		// areas (see src/wails/AppShell.tsx).
		Width:     1280,
		Height:    840,
		MinWidth:  800,
		MinHeight: 600,
		// Frameless: the shared TitleBar renders the window controls. Drag
		// and edge-resize come from the Wails runtime's `--wails-draggable:
		// drag` CSS hook (see src/wails/wails.css). UseApplicationMenu makes
		// Windows/Linux inherit the native menu built by BuildAppMenu
		// (internal/app/menu.go); the frontend installs it once the language
		// is known (src/wails/menu-bridge.ts).
		Frameless:          true,
		UseApplicationMenu: true,
		BackgroundColour:   application.NewRGB(6, 7, 15),
		URL:                "/",
	})

	if err := appInstance.Run(); err != nil {
		log.Fatal(err)
	}
}
