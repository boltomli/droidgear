//! Codex CLI 配置管理命令（Tauri wrappers）。
//!
//! Core logic lives in `droidgear-core`.

pub use droidgear_core::codex::{CodexConfigStatus, CodexCurrentConfig, CodexProfile};

use crate::utils::preferences::load_preferences;
use crate::utils::terminal_launch::{launch_in_terminal, LaunchSpec};
use droidgear_core::codex_runtime::{self, CodexTemporaryLaunchPlan};

/// List all Codex profiles
#[tauri::command]
#[specta::specta]
pub async fn list_codex_profiles() -> Result<Vec<CodexProfile>, String> {
    droidgear_core::codex::list_codex_profiles()
}

/// Save a profile (create or update)
#[tauri::command]
#[specta::specta]
pub async fn save_codex_profile(profile: CodexProfile) -> Result<(), String> {
    droidgear_core::codex::save_codex_profile(profile)
}

/// Delete a profile
#[tauri::command]
#[specta::specta]
pub async fn delete_codex_profile(id: String) -> Result<(), String> {
    droidgear_core::codex::delete_codex_profile(&id)
}

/// Duplicate a profile
#[tauri::command]
#[specta::specta]
pub async fn duplicate_codex_profile(id: String, new_name: String) -> Result<CodexProfile, String> {
    droidgear_core::codex::duplicate_codex_profile(&id, &new_name)
}

/// Create default profile (when no profiles exist)
#[tauri::command]
#[specta::specta]
pub async fn create_default_codex_profile() -> Result<CodexProfile, String> {
    droidgear_core::codex::create_default_codex_profile()
}

/// Get active profile ID
#[tauri::command]
#[specta::specta]
pub async fn get_active_codex_profile_id() -> Result<Option<String>, String> {
    droidgear_core::codex::get_active_codex_profile_id()
}

/// Apply a profile to `~/.codex/*`
#[tauri::command]
#[specta::specta]
pub async fn apply_codex_profile(id: String) -> Result<(), String> {
    droidgear_core::codex::apply_codex_profile(&id)
}

/// Get Codex config status
#[tauri::command]
#[specta::specta]
pub async fn get_codex_config_status() -> Result<CodexConfigStatus, String> {
    droidgear_core::codex::get_codex_config_status()
}

/// Read current Codex configuration from config files
#[tauri::command]
#[specta::specta]
pub async fn read_codex_current_config() -> Result<CodexCurrentConfig, String> {
    droidgear_core::codex::read_codex_current_config()
}

/// Launch Codex using a runtime `CODEX_HOME` snapshot instead of mutating live config.
#[tauri::command]
#[specta::specta]
pub async fn launch_codex(
    id: String,
    app: tauri::AppHandle,
    cwd: Option<String>,
) -> Result<(), String> {
    let home_dir = dirs::home_dir().ok_or_else(|| "Failed to get home directory".to_string())?;
    if let Err(error) = codex_runtime::cleanup_stale_runtime_homes_for_home(&home_dir) {
        log::warn!("Failed to clean up stale Codex runtime homes: {error}");
    }

    let profile = droidgear_core::codex::get_codex_profile(&id)?;
    let plan = codex_runtime::build_temporary_run_plan(&profile)?;
    let prefs = load_preferences(&app).unwrap_or_default();
    let preferred = prefs.preferred_terminal.unwrap_or_default();

    let mut spec = build_codex_launch_spec(&plan);
    spec.cwd = cwd.map(std::path::PathBuf::from);

    launch_in_terminal(&spec, &preferred)
}

fn build_codex_launch_spec(plan: &CodexTemporaryLaunchPlan) -> LaunchSpec {
    LaunchSpec {
        program: plan.program.clone(),
        args: plan.args.clone(),
        env: plan.env.clone(),
        secret_env: plan.secret_env.clone(),
        unset_env: plan.unset_env.clone(),
        cwd: None,
        support_dir: Some(plan.runtime_home_path.clone()),
        no_keep_open: false,
    }
}

#[cfg(test)]
mod tests {
    use super::build_codex_launch_spec;
    use droidgear_core::codex_runtime::CodexTemporaryLaunchPlan;
    use std::path::PathBuf;

    #[test]
    fn build_codex_launch_spec_preserves_args_env_and_secret_env() {
        let spec = build_codex_launch_spec(&CodexTemporaryLaunchPlan {
            program: "codex".to_string(),
            args: vec![],
            env: vec![("CODEX_HOME".to_string(), "/tmp/runtime-codex".to_string())],
            secret_env: vec![("EXAMPLE_API_KEY".to_string(), "sk-test".to_string())],
            unset_env: vec!["OPENAI_API_KEY".to_string()],
            warnings: vec!["warning".to_string()],
            runtime_home_path: PathBuf::from("/tmp/runtime-codex"),
        });

        assert_eq!(spec.program, "codex");
        assert!(spec.args.is_empty());
        assert_eq!(
            spec.env,
            vec![("CODEX_HOME".to_string(), "/tmp/runtime-codex".to_string())]
        );
        assert_eq!(
            spec.secret_env,
            vec![("EXAMPLE_API_KEY".to_string(), "sk-test".to_string())]
        );
        assert_eq!(spec.unset_env, vec!["OPENAI_API_KEY".to_string()]);
        assert_eq!(spec.support_dir, Some(PathBuf::from("/tmp/runtime-codex")));
    }
}
