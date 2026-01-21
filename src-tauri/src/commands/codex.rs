//! Codex CLI 配置管理命令。
//!
//! 提供 Profile CRUD，并支持将 Profile 应用到 `~/.codex/auth.json` 与 `~/.codex/config.toml`。

use chrono::Utc;
use serde::{Deserialize, Serialize};
use serde_json::Value;
use specta::Type;
use std::collections::HashMap;
use std::path::{Path, PathBuf};
use uuid::Uuid;

// ============================================================================
// Types
// ============================================================================

/// Codex Profile（用于在 DroidGear 内部保存并切换）
#[derive(Debug, Clone, Serialize, Deserialize, Type)]
#[serde(rename_all = "camelCase")]
pub struct CodexProfile {
    pub id: String,
    pub name: String,
    #[serde(skip_serializing_if = "Option::is_none")]
    pub description: Option<String>,
    pub created_at: String,
    pub updated_at: String,
    #[serde(default)]
    pub auth: HashMap<String, Value>,
    #[serde(default)]
    pub config_toml: String,
}

/// Codex Live 配置状态
#[derive(Debug, Clone, Serialize, Deserialize, Type)]
#[serde(rename_all = "camelCase")]
pub struct CodexConfigStatus {
    pub auth_exists: bool,
    pub config_exists: bool,
    pub auth_path: String,
    pub config_path: String,
}

/// 当前 Codex Live 配置（从 `~/.codex/*` 读取）
#[derive(Debug, Clone, Serialize, Deserialize, Type)]
#[serde(rename_all = "camelCase")]
pub struct CodexCurrentConfig {
    #[serde(default)]
    pub auth: HashMap<String, Value>,
    #[serde(default)]
    pub config_toml: String,
}

// ============================================================================
// Path Helpers
// ============================================================================

fn get_home_dir() -> Result<PathBuf, String> {
    dirs::home_dir().ok_or("Failed to get home directory".to_string())
}

fn get_droidgear_codex_dir() -> Result<PathBuf, String> {
    Ok(get_home_dir()?.join(".droidgear").join("codex"))
}

/// `~/.droidgear/codex/profiles/`
fn get_profiles_dir() -> Result<PathBuf, String> {
    let dir = get_droidgear_codex_dir()?.join("profiles");
    if !dir.exists() {
        std::fs::create_dir_all(&dir)
            .map_err(|e| format!("Failed to create codex profiles directory: {e}"))?;
    }
    Ok(dir)
}

/// `~/.droidgear/codex/active-profile.txt`
fn get_active_profile_path() -> Result<PathBuf, String> {
    let dir = get_droidgear_codex_dir()?;
    if !dir.exists() {
        std::fs::create_dir_all(&dir)
            .map_err(|e| format!("Failed to create codex directory: {e}"))?;
    }
    Ok(dir.join("active-profile.txt"))
}

/// `~/.codex/`
fn get_codex_config_dir() -> Result<PathBuf, String> {
    let dir = get_home_dir()?.join(".codex");
    if !dir.exists() {
        std::fs::create_dir_all(&dir)
            .map_err(|e| format!("Failed to create codex config directory: {e}"))?;
    }
    Ok(dir)
}

fn get_codex_auth_path() -> Result<PathBuf, String> {
    Ok(get_codex_config_dir()?.join("auth.json"))
}

fn get_codex_config_path() -> Result<PathBuf, String> {
    Ok(get_codex_config_dir()?.join("config.toml"))
}

fn validate_profile_id(id: &str) -> Result<(), String> {
    let ok = id
        .chars()
        .all(|c| c.is_ascii_alphanumeric() || c == '-' || c == '_');
    if ok && !id.is_empty() {
        Ok(())
    } else {
        Err("Invalid profile id".to_string())
    }
}

fn get_profile_path(id: &str) -> Result<PathBuf, String> {
    validate_profile_id(id)?;
    Ok(get_profiles_dir()?.join(format!("{id}.json")))
}

// ============================================================================
// File Helpers
// ============================================================================

fn atomic_write(path: &Path, bytes: &[u8]) -> Result<(), String> {
    if let Some(parent) = path.parent() {
        if !parent.exists() {
            std::fs::create_dir_all(parent)
                .map_err(|e| format!("Failed to create directory: {e}"))?;
        }
    }

    let temp_path = path.with_extension("tmp");
    std::fs::write(&temp_path, bytes).map_err(|e| format!("Failed to write file: {e}"))?;
    std::fs::rename(&temp_path, path).map_err(|e| {
        let _ = std::fs::remove_file(&temp_path);
        format!("Failed to finalize file: {e}")
    })?;
    Ok(())
}

fn read_json_object_file(path: &Path) -> Result<HashMap<String, Value>, String> {
    if !path.exists() {
        return Ok(HashMap::new());
    }
    let s = std::fs::read_to_string(path).map_err(|e| format!("Failed to read file: {e}"))?;
    if s.trim().is_empty() {
        return Ok(HashMap::new());
    }
    let v: Value = serde_json::from_str(&s).map_err(|e| format!("Invalid JSON: {e}"))?;
    match v {
        Value::Object(map) => Ok(map.into_iter().collect()),
        _ => Err("Invalid JSON: expected object".to_string()),
    }
}

fn write_json_object_file(path: &Path, obj: &HashMap<String, Value>) -> Result<(), String> {
    let v = Value::Object(obj.clone().into_iter().collect());
    let s =
        serde_json::to_string_pretty(&v).map_err(|e| format!("Failed to serialize JSON: {e}"))?;
    atomic_write(path, s.as_bytes())
}

fn read_text_file(path: &Path) -> Result<String, String> {
    if !path.exists() {
        return Ok(String::new());
    }
    std::fs::read_to_string(path).map_err(|e| format!("Failed to read file: {e}"))
}

fn validate_toml(text: &str) -> Result<(), String> {
    if text.trim().is_empty() {
        return Ok(());
    }
    toml::from_str::<toml::Table>(text)
        .map(|_| ())
        .map_err(|e| format!("Invalid TOML: {e}"))
}

fn write_codex_live_atomic(auth: &HashMap<String, Value>, config_toml: &str) -> Result<(), String> {
    validate_toml(config_toml)?;

    let auth_path = get_codex_auth_path()?;
    let config_path = get_codex_config_path()?;

    let old_auth = if auth_path.exists() {
        Some(std::fs::read(&auth_path).map_err(|e| format!("Failed to read auth.json: {e}"))?)
    } else {
        None
    };
    let old_config = if config_path.exists() {
        Some(std::fs::read(&config_path).map_err(|e| format!("Failed to read config.toml: {e}"))?)
    } else {
        None
    };

    // 1) 写 auth.json
    write_json_object_file(&auth_path, auth)?;

    // 2) 写 config.toml（失败回滚 auth.json 与 config.toml）
    if let Err(e) = atomic_write(&config_path, config_toml.as_bytes()) {
        if let Some(bytes) = old_auth {
            let _ = atomic_write(&auth_path, &bytes);
        } else {
            let _ = std::fs::remove_file(&auth_path);
        }
        if let Some(bytes) = old_config {
            let _ = atomic_write(&config_path, &bytes);
        } else {
            let _ = std::fs::remove_file(&config_path);
        }
        return Err(e);
    }

    Ok(())
}

// ============================================================================
// Profile Helpers
// ============================================================================

fn now_rfc3339() -> String {
    Utc::now().to_rfc3339()
}

fn default_codex_template_config() -> String {
    // 参考 cc-switch 的 Codex 自定义模板，作为 DroidGear 默认 Profile 初始值。
    r#"model_provider = "custom"
model = "gpt-5.2"
model_reasoning_effort = "high"
disable_response_storage = true

[model_providers.custom]
name = "custom"
wire_api = "responses"
requires_openai_auth = true
"#
    .to_string()
}

fn read_profile_file(path: &Path) -> Result<CodexProfile, String> {
    let s = std::fs::read_to_string(path).map_err(|e| format!("Failed to read profile: {e}"))?;
    serde_json::from_str::<CodexProfile>(&s).map_err(|e| format!("Invalid profile JSON: {e}"))
}

fn write_profile_file(profile: &CodexProfile) -> Result<(), String> {
    let path = get_profile_path(&profile.id)?;
    let s = serde_json::to_string_pretty(profile)
        .map_err(|e| format!("Failed to serialize profile JSON: {e}"))?;
    atomic_write(&path, s.as_bytes())
}

fn load_profile_by_id(id: &str) -> Result<CodexProfile, String> {
    let path = get_profile_path(id)?;
    read_profile_file(&path)
}

// ============================================================================
// Tauri Commands
// ============================================================================

/// 列出所有 Codex Profiles
#[tauri::command]
#[specta::specta]
pub async fn list_codex_profiles() -> Result<Vec<CodexProfile>, String> {
    let dir = get_profiles_dir()?;
    let mut profiles = Vec::new();

    for entry in std::fs::read_dir(&dir).map_err(|e| format!("Failed to read profiles dir: {e}"))? {
        let entry = entry.map_err(|e| format!("Failed to read dir entry: {e}"))?;
        let path = entry.path();
        if path.extension().and_then(|s| s.to_str()) != Some("json") {
            continue;
        }
        if let Ok(p) = read_profile_file(&path) {
            profiles.push(p);
        }
    }

    profiles.sort_by(|a, b| a.name.to_lowercase().cmp(&b.name.to_lowercase()));
    Ok(profiles)
}

/// 获取指定 Profile
#[tauri::command]
#[specta::specta]
pub async fn get_codex_profile(id: String) -> Result<CodexProfile, String> {
    load_profile_by_id(&id)
}

/// 保存 Profile（新建或更新）
#[tauri::command]
#[specta::specta]
pub async fn save_codex_profile(mut profile: CodexProfile) -> Result<(), String> {
    if profile.id.trim().is_empty() {
        profile.id = Uuid::new_v4().to_string();
        profile.created_at = now_rfc3339();
    } else if get_profile_path(&profile.id)?.exists() {
        // 保留 created_at
        if let Ok(old) = load_profile_by_id(&profile.id) {
            profile.created_at = old.created_at;
        }
    } else if profile.created_at.trim().is_empty() {
        profile.created_at = now_rfc3339();
    }

    profile.updated_at = now_rfc3339();
    write_profile_file(&profile)
}

/// 删除 Profile
#[tauri::command]
#[specta::specta]
pub async fn delete_codex_profile(id: String) -> Result<(), String> {
    let path = get_profile_path(&id)?;
    if path.exists() {
        std::fs::remove_file(&path).map_err(|e| format!("Failed to delete profile: {e}"))?;
    }

    // 如果删除的是 active profile，则清空
    if let Ok(active) = get_active_profile_id_internal() {
        if active.as_deref() == Some(id.as_str()) {
            let active_path = get_active_profile_path()?;
            let _ = std::fs::remove_file(active_path);
        }
    }

    Ok(())
}

/// 复制 Profile
#[tauri::command]
#[specta::specta]
pub async fn duplicate_codex_profile(id: String, new_name: String) -> Result<CodexProfile, String> {
    let mut profile = load_profile_by_id(&id)?;
    profile.id = Uuid::new_v4().to_string();
    profile.name = new_name;
    profile.created_at = now_rfc3339();
    profile.updated_at = profile.created_at.clone();
    write_profile_file(&profile)?;
    Ok(profile)
}

/// 创建默认 Profile（当无 Profile 时调用）
#[tauri::command]
#[specta::specta]
pub async fn create_default_codex_profile() -> Result<CodexProfile, String> {
    let id = Uuid::new_v4().to_string();
    let now = now_rfc3339();
    let mut auth = HashMap::new();
    auth.insert("OPENAI_API_KEY".to_string(), Value::String(String::new()));

    let profile = CodexProfile {
        id,
        name: "默认".to_string(),
        description: Some("Codex 自定义模板（需填写 API Key）".to_string()),
        created_at: now.clone(),
        updated_at: now,
        auth,
        config_toml: default_codex_template_config(),
    };

    write_profile_file(&profile)?;
    Ok(profile)
}

fn get_active_profile_id_internal() -> Result<Option<String>, String> {
    let path = get_active_profile_path()?;
    if !path.exists() {
        return Ok(None);
    }
    let s = std::fs::read_to_string(&path)
        .map_err(|e| format!("Failed to read active profile id: {e}"))?;
    let id = s.trim().to_string();
    if id.is_empty() {
        Ok(None)
    } else {
        Ok(Some(id))
    }
}

/// 读取 active Profile ID
#[tauri::command]
#[specta::specta]
pub async fn get_active_codex_profile_id() -> Result<Option<String>, String> {
    get_active_profile_id_internal()
}

fn set_active_profile_id(id: &str) -> Result<(), String> {
    let path = get_active_profile_path()?;
    atomic_write(&path, id.as_bytes())
}

/// 应用指定 Profile 到 `~/.codex/*`
#[tauri::command]
#[specta::specta]
pub async fn apply_codex_profile(id: String) -> Result<(), String> {
    let profile = load_profile_by_id(&id)?;
    write_codex_live_atomic(&profile.auth, &profile.config_toml)?;
    set_active_profile_id(&id)?;
    Ok(())
}

/// 获取 Codex Live 配置状态（文件是否存在及路径）
#[tauri::command]
#[specta::specta]
pub async fn get_codex_config_status() -> Result<CodexConfigStatus, String> {
    let auth_path = get_codex_auth_path()?;
    let config_path = get_codex_config_path()?;
    Ok(CodexConfigStatus {
        auth_exists: auth_path.exists(),
        config_exists: config_path.exists(),
        auth_path: auth_path.to_string_lossy().to_string(),
        config_path: config_path.to_string_lossy().to_string(),
    })
}

/// 读取当前 `~/.codex/*` 配置（若不存在则返回空）
#[tauri::command]
#[specta::specta]
pub async fn read_codex_current_config() -> Result<CodexCurrentConfig, String> {
    let auth_path = get_codex_auth_path()?;
    let config_path = get_codex_config_path()?;

    let auth = read_json_object_file(&auth_path)?;
    let config_toml = read_text_file(&config_path)?;

    Ok(CodexCurrentConfig { auth, config_toml })
}
