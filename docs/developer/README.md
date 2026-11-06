# Developer Documentation

Technical documentation for building and extending this app. These docs describe established patterns and are intended for both human developers and AI coding agents.

## Architecture & Patterns

| Document                                                  | Description                                                                                                   |
| --------------------------------------------------------- | ------------------------------------------------------------------------------------------------------------- |
| [Architecture Guide](./architecture-guide.md)             | High-level overview — **Tauri-era, bannered**; current shell docs are `docs/agents/wails-port.md`             |
| [Wails v3 Port Guidelines](./wails-port-guidelines.zh.md) | Go/Wails port audit + GUI/TUI/CLI guidance                                                                    |
| [Wails v3 Round 2](./wails-round2-layout-and-gaps.zh.md)  | Layout regression root cause, window/menu limits, remaining gaps, agent-config review                         |
| [TUI Design](./tui-design.md)                             | Headless TUI architecture — **Rust TUI era** (ADR-001 supersedes; see `wails-tui-cli-design.zh.md`)           |
| [ADR-001: TUI/CLI Fate](./adr-001-tui-fate.zh.md)         | Decision: rewrite the TUI in Go (freeze Rust TUI), phased `internal/service` + `cmd/dg`                       |
| [Wails CLI/TUI Design](./wails-tui-cli-design.zh.md)      | Execution spec: service layering, `dg` CLI contract, headless rules, TUI deferred, Wails-only branch boundary |
| [Tauri Removal Design](./tauri-removal-design.zh.md)      | Plan to drop Tauri entirely: shell abstraction, deletion order, CI/release merge, risks                       |
| [Tauri Removal Tasks](./tauri-removal-tasks.zh.md)        | Execution checklist for the above: per-task files, verification commands, definition of done                  |
| [TUI Parity](./tui-parity.zh.md)                          | Go TUI must match the GUI: GUI-only exemptions, parity matrix, phasing, `dg` retirement                       |
| [State Management](./state-management.md)                 | Three-layer state onion, Zustand, TanStack Query                                                              |
| [Error Handling](./error-handling.md)                     | Error propagation, user feedback, retry patterns                                                              |

## Core Systems

| Document                                                | Description                                                    |
| ------------------------------------------------------- | -------------------------------------------------------------- |
| [Command System](./command-system.md)                   | Unified action dispatch, command registration                  |
| [Keyboard Shortcuts](./keyboard-shortcuts.md)           | Global shortcut handling, platform modifiers                   |
| [Menus](./menus.md)                                     | Native menu building with i18n                                 |
| [Quick Panes](./quick-panes.md)                         | Multi-window quick entry pattern — **Tauri-era, impl removed** |
| [Claude Code Profiles](./claude-code-profile-design.md) | Product and runtime design for Claude Code profiles            |
| [DSH Config Surfaces](./dsh-config-surfaces.zh.md)      | DeepSeek Harness local config model and Dsh page surfaces      |

- [GitHub Copilot BYOK](./copilot-byok.md) — Profiles, local configuration, and temporary launchers

## UI & UX

| Document                                   | Description                                                 |
| ------------------------------------------ | ----------------------------------------------------------- |
| [UI Patterns](./ui-patterns.md)            | CSS architecture, shadcn/ui components                      |
| [Internationalization](./i18n-patterns.md) | Translation system, RTL support                             |
| [Notifications](./notifications.md)        | Toast and native notifications                              |
| [Cross-Platform](./cross-platform.md)      | Platform detection, OS-specific adaptations — **Tauri-era** |

## Data & Storage

| Document                                  | Description                                                      |
| ----------------------------------------- | ---------------------------------------------------------------- |
| [Data Persistence](./data-persistence.md) | File storage patterns — **Tauri-era**; Go: `wails/internal/core` |

## Quality & Tooling

| Document                                              | Description                                             |
| ----------------------------------------------------- | ------------------------------------------------------- |
| [Static Analysis](./static-analysis.md)               | ESLint, Prettier, ast-grep, knip, jscpd, React Compiler |
| [Writing ast-grep Rules](./writing-ast-grep-rules.md) | AI reference for creating custom rules                  |
| [Testing](./testing.md)                               | Test patterns (Vitest)                                  |
| [Writing Docs](./writing-docs.md)                     | Guide for creating and maintaining these docs           |

## Release & Distribution

Release flow (Wails): [`../agents/commands.md`](../agents/commands.md) →
_CI / Release tags_ and [`../../wails/README.md`](../../wails/README.md) →
release workflows.

| Document                                                   | Description                                                                     |
| ---------------------------------------------------------- | ------------------------------------------------------------------------------- |
| [Single-file self-update](./self-update-single-file.zh.md) | How `wails-v*` releases are discovered, verified (`SHA256SUMS.txt`) and swapped |

## Archive

Tauri/Rust-era documents kept for history only — their commands no longer
run: [`../archive/tauri/`](../archive/tauri/README.md).

---

**Updating these docs:** When adding new patterns or systems, update the relevant doc file and add a link here if creating a new document.
