# Rust Code Style

Code style guidelines for Rust code in this project.

## Basics

- **Edition**: 2021
- **MSRV**: 1.82
- All warnings treated as errors via clippy

## Formatting

Use modern inline formatting:

```rust
// ✅ GOOD
format!("{variable}")

// ❌ BAD
format!("{}", variable)
```

## Clippy

Clippy runs with warnings as errors. Fix all warnings before committing:

```bash
npm run rust:clippy      # Check
npm run rust:clippy:fix  # Auto-fix
```

## Type-Safe Bindings (tauri-specta)

Use `tauri-specta` for type-safe command bindings between Rust and TypeScript:

```bash
npm run rust:bindings    # Regenerate TypeScript bindings
```

After adding or modifying Rust commands, always regenerate bindings.

## Further Reading

- `docs/developer/rust-architecture.md` — Rust module organization and patterns
- `docs/developer/tauri-commands.md` — Adding new Tauri commands
