package core

import (
	"bytes"
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"strings"
)

// This file holds the small path/JSON helpers that mirror Rust std /
// droidgear-core behaviour. They used to live in internal/app (specs.go),
// where the Droid tool files shared them; internal/service needs them too,
// and service may not import app (layering rule R1), so they live here.

// DecodeJSONValue parses JSON preserving number literals, mirroring
// serde_json::Value (integers stay integers; unknown structure is kept).
func DecodeJSONValue(data []byte) (any, error) {
	dec := json.NewDecoder(bytes.NewReader(data))
	dec.UseNumber()
	var value any
	if err := dec.Decode(&value); err != nil {
		return nil, err
	}
	if dec.More() {
		return nil, fmt.Errorf("trailing garbage after JSON value")
	}
	return value, nil
}

// MarshalPretty renders JSON the way serde_json::to_string_pretty does:
// two-space indent, no HTML escaping, no trailing newline, map keys sorted.
func MarshalPretty(value any) ([]byte, error) {
	var buf bytes.Buffer
	enc := json.NewEncoder(&buf)
	enc.SetEscapeHTML(false)
	enc.SetIndent("", "  ")
	if err := enc.Encode(value); err != nil {
		return nil, err
	}
	return bytes.TrimRight(buf.Bytes(), "\n"), nil
}

// PathExists reports whether path exists (any file type). FileExists is the
// stricter sibling: it only accepts regular files.
func PathExists(path string) bool {
	_, err := os.Stat(path)
	return err == nil
}

// IsSymlink reports whether path is a symlink (mirrors Path::is_symlink).
func IsSymlink(path string) bool {
	info, err := os.Lstat(path)
	return err == nil && info.Mode()&os.ModeSymlink != 0
}

// WithExtension mirrors Rust Path::with_extension: the suffix after the
// final dot is replaced (dotfiles keep their whole name and gain the ext).
func WithExtension(path string, ext string) string {
	dir, base := filepath.Split(path)
	if i := strings.LastIndex(base, "."); i > 0 {
		base = base[:i]
	}
	return dir + base + "." + ext
}

// FactoryHomeForHome resolves the Factory home for home, honoring the
// configPaths.factory override in <home>/.droidgear/settings.json (mirrors
// paths::get_factory_home_for_home). A configPaths object whose known keys
// are not all strings fails to deserialize in Rust and yields the default.
//
// FactoryHome() is the HOME-based variant; this one takes an explicit home
// so tests and per-home tooling do not have to mutate the environment.
func FactoryHomeForHome(home string) string {
	fallback := filepath.Join(home, ".factory")
	data, err := os.ReadFile(filepath.Join(home, ".droidgear", "settings.json")) // #nosec G304 -- user config path
	if err != nil || strings.TrimSpace(string(data)) == "" {
		return fallback
	}
	settings, err := DecodeJSONValue(data)
	if err != nil {
		return fallback
	}
	root, ok := settings.(map[string]any)
	if !ok {
		return fallback
	}
	paths, ok := root["configPaths"].(map[string]any)
	if !ok {
		return fallback
	}
	for _, key := range []string{"factory", "opencode", "opencodeAuth", "codex", "claude", "openclaw", "hermes", "pi", "omp"} {
		if value, present := paths[key]; present {
			if _, isString := value.(string); !isString {
				return fallback
			}
		}
	}
	if override, ok := paths["factory"].(string); ok && override != "" {
		return override
	}
	return fallback
}
