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

For deep-dive topics, see `docs/developer/README.md`.

## Overview

Tauri v2 + React 19 desktop app. Uses npm (NOT pnpm), TypeScript strict mode, Zustand for state, TanStack Query for persistence.

## Core Rules (Always Apply)

1. **Progressive disclosure** — Read relevant `docs/agents/*.md` before working on unfamiliar areas
2. **Use npm only** — NOT pnpm
3. **Read before editing** — Understand context first
4. **Run `npm run check:all`** after significant changes
5. **No manual memoization** — React Compiler handles it
6. **Tauri v2 docs only** — v1 patterns are incompatible
7. **No unsolicited commits** — Only when explicitly requested
8. **Use `rm -f`** when removing files
9. **GUI/TUI parity** — When implementing a new GUI feature, synchronously implement the corresponding TUI version
10. **Pre-commit ready** — Before declaring work complete, run `npm audit fix`, `npm run rust:fmt`, `npm run format` (the `.` means ALL files, not just your changes), and verify `npm run check:all` passes with ZERO errors. If `check:all` reports issues in ANY file — even files you didn't touch — you MUST fix them. Code must be commit-ready; do not leave formatting, lint, or audit issues for the user to discover at commit time

## Version Requirements

Tauri v2.x, React 19.x, Zustand v5.x, Tailwind v4.x, shadcn/ui v4.x, Vite v8.x, Vitest v4.x

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

## Where to Look (Extended)

| Task                     | Location                               | Notes                          |
| ------------------------ | -------------------------------------- | ------------------------------ |
| Build/lint/test commands | `docs/agents/commands.md`              | Full command reference         |
| TypeScript/React style   | `docs/agents/code-style-typescript.md` | Import ordering, naming        |
| Rust style               | `docs/agents/code-style-rust.md`       | Edition 2021, MSRV 1.82        |
| State management         | `docs/agents/state-management.md`      | Three-layer onion model        |
| Architecture patterns    | `docs/agents/architecture.md`          | Event bridge, command system   |
| UI components (Radix)    | `docs/agents/ui-components.md`         | Focus management, IME          |
| Deep-dive docs           | `docs/developer/README.md`             | 26 technical references        |
| Zustand stores           | `src/store/AGENTS.md`                  | Store patterns and conventions |
| Component architecture   | `src/components/AGENTS.md`             | Barrel exports, feature dirs   |
| Rust backend             | `src-tauri/AGENTS.md`                  | Crates, commands, bindings     |

## Code Map

### High-Reference Symbols

| Symbol          | Location                   | Refs                     | Role                                   |
| --------------- | -------------------------- | ------------------------ | -------------------------------------- |
| `commands`      | `src/lib/bindings.ts`      | every store              | Type-safe Tauri IPC (auto-generated)   |
| `useUIStore`    | `src/store/ui-store.ts`    | layout + pages           | Navigation, sidebar, preferences state |
| `useModelStore` | `src/store/model-store.ts` | channels, models         | Model registry, defaults, favorites    |
| `cn()`          | `src/lib/utils.ts`         | nearly all components    | Tailwind class merging                 |
| `logger`        | `src/lib/logger.ts`        | stores, services         | Singleton logging                      |
| shadcn/ui       | `src/components/ui/`       | virtually all components | Primitives (don't modify)              |

### Hotspots (>500 LOC)

| LOC   | File                                              | Notes                       |
| ----- | ------------------------------------------------- | --------------------------- |
| 4,801 | `src-tauri/crates/droidgear-tui/src/tui/modal.rs` | TUI modal system            |
| 4,043 | `src-tauri/crates/droidgear-tui/src/ui.rs`        | TUI draw() function         |
| 3,429 | `src/lib/bindings.ts`                             | Auto-generated, DO NOT EDIT |
| 2,812 | `src-tauri/crates/droidgear-core/src/openclaw.rs` | OpenClaw business logic     |
| 1,835 | `src-tauri/crates/droidgear-core/src/pi.rs`       | Pi AI integration           |
| 1,759 | `src-tauri/crates/droidgear-core/src/hermes.rs`   | Hermes integration          |
| 1,155 | `src/components/models/ModelDialog.tsx`           | Model management dialog     |
| 1,026 | `src/components/droid/DroidSettingsPage.tsx`      | Core settings page          |

## Conventions

### TypeScript/React

- **Imports**: type imports with `type` keyword, `@/` path alias to `src/`
- **Components**: PascalCase filenames, barrel `index.ts` in each feature dir
- **Hooks**: `use-kebab-case.ts` (e.g., `use-platform.ts`)
- **Stores**: `kebab-case-store.ts` (e.g., `ui-store.ts`)
- **Formatting**: No semicolons, single quotes, 2-space indent, 80 char width
- **React Compiler**: No manual `useMemo`/`useCallback`/`React.memo`

### Zustand

- Selector syntax: `useStore(state => state.x)` — NEVER destructure
- `getState()` in callbacks to avoid subscriptions
- All stores use `devtools`; only `ui-store` and `terminal-store` use `persist`

### Rust

- **Command pattern**: `#[tauri::command] #[specta::specta]` → thin wrapper → core
- **Dual-function**: `fn foo_for_home(home_dir, ...)` + `fn foo(...)` for testability
- **Errors**: Core returns `Result<T, String>`, TUI uses `anyhow::Result`
- **After adding commands**: Run `npm run rust:bindings`

### i18n

- Translation keys: dotted paths (e.g., `commands.showLeftSidebar.label`)
- Interpolation: `{{variableName}}` mustache syntax
- Both `en.json` and `zh.json` must stay in sync (1,469 keys each)

## Anti-Patterns (This Project)

- ❌ Editing `src/lib/bindings.ts` or `src/lib/tauri-bindings.ts` — auto-generated
- ❌ String-based `invoke()` — use `commands` from tauri-bindings
- ❌ Destructuring Zustand stores — causes render cascades
- ❌ Store subscriptions in callbacks — use `getState()` instead
- ❌ Modifying `src/components/ui/` — shadcn/ui managed code
- ❌ Missing `onCloseAutoFocus` on dialogs — breaks IME
- ❌ Using pnpm — this project uses npm exclusively
- ❌ Tauri v1 patterns — v2 only

## Commands

```bash
npm run dev              # Vite dev server (port 1420)
npm run tauri:dev         # Full Tauri app with hot reload
npm run build            # tsgo + vite build
npm run check:all        # Full quality gate (typecheck + lint + ast + format + rust + tests)
npm run fix:all          # Auto-fix all linters
npm run test:run         # Vitest single run
npm run rust:test        # Cargo test (core + tui)
npm run rust:bindings    # Regenerate TypeScript bindings
npm run tui              # Run TUI (cargo run)
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
