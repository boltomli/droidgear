# src/components — React Component Architecture

## OVERVIEW

175 component files across 21 feature subdirectories. Each subfolder has a barrel `index.ts` with named exports. `ui/` contains shadcn/ui primitives — don't modify.

## STRUCTURE

```
components/
├── ui/              # shadcn/ui (DO NOT modify)
├── layout/          # MainWindow, sidebars, content area
├── titlebar/        # Window controls per platform
├── preferences/     # Settings dialog + panes
├── command-palette/ # Cmd+K command palette
├── models/          # Model registry UI
├── channels/        # Channel management
├── droid/           # Core DroidGear features
├── claude/          # Claude integration
├── codex/           # Codex integration
├── opencode/        # OpenCode integration
├── openclaw/        # OpenClaw integration
├── hermes/          # Hermes integration
├── pi/              # Pi AI integration
├── omp/             # OMP integration
├── dsh/             # DSH feature
├── export/          # Export templates
├── factory-auth/    # Factory authentication
└── updates/         # Auto-update UI
```

## COMPONENT MAP

| Directory          | Files   | Purpose                                                                                                            |
| ------------------ | ------- | ------------------------------------------------------------------------------------------------------------------ |
| `ui/`              | 45      | shadcn/ui primitives — Dialog, Select, Tabs, etc. DO NOT modify                                                    |
| `layout/`          | 5       | MainWindow, LeftSideBar, RightSideBar, MainWindowContent                                                           |
| `titlebar/`        | 7       | TitleBar + platform-specific window controls (MacOS, Windows, Linux)                                               |
| `preferences/`     | 3+panes | PreferencesDialog with panes: General, Appearance, Models, Paths, Advanced, About                                  |
| `command-palette/` | 2       | Cmd+K command palette system                                                                                       |
| `models/`          | 11      | ModelList, ModelCard, ModelDialog (1,155 LOC), ModelConfigPage, ConnectivityPanel, BatchModelSelector              |
| `channels/`        | 10      | ChannelList, ChannelDialog, ChannelDetail, KeyList, ChannelModelPicker, Export/Import                              |
| `droid/`           | 16      | Core settings: DroidSettingsPage, TerminalPage, McpPage, SessionsPage, SpecsPage, MissionsPage, TrustedFoldersPage |
| `claude/`          | 4       | ClaudeFeatureList, ClaudeSettingsPage (718 LOC), ImportFromChannelDialog                                           |
| `codex/`           | 8       | CodexConfigPage (779 LOC), CodexAuthPage, ProviderCard/Dialog, ConfigStatus                                        |
| `opencode/`        | 9       | OpenCodeConfigPage (566 LOC), ProviderCard/Dialog, ImportDialog                                                    |
| `openclaw/`        | 9       | OpenClawConfigPage (675 LOC), HelpersPage, SubagentsPage, ProviderCard/Dialog                                      |
| `hermes/`          | 5       | HermesConfigPage (716 LOC), FeatureList, ImportFromChannelDialog                                                   |
| `pi/`              | 9       | PiConfigPage (568 LOC), ProviderDialog (851 LOC), PiModelRegistryDialog                                            |
| `omp/`             | 5       | OmpConfigPage, FeatureList, ProviderCard                                                                           |
| `dsh/`             | 6       | DshConfigPage, ProviderDialog (700 LOC)                                                                            |
| `export/`          | 2       | ExportTemplateDialog (561 LOC), ExportTemplatesPage                                                                |
| `factory-auth/`    | 2       | FactoryAuthPage                                                                                                    |
| `updates/`         | 2       | UpdateNotificationContent                                                                                          |

### Tool Integration Pattern

Each AI tool (Claude, Codex, OpenClaw, OpenCode, Hermes, Pi, OMP, DSH) follows the same component pattern:

- `FeatureList.tsx` — Tool feature toggle list
- `ConfigPage.tsx` — Full configuration page
- `ProviderCard.tsx` — Provider display card
- `ProviderDialog.tsx` — Provider add/edit dialog
- `ConfigStatus.tsx` — Config file status indicator
- Optional: `ImportFromChannelDialog.tsx`, `AuthPage.tsx`

## WHERE TO LOOK

| Task                 | Location                | Notes                                                                |
| -------------------- | ----------------------- | -------------------------------------------------------------------- |
| App shell/layout     | `layout/MainWindow.tsx` | Top-level orchestrator                                               |
| Window controls      | `titlebar/`             | Platform-specific (mac/win/linux)                                    |
| Settings UI          | `preferences/`          | Dialog + panes (General, Appearance, Models, Paths, Advanced, About) |
| Model management     | `models/`               | ModelList, ModelDialog (1,155 LOC), ModelConfigPage                  |
| Tool config pages    | `<tool>/` folders       | Each has ConfigPage, FeatureList, ProviderCard/Dialog                |
| Shared UI primitives | `ui/`                   | shadcn/ui — don't modify                                             |

## CONVENTIONS

- **Barrel exports**: Each subfolder has `index.ts` with named exports only
- **PascalCase filenames** for components: `ModelDialog.tsx`
- **Imports**: `@/components/ui/` (shadcn), `@/store/`, `@/lib/`, `@/hooks/`
- **Error boundary**: `ErrorBoundary.tsx` class component wraps the app
- **ThemeProvider**: Context at root, exported from `@/lib/theme-context`
- **Radix focus**: Use `onCloseAutoFocus={e => e.preventDefault()}` for dialogs with IME
- **ResizableDialog**: Always pass `onCloseAutoFocus` to prevent double-click issues
- **Tests**: Co-located `*.test.tsx` files, 27 total

## ANTI-PATTERNS

- ❌ Modifying `ui/` components — they're shadcn/ui managed code
- ❌ Default exports in barrel files — named exports only
- ❌ Missing `onCloseAutoFocus` on dialogs with inputs — breaks IME
