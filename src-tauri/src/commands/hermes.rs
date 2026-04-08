//! Hermes Agent 配置管理命令（Tauri wrappers）。
//!
//! Core logic lives in `droidgear-core`.

pub use droidgear_core::hermes::{HermesConfigStatus, HermesCurrentConfig, HermesProfile};

/// List all Hermes profiles
#[tauri::command]
#[specta::specta]
pub async fn list_hermes_profiles() -> Result<Vec<HermesProfile>, String> {
    droidgear_core::hermes::list_hermes_profiles()
}

/// Get a profile by ID
#[tauri::command]
#[specta::specta]
pub async fn get_hermes_profile(id: String) -> Result<HermesProfile, String> {
    droidgear_core::hermes::get_hermes_profile(&id)
}

/// Save a profile (create or update)
#[tauri::command]
#[specta::specta]
pub async fn save_hermes_profile(profile: HermesProfile) -> Result<(), String> {
    droidgear_core::hermes::save_hermes_profile(profile)
}

/// Delete a profile
#[tauri::command]
#[specta::specta]
pub async fn delete_hermes_profile(id: String) -> Result<(), String> {
    droidgear_core::hermes::delete_hermes_profile(&id)
}

/// Duplicate a profile
#[tauri::command]
#[specta::specta]
pub async fn duplicate_hermes_profile(
    id: String,
    new_name: String,
) -> Result<HermesProfile, String> {
    droidgear_core::hermes::duplicate_hermes_profile(&id, &new_name)
}

/// Create default profile (when no profiles exist)
#[tauri::command]
#[specta::specta]
pub async fn create_default_hermes_profile() -> Result<HermesProfile, String> {
    droidgear_core::hermes::create_default_hermes_profile()
}

/// Get active profile ID
#[tauri::command]
#[specta::specta]
pub async fn get_active_hermes_profile_id() -> Result<Option<String>, String> {
    droidgear_core::hermes::get_active_hermes_profile_id()
}

/// Apply a profile to `~/.hermes/config.yaml`
#[tauri::command]
#[specta::specta]
pub async fn apply_hermes_profile(id: String) -> Result<(), String> {
    droidgear_core::hermes::apply_hermes_profile(&id)
}

/// Get Hermes config status
#[tauri::command]
#[specta::specta]
pub async fn get_hermes_config_status() -> Result<HermesConfigStatus, String> {
    droidgear_core::hermes::get_hermes_config_status()
}

/// Read current Hermes configuration from config files
#[tauri::command]
#[specta::specta]
pub async fn read_hermes_current_config() -> Result<HermesCurrentConfig, String> {
    droidgear_core::hermes::read_hermes_current_config()
}
