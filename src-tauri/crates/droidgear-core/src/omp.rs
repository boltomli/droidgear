//! OMP (Oh My Pi) configuration management (core).
//!
//! Provides type definitions and path helpers for OMP's `~/.omp/agent/models.yml`
//! configuration. OMP is a fork of Pi; the provider-model hierarchy is
//! structurally identical but stored as YAML with a few OMP-specific fields.

use chrono::Utc;
use serde::{Deserialize, Serialize};
use specta::Type;
use std::collections::HashMap;
use std::path::{Path, PathBuf};
use uuid::Uuid;

use crate::{paths, storage};

// ============================================================================
// Types
// ============================================================================

/// OMP model cost tier. Rates are in dollars per million tokens.
#[derive(Debug, Clone, Serialize, Deserialize, Type)]
#[serde(rename_all = "camelCase")]
pub struct OmpModelCostTier {
    pub input_tokens_above: u32,
    pub input: f64,
    pub output: f64,
    pub cache_read: f64,
    pub cache_write: f64,
}

/// OMP model cost configuration
#[derive(Debug, Clone, Default, Serialize, Deserialize, Type)]
#[serde(rename_all = "camelCase")]
pub struct OmpModelCost {
    #[serde(default)]
    pub input: f64,
    #[serde(default)]
    pub output: f64,
    #[serde(default)]
    pub cache_read: f64,
    #[serde(default)]
    pub cache_write: f64,
    #[serde(skip_serializing_if = "Option::is_none")]
    pub tiers: Option<Vec<OmpModelCostTier>>,
    #[serde(flatten)]
    #[specta(skip)]
    pub extra: HashMap<String, serde_json::Value>,
}

/// OMP compatibility configuration. Unknown fields are retained for newer OMP versions.
#[derive(Debug, Clone, Default, Serialize, Deserialize, Type)]
#[serde(rename_all = "camelCase")]
pub struct OmpCompatConfig {
    #[serde(skip_serializing_if = "Option::is_none")]
    pub supports_store: Option<bool>,
    #[serde(skip_serializing_if = "Option::is_none")]
    pub supports_developer_role: Option<bool>,
    #[serde(skip_serializing_if = "Option::is_none")]
    pub supports_reasoning_effort: Option<bool>,
    #[serde(skip_serializing_if = "Option::is_none")]
    pub reasoning_effort_map: Option<HashMap<String, serde_json::Value>>,
    #[serde(skip_serializing_if = "Option::is_none")]
    pub supports_usage_in_streaming: Option<bool>,
    #[serde(skip_serializing_if = "Option::is_none")]
    pub max_tokens_field: Option<String>,
    #[serde(skip_serializing_if = "Option::is_none")]
    pub requires_tool_result_name: Option<bool>,
    #[serde(skip_serializing_if = "Option::is_none")]
    pub requires_assistant_after_tool_result: Option<bool>,
    #[serde(skip_serializing_if = "Option::is_none")]
    pub requires_thinking_as_text: Option<bool>,
    #[serde(skip_serializing_if = "Option::is_none")]
    pub requires_reasoning_content_on_assistant_messages: Option<bool>,
    #[serde(skip_serializing_if = "Option::is_none")]
    pub thinking_format: Option<String>,
    #[serde(skip_serializing_if = "Option::is_none")]
    pub cache_control_format: Option<String>,
    #[serde(skip_serializing_if = "Option::is_none")]
    pub supports_strict_mode: Option<bool>,
    #[serde(skip_serializing_if = "Option::is_none")]
    pub supports_long_cache_retention: Option<bool>,
    #[serde(skip_serializing_if = "Option::is_none")]
    pub supports_eager_tool_input_streaming: Option<bool>,
    #[serde(skip_serializing_if = "Option::is_none")]
    pub open_router_routing: Option<serde_json::Value>,
    #[serde(skip_serializing_if = "Option::is_none")]
    pub vercel_gateway_routing: Option<serde_json::Value>,
    #[serde(flatten)]
    #[specta(skip)]
    pub extra: HashMap<String, serde_json::Value>,
}

/// OMP model definition
#[derive(Debug, Clone, Serialize, Deserialize, Type)]
#[serde(rename_all = "camelCase")]
pub struct OmpModel {
    pub id: String,
    #[serde(skip_serializing_if = "Option::is_none")]
    pub name: Option<String>,
    #[serde(skip_serializing_if = "Option::is_none")]
    pub api: Option<String>,
    #[serde(default)]
    pub reasoning: bool,
    #[serde(skip_serializing_if = "Option::is_none")]
    pub thinking_level_map: Option<HashMap<String, Option<String>>>,
    #[serde(default = "default_input")]
    pub input: Vec<String>,
    #[serde(default = "default_context_window")]
    pub context_window: u32,
    #[serde(default = "default_max_tokens")]
    pub max_tokens: u32,
    #[serde(skip_serializing_if = "Option::is_none")]
    pub cost: Option<OmpModelCost>,
    #[serde(skip_serializing_if = "Option::is_none")]
    pub headers: Option<HashMap<String, String>>,
    #[serde(skip_serializing_if = "Option::is_none")]
    pub compat: Option<OmpCompatConfig>,
    /// OMP-specific: when context exceeds contextWindow, swap to this model before fallback.
    #[serde(skip_serializing_if = "Option::is_none")]
    pub context_promotion_target: Option<String>,
    #[serde(flatten)]
    #[specta(skip)]
    pub extra: HashMap<String, serde_json::Value>,
}

impl Default for OmpModel {
    fn default() -> Self {
        Self {
            id: String::new(),
            name: None,
            api: None,
            reasoning: false,
            thinking_level_map: None,
            input: default_input(),
            context_window: default_context_window(),
            max_tokens: default_max_tokens(),
            cost: None,
            headers: None,
            compat: None,
            context_promotion_target: None,
            extra: HashMap::new(),
        }
    }
}

/// OMP model override (subset of OmpModel fields for overriding built-in models)
#[derive(Debug, Clone, Default, Serialize, Deserialize, Type)]
#[serde(rename_all = "camelCase")]
pub struct OmpModelOverride {
    #[serde(skip_serializing_if = "Option::is_none")]
    pub name: Option<String>,
    #[serde(skip_serializing_if = "Option::is_none")]
    pub api: Option<String>,
    #[serde(skip_serializing_if = "Option::is_none")]
    pub reasoning: Option<bool>,
    #[serde(skip_serializing_if = "Option::is_none")]
    pub thinking_level_map: Option<HashMap<String, Option<String>>>,
    #[serde(skip_serializing_if = "Option::is_none")]
    pub input: Option<Vec<String>>,
    #[serde(skip_serializing_if = "Option::is_none")]
    pub context_window: Option<u32>,
    #[serde(skip_serializing_if = "Option::is_none")]
    pub max_tokens: Option<u32>,
    #[serde(skip_serializing_if = "Option::is_none")]
    pub cost: Option<OmpModelCost>,
    #[serde(skip_serializing_if = "Option::is_none")]
    pub headers: Option<HashMap<String, String>>,
    #[serde(skip_serializing_if = "Option::is_none")]
    pub compat: Option<OmpCompatConfig>,
    #[serde(skip_serializing_if = "Option::is_none")]
    pub context_promotion_target: Option<String>,
    #[serde(flatten)]
    #[specta(skip)]
    pub extra: HashMap<String, serde_json::Value>,
}

/// OMP discovery configuration for live model listing
#[derive(Debug, Clone, Default, Serialize, Deserialize, Type)]
#[serde(rename_all = "camelCase")]
pub struct OmpDiscovery {
    #[serde(rename = "type")]
    pub discovery_type: String,
    #[serde(flatten)]
    #[specta(skip)]
    pub extra: HashMap<String, serde_json::Value>,
}

/// OMP provider configuration
#[derive(Debug, Clone, Default, Serialize, Deserialize, Type)]
#[serde(rename_all = "camelCase")]
pub struct OmpProviderConfig {
    #[serde(skip_serializing_if = "Option::is_none")]
    pub base_url: Option<String>,
    #[serde(skip_serializing_if = "Option::is_none")]
    pub api: Option<String>,
    #[serde(skip_serializing_if = "Option::is_none")]
    pub api_key: Option<String>,
    #[serde(skip_serializing_if = "Option::is_none")]
    pub oauth: Option<String>,
    /// OMP-specific: auth scheme (apiKey, none, oauth)
    #[serde(skip_serializing_if = "Option::is_none")]
    pub auth: Option<String>,
    #[serde(skip_serializing_if = "Option::is_none")]
    pub headers: Option<HashMap<String, String>>,
    #[serde(skip_serializing_if = "Option::is_none")]
    pub auth_header: Option<bool>,
    #[serde(default, skip_serializing_if = "Vec::is_empty")]
    pub models: Vec<OmpModel>,
    #[serde(skip_serializing_if = "Option::is_none")]
    pub model_overrides: Option<HashMap<String, OmpModelOverride>>,
    #[serde(skip_serializing_if = "Option::is_none")]
    pub compat: Option<OmpCompatConfig>,
    /// OMP-specific: disable strict tool schema
    #[serde(skip_serializing_if = "Option::is_none")]
    pub disable_strict_tools: Option<bool>,
    /// OMP-specific: live model discovery
    #[serde(skip_serializing_if = "Option::is_none")]
    pub discovery: Option<OmpDiscovery>,
    #[serde(flatten)]
    #[specta(skip)]
    pub extra: HashMap<String, serde_json::Value>,
}

/// OMP profile (stored in DroidGear)
#[derive(Debug, Clone, Serialize, Deserialize, Type)]
#[serde(rename_all = "camelCase")]
pub struct OmpProfile {
    pub id: String,
    pub name: String,
    #[serde(skip_serializing_if = "Option::is_none")]
    pub description: Option<String>,
    pub created_at: String,
    pub updated_at: String,
    #[serde(default)]
    pub providers: HashMap<String, OmpProviderConfig>,
}

/// OMP config status
#[derive(Debug, Clone, Serialize, Deserialize, Type)]
#[serde(rename_all = "camelCase")]
pub struct OmpConfigStatus {
    pub config_exists: bool,
    pub config_path: String,
}

/// Current OMP configuration (from `~/.omp/agent/models.yml`)
#[derive(Debug, Clone, Default, Serialize, Deserialize, Type)]
#[serde(rename_all = "camelCase")]
pub struct OmpCurrentConfig {
    #[serde(default)]
    pub providers: HashMap<String, OmpProviderConfig>,
}

// ============================================================================
// Default value functions
// ============================================================================

fn default_input() -> Vec<String> {
    vec!["text".to_string()]
}

fn default_context_window() -> u32 {
    128000
}

fn default_max_tokens() -> u32 {
    16384
}

// ============================================================================
// DroidGear Model Registry
// ============================================================================

const MODEL_REGISTRY_JSON: &str = include_str!("../../../../src/lib/model-registry-data.json");

#[derive(Deserialize)]
#[serde(rename_all = "camelCase")]
struct RegistryModel {
    id: String,
    name: String,
    #[serde(default)]
    aliases: Vec<String>,
    reasoning: bool,
    input: Vec<String>,
    #[serde(default)]
    thinking_level_map: Option<HashMap<String, Option<String>>>,
    context_window: u32,
    max_output_tokens: Option<u32>,
}

fn registry_models() -> &'static [RegistryModel] {
    static MODELS: std::sync::OnceLock<Vec<RegistryModel>> = std::sync::OnceLock::new();
    MODELS.get_or_init(|| serde_json::from_str(MODEL_REGISTRY_JSON).unwrap_or_default())
}

pub fn enrich_omp_model_from_registry(model: &mut OmpModel) -> bool {
    let Some(metadata) = registry_models()
        .iter()
        .find(|entry| entry.id == model.id || entry.aliases.iter().any(|alias| alias == &model.id))
    else {
        return false;
    };

    model.name = Some(metadata.name.clone());
    model.reasoning = metadata.reasoning;
    model.input = metadata.input.clone();
    model.thinking_level_map = metadata.thinking_level_map.clone();
    model.context_window = metadata.context_window;
    if let Some(max_tokens) = metadata.max_output_tokens {
        model.max_tokens = max_tokens;
    }
    true
}

// ============================================================================
// Path Helpers
// ============================================================================

fn droidgear_omp_dir_for_home(home_dir: &Path) -> PathBuf {
    home_dir.join(".droidgear").join("omp")
}

/// `~/.droidgear/omp/profiles/`
pub fn profiles_dir_for_home(home_dir: &Path) -> Result<PathBuf, String> {
    let dir = droidgear_omp_dir_for_home(home_dir).join("profiles");
    if !dir.exists() {
        std::fs::create_dir_all(&dir)
            .map_err(|e| format!("Failed to create OMP profiles directory: {e}"))?;
    }
    Ok(dir)
}

/// `~/.droidgear/omp/active-profile.txt`
pub fn active_profile_path_for_home(home_dir: &Path) -> Result<PathBuf, String> {
    let dir = droidgear_omp_dir_for_home(home_dir);
    if !dir.exists() {
        std::fs::create_dir_all(&dir)
            .map_err(|e| format!("Failed to create OMP directory: {e}"))?;
    }
    Ok(dir.join("active-profile.txt"))
}

/// `~/.omp/agent/` (OMP home directory)
pub fn omp_config_dir_for_home(home_dir: &Path) -> Result<PathBuf, String> {
    let config_paths = paths::load_config_paths_for_home(home_dir);
    let dir = paths::get_omp_home_for_home(home_dir, &config_paths)?;
    if !dir.exists() {
        std::fs::create_dir_all(&dir)
            .map_err(|e| format!("Failed to create OMP config directory: {e}"))?;
    }
    Ok(dir)
}

/// `~/.omp/agent/models.yml`
pub fn omp_config_path_for_home(home_dir: &Path) -> Result<PathBuf, String> {
    Ok(omp_config_dir_for_home(home_dir)?.join("models.yml"))
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

pub fn profile_path_for_home(home_dir: &Path, id: &str) -> Result<PathBuf, String> {
    validate_profile_id(id)?;
    Ok(profiles_dir_for_home(home_dir)?.join(format!("{id}.json")))
}

// ============================================================================
// System wrappers (use system home dir)
// ============================================================================

fn system_home_dir() -> Result<PathBuf, String> {
    dirs::home_dir().ok_or_else(|| "Failed to get home directory".to_string())
}

pub fn profiles_dir() -> Result<PathBuf, String> {
    profiles_dir_for_home(&system_home_dir()?)
}

pub fn active_profile_path() -> Result<PathBuf, String> {
    active_profile_path_for_home(&system_home_dir()?)
}

pub fn omp_config_dir() -> Result<PathBuf, String> {
    omp_config_dir_for_home(&system_home_dir()?)
}

pub fn omp_config_path() -> Result<PathBuf, String> {
    omp_config_path_for_home(&system_home_dir()?)
}

pub fn profile_path(id: &str) -> Result<PathBuf, String> {
    profile_path_for_home(&system_home_dir()?, id)
}

// ============================================================================
// CRUD Helpers
// ============================================================================

fn now_rfc3339() -> String {
    Utc::now().to_rfc3339()
}

fn read_profile_file(path: &Path) -> Result<OmpProfile, String> {
    let s = std::fs::read_to_string(path).map_err(|e| format!("Failed to read profile: {e}"))?;
    serde_json::from_str::<OmpProfile>(&s).map_err(|e| format!("Invalid profile JSON: {e}"))
}

fn write_profile_file(home_dir: &Path, profile: &OmpProfile) -> Result<(), String> {
    let path = profile_path_for_home(home_dir, &profile.id)?;
    let s = serde_json::to_string_pretty(profile)
        .map_err(|e| format!("Failed to serialize profile JSON: {e}"))?;
    storage::atomic_write(&path, s.as_bytes())
}

fn load_profile_by_id(home_dir: &Path, id: &str) -> Result<OmpProfile, String> {
    let path = profile_path_for_home(home_dir, id)?;
    read_profile_file(&path)
}

// ============================================================================
// YAML Config Helpers
// ============================================================================

/// Parse `~/.omp/agent/models.yml` into OmpCurrentConfig.
///
/// The YAML root is expected to have a `providers:` mapping at the top level.
/// If the file is missing, returns an empty config. If it contains malformed
/// YAML, returns an error.
fn parse_omp_yaml_config(content: &str) -> Result<OmpCurrentConfig, String> {
    if content.trim().is_empty() {
        return Ok(OmpCurrentConfig {
            providers: HashMap::new(),
        });
    }

    let value: serde_yaml::Value =
        serde_yaml::from_str(content).map_err(|e| format!("Invalid OMP YAML config: {e}"))?;

    let providers = match value.get("providers") {
        Some(serde_yaml::Value::Mapping(map)) => {
            let mut result = HashMap::new();
            for (key, val) in map {
                if let serde_yaml::Value::String(name) = key {
                    if val.as_mapping().is_some() {
                        let provider: OmpProviderConfig = serde_yaml::from_value(val.clone())
                            .map_err(|e| format!("Invalid provider config for '{name}': {e}"))?;
                        result.insert(name.clone(), provider);
                    }
                }
            }
            result
        }
        _ => HashMap::new(),
    };

    Ok(OmpCurrentConfig { providers })
}

/// Serialize OmpCurrentConfig to YAML and write to `models.yml`.
fn write_omp_yaml_config(home_dir: &Path, config: &OmpCurrentConfig) -> Result<(), String> {
    let config_path = omp_config_path_for_home(home_dir)?;

    // Build the YAML value manually to preserve structure
    let mut root = serde_yaml::Mapping::new();
    let mut providers_map = serde_yaml::Mapping::new();

    for (name, provider) in &config.providers {
        let provider_yaml = serde_yaml::to_value(provider)
            .map_err(|e| format!("Failed to convert provider '{name}' to YAML: {e}"))?;
        providers_map.insert(serde_yaml::Value::String(name.clone()), provider_yaml);
    }

    root.insert(
        serde_yaml::Value::String("providers".to_string()),
        serde_yaml::Value::Mapping(providers_map),
    );

    let yaml_str = serde_yaml::to_string(&serde_yaml::Value::Mapping(root))
        .map_err(|e| format!("Failed to serialize OMP config YAML: {e}"))?;

    // Ensure the config directory exists
    let config_dir = omp_config_dir_for_home(home_dir)?;
    if !config_dir.exists() {
        std::fs::create_dir_all(&config_dir)
            .map_err(|e| format!("Failed to create OMP config directory: {e}"))?;
    }

    storage::atomic_write(&config_path, yaml_str.as_bytes())
}

// ============================================================================
// CRUD (Profiles)
// ============================================================================

pub fn list_omp_profiles_for_home(home_dir: &Path) -> Result<Vec<OmpProfile>, String> {
    let dir = profiles_dir_for_home(home_dir)?;
    if !dir.exists() {
        return Ok(vec![]);
    }

    let mut profiles = Vec::new();
    for entry in std::fs::read_dir(&dir).map_err(|e| format!("Failed to read profiles dir: {e}"))? {
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

    profiles.sort_by_key(|a| a.name.to_lowercase());
    Ok(profiles)
}

pub fn get_omp_profile_for_home(home_dir: &Path, id: &str) -> Result<OmpProfile, String> {
    load_profile_by_id(home_dir, id)
}

pub fn save_omp_profile_for_home(home_dir: &Path, mut profile: OmpProfile) -> Result<(), String> {
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

pub fn delete_omp_profile_for_home(home_dir: &Path, id: &str) -> Result<(), String> {
    let path = profile_path_for_home(home_dir, id)?;
    if path.exists() {
        std::fs::remove_file(&path).map_err(|e| format!("Failed to delete profile: {e}"))?;
    }

    if let Ok(active) = get_active_omp_profile_id_for_home(home_dir) {
        if active.as_deref() == Some(id) {
            let active_path = active_profile_path_for_home(home_dir)?;
            let _ = std::fs::remove_file(active_path);
        }
    }
    Ok(())
}

pub fn duplicate_omp_profile_for_home(
    home_dir: &Path,
    id: &str,
    new_name: &str,
) -> Result<OmpProfile, String> {
    let mut profile = load_profile_by_id(home_dir, id)?;
    profile.id = Uuid::new_v4().to_string();
    profile.name = new_name.to_string();
    profile.created_at = now_rfc3339();
    profile.updated_at = profile.created_at.clone();
    write_profile_file(home_dir, &profile)?;
    Ok(profile)
}

pub fn create_default_omp_profile_for_home(home_dir: &Path) -> Result<OmpProfile, String> {
    let profiles = list_omp_profiles_for_home(home_dir)?;
    if !profiles.is_empty() {
        return Err("Profiles already exist".to_string());
    }

    let id = Uuid::new_v4().to_string();
    let now = now_rfc3339();

    let profile = OmpProfile {
        id,
        name: "Default".to_string(),
        description: None,
        created_at: now.clone(),
        updated_at: now,
        providers: HashMap::new(),
    };

    write_profile_file(home_dir, &profile)?;
    Ok(profile)
}

// ============================================================================
// Active profile
// ============================================================================

pub fn get_active_omp_profile_id_for_home(home_dir: &Path) -> Result<Option<String>, String> {
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

pub fn set_active_omp_profile_id_for_home(home_dir: &Path, id: &str) -> Result<(), String> {
    let path = active_profile_path_for_home(home_dir)?;
    storage::atomic_write(&path, id.as_bytes())
}

// ============================================================================
// Apply + Config Status + Read Current Config
// ============================================================================

/// Apply a profile to `~/.omp/agent/models.yml`.
///
/// Reads the profile, extracts the providers map, and writes it as
/// YAML to OMP's models.yml. Also sets the active profile ID.
pub fn apply_omp_profile_for_home(home_dir: &Path, id: &str) -> Result<(), String> {
    let profile = load_profile_by_id(home_dir, id)?;

    let config = OmpCurrentConfig {
        providers: profile.providers,
    };
    write_omp_yaml_config(home_dir, &config)?;
    set_active_omp_profile_id_for_home(home_dir, id)?;
    Ok(())
}

/// Get the status of `~/.omp/agent/models.yml`.
pub fn get_omp_config_status_for_home(home_dir: &Path) -> Result<OmpConfigStatus, String> {
    let config_path = omp_config_path_for_home(home_dir)?;
    Ok(OmpConfigStatus {
        config_exists: config_path.exists(),
        config_path: config_path.to_string_lossy().to_string(),
    })
}

/// Read the current OMP config from `~/.omp/agent/models.yml`.
///
/// Returns the parsed config. If the file does not exist, returns an empty
/// config (no providers). If it contains malformed YAML, returns an error.
pub fn read_omp_current_config_for_home(home_dir: &Path) -> Result<OmpCurrentConfig, String> {
    let config_path = omp_config_path_for_home(home_dir)?;
    if !config_path.exists() {
        return Ok(OmpCurrentConfig {
            providers: HashMap::new(),
        });
    }
    let s = std::fs::read_to_string(&config_path)
        .map_err(|e| format!("Failed to read OMP config file: {e}"))?;
    parse_omp_yaml_config(&s)
}

// ============================================================================
// System wrappers (CRUD)
// ============================================================================

pub fn list_omp_profiles() -> Result<Vec<OmpProfile>, String> {
    list_omp_profiles_for_home(&system_home_dir()?)
}

pub fn get_omp_profile(id: &str) -> Result<OmpProfile, String> {
    get_omp_profile_for_home(&system_home_dir()?, id)
}

pub fn save_omp_profile(profile: OmpProfile) -> Result<(), String> {
    save_omp_profile_for_home(&system_home_dir()?, profile)
}

pub fn delete_omp_profile(id: &str) -> Result<(), String> {
    delete_omp_profile_for_home(&system_home_dir()?, id)
}

pub fn duplicate_omp_profile(id: &str, new_name: &str) -> Result<OmpProfile, String> {
    duplicate_omp_profile_for_home(&system_home_dir()?, id, new_name)
}

pub fn create_default_omp_profile() -> Result<OmpProfile, String> {
    create_default_omp_profile_for_home(&system_home_dir()?)
}

pub fn get_active_omp_profile_id() -> Result<Option<String>, String> {
    get_active_omp_profile_id_for_home(&system_home_dir()?)
}

pub fn set_active_omp_profile_id(id: &str) -> Result<(), String> {
    set_active_omp_profile_id_for_home(&system_home_dir()?, id)
}

pub fn apply_omp_profile(id: &str) -> Result<(), String> {
    apply_omp_profile_for_home(&system_home_dir()?, id)
}

pub fn get_omp_config_status() -> Result<OmpConfigStatus, String> {
    get_omp_config_status_for_home(&system_home_dir()?)
}

pub fn read_omp_current_config() -> Result<OmpCurrentConfig, String> {
    read_omp_current_config_for_home(&system_home_dir()?)
}

// ============================================================================
// Unit Tests
// ============================================================================

#[cfg(test)]
mod tests {
    use super::*;
    use tempfile::TempDir;

    fn home(temp: &TempDir) -> &Path {
        temp.path()
    }

    fn make_profile(id: &str, name: &str) -> OmpProfile {
        OmpProfile {
            id: id.to_string(),
            name: name.to_string(),
            description: None,
            created_at: "2026-01-01T00:00:00Z".to_string(),
            updated_at: "2026-01-01T00:00:00Z".to_string(),
            providers: HashMap::new(),
        }
    }

    fn make_profile_with_provider(id: &str, name: &str, provider_key: &str) -> OmpProfile {
        let mut providers = HashMap::new();
        providers.insert(
            provider_key.to_string(),
            OmpProviderConfig {
                base_url: Some("http://localhost:11434/v1".to_string()),
                api: Some("openai-completions".to_string()),
                api_key: Some("ollama".to_string()),
                auth: Some("apiKey".to_string()),
                headers: None,
                auth_header: Some(false),
                models: vec![OmpModel {
                    id: "llama3.1:8b".to_string(),
                    name: Some("Llama 3.1 8B".to_string()),
                    api: Some("openai-completions".to_string()),
                    reasoning: false,
                    input: vec!["text".to_string()],
                    context_window: 128000,
                    max_tokens: 16384,
                    cost: None,
                    compat: None,
                    context_promotion_target: None,
                    ..Default::default()
                }],
                model_overrides: None,
                compat: None,
                disable_strict_tools: None,
                discovery: None,
                ..Default::default()
            },
        );
        OmpProfile {
            id: id.to_string(),
            name: name.to_string(),
            description: Some("A test profile".to_string()),
            created_at: "2026-01-01T00:00:00Z".to_string(),
            updated_at: "2026-01-01T00:00:00Z".to_string(),
            providers,
        }
    }

    #[test]
    fn test_type_serialization() {
        let model = OmpModel {
            id: "llama3.1:8b".to_string(),
            name: Some("Llama 3.1 8B".to_string()),
            api: Some("openai-completions".to_string()),
            reasoning: false,
            input: vec!["text".to_string()],
            context_window: 128000,
            max_tokens: 16384,
            cost: Some(OmpModelCost {
                input: 0.0,
                output: 0.0,
                cache_read: 0.0,
                cache_write: 0.0,
                ..Default::default()
            }),
            compat: None,
            context_promotion_target: None,
            ..Default::default()
        };

        let json = serde_json::to_string_pretty(&model).unwrap();
        assert!(json.contains("\"id\": \"llama3.1:8b\""));
        assert!(json.contains("\"contextWindow\": 128000"));
        assert!(json.contains("\"maxTokens\": 16384"));
    }

    #[test]
    fn test_registry_enriches_known_model_metadata() {
        let mut model = OmpModel {
            id: "gpt-5.2".to_string(),
            api: Some("openai-completions".to_string()),
            ..Default::default()
        };

        assert!(enrich_omp_model_from_registry(&mut model));
        assert_eq!(model.name.as_deref(), Some("GPT-5.2"));
        assert!(model.reasoning);
        assert_eq!(model.input, ["text", "image"]);
        assert_eq!(model.context_window, 400000);
        assert_eq!(model.max_tokens, 128000);
    }

    #[test]
    fn test_yaml_config_roundtrip() {
        let yaml = r#"
providers:
  ollama:
    baseUrl: http://localhost:11434/v1
    api: openai-completions
    apiKey: ollama
    auth: apiKey
    models:
      - id: llama3.1:8b
        name: Llama 3.1 8B
        reasoning: false
        input:
          - text
        contextWindow: 128000
        maxTokens: 16384
"#;

        let config = parse_omp_yaml_config(yaml).unwrap();
        assert!(config.providers.contains_key("ollama"));
        let provider = &config.providers["ollama"];
        assert_eq!(
            provider.base_url.as_deref(),
            Some("http://localhost:11434/v1")
        );
        assert_eq!(provider.models.len(), 1);
        assert_eq!(provider.models[0].id, "llama3.1:8b");
        assert_eq!(provider.models[0].context_window, 128000);
    }

    #[test]
    fn test_yaml_config_empty() {
        let config = parse_omp_yaml_config("").unwrap();
        assert!(config.providers.is_empty());
    }

    #[test]
    fn test_yaml_config_with_omp_specific_fields() {
        let yaml = r#"
providers:
  myco:
    baseUrl: https://llm.internal.myco.dev/v1
    apiKey: MYCO_API_KEY
    api: openai-responses
    auth: apiKey
    disableStrictTools: true
    models:
      - id: myco-large
        name: MyCo Large
        reasoning: true
        input:
          - text
          - image
        contextWindow: 200000
        maxTokens: 32000
        contextPromotionTarget: anthropic/claude-opus-4-6
"#;

        let config = parse_omp_yaml_config(yaml).unwrap();
        let provider = &config.providers["myco"];
        assert_eq!(provider.disable_strict_tools, Some(true));
        assert_eq!(
            provider.models[0].context_promotion_target.as_deref(),
            Some("anthropic/claude-opus-4-6")
        );
    }

    #[test]
    fn test_provider_serialization() {
        let provider = OmpProviderConfig {
            base_url: Some("http://localhost:11434/v1".to_string()),
            api: Some("openai-completions".to_string()),
            api_key: Some("ollama".to_string()),
            auth: Some("apiKey".to_string()),
            headers: None,
            auth_header: Some(false),
            models: vec![],
            model_overrides: None,
            compat: None,
            disable_strict_tools: None,
            discovery: None,
            ..Default::default()
        };

        let json = serde_json::to_string_pretty(&provider).unwrap();
        assert!(json.contains("\"baseUrl\": \"http://localhost:11434/v1\""));
        assert!(json.contains("\"apiKey\": \"ollama\""));
        assert!(json.contains("\"auth\": \"apiKey\""));
    }

    #[test]
    fn test_profile_serialization() {
        let profile = OmpProfile {
            id: "test-id".to_string(),
            name: "Test Profile".to_string(),
            description: Some("A test profile".to_string()),
            created_at: "2026-01-01T00:00:00Z".to_string(),
            updated_at: "2026-01-01T00:00:00Z".to_string(),
            providers: HashMap::new(),
        };

        let json = serde_json::to_string_pretty(&profile).unwrap();
        assert!(json.contains("\"id\": \"test-id\""));
        assert!(json.contains("\"createdAt\":"));
        assert!(json.contains("\"updatedAt\":"));
    }

    #[test]
    fn test_path_helpers() {
        let temp = TempDir::new().unwrap();
        let home = temp.path();

        let profiles = profiles_dir_for_home(home).unwrap();
        assert!(profiles.ends_with(".droidgear/omp/profiles"));

        let active = active_profile_path_for_home(home).unwrap();
        assert!(active.ends_with(".droidgear/omp/active-profile.txt"));
    }

    #[test]
    fn test_model_defaults() {
        let json = r#"{"id": "test-model"}"#;
        let model: OmpModel = serde_json::from_str(json).unwrap();
        assert_eq!(model.id, "test-model");
        assert!(!model.reasoning);
        assert_eq!(model.input, vec!["text".to_string()]);
        assert_eq!(model.context_window, 128000);
        assert_eq!(model.max_tokens, 16384);
    }

    #[test]
    fn test_validate_profile_id() {
        assert!(validate_profile_id("valid-id").is_ok());
        assert!(validate_profile_id("valid_id").is_ok());
        assert!(validate_profile_id("abc123").is_ok());
        assert!(validate_profile_id("").is_err());
        assert!(validate_profile_id("has spaces").is_err());
        assert!(validate_profile_id("has/slash").is_err());
    }

    // =========================================================================
    // CRUD Tests
    // =========================================================================

    #[test]
    fn test_create_default_profile() {
        let temp = TempDir::new().unwrap();
        let home = home(&temp);

        let profile = create_default_omp_profile_for_home(home).unwrap();
        assert!(!profile.id.is_empty());
        assert_eq!(profile.name, "Default");
        assert!(profile.providers.is_empty());

        // Should fail when profiles already exist
        let err = create_default_omp_profile_for_home(home).unwrap_err();
        assert_eq!(err, "Profiles already exist");
    }

    #[test]
    fn test_save_and_get_profile() {
        let temp = TempDir::new().unwrap();
        let home = home(&temp);

        let profile = make_profile("p1", "Profile 1");
        save_omp_profile_for_home(home, profile).unwrap();

        let loaded = get_omp_profile_for_home(home, "p1").unwrap();
        assert_eq!(loaded.id, "p1");
        assert_eq!(loaded.name, "Profile 1");
    }

    #[test]
    fn test_save_profile_generates_id_when_empty() {
        let temp = TempDir::new().unwrap();
        let home = home(&temp);

        let profile = OmpProfile {
            id: "".to_string(),
            name: "New Profile".to_string(),
            description: None,
            created_at: "".to_string(),
            updated_at: "".to_string(),
            providers: HashMap::new(),
        };

        save_omp_profile_for_home(home, profile).unwrap();

        let profiles = list_omp_profiles_for_home(home).unwrap();
        assert_eq!(profiles.len(), 1);
        assert!(!profiles[0].id.is_empty());
        assert!(!profiles[0].created_at.is_empty());
        assert!(!profiles[0].updated_at.is_empty());
        assert_eq!(profiles[0].name, "New Profile");
    }

    #[test]
    fn test_list_profiles_sorted_by_name() {
        let temp = TempDir::new().unwrap();
        let home = home(&temp);

        save_omp_profile_for_home(home, make_profile("p1", "Zebra")).unwrap();
        save_omp_profile_for_home(home, make_profile("p2", "Alpha")).unwrap();
        save_omp_profile_for_home(home, make_profile("p3", "Middle")).unwrap();

        let profiles = list_omp_profiles_for_home(home).unwrap();
        assert_eq!(profiles.len(), 3);
        assert_eq!(profiles[0].name, "Alpha");
        assert_eq!(profiles[1].name, "Middle");
        assert_eq!(profiles[2].name, "Zebra");
    }

    #[test]
    fn test_delete_profile() {
        let temp = TempDir::new().unwrap();
        let home = home(&temp);

        save_omp_profile_for_home(home, make_profile("p1", "Profile 1")).unwrap();
        assert_eq!(list_omp_profiles_for_home(home).unwrap().len(), 1);

        delete_omp_profile_for_home(home, "p1").unwrap();
        assert_eq!(list_omp_profiles_for_home(home).unwrap().len(), 0);
    }

    #[test]
    fn test_delete_profile_clears_active() {
        let temp = TempDir::new().unwrap();
        let home = home(&temp);

        save_omp_profile_for_home(home, make_profile("p1", "Profile 1")).unwrap();
        set_active_omp_profile_id_for_home(home, "p1").unwrap();

        let active = get_active_omp_profile_id_for_home(home).unwrap();
        assert_eq!(active.as_deref(), Some("p1"));

        delete_omp_profile_for_home(home, "p1").unwrap();

        let active_after = get_active_omp_profile_id_for_home(home).unwrap();
        assert!(active_after.is_none());
    }

    #[test]
    fn test_duplicate_profile() {
        let temp = TempDir::new().unwrap();
        let home = home(&temp);

        let profile = make_profile_with_provider("p1", "Original", "ollama");
        save_omp_profile_for_home(home, profile).unwrap();

        let dup = duplicate_omp_profile_for_home(home, "p1", "Copy").unwrap();
        assert_ne!(dup.id, "p1");
        assert_eq!(dup.name, "Copy");
        assert!(dup.providers.contains_key("ollama"));

        // Both should exist
        let profiles = list_omp_profiles_for_home(home).unwrap();
        assert_eq!(profiles.len(), 2);
    }

    #[test]
    fn test_active_profile_get_set() {
        let temp = TempDir::new().unwrap();
        let home = home(&temp);

        // Initially no active profile
        let active = get_active_omp_profile_id_for_home(home).unwrap();
        assert!(active.is_none());

        // Set active
        set_active_omp_profile_id_for_home(home, "p1").unwrap();
        let active = get_active_omp_profile_id_for_home(home).unwrap();
        assert_eq!(active.as_deref(), Some("p1"));

        // Overwrite
        set_active_omp_profile_id_for_home(home, "p2").unwrap();
        let active = get_active_omp_profile_id_for_home(home).unwrap();
        assert_eq!(active.as_deref(), Some("p2"));
    }

    #[test]
    fn test_apply_profile_writes_yaml() {
        let temp = TempDir::new().unwrap();
        let home = home(&temp);

        let profile = make_profile_with_provider("p1", "Test", "ollama");
        save_omp_profile_for_home(home, profile).unwrap();

        apply_omp_profile_for_home(home, "p1").unwrap();

        let config_path = omp_config_path_for_home(home).unwrap();
        assert!(config_path.exists());

        let content = std::fs::read_to_string(&config_path).unwrap();
        assert!(content.contains("providers:"));
        assert!(content.contains("ollama:"));
        assert!(content.contains("http://localhost:11434/v1"));

        // Active profile should be set
        let active = get_active_omp_profile_id_for_home(home).unwrap();
        assert_eq!(active.as_deref(), Some("p1"));
    }

    #[test]
    fn test_config_status() {
        let temp = TempDir::new().unwrap();
        let home = home(&temp);

        let status = get_omp_config_status_for_home(home).unwrap();
        assert!(!status.config_exists);

        // Create the config file
        let config_dir = omp_config_dir_for_home(home).unwrap();
        std::fs::write(
            config_dir.join("models.yml"),
            "providers:\n  test:\n    baseUrl: http://localhost\n",
        )
        .unwrap();

        let status = get_omp_config_status_for_home(home).unwrap();
        assert!(status.config_exists);
    }

    #[test]
    fn test_read_current_config_missing_file() {
        let temp = TempDir::new().unwrap();
        let home = home(&temp);

        let config = read_omp_current_config_for_home(home).unwrap();
        assert!(config.providers.is_empty());
    }

    #[test]
    fn test_read_current_config_existing_file() {
        let temp = TempDir::new().unwrap();
        let home = home(&temp);

        let config_dir = omp_config_dir_for_home(home).unwrap();
        std::fs::write(
            config_dir.join("models.yml"),
            "providers:\n  ollama:\n    baseUrl: http://localhost:11434/v1\n    api: openai-completions\n    models:\n      - id: llama3\n",
        )
        .unwrap();

        let config = read_omp_current_config_for_home(home).unwrap();
        assert!(config.providers.contains_key("ollama"));
    }
}
