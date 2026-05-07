//! Zed Editor configuration management (core).
//!
//! Handles Profile CRUD and applying profiles to Zed's `settings.json`.
//! Manages only `language_models.openai_compatible` — all other settings are preserved.

use serde::{Deserialize, Serialize};
use serde_json::{json, Map, Value};
use specta::Type;
use std::collections::HashMap;
use std::io::Read;
use std::path::{Path, PathBuf};
use uuid::Uuid;

use crate::{paths, storage};

// ============================================================================
// Types
// ============================================================================

/// Model capabilities — matches Zed's actual settings.json schema
#[derive(Debug, Clone, Serialize, Deserialize, Type)]
#[serde(rename_all = "camelCase")]
pub struct ZedModelCapabilities {
    /// Whether tool/function calling is supported
    pub tools: bool,
    /// Whether image input is supported
    pub images: bool,
    /// Whether parallel tool calls are supported
    #[serde(default, skip_serializing_if = "is_false")]
    pub parallel_tool_calls: bool,
    /// Whether prompt caching is supported
    #[serde(default, skip_serializing_if = "is_false")]
    pub prompt_cache_key: bool,
    /// Whether chat completions API is supported
    #[serde(default, skip_serializing_if = "is_false")]
    pub chat_completions: bool,
    /// Whether interleaved reasoning is supported
    #[serde(default, skip_serializing_if = "is_false")]
    pub interleaved_reasoning: bool,
}

fn is_false(b: &bool) -> bool {
    !b
}

/// A single model within a Zed provider
#[derive(Debug, Clone, Serialize, Deserialize, Type)]
#[serde(rename_all = "camelCase")]
pub struct ZedModel {
    pub name: String,
    #[serde(skip_serializing_if = "Option::is_none")]
    pub display_name: Option<String>,
    #[serde(skip_serializing_if = "Option::is_none")]
    pub max_tokens: Option<u32>,
    /// Max output tokens for this model
    #[serde(skip_serializing_if = "Option::is_none")]
    pub max_output_tokens: Option<u32>,
    /// Max completion tokens for this model
    #[serde(skip_serializing_if = "Option::is_none")]
    pub max_completion_tokens: Option<u32>,
    /// Model capabilities. Defaults to None.
    #[serde(skip_serializing_if = "Option::is_none")]
    pub capabilities: Option<ZedModelCapabilities>,
}

/// A provider configuration for Zed's openai_compatible format
#[derive(Debug, Clone, Serialize, Deserialize, Type)]
#[serde(rename_all = "camelCase")]
pub struct ZedProviderConfig {
    /// The API endpoint URL (e.g., "https://api.openai.com/v1")
    #[serde(rename = "api_url")]
    pub api_url: String,
    /// Available models for this provider
    #[serde(skip_serializing_if = "Option::is_none")]
    pub available_models: Option<Vec<ZedModel>>,
    /// API key — stored in profile ONLY, NEVER written to settings.json
    #[serde(skip_serializing_if = "Option::is_none")]
    pub api_key: Option<String>,
}

/// Zed Profile stored in `~/.droidgear/zed/profiles/<uuid>.json`
#[derive(Debug, Clone, Serialize, Deserialize, Type)]
#[serde(rename_all = "camelCase")]
pub struct ZedProfile {
    pub id: String,
    pub name: String,
    #[serde(skip_serializing_if = "Option::is_none")]
    pub description: Option<String>,
    pub created_at: String,
    pub updated_at: String,
    /// Map of provider name → provider config (api_key included for DroidGear storage)
    #[serde(default)]
    pub providers: HashMap<String, ZedProviderConfig>,
    /// Map of provider name → API key (separate map for clarity)
    #[serde(default, skip_serializing_if = "Option::is_none")]
    pub api_keys: Option<HashMap<String, String>>,
}

/// Live Zed configuration status
#[derive(Debug, Clone, Serialize, Deserialize, Type)]
#[serde(rename_all = "camelCase")]
pub struct ZedConfigStatus {
    pub config_exists: bool,
    pub has_openai_compatible: bool,
    pub config_path: String,
}

/// Current Zed config (parsed from settings.json)
#[derive(Debug, Clone, Serialize, Deserialize, Type, Default)]
#[serde(rename_all = "camelCase")]
pub struct ZedCurrentConfig {
    #[serde(default)]
    pub providers: HashMap<String, ZedProviderConfig>,
}

// ============================================================================
// Path Helpers
// ============================================================================

/// `~/.droidgear/zed/`
fn droidgear_zed_dir_for_home(home_dir: &Path) -> PathBuf {
    home_dir.join(".droidgear").join("zed")
}

/// `~/.droidgear/zed/profiles/`
fn profiles_dir_for_home(home_dir: &Path) -> Result<PathBuf, String> {
    let dir = droidgear_zed_dir_for_home(home_dir).join("profiles");
    if !dir.exists() {
        std::fs::create_dir_all(&dir)
            .map_err(|e| format!("Failed to create zed profiles directory: {e}"))?;
    }
    Ok(dir)
}

/// `~/.droidgear/zed/active-profile.txt`
fn active_profile_path_for_home(home_dir: &Path) -> Result<PathBuf, String> {
    let dir = droidgear_zed_dir_for_home(home_dir);
    if !dir.exists() {
        std::fs::create_dir_all(&dir)
            .map_err(|e| format!("Failed to create zed directory: {e}"))?;
    }
    Ok(dir.join("active-profile.txt"))
}

/// `~/.config/zed/` (or custom path)
fn zed_config_dir_for_home(home_dir: &Path) -> Result<PathBuf, String> {
    let config_paths = paths::load_config_paths_for_home(home_dir);
    let dir = paths::get_zed_config_dir_for_home(home_dir, &config_paths)?;
    Ok(dir)
}

/// `~/.config/zed/settings.json` (or custom path)
fn zed_config_path_for_home(home_dir: &Path) -> Result<PathBuf, String> {
    Ok(zed_config_dir_for_home(home_dir)?.join("settings.json"))
}

fn profile_path_for_home(home_dir: &Path, id: &str) -> Result<PathBuf, String> {
    // Validate that id is a valid UUID filename
    if id.is_empty() || id.contains('/') || id.contains('\\') {
        return Err("Invalid profile id".to_string());
    }
    Ok(profiles_dir_for_home(home_dir)?.join(format!("{id}.json")))
}

fn now_rfc3339() -> String {
    chrono::Utc::now().to_rfc3339()
}

// ============================================================================
// File helpers
// ============================================================================

fn read_profile_file(path: &Path) -> Result<ZedProfile, String> {
    let s = std::fs::read_to_string(path).map_err(|e| format!("Failed to read profile: {e}"))?;
    serde_json::from_str::<ZedProfile>(&s).map_err(|e| format!("Invalid profile JSON: {e}"))
}

fn write_profile_file(home_dir: &Path, profile: &ZedProfile) -> Result<(), String> {
    let path = profile_path_for_home(home_dir, &profile.id)?;
    let s = serde_json::to_string_pretty(profile)
        .map_err(|e| format!("Failed to serialize profile JSON: {e}"))?;
    storage::atomic_write(&path, s.as_bytes())
}

fn load_profile_by_id(home_dir: &Path, id: &str) -> Result<ZedProfile, String> {
    let path = profile_path_for_home(home_dir, id)?;
    read_profile_file(&path)
}

/// Read a settings.json file, stripping JSON comments.
/// Returns an error if the JSON is malformed (including trailing commas).
fn read_settings_json(path: &Path) -> Result<Value, String> {
    if !path.exists() {
        return Ok(json!({}));
    }
    let content = std::fs::read_to_string(path).map_err(|e| format!("Failed to read file: {e}"))?;
    if content.trim().is_empty() {
        return Ok(json!({}));
    }

    // Strip comments using json_comments
    let stripped = json_comments::StripComments::new(content.as_bytes());
    let mut buf = String::new();
    std::io::BufReader::new(stripped)
        .read_to_string(&mut buf)
        .map_err(|e| format!("Failed to strip comments: {e}"))?;

    // If after stripping comments, the content is empty, treat as empty object
    if buf.trim().is_empty() {
        return Ok(json!({}));
    }

    // Parse the stripped JSON — trailing commas will cause a parse error here
    serde_json::from_str(&buf).map_err(|e| format!("Invalid JSON in settings.json: {e}"))
}

// ============================================================================
// Profile CRUD
// ============================================================================

/// List all Zed profiles, sorted alphabetically by name (case-insensitive).
/// Auto-creates a default profile if none exist.
pub fn list_zed_profiles_for_home(home_dir: &Path) -> Result<Vec<ZedProfile>, String> {
    let dir = profiles_dir_for_home(home_dir)?;
    let mut profiles = Vec::new();

    if dir.exists() {
        for entry in
            std::fs::read_dir(&dir).map_err(|e| format!("Failed to read profiles dir: {e}"))?
        {
            let entry = match entry {
                Ok(e) => e,
                Err(_) => continue,
            };
            let path = entry.path();
            if path.extension().and_then(|s| s.to_str()) != Some("json") {
                continue;
            }
            if let Ok(profile) = read_profile_file(&path) {
                profiles.push(profile);
            }
        }
    }

    profiles.sort_by_key(|a| a.name.to_lowercase());
    Ok(profiles)
}

/// Get a single Zed profile by ID.
pub fn get_zed_profile_for_home(home_dir: &Path, id: &str) -> Result<ZedProfile, String> {
    load_profile_by_id(home_dir, id)
}

/// Save a Zed profile. If the profile has no ID, generates a new UUID.
/// Preserves existing `created_at` if the profile already exists.
pub fn save_zed_profile_for_home(home_dir: &Path, mut profile: ZedProfile) -> Result<(), String> {
    if profile.id.trim().is_empty() {
        profile.id = Uuid::new_v4().to_string();
        profile.created_at = now_rfc3339();
    } else if profile_path_for_home(home_dir, &profile.id)?.exists() {
        if let Ok(old) = load_profile_by_id(home_dir, &profile.id) {
            profile.created_at = old.created_at;
        }
    } else if profile.created_at.trim().is_empty() {
        profile.created_at = now_rfc3339();
    }

    profile.updated_at = now_rfc3339();
    write_profile_file(home_dir, &profile)
}

/// Delete a Zed profile by ID. If the deleted profile was the active one,
/// removes the active-profile.txt file.
pub fn delete_zed_profile_for_home(home_dir: &Path, id: &str) -> Result<(), String> {
    let path = profile_path_for_home(home_dir, id)?;
    if path.exists() {
        std::fs::remove_file(&path).map_err(|e| format!("Failed to delete profile: {e}"))?;
    }

    if let Ok(active) = get_active_zed_profile_id_for_home(home_dir) {
        if active.as_deref() == Some(id) {
            let active_path = active_profile_path_for_home(home_dir)?;
            let _ = std::fs::remove_file(active_path);
        }
    }

    Ok(())
}

/// Duplicate a Zed profile with a new ID and name.
pub fn duplicate_zed_profile_for_home(
    home_dir: &Path,
    id: &str,
    new_name: &str,
) -> Result<ZedProfile, String> {
    let mut profile = load_profile_by_id(home_dir, id)?;
    profile.id = Uuid::new_v4().to_string();
    profile.name = new_name.to_string();
    profile.created_at = now_rfc3339();
    profile.updated_at = profile.created_at.clone();
    write_profile_file(home_dir, &profile)?;
    Ok(profile)
}

/// Create a default "Default" Zed profile.
/// Returns error if profiles already exist (except if the profiles directory doesn't exist).
pub fn create_default_zed_profile_for_home(home_dir: &Path) -> Result<ZedProfile, String> {
    // Check if any profiles exist
    let existing = list_zed_profiles_for_home(home_dir)?;
    if !existing.is_empty() {
        return Err("Profiles already exist".to_string());
    }

    let now = now_rfc3339();
    let profile = ZedProfile {
        id: Uuid::new_v4().to_string(),
        name: "Default".to_string(),
        description: None,
        created_at: now.clone(),
        updated_at: now,
        providers: HashMap::new(),
        api_keys: None,
    };

    save_zed_profile_for_home(home_dir, profile.clone())?;

    // Set as active
    let active_path = active_profile_path_for_home(home_dir)?;
    storage::atomic_write(&active_path, profile.id.as_bytes())?;

    Ok(profile)
}

// ============================================================================
// Active profile tracking
// ============================================================================

pub fn get_active_zed_profile_id_for_home(home_dir: &Path) -> Result<Option<String>, String> {
    let path = active_profile_path_for_home(home_dir)?;
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

fn set_active_zed_profile_id_for_home(home_dir: &Path, id: &str) -> Result<(), String> {
    let path = active_profile_path_for_home(home_dir)?;
    storage::atomic_write(&path, id.as_bytes())
}

// ============================================================================
// Apply profile — MERGE semantics
// ============================================================================

/// Apply a Zed profile to settings.json.
///
/// **MERGE semantics** (never REPLACE):
/// - Reads existing settings.json
/// - Modifies only `language_models.openai_compatible`
/// - Preserves ALL other keys (theme, keybindings, etc.)
/// - Preserves providers NOT in the profile (external providers)
/// - API keys are STRIPPED from the output (only stored in profile)
/// - Deterministic ordering: provider keys sorted alphabetically
/// - Output is strict JSON with 2-space indentation
pub fn apply_zed_profile_for_home(home_dir: &Path, id: &str) -> Result<(), String> {
    let profile = load_profile_by_id(home_dir, id)?;

    // Determine effective config path
    let config_path = zed_config_path_for_home(home_dir)?;

    // Read existing settings.json (strict: malformed JSON returns error)
    let mut config = read_settings_json(&config_path)?;

    // Ensure config is an object
    let config_obj = config
        .as_object_mut()
        .ok_or_else(|| "settings.json root is not an object".to_string())?;

    // Ensure language_models exists and is an object
    let language_models = config_obj
        .entry("language_models")
        .or_insert_with(|| json!({}));

    // If language_models is not an object (e.g., it's a string), replace it
    if !language_models.is_object() {
        *language_models = json!({});
    }

    let lm_obj = language_models
        .as_object_mut()
        .ok_or_else(|| "language_models is not an object".to_string())?;

    // Ensure openai_compatible exists
    let openai_compatible = lm_obj
        .entry("openai_compatible")
        .or_insert_with(|| json!({}));

    // If openai_compatible is not an object (e.g., it's an array or string), replace it
    if !openai_compatible.is_object() {
        *openai_compatible = json!({});
    }

    let oc_obj = openai_compatible
        .as_object_mut()
        .ok_or_else(|| "openai_compatible is not an object".to_string())?;

    // For each profile provider, insert/update into openai_compatible (strip api_key)
    let mut provider_keys: Vec<&String> = profile.providers.keys().collect();
    provider_keys.sort(); // Deterministic ordering

    for provider_name in provider_keys {
        let provider_config = &profile.providers[provider_name];

        // Build the Zed-compatible provider entry (without api_key)
        let mut entry = Map::new();
        entry.insert(
            "api_url".to_string(),
            Value::String(provider_config.api_url.clone()),
        );

        // Add available_models if present
        if let Some(ref models) = provider_config.available_models {
            if !models.is_empty() {
                let models_value: Vec<Value> = models
                    .iter()
                    .map(|m| {
                        let mut model_entry = Map::new();
                        model_entry.insert("name".to_string(), Value::String(m.name.clone()));
                        if let Some(ref display_name) = m.display_name {
                            model_entry.insert(
                                "display_name".to_string(),
                                Value::String(display_name.clone()),
                            );
                        }
                        if let Some(max_tokens) = m.max_tokens {
                            model_entry.insert(
                                "max_tokens".to_string(),
                                Value::Number(serde_json::Number::from(max_tokens)),
                            );
                        }
                        if let Some(max_output) = m.max_output_tokens {
                            model_entry.insert(
                                "max_output_tokens".to_string(),
                                Value::Number(serde_json::Number::from(max_output)),
                            );
                        }
                        if let Some(max_completion) = m.max_completion_tokens {
                            model_entry.insert(
                                "max_completion_tokens".to_string(),
                                Value::Number(serde_json::Number::from(max_completion)),
                            );
                        }
                        if let Some(ref capabilities) = m.capabilities {
                            model_entry.insert(
                                "capabilities".to_string(),
                                serde_json::to_value(capabilities).unwrap_or_default(),
                            );
                        }
                        Value::Object(model_entry)
                    })
                    .collect();
                entry.insert("available_models".to_string(), Value::Array(models_value));
            }
        }

        oc_obj.insert(provider_name.clone(), Value::Object(entry));
    }

    // Write the result as strict JSON with 2-space indentation
    let output = serde_json::to_string_pretty(&config)
        .map_err(|e| format!("Failed to serialize config: {e}"))?;
    storage::atomic_write(&config_path, output.as_bytes())?;

    // Set active profile
    set_active_zed_profile_id_for_home(home_dir, id)?;

    Ok(())
}

// ============================================================================
// Config status & read current
// ============================================================================

/// Get the status of the live Zed configuration.
pub fn get_zed_config_status_for_home(home_dir: &Path) -> Result<ZedConfigStatus, String> {
    let config_path = zed_config_path_for_home(home_dir)?;
    let config_exists = config_path.exists();

    let has_openai_compatible = if config_exists {
        match read_settings_json(&config_path) {
            Ok(config) => config
                .get("language_models")
                .and_then(|lm| lm.get("openai_compatible"))
                .and_then(|oc| oc.as_object())
                .map(|obj| !obj.is_empty())
                .unwrap_or(false),
            Err(_) => false,
        }
    } else {
        false
    };

    Ok(ZedConfigStatus {
        config_exists,
        has_openai_compatible,
        config_path: config_path.to_string_lossy().to_string(),
    })
}

/// Read the current live Zed configuration, extracting all openai_compatible providers.
pub fn read_zed_current_config_for_home(home_dir: &Path) -> Result<ZedCurrentConfig, String> {
    let config_path = zed_config_path_for_home(home_dir)?;

    if !config_path.exists() {
        return Ok(ZedCurrentConfig::default());
    }

    let config = read_settings_json(&config_path)?;

    let providers = config
        .get("language_models")
        .and_then(|lm| lm.get("openai_compatible"))
        .and_then(|oc| oc.as_object())
        .map(|obj| {
            obj.iter()
                .filter_map(|(name, value)| {
                    let api_url = value
                        .get("api_url")
                        .and_then(|v| v.as_str())
                        .map(|s| s.to_string())?;

                    let available_models = value
                        .get("available_models")
                        .and_then(|v| v.as_array())
                        .map(|arr| {
                            arr.iter()
                                .filter_map(|m| {
                                    let name = m.get("name").and_then(|n| n.as_str())?;
                                    let capabilities = m.get("capabilities").and_then(|c| {
                                        Some(ZedModelCapabilities {
                                            tools: c.get("tools")?.as_bool().unwrap_or(false),
                                            images: c.get("images")?.as_bool().unwrap_or(false),
                                            parallel_tool_calls: c
                                                .get("parallel_tool_calls")
                                                .and_then(|v| v.as_bool())
                                                .unwrap_or(false),
                                            prompt_cache_key: c
                                                .get("prompt_cache_key")
                                                .and_then(|v| v.as_bool())
                                                .unwrap_or(false),
                                            chat_completions: c
                                                .get("chat_completions")
                                                .and_then(|v| v.as_bool())
                                                .unwrap_or(false),
                                            interleaved_reasoning: c
                                                .get("interleaved_reasoning")
                                                .and_then(|v| v.as_bool())
                                                .unwrap_or(false),
                                        })
                                    });
                                    Some(ZedModel {
                                        name: name.to_string(),
                                        display_name: m
                                            .get("display_name")
                                            .and_then(|d| d.as_str())
                                            .map(|s| s.to_string()),
                                        max_tokens: m
                                            .get("max_tokens")
                                            .and_then(|t| t.as_u64())
                                            .map(|t| t as u32),
                                        max_output_tokens: m
                                            .get("max_output_tokens")
                                            .and_then(|t| t.as_u64())
                                            .map(|t| t as u32),
                                        max_completion_tokens: m
                                            .get("max_completion_tokens")
                                            .and_then(|t| t.as_u64())
                                            .map(|t| t as u32),
                                        capabilities,
                                    })
                                })
                                .collect::<Vec<_>>()
                        });

                    Some((
                        name.clone(),
                        ZedProviderConfig {
                            api_url,
                            available_models,
                            api_key: None,
                        },
                    ))
                })
                .collect::<HashMap<_, _>>()
        })
        .unwrap_or_default();

    Ok(ZedCurrentConfig { providers })
}

/// Derive the expected environment variable name for a provider's API key.
/// Convention: uppercased provider name, spaces → underscores, suffixed with `_API_KEY`.
pub fn derive_env_var_name(provider_name: &str) -> String {
    let upper = provider_name.to_uppercase();
    let sanitized = upper
        .chars()
        .map(|c| if c == ' ' { '_' } else { c })
        .collect::<String>();
    format!("{sanitized}_API_KEY")
}

// ============================================================================
// System wrappers (use system home dir)
// ============================================================================

fn system_home_dir() -> Result<PathBuf, String> {
    dirs::home_dir().ok_or_else(|| "Failed to get home directory".to_string())
}

pub fn list_zed_profiles() -> Result<Vec<ZedProfile>, String> {
    list_zed_profiles_for_home(&system_home_dir()?)
}

pub fn get_zed_profile(id: &str) -> Result<ZedProfile, String> {
    get_zed_profile_for_home(&system_home_dir()?, id)
}

pub fn save_zed_profile(profile: ZedProfile) -> Result<(), String> {
    save_zed_profile_for_home(&system_home_dir()?, profile)
}

pub fn delete_zed_profile(id: &str) -> Result<(), String> {
    delete_zed_profile_for_home(&system_home_dir()?, id)
}

pub fn duplicate_zed_profile(id: &str, new_name: &str) -> Result<ZedProfile, String> {
    duplicate_zed_profile_for_home(&system_home_dir()?, id, new_name)
}

pub fn create_default_zed_profile() -> Result<ZedProfile, String> {
    create_default_zed_profile_for_home(&system_home_dir()?)
}

pub fn get_active_zed_profile_id() -> Result<Option<String>, String> {
    get_active_zed_profile_id_for_home(&system_home_dir()?)
}

pub fn apply_zed_profile(id: &str) -> Result<(), String> {
    apply_zed_profile_for_home(&system_home_dir()?, id)
}

pub fn get_zed_config_status() -> Result<ZedConfigStatus, String> {
    get_zed_config_status_for_home(&system_home_dir()?)
}

pub fn read_zed_current_config() -> Result<ZedCurrentConfig, String> {
    read_zed_current_config_for_home(&system_home_dir()?)
}

// ============================================================================
// Tests
// ============================================================================

#[cfg(test)]
mod tests {
    use super::*;
    use std::fs;
    use tempfile::TempDir;

    fn setup_home() -> (TempDir, PathBuf) {
        let tmp = TempDir::new().expect("Failed to create temp dir");
        let home = tmp.path().to_path_buf();
        (tmp, home)
    }

    fn create_test_provider(_name: &str, api_url: &str) -> ZedProviderConfig {
        ZedProviderConfig {
            api_url: api_url.to_string(),
            available_models: Some(vec![
                ZedModel {
                    name: "gpt-4".to_string(),
                    display_name: Some("GPT-4".to_string()),
                    max_tokens: Some(8192),
                    max_output_tokens: None,
                    max_completion_tokens: None,
                    capabilities: None,
                },
                ZedModel {
                    name: "gpt-3.5-turbo".to_string(),
                    display_name: None,
                    max_tokens: Some(4096),
                    max_output_tokens: None,
                    max_completion_tokens: None,
                    capabilities: None,
                },
            ]),
            api_key: Some("sk-test-key".to_string()),
        }
    }

    fn create_test_provider_no_key(_name: &str, api_url: &str) -> ZedProviderConfig {
        ZedProviderConfig {
            api_url: api_url.to_string(),
            available_models: Some(vec![ZedModel {
                name: "claude-3".to_string(),
                display_name: None,
                max_tokens: None,
                max_output_tokens: None,
                max_completion_tokens: None,
                capabilities: None,
            }]),
            api_key: None,
        }
    }

    fn create_test_profile(
        _home_dir: &Path,
        name: &str,
        providers: HashMap<String, ZedProviderConfig>,
    ) -> ZedProfile {
        let now = now_rfc3339();
        ZedProfile {
            id: Uuid::new_v4().to_string(),
            name: name.to_string(),
            description: None,
            created_at: now.clone(),
            updated_at: now,
            providers,
            api_keys: None,
        }
    }

    // ========================================================================
    // Profile CRUD tests
    // ========================================================================

    #[test]
    fn test_create_and_list_profiles_alphabetic() {
        let (_tmp, home) = setup_home();

        let mut providers1 = HashMap::new();
        providers1.insert(
            "openai".to_string(),
            create_test_provider("openai", "https://api.openai.com/v1"),
        );
        let profile1 = create_test_profile(&home, "Work Profile", providers1);
        save_zed_profile_for_home(&home, profile1).unwrap();

        let mut providers2 = HashMap::new();
        providers2.insert(
            "anthropic".to_string(),
            create_test_provider("anthropic", "https://api.anthropic.com"),
        );
        let profile2 = create_test_profile(&home, "Alpha Profile", providers2);
        save_zed_profile_for_home(&home, profile2).unwrap();

        let mut providers3 = HashMap::new();
        providers3.insert(
            "gemini".to_string(),
            create_test_provider("gemini", "https://generativelanguage.googleapis.com"),
        );
        let profile3 = create_test_profile(&home, "beta profile", providers3);
        save_zed_profile_for_home(&home, profile3).unwrap();

        let profiles = list_zed_profiles_for_home(&home).unwrap();
        assert_eq!(profiles.len(), 3);
        // Verify alphabetical ordering (case-insensitive)
        assert_eq!(profiles[0].name, "Alpha Profile");
        assert_eq!(profiles[1].name, "beta profile");
        assert_eq!(profiles[2].name, "Work Profile");
    }

    #[test]
    fn test_save_profile_generates_uuid() {
        let (_tmp, home) = setup_home();

        let mut providers = HashMap::new();
        providers.insert(
            "openai".to_string(),
            create_test_provider("openai", "https://api.openai.com/v1"),
        );

        // Save profile with empty ID
        let mut profile = create_test_profile(&home, "Test", providers.clone());
        profile.id = String::new();
        profile.created_at = String::new();
        save_zed_profile_for_home(&home, profile).unwrap();

        let profiles = list_zed_profiles_for_home(&home).unwrap();
        assert_eq!(profiles.len(), 1);
        let saved = &profiles[0];
        assert!(!saved.id.is_empty());
        assert!(!saved.created_at.is_empty());
        assert!(!saved.updated_at.is_empty());
        assert_eq!(saved.name, "Test");

        // Save again with same ID should preserve created_at
        let mut updated = saved.clone();
        updated.name = "Renamed".to_string();
        save_zed_profile_for_home(&home, updated).unwrap();

        let profiles = list_zed_profiles_for_home(&home).unwrap();
        assert_eq!(profiles.len(), 1);
        assert_eq!(profiles[0].name, "Renamed");
        assert_eq!(profiles[0].created_at, saved.created_at);
        assert!(
            profiles[0].updated_at > saved.updated_at || profiles[0].updated_at == saved.updated_at
        );
    }

    #[test]
    fn test_get_profile_by_id() {
        let (_tmp, home) = setup_home();

        let mut providers = HashMap::new();
        providers.insert(
            "openai".to_string(),
            create_test_provider("openai", "https://api.openai.com/v1"),
        );
        let profile = create_test_profile(&home, "Test", providers);
        let id = profile.id.clone();
        save_zed_profile_for_home(&home, profile).unwrap();

        let loaded = get_zed_profile_for_home(&home, &id).unwrap();
        assert_eq!(loaded.id, id);
        assert_eq!(loaded.name, "Test");

        // Non-existent ID
        let err = get_zed_profile_for_home(&home, "nonexistent").unwrap_err();
        assert!(err.contains("Failed to read profile") || err.contains("No such file"));
    }

    #[test]
    fn test_delete_profile_clears_active() {
        let (_tmp, home) = setup_home();

        let mut providers = HashMap::new();
        providers.insert(
            "openai".to_string(),
            create_test_provider("openai", "https://api.openai.com/v1"),
        );
        let profile = create_test_profile(&home, "ToDelete", providers);
        let id = profile.id.clone();
        save_zed_profile_for_home(&home, profile).unwrap();

        // Set as active
        let active_path = active_profile_path_for_home(&home).unwrap();
        fs::write(&active_path, &id).unwrap();

        // Delete
        delete_zed_profile_for_home(&home, &id).unwrap();

        // Verify profile file is gone
        let profiles = list_zed_profiles_for_home(&home).unwrap();
        assert!(profiles.iter().all(|p| p.id != id));

        // Verify active profile file is removed
        assert!(!active_path.exists());
    }

    #[test]
    fn test_duplicate_profile_new_uuid() {
        let (_tmp, home) = setup_home();

        let mut providers = HashMap::new();
        providers.insert(
            "openai".to_string(),
            create_test_provider("openai", "https://api.openai.com/v1"),
        );
        let profile = create_test_profile(&home, "Original", providers.clone());
        let id = profile.id.clone();
        save_zed_profile_for_home(&home, profile).unwrap();

        let duplicated = duplicate_zed_profile_for_home(&home, &id, "Original (Copy)").unwrap();

        assert_ne!(duplicated.id, id);
        assert_eq!(duplicated.name, "Original (Copy)");
        assert_eq!(duplicated.providers.len(), 1);
        assert!(duplicated.providers.contains_key("openai"));
        assert_eq!(
            duplicated.providers["openai"].api_url,
            "https://api.openai.com/v1"
        );

        // Verify both profiles exist
        let profiles = list_zed_profiles_for_home(&home).unwrap();
        assert_eq!(profiles.len(), 2);
    }

    #[test]
    fn test_create_default_profile_first_time() {
        let (_tmp, home) = setup_home();

        let profile = create_default_zed_profile_for_home(&home).unwrap();
        assert_eq!(profile.name, "Default");
        assert!(!profile.id.is_empty());

        // Active should be set
        let active = get_active_zed_profile_id_for_home(&home).unwrap();
        assert_eq!(active, Some(profile.id));
    }

    #[test]
    fn test_create_default_profile_when_exists() {
        let (_tmp, home) = setup_home();

        let mut providers = HashMap::new();
        providers.insert(
            "openai".to_string(),
            create_test_provider("openai", "https://api.openai.com/v1"),
        );
        let profile = create_test_profile(&home, "Existing", providers);
        save_zed_profile_for_home(&home, profile).unwrap();

        let err = create_default_zed_profile_for_home(&home).unwrap_err();
        assert!(err.contains("already exist"));
    }

    // ========================================================================
    // Config apply tests
    // ========================================================================

    #[test]
    fn test_apply_merge_preserves_other_keys() {
        let (_tmp, home) = setup_home();

        // Create existing settings.json with non-openai_compatible settings
        let config_path = zed_config_path_for_home(&home).unwrap();
        fs::create_dir_all(config_path.parent().unwrap()).unwrap();
        let existing_config = json!({
            "theme": "one-dark",
            "vim_mode": true,
            "buffer_font_size": 14,
            "ui_font_size": 16,
            "languages": {
                "Python": {
                    "tab_size": 4
                }
            },
            "telemetry": {
                "metrics": false
            }
        });
        fs::write(
            &config_path,
            serde_json::to_string_pretty(&existing_config).unwrap(),
        )
        .unwrap();

        // Create and save a profile
        let mut providers = HashMap::new();
        providers.insert(
            "openai".to_string(),
            create_test_provider("openai", "https://api.openai.com/v1"),
        );
        let profile = create_test_profile(&home, "Test", providers);
        let id = profile.id.clone();
        save_zed_profile_for_home(&home, profile).unwrap();

        // Apply
        apply_zed_profile_for_home(&home, &id).unwrap();

        // Read back
        let result = read_settings_json(&config_path).unwrap();
        let obj = result.as_object().unwrap();

        // Verify non-openai_compatible keys preserved
        assert_eq!(obj["theme"], "one-dark");
        assert_eq!(obj["vim_mode"], true);
        assert_eq!(obj["buffer_font_size"], 14);
        assert_eq!(obj["ui_font_size"], 16);
        assert_eq!(obj["languages"]["Python"]["tab_size"], 4);
        assert_eq!(obj["telemetry"]["metrics"], false);

        // Verify openai_compatible was added
        let oc = &obj["language_models"]["openai_compatible"];
        assert!(oc.is_object());
        assert!(oc.get("openai").is_some());
    }

    #[test]
    fn test_apply_merge_preserves_external_providers() {
        let (_tmp, home) = setup_home();

        // Create settings.json with an external provider
        let config_path = zed_config_path_for_home(&home).unwrap();
        fs::create_dir_all(config_path.parent().unwrap()).unwrap();
        let existing_config = json!({
            "theme": "one-dark",
            "language_models": {
                "openai_compatible": {
                    "external-sdk": {
                        "api_url": "https://custom-sdk.example.com/v1",
                        "available_models": [
                            { "name": "custom-model" }
                        ]
                    }
                }
            }
        });
        fs::write(
            &config_path,
            serde_json::to_string_pretty(&existing_config).unwrap(),
        )
        .unwrap();

        // Create profile with a different provider
        let mut providers = HashMap::new();
        providers.insert(
            "openai".to_string(),
            create_test_provider("openai", "https://api.openai.com/v1"),
        );
        let profile = create_test_profile(&home, "Test", providers);
        let id = profile.id.clone();
        save_zed_profile_for_home(&home, profile).unwrap();

        // Apply
        apply_zed_profile_for_home(&home, &id).unwrap();

        // Read back — external provider should survive
        let result = read_settings_json(&config_path).unwrap();
        let oc = &result["language_models"]["openai_compatible"];
        assert!(oc.get("external-sdk").is_some());
        assert!(oc.get("openai").is_some());
        assert_eq!(
            oc["external-sdk"]["api_url"],
            "https://custom-sdk.example.com/v1"
        );
    }

    #[test]
    fn test_apply_strips_api_keys() {
        let (_tmp, home) = setup_home();

        // Create a provider with api_key
        let mut providers = HashMap::new();
        providers.insert(
            "openai".to_string(),
            create_test_provider("openai", "https://api.openai.com/v1"),
        );
        let profile = create_test_profile(&home, "Test", providers);
        let id = profile.id.clone();
        save_zed_profile_for_home(&home, profile).unwrap();

        // Apply
        apply_zed_profile_for_home(&home, &id).unwrap();

        // Read settings.json and verify NO api_key
        let config_path = zed_config_path_for_home(&home).unwrap();
        let result = read_settings_json(&config_path).unwrap();
        let oc = &result["language_models"]["openai_compatible"];

        // The openai provider entry should NOT have api_key
        let openai_entry = oc.get("openai").unwrap();
        assert!(
            openai_entry.get("api_key").is_none(),
            "api_key should be stripped from settings.json"
        );
        assert_eq!(openai_entry["api_url"], "https://api.openai.com/v1");
    }

    #[test]
    fn test_apply_creates_missing_file_and_directory() {
        let (_tmp, home) = setup_home();

        // Ensure no config directory exists
        let config_path = zed_config_path_for_home(&home).unwrap();
        assert!(!config_path.parent().unwrap().exists());

        // Create and save a profile
        let mut providers = HashMap::new();
        providers.insert(
            "openai".to_string(),
            create_test_provider("openai", "https://api.openai.com/v1"),
        );
        let profile = create_test_profile(&home, "Test", providers);
        let id = profile.id.clone();
        save_zed_profile_for_home(&home, profile).unwrap();

        // Apply
        apply_zed_profile_for_home(&home, &id).unwrap();

        // Verify file and directory were created
        assert!(config_path.exists());
        assert!(config_path.parent().unwrap().exists());

        // Verify content
        let result = read_settings_json(&config_path).unwrap();
        let oc = &result["language_models"]["openai_compatible"];
        assert!(oc.get("openai").is_some());
        assert_eq!(oc["openai"]["api_url"], "https://api.openai.com/v1");
    }

    #[test]
    fn test_apply_malformed_json_error() {
        let (_tmp, home) = setup_home();

        // Create settings.json with malformed JSON
        let config_path = zed_config_path_for_home(&home).unwrap();
        fs::create_dir_all(config_path.parent().unwrap()).unwrap();
        fs::write(&config_path, r#"{ "theme": "broken", }"#).unwrap(); // Trailing comma

        // Create and save a profile
        let mut providers = HashMap::new();
        providers.insert(
            "openai".to_string(),
            create_test_provider("openai", "https://api.openai.com/v1"),
        );
        let profile = create_test_profile(&home, "Test", providers);
        let id = profile.id.clone();
        save_zed_profile_for_home(&home, profile).unwrap();

        // Apply should return error and NOT overwrite
        let err = apply_zed_profile_for_home(&home, &id).unwrap_err();
        assert!(
            err.contains("Invalid JSON")
                || err.contains("trailing comma")
                || err.contains("control character"),
            "Error should mention malformed JSON: {err}"
        );

        // Original file should be unchanged
        let content = fs::read_to_string(&config_path).unwrap();
        assert_eq!(content, r#"{ "theme": "broken", }"#);
    }

    #[test]
    fn test_apply_idempotent() {
        let (_tmp, home) = setup_home();

        // Create settings.json with some existing content
        let config_path = zed_config_path_for_home(&home).unwrap();
        fs::create_dir_all(config_path.parent().unwrap()).unwrap();
        let existing_config = json!({
            "theme": "one-dark",
            "language_models": {
                "openai_compatible": {
                    "old-provider": {
                        "api_url": "https://old.example.com"
                    }
                }
            }
        });
        fs::write(
            &config_path,
            serde_json::to_string_pretty(&existing_config).unwrap(),
        )
        .unwrap();

        // Create a profile
        let mut providers = HashMap::new();
        providers.insert(
            "openai".to_string(),
            create_test_provider("openai", "https://api.openai.com/v1"),
        );
        let profile = create_test_profile(&home, "Test", providers);
        let id = profile.id.clone();
        save_zed_profile_for_home(&home, profile).unwrap();

        // Apply twice
        apply_zed_profile_for_home(&home, &id).unwrap();
        let result1 = fs::read_to_string(&config_path).unwrap();

        apply_zed_profile_for_home(&home, &id).unwrap();
        let result2 = fs::read_to_string(&config_path).unwrap();

        // Should be byte-identical
        assert_eq!(
            result1, result2,
            "Two consecutive applies should produce identical output"
        );
    }

    #[test]
    fn test_apply_with_empty_providers() {
        let (_tmp, home) = setup_home();

        // Create settings.json with existing providers
        let config_path = zed_config_path_for_home(&home).unwrap();
        fs::create_dir_all(config_path.parent().unwrap()).unwrap();
        let existing_config = json!({
            "theme": "one-dark",
            "language_models": {
                "openai_compatible": {
                    "existing-provider": {
                        "api_url": "https://existing.example.com"
                    }
                }
            }
        });
        fs::write(
            &config_path,
            serde_json::to_string_pretty(&existing_config).unwrap(),
        )
        .unwrap();

        // Create a profile with NO providers
        let profile = create_test_profile(&home, "Empty", HashMap::new());
        let id = profile.id.clone();
        save_zed_profile_for_home(&home, profile).unwrap();

        // Apply
        apply_zed_profile_for_home(&home, &id).unwrap();

        // Existing providers should survive
        let result = read_settings_json(&config_path).unwrap();
        let oc = &result["language_models"]["openai_compatible"];
        assert!(
            oc.get("existing-provider").is_some(),
            "Existing providers should survive empty apply"
        );
    }

    #[test]
    fn test_apply_non_object_openai_compatible() {
        let (_tmp, home) = setup_home();

        // Create settings.json with openai_compatible as array (invalid)
        let config_path = zed_config_path_for_home(&home).unwrap();
        fs::create_dir_all(config_path.parent().unwrap()).unwrap();
        let existing_config = json!({
            "theme": "one-dark",
            "language_models": {
                "openai_compatible": []
            }
        });
        fs::write(
            &config_path,
            serde_json::to_string_pretty(&existing_config).unwrap(),
        )
        .unwrap();

        // Create a profile
        let mut providers = HashMap::new();
        providers.insert(
            "openai".to_string(),
            create_test_provider("openai", "https://api.openai.com/v1"),
        );
        let profile = create_test_profile(&home, "Test", providers);
        let id = profile.id.clone();
        save_zed_profile_for_home(&home, profile).unwrap();

        // Apply — should replace the array with an object
        apply_zed_profile_for_home(&home, &id).unwrap();

        let result = read_settings_json(&config_path).unwrap();
        let oc = &result["language_models"]["openai_compatible"];
        assert!(oc.is_object(), "openai_compatible should now be an object");
        assert!(oc.get("openai").is_some());
        assert_eq!(result["theme"], "one-dark", "Other settings preserved");
    }

    #[test]
    fn test_apply_non_object_language_models_string() {
        let (_tmp, home) = setup_home();

        // Create settings.json with language_models as a string
        let config_path = zed_config_path_for_home(&home).unwrap();
        fs::create_dir_all(config_path.parent().unwrap()).unwrap();
        let existing_config = json!({
            "theme": "one-dark",
            "language_models": "some-string-value"
        });
        fs::write(
            &config_path,
            serde_json::to_string_pretty(&existing_config).unwrap(),
        )
        .unwrap();

        // Create a profile
        let mut providers = HashMap::new();
        providers.insert(
            "openai".to_string(),
            create_test_provider("openai", "https://api.openai.com/v1"),
        );
        let profile = create_test_profile(&home, "Test", providers);
        let id = profile.id.clone();
        save_zed_profile_for_home(&home, profile).unwrap();

        // Apply should work — will replace language_models with an object containing openai_compatible
        apply_zed_profile_for_home(&home, &id).unwrap();

        let result = read_settings_json(&config_path).unwrap();
        assert!(result["language_models"].is_object());
        assert!(result["language_models"]["openai_compatible"].is_object());
        assert_eq!(result["theme"], "one-dark");
    }

    #[test]
    fn test_apply_json_comments_stripped() {
        let (_tmp, home) = setup_home();

        let config_path = zed_config_path_for_home(&home).unwrap();
        fs::create_dir_all(config_path.parent().unwrap()).unwrap();
        // JSON with comments
        let content = r#"{
            // This is a single-line comment
            "theme": "one-dark",
            /* Multi-line
               comment */
            "buffer_font_size": 14
        }"#;
        fs::write(&config_path, content).unwrap();

        // Verify it can be read
        let parsed = read_settings_json(&config_path).unwrap();
        assert_eq!(parsed["theme"], "one-dark");
        assert_eq!(parsed["buffer_font_size"], 14);
    }

    #[test]
    fn test_apply_trailing_commas_error() {
        let (_tmp, home) = setup_home();

        let config_path = zed_config_path_for_home(&home).unwrap();
        fs::create_dir_all(config_path.parent().unwrap()).unwrap();
        // JSON with trailing comma
        fs::write(&config_path, r#"{"theme": "one-dark",}"#).unwrap();

        // Verify it returns an error
        let err = read_settings_json(&config_path).unwrap_err();
        assert!(
            err.contains("trailing") || err.contains("Invalid JSON"),
            "Error should mention trailing comma: {err}"
        );
    }

    #[test]
    fn test_apply_schema_provider_format() {
        let (_tmp, home) = setup_home();

        let mut providers = HashMap::new();
        providers.insert(
            "My Provider".to_string(),
            ZedProviderConfig {
                api_url: "https://api.example.com/v1".to_string(),
                available_models: Some(vec![
                    ZedModel {
                        name: "model-1".to_string(),
                        display_name: Some("Model One".to_string()),
                        max_tokens: Some(4096),
                        max_output_tokens: None,
                        max_completion_tokens: None,
                        capabilities: None,
                    },
                    ZedModel {
                        name: "model-2".to_string(),
                        display_name: None,
                        max_tokens: None,
                        max_output_tokens: None,
                        max_completion_tokens: None,
                        capabilities: None,
                    },
                ]),
                api_key: Some("sk-secret".to_string()),
            },
        );
        let profile = create_test_profile(&home, "Test", providers);
        let id = profile.id.clone();
        save_zed_profile_for_home(&home, profile).unwrap();

        apply_zed_profile_for_home(&home, &id).unwrap();

        let config_path = zed_config_path_for_home(&home).unwrap();
        let result = read_settings_json(&config_path).unwrap();
        let oc = &result["language_models"]["openai_compatible"];

        // Verify schema: key IS the provider name
        assert!(oc.get("My Provider").is_some());
        let entry = oc.get("My Provider").unwrap();

        // Verify api_url present
        assert_eq!(entry["api_url"], "https://api.example.com/v1");

        // Verify available_models
        let models = entry["available_models"].as_array().unwrap();
        assert_eq!(models.len(), 2);
        assert_eq!(models[0]["name"], "model-1");
        assert_eq!(models[0]["display_name"], "Model One");
        assert_eq!(models[0]["max_tokens"], 4096);
        assert_eq!(models[1]["name"], "model-2");
        assert!(models[1].get("display_name").is_none());
        assert!(models[1].get("max_tokens").is_none());

        // Verify NO api_key
        assert!(
            entry.get("api_key").is_none(),
            "api_key must not be in settings.json"
        );
    }

    // ========================================================================
    // Config status tests
    // ========================================================================

    #[test]
    fn test_config_status_missing() {
        let (_tmp, home) = setup_home();

        let status = get_zed_config_status_for_home(&home).unwrap();
        assert!(!status.config_exists);
        assert!(!status.has_openai_compatible);
    }

    #[test]
    fn test_config_status_no_openai_compatible() {
        let (_tmp, home) = setup_home();

        let config_path = zed_config_path_for_home(&home).unwrap();
        fs::create_dir_all(config_path.parent().unwrap()).unwrap();
        fs::write(&config_path, r#"{"theme": "one-dark"}"#).unwrap();

        let status = get_zed_config_status_for_home(&home).unwrap();
        assert!(status.config_exists);
        assert!(!status.has_openai_compatible);
    }

    #[test]
    fn test_config_status_with_openai_compatible() {
        let (_tmp, home) = setup_home();

        let config_path = zed_config_path_for_home(&home).unwrap();
        fs::create_dir_all(config_path.parent().unwrap()).unwrap();
        fs::write(
            &config_path,
            r#"{
                "language_models": {
                    "openai_compatible": {
                        "openai": { "api_url": "https://api.openai.com/v1" }
                    }
                }
            }"#,
        )
        .unwrap();

        let status = get_zed_config_status_for_home(&home).unwrap();
        assert!(status.config_exists);
        assert!(status.has_openai_compatible);
    }

    #[test]
    fn test_config_status_empty_openai_compatible() {
        let (_tmp, home) = setup_home();

        let config_path = zed_config_path_for_home(&home).unwrap();
        fs::create_dir_all(config_path.parent().unwrap()).unwrap();
        fs::write(
            &config_path,
            r#"{
                "language_models": {
                    "openai_compatible": {}
                }
            }"#,
        )
        .unwrap();

        let status = get_zed_config_status_for_home(&home).unwrap();
        assert!(status.config_exists);
        assert!(
            !status.has_openai_compatible,
            "Empty openai_compatible should report false"
        );
    }

    // ========================================================================
    // Read current config tests
    // ========================================================================

    #[test]
    fn test_read_current_config_empty() {
        let (_tmp, home) = setup_home();

        let current = read_zed_current_config_for_home(&home).unwrap();
        assert!(current.providers.is_empty());
    }

    #[test]
    fn test_read_current_config_extracts_providers() {
        let (_tmp, home) = setup_home();

        let config_path = zed_config_path_for_home(&home).unwrap();
        fs::create_dir_all(config_path.parent().unwrap()).unwrap();
        fs::write(
            &config_path,
            r#"{
                "theme": "one-dark",
                "language_models": {
                    "openai_compatible": {
                        "openai": {
                            "api_url": "https://api.openai.com/v1",
                            "available_models": [
                                { "name": "gpt-4", "max_tokens": 8192 },
                                { "name": "gpt-3.5-turbo" }
                            ]
                        },
                        "anthropic": {
                            "api_url": "https://api.anthropic.com"
                        }
                    }
                }
            }"#,
        )
        .unwrap();

        let current = read_zed_current_config_for_home(&home).unwrap();
        assert_eq!(current.providers.len(), 2);

        let openai = current.providers.get("openai").unwrap();
        assert_eq!(openai.api_url, "https://api.openai.com/v1");
        assert_eq!(openai.available_models.as_ref().unwrap().len(), 2);
        assert_eq!(openai.available_models.as_ref().unwrap()[0].name, "gpt-4");
        assert_eq!(
            openai.available_models.as_ref().unwrap()[0].max_tokens,
            Some(8192)
        );
        assert_eq!(
            openai.available_models.as_ref().unwrap()[1].name,
            "gpt-3.5-turbo"
        );
        assert_eq!(
            openai.available_models.as_ref().unwrap()[1].max_tokens,
            None
        );

        let anthropic = current.providers.get("anthropic").unwrap();
        assert_eq!(anthropic.api_url, "https://api.anthropic.com");
        assert!(anthropic.available_models.is_none());
    }

    // ========================================================================
    // Env var naming test
    // ========================================================================

    #[test]
    fn test_derive_env_var_name() {
        assert_eq!(derive_env_var_name("OpenRouter"), "OPENROUTER_API_KEY");
        assert_eq!(derive_env_var_name("openai"), "OPENAI_API_KEY");
        assert_eq!(derive_env_var_name("My Provider"), "MY_PROVIDER_API_KEY");
        assert_eq!(derive_env_var_name("Anthropic"), "ANTHROPIC_API_KEY");
        assert_eq!(derive_env_var_name("custom-sdk"), "CUSTOM-SDK_API_KEY");
        assert_eq!(derive_env_var_name(""), "_API_KEY");
    }

    // ========================================================================
    // Sequential apply tests (cross-flow)
    // ========================================================================

    #[test]
    fn test_sequential_apply_cumulative_merge() {
        let (_tmp, home) = setup_home();

        // Profile A with openai
        let mut providers_a = HashMap::new();
        providers_a.insert(
            "openai".to_string(),
            create_test_provider("openai", "https://api.openai.com/v1"),
        );
        let profile_a = create_test_profile(&home, "Profile A", providers_a);
        let id_a = profile_a.id.clone();
        save_zed_profile_for_home(&home, profile_a).unwrap();

        // Profile B with anthropic
        let mut providers_b = HashMap::new();
        providers_b.insert(
            "anthropic".to_string(),
            create_test_provider_no_key("anthropic", "https://api.anthropic.com"),
        );
        let profile_b = create_test_profile(&home, "Profile B", providers_b);
        let id_b = profile_b.id.clone();
        save_zed_profile_for_home(&home, profile_b).unwrap();

        // Apply A → should have openai
        apply_zed_profile_for_home(&home, &id_a).unwrap();
        let config_path = zed_config_path_for_home(&home).unwrap();
        let result_a = read_settings_json(&config_path).unwrap();
        let oc_a = &result_a["language_models"]["openai_compatible"];
        assert!(oc_a.get("openai").is_some(), "A should add openai");
        assert!(
            oc_a.get("anthropic").is_none(),
            "A should not have anthropic"
        );

        // Apply B → should have BOTH openai AND anthropic (MERGE)
        apply_zed_profile_for_home(&home, &id_b).unwrap();
        let result_b = read_settings_json(&config_path).unwrap();
        let oc_b = &result_b["language_models"]["openai_compatible"];
        assert!(oc_b.get("openai").is_some(), "openai from A should survive");
        assert!(
            oc_b.get("anthropic").is_some(),
            "anthropic from B should be added"
        );

        // External provider manually added
        // Simulate by adding an external provider to the config
        let mut config = result_b;
        let oc_obj = config["language_models"]["openai_compatible"]
            .as_object_mut()
            .unwrap();
        oc_obj.insert(
            "custom-sdk".to_string(),
            json!({"api_url": "https://custom.example.com"}),
        );
        fs::write(&config_path, serde_json::to_string_pretty(&config).unwrap()).unwrap();

        // Apply A again → all three should coexist
        apply_zed_profile_for_home(&home, &id_a).unwrap();
        let result_c = read_settings_json(&config_path).unwrap();
        let oc_c = &result_c["language_models"]["openai_compatible"];
        assert!(oc_c.get("openai").is_some(), "openai should still be there");
        assert!(
            oc_c.get("anthropic").is_some(),
            "anthropic should still be there"
        );
        assert!(
            oc_c.get("custom-sdk").is_some(),
            "custom-sdk external provider should survive"
        );
    }

    #[test]
    fn test_non_provider_settings_survive_all_apply_cycles() {
        let (_tmp, home) = setup_home();

        let config_path = zed_config_path_for_home(&home).unwrap();
        fs::create_dir_all(config_path.parent().unwrap()).unwrap();
        let initial = json!({
            "theme": "one-dark",
            "vim_mode": true,
            "ui_font_size": 16,
            "buffer_font_size": 14,
            "languages": {
                "Python": { "tab_size": 4 }
            },
            "lsp": {
                "rust-analyzer": { "checkOnSave": true }
            },
            "telemetry": { "metrics": false }
        });
        fs::write(
            &config_path,
            serde_json::to_string_pretty(&initial).unwrap(),
        )
        .unwrap();

        // Create two profiles and apply them sequentially
        let mut providers_a = HashMap::new();
        providers_a.insert(
            "openai".to_string(),
            create_test_provider("openai", "https://api.openai.com/v1"),
        );
        let profile_a = create_test_profile(&home, "A", providers_a);
        let id_a = profile_a.id.clone();
        save_zed_profile_for_home(&home, profile_a).unwrap();

        let mut providers_b = HashMap::new();
        providers_b.insert(
            "anthropic".to_string(),
            create_test_provider_no_key("anthropic", "https://api.anthropic.com"),
        );
        let profile_b = create_test_profile(&home, "B", providers_b);
        let id_b = profile_b.id.clone();
        save_zed_profile_for_home(&home, profile_b).unwrap();

        apply_zed_profile_for_home(&home, &id_a).unwrap();
        apply_zed_profile_for_home(&home, &id_b).unwrap();

        let result = read_settings_json(&config_path).unwrap();
        // All non-provider settings should be preserved
        assert_eq!(result["theme"], "one-dark");
        assert_eq!(result["vim_mode"], true);
        assert_eq!(result["ui_font_size"], 16);
        assert_eq!(result["buffer_font_size"], 14);
        assert_eq!(result["languages"]["Python"]["tab_size"], 4);
        assert_eq!(result["lsp"]["rust-analyzer"]["checkOnSave"], true);
        assert_eq!(result["telemetry"]["metrics"], false);
    }

    #[test]
    fn test_apply_creates_parent_directory() {
        let (_tmp, home) = setup_home();

        let config_path = zed_config_path_for_home(&home).unwrap();
        // Parent directory should not exist
        assert!(!config_path.parent().unwrap().exists());

        let mut providers = HashMap::new();
        providers.insert(
            "openai".to_string(),
            create_test_provider("openai", "https://api.openai.com/v1"),
        );
        let profile = create_test_profile(&home, "Test", providers);
        let id = profile.id.clone();
        save_zed_profile_for_home(&home, profile).unwrap();

        apply_zed_profile_for_home(&home, &id).unwrap();

        assert!(config_path.exists());
    }

    #[test]
    fn test_apply_atomic_write() {
        let (_tmp, home) = setup_home();

        let config_path = zed_config_path_for_home(&home).unwrap();
        fs::create_dir_all(config_path.parent().unwrap()).unwrap();
        // Valid JSON
        fs::write(&config_path, r#"{"theme": "one-dark"}"#).unwrap();

        let mut providers = HashMap::new();
        providers.insert(
            "openai".to_string(),
            create_test_provider("openai", "https://api.openai.com/v1"),
        );
        let profile = create_test_profile(&home, "Test", providers);
        let id = profile.id.clone();
        save_zed_profile_for_home(&home, profile).unwrap();

        // Apply should succeed (atomic write via storage::atomic_write)
        apply_zed_profile_for_home(&home, &id).unwrap();

        // File should contain valid JSON with both the old and new content
        let result = read_settings_json(&config_path).unwrap();
        assert_eq!(result["theme"], "one-dark");
    }

    #[test]
    fn test_empty_settings_file_handled() {
        let (_tmp, home) = setup_home();

        let config_path = zed_config_path_for_home(&home).unwrap();
        fs::create_dir_all(config_path.parent().unwrap()).unwrap();
        fs::write(&config_path, "").unwrap();

        let mut providers = HashMap::new();
        providers.insert(
            "openai".to_string(),
            create_test_provider("openai", "https://api.openai.com/v1"),
        );
        let profile = create_test_profile(&home, "Test", providers);
        let id = profile.id.clone();
        save_zed_profile_for_home(&home, profile).unwrap();

        apply_zed_profile_for_home(&home, &id).unwrap();

        let result = read_settings_json(&config_path).unwrap();
        assert!(result.get("language_models").is_some());
    }
}
