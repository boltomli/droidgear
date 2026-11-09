package main

import (
	"io"
	"log/slog"
	"testing"
)

func TestEnvEnabled(t *testing.T) {
	for _, value := range []string{"1", "true", "TRUE", "yes", "on", " On "} {
		if !envEnabled(value) {
			t.Errorf("envEnabled(%q) = false, want true", value)
		}
	}
	for _, value := range []string{"", "0", "false", "off", "no", "2"} {
		if envEnabled(value) {
			t.Errorf("envEnabled(%q) = true, want false", value)
		}
	}
}

func TestWindowsOptionsReadsEscapeHatches(t *testing.T) {
	logger := slog.New(slog.NewTextHandler(io.Discard, nil))

	t.Setenv(WebviewArgsEnv, " --disable-gpu   --disable-gpu-compositing ")
	t.Setenv(WebviewVisualHostingEnv, "1")
	options := windowsOptions(logger)
	if len(options.AdditionalBrowserArgs) != 2 ||
		options.AdditionalBrowserArgs[0] != "--disable-gpu" ||
		options.AdditionalBrowserArgs[1] != "--disable-gpu-compositing" {
		t.Fatalf("AdditionalBrowserArgs = %#v", options.AdditionalBrowserArgs)
	}
	if !options.UseVisualHosting {
		t.Fatal("UseVisualHosting should be on")
	}

	// Unset: neither WebView2 knob is touched (Wails keeps its defaults).
	t.Setenv(WebviewArgsEnv, "")
	t.Setenv(WebviewVisualHostingEnv, "")
	options = windowsOptions(logger)
	if options.AdditionalBrowserArgs != nil || options.UseVisualHosting {
		t.Fatalf("options = %#v, want zero value", options)
	}
}
