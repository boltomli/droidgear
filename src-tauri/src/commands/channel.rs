//! API Channel management commands (Tauri wrappers).
//!
//! Core logic lives in `droidgear-core`.

use super::config::ModelInfo;

pub use droidgear_core::channel::{ApiChannel, ChannelToken, ChannelType};

/// Loads all channels from ~/.droidgear/channels.json
/// Falls back to ~/.factory/settings.json for migration
#[tauri::command]
#[specta::specta]
pub async fn load_channels() -> Result<Vec<ApiChannel>, String> {
    droidgear_core::channel::load_channels()
}

/// Saves all channels to ~/.droidgear/channels.json
#[tauri::command]
#[specta::specta]
pub async fn save_channels(channels: Vec<ApiChannel>) -> Result<(), String> {
    droidgear_core::channel::save_channels(channels)
}

/// Saves a channel's credentials to ~/.droidgear/auth/
#[tauri::command]
#[specta::specta]
pub async fn save_channel_credentials(
    channel_id: String,
    username: String,
    password: String,
) -> Result<(), String> {
    droidgear_core::channel::save_channel_credentials(&channel_id, &username, &password)
}

/// Gets a channel's credentials from ~/.droidgear/auth/
#[tauri::command]
#[specta::specta]
pub async fn get_channel_credentials(
    channel_id: String,
) -> Result<Option<(String, String)>, String> {
    droidgear_core::channel::get_channel_credentials(&channel_id)
}

/// Saves a channel's API key to ~/.droidgear/auth/
#[tauri::command]
#[specta::specta]
pub async fn save_channel_api_key(channel_id: String, api_key: String) -> Result<(), String> {
    droidgear_core::channel::save_channel_api_key(&channel_id, &api_key)
}

/// Gets a channel's API key from ~/.droidgear/auth/
#[tauri::command]
#[specta::specta]
pub async fn get_channel_api_key(channel_id: String) -> Result<Option<String>, String> {
    droidgear_core::channel::get_channel_api_key(&channel_id)
}

/// Deletes a channel's credentials from ~/.droidgear/auth/
#[tauri::command]
#[specta::specta]
pub async fn delete_channel_credentials(channel_id: String) -> Result<(), String> {
    droidgear_core::channel::delete_channel_credentials(&channel_id)
}

/// Detects channel type by probing characteristic endpoints
#[tauri::command]
#[specta::specta]
pub async fn detect_channel_type(base_url: String) -> Result<ChannelType, String> {
    droidgear_core::channel::detect_channel_type(&base_url).await
}

/// Fetches tokens from a channel (dispatches based on channel type)
#[tauri::command]
#[specta::specta]
pub async fn fetch_channel_tokens(
    channel_type: ChannelType,
    base_url: String,
    username: String,
    password: String,
) -> Result<Vec<ChannelToken>, String> {
    droidgear_core::channel::fetch_channel_tokens(channel_type, &base_url, &username, &password)
        .await
}

/// Fetches models using an API key (for quick model addition from channels)
#[tauri::command]
#[specta::specta]
pub async fn fetch_models_by_api_key(
    base_url: String,
    api_key: String,
    platform: Option<String>,
) -> Result<Vec<ModelInfo>, String> {
    droidgear_core::channel::fetch_models_by_api_key(&base_url, &api_key, platform.as_deref()).await
}
