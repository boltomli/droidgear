# TypeScript/React Code Style

Code style guidelines for TypeScript and React code in this project.

## Imports

Use type imports, path aliases, and group by external/internal:

```typescript
import { type ReactNode } from 'react' // Type imports with 'type' keyword
import { useTranslation } from 'react-i18next' // External packages first
import { useUIStore } from '@/store/ui-store' // Use @/ alias for src/
import { logger } from '@/lib/logger' // Internal modules
```

## Formatting (Prettier Enforced)

- No semicolons, single quotes, 2-space indent, 80 char line width
- Trailing commas in ES5 positions, arrow parens avoided when possible

Configuration in `prettier.config.js`.

## Naming Conventions

| Category    | Convention       | Example            |
| ----------- | ---------------- | ------------------ |
| Components  | `PascalCase`     | `MainWindow.tsx`   |
| Hooks       | `use-kebab-case` | `use-platform.ts`  |
| Stores      | `kebab-case`     | `ui-store.ts`      |
| Types       | `PascalCase`     | `type UserProfile` |
| Unused vars | `_` prefix       | `_unusedParam`     |

Always use `type` keyword for type imports:

```typescript
import { type ReactNode } from 'react'
```

## Error Handling

Tauri commands return a Result type. Always check the status:

```typescript
// Tauri commands return Result type
const result = await commands.loadPreferences()
if (result.status === 'ok') {
  return result.data
} else {
  logger.error('Failed to load preferences', result.error)
}
```

See `docs/developer/error-handling.md` for retry configuration and error display patterns.
