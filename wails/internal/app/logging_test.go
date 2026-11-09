package app

import (
	"log"
	"log/slog"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"droidgear-wails/internal/core"
)

// isolateLogging points the log directory at a temp dir and restores the
// process-wide slog/log defaults when the test ends.
func isolateLogging(t *testing.T) string {
	t.Helper()
	dir := t.TempDir()
	t.Setenv(core.LogDirEnv, dir)

	prevDefault := slog.Default()
	prevLogWriter := log.Writer()
	t.Cleanup(func() {
		slog.SetDefault(prevDefault)
		log.SetOutput(prevLogWriter)
	})
	return dir
}

func TestSetupLoggingWritesToTheLogFile(t *testing.T) {
	dir := isolateLogging(t)

	logger, closeLog := SetupLogging()
	defer closeLog()

	// Both the logger returned to Wails and the stdlib logger must land in the
	// file: the Wails "Platform Info" line arrives via slog, our shell messages
	// via the standard log package.
	logger.Error("webview failed", "reason", "test")
	log.Print("stdlib line")
	closeLog()

	data, err := os.ReadFile(filepath.Join(dir, "app.log"))
	if err != nil {
		t.Fatalf("read log: %v", err)
	}
	for _, want := range []string{"log started", "webview failed", "reason=test", "stdlib line"} {
		if !strings.Contains(string(data), want) {
			t.Fatalf("log file missing %q:\n%s", want, data)
		}
	}
}

func TestSetupLoggingHonoursLogLevel(t *testing.T) {
	dir := isolateLogging(t)
	t.Setenv(LogLevelEnv, "warn")

	logger, closeLog := SetupLogging()
	defer closeLog()

	logger.Info("should be dropped")
	logger.Warn("should be kept")

	data, err := os.ReadFile(filepath.Join(dir, "app.log"))
	if err != nil {
		t.Fatalf("read log: %v", err)
	}
	if strings.Contains(string(data), "should be dropped") {
		t.Fatalf("info line written despite DROIDGEAR_LOG_LEVEL=warn:\n%s", data)
	}
	if !strings.Contains(string(data), "should be kept") {
		t.Fatalf("warn line missing:\n%s", data)
	}
}

// A crash loop must not grow the log without bound.
func TestOpenRotatingLogRotatesOnceItIsFull(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "app.log")
	if err := os.WriteFile(path, []byte(strings.Repeat("x", 32)), 0o644); err != nil {
		t.Fatalf("seed log: %v", err)
	}

	file, err := openRotatingLog(path, 16)
	if err != nil {
		t.Fatalf("openRotatingLog: %v", err)
	}
	defer file.Close()

	rotated, err := os.ReadFile(path + ".1")
	if err != nil {
		t.Fatalf("read rotated log: %v", err)
	}
	if len(rotated) != 32 {
		t.Fatalf("rotated log = %d bytes, want 32", len(rotated))
	}
	if info, err := os.Stat(path); err != nil || info.Size() != 0 {
		t.Fatalf("fresh log should be empty, got %v (err %v)", info, err)
	}
}

func TestOpenRotatingLogKeepsOneGeneration(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "app.log")
	if err := os.WriteFile(path+".1", []byte("older"), 0o644); err != nil {
		t.Fatalf("seed rotated log: %v", err)
	}
	if err := os.WriteFile(path, []byte(strings.Repeat("y", 40)), 0o644); err != nil {
		t.Fatalf("seed log: %v", err)
	}

	file, err := openRotatingLog(path, 16)
	if err != nil {
		t.Fatalf("openRotatingLog: %v", err)
	}
	defer file.Close()

	rotated, err := os.ReadFile(path + ".1")
	if err != nil {
		t.Fatalf("read rotated log: %v", err)
	}
	if string(rotated) != strings.Repeat("y", 40) {
		t.Fatalf("path.1 = %q, want the newest generation", rotated)
	}
}

func TestLogLevelParsing(t *testing.T) {
	cases := map[string]slog.Level{
		"":         slog.LevelInfo,
		"debug":    slog.LevelDebug,
		"DEBUG":    slog.LevelDebug,
		"warn":     slog.LevelWarn,
		"warning":  slog.LevelWarn,
		"error":    slog.LevelError,
		"nonsense": slog.LevelInfo,
		" info ":   slog.LevelInfo,
	}
	for value, want := range cases {
		t.Setenv(LogLevelEnv, value)
		if got := logLevel(); got != want {
			t.Errorf("logLevel(%q) = %v, want %v", value, got, want)
		}
	}
}
