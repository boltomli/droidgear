# Build/Lint/Test Commands

Complete reference for all project commands.

## Development

```bash
npm run dev              # Start Vite dev server (frontend only)
npm run tauri:dev        # Start full Tauri app with hot reload
```

## Build

```bash
npm run build            # TypeScript check + Vite build
npm run tauri:build      # Full Tauri production build
```

## Quality Gates

```bash
# Run after significant changes (REQUIRED)
npm run check:all        # All checks: typecheck, lint, ast-grep, format, rust checks, tests
```

## Individual Checks

```bash
npm run typecheck        # TypeScript type checking
npm run lint             # ESLint (strict, zero warnings allowed)
npm run lint:fix         # ESLint with auto-fix
npm run format           # Prettier format all files
npm run format:check     # Check formatting without changes
npm run ast:lint         # ast-grep architecture rules
npm run ast:fix          # ast-grep with auto-fix
```

## Testing — Frontend (Vitest)

```bash
npm run test             # Watch mode
npm run test:run         # Single run
npm run test:run -- src/hooks/use-platform.test.ts           # Single file
npm run test:run -- -t "should detect macOS"                 # Single test by name
npm run test:run -- src/store/ui-store.test.ts -t "toggle"   # File + test name
npm run test:coverage    # With coverage report
```

## Testing — Rust

```bash
npm run rust:test        # Run all Rust tests
npm run rust:fmt         # Format Rust code
npm run rust:fmt:check   # Check Rust formatting
npm run rust:clippy      # Rust linter (warnings = errors)
npm run rust:bindings    # Regenerate tauri-specta TypeScript bindings
```
