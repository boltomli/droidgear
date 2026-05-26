//! Codex CLI 配置管理命令（Tauri wrappers）。
//!
//! Core logic lives in `droidgear-core`.

pub use droidgear_core::codex::{CodexConfigStatus, CodexCurrentConfig, CodexProfile};

use droidgear_core::codex_runtime::{
    self, CodexCliCapability, CodexTemporaryLaunchPlan, CodexTemporaryRunPlan,
};

use crate::utils::login_shell::run_command_in_login_shell;
use crate::utils::preferences::load_preferences;
use crate::utils::terminal_launch::{launch_in_terminal, LaunchSpec};

/// Result returned by the desktop launch command.
#[derive(Debug, Clone, serde::Serialize, specta::Type)]
pub struct CodexDesktopLaunchResult {
    pub debug_port: u16,
    pub cd_uri: String,
}

/// List all Codex profiles
#[tauri::command]
#[specta::specta]
pub async fn list_codex_profiles() -> Result<Vec<CodexProfile>, String> {
    droidgear_core::codex::list_codex_profiles()
}

/// Get a profile by ID
#[tauri::command]
#[specta::specta]
pub async fn get_codex_profile(id: String) -> Result<CodexProfile, String> {
    droidgear_core::codex::get_codex_profile(&id)
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

/// Inspect the installed Codex CLI and report whether temporary-run launch-time
/// overrides are supported.
#[tauri::command]
#[specta::specta]
pub async fn get_codex_cli_capability() -> Result<CodexCliCapability, String> {
    let version_output = run_command_in_login_shell("codex", &["--version"])?;
    if version_output.status.code() == Some(127) {
        return Err("Failed to execute codex --version: No such file or directory".to_string());
    }
    if !version_output.status.success() {
        return Err("Failed to read Codex CLI version".to_string());
    }

    let help_output = run_command_in_login_shell("codex", &["--help"])?;
    if help_output.status.code() == Some(127) {
        return Err("Failed to execute codex --help: No such file or directory".to_string());
    }
    if !help_output.status.success() {
        return Err("Failed to read Codex CLI help".to_string());
    }

    Ok(codex_runtime::parse_codex_cli_capability(
        &String::from_utf8_lossy(&help_output.stdout),
        &String::from_utf8_lossy(&version_output.stdout),
    ))
}

/// Build the zero-write temporary-run launch plan preview for a Codex profile.
#[tauri::command]
#[specta::specta]
pub async fn get_codex_temporary_run_plan(id: String) -> Result<CodexTemporaryRunPlan, String> {
    let profile = droidgear_core::codex::get_codex_profile(&id)?;
    let plan = codex_runtime::build_temporary_run_plan(&profile)?;
    Ok((&plan).into())
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
    }
}

/// Launch the Codex desktop (Electron) app on Windows with CDP remote debugging
/// enabled and inject a bootstrap script.
#[tauri::command]
#[specta::specta]
pub async fn launch_codex_desktop(
    id: String,
    cwd: Option<String>,
) -> Result<CodexDesktopLaunchResult, String> {
    launch_codex_desktop_impl(id, cwd).await
}

#[cfg(windows)]
async fn launch_codex_desktop_impl(
    id: String,
    cwd: Option<String>,
) -> Result<CodexDesktopLaunchResult, String> {
    use std::os::windows::process::CommandExt;
    use std::process::{Command, Stdio};

    // Apply the profile so Codex desktop picks up the config.
    droidgear_core::codex::apply_codex_profile(&id)?;

    // Find the Codex executable.
    let codex_exe = find_codex_desktop_exe()?;

    let debug_port = droidgear_core::cdp::CDP_DEFAULT_PORT;
    let args = [
        format!("--remote-debugging-port={debug_port}"),
        format!("--remote-allow-origins=http://127.0.0.1:{debug_port}"),
    ];

    // Launch Codex desktop with CDP flags, without spawning a console window.
    let _child = Command::new(&codex_exe)
        .args(&args)
        .creation_flags(0x08000000) // CREATE_NO_WINDOW
        .stdout(Stdio::null())
        .stderr(Stdio::null())
        .spawn()
        .map_err(|e| {
            format!(
                "Failed to launch Codex desktop at '{}': {e}",
                codex_exe.display()
            )
        })?;

    log::info!(
        "Launched Codex desktop at {} with CDP port {debug_port}",
        codex_exe.display()
    );

    // Wait for CDP to become available and inject bootstrap script.
    let cwd_path = cwd.map(std::path::PathBuf::from);
    inject_bootstrap(debug_port, cwd_path.as_deref()).await?;

    Ok(CodexDesktopLaunchResult {
        debug_port,
        cd_uri: format!("http://127.0.0.1:{debug_port}/json"),
    })
}

#[cfg(not(windows))]
async fn launch_codex_desktop_impl(
    _id: String,
    _cwd: Option<String>,
) -> Result<CodexDesktopLaunchResult, String> {
    Err("Desktop launch is only supported on Windows".to_string())
}

#[cfg(windows)]
fn find_codex_desktop_exe() -> Result<std::path::PathBuf, String> {
    // 1. Check known install locations first.
    let local_app_data = std::env::var("LOCALAPPDATA").unwrap_or_default();
    let known_paths = [
        format!("{local_app_data}\\Programs\\codex\\Codex.exe"),
        format!("{local_app_data}\\codex\\Codex.exe"),
    ];

    for path in &known_paths {
        let candidate = std::path::PathBuf::from(path);
        if candidate.exists() {
            return Ok(candidate);
        }
    }

    // 2. Fall back to searching PATH for codex.exe or Codex.exe.
    if let Ok(path_var) = std::env::var("PATH") {
        for dir in std::env::split_paths(&path_var) {
            for name in &["Codex.exe", "codex.exe"] {
                let candidate = dir.join(name);
                if candidate.exists() {
                    log::info!("Found Codex desktop exe in PATH: {}", candidate.display());
                    return Ok(candidate);
                }
            }
        }
    }

    Err(format!(
        "Codex desktop executable not found. Searched known paths ({}) and PATH.",
        known_paths
            .iter()
            .map(|p| p.as_str())
            .collect::<Vec<_>>()
            .join(", ")
    ))
}

#[cfg(windows)]
async fn inject_bootstrap(debug_port: u16, cwd: Option<&std::path::Path>) -> Result<(), String> {
    let cwd_json = cwd
        .and_then(|p| p.to_str())
        .map(|s| serde_json::Value::String(s.to_string()))
        .unwrap_or(serde_json::Value::Null);

    let script = format!(
        r#"
(() => {{
    console.log('[DroidGear] CDP injection active');
    window.__droidgear = {{
        debugPort: {debug_port},
        cwd: {cwd_json},
        injectedAt: new Date().toISOString(),
    }};
}})();
"#
    );

    droidgear_core::cdp::injection::inject_script(debug_port, &script)
        .await
        .map_err(|e| format!("CDP injection failed: {e}"))?;

    log::info!("Successfully injected bootstrap script into Codex desktop");
    Ok(())
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
