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
		Title:            "DroidGear Minimal",
		Width:            1280,
		Height:           840,
		MinWidth:         960,
		MinHeight:        640,
		BackgroundColour: application.NewRGB(6, 7, 15),
		URL:              "/",
	})

	if err := appInstance.Run(); err != nil {
		log.Fatal(err)
	}
}
