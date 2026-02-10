# Architecture Patterns

Key architecture patterns for this project.

## Event-Driven Bridge

Communication between Rust backend and React frontend:

- **Rust → React**: `app.emit("event-name", data)` → `listen("event-name", handler)`
- **React → Rust**: Use typed commands from `@/lib/tauri-bindings`

## Tauri Commands (tauri-specta)

Always use the typed commands, never raw `invoke`:

```typescript
import { commands } from '@/lib/tauri-bindings'
const result = await commands.loadPreferences() // ✅ Type-safe
// NOT: await invoke('load_preferences')         // ❌ No type safety
```

After adding or modifying Rust commands, regenerate bindings:

```bash
npm run rust:bindings
```

## i18n

```typescript
import { useTranslation } from 'react-i18next'
const { t } = useTranslation()
return <h1>{t('myFeature.title')}</h1>
```

- Translations live in `/locales/*.json`
- Use CSS logical properties for RTL support

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
docs/developer/          # Architecture documentation
docs/agents/             # Agent coding guidelines (this directory)
```

## Further Reading

- `docs/developer/architecture-guide.md` — High-level overview, mental models, system architecture
- `docs/developer/tauri-commands.md` — Type-safe Rust-TypeScript bridge details
- `docs/developer/command-system.md` — Unified action dispatch, command registration
- `docs/developer/i18n-patterns.md` — Translation system, RTL support
