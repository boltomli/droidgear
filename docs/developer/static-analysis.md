# Static Analysis

All static analysis tools configured in this app and how to use them.

## Quick Reference

| Tool           | Purpose                       | Command                  | In check:all |
| -------------- | ----------------------------- | ------------------------ | ------------ |
| TypeScript     | Type checking                 | `npm run typecheck`      | Yes          |
| Biome          | exhaustive-deps + import type | `npm run biome:lint`     | Yes          |
| ast-grep       | Code quality + rules          | `npm run ast:lint`       | Yes          |
| Prettier       | Code formatting               | `npm run format:check`   | Yes          |
| React Compiler | Automatic memoization         | Build-time               | Yes          |
| cargo fmt      | Rust formatting               | `npm run rust:fmt:check` | Yes          |
| clippy         | Rust linting                  | `npm run rust:clippy`    | Yes          |
| Vitest         | Frontend tests                | `npm run test:run`       | Yes          |
| cargo test     | Rust tests                    | `npm run rust:test`      | Yes          |
| knip           | Unused code detection         | `npm run knip`           | No           |
| jscpd          | Duplicate code detection      | `npm run jscpd`          | No           |

## Running All Checks

```bash
npm run check:all    # Must pass before commits
npm run fix:all      # Auto-fix what can be fixed
```

## Tool Details

### TypeScript (tsgo, TS7)

Type checking via the Go-based TypeScript compiler.

```bash
npm run typecheck    # Check types
npm run build        # Type check + Vite build
```

Configuration in `tsconfig.json`. Strict mode enabled with all recommended checks.

### Biome

Enforces React hooks dependency correctness and TypeScript import type consistency.

```bash
npm run biome:lint    # Check for issues
npm run biome:fix     # Auto-fix issues
```

**Key rules:**

- `useExhaustiveDependencies` — correct dependency arrays for useEffect/useMemo/useCallback
- `useImportType` — enforce `import { type X }` for type-only imports

Configuration in `biome.json`. Only these two rules are enabled; formatting is handled by Prettier.

### ast-grep

Enforces code quality rules and architectural patterns. Catches violations like explicit `any` types, Zustand destructuring, and hooks in wrong directories.

```bash
npm run ast:lint    # Scan for violations
npm run ast:fix     # Auto-fix where possible
```

**Key rules:**

- No explicit `any` types (use `unknown` or generics)
- No Zustand destructuring (causes render cascades)
- Hooks must be in `hooks/` directory
- No store subscriptions in `lib/`

See [writing-ast-grep-rules.md](./writing-ast-grep-rules.md) for creating new rules.

### Prettier

Consistent code formatting.

```bash
npm run format:check   # Check formatting
npm run format         # Fix formatting
```

Configuration in `prettier.config.js`.

### React Compiler

Handles memoization automatically at build time. You do **not** need to manually add:

- `useMemo` for computed values
- `useCallback` for function references
- `React.memo` for components

The compiler analyzes code and adds memoization where beneficial.

**Note:** The `getState()` pattern is still critical - it avoids store subscriptions, not memoization. See [state-management.md](./state-management.md).

### Rust Tooling

```bash
npm run rust:fmt:check   # Check formatting
npm run rust:fmt         # Fix formatting
npm run rust:clippy      # Lint with clippy
npm run rust:clippy:fix  # Auto-fix clippy warnings
npm run rust:test        # Run Rust tests
```

### knip (Periodic Cleanup)

Detects unused exports, dependencies, and files. Not in `check:all` - use periodically.

```bash
npm run knip
```

### jscpd (Periodic Cleanup)

Detects duplicated code blocks. Not in `check:all` - use periodically.

```bash
npm run jscpd
```

Use the `/cleanup` command for guided analysis and cleanup of both knip and jscpd findings.

## CI Integration

`check:all` runs in CI. Ensure it passes locally before pushing:

```bash
npm run check:all
```

## Adding New Rules

**Biome:** Modify `biome.json`

**ast-grep:** Create YAML files in `.ast-grep/rules/`. See [writing-ast-grep-rules.md](./writing-ast-grep-rules.md).

**Prettier:** Modify `prettier.config.js`
