//! Specs management commands.
//!
//! Handles reading spec files from ~/.factory/specs directory.

use notify::{Config, RecommendedWatcher, RecursiveMode, Watcher};
use serde::{Deserialize, Serialize};
use std::fs;
use std::path::PathBuf;
use std::sync::Mutex;
use tauri::{AppHandle, Emitter, Manager};

/// Spec file metadata
#[derive(Debug, Clone, Serialize, Deserialize, specta::Type)]
#[serde(rename_all = "camelCase")]
pub struct SpecFile {
    /// File name (e.g., "2025-12-18-ui.md")
    pub name: String,
    /// Full path to the file
    pub path: String,
    /// File content
    pub content: String,
    /// Last modified timestamp in milliseconds
    pub modified_at: f64,
}

/// Gets the path to the specs directory (~/.factory/specs).
fn get_specs_dir() -> Result<PathBuf, String> {
    let home_dir = dirs::home_dir().ok_or_else(|| "Failed to get home directory".to_string())?;
    Ok(home_dir.join(".factory").join("specs"))
}

/// Lists all spec files from ~/.factory/specs directory.
/// Returns files sorted by modification time (newest first).
#[tauri::command]
#[specta::specta]
pub async fn list_specs() -> Result<Vec<SpecFile>, String> {
    log::debug!("Listing specs from ~/.factory/specs");
    let specs_dir = get_specs_dir()?;

    if !specs_dir.exists() {
        log::info!("Specs directory does not exist: {specs_dir:?}");
        return Ok(Vec::new());
    }

    let mut specs: Vec<SpecFile> = Vec::new();

    let entries = fs::read_dir(&specs_dir).map_err(|e| {
        log::error!("Failed to read specs directory: {e}");
        format!("Failed to read specs directory: {e}")
    })?;

    for entry in entries {
        let entry = match entry {
            Ok(e) => e,
            Err(e) => {
                log::warn!("Failed to read directory entry: {e}");
                continue;
            }
        };

        let path = entry.path();

        // Only process markdown files
        if path.extension().and_then(|s| s.to_str()) != Some("md") {
            continue;
        }

        let metadata = match fs::metadata(&path) {
            Ok(m) => m,
            Err(e) => {
                log::warn!("Failed to read file metadata for {path:?}: {e}");
                continue;
            }
        };

        if !metadata.is_file() {
            continue;
        }

        let modified_at = metadata
            .modified()
            .ok()
            .and_then(|t| t.duration_since(std::time::UNIX_EPOCH).ok())
            .map(|d| d.as_millis() as f64)
            .unwrap_or(0.0);

        let content = match fs::read_to_string(&path) {
            Ok(c) => c,
            Err(e) => {
                log::warn!("Failed to read file content for {path:?}: {e}");
                continue;
            }
        };

        let name = path
            .file_name()
            .and_then(|s| s.to_str())
            .unwrap_or("")
            .to_string();

        specs.push(SpecFile {
            name,
            path: path.to_string_lossy().to_string(),
            content,
            modified_at,
        });
    }

    // Sort by modified time, newest first
    specs.sort_by(|a, b| {
        b.modified_at
            .partial_cmp(&a.modified_at)
            .unwrap_or(std::cmp::Ordering::Equal)
    });

    log::info!("Found {} spec files", specs.len());
    Ok(specs)
}

/// Reads a single spec file by path.
#[tauri::command]
#[specta::specta]
pub async fn read_spec(path: String) -> Result<SpecFile, String> {
    log::debug!("Reading spec file: {path}");

    let path_buf = PathBuf::from(&path);

    if !path_buf.exists() {
        return Err("Spec file not found".to_string());
    }

    let metadata = fs::metadata(&path_buf).map_err(|e| {
        log::error!("Failed to read file metadata: {e}");
        format!("Failed to read file metadata: {e}")
    })?;

    let modified_at = metadata
        .modified()
        .ok()
        .and_then(|t| t.duration_since(std::time::UNIX_EPOCH).ok())
        .map(|d| d.as_millis() as f64)
        .unwrap_or(0.0);

    let content = fs::read_to_string(&path_buf).map_err(|e| {
        log::error!("Failed to read file content: {e}");
        format!("Failed to read file content: {e}")
    })?;

    let name = path_buf
        .file_name()
        .and_then(|s| s.to_str())
        .unwrap_or("")
        .to_string();

    Ok(SpecFile {
        name,
        path,
        content,
        modified_at,
    })
}

/// Renames a spec file.
#[tauri::command]
#[specta::specta]
pub async fn rename_spec(old_path: String, new_name: String) -> Result<SpecFile, String> {
    log::debug!("Renaming spec file: {old_path} -> {new_name}");

    let specs_dir = get_specs_dir()?;
    let old_path_buf = PathBuf::from(&old_path);

    // Security check: ensure the file is in specs directory
    if !old_path_buf.starts_with(&specs_dir) {
        return Err("Invalid file path".to_string());
    }

    if !old_path_buf.exists() {
        return Err("Spec file not found".to_string());
    }

    // Validate new name
    let new_name = new_name.trim();
    if new_name.is_empty() {
        return Err("File name cannot be empty".to_string());
    }

    // Ensure .md extension
    let new_name = if new_name.ends_with(".md") {
        new_name.to_string()
    } else {
        format!("{new_name}.md")
    };

    // Validate filename (no path separators)
    if new_name.contains('/') || new_name.contains('\\') {
        return Err("Invalid file name".to_string());
    }

    let new_path = specs_dir.join(&new_name);

    // Check if target already exists
    if new_path.exists() && new_path != old_path_buf {
        return Err("A file with this name already exists".to_string());
    }

    // Rename the file
    fs::rename(&old_path_buf, &new_path).map_err(|e| {
        log::error!("Failed to rename spec file: {e}");
        format!("Failed to rename file: {e}")
    })?;

    log::info!("Renamed spec file to: {new_path:?}");

    // Return updated spec file
    read_spec(new_path.to_string_lossy().to_string()).await
}

/// Deletes a spec file.
#[tauri::command]
#[specta::specta]
pub async fn delete_spec(path: String) -> Result<(), String> {
    log::debug!("Deleting spec file: {path}");

    let specs_dir = get_specs_dir()?;
    let path_buf = PathBuf::from(&path);

    // Security check: ensure the file is in specs directory
    if !path_buf.starts_with(&specs_dir) {
        return Err("Invalid file path".to_string());
    }

    if !path_buf.exists() {
        return Err("Spec file not found".to_string());
    }

    fs::remove_file(&path_buf).map_err(|e| {
        log::error!("Failed to delete spec file: {e}");
        format!("Failed to delete file: {e}")
    })?;

    log::info!("Deleted spec file: {path}");
    Ok(())
}

/// Updates a spec file content.
#[tauri::command]
#[specta::specta]
pub async fn update_spec(path: String, content: String) -> Result<SpecFile, String> {
    log::debug!("Updating spec file: {path}");

    let specs_dir = get_specs_dir()?;
    let path_buf = PathBuf::from(&path);

    // Security check: ensure the file is in specs directory
    if !path_buf.starts_with(&specs_dir) {
        return Err("Invalid file path".to_string());
    }

    if !path_buf.exists() {
        return Err("Spec file not found".to_string());
    }

    // Write the new content
    fs::write(&path_buf, &content).map_err(|e| {
        log::error!("Failed to write spec file: {e}");
        format!("Failed to write file: {e}")
    })?;

    log::info!("Updated spec file: {path}");

    // Return updated spec file
    read_spec(path).await
}

/// State for the specs file watcher
pub struct SpecsWatcherState(pub Mutex<Option<RecommendedWatcher>>);

/// Starts watching the specs directory for changes.
/// Emits "specs-changed" event when files are added, modified, or removed.
#[tauri::command]
#[specta::specta]
pub async fn start_specs_watcher(app: AppHandle) -> Result<(), String> {
    log::debug!("Starting specs watcher");

    let specs_dir = get_specs_dir()?;

    // Create directory if it doesn't exist
    if !specs_dir.exists() {
        fs::create_dir_all(&specs_dir).map_err(|e| {
            log::error!("Failed to create specs directory: {e}");
            format!("Failed to create specs directory: {e}")
        })?;
    }

    let app_handle = app.clone();

    let watcher = RecommendedWatcher::new(
        move |res: Result<notify::Event, notify::Error>| {
            if let Ok(event) = res {
                // Only emit for relevant events
                use notify::EventKind;
                match event.kind {
                    EventKind::Create(_) | EventKind::Modify(_) | EventKind::Remove(_) => {
                        log::debug!("Specs directory changed: {event:?}");
                        let _ = app_handle.emit("specs-changed", ());
                    }
                    _ => {}
                }
            }
        },
        Config::default(),
    )
    .map_err(|e| {
        log::error!("Failed to create watcher: {e}");
        format!("Failed to create watcher: {e}")
    })?;

    // Store watcher in state
    let state = app.state::<SpecsWatcherState>();
    let mut guard = state.0.lock().map_err(|e| format!("Lock error: {e}"))?;

    // Replace existing watcher
    if let Some(mut old_watcher) = guard.take() {
        let _ = old_watcher.unwatch(&specs_dir);
    }

    let mut watcher = watcher;
    watcher
        .watch(&specs_dir, RecursiveMode::NonRecursive)
        .map_err(|e| {
            log::error!("Failed to watch specs directory: {e}");
            format!("Failed to watch directory: {e}")
        })?;

    *guard = Some(watcher);

    log::info!("Started watching specs directory: {specs_dir:?}");
    Ok(())
}

/// Stops watching the specs directory.
#[tauri::command]
#[specta::specta]
pub async fn stop_specs_watcher(app: AppHandle) -> Result<(), String> {
    log::debug!("Stopping specs watcher");

    let specs_dir = get_specs_dir()?;
    let state = app.state::<SpecsWatcherState>();
    let mut guard = state.0.lock().map_err(|e| format!("Lock error: {e}"))?;

    if let Some(mut watcher) = guard.take() {
        let _ = watcher.unwatch(&specs_dir);
        log::info!("Stopped watching specs directory");
    }

    Ok(())
}
