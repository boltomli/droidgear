# Build/Lint/Test Commands

Complete reference for all project commands.

> **Branch scope** — `spike/wails-v3-minimal` is Wails-only: the Tauri
> toolchain, the Rust workspace and the Rust TUI were removed (ADR-001).
> There is no `npm run tauri:*`, `npm run rust:*` or `npm run tui*` script
> anymore; Go checks live in the Wails section below.

## Development

```bash
npm run dev              # Start Vite dev server (frontend only)
```

## Build

```bash
npm run build            # TypeScript check + Vite build
npm run build:analyze    # Production build + bundle size report
```

## Quality Gates

```bash
# Run after significant changes (REQUIRED)
npm run check:all        # typecheck + lint + ast-grep + format:check + check:no-tauri + tests + build
```

## Individual Checks

```bash
npm run typecheck        # TypeScript type checking (tsgo / TS7)
npm run typecheck:ts6    # Same check with the stable tsc (TS6)
npm run lint             # ESLint (strict, zero warnings allowed)
npm run lint:fix         # ESLint with auto-fix
npm run format           # Prettier format all files
npm run format:check     # Check formatting without changes
npm run ast:lint         # ast-grep architecture rules
npm run ast:fix          # ast-grep with auto-fix
npm run check:no-tauri   # Fails if unbridged Tauri APIs creep into src/
```

## Testing — Frontend (Vitest)

```bash
npm run test             # Watch mode
npm run test:run         # Single run
npm run test:run -- src/hooks/use-platform.test.ts           # Single file
npm run test:run -- -t "should detect macOS"                 # Single test by name
npm run test:coverage    # Run with coverage
```

## CLI (`dg`)

The Go CLI is the primary non-GUI consumer of the service layer (ADR-001):

```bash
cd wails && go run ./cmd/dg version     # e.g. prints the build version
cd wails && go build ./cmd/dg           # build the dg binary
```

## Wails (Go shell)

Full verification order (bindings are gitignored — step 1 is mandatory
before any frontend build):

```bash
npm ci                                                             # sync deps (@wailsio/runtime etc.)
cd wails && wails3 generate bindings -clean=true -ts -i && cd ..   # gitignored, required pre-build
npm run build                                                   # → wails/frontend/dist (go:embed)
cd wails && go vet ./... && go test ./...
cd wails && wails3 task run     # run for real (embeds the built frontend)
cd wails && wails3 dev          # rebuild + restart on Go/frontend changes (no Vite HMR)
node scripts/check-command-surface.mjs          # command-surface drift
node scripts/check-go-layering.mjs              # service must not import wails; cmd/dg must not import internal/app
node wails/scripts/sync-version.mjs --check     # version drift
node wails/scripts/sync-registry.mjs            # shared-data vendoring (run after registry/schema edits)
```

**Known blind spot**: `npm run check:all` does NOT include the Wails frontend
production build, the Go steps, or the drift scripts above — run them
explicitly (CI covers them in `.github/workflows/wails.yml`).

## CI / Release tags

| Tag / trigger                              | Workflow             | What it runs                                                                                                                            |
| ------------------------------------------ | -------------------- | --------------------------------------------------------------------------------------------------------------------------------------- |
| `wails-v<version>-<n>` (**default**)       | `release-single.yml` | Single-file binaries (Windows/Linux/macOS GUI + `dg` CLI) as a **draft** release, built from the tagged commit — no `main` merge needed |
| `vX.Y.Z`                                   | `release.yml`        | Official release (changelog-driven draft + Windows NSIS installer). The Tauri jobs are gone; this pipeline is Wails-only                |
| pull request touching `wails/**`, `src/**` | `wails.yml`          | gofmt/vet/test, command-surface and layering drift, frontend bundle gates                                                               |

On this branch always use the `wails-v*` form, e.g.:

```bash
git tag wails-v1.3.9-4 && git push origin HEAD --tags
```

`release.yml` is **not** disabled in this repo — it still fires for `v*`
tags (only its Tauri jobs were removed in `fe22c57`), so avoid pushing a
`v*` tag unless you intend an official release.

## All Commands (Quick Reference)

| Category | Command                                     | Description                                        |
| -------- | ------------------------------------------- | -------------------------------------------------- |
| Dev      | `npm run dev`                               | Vite dev server (frontend only)                    |
| Build    | `npm run build`                             | TypeScript check + Vite build                      |
| Check    | `npm run check:all`                         | Full check suite                                   |
| Quality  | `npm run fix:all`                           | Auto-fix all fixable issues                        |
| Test     | `npm run test:run`                          | Vitest (single run)                                |
| CLI      | `cd wails && go run ./cmd/dg ...`           | dg CLI                                             |
| Wails    | `npm run build`                             | Frontend build into wails/frontend/dist (go:embed) |
| Wails    | `cd wails && go vet ./... && go test ./...` | Go static checks + tests                           |
| Wails    | `node scripts/check-command-surface.mjs`    | Command-surface drift check                        |
| Wails    | `node scripts/check-go-layering.mjs`        | Go layering drift check                            |
