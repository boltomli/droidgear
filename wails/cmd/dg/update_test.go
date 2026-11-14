package main

import (
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"droidgear-wails/internal/version"
)

// fakeUpdateServer serves one verifiable `wails-v2.0.0` release with a CLI
// asset for windows/amd64 plus SHA256SUMS.txt.
func fakeUpdateServer(t *testing.T, payload []byte) *httptest.Server {
	t.Helper()
	return fakeReleaseServer(t, "wails-v2.0.0", false, payload)
}

// fakeReleaseServer serves one verifiable release with the given tag and
// GitHub `prerelease` flag (the channel marker).
func fakeReleaseServer(t *testing.T, tag string, prerelease bool, payload []byte) *httptest.Server {
	t.Helper()
	const asset = "dg-windows-x64.exe"

	var server *httptest.Server
	sum := sha256.Sum256(payload)
	checksums := hex.EncodeToString(sum[:]) + "  " + asset + "\n"

	mux := http.NewServeMux()
	mux.HandleFunc("/repos/owner/repo/releases", func(w http.ResponseWriter, r *http.Request) {
		release := map[string]any{
			"tag_name":     tag,
			"draft":        false,
			"prerelease":   prerelease,
			"published_at": "2026-01-01T00:00:00Z",
			"body":         "notes",
			"html_url":     "https://example.test/" + tag,
			"assets": []map[string]any{
				{"name": asset, "size": len(payload), "browser_download_url": server.URL + "/dl/" + asset},
				{"name": "SHA256SUMS.txt", "size": len(checksums), "browser_download_url": server.URL + "/dl/SHA256SUMS.txt"},
			},
		}
		_ = json.NewEncoder(w).Encode([]any{release})
	})
	mux.HandleFunc("/dl/", func(w http.ResponseWriter, r *http.Request) {
		if filepath.Base(r.URL.Path) == "SHA256SUMS.txt" {
			_, _ = w.Write([]byte(checksums))
			return
		}
		_, _ = w.Write(payload)
	})
	server = httptest.NewServer(mux)
	t.Cleanup(server.Close)
	return server
}

// updateSandbox wires the updater at the fake server and at a throwaway
// executable (never the running test binary).
func updateSandbox(t *testing.T, payload []byte) (exe string, current string) {
	t.Helper()
	return updateSandboxRelease(t, "wails-v2.0.0", false, payload)
}

func updateSandboxRelease(t *testing.T, tag string, prerelease bool, payload []byte) (exe string, current string) {
	t.Helper()
	home := sandbox(t)
	server := fakeReleaseServer(t, tag, prerelease, payload)
	exe = filepath.Join(home, "dg.exe")
	if err := os.WriteFile(exe, []byte("old-binary"), 0o755); err != nil {
		t.Fatalf("write fake exe: %v", err)
	}
	t.Setenv("DROIDGEAR_UPDATE_API", server.URL)
	t.Setenv("DROIDGEAR_UPDATE_REPO", "owner/repo")
	t.Setenv("DROIDGEAR_UPDATE_CACHE", filepath.Join(home, "update-cache"))
	t.Setenv("DROIDGEAR_UPDATE_EXE", exe)

	current = version.Version
	version.Version = "1.0.0"
	t.Cleanup(func() { version.Version = current })
	return exe, current
}

func TestUpdateCheckReportsNewerRelease(t *testing.T) {
	updateSandbox(t, []byte("new-binary"))

	code, out, errOut := runCLI(t, "--json", "update", "--check")
	if code != exitOK {
		t.Fatalf("update --check exit = %d, stderr = %s", code, errOut)
	}
	got := decode(t, out)
	if got["updateAvailable"] != true {
		t.Fatalf("updateAvailable = %v, want true (%s)", got["updateAvailable"], out)
	}
	if got["latestVersion"] != "2.0.0" || got["currentVersion"] != "1.0.0" {
		t.Fatalf("versions = %v → %v", got["currentVersion"], got["latestVersion"])
	}
	if got["assetName"] != "dg-windows-x64.exe" {
		t.Fatalf("assetName = %v", got["assetName"])
	}
	if got["applied"] != false {
		t.Fatalf("--check must not claim an apply: %s", out)
	}
}

func TestUpdateAppliesVerifiedBinary(t *testing.T) {
	payload := []byte("new-binary")
	exe, _ := updateSandbox(t, payload)

	code, out, errOut := runCLI(t, "--json", "update")
	if code != exitOK {
		t.Fatalf("update exit = %d, stderr = %s", code, errOut)
	}
	got := decode(t, out)
	if got["applied"] != true || got["toVersion"] != "2.0.0" || got["fromVersion"] != "1.0.0" {
		t.Fatalf("receipt = %s", out)
	}
	data, err := os.ReadFile(exe)
	if err != nil {
		t.Fatalf("read updated exe: %v", err)
	}
	if string(data) != string(payload) {
		t.Fatalf("dg was not replaced: %q", data)
	}
	if _, err := os.Stat(exe + ".old"); err == nil {
		t.Fatalf("backup left behind after a successful update")
	}
}

func TestUpdateDryRunPlansWithoutDownloading(t *testing.T) {
	exe, _ := updateSandbox(t, []byte("new-binary"))

	code, out, errOut := runCLI(t, "--json", "--dry-run", "update")
	if code != exitOK {
		t.Fatalf("update --dry-run exit = %d, stderr = %s", code, errOut)
	}
	got := decode(t, out)
	if got["applied"] != false || got["updateAvailable"] != true {
		t.Fatalf("dry-run document = %s", out)
	}
	if got["executable"] != exe {
		t.Fatalf("executable = %v, want %v", got["executable"], exe)
	}
	// Nothing was written: the binary and the cache stay untouched.
	data, err := os.ReadFile(exe)
	if err != nil || string(data) != "old-binary" {
		t.Fatalf("dry run replaced the executable: %q (%v)", data, err)
	}
	if _, err := os.Stat(filepath.Join(os.Getenv("DROIDGEAR_UPDATE_CACHE"), "2.0.0")); err == nil {
		t.Fatalf("dry run downloaded the asset")
	}
}

func TestUpdateChannelSwitchesPreviewAndStable(t *testing.T) {
	// A preview (x.y.z.w) build is GitHub-marked prerelease; it is what the
	// single-file pipeline actually publishes.
	updateSandboxRelease(t, "wails-v2.0.1", true, []byte("preview-binary"))

	// Default channel is preview, so the prerelease is offered.
	code, out, errOut := runCLI(t, "--json", "update", "--check")
	if code != exitOK {
		t.Fatalf("update --check exit = %d, stderr = %s", code, errOut)
	}
	got := decode(t, out)
	if got["channel"] != "preview" || got["updateAvailable"] != true {
		t.Fatalf("preview plan = %s", out)
	}
	if got["preview"] != true || got["latestVersion"] != "2.0.1" {
		t.Fatalf("preview plan = %s", out)
	}

	// A stable install does not follow previews.
	code, out, errOut = runCLI(t, "--json", "update", "--check", "--channel", "stable")
	if code != exitOK {
		t.Fatalf("stable check exit = %d, stderr = %s", code, errOut)
	}
	got = decode(t, out)
	if got["channel"] != "stable" || got["updateAvailable"] != false {
		t.Fatalf("stable plan = %s", out)
	}

	// An unknown channel is a usage error, never a silent fallback.
	if code, _, _ := runCLI(t, "--json", "update", "--check", "--channel", "nightly"); code != exitUsage {
		t.Fatalf("exit = %d, want %d for an unknown channel", code, exitUsage)
	}
}

func TestUpdateAllowDowngradeSwitchesChannel(t *testing.T) {
	payload := []byte("stable-binary")
	exe, _ := updateSandboxRelease(t, "wails-v1.4.0.1", false, payload)
	// Running a preview build that the stable line has not caught up with: the
	// stable newest release is older, so the default check is "up to date".
	version.Version = "2.0.0"

	code, out, errOut := runCLI(t, "--json", "update", "--check", "--channel", "stable")
	if code != exitOK {
		t.Fatalf("check exit = %d, stderr = %s", code, errOut)
	}
	if got := decode(t, out); got["updateAvailable"] != false {
		t.Fatalf("a stale-line build must not be offered by default: %s", out)
	}

	// Explicit channel switch: the older release is offered, and labelled.
	code, out, errOut = runCLI(t, "--json", "update", "--check", "--channel", "stable", "--allow-downgrade")
	if code != exitOK {
		t.Fatalf("check exit = %d, stderr = %s", code, errOut)
	}
	got := decode(t, out)
	if got["updateAvailable"] != true || got["downgrade"] != true || got["latestVersion"] != "1.4.0.1" {
		t.Fatalf("downgrade plan = %s", out)
	}

	// And it installs.
	code, out, errOut = runCLI(t, "--json", "update", "--allow-downgrade")
	if code != exitOK {
		t.Fatalf("update exit = %d, stderr = %s", code, errOut)
	}
	got = decode(t, out)
	if got["applied"] != true || got["toVersion"] != "1.4.0.1" || got["downgrade"] != true {
		t.Fatalf("receipt = %s", out)
	}
	if data, err := os.ReadFile(exe); err != nil || string(data) != string(payload) {
		t.Fatalf("dg was not replaced: %q (%v)", data, err)
	}
}

func TestUpdateHumanOutput(t *testing.T) {
	updateSandbox(t, []byte("new-binary"))

	code, out, errOut := runCLI(t, "update", "--check")
	if code != exitOK {
		t.Fatalf("update --check exit = %d, stderr = %s", code, errOut)
	}
	if !strings.Contains(out, "update available: 1.0.0 → 2.0.0") {
		t.Fatalf("human output = %q", out)
	}
	if !strings.Contains(out, "dg-windows-x64.exe") {
		t.Fatalf("human output is missing the asset name: %q", out)
	}
}

func TestUpdateDevBuildNeedsForce(t *testing.T) {
	sandbox(t)
	version.Version = "dev"
	t.Cleanup(func() { version.Version = "dev" })

	code, out, errOut := runCLI(t, "--json", "update", "--check")
	if code != exitRuntime {
		t.Fatalf("exit = %d, want %d (%s)", code, exitRuntime, errOut)
	}
	if out != "" {
		t.Fatalf("a failed command must keep stdout empty, got %q", out)
	}
	var envelope map[string]map[string]string
	if err := json.Unmarshal([]byte(errOut), &envelope); err != nil {
		t.Fatalf("stderr is not one JSON document: %v\n%s", err, errOut)
	}
	if envelope["error"]["code"] != "dev_build" {
		t.Fatalf("error = %v", envelope["error"])
	}
	if !strings.Contains(envelope["error"]["hint"], "--force") {
		t.Fatalf("hint should mention --force: %v", envelope["error"])
	}
}

func TestUpdateUnreachableSourceExits4(t *testing.T) {
	home := sandbox(t)
	t.Setenv("DROIDGEAR_UPDATE_API", "http://127.0.0.1:1")
	t.Setenv("DROIDGEAR_UPDATE_REPO", "owner/repo")
	t.Setenv("DROIDGEAR_UPDATE_CACHE", filepath.Join(home, "update-cache"))
	t.Setenv("DROIDGEAR_UPDATE_EXE", filepath.Join(home, "dg.exe"))
	current := version.Version
	version.Version = "1.0.0"
	t.Cleanup(func() { version.Version = current })

	code, _, errOut := runCLI(t, "--json", "update", "--check")
	if code != exitUpstream {
		t.Fatalf("exit = %d, want %d (%s)", code, exitUpstream, errOut)
	}
	if !strings.Contains(errOut, `"code": "upstream"`) {
		t.Fatalf("stderr = %s", errOut)
	}
}
