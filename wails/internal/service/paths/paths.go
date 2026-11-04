// Package paths exposes the configurable DroidGear path overrides: the
// Preferences "Paths" pane, the `dg path` CLI subcommands and (later) the
// Go TUI all read the same effective values through this package.
//
// It mirrors src-tauri/src/commands/paths.rs field-for-field (camelCase
// EffectivePaths) — the JSON shape is the frontend contract, so the struct
// tags here are load-bearing: internal/app aliases these types instead of
// redefining them.
package paths

import (
	"context"
	"errors"

	"droidgear-wails/internal/core"
)

// EffectivePath mirrors paths::EffectivePath.
type EffectivePath struct {
	Key       string `json:"key"`
	Path      string `json:"path"`
	IsDefault bool   `json:"isDefault"`
}

// EffectivePaths mirrors paths::EffectivePaths (all nine configurable keys).
type EffectivePaths struct {
	Factory      EffectivePath `json:"factory"`
	Opencode     EffectivePath `json:"opencode"`
	OpencodeAuth EffectivePath `json:"opencodeAuth"`
	Codex        EffectivePath `json:"codex"`
	Claude       EffectivePath `json:"claude"`
	Openclaw     EffectivePath `json:"openclaw"`
	Hermes       EffectivePath `json:"hermes"`
	Pi           EffectivePath `json:"pi"`
	Omp          EffectivePath `json:"omp"`
}

// WslInfo mirrors paths::WslInfo.
type WslInfo struct {
	Available bool             `json:"available"`
	Distros   []core.WslDistro `json:"distros"`
}

// keys are the nine configurable path keys, in the order the GUI lists them.
// EffectivePaths and the override map must stay in lockstep with this slice.
var keys = []struct {
	key string
	dst func(*EffectivePaths) *EffectivePath
}{
	{"factory", func(p *EffectivePaths) *EffectivePath { return &p.Factory }},
	{"opencode", func(p *EffectivePaths) *EffectivePath { return &p.Opencode }},
	{"opencodeAuth", func(p *EffectivePaths) *EffectivePath { return &p.OpencodeAuth }},
	{"codex", func(p *EffectivePaths) *EffectivePath { return &p.Codex }},
	{"claude", func(p *EffectivePaths) *EffectivePath { return &p.Claude }},
	{"openclaw", func(p *EffectivePaths) *EffectivePath { return &p.Openclaw }},
	{"hermes", func(p *EffectivePaths) *EffectivePath { return &p.Hermes }},
	{"pi", func(p *EffectivePaths) *EffectivePath { return &p.Pi }},
	{"omp", func(p *EffectivePaths) *EffectivePath { return &p.Omp }},
}

// Keys returns the nine configurable path keys in display order. The CLI
// validates `dg path set <key>` against it and the human table iterates it.
func Keys() []string {
	out := make([]string, 0, len(keys))
	for _, entry := range keys {
		out = append(out, entry.key)
	}
	return out
}

// List flattens the struct into display order, so callers do not need
// reflection (or a nine-branch switch) to render a table.
func (p EffectivePaths) List() []EffectivePath {
	out := make([]EffectivePath, 0, len(keys))
	for _, entry := range keys {
		out = append(out, *entry.dst(&p))
	}
	return out
}

// Effective returns all nine paths with the user's overrides applied.
func Effective(ctx context.Context) (EffectivePaths, error) {
	var out EffectivePaths
	for _, entry := range keys {
		path, isDefault, err := core.EffectivePathForKey(entry.key)
		if err != nil {
			return EffectivePaths{}, err
		}
		*entry.dst(&out) = EffectivePath{Key: entry.key, Path: path, IsDefault: isDefault}
	}
	return out, nil
}

// Defaults returns the built-in default paths (for UI display).
func Defaults(ctx context.Context) (EffectivePaths, error) {
	var out EffectivePaths
	for _, entry := range keys {
		path, err := core.DefaultPathForKey(entry.key)
		if err != nil {
			return EffectivePaths{}, err
		}
		*entry.dst(&out) = EffectivePath{Key: entry.key, Path: path, IsDefault: true}
	}
	return out, nil
}

// SaveOverride persists a single path override.
func SaveOverride(ctx context.Context, key string, path string) error {
	return core.SaveConfigPathOverride(key, path)
}

// ResetOverride removes a single path override.
func ResetOverride(ctx context.Context, key string) error {
	return core.ResetConfigPathOverride(key)
}

// Wsl reports WSL availability and distributions (Windows only).
func Wsl(ctx context.Context) (WslInfo, error) {
	available, distros := core.GetWslInfo()
	if distros == nil {
		distros = []core.WslDistro{}
	}
	return WslInfo{Available: available, Distros: distros}, nil
}

// WslUsername resolves the username inside a WSL distribution.
func WslUsername(ctx context.Context, distro string) (string, error) {
	username, ok := core.GetWslUsername(distro)
	if !ok {
		return "", errors.New("Failed to get WSL username")
	}
	return username, nil
}

// BuildWslPath builds the \\wsl$ path for a config key.
func BuildWslPath(ctx context.Context, distro string, username string, configKey string) (string, error) {
	return core.BuildWslPath(distro, username, configKey)
}
