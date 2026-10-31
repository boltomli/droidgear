package core

import (
	"fmt"
	"os"
	"path/filepath"
	"sync/atomic"
	"time"
)

var tempCounter atomic.Uint64

// AtomicWrite replaces path with the given bytes.
//
// A sibling temp file with a unique suffix is written first, then renamed
// over the target, so concurrent writers never share a temp file and readers
// only ever observe a complete document.
func AtomicWrite(path string, data []byte) error {
	return atomicWriteWithMode(path, data, false)
}

// AtomicWritePrivate replaces path with an owner-only (0600) document.
//
// Used for files the DeepSeek Harness also owns and writes with mode 0600
// (`cordis.patch.yml`, `.credentials.yaml`).
func AtomicWritePrivate(path string, data []byte) error {
	return atomicWriteWithMode(path, data, true)
}

func atomicWriteWithMode(path string, data []byte, private bool) error {
	parent := filepath.Dir(path)
	if err := EnsureDir(parent); err != nil {
		return fmt.Errorf("failed to create directory: %w", err)
	}

	tempPath := uniqueTempPath(parent, path)
	mode := os.FileMode(0o644)
	if private {
		mode = 0o600
	}
	file, err := os.OpenFile(tempPath, os.O_WRONLY|os.O_CREATE|os.O_EXCL, mode) // #nosec G304 -- sibling temp file
	if err != nil {
		return fmt.Errorf("failed to write file: %w", err)
	}
	if _, err := file.Write(data); err != nil {
		_ = file.Close()
		_ = os.Remove(tempPath)
		return fmt.Errorf("failed to write file: %w", err)
	}
	if err := file.Close(); err != nil {
		_ = os.Remove(tempPath)
		return fmt.Errorf("failed to write file: %w", err)
	}
	if err := os.Rename(tempPath, path); err != nil {
		_ = os.Remove(tempPath)
		return fmt.Errorf("failed to write file: %w", err)
	}
	return nil
}

func uniqueTempPath(parent, target string) string {
	name := filepath.Base(target)
	if name == "." || name == string(filepath.Separator) || name == "" {
		name = "file"
	}
	nanos := time.Now().UnixNano()
	for counter := uint64(0); ; counter++ {
		suffix := fmt.Sprintf("%d-%d", os.Getpid(), nanos)
		if counter > 0 {
			suffix = fmt.Sprintf("%s-%d", suffix, counter)
		}
		candidate := filepath.Join(parent, "."+name+"."+suffix+".tmp")
		if _, err := os.Stat(candidate); os.IsNotExist(err) {
			return candidate
		}
	}
}
