//! Sessions management commands (Tauri wrappers + watcher).
//!
//! Listing/parsing logic lives in `droidgear-core`. The watcher remains in the Tauri layer.

use notify::{Config, RecommendedWatcher, RecursiveMode, Watcher};
use std::path::PathBuf;
use std::sync::Mutex;
use tauri::{AppHandle, Emitter, Manager};

pub use droidgear_core::sessions::{SessionDetail, SessionProject, SessionSummary};

fn sessions_dir() -> Result<PathBuf, String> {
    Ok(droidgear_core::paths::get_factory_home()?.join("sessions"))
}

/// Lists all session projects from ~/.factory/sessions directory.
#[tauri::command]
#[specta::specta]
pub async fn list_session_projects() -> Result<Vec<SessionProject>, String> {
    droidgear_core::sessions::list_session_projects()
}

/// Lists all sessions, optionally filtered by project.
#[tauri::command]
#[specta::specta]
pub async fn list_sessions(project: Option<String>) -> Result<Vec<SessionSummary>, String> {
    droidgear_core::sessions::list_sessions(project.as_deref())
}

/// Gets detailed session information including messages.
#[tauri::command]
#[specta::specta]
pub async fn get_session_detail(session_path: String) -> Result<SessionDetail, String> {
    droidgear_core::sessions::get_session_detail(&session_path)
}

/// Deletes a session by removing its .jsonl and .settings.json files.
#[tauri::command]
#[specta::specta]
pub async fn delete_session(session_path: String) -> Result<(), String> {
    droidgear_core::sessions::delete_session(&session_path)
}

/// State for the sessions file watcher
pub struct SessionsWatcherState(pub Mutex<Option<RecommendedWatcher>>);

/// Starts watching the sessions directory for changes.
#[tauri::command]
#[specta::specta]
pub async fn start_sessions_watcher(app: AppHandle) -> Result<(), String> {
    let sessions_dir = sessions_dir()?;

    if !sessions_dir.exists() {
        return Ok(());
    }

    let app_handle = app.clone();

    let watcher = RecommendedWatcher::new(
        move |res: Result<notify::Event, notify::Error>| {
            if let Ok(event) = res {
                use notify::EventKind;
                match event.kind {
                    EventKind::Create(_) | EventKind::Modify(_) | EventKind::Remove(_) => {
                        let _ = app_handle.emit("sessions-changed", ());
                    }
                    _ => {}
                }
            }
        },
        Config::default(),
    )
    .map_err(|e| format!("Failed to create watcher: {e}"))?;

    let state = app.state::<SessionsWatcherState>();
    let mut guard = state.0.lock().map_err(|e| format!("Lock error: {e}"))?;

    if let Some(mut old_watcher) = guard.take() {
        let _ = old_watcher.unwatch(&sessions_dir);
    }

    let mut watcher = watcher;
    watcher
        .watch(&sessions_dir, RecursiveMode::Recursive)
        .map_err(|e| format!("Failed to watch directory: {e}"))?;

    *guard = Some(watcher);
    Ok(())
}

/// Stops watching the sessions directory.
#[tauri::command]
#[specta::specta]
pub async fn stop_sessions_watcher(app: AppHandle) -> Result<(), String> {
    let sessions_dir = sessions_dir()?;
    let state = app.state::<SessionsWatcherState>();
    let mut guard = state.0.lock().map_err(|e| format!("Lock error: {e}"))?;

    if let Some(mut watcher) = guard.take() {
        let _ = watcher.unwatch(&sessions_dir);
    }

    Ok(())
}
