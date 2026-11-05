# AI Agent Instructions

## Rule Zero: Progressive Disclosure

This file contains only core rules. Detailed guidelines live in `docs/agents/`.

**When working on a specific area, read the relevant doc BEFORE making changes:**

| When you need...              | Read                                   |
| ----------------------------- | -------------------------------------- |
| Build/lint/test commands      | `docs/agents/commands.md`              |
| TypeScript/React code style   | `docs/agents/code-style-typescript.md` |
| Rust code style               | `docs/agents/code-style-rust.md`       |
| Zustand / state management    | `docs/agents/state-management.md`      |
| Architecture patterns         | `docs/agents/architecture.md`          |
| UI components (Radix, shadcn) | `docs/agents/ui-components.md`         |
| Wails v3 shell (Go service)   | `docs/agents/wails-port.md`            |

For deep-dive topics, see `docs/developer/README.md`.

## Overview

Tauri v2 + React 19 desktop app. Uses npm (NOT pnpm), TypeScript strict mode, Zustand for state, TanStack Query for persistence.

## Dual Shell (Tauri + Wails v3)

> **Branch scope — `spike/wails-v3-minimal` is Wails-only.** The Tauri shell
> (`src-tauri/`) and the Rust TUI are **frozen**: no new Tauri commands,
> plugins, plugin-\* shims, or Rust TUI features. New work lands in `wails/`
> (Go) + the shared `src/`. Differences where Rust and Go disagree are
> resolved in favour of Go. Rules 1–2 below still apply to the shared
> frontend (shims must resolve), but never extend them for Tauri's sake.
> See `docs/developer/wails-tui-cli-design.zh.md` §0; drop this banner when
> merging back into a dual-shell branch.

`src/` is a component tree shared by two shells: **Tauri** (`src/main.tsx` →
`src-tauri/`) and **Wails v3** (`src/wails/main.tsx` → `wails/`). Outside
`src/wails/` and `src-tauri/`, **any change under `src/` must be evaluated
against both shells**.

1. **Shared code must not depend on unbridged Tauri capabilities**: every new
   `@tauri-apps/plugin-*` usage needs a mapping in `src/wails/shims/` (see
   `wails/PORTING.md` — no no-op shims), or it must be gated via
   `src/lib/capabilities.ts`.
2. **Command surface must stay aligned in four places**: Rust command ↔ Go
   method ↔ shim mapping ↔ generated TS bindings.
   `node scripts/check-command-surface.mjs` enforces this (CI runs it).
3. **Wails verification order**: `npm ci` →
   `wails3 generate bindings -clean=true -ts -i` →
   `npx vite build --config wails/vite.config.ts --mode production` →
   `cd wails && go vet ./... && go test ./...` → `wails3 task run`. Skipping
   the bindings step fails the frontend build (the bindings directory is
   gitignored).
4. **Style changes must check both shells' CSS output**: the Wails build sets
   the Vite `root` to `wails/`, so Tailwind v4's automatic content detection
   misses `src/`. `src/App.css` must keep the explicit
   `@source './**/*.{ts,tsx}'`.
5. **Tauri-only surface (auto-updater)**: never leave a failing entry point
   under Wails — gate it with `isUpdaterSupported()`.

Deep dive: `docs/agents/wails-port.md`; full audit:
`docs/developer/wails-port-guidelines.zh.md` + the round2 follow-up.

## Core Rules (Always Apply)

1. **Progressive disclosure** — Read relevant `docs/agents/*.md` before working on unfamiliar areas
2. **Use npm only** — NOT pnpm
3. **Read before editing** — Understand context first
4. **Run `npm run check:all`** after significant changes
5. **No manual memoization** — React Compiler handles it
6. **Tauri v2 docs only** — v1 patterns are incompatible; for the Wails shell **Wails v3 docs only** (v2 and v3 APIs are incompatible)
7. **No unsolicited commits** — Only when explicitly requested
8. **Use `rm -f`** when removing files
9. **GUI/TUI parity** — When implementing a new GUI feature, synchronously implement the corresponding TUI version. **Transition exception (ADR-001)**: the Rust TUI (`src-tauri/crates/droidgear-tui`) is frozen — new features go to the Go `internal/service` layer and must reach at least one non-GUI consumer (`cmd/dg` CLI or the Go TUI), or the PR description must record the exemption and reason. GUI-only features (window, menu, updater, notifications) are exempt. `cmd/dg` is the primary non-GUI consumer; the contract (layering, flags, exit codes, JSON shapes, exemptions) is `docs/developer/wails-tui-cli-design.zh.md`. See also `docs/developer/adr-001-tui-fate.zh.md`
10. **Pre-commit ready** — Before declaring work complete, run `npm audit fix`, `npm run format` (the `.` means ALL files, not just your changes), and verify `npm run check:all` passes with ZERO errors. If`check:all` reports issues in ANY file — even files you didn't touch — you MUST fix them. Code must be commit-ready; do not leave formatting, lint, or audit issues for the user to discover at commit time

## Version Requirements

Tauri v2.x, React 19.x, Zustand v5.x, Tailwind v4.x, shadcn/ui v4.x, Vite v7.x, Vitest v4.x

### Go-based TypeScript

TypeScript 7.0 (beta) is installed via `@typescript/native-preview@beta` and uses the `tsgo` entry point instead of `tsc`. The stable `typescript` package (v6.x) is retained for tooling compatibility (typescript-eslint, etc.).

- `npm run typecheck` / `npm run build` → uses `tsgo` (TS7)
- `npm run typecheck:ts6` → uses `tsc` (TS6)
- `npm run ts7:version` → shows TS7 version

## File Organization

```
src/
├── components/          # React components by feature
│   ├── ui/              # shadcn/ui components (don't modify)
│   └── layout/          # Layout components
├── hooks/               # Custom React hooks (use-*.ts)
├── lib/
│   ├── commands/        # Command system
│   └── tauri-bindings.ts # Auto-generated (don't edit)
├── store/               # Zustand stores (*-store.ts)
└── services/            # TanStack Query + Tauri integration
src-tauri/
├── src/commands/        # Rust Tauri commands
└── capabilities/        # Window permissions (security)
locales/                 # i18n translation files
docs/
├── agents/              # Agent coding guidelines (read these!)
└── developer/           # Deep-dive technical documentation
```

## Documentation

- **Agent guidelines**: `docs/agents/README.md` — Coding guidelines for AI agents
- **Developer docs**: `docs/developer/README.md` — Deep-dive technical documentation

## Terminology

| Term                               | Meaning                                                                                                                                                                                         |
| ---------------------------------- | ----------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------- |
| **Factory Droid** (简称 **Droid**) | Factory AI 的 coding plan 功能，配置文件位于 `~/.factory/settings.json`，对应代码中的 `factory_settings` 模块和 `CustomModel` 类型。[官方 BYOK 文档](https://docs.factory.ai/cli/byok/overview) |
| **OpenClaw**                       | 独立的 AI 代理框架，配置文件位于 `~/.openclaw/openclaw.json`，对应代码中的 `openclaw` 模块                                                                                                      |
| **OpenCode**                       | 另一个独立工具，对应 `opencode` 模块                                                                                                                                                            |
| **Codex**                          | OpenAI Codex CLI 工具，对应 `codex` 模块                                                                                                                                                        |
