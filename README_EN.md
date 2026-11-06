# DroidGear

[中文](README.md)

A desktop enhancement tool for [Codex](https://github.com/openai/codex) / [Factory Droid](https://factory.ai) / [Hermes Agent](https://hermes-agent.nousresearch.com) / [OMP (Oh My Pi)](https://github.com/can1357/oh-my-pi) / [Pi](https://pi.dev), with a second batch for [Claude](https://claude.ai) / [GitHub Copilot](https://github.com/features/copilot) / [Dsh (DeepSeek Harness)](https://deepseek.ai).

> Modules ship in batches: batch 1 `codex · droid · hermes · omp · pi`, batch 2 `claude · copilot · dsh`. `opencode` / `openclaw` are **deprecated** — the sidebar renders them last with a badge.

Supports custom AI models (BYOK), embedded terminal, session & specs management, MCP server configuration, and more.

## Installation

### macOS

Downloaded apps may be blocked by Gatekeeper since they are not signed by Apple. Run this command to fix:

```bash
xattr -cr /Applications/DroidGear.app
```

### Windows / Linux

Run the installer directly.

## Features

### Custom Model Management

- **Multi-Provider Support** - Configure models from Anthropic, OpenAI, or any Generic Chat Completion API
- **Visual Model Management** - Add, edit, delete, and reorder custom models with drag-and-drop
- **Model Favorites** - Favorite frequently used Droid models; the manager limits built-in models and preserves dialog order
- **Batch Operations** - Copy models, filter and batch delete, set default model
- **Model Registry** - Browse and search available AI models from a built-in registry in Preferences
- **API Model Discovery** - Fetch available models directly from provider APIs with auto-generated IDs and display names
- **Import/Export** - Support configuration import/export and batch management
- **Multi-Platform Support** - Support sub2api, antigravity, DeepSeek and other API proxy platforms
- **Ollama Support** - Ollama channel support with automatic local service detection
- **Provider Templates** - Built-in OpenAI, Gemini provider templates for quick setup

### Embedded Terminal

- **Built-in Terminal** - Integrated terminal with state save and restore
- **Custom Configuration** - Custom font, force dark mode
- **Convenient Operations** - Copy-on-select, OSC 9 notifications, derived sub-windows
- **External Terminal Launch** - Launch CLI tools via Ghostty or iTerm2 on macOS
- **Keyboard Shortcuts** - Cmd/Ctrl+1~0 to switch terminals, Cmd/Ctrl+W to close tabs, Cmd/Ctrl+Shift+[ to switch tabs
- **Code Snippets** - Snippets support on terminal pages

### Droid Session Management

- **Session Viewer** - View and manage Droid sessions with delete support
- **Multiple Views** - Toggle between list/grouped view, hide empty sessions
- **Follow Mode** - Session follow mode with thinking expansion toggle
- **Cloud Sync** - Cloud session sync toggle

### Droid Configuration Management

- **Multi-Settings Files** - Manage and switch between multiple Factory Droid configuration files (settings.json)
- **Link Local Settings** - Link a local JSON file as a launch settings profile (`--settings`); the file stays at its original location (GUI and TUI)
- **Trusted Folders** - View and manage Droid trusted folders in GUI and TUI
- **Terminal Preferences** - Set terminal preferences independently for each configuration file
- **Panel Refresh** - Automatically refresh relevant panels when switching configuration files

### Missions Management

- **Model Configuration** - Configure Worker model and Validation Worker model for Missions
- **Reasoning Effort** - Set reasoning effort independently for each model (none/low/medium/high/max), displays 1M context badge on model cards

### Specs File Management

- **File Browser** - View spec files in `~/.factory/specs` directory
- **Markdown Rendering** - Support Markdown format rendering
- **File Operations** - Rename, delete, save as, copy full path
- **Edit Mode** - Support spec selection and edit mode
- **Export** - Export spec items

### MCP Server Management

- **Presets** - Built-in MCP presets (including exa, etc.)
- **Server Management** - MCP server configuration management

#### Baizhi MCP tool bundle

The GUI MCP Quick Add card or `b` on the TUI MCP list opens a Baizhi confirmation. Selecting or cancelling does not save configuration or connect. Confirming saves a disabled HTTP entry with the fixed URL `https://agent-toolkit.app.baizhi.cloud/mcp` and the literal header `Authorization: Bearer ${BAIZHI_API_KEY}`. This preset flow does not accept a key, query BAIZHI_API_KEY or expand its value; saving reads the existing configuration to preserve other records.

Use the [Baizhi toolkit entry](https://agent-toolkit.app.baizhi.cloud/) for the separate account and fees. Set `BAIZHI_API_KEY` independently in the **environment that launches Factory**, then explicitly enable the server. Do not put a real key into the preset or URL. [Factory's MCP documentation](https://docs.factory.com/harness/mcp) says header environment references expand at connection time, an unset variable fails connection, and expanded values are not written back. These are documented behavior, not verification of a real Factory connection, model call, or GUI/shell environment inheritance.

Configuration remains ordinary `~/.factory/mcp.json`: this preset stores a nonsecret reference, not encrypted/keychain storage. The Baizhi writer includes `oauth: false` in the new raw JSON entry, as Factory recommends for header authentication. Enable/disable toggles preserve it. Saving through the existing generic editor may omit the unmodeled `oauth` field; inspect the configuration after edits. Actual Factory OAuth behavior has not been verified. When enabled, tool arguments go to Baizhi and returned content may enter your selected AI model. Disabling/deleting the local entry does not revoke a key, remote task, or charges.

### Codex Support

- **Codex CLI Integration** - Manage Codex configuration profiles
- **Auth Profiles** - Save and switch between official login and BYOK auth profiles with conflict detection
- **Instant Apply** - Edits to the currently active profile (including provider switches) apply immediately, no explicit apply needed
- **Model Catalog Sync** - Applying DeepSeek V4 / MiMo models automatically syncs per-family catalogs under `~/.codex/model-catalogs/` (with `model_catalog_json` switched per family)

- **Configuration Management** - Load and save auth/config.toml (`~/.codex`)
- **Management Pages** - MCP servers / sessions / terminal subpages under Codex (sessions page supports project-grouped browsing, thinking streams with Markdown rendering, deletion and provider reassignment)

### Hermes Agent Support

- **Configuration Management** - Hermes Agent YAML profile management
- **Reasoning Effort** - Set reasoning effort independently for each Hermes profile (none/minimal/low/medium/high/xhigh/max/ultra)
- **Channel Import** - Import Hermes Agent configuration from channels (GUI, TUI `i`, and `dg hermes providers import` share one rule set: add `/v1`, write the v12 `providers` entry, and make it the default model)
- **Provider Request Options** - Edit `extra_headers`, `session_affinity_header`, `discover_models`, `transport`, `capabilities`, plus per-model `context_length` / `prompt_caching` / `answer_in_reasoning` and `model.api_mode` / `model.streaming`

### OMP (Oh My Pi) Support

- **Custom Providers** - Read and write custom providers and models in the official `~/.omp/agent/models.yml` (GUI and TUI CRUD, plus `dg omp providers`), update models and providers from a channel, and preserve unrelated providers and unknown fields
- **Model Roles** - Manage the 15 upstream model roles in `~/.omp/agent/config.yml` (default/smol/slow/vision/plan/commit/tiny/memory/task/advisor + image/web/speech/dictation/judge), and load them from the live config; each input has a picker listing the `provider/model-id` values from `models.yml` plus OMP's cache (same list as `dg omp roles candidates`). **Roles follow the write**: deleting a provider or shrinking a model list re-points roles that would dangle at the first available combination (built-in providers known to the models.db cache are left alone), clears them when nothing is left, resets `defaultThinkingLevel` to `auto` when the new default model cannot express it (per-model `thinking.efforts`), and explains every change in the GUI toast, the TUI preview and the `dg` receipt; `omp profiles apply` warns about unresolvable roles instead of rewriting the user's choice
- **Session History** - Browse `~/.omp/agent/sessions` JSONL logs (GUI): list or project-grouped views, active-branch rendering with thinking blocks, follow new messages, hide empty sessions, and delete. The list loads in batches as you scroll

### Pi Support

- **Pi Coding Agent Integration** - Pi (pi.dev) custom model configuration management
- **Provider/Model Management** - Support configuring multiple providers and their models (baseUrl, api, apiKey, headers, compat, etc.)
- **Connection Test** - Test Pi provider connectivity
- **Profile Management** - Multi-profile support with one-click apply to `~/.pi/agent/models.json`
- **Default Model** - Pin the startup `defaultProvider` / `defaultModel` / `defaultThinkingLevel` in `~/.pi/agent/settings.json` (GUI default-model card, TUI Pi → Defaults, and `dg pi settings show|set|reset|candidates` share one path); every other settings key is preserved, and empty fields hand the choice back to Pi. **The default model follows the profile**: after an apply, a startup model that models.json no longer supplies (catalog models are left alone) is re-pointed at the profile's first provider/model, with the reason shown in the GUI toast, the TUI preview and the `dg pi profiles apply` receipt
- **Live Config Reading** - Load existing configuration from Pi's live config
- **Channel Import** - Import a provider from a channel (GUI, TUI `i`, and `dg pi providers import` share one rule set: normalise the base URL, add `/v1`, enrich models from the registry, and set `compat.forceAdaptiveThinking` when required)
- **Registry-Driven Models** - Enrich Pi provider models from the built-in model registry, with provider-neutral thinking level mapping. Models whose Anthropic profile is `anthropic-adaptive` (such as `claude-opus-5-5`) get `compat.forceAdaptiveThinking`; an explicit opt-out already stored on the model is kept
- **Advanced Model Options** - One JSON block per model edits `inputLimits`, `promptCache`, `samplingParams`, and `samplingParamsByThinkingLevel` (proxy-side request tuning, so the dialog does not grow a dozen low-frequency fields)

### Claude Support

- **Claude Settings File Management** - Switch and edit multiple settings files (Global + custom) with structured Provider / Model / Reasoning / Thinking / General / Permissions sections
- **Settings File Actions** - Create, delete, duplicate, load from live config, merge to global, preview changes, and run once temporarily
- **Set Active & Dirty Tracking** - Set a settings file as the active one, with unsaved-change dirty tracking (TUI)
- **Merge Apply** - Shallow-merge env and permissions from a settings file into the global settings
- **Import from Channel** - One-click import of provider configuration (base URL, API key, default model) from configured channels
- **Fetch Models** - Load model IDs from the configured provider with the bearer token and pick the main or small model from the list, while still allowing a manual ID (GUI + TUI)
- **Auto-compact Window** - Set the root `autoCompactWindow` (100,000–1,000,000 tokens) in the GUI and TUI. Leave it unset and Claude Code picks a window per model; a per-model window saved with `/autocompact`, or `CLAUDE_CODE_AUTO_COMPACT_WINDOW` in the environment, still takes precedence
- **Runtime Core** - Settings loaded via a runtime overlay at launch so the live configuration is never mutated
- **GUI/TUI Parity** - Desktop and TUI versions share the same Claude settings features and semantics (an empty small model uses Claude's default Haiku 5.5; mirroring the main model is explicit)

### GitHub Copilot Support

- **BYOK Profile Management** - Manage GitHub Copilot BYOK profiles (OpenAI / Anthropic providers, model, and token limits) with create, duplicate, delete, apply, and load-applied operations
- **Channel Import** - Import providers and models from channels, normalizing base URLs automatically (add `/v1` for OpenAI, strip `/v1` for Anthropic)
- **Official Subscription Mode** - Clear BYOK overrides and use the Copilot CLI's existing login or auth token
- **Temporary Run** - Launch Copilot with the selected profile in one click (process-scoped environment variables plus an explicit `--model` argument)
- **Standalone Launcher** - Ships `copilot-local.sh` / `copilot-local.cmd` launchers to run Copilot with an exported profile without the GUI

### Dsh (DeepSeek Harness) Support

- **Provider Configuration** - Manage `llm-pi-ai.providers` in the modern multi-profile DSH `cordis.patch.yml` (displayName, baseURL, apiKeyEnv, api, compat, model list): supports the official `desktop` / `web` runtime profiles (default `desktop`), preserves other patch entries and `!!js` expressions on write; falls back to the legacy `~/.dsh/settings.yaml` without a profile, and merges legacy → profile patch → home patch on read
- **API Key Values** - Configure API key values in `~/.dsh/.credentials.yaml` (0600 permissions). A provider's credential reference defaults to `<PROVIDER_ID>_API_KEY` and can be overridden to reuse an existing environment variable; the stored value wins over the environment and `.env`
- **Official DeepSeek API Key** - On the Models tab and in the TUI (`o`), set the official key for users who do not sign in through DSH. The value is stored for `llm-deepseek.apiKeyEnv` (default `DEEPSEEK_API_KEY`); clearing it falls back to the environment
- **Visual Configuration** - The composed desktop / web profile is edited across seven tabs: overview, models, general, agent, network, advanced, and diagnostics. In the TUI, `c` inspects and edits fields and toggles an entry's disabled state. Writes go to the profile `cordis.patch.yml`, keep comments and `!!js`, redact secrets, and use a revision hash so a concurrent edit is not overwritten
- **Default Reasoning and Default Model** - A pi-ai route can set a default thinking level (`off` through `max`, suggested as High when every model on the route supports it). Marking a model as the default writes `agent-default-model` (TUI: `g`)
- **Model Input Modality** - Show and edit per-model text / image input. Until pinned, image follows the registry suggestion, so a custom route is not treated as text-only by DSH
- **Channel Import** - Import providers from channels (auto-fills ID, base URL, key, and API type)
- **Model Fetching** - Fetch the model list from the provider API after configuring the URL and key, with multi-select before adding
- **Session History** - Browse `~/.dsh/sessions` logs (GUI + TUI): list or project-grouped views, thinking blocks, live follow, hide-empty and delete; consecutive reasoning-only steps collapse into one expandable "Thinking process" block
- **Registry Auto-Adaptation** - Auto-fill model name, context window, max tokens, and reasoning efforts from the registry by model ID

### OpenCode Support (deprecated)

> **Deprecated**: the OpenCode module receives no new features; it still reads/writes existing configuration. New work lands in the batch 1 / batch 2 modules. The sidebar renders it last with a "deprecated" badge.

- **AI Development Integration** - OpenCode tool integration
- **Configuration Management** - Load and save provider/auth configurations

### OpenClaw Support (deprecated)

> **Deprecated**: the OpenClaw module receives no new features; it still reads/writes existing configuration. New work lands in the batch 1 / batch 2 modules. The sidebar renders it last with a "deprecated" badge.

- **AI Development Integration** - OpenClaw tool integration
- **Configuration Management** - Load and save provider configurations
- **Installation Helper** - Built-in installation commands for macOS/Linux/Windows

### Other Features

- **Self-update (single-file releases)** - Version check, SHA-256 verification, download progress and one-click restart (`dg update` offers the same from the CLI)
- **Window State** - Save and restore window state
- **Exit Protection** - Warns before closing with unsaved changes
- **Cross-Platform** - Works on macOS, Windows, and Linux

## TUI Version (Headless Terminal)

The DroidGear TUI is a terminal interface designed for SSH and headless
environments. It shares the exact same `internal/service` configuration layer
with the desktop app (one implementation, identical behavior). Feature parity
is tracked by the matrix in
[docs/developer/tui-parity.zh.md](docs/developer/tui-parity.zh.md) §2.

### Installation

Download the `dg` binary from the `wails-v*` releases on
[Releases](https://github.com/Sunshow/droidgear/releases) — `dg` carries both
the interactive TUI and the scriptable CLI subcommands:

- Windows x64: `dg-windows-x64.exe`
- macOS arm64: `dg-macos-arm64`
- Linux x64: `dg-linux-x64`

Or build from source (pure Go, no desktop dependencies):

```bash
cd wails && CGO_ENABLED=0 go build ./cmd/dg
```

### Running

```bash
# Open the interactive TUI (the default terminal entry point)
dg tui

# Scriptable headless layer: --json output, --dry-run previews, exit codes
dg path show
dg codex profiles list
dg mcp list
```

### Modules (mirroring the GUI sidebar)

Order matches the GUI sidebar: **batch 1** → **batch 2** → Preferences.

- **Channels**: channel detail + Export templates (sections)
- **Batch 1**
  - **Codex**: Providers · Auth · Sessions (sections)
  - **Droid**: Settings · Missions · Trusted · Auth · Specs · MCP · Models · Sessions · Legacy
  - **Hermes**: Model / profile lists
  - **OMP**: Config + Providers + Sessions (sections)
  - **Pi**: Providers · Defaults + Sessions (sections)
- **Batch 2**
  - **Claude**: settings files / model / profile lists
  - **Copilot**: profile list
  - **Dsh**: Desktop · Web · Sessions (sections)
- **Paths / Settings**: Preferences path overrides and general/appearance items

> `opencode` / `openclaw` are deprecated and have no TUI screen.

### Basic Keys

- `↑↓` / `jk`: move · `Enter`: open · `Esc` / `q`: back · `Ctrl+C`: quit
- `Tab` / `Shift+Tab` / `1..9`: switch sections
- `r`: reveal/hide secrets (redacted by default)
- Writes (always through a ChangeSet preview that shares the write path):
  `e` edit path, `a` apply/switch/activate, `d` delete, `t` toggle/cycle;
  `y` confirms, `n` cancels — if a file changed since the preview you get a
  conflict prompt, where `y` forces the write (`--force` equivalent)

### Configuration Files

The TUI shares configuration files with the desktop version:

- Factory config: `~/.factory/settings.json`
- MCP config: `~/.factory/mcp.json`
- DroidGear config: `~/.droidgear/`
- Codex config: `~/.codex/`
- OpenCode config: `~/.config/opencode/`
- OpenClaw config: `~/.openclaw/`
- Hermes config: `~/.hermes/`
- Pi config: `~/.pi/agent/`

For design documentation see
[docs/developer/tui-parity.zh.md](docs/developer/tui-parity.zh.md) and
[docs/developer/wails-tui-cli-design.zh.md](docs/developer/wails-tui-cli-design.zh.md)
(the historical ChangeSet draft lives in
[docs/developer/tui-design.md](docs/developer/tui-design.md)).

## Configuration

DroidGear reads and writes to `~/.factory/settings.json`:

```json
{
  "customModels": [
    {
      "model": "your-model-id",
      "displayName": "My Custom Model",
      "baseUrl": "https://api.provider.com/v1",
      "apiKey": "YOUR_API_KEY",
      "provider": "generic-chat-completion-api",
      "maxOutputTokens": 16384
    }
  ]
}
```

### Supported Providers

| Provider    | Value                         |
| ----------- | ----------------------------- |
| Anthropic   | `anthropic`                   |
| OpenAI      | `openai`                      |
| Generic API | `generic-chat-completion-api` |

## Development

### Prerequisites

- Node.js 20+
- Go 1.27+ and the Wails v3 CLI: `go install github.com/wailsapp/wails/v3/cmd/wails3@v3.0.0`
- Runtime: WebView2 on Windows (preinstalled on Windows 10/11); GTK3 + WebKitGTK 4.1 on Linux

### Setup

```bash
npm install
cd wails && wails3 generate bindings -clean=true -ts -i && cd ..   # first run: TS bindings
npm run dev                          # frontend only (Vite dev server)
cd wails && wails3 task dev          # full app (rebuilds on Go/TS changes)
```

### Build

```bash
npm run build                        # typecheck + frontend → wails/frontend/dist
cd wails && wails3 task build        # production build → wails/bin/droidgear-minimal.exe
```

## Tech Stack

- **Frontend**: React 19, TypeScript, Vite, Tailwind CSS, shadcn/ui
- **Backend**: Wails v3 (Go)
- **State**: Zustand

## Privacy

DroidGear values your privacy. Your username, password, API keys, and other sensitive data are stored locally on your device only and are never uploaded to any server.

## Changelog

See full changelog at [CHANGELOG.md](CHANGELOG.md)

## Acknowledgements

This project is based on [tauri-template](https://github.com/dannysmith/tauri-template) by Danny Smith. Thanks for the excellent template!

## License

[MIT](LICENSE.md)
