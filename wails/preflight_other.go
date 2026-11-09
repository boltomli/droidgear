//go:build !windows

package main

import "log/slog"

// checkWebView2Runtime is a no-op: WebView2 is Windows-only (macOS uses the
// system WebKit, Linux links WebKitGTK), and a missing webview there fails
// loudly at startup instead of leaving an invisible window.
func checkWebView2Runtime(*slog.Logger) {}
