//go:build !windows

package core

// WSL is a Windows feature; the helpers are no-ops elsewhere so the Hermes
// path resolution still compiles for other platforms.

func runWslCapture(_ ...string) (string, bool) {
	return "", false
}

func localAppData() string {
	return ""
}
