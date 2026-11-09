package app

import (
	"io"
	"log"
	"log/slog"
	"os"
	"path/filepath"
	"runtime"
	"strings"

	"droidgear-wails/internal/core"
	"droidgear-wails/internal/version"
)

// The GUI has no console to print to — the Windows build is linked with
// `-H windowsgui`, and on macOS/Linux a double-clicked app bundle discards
// stderr as well — so everything Wails logs is normally lost, including the
// one line that explains a blank window (`WebView2 Error: ...`, a missing
// WebView2 runtime, a failed asset request). SetupLogging mirrors the process
// log into ~/.droidgear/logs/app.log so a failed launch leaves evidence
// behind; `dg doctor` reports the same path.
const (
	// LogSizeLimit rotates app.log to app.log.1 once it grows past this size.
	// The file is opened again on the next start, so a crash loop cannot fill
	// the disk.
	LogSizeLimit int64 = 4 << 20

	// LogLevelEnv overrides the level (debug|info|warn|error).
	LogLevelEnv = "DROIDGEAR_LOG_LEVEL"
)

// SetupLogging installs a slog logger that writes to stderr *and* the log
// file, makes it the process default (stdlib `log` included) and returns it
// for application.Options.Logger plus a closer for it.
//
// Logging must never be the reason the app fails to start: if the home
// directory cannot be resolved or the file cannot be opened, this degrades to
// stderr and warns instead of failing.
func SetupLogging() (*slog.Logger, func()) {
	level := logLevel()

	path, err := core.LogPath()
	if err != nil {
		logger, closeFn := stderrLogger(level)
		logger.Warn("log file disabled: cannot resolve ~/.droidgear", "error", err)
		return logger, closeFn
	}
	file, err := openRotatingLog(path, LogSizeLimit)
	if err != nil {
		logger, closeFn := stderrLogger(level)
		logger.Warn("log file disabled", "path", path, "error", err)
		return logger, closeFn
	}

	// Wails (slog) and internal/app (stdlib log) both end up here.
	out := io.MultiWriter(os.Stderr, file)
	logger := slog.New(slog.NewTextHandler(out, &slog.HandlerOptions{Level: level}))
	slog.SetDefault(logger)
	log.SetOutput(out)
	logger.Info("log started",
		"path", path,
		"version", version.Version,
		"level", level.String(),
		"goos", runtime.GOOS,
	)
	return logger, func() { _ = file.Close() }
}

// LogPathString is core.LogPath with the error dropped, for log fields.
func LogPathString() string {
	path, err := core.LogPath()
	if err != nil {
		return ""
	}
	return path
}

// stderrLogger is the fallback used when the log file is unavailable.
func stderrLogger(level slog.Level) (*slog.Logger, func()) {
	logger := slog.New(slog.NewTextHandler(os.Stderr, &slog.HandlerOptions{Level: level}))
	slog.SetDefault(logger)
	log.SetOutput(os.Stderr)
	return logger, func() {}
}

// logLevel reads DROIDGEAR_LOG_LEVEL, defaulting to info.
func logLevel() slog.Level {
	switch strings.ToLower(strings.TrimSpace(os.Getenv(LogLevelEnv))) {
	case "debug":
		return slog.LevelDebug
	case "warn", "warning":
		return slog.LevelWarn
	case "error":
		return slog.LevelError
	default:
		return slog.LevelInfo
	}
}

// openRotatingLog opens path for appending, first rotating an oversized file
// to path+".1" (one generation only).
func openRotatingLog(path string, limit int64) (*os.File, error) {
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		return nil, err
	}
	if info, err := os.Stat(path); err == nil && info.Size() >= limit {
		_ = os.Remove(path + ".1")
		if err := os.Rename(path, path+".1"); err != nil {
			return nil, err
		}
	}
	// #nosec G304 -- fixed path under ~/.droidgear (or DROIDGEAR_LOG_DIR).
	return os.OpenFile(path, os.O_CREATE|os.O_WRONLY|os.O_APPEND, 0o644)
}
