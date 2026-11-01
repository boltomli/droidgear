//! Specs management commands (Tauri wrappers + watcher).
//!
//! CRUD logic lives in `droidgear-core`. The watcher remains in the Tauri layer.

use notify::{Config, RecommendedWatcher, RecursiveMode, Watcher};
use std::path::PathBuf;
use std::sync::Mutex;
use tauri::{AppHandle, Emitter, Manager};

pub use droidgear_core::specs::SpecFile;

fn specs_dir() -> Result<PathBuf, String> {
    Ok(droidgear_core::paths::get_factory_home()?.join("specs"))
}

/// Lists all spec files from ~/.factory/specs directory.
#[tauri::command]
#[specta::specta]
pub async fn list_specs() -> Result<Vec<SpecFile>, String> {
    droidgear_core::specs::list_specs()
}

/// Renames a spec file.
#[tauri::command]
#[specta::specta]
pub async fn rename_spec(old_path: String, new_name: String) -> Result<SpecFile, String> {
    droidgear_core::specs::rename_spec(&old_path, &new_name)
}

/// Deletes a spec file.
#[tauri::command]
#[specta::specta]
pub async fn delete_spec(path: String) -> Result<(), String> {
    droidgear_core::specs::delete_spec(&path)
}

/// Updates a spec file content.
#[tauri::command]
#[specta::specta]
pub async fn update_spec(path: String, content: String) -> Result<SpecFile, String> {
    droidgear_core::specs::update_spec(&path, &content)
}

/// State for the specs file watcher
pub struct SpecsWatcherState(pub Mutex<Option<RecommendedWatcher>>);

/// Starts watching the specs directory for changes.
#[tauri::command]
#[specta::specta]
pub async fn start_specs_watcher(app: AppHandle) -> Result<(), String> {
    let specs_dir = specs_dir()?;

    if !specs_dir.exists() {
        std::fs::create_dir_all(&specs_dir)
            .map_err(|e| format!("Failed to create specs directory: {e}"))?;
    }

    let app_handle = app.clone();

    let watcher = RecommendedWatcher::new(
        move |res: Result<notify::Event, notify::Error>| {
            if let Ok(event) = res {
                use notify::EventKind;
                match event.kind {
                    EventKind::Create(_) | EventKind::Modify(_) | EventKind::Remove(_) => {
                        let _ = app_handle.emit("specs-changed", ());
                    }
                    _ => {}
                }
            }
        },
        Config::default(),
    )
    .map_err(|e| format!("Failed to create watcher: {e}"))?;

    let state = app.state::<SpecsWatcherState>();
    let mut guard = state.0.lock().map_err(|e| format!("Lock error: {e}"))?;

    if let Some(mut old_watcher) = guard.take() {
        let _ = old_watcher.unwatch(&specs_dir);
    }

    let mut watcher = watcher;
    watcher
        .watch(&specs_dir, RecursiveMode::NonRecursive)
        .map_err(|e| format!("Failed to watch directory: {e}"))?;

    *guard = Some(watcher);
    Ok(())
}

/// Stops watching the specs directory.
#[tauri::command]
#[specta::specta]
pub async fn stop_specs_watcher(app: AppHandle) -> Result<(), String> {
    let specs_dir = specs_dir()?;
    let state = app.state::<SpecsWatcherState>();
    let mut guard = state.0.lock().map_err(|e| format!("Lock error: {e}"))?;

    if let Some(mut watcher) = guard.take() {
        let _ = watcher.unwatch(&specs_dir);
    }

    Ok(())
}
