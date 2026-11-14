// Package updater implements DroidGear's single-file self-update flow.
//
// A release ships exactly one executable per platform (no installer, no
// archive): the Wails GUI (`droidgear-wails-<os>-<arch>[.exe]`) and the
// headless CLI (`dg-<os>-<arch>[.exe]`). Both binaries are the same build, so
// "updating" means: find the newest `wails-v*` release on GitHub, download the
// asset that matches the running binary, verify it against the release's
// `SHA256SUMS.txt`, swap it into place and — for the GUI — restart itself.
//
// The package is Wails-free (layering rule R1): the GUI shell injects an
// Emitter for progress events and performs the quit; `cmd/dg` calls the same
// functions with no emitter and never restarts.
//
// Design/limitations: docs/developer/self-update-single-file.zh.md.
package updater

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"hash"
	"io"
	"net/http"
	"os"
	"path/filepath"
	"runtime"
	"strconv"
	"strings"
	"time"

	"droidgear-wails/internal/core"
	"droidgear-wails/internal/service/preferences"
	"droidgear-wails/internal/version"
)

// ---------------------------------------------------------------------------
// Configuration
// ---------------------------------------------------------------------------

const (
	// DefaultRepo is the repository the release assets are published to.
	DefaultRepo = "boltomli/droidgear"
	// DefaultAPIBase is the public GitHub API root.
	DefaultAPIBase = "https://api.github.com"

	// releaseTagPrefix selects the single-file release pipeline
	// (.github/workflows/release-single.yml). Plain `v*` tags are the
	// installer pipeline and carry no self-update assets.
	releaseTagPrefix = "wails-v"
	// checksumsAsset is uploaded next to the binaries by the same workflow.
	checksumsAsset = "SHA256SUMS.txt"
	// relaunchFlag is passed to the freshly installed binary so it waits for
	// this process to exit (and release the single-instance lock) before it
	// continues normal startup.
	relaunchFlag = "--apply-update-relaunch"

	// devVersion is the build-time fallback in internal/version; a source
	// build must never be silently replaced by a release binary.
	devVersion = "dev"
)

// AssetKind selects which of the two binaries a caller wants to update.
type AssetKind string

const (
	// KindGUI is the Wails desktop shell.
	KindGUI AssetKind = "gui"
	// KindCLI is the `dg` headless binary.
	KindCLI AssetKind = "cli"
)

// Channel selects which releases are offered to the running build.
//
// The GitHub `prerelease` flag is the channel marker: the release pipeline
// ships every build as `x.y.z.w`, and whether that release is a preview or a
// stable one is decided when publishing (`prerelease: true` = preview). A
// preview install follows both; a stable install only takes releases GitHub
// does not flag as pre-releases.
//
// Preview is the default: the pipeline's normal cadence is a preview build, and
// a default that ignored prereleases silently froze updates (see
// docs/developer/self-update-single-file.zh.md §3.0).
type Channel string

const (
	// ChannelPreview accepts preview and stable releases (default).
	ChannelPreview Channel = "preview"
	// ChannelStable accepts released builds only.
	ChannelStable Channel = "stable"

	// ChannelEnv overrides the channel for one process
	// (DROIDGEAR_UPDATE_CHANNEL=preview|stable); it wins over the stored
	// preference so `dg update --channel`-style runs and CI can steer it.
	ChannelEnv = preferences.UpdateChannelEnv
	// DefaultChannel is what an untouched install follows.
	DefaultChannel = ChannelPreview
)

// NormalizeChannel maps an arbitrary value onto a known channel; anything
// that is not "stable" means preview (the permissive default — a typo must
// never look like "nothing to update").
func NormalizeChannel(value string) Channel {
	if strings.EqualFold(strings.TrimSpace(value), string(ChannelStable)) {
		return ChannelStable
	}
	return ChannelPreview
}

// Accepts reports whether a release with this `prerelease` flag is offered on
// the channel.
func (c Channel) Accepts(prerelease bool) bool {
	return c != ChannelStable || !prerelease
}

// Platform assets published by release-single.yml. Keep in sync with the
// workflow matrix; an unknown pair is reported as unsupported instead of
// downloading the wrong binary.
var releaseAssets = map[AssetKind]map[string]string{
	KindGUI: {
		"windows/amd64": "droidgear-wails-windows-x64.exe",
		"windows/arm64": "droidgear-wails-windows-arm64.exe",
		"darwin/arm64":  "droidgear-wails-macos-arm64",
		"linux/amd64":   "droidgear-wails-linux-x64",
	},
	KindCLI: {
		"windows/amd64": "dg-windows-x64.exe",
		"windows/arm64": "dg-windows-arm64.exe",
		"darwin/arm64":  "dg-macos-arm64",
		"linux/amd64":   "dg-linux-x64",
	},
}

// Config carries everything the update flow needs to know about the running
// process. DefaultConfig fills it from the environment so both the GUI and the
// CLI get the same behaviour without wiring.
type Config struct {
	// Repo is "owner/name" (env DROIDGEAR_UPDATE_REPO).
	Repo string
	// APIBase is the GitHub API root (env DROIDGEAR_UPDATE_API) — also the
	// seam the tests use to point at an httptest server.
	APIBase string
	// Token is an optional GitHub token (env DROIDGEAR_UPDATE_TOKEN, then
	// GH_TOKEN, then GITHUB_TOKEN) that lifts the 60 req/h anonymous limit.
	Token string
	// CurrentVersion is internal/version.Version.
	CurrentVersion string
	// GOOS/GOARCH default to the build target.
	GOOS   string
	GOARCH string
	// CacheDir holds the staged download plus state.json (env
	// DROIDGEAR_UPDATE_CACHE).
	CacheDir string
	// Channel is the release channel to follow. Empty resolves through
	// DROIDGEAR_UPDATE_CHANNEL, then preferences.json (`updates_channel`),
	// then ChannelPreview.
	Channel Channel
	// AllowDowngrade lets the flow offer — and install — the channel's newest
	// release even when it is older than the running build. That is how a user
	// who took a preview build rejoins the stable line; it stays off by
	// default because an automatic downgrade is never what anyone wants.
	AllowDowngrade bool
	// Executable is the binary that gets replaced (env DROIDGEAR_UPDATE_EXE
	// overrides it, else os.Executable() — tests and wrapper launchers).
	Executable string
}

// DefaultConfig builds the effective configuration for this process.
func DefaultConfig() Config {
	exe, err := os.Executable()
	if err != nil {
		exe = ""
	}
	return Config{
		Repo:           firstNonEmpty(os.Getenv("DROIDGEAR_UPDATE_REPO"), DefaultRepo),
		APIBase:        firstNonEmpty(os.Getenv("DROIDGEAR_UPDATE_API"), DefaultAPIBase),
		Token:          firstNonEmpty(os.Getenv("DROIDGEAR_UPDATE_TOKEN"), os.Getenv("GH_TOKEN"), os.Getenv("GITHUB_TOKEN")),
		CurrentVersion: version.Version,
		GOOS:           runtime.GOOS,
		GOARCH:         runtime.GOARCH,
		CacheDir:       firstNonEmpty(os.Getenv("DROIDGEAR_UPDATE_CACHE"), defaultCacheDir()),
		Executable:     firstNonEmpty(os.Getenv("DROIDGEAR_UPDATE_EXE"), exe),
		Channel:        resolveChannel(""),
	}
}

// resolveChannel answers "which channel does this process follow?": the
// caller's explicit value, then DROIDGEAR_UPDATE_CHANNEL, then the preference
// written by Preferences → General (GUI) or the TUI Preferences screen (`c`),
// then the preview default.
func resolveChannel(explicit Channel) Channel {
	if strings.TrimSpace(string(explicit)) != "" {
		return NormalizeChannel(string(explicit))
	}
	if env := os.Getenv(ChannelEnv); strings.TrimSpace(env) != "" {
		return NormalizeChannel(env)
	}
	if prefs, err := preferences.Load(); err == nil {
		return NormalizeChannel(prefs.UpdateChannel())
	}
	return DefaultChannel
}

func (c Config) withDefaults() Config {
	if c.Repo == "" {
		c.Repo = DefaultRepo
	}
	if c.APIBase == "" {
		c.APIBase = DefaultAPIBase
	}
	if c.GOOS == "" {
		c.GOOS = runtime.GOOS
	}
	if c.GOARCH == "" {
		c.GOARCH = runtime.GOARCH
	}
	if c.CurrentVersion == "" {
		c.CurrentVersion = version.Version
	}
	if c.CacheDir == "" {
		c.CacheDir = defaultCacheDir()
	}
	c.Channel = resolveChannel(c.Channel)
	if c.Executable == "" {
		if exe, err := os.Executable(); err == nil {
			c.Executable = exe
		}
	}
	return c
}

func defaultCacheDir() string {
	base, err := os.UserCacheDir()
	if err != nil || base == "" {
		base = os.TempDir()
	}
	return filepath.Join(base, "droidgear", "updates")
}

func firstNonEmpty(values ...string) string {
	for _, value := range values {
		if strings.TrimSpace(value) != "" {
			return value
		}
	}
	return ""
}

// ---------------------------------------------------------------------------
// Errors
// ---------------------------------------------------------------------------

var (
	// ErrUpstream marks network / GitHub API failures (CLI exit code 4).
	ErrUpstream = errors.New("update source unreachable")
	// ErrUnsupportedPlatform means no asset is published for this pair.
	ErrUnsupportedPlatform = errors.New("no update published for this platform")
	// ErrNoChecksums means the release cannot be verified (older release).
	ErrNoChecksums = errors.New("release has no SHA256SUMS.txt")
	// ErrChecksumMismatch means the downloaded bytes are not the published ones.
	ErrChecksumMismatch = errors.New("checksum mismatch")
	// ErrKindMismatch means the staged download belongs to the other binary
	// (dg's asset staged for the GUI, or the reverse).
	ErrKindMismatch = errors.New("staged update is for the other binary")
	// ErrNoPending means there is nothing staged to apply.
	ErrNoPending = errors.New("no downloaded update to apply")
	// ErrUpToDate means the running binary is already the newest release.
	ErrUpToDate = errors.New("already up to date")
	// ErrDowngrade means the staged release is older than the running build and
	// the caller did not allow a downgrade (channel switch).
	ErrDowngrade = errors.New("staged release is older than the running build")
	// ErrDevBuild means the running binary was built from source.
	ErrDevBuild = errors.New("development build — updates are disabled")
)

// upstreamError keeps the original message and adds the ErrUpstream mark.
type upstreamError struct{ err error }

func (e upstreamError) Error() string { return e.err.Error() }
func (e upstreamError) Unwrap() error { return e.err }
func (e upstreamError) Is(target error) bool {
	return target == ErrUpstream
}

func upstreamf(format string, args ...any) error {
	return upstreamError{fmt.Errorf(format, args...)}
}

// ---------------------------------------------------------------------------
// Result types (camelCase JSON — shared by bindings and `dg --json`)
// ---------------------------------------------------------------------------

// CheckResult is the answer to "is there a newer build?".
type CheckResult struct {
	CurrentVersion  string `json:"currentVersion"`
	LatestVersion   string `json:"latestVersion"`
	UpdateAvailable bool   `json:"updateAvailable"`
	// DevBuild reports a source build (version "dev"): no offer is made.
	DevBuild bool `json:"devBuild"`
	// Channel is the channel this check followed ("preview" | "stable").
	Channel string `json:"channel"`
	// Preview marks an offered release GitHub flags as a pre-release, i.e. one
	// published on the preview channel. Always false on the stable channel.
	Preview bool `json:"preview"`
	// Downgrade marks an offer that is older than the running build: the
	// channel's newest release, reachable only with AllowDowngrade (an explicit
	// "switch back to this channel" action).
	Downgrade  bool   `json:"downgrade"`
	ReleaseURL string `json:"releaseUrl,omitempty"`
	Notes      string `json:"notes,omitempty"`
	Published  string `json:"publishedAt,omitempty"`
	AssetName  string `json:"assetName,omitempty"`
	AssetSize  int64  `json:"assetSize,omitempty"`
	Checksum   string `json:"checksum,omitempty"`
	// DismissedVersion is the version the user hid the startup banner for.
	DismissedVersion string `json:"dismissedVersion,omitempty"`
	Repo             string `json:"repo"`
	CheckedAt        string `json:"checkedAt"`
}

// DownloadResult describes a verified, staged update.
type DownloadResult struct {
	Kind          AssetKind `json:"kind"`
	Version       string    `json:"version"`
	AssetName     string    `json:"assetName"`
	Path          string    `json:"path"`
	Size          int64     `json:"size"`
	SHA256        string    `json:"sha256"`
	AlreadyStaged bool      `json:"alreadyStaged"`
}

// ApplyResult describes a completed executable swap.
type ApplyResult struct {
	FromVersion string `json:"fromVersion"`
	ToVersion   string `json:"toVersion"`
	Executable  string `json:"executable"`
	Backup      string `json:"backup,omitempty"`
	Restart     bool   `json:"restart"`
	RestartHint string `json:"restartHint,omitempty"`
	// Downgrade reports that ToVersion is older than FromVersion (an explicit
	// channel switch, never an automatic update).
	Downgrade bool `json:"downgrade"`
}

// PendingUpdate is the staged download recorded in state.json.
//
// Kind/AssetName are part of the record on purpose: the GUI and `dg` share the
// cache, and applying the wrong binary (dg's asset over the GUI executable)
// would brick the install.
type PendingUpdate struct {
	Kind         AssetKind `json:"kind"`
	Version      string    `json:"version"`
	AssetName    string    `json:"assetName"`
	Path         string    `json:"path"`
	SHA256       string    `json:"sha256"`
	Size         int64     `json:"size"`
	DownloadedAt string    `json:"downloadedAt"`
}

// State is the small on-disk bookkeeping document (dismissed nags, staged
// download). It is not user configuration — it lives in the cache dir.
type State struct {
	DismissedVersion string         `json:"dismissedVersion,omitempty"`
	LastCheckedAt    string         `json:"lastCheckedAt,omitempty"`
	Pending          *PendingUpdate `json:"pending,omitempty"`
}

// Progress is emitted while downloading/installing.
type Progress struct {
	Phase    string  `json:"phase"` // "download" | "apply"
	Received int64   `json:"received"`
	Total    int64   `json:"total"`
	Percent  float64 `json:"percent"`
	Message  string  `json:"message,omitempty"`
}

// Emitter receives progress events. The GUI injects one backed by the Wails
// event bus; the CLI passes nil (no-op path, never a fake flag).
type Emitter interface {
	UpdateProgress(Progress)
}

func emit(emitter Emitter, progress Progress) {
	if emitter != nil {
		emitter.UpdateProgress(progress)
	}
}

// ---------------------------------------------------------------------------
// Versions
// ---------------------------------------------------------------------------

// NormalizeVersion strips the tag prefix and a leading "v": `wails-v1.3.9.11`
// and `v1.3.9.11` both become `1.3.9.11`.
func NormalizeVersion(tag string) string {
	tag = strings.TrimSpace(tag)
	tag = strings.TrimPrefix(tag, releaseTagPrefix)
	tag = strings.TrimPrefix(tag, "v")
	return tag
}

// parseVersion turns `1.3.9.11` into its numeric parts. A trailing
// prerelease/build suffix is ignored (the single-file pipeline publishes
// release tags only).
func parseVersion(v string) ([]int, bool) {
	v = NormalizeVersion(v)
	if v == "" || v == devVersion {
		return nil, false
	}
	core3 := v
	if idx := strings.IndexAny(core3, "-+"); idx >= 0 {
		core3 = core3[:idx]
	}
	parts := strings.Split(core3, ".")
	out := make([]int, 0, len(parts))
	for _, part := range parts {
		n, err := strconv.Atoi(part)
		if err != nil || n < 0 {
			return nil, false
		}
		out = append(out, n)
	}
	return out, true
}

// IsDevBuild reports whether the running version came from a source build
// (internal/version's "dev" fallback, or an unparseable value).
func IsDevBuild(v string) bool {
	_, ok := parseVersion(v)
	return !ok
}

// CompareVersions returns -1, 0 or 1. Missing trailing parts count as 0, so
// 1.3.9 == 1.3.9.0 < 1.3.9.1. Unparseable versions sort below everything.
func CompareVersions(a, b string) int {
	left, leftOK := parseVersion(a)
	right, rightOK := parseVersion(b)
	switch {
	case !leftOK && !rightOK:
		return 0
	case !leftOK:
		return -1
	case !rightOK:
		return 1
	}
	for i := 0; i < len(left) || i < len(right); i++ {
		var l, r int
		if i < len(left) {
			l = left[i]
		}
		if i < len(right) {
			r = right[i]
		}
		if l != r {
			if l < r {
				return -1
			}
			return 1
		}
	}
	return 0
}

// assetFor maps the running platform onto the published asset name.
func assetFor(kind AssetKind, goos, goarch string) (string, error) {
	assets, ok := releaseAssets[kind]
	if !ok {
		return "", fmt.Errorf("%w: unknown target %q", ErrUnsupportedPlatform, kind)
	}
	name, ok := assets[goos+"/"+goarch]
	if !ok {
		return "", fmt.Errorf("%w: %s/%s", ErrUnsupportedPlatform, goos, goarch)
	}
	return name, nil
}

// ---------------------------------------------------------------------------
// GitHub release discovery
// ---------------------------------------------------------------------------

type ghRelease struct {
	TagName     string    `json:"tag_name"`
	Name        string    `json:"name"`
	Draft       bool      `json:"draft"`
	Prerelease  bool      `json:"prerelease"`
	PublishedAt string    `json:"published_at"`
	Body        string    `json:"body"`
	HTMLURL     string    `json:"html_url"`
	Assets      []ghAsset `json:"assets"`
}

type ghAsset struct {
	Name               string `json:"name"`
	Size               int64  `json:"size"`
	BrowserDownloadURL string `json:"browser_download_url"`
}

func (r ghRelease) findAsset(name string) (ghAsset, bool) {
	for _, asset := range r.Assets {
		if asset.Name == name {
			return asset, true
		}
	}
	return ghAsset{}, false
}

func (c Config) apiClient() *http.Client { return core.NewHTTPClient(30 * time.Second) }

// get performs one authenticated GET. Every network failure is marked
// ErrUpstream so the CLI can map it to exit code 4 and the GUI can offer a
// retry instead of a generic error.
func (c Config) get(ctx context.Context, url string) (*http.Response, error) {
	request, err := http.NewRequestWithContext(ctx, http.MethodGet, url, nil)
	if err != nil {
		return nil, upstreamf("invalid update URL %q: %v", url, err)
	}
	request.Header.Set("Accept", "application/vnd.github+json")
	request.Header.Set("X-GitHub-Api-Version", "2022-11-28")
	request.Header.Set("User-Agent", "DroidGear/"+c.CurrentVersion)
	if c.Token != "" {
		request.Header.Set("Authorization", "Bearer "+c.Token)
	}
	response, err := c.apiClient().Do(request)
	if err != nil {
		return nil, upstreamf("cannot reach %s: %v", url, err)
	}
	return response, nil
}

// fetchReleases lists the published single-file releases (newest first).
func fetchReleases(ctx context.Context, cfg Config) ([]ghRelease, error) {
	endpoint := fmt.Sprintf("%s/repos/%s/releases?per_page=30", strings.TrimRight(cfg.APIBase, "/"), cfg.Repo)
	response, err := cfg.get(ctx, endpoint)
	if err != nil {
		return nil, err
	}
	defer func() { _ = response.Body.Close() }()

	body, readErr := io.ReadAll(io.LimitReader(response.Body, 8<<20))
	if readErr != nil {
		return nil, upstreamf("cannot read the release list: %v", readErr)
	}
	if response.StatusCode == http.StatusForbidden || response.StatusCode == http.StatusTooManyRequests {
		reset := response.Header.Get("X-RateLimit-Reset")
		hint := "set DROIDGEAR_UPDATE_TOKEN (or GH_TOKEN) to raise the GitHub API limit"
		if remaining := response.Header.Get("X-RateLimit-Remaining"); remaining == "0" && reset != "" {
			if seconds, err := strconv.ParseInt(reset, 10, 64); err == nil {
				hint = fmt.Sprintf("rate limit resets at %s — %s",
					time.Unix(seconds, 0).Format(time.RFC3339), hint)
			}
		}
		return nil, upstreamf("GitHub API rejected the request (%s); %s", response.Status, hint)
	}
	if response.StatusCode != http.StatusOK {
		return nil, upstreamf("GitHub API returned %s: %s", response.Status, core.TruncateBody(string(body)))
	}
	var releases []ghRelease
	if err := json.Unmarshal(body, &releases); err != nil {
		return nil, upstreamf("cannot parse the release list: %v", err)
	}
	return releases, nil
}

// Check reports whether a newer verifiable release exists for this binary on
// the configured channel.
func Check(ctx context.Context, cfg Config, kind AssetKind) (CheckResult, error) {
	cfg = cfg.withDefaults()
	channel := cfg.Channel
	now := time.Now().UTC().Format(time.RFC3339)
	state := LoadState(cfg)
	result := CheckResult{
		CurrentVersion:   cfg.CurrentVersion,
		Channel:          string(channel),
		DismissedVersion: state.DismissedVersion,
		Repo:             cfg.Repo,
		CheckedAt:        now,
	}

	assetName, err := assetFor(kind, cfg.GOOS, cfg.GOARCH)
	if err != nil {
		return result, err
	}
	if _, ok := parseVersion(cfg.CurrentVersion); !ok {
		// Source build: report the version we have, offer nothing.
		result.DevBuild = true
		result.LatestVersion = cfg.CurrentVersion
		return result, nil
	}

	releases, err := fetchReleases(ctx, cfg)
	if err != nil {
		return result, err
	}

	var latest *ghRelease
	var latestAsset ghAsset
	for i := range releases {
		release := releases[i]
		if release.Draft {
			continue
		}
		// The channel decides whether GitHub's `prerelease` flag (the preview
		// channel marker) is acceptable.
		if !channel.Accepts(release.Prerelease) {
			continue
		}
		tag := NormalizeVersion(release.TagName)
		if !strings.HasPrefix(strings.TrimSpace(release.TagName), releaseTagPrefix) {
			continue
		}
		if _, ok := parseVersion(tag); !ok {
			continue
		}
		asset, ok := release.findAsset(assetName)
		if !ok {
			continue
		}
		// Unverifiable releases are never offered: without the checksums file
		// there is no way to prove the bytes are the ones CI produced.
		if _, ok := release.findAsset(checksumsAsset); !ok {
			continue
		}
		if latest == nil || CompareVersions(tag, NormalizeVersion(latest.TagName)) > 0 {
			latest = &releases[i]
			latestAsset = asset
		}
	}
	if latest == nil {
		// Nothing published for this platform yet — not an error, just no offer.
		result.LatestVersion = cfg.CurrentVersion
		return result, nil
	}

	result.LatestVersion = NormalizeVersion(latest.TagName)
	result.Preview = latest.Prerelease
	result.ReleaseURL = latest.HTMLURL
	result.Notes = latest.Body
	result.Published = latest.PublishedAt
	result.AssetName = latestAsset.Name
	result.AssetSize = latestAsset.Size
	result.UpdateAvailable = CompareVersions(result.LatestVersion, result.CurrentVersion) > 0
	// A channel switch may point at an older line (the running build came from
	// a preview that stable has not caught up with). Only an explicit
	// AllowDowngrade turns that into an offer.
	if !result.UpdateAvailable && cfg.AllowDowngrade {
		if CompareVersions(result.LatestVersion, result.CurrentVersion) < 0 {
			result.UpdateAvailable = true
			result.Downgrade = true
		}
	}

	// `dg --dry-run` must stay read-only: the timestamp is bookkeeping, so it
	// is simply not recorded there.
	if !core.InDryRun() {
		state.LastCheckedAt = now
		_ = SaveState(cfg, state)
	}
	return result, nil
}

// ---------------------------------------------------------------------------
// Download
// ---------------------------------------------------------------------------

// Download checks for an update and, when one is available, stages a verified
// copy in the cache directory. Re-downloading the same version is skipped
// (AlreadyStaged) when the staged bytes still match.
func Download(ctx context.Context, cfg Config, kind AssetKind, emitter Emitter) (DownloadResult, error) {
	cfg = cfg.withDefaults()
	check, err := Check(ctx, cfg, kind)
	if err != nil {
		return DownloadResult{}, err
	}
	if !check.UpdateAvailable {
		return DownloadResult{}, fmt.Errorf("%w: %s (latest %s)", ErrUpToDate, check.CurrentVersion, check.LatestVersion)
	}

	// Re-fetch the release so the download URLs/checksums come from the same
	// document the plan was built from.
	releases, err := fetchReleases(ctx, cfg)
	if err != nil {
		return DownloadResult{}, err
	}
	var chosen *ghRelease
	for i := range releases {
		if NormalizeVersion(releases[i].TagName) == check.LatestVersion {
			chosen = &releases[i]
			break
		}
	}
	if chosen == nil {
		return DownloadResult{}, upstreamf("release %s vanished from the release list", check.LatestVersion)
	}
	asset, ok := chosen.findAsset(check.AssetName)
	if !ok {
		return DownloadResult{}, upstreamf("release %s no longer has %s", check.LatestVersion, check.AssetName)
	}
	checksums, ok := chosen.findAsset(checksumsAsset)
	if !ok {
		return DownloadResult{}, fmt.Errorf("%w: %s", ErrNoChecksums, check.LatestVersion)
	}

	expected, err := fetchChecksum(ctx, cfg, checksums, check.AssetName)
	if err != nil {
		return DownloadResult{}, err
	}
	dir := filepath.Join(cfg.CacheDir, check.LatestVersion)
	target := filepath.Join(dir, check.AssetName)
	if staged, ok := stagedMatches(target, expected, asset.Size); ok {
		result := DownloadResult{
			Kind:          kind,
			Version:       check.LatestVersion,
			AssetName:     check.AssetName,
			Path:          staged,
			Size:          asset.Size,
			SHA256:        expected,
			AlreadyStaged: true,
		}
		if err := recordPending(cfg, result); err != nil {
			return DownloadResult{}, err
		}
		return result, nil
	}

	if err := os.MkdirAll(dir, 0o755); err != nil {
		return DownloadResult{}, fmt.Errorf("cannot create the update cache: %w", err)
	}
	partial := target + ".part"
	_ = os.Remove(partial)
	if err := downloadToFile(ctx, cfg, asset, partial, emitter); err != nil {
		_ = os.Remove(partial)
		return DownloadResult{}, err
	}
	sum, size, err := fileHash(partial)
	if err != nil {
		_ = os.Remove(partial)
		return DownloadResult{}, err
	}
	if asset.Size > 0 && size != asset.Size {
		_ = os.Remove(partial)
		return DownloadResult{}, fmt.Errorf("%w: expected %d bytes, got %d", ErrChecksumMismatch, asset.Size, size)
	}
	if !strings.EqualFold(sum, expected) {
		_ = os.Remove(partial)
		return DownloadResult{}, fmt.Errorf("%w: expected %s, got %s", ErrChecksumMismatch, expected, sum)
	}
	if err := os.Rename(partial, target); err != nil {
		_ = os.Remove(partial)
		return DownloadResult{}, err
	}
	_ = os.Chmod(target, 0o755)

	result := DownloadResult{
		Kind:      kind,
		Version:   check.LatestVersion,
		AssetName: check.AssetName,
		Path:      target,
		Size:      size,
		SHA256:    sum,
	}
	if err := recordPending(cfg, result); err != nil {
		return DownloadResult{}, err
	}
	return result, nil
}

func downloadToFile(ctx context.Context, cfg Config, asset ghAsset, dest string, emitter Emitter) error {
	response, err := cfg.get(ctx, asset.BrowserDownloadURL)
	if err != nil {
		return fmt.Errorf("cannot download %s: %w", asset.Name, err)
	}
	defer func() { _ = response.Body.Close() }()
	if response.StatusCode != http.StatusOK {
		body, _ := io.ReadAll(io.LimitReader(response.Body, 4<<10))
		return upstreamf("download failed with %s: %s", response.Status, core.TruncateBody(string(body)))
	}

	file, err := os.OpenFile(dest, os.O_CREATE|os.O_WRONLY|os.O_TRUNC, 0o600) // #nosec G304 -- cache path built from the release asset name
	if err != nil {
		return err
	}
	sink := &progressSink{writer: file, total: asset.Size, emitter: emitter}
	_, copyErr := io.Copy(sink, response.Body)
	closeErr := file.Close()
	if copyErr != nil {
		return upstreamf("download interrupted: %v", copyErr)
	}
	if closeErr != nil {
		return closeErr
	}
	sink.finish()
	return nil
}

// progressSink hashes the stream and throttles progress events to ~10/s so the
// event bus is not flooded by a 60 MB download.
type progressSink struct {
	writer  io.Writer
	digest  hash.Hash
	total   int64
	done    int64
	emitter Emitter
	last    time.Time
}

func (s *progressSink) Write(p []byte) (int, error) {
	if s.digest == nil {
		s.digest = sha256.New()
	}
	n, err := s.writer.Write(p)
	if n > 0 {
		_, _ = s.digest.Write(p[:n])
		s.done += int64(n)
		if now := time.Now(); now.Sub(s.last) >= 100*time.Millisecond {
			s.last = now
			emit(s.emitter, Progress{Phase: "download", Received: s.done, Total: s.total, Percent: percent(s.done, s.total)})
		}
	}
	return n, err
}

func (s *progressSink) finish() {
	emit(s.emitter, Progress{Phase: "download", Received: s.done, Total: s.total, Percent: percent(s.done, s.total), Message: "verified"})
}

func percent(done, total int64) float64 {
	if total <= 0 {
		return 0
	}
	value := float64(done) / float64(total) * 100
	if value > 100 {
		return 100
	}
	return value
}

func fetchChecksum(ctx context.Context, cfg Config, asset ghAsset, name string) (string, error) {
	response, err := cfg.get(ctx, asset.BrowserDownloadURL)
	if err != nil {
		return "", fmt.Errorf("cannot fetch %s: %w", checksumsAsset, err)
	}
	defer func() { _ = response.Body.Close() }()
	if response.StatusCode != http.StatusOK {
		return "", upstreamf("%s download failed with %s", checksumsAsset, response.Status)
	}
	body, err := io.ReadAll(io.LimitReader(response.Body, 1<<20))
	if err != nil {
		return "", upstreamf("cannot read %s: %v", checksumsAsset, err)
	}
	for _, line := range strings.Split(string(body), "\n") {
		fields := strings.Fields(strings.TrimSpace(line))
		if len(fields) != 2 {
			continue
		}
		// Accept both `sha  name` and `sha *name` (binary-mode marker).
		if strings.TrimPrefix(fields[1], "*") == name {
			return strings.ToLower(fields[0]), nil
		}
	}
	return "", fmt.Errorf("%w: %s is not listed in %s", ErrNoChecksums, name, checksumsAsset)
}

func stagedMatches(path, checksum string, size int64) (string, bool) {
	info, err := os.Stat(path)
	if err != nil || info.IsDir() {
		return "", false
	}
	if size > 0 && info.Size() != size {
		return "", false
	}
	sum, got, err := fileHash(path)
	if err != nil || got != info.Size() || !strings.EqualFold(sum, checksum) {
		return "", false
	}
	return path, true
}

func fileHash(path string) (string, int64, error) {
	file, err := os.Open(path) // #nosec G304 -- cache dir, verified above
	if err != nil {
		return "", 0, err
	}
	defer func() { _ = file.Close() }()
	digest := sha256.New()
	size, err := io.Copy(digest, file)
	if err != nil {
		return "", 0, err
	}
	return hex.EncodeToString(digest.Sum(nil)), size, nil
}

// ---------------------------------------------------------------------------
// State
// ---------------------------------------------------------------------------

// StatePath is the bookkeeping file inside the cache directory.
func StatePath(cfg Config) string {
	cfg = cfg.withDefaults()
	return filepath.Join(cfg.CacheDir, "state.json")
}

// LoadState reads state.json, falling back to an empty document.
func LoadState(cfg Config) State {
	data, err := os.ReadFile(StatePath(cfg)) // #nosec G304 -- fixed cache path
	if err != nil {
		return State{}
	}
	var state State
	if err := json.Unmarshal(data, &state); err != nil {
		return State{}
	}
	return state
}

// SaveState writes state.json atomically (creating the cache dir).
func SaveState(cfg Config, state State) error {
	path := StatePath(cfg)
	if err := core.EnsureDir(filepath.Dir(path)); err != nil {
		return err
	}
	data, err := json.MarshalIndent(state, "", "  ")
	if err != nil {
		return err
	}
	return core.AtomicWrite(path, data)
}

// Pending returns the staged update, if any.
func Pending(cfg Config) *PendingUpdate {
	return LoadState(cfg).Pending
}

// Dismiss records the version the user does not want to be nagged about.
func Dismiss(cfg Config, version string) error {
	state := LoadState(cfg)
	state.DismissedVersion = NormalizeVersion(version)
	return SaveState(cfg, state)
}

func recordPending(cfg Config, result DownloadResult) error {
	state := LoadState(cfg)
	state.Pending = &PendingUpdate{
		Kind:         result.Kind,
		Version:      result.Version,
		AssetName:    result.AssetName,
		Path:         result.Path,
		SHA256:       result.SHA256,
		Size:         result.Size,
		DownloadedAt: time.Now().UTC().Format(time.RFC3339),
	}
	return SaveState(cfg, state)
}

// ---------------------------------------------------------------------------
// Apply
// ---------------------------------------------------------------------------

// Apply installs the staged update over the running executable.
//
// With restart=true (GUI only) the freshly installed binary is spawned with
// the internal relaunch flag and the caller is expected to quit right after;
// the child waits for this process to exit before continuing, because the
// Wails single-instance lock is still held until then.
func Apply(cfg Config, kind AssetKind, restart bool, emitter Emitter) (ApplyResult, error) {
	cfg = cfg.withDefaults()
	state := LoadState(cfg)
	pending := state.Pending
	if pending == nil || pending.Path == "" {
		return ApplyResult{}, ErrNoPending
	}
	assetName, err := assetFor(kind, cfg.GOOS, cfg.GOARCH)
	if err != nil {
		return ApplyResult{}, err
	}
	if pending.Kind != kind || pending.AssetName != assetName {
		return ApplyResult{}, fmt.Errorf("%w: staged %q for %q, expected %q for %q",
			ErrKindMismatch, pending.Version, pending.Kind, assetName, kind)
	}

	target := cfg.Executable
	if resolved, err := filepath.EvalSymlinks(target); err == nil {
		target = resolved
	}
	if _, ok := parseVersion(cfg.CurrentVersion); !ok {
		return ApplyResult{}, ErrDevBuild
	}
	if info, err := os.Stat(target); err != nil || info.IsDir() {
		return ApplyResult{}, fmt.Errorf("cannot update %s: %w", target, err)
	}
	switch cmp := CompareVersions(pending.Version, cfg.CurrentVersion); {
	case cmp == 0:
		return ApplyResult{}, fmt.Errorf("staged version %s is already installed", pending.Version)
	case cmp < 0 && !cfg.AllowDowngrade:
		// Rejoining an older line (a preview build switching back to stable) is a
		// deliberate move: it only happens when the caller asked for it.
		return ApplyResult{}, fmt.Errorf("%w: staged %s is older than %s", ErrDowngrade, pending.Version, cfg.CurrentVersion)
	}

	sum, size, err := fileHash(pending.Path)
	if err != nil {
		return ApplyResult{}, fmt.Errorf("%w: staged file unreadable: %v", ErrNoPending, err)
	}
	if !strings.EqualFold(sum, pending.SHA256) || (pending.Size > 0 && size != pending.Size) {
		return ApplyResult{}, fmt.Errorf("%w: staged file changed after download", ErrChecksumMismatch)
	}

	emit(emitter, Progress{Phase: "apply", Message: "installing " + pending.Version})
	backup, err := installFile(pending.Path, target)
	if err != nil {
		return ApplyResult{}, err
	}

	result := ApplyResult{
		FromVersion: cfg.CurrentVersion,
		ToVersion:   pending.Version,
		Executable:  target,
		Backup:      backup,
		Restart:     restart,
		Downgrade:   CompareVersions(pending.Version, cfg.CurrentVersion) < 0,
	}
	if backup != "" {
		// Windows keeps the old image locked while this process runs, so the
		// backup may already be gone (or still be there until the next start).
		if _, err := os.Stat(backup); err != nil {
			result.Backup = ""
		}
	}

	if restart {
		if err := spawnRelaunch(target); err != nil {
			// The new binary is in place; only the automatic restart failed.
			result.Restart = false
			result.RestartHint = "restart DroidGear manually to finish the update"
			return result, nil //nolint:nilerr // the swap succeeded; restart failure is reported, not fatal
		}
		result.RestartHint = "restarting"
	}

	// Clear the staged entry and the leftover bytes; the executable is already
	// replaced, so a failure here is cosmetic.
	state.Pending = nil
	_ = SaveState(cfg, state)
	_ = os.RemoveAll(filepath.Dir(pending.Path))
	return result, nil
}

// CleanupStale removes the previous executable (Windows keeps it locked while
// the old process runs) and cache entries that are not the pending download.
// Called at GUI startup and from `dg update`.
func CleanupStale(cfg Config) {
	cfg = cfg.withDefaults()
	if cfg.Executable != "" {
		_ = os.Remove(cfg.Executable + ".old")
		_ = os.Remove(cfg.Executable + ".new")
	}
	pending := Pending(cfg)
	keep := ""
	if pending != nil {
		keep = filepath.Dir(pending.Path)
	}
	entries, err := os.ReadDir(cfg.CacheDir)
	if err != nil {
		return
	}
	for _, entry := range entries {
		if !entry.IsDir() {
			continue
		}
		dir := filepath.Join(cfg.CacheDir, entry.Name())
		if keep != "" && filepath.Clean(dir) == filepath.Clean(keep) {
			continue
		}
		_ = os.RemoveAll(dir)
	}
}

// RelaunchArgs returns the argv the freshly installed GUI binary is started
// with, plus whether argv carries that relaunch marker.
func RelaunchArgs() []string {
	return []string{relaunchFlag, strconv.Itoa(os.Getpid())}
}

// RelaunchPID reports the parent PID from a relaunch invocation.
func RelaunchPID(args []string) (int, bool) {
	if len(args) != 2 || args[0] != relaunchFlag {
		return 0, false
	}
	pid, err := strconv.Atoi(args[1])
	if err != nil || pid <= 0 {
		return 0, false
	}
	return pid, true
}

// WaitForExit blocks until pid is gone or the timeout elapses. It is used by
// the relaunched binary so it does not fight the old process for the
// single-instance lock.
func WaitForExit(pid int, timeout time.Duration) {
	deadline := time.Now().Add(timeout)
	for time.Now().Before(deadline) {
		if !processAlive(pid) {
			return
		}
		time.Sleep(100 * time.Millisecond)
	}
}
