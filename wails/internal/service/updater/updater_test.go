package updater

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"sync/atomic"
	"testing"
)

// ---------------------------------------------------------------------------
// Version handling
// ---------------------------------------------------------------------------

func TestCompareVersions(t *testing.T) {
	cases := []struct {
		a, b string
		want int
	}{
		{"1.3.9", "1.3.9", 0},
		{"1.3.9", "1.3.9.0", 0},
		{"1.3.9.4", "1.3.9.11", -1}, // numeric, not lexicographic
		{"1.3.9.11", "1.3.9.4", 1},
		{"wails-v1.4.0", "1.3.9.11", 1},
		{"v1.3.9", "1.3.9", 0},
		{"dev", "1.3.9", -1},
		{"", "1.3.9", -1},
	}
	for _, tc := range cases {
		if got := CompareVersions(tc.a, tc.b); got != tc.want {
			t.Fatalf("CompareVersions(%q, %q) = %d, want %d", tc.a, tc.b, got, tc.want)
		}
	}
}

func TestNormalizeVersion(t *testing.T) {
	for input, want := range map[string]string{
		"wails-v1.3.9.11": "1.3.9.11",
		"v1.3.9":          "1.3.9",
		" 1.3.9 ":         "1.3.9",
	} {
		if got := NormalizeVersion(input); got != want {
			t.Fatalf("NormalizeVersion(%q) = %q, want %q", input, got, want)
		}
	}
}

func TestAssetFor(t *testing.T) {
	cases := []struct {
		kind        AssetKind
		goos, arch  string
		want        string
		unsupported bool
	}{
		{KindGUI, "windows", "amd64", "droidgear-wails-windows-x64.exe", false},
		{KindGUI, "darwin", "arm64", "droidgear-wails-macos-arm64", false},
		{KindCLI, "linux", "amd64", "dg-linux-x64", false},
		{KindCLI, "windows", "arm64", "", true},
		{AssetKind("nope"), "windows", "amd64", "", true},
	}
	for _, tc := range cases {
		got, err := assetFor(tc.kind, tc.goos, tc.arch)
		if tc.unsupported {
			if !errors.Is(err, ErrUnsupportedPlatform) {
				t.Fatalf("assetFor(%s, %s/%s) err = %v, want ErrUnsupportedPlatform", tc.kind, tc.goos, tc.arch, err)
			}
			continue
		}
		if err != nil || got != tc.want {
			t.Fatalf("assetFor(%s, %s/%s) = %q, %v; want %q", tc.kind, tc.goos, tc.arch, got, err, tc.want)
		}
	}
}

// ---------------------------------------------------------------------------
// Fake release server
// ---------------------------------------------------------------------------

type fakeRelease struct {
	tag        string
	assetName  string
	payload    []byte
	checksums  string // empty = no SHA256SUMS.txt asset
	draft      bool
	prerelease bool
}

type fakeGitHub struct {
	server       *httptest.Server
	assetHits    atomic.Int64
	checksumHits atomic.Int64
	listHits     atomic.Int64
}

func newFakeGitHub(t *testing.T, releases []fakeRelease) *fakeGitHub {
	t.Helper()
	fake := &fakeGitHub{}
	mux := http.NewServeMux()
	mux.HandleFunc("/repos/owner/repo/releases", func(w http.ResponseWriter, r *http.Request) {
		fake.listHits.Add(1)
		base := fake.server.URL
		payload := make([]json.RawMessage, 0, len(releases))
		for _, release := range releases {
			assets := []map[string]any{{
				"name":                 release.assetName,
				"size":                 len(release.payload),
				"browser_download_url": base + "/assets/" + release.tag + "/" + release.assetName,
			}}
			if release.checksums != "" {
				assets = append(assets, map[string]any{
					"name":                 checksumsAsset,
					"size":                 len(release.checksums),
					"browser_download_url": base + "/assets/" + release.tag + "/" + checksumsAsset,
				})
			}
			entry := map[string]any{
				"tag_name":     release.tag,
				"draft":        release.draft,
				"prerelease":   release.prerelease,
				"published_at": "2026-01-01T00:00:00Z",
				"body":         "notes for " + release.tag,
				"html_url":     "https://example.test/" + release.tag,
				"assets":       assets,
			}
			encoded, err := json.Marshal(entry)
			if err != nil {
				t.Errorf("marshal release: %v", err)
			}
			payload = append(payload, encoded)
		}
		w.Header().Set("Content-Type", "application/json")
		_ = json.NewEncoder(w).Encode(payload)
	})
	mux.HandleFunc("/assets/", func(w http.ResponseWriter, r *http.Request) {
		for _, release := range releases {
			name := filepath.Base(r.URL.Path)
			switch name {
			case release.assetName:
				fake.assetHits.Add(1)
				_, _ = w.Write(release.payload)
				return
			case checksumsAsset:
				if release.checksums == "" {
					continue
				}
				fake.checksumHits.Add(1)
				_, _ = w.Write([]byte(release.checksums))
				return
			}
		}
		http.NotFound(w, r)
	})
	fake.server = httptest.NewServer(mux)
	t.Cleanup(fake.server.Close)
	return fake
}

func (f *fakeGitHub) config(t *testing.T, current string) Config {
	t.Helper()
	return Config{
		Repo:           "owner/repo",
		APIBase:        f.server.URL,
		CurrentVersion: current,
		GOOS:           "windows",
		GOARCH:         "amd64",
		CacheDir:       t.TempDir(),
		Executable:     filepath.Join(t.TempDir(), "droidgear.exe"),
	}
}

func checksumLine(payload []byte, name string) string {
	sum := sha256.Sum256(payload)
	return hex.EncodeToString(sum[:]) + "  " + name + "\n"
}

// ---------------------------------------------------------------------------
// Check
// ---------------------------------------------------------------------------

func TestCheckPicksNewestVerifiableRelease(t *testing.T) {
	oldBytes := []byte("old-binary")
	newBytes := []byte("new-binary-bytes")
	fake := newFakeGitHub(t, []fakeRelease{
		{tag: "v1.0.0", assetName: "installer-1.0.0.exe", payload: []byte("x")}, // installer pipeline, ignored
		{tag: "wails-v1.3.9", assetName: "droidgear-wails-windows-x64.exe", payload: oldBytes, checksums: checksumLine(oldBytes, "droidgear-wails-windows-x64.exe")},
		{tag: "wails-v1.4.0", assetName: "droidgear-wails-windows-x64.exe", payload: newBytes, checksums: ""}, // unverifiable → skipped
		{tag: "wails-v1.3.10", assetName: "droidgear-wails-windows-x64.exe", payload: newBytes, checksums: checksumLine(newBytes, "droidgear-wails-windows-x64.exe")},
		{tag: "wails-v9.0.0", assetName: "droidgear-wails-windows-x64.exe", payload: []byte("draft"), draft: true, checksums: "deadbeef  droidgear-wails-windows-x64.exe\n"},
		{tag: "wails-v8.0.0", assetName: "droidgear-wails-windows-x64.exe", payload: []byte("pre"), prerelease: true, checksums: "deadbeef  droidgear-wails-windows-x64.exe\n"},
	})

	result, err := Check(context.Background(), fake.config(t, "1.3.9.11"), KindGUI)
	if err != nil {
		t.Fatalf("Check: %v", err)
	}
	if result.LatestVersion != "1.3.10" {
		t.Fatalf("latest = %q, want 1.3.10", result.LatestVersion)
	}
	if !result.UpdateAvailable {
		t.Fatalf("expected an update to be available: %+v", result)
	}
	if result.AssetName != "droidgear-wails-windows-x64.exe" || result.AssetSize != int64(len(newBytes)) {
		t.Fatalf("asset = %q (%d), want the windows asset (%d bytes)", result.AssetName, result.AssetSize, len(newBytes))
	}
	if result.ReleaseURL == "" || result.Notes != "notes for wails-v1.3.10" {
		t.Fatalf("release metadata not carried: %+v", result)
	}
	if fake.listHits.Load() != 1 {
		t.Fatalf("release list fetched %d times, want 1", fake.listHits.Load())
	}
	// The check must not download the binary.
	if fake.assetHits.Load() != 0 {
		t.Fatalf("check downloaded the asset %d times", fake.assetHits.Load())
	}
}

func TestCheckUsesTheCLIAssetForCLIKind(t *testing.T) {
	cliBytes := []byte("dg-binary")
	fake := newFakeGitHub(t, []fakeRelease{{
		tag:       "wails-v2.0.0",
		assetName: "dg-windows-x64.exe",
		payload:   cliBytes,
		checksums: checksumLine(cliBytes, "dg-windows-x64.exe"),
	}})
	result, err := Check(context.Background(), fake.config(t, "1.9.0"), KindCLI)
	if err != nil {
		t.Fatalf("Check: %v", err)
	}
	if result.AssetName != "dg-windows-x64.exe" || !result.UpdateAvailable {
		t.Fatalf("unexpected result: %+v", result)
	}
}

func TestCheckReportsUpToDate(t *testing.T) {
	payload := []byte("binary")
	fake := newFakeGitHub(t, []fakeRelease{{
		tag:       "wails-v1.3.9",
		assetName: "droidgear-wails-windows-x64.exe",
		payload:   payload,
		checksums: checksumLine(payload, "droidgear-wails-windows-x64.exe"),
	}})
	result, err := Check(context.Background(), fake.config(t, "1.3.9.0"), KindGUI)
	if err != nil {
		t.Fatalf("Check: %v", err)
	}
	if result.UpdateAvailable {
		t.Fatalf("1.3.9.0 must not be offered 1.3.9: %+v", result)
	}
	if result.LatestVersion != "1.3.9" {
		t.Fatalf("latest = %q", result.LatestVersion)
	}
}

func TestCheckDevBuildMakesNoRequest(t *testing.T) {
	fake := newFakeGitHub(t, nil)
	result, err := Check(context.Background(), fake.config(t, "dev"), KindGUI)
	if err != nil {
		t.Fatalf("Check: %v", err)
	}
	if !result.DevBuild || result.UpdateAvailable {
		t.Fatalf("dev build must not be offered an update: %+v", result)
	}
	if fake.listHits.Load() != 0 {
		t.Fatalf("dev build contacted the API")
	}
}

func TestCheckUnsupportedPlatform(t *testing.T) {
	cfg := Config{
		Repo: "owner/repo", APIBase: "https://example.test", CurrentVersion: "1.0.0",
		GOOS: "windows", GOARCH: "arm64", CacheDir: t.TempDir(),
	}
	if _, err := Check(context.Background(), cfg, KindGUI); !errors.Is(err, ErrUnsupportedPlatform) {
		t.Fatalf("err = %v, want ErrUnsupportedPlatform", err)
	}
}

func TestCheckMarksUpstreamFailures(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		http.Error(w, "boom", http.StatusInternalServerError)
	}))
	defer server.Close()
	cfg := Config{
		Repo: "owner/repo", APIBase: server.URL, CurrentVersion: "1.0.0",
		GOOS: "windows", GOARCH: "amd64", CacheDir: t.TempDir(),
	}
	_, err := Check(context.Background(), cfg, KindGUI)
	if !errors.Is(err, ErrUpstream) {
		t.Fatalf("err = %v, want ErrUpstream", err)
	}
}

// ---------------------------------------------------------------------------
// Download
// ---------------------------------------------------------------------------

func TestDownloadVerifiesAndStages(t *testing.T) {
	payload := []byte(strings.Repeat("binary", 1000))
	name := "droidgear-wails-windows-x64.exe"
	fake := newFakeGitHub(t, []fakeRelease{{
		tag:       "wails-v2.0.0",
		assetName: name,
		payload:   payload,
		checksums: checksumLine(payload, name),
	}})
	cfg := fake.config(t, "1.0.0")
	cfg.Executable = filepath.Join(t.TempDir(), "droidgear.exe")
	var progress []Progress
	emitter := EmitterFunc(func(p Progress) { progress = append(progress, p) })

	result, err := Download(context.Background(), cfg, KindGUI, emitter)
	if err != nil {
		t.Fatalf("Download: %v", err)
	}
	if result.AlreadyStaged {
		t.Fatalf("first download should not be reported as already staged")
	}
	if result.Version != "2.0.0" || result.Size != int64(len(payload)) {
		t.Fatalf("result = %+v", result)
	}
	data, err := os.ReadFile(result.Path)
	if err != nil {
		t.Fatalf("read staged file: %v", err)
	}
	if string(data) != string(payload) {
		t.Fatalf("staged bytes differ from the release asset")
	}
	if len(progress) == 0 {
		t.Fatalf("no progress events were emitted")
	}
	if last := progress[len(progress)-1]; last.Percent != 100 {
		t.Fatalf("last progress = %+v", last)
	}

	pending := Pending(cfg)
	if pending == nil || pending.Version != "2.0.0" || pending.SHA256 != result.SHA256 || pending.Kind != KindGUI {
		t.Fatalf("pending state = %+v", pending)
	}

	// A second download reuses the verified bytes.
	hits := fake.assetHits.Load()
	again, err := Download(context.Background(), cfg, KindGUI, nil)
	if err != nil {
		t.Fatalf("second Download: %v", err)
	}
	if !again.AlreadyStaged {
		t.Fatalf("second download should hit the staged copy")
	}
	if fake.assetHits.Load() != hits {
		t.Fatalf("second download re-fetched the asset")
	}
}

func TestDownloadRejectsChecksumMismatch(t *testing.T) {
	payload := []byte("tampered")
	name := "droidgear-wails-windows-x64.exe"
	fake := newFakeGitHub(t, []fakeRelease{{
		tag:       "wails-v2.0.0",
		assetName: name,
		payload:   payload,
		checksums: strings.Repeat("0", 64) + "  " + name + "\n",
	}})
	cfg := fake.config(t, "1.0.0")
	_, err := Download(context.Background(), cfg, KindGUI, nil)
	if !errors.Is(err, ErrChecksumMismatch) {
		t.Fatalf("err = %v, want ErrChecksumMismatch", err)
	}
	if pending := Pending(cfg); pending != nil {
		t.Fatalf("a rejected download must not be recorded as pending: %+v", pending)
	}
	entries, _ := os.ReadDir(filepath.Join(cfg.CacheDir, "2.0.0"))
	for _, entry := range entries {
		if strings.HasSuffix(entry.Name(), ".part") {
			t.Fatalf("partial download left behind: %s", entry.Name())
		}
	}
}

func TestDownloadRequiresChecksumsAsset(t *testing.T) {
	name := "droidgear-wails-windows-x64.exe"
	fake := newFakeGitHub(t, []fakeRelease{{
		tag:       "wails-v2.0.0",
		assetName: name,
		payload:   []byte("binary"),
	}})
	cfg := fake.config(t, "1.0.0")
	// Check already refuses unverifiable releases, so it reports "up to date".
	result, err := Check(context.Background(), cfg, KindGUI)
	if err != nil {
		t.Fatalf("Check: %v", err)
	}
	if result.UpdateAvailable {
		t.Fatalf("release without checksums must not be offered: %+v", result)
	}
	if _, err := Download(context.Background(), cfg, KindGUI, nil); !errors.Is(err, ErrUpToDate) {
		t.Fatalf("err = %v, want ErrUpToDate", err)
	}
}

func TestDownloadUpToDate(t *testing.T) {
	fake := newFakeGitHub(t, nil)
	cfg := fake.config(t, "1.0.0")
	if _, err := Download(context.Background(), cfg, KindGUI, nil); !errors.Is(err, ErrUpToDate) {
		t.Fatalf("err = %v, want ErrUpToDate", err)
	}
}

// ---------------------------------------------------------------------------
// Apply
// ---------------------------------------------------------------------------

// stageForApply writes a fake staged binary plus the pending state, and returns
// the config with a fake current executable.
func stageForApply(t *testing.T, version string) (Config, string, []byte) {
	t.Helper()
	dir := t.TempDir()
	exe := filepath.Join(dir, "droidgear.exe")
	if err := os.WriteFile(exe, []byte("old-binary"), 0o755); err != nil {
		t.Fatalf("write old exe: %v", err)
	}
	cache := t.TempDir()
	stagedDir := filepath.Join(cache, version)
	if err := os.MkdirAll(stagedDir, 0o755); err != nil {
		t.Fatalf("mkdir staged: %v", err)
	}
	payload := []byte("new-binary")
	staged := filepath.Join(stagedDir, "droidgear.exe")
	if err := os.WriteFile(staged, payload, 0o755); err != nil {
		t.Fatalf("write staged: %v", err)
	}
	sum := sha256.Sum256(payload)
	cfg := Config{
		Repo: "owner/repo", APIBase: "https://example.test", CurrentVersion: "1.0.0",
		GOOS: "windows", GOARCH: "amd64", CacheDir: cache, Executable: exe,
	}
	if err := SaveState(cfg, State{Pending: &PendingUpdate{
		Kind: KindGUI, Version: version, AssetName: "droidgear-wails-windows-x64.exe", Path: staged,
		SHA256: hex.EncodeToString(sum[:]), Size: int64(len(payload)),
	}}); err != nil {
		t.Fatalf("save state: %v", err)
	}
	return cfg, staged, payload
}

func TestApplyReplacesTheExecutable(t *testing.T) {
	cfg, staged, payload := stageForApply(t, "1.1.0")
	result, err := Apply(cfg, KindGUI, false, nil)
	if err != nil {
		t.Fatalf("Apply: %v", err)
	}
	if result.ToVersion != "1.1.0" || result.FromVersion != "1.0.0" || result.Restart {
		t.Fatalf("result = %+v", result)
	}
	data, err := os.ReadFile(cfg.Executable)
	if err != nil {
		t.Fatalf("read replaced exe: %v", err)
	}
	if string(data) != string(payload) {
		t.Fatalf("executable was not replaced: %q", data)
	}
	if _, err := os.Stat(staged); !os.IsNotExist(err) {
		t.Fatalf("staged file still present: %v", err)
	}
	if pending := Pending(cfg); pending != nil {
		t.Fatalf("pending state not cleared: %+v", pending)
	}
}

func TestApplyRequiresStagedUpdate(t *testing.T) {
	cfg := Config{CacheDir: t.TempDir(), CurrentVersion: "1.0.0", Executable: filepath.Join(t.TempDir(), "x")}
	if _, err := Apply(cfg, KindGUI, false, nil); !errors.Is(err, ErrNoPending) {
		t.Fatalf("err = %v, want ErrNoPending", err)
	}
}

func TestApplyRefusesTheOtherBinariesAsset(t *testing.T) {
	cfg, _, _ := stageForApply(t, "1.1.0")
	if _, err := Apply(cfg, KindCLI, false, nil); !errors.Is(err, ErrKindMismatch) {
		t.Fatalf("err = %v, want ErrKindMismatch", err)
	}
	data, err := os.ReadFile(cfg.Executable)
	if err != nil || string(data) != "old-binary" {
		t.Fatalf("executable must be untouched, got %q (%v)", data, err)
	}
}

func TestApplyRefusesDowngrade(t *testing.T) {
	cfg, _, _ := stageForApply(t, "0.9.0")
	if _, err := Apply(cfg, KindGUI, false, nil); err == nil {
		t.Fatalf("expected a downgrade to be refused")
	}
}

func TestApplyRefusesTamperedStagedFile(t *testing.T) {
	cfg, staged, _ := stageForApply(t, "1.1.0")
	if err := os.WriteFile(staged, []byte("evil"), 0o755); err != nil {
		t.Fatalf("tamper: %v", err)
	}
	if _, err := Apply(cfg, KindGUI, false, nil); !errors.Is(err, ErrChecksumMismatch) {
		t.Fatalf("err = %v, want ErrChecksumMismatch", err)
	}
	data, err := os.ReadFile(cfg.Executable)
	if err != nil || string(data) != "old-binary" {
		t.Fatalf("executable must be untouched, got %q (%v)", data, err)
	}
}

func TestApplyRefusesDevBuild(t *testing.T) {
	cfg, _, _ := stageForApply(t, "1.1.0")
	cfg.CurrentVersion = "dev"
	if _, err := Apply(cfg, KindGUI, false, nil); !errors.Is(err, ErrDevBuild) {
		t.Fatalf("err = %v, want ErrDevBuild", err)
	}
}

// ---------------------------------------------------------------------------
// Housekeeping
// ---------------------------------------------------------------------------

func TestCleanupStale(t *testing.T) {
	cfg, _, _ := stageForApply(t, "1.1.0")
	old := cfg.Executable + ".old"
	if err := os.WriteFile(old, []byte("previous"), 0o755); err != nil {
		t.Fatalf("write backup: %v", err)
	}
	staleDir := filepath.Join(cfg.CacheDir, "0.9.0")
	if err := os.MkdirAll(staleDir, 0o755); err != nil {
		t.Fatalf("mkdir stale: %v", err)
	}

	CleanupStale(cfg)

	if _, err := os.Stat(old); !os.IsNotExist(err) {
		t.Fatalf("backup not removed: %v", err)
	}
	if _, err := os.Stat(staleDir); !os.IsNotExist(err) {
		t.Fatalf("stale cache dir not removed: %v", err)
	}
	if pending := Pending(cfg); pending == nil {
		t.Fatalf("the pending download must be kept")
	}
	if _, err := os.Stat(filepath.Join(cfg.CacheDir, "1.1.0")); err != nil {
		t.Fatalf("pending version dir removed: %v", err)
	}
}

func TestRelaunchArgsRoundTrip(t *testing.T) {
	pid, ok := RelaunchPID(RelaunchArgs())
	if !ok || pid != os.Getpid() {
		t.Fatalf("RelaunchPID(RelaunchArgs()) = %d, %v", pid, ok)
	}
	if _, ok := RelaunchPID([]string{"--apply-update-relaunch"}); ok {
		t.Fatalf("missing pid must not parse")
	}
	if _, ok := RelaunchPID([]string{"--json"}); ok {
		t.Fatalf("unrelated args must not parse")
	}
}

func TestDismissPersists(t *testing.T) {
	cfg := Config{CacheDir: t.TempDir()}
	if err := Dismiss(cfg, "wails-v2.0.0"); err != nil {
		t.Fatalf("Dismiss: %v", err)
	}
	if got := LoadState(cfg).DismissedVersion; got != "2.0.0" {
		t.Fatalf("dismissed = %q", got)
	}
}

func TestStatePathIsInsideCacheDir(t *testing.T) {
	cfg := Config{CacheDir: filepath.Join("tmp", "cache")}
	if want := filepath.Join("tmp", "cache", "state.json"); StatePath(cfg) != want {
		t.Fatalf("StatePath = %q, want %q", StatePath(cfg), want)
	}
}

// EmitterFunc adapts a function to the Emitter interface (tests only).
type EmitterFunc func(Progress)

func (f EmitterFunc) UpdateProgress(p Progress) { f(p) }
