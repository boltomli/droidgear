# src/store — Zustand State Management

## OVERVIEW

16 Zustand stores following the `create<State>()(devtools(...))` pattern. Stores contain both state and actions — no separate action files.

## WHERE TO LOOK

| Task                   | Location                | Notes                                           |
| ---------------------- | ----------------------- | ----------------------------------------------- |
| UI navigation state    | `ui-store.ts`           | Only store with `persist` middleware            |
| Terminal instances     | `terminal-store.ts`     | Also uses `persist`                             |
| Model registry data    | `model-store.ts`        | Custom models, defaults, favorites              |
| Channel auth/tokens    | `channel-store.ts`      | Per-channel auth state                          |
| Tool-specific profiles | `*-store.ts`            | codex, openclaw, opencode, hermes, pi, omp, dsh |
| Connectivity tests     | `connectivity-store.ts` | Connection diagnostics                          |
| Export templates       | `export-store.ts`       | User-defined export configs                     |

## CONVENTIONS

- **Naming**: `use<Domain>Store` (e.g., `useChannelStore`, `useModelStore`)
- **File**: `<domain>-store.ts` (kebab-case)
- **Pattern**: `create<Type>()(devtools(...))` — ALL stores use devtools
- **Persist**: Only `ui-store` and `terminal-store` use `persist` middleware
- **Selectors**: ALWAYS use selector syntax: `useStore(state => state.x)` — NEVER destructure
- **getState()**: Use in callbacks/handlers to avoid subscriptions
- **Tests**: Co-located `*.test.ts` files

## ANTI-PATTERNS

- ❌ Destructuring stores: `const { x } = useStore()` — causes render cascades
- ❌ Store subscriptions in callbacks — use `getState()` instead
- ❌ Manual memoization — React Compiler handles it
