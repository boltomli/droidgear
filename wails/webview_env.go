package main

import (
	"log/slog"
	"os"
	"strings"

	"github.com/wailsapp/wails/v3/pkg/application"
)

// Escape hatches for the environments where WebView2 creates a window but
// renders nothing — ARM VMs without GPU acceleration (Parallels, UTM, VMware),
// RDP sessions, machines with a broken display driver. Both are read once, at
// startup, before WebView2 is initialised.
const (
	// WebviewArgsEnv passes extra Chromium switches, e.g.
	// `DROIDGEAR_WEBVIEW_ARGS="--disable-gpu --disable-gpu-compositing"`.
	// WebView2's own WEBVIEW2_ADDITIONAL_BROWSER_ARGUMENTS covers the same
	// ground without a rebuild; this one is easier to keep next to the app.
	WebviewArgsEnv = "DROIDGEAR_WEBVIEW_ARGS"
	// WebviewVisualHostingEnv forces IDCompositionVisual hosting instead of
	// windowed (HWND-child) hosting, which removes the DPI re-marshal stalls
	// WebView2 hits over remote/virtual displays.
	WebviewVisualHostingEnv = "DROIDGEAR_WEBVIEW_VISUAL_HOSTING"
)

// windowsOptions maps those environment variables onto Wails' WebView2
// options. Non-Windows builds inherit the struct but never read it.
func windowsOptions(logger *slog.Logger) application.WindowsOptions {
	options := application.WindowsOptions{}
	if args := strings.Fields(os.Getenv(WebviewArgsEnv)); len(args) > 0 {
		options.AdditionalBrowserArgs = args
		logger.Warn("WebView2 additional browser args from env", "env", WebviewArgsEnv, "args", args)
	}
	if envEnabled(os.Getenv(WebviewVisualHostingEnv)) {
		options.UseVisualHosting = true
		logger.Warn("WebView2 visual hosting forced on", "env", WebviewVisualHostingEnv)
	}
	return options
}

// envEnabled treats 1/true/yes/on (any case) as true.
func envEnabled(value string) bool {
	switch strings.ToLower(strings.TrimSpace(value)) {
	case "1", "true", "yes", "on":
		return true
	default:
		return false
	}
}
