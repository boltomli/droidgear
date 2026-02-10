# State Management

State management patterns and critical rules for this project.

## Zustand Selector Pattern (CRITICAL — enforced by ast-grep)

```typescript
// ✅ GOOD: Selector syntax prevents render cascades
const leftSidebarVisible = useUIStore(state => state.leftSidebarVisible)

// ❌ BAD: Destructuring causes unnecessary re-renders
const { leftSidebarVisible } = useUIStore()
```

This rule is enforced by ast-grep and will fail `npm run ast:lint`.

## The `getState()` Pattern

Use `getState()` in callbacks to avoid store subscriptions:

```typescript
// ✅ GOOD: Use getState() in callbacks
const handleAction = () => {
  const { setData } = useStore.getState()
  setData(newData)
}
```

**When to use `getState()`:**

- In `useCallback` dependencies when you need current state but don't want re-renders
- In event handlers for accessing latest state without subscriptions
- In `useEffect` with empty deps when you need current state on mount only
- In async operations when state might change during execution

## Three-Layer State Onion

```
useState (component) → Zustand (global UI) → TanStack Query (persistent data)
```

| Layer          | Use For                                     |
| -------------- | ------------------------------------------- |
| `useState`     | Component-local UI state (dropdowns, etc.)  |
| Zustand        | Global UI state (panels, modes, navigation) |
| TanStack Query | Persistent data from Tauri backend / APIs   |

## React Compiler (Automatic Memoization)

This app uses React Compiler. You do **not** need to manually add:

- `useMemo` for computed values
- `useCallback` for function references
- `React.memo` for components

**Note:** The `getState()` pattern is still critical — it avoids store subscriptions, not memoization.

## Further Reading

- `docs/developer/state-management.md` — Full state management guide with store boundaries and adding new stores
