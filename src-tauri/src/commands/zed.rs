//! Tauri command wrappers for Zed profile management.
//!
//! These are thin wrappers around `droidgear_core::zed` functions.
//! All commands return `Result<T, String>` and are registered in `bindings.rs`.

use droidgear_core::zed::{
    apply_zed_profile, create_default_zed_profile, delete_zed_profile, duplicate_zed_profile,
    get_active_zed_profile_id, get_zed_config_status, get_zed_profile, list_zed_profiles,
    read_zed_current_config, save_zed_profile, ZedConfigStatus, ZedCurrentConfig, ZedProfile,
};

/// List all Zed profiles, sorted alphabetically by name.
#[tauri::command]
#[specta::specta]
pub async fn list_zed_profiles_cmd() -> Result<Vec<ZedProfile>, String> {
    list_zed_profiles()
}

/// Get a single Zed profile by ID.
#[tauri::command]
#[specta::specta]
pub async fn get_zed_profile_cmd(id: String) -> Result<ZedProfile, String> {
    get_zed_profile(&id)
}

/// Save a Zed profile (creates or updates).
#[tauri::command]
#[specta::specta]
pub async fn save_zed_profile_cmd(profile: ZedProfile) -> Result<(), String> {
    save_zed_profile(profile)
}

/// Delete a Zed profile by ID.
#[tauri::command]
#[specta::specta]
pub async fn delete_zed_profile_cmd(id: String) -> Result<(), String> {
    delete_zed_profile(&id)
}

/// Duplicate a Zed profile with a new name.
#[tauri::command]
#[specta::specta]
pub async fn duplicate_zed_profile_cmd(id: String, new_name: String) -> Result<ZedProfile, String> {
    duplicate_zed_profile(&id, &new_name)
}

/// Create a default "Default" Zed profile (only if none exist).
#[tauri::command]
#[specta::specta]
pub async fn create_default_zed_profile_cmd() -> Result<ZedProfile, String> {
    create_default_zed_profile()
}

/// Get the active Zed profile ID, if any.
#[tauri::command]
#[specta::specta]
pub async fn get_active_zed_profile_id_cmd() -> Result<Option<String>, String> {
    get_active_zed_profile_id()
}

/// Apply a Zed profile to `~/.config/zed/settings.json` (MERGE semantics).
#[tauri::command]
#[specta::specta]
pub async fn apply_zed_profile_cmd(id: String) -> Result<(), String> {
    apply_zed_profile(&id)
}

/// Get the live Zed config status.
#[tauri::command]
#[specta::specta]
pub async fn get_zed_config_status_cmd() -> Result<ZedConfigStatus, String> {
    get_zed_config_status()
}

/// Read the current live Zed configuration from settings.json.
#[tauri::command]
#[specta::specta]
pub async fn read_zed_current_config_cmd() -> Result<ZedCurrentConfig, String> {
    read_zed_current_config()
}
