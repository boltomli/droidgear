//! OpenCode configuration management commands (Tauri wrappers).
//!
//! Core logic lives in `droidgear-core`.

pub use droidgear_core::opencode::{
    OpenCodeConfigStatus, OpenCodeCurrentConfig, OpenCodeProfile, ProviderTemplate,
};

/// List all OpenCode profiles
#[tauri::command]
#[specta::specta]
pub async fn list_opencode_profiles() -> Result<Vec<OpenCodeProfile>, String> {
    droidgear_core::opencode::list_opencode_profiles()
}

/// Get a profile by ID
#[tauri::command]
#[specta::specta]
pub async fn get_opencode_profile(id: String) -> Result<OpenCodeProfile, String> {
    droidgear_core::opencode::get_opencode_profile(&id)
}

/// Save a profile (create or update)
#[tauri::command]
#[specta::specta]
pub async fn save_opencode_profile(profile: OpenCodeProfile) -> Result<(), String> {
    droidgear_core::opencode::save_opencode_profile(profile)
}

/// Delete a profile
#[tauri::command]
#[specta::specta]
pub async fn delete_opencode_profile(id: String) -> Result<(), String> {
    droidgear_core::opencode::delete_opencode_profile(&id)
}

/// Duplicate a profile
#[tauri::command]
#[specta::specta]
pub async fn duplicate_opencode_profile(
    id: String,
    new_name: String,
) -> Result<OpenCodeProfile, String> {
    droidgear_core::opencode::duplicate_opencode_profile(&id, &new_name)
}

/// Create default profile if none exists
#[tauri::command]
#[specta::specta]
pub async fn create_default_profile() -> Result<OpenCodeProfile, String> {
    droidgear_core::opencode::create_default_profile()
}

/// Get active profile ID
#[tauri::command]
#[specta::specta]
pub async fn get_active_opencode_profile_id() -> Result<Option<String>, String> {
    droidgear_core::opencode::get_active_opencode_profile_id()
}

/// Apply a profile to OpenCode config files
#[tauri::command]
#[specta::specta]
pub async fn apply_opencode_profile(id: String) -> Result<(), String> {
    droidgear_core::opencode::apply_opencode_profile(&id)
}

/// Get OpenCode config status
#[tauri::command]
#[specta::specta]
pub async fn get_opencode_config_status() -> Result<OpenCodeConfigStatus, String> {
    droidgear_core::opencode::get_opencode_config_status()
}

/// Get provider templates
#[tauri::command]
#[specta::specta]
pub async fn get_opencode_provider_templates() -> Result<Vec<ProviderTemplate>, String> {
    Ok(droidgear_core::opencode::get_opencode_provider_templates())
}

/// Test provider connection
#[tauri::command]
#[specta::specta]
pub async fn test_opencode_provider_connection(
    provider_id: String,
    base_url: String,
    api_key: String,
) -> Result<bool, String> {
    droidgear_core::opencode::test_opencode_provider_connection(&provider_id, &base_url, &api_key)
        .await
}

/// Read current OpenCode configuration from config files
#[tauri::command]
#[specta::specta]
pub async fn read_opencode_current_config() -> Result<OpenCodeCurrentConfig, String> {
    droidgear_core::opencode::read_opencode_current_config()
}
