//go:build windows

package main

import (
	"log/slog"
	"runtime"
	"unsafe"

	"github.com/wailsapp/go-webview2/webviewloader"
	"golang.org/x/sys/windows"
)

// WebView2 is the only runtime the Windows build needs, and its absence fails
// quietly here: the window is created hidden and the frontend is supposed to
// reveal it, so a missing runtime (or an x64-only install on an ARM64 host)
// looks exactly like "the app does not start". Report it in a native dialog
// before Wails tries, and keep a line in the log either way.
const webview2DownloadPage = "https://developer.microsoft.com/microsoft-edge/webview2/"

// MessageBox flags (winuser.h).
const (
	msgBoxOK          = 0x00000000
	msgBoxIconError   = 0x00000010
	msgBoxIconWarning = 0x00000030
	msgBoxSystemModal = 0x00001000
)

var procMessageBoxW = windows.NewLazySystemDLL("user32.dll").NewProc("MessageBoxW")

// checkWebView2Runtime logs the runtime Wails is about to use and shows a
// dialog when there is none. It never aborts: the version probe can be wrong
// (a per-user install, a fixed-version runtime path), and a working app must
// not be blocked by a false negative — the log line is what matters.
func checkWebView2Runtime(logger *slog.Logger) {
	version, err := webviewloader.GetAvailableCoreWebView2BrowserVersionString("")
	if err == nil && version != "" {
		logger.Info("WebView2 runtime available", "version", version, "arch", runtime.GOARCH)
		return
	}
	logger.Error("WebView2 runtime unavailable — the window will stay blank",
		"arch", runtime.GOARCH, "error", err)

	arch := "ARM64"
	if runtime.GOARCH != "arm64" {
		arch = "x64"
	}
	messageBox(
		"DroidGear — WebView2 runtime missing",
		"The Microsoft Edge WebView2 Runtime is not installed, so DroidGear "+
			"cannot render its window (it stays invisible or black).\n\n"+
			"Install the "+arch+" build of the Evergreen runtime, then start "+
			"DroidGear again:\n"+webview2DownloadPage+"\n\n"+
			"In a virtual machine (Parallels, UTM, VMware) the runtime may be "+
			"present but unable to composite. Try starting DroidGear from a "+
			"terminal with:\n"+
			"    set WEBVIEW2_ADDITIONAL_BROWSER_ARGUMENTS=--disable-gpu\n\n"+
			"Details are logged to %USERPROFILE%\\.droidgear\\logs\\app.log",
		msgBoxOK|msgBoxIconWarning|msgBoxSystemModal,
	)
}

// messageBox is a thin MessageBoxW wrapper: the app has no console, and this
// runs before the Wails application (and its dialog API) exists.
func messageBox(title, text string, flags uint32) {
	titlePtr, err := windows.UTF16PtrFromString(title)
	if err != nil {
		return
	}
	textPtr, err := windows.UTF16PtrFromString(text)
	if err != nil {
		return
	}
	// #nosec G103 -- MessageBoxW only reads the two NUL-terminated strings.
	_, _, _ = procMessageBoxW.Call(
		0,
		uintptr(unsafe.Pointer(textPtr)),
		uintptr(unsafe.Pointer(titlePtr)),
		uintptr(flags),
	)
}
