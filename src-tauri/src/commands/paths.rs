//! Configuration paths management (Tauri command wrappers).
//!
//! Core logic lives in `droidgear-core`.

pub use droidgear_core::paths::{ConfigPaths, EffectivePaths, WslInfo};

/// Gets the current configuration paths (custom values only)
#[tauri::command]
#[specta::specta]
pub async fn get_config_paths() -> Result<ConfigPaths, String> {
    Ok(droidgear_core::paths::load_config_paths())
}

/// Gets all effective paths with default indicators
#[tauri::command]
#[specta::specta]
pub async fn get_effective_paths() -> Result<EffectivePaths, String> {
    droidgear_core::paths::get_effective_paths()
}

/// Saves a single configuration path
#[tauri::command]
#[specta::specta]
pub async fn save_config_path(key: String, path: String) -> Result<(), String> {
    droidgear_core::paths::save_config_path(&key, &path)
}

/// Resets a single configuration path to default
#[tauri::command]
#[specta::specta]
pub async fn reset_config_path(key: String) -> Result<(), String> {
    droidgear_core::paths::reset_config_path(&key)
}

/// Gets the default paths (for UI display)
#[tauri::command]
#[specta::specta]
pub async fn get_default_paths() -> Result<EffectivePaths, String> {
    droidgear_core::paths::get_default_paths()
}

/// Checks if WSL is available and lists distributions (Windows only)
#[tauri::command]
#[specta::specta]
pub async fn get_wsl_info() -> Result<WslInfo, String> {
    droidgear_core::paths::get_wsl_info()
}

/// Gets the WSL username for a specific distribution
#[tauri::command]
#[specta::specta]
pub async fn get_wsl_username(distro: String) -> Result<String, String> {
    droidgear_core::paths::get_wsl_username(&distro)
}

/// Builds a WSL path for a config directory
#[tauri::command]
#[specta::specta]
pub async fn build_wsl_path(
    distro: String,
    username: String,
    config_key: String,
) -> Result<String, String> {
    droidgear_core::paths::build_wsl_path(&distro, &username, &config_key)
}
