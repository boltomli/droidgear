//! Window state safety commands.
//!
//! Hardens the `tauri-plugin-window-state` restore path against corrupted
//! state files, missing monitors (e.g. unplugged secondary display), and
//! out-of-range geometry that would otherwise leave the window invisible
//! or unmanageable.

use serde_json::Value;
use std::path::{Path, PathBuf};
use std::time::{SystemTime, UNIX_EPOCH};
use tauri::{AppHandle, LogicalSize, Manager, PhysicalPosition, WebviewWindow};

/// File name used by `tauri-plugin-window-state` (matches its `DEFAULT_FILENAME`).
const PLUGIN_STATE_FILENAME: &str = ".window-state.json";
/// Bundle identifier — kept in sync with `tauri.conf.json`. Used to compute
/// the window state file path before an `AppHandle` exists.
const BUNDLE_IDENTIFIER: &str = "com.droidgear.app";

const DEFAULT_WIDTH: f64 = 1150.0;
const DEFAULT_HEIGHT: f64 = 700.0;
/// Largest plausible monitor edge in physical pixels (8K + headroom).
/// Anything above this in a saved state file is treated as corruption.
const SANITY_MAX_DIMENSION: u64 = 16_384;
/// Minimum visible window area (in physical pixels) required to consider a
/// window restorable on a given monitor.
const MIN_VISIBLE_PX: i64 = 200;
/// Allow a small overflow over the largest monitor (DPI rounding, etc.)
/// before declaring the window oversized.
const OVERSIZE_SLACK_PX: i64 = 100;

/// Axis-aligned rectangle in physical pixels.
#[derive(Debug, Clone, Copy, PartialEq, Eq)]
pub struct Rect {
    pub x: i64,
    pub y: i64,
    pub w: i64,
    pub h: i64,
}

impl Rect {
    fn intersects_with_min(&self, other: Rect, min: i64) -> bool {
        let ix = self.x.max(other.x);
        let iy = self.y.max(other.y);
        let ax = (self.x + self.w).min(other.x + other.w);
        let ay = (self.y + self.h).min(other.y + other.h);
        ax - ix >= min && ay - iy >= min
    }
}

/// Decide whether the saved window rectangle is still usable on the current
/// monitor layout. Returns `Some(default)` when the caller should reset to a
/// safe default, or `None` when the saved geometry is acceptable.
pub fn clamp_decision(win: Rect, monitors: &[Rect], default: Rect) -> Option<Rect> {
    if monitors.is_empty() {
        return Some(default);
    }

    let visible = monitors
        .iter()
        .any(|m| win.intersects_with_min(*m, MIN_VISIBLE_PX));
    let max_w = monitors.iter().map(|m| m.w).max().unwrap_or(default.w);
    let max_h = monitors.iter().map(|m| m.h).max().unwrap_or(default.h);
    let oversized = win.w > max_w + OVERSIZE_SLACK_PX || win.h > max_h + OVERSIZE_SLACK_PX;
    let degenerate = win.w <= 0 || win.h <= 0;

    if !visible || oversized || degenerate {
        Some(default)
    } else {
        None
    }
}

/// Compute the path of the plugin-managed window state file without needing
/// an `AppHandle`. `dirs::config_dir()` mirrors Tauri's `app_config_dir`
/// resolution on each desktop platform.
fn plugin_state_path() -> Option<PathBuf> {
    dirs::config_dir().map(|base| base.join(BUNDLE_IDENTIFIER).join(PLUGIN_STATE_FILENAME))
}

/// Path of the plugin-managed window state file via an `AppHandle` (used at
/// reset time). Falls back to the dirs-based path when the handle resolution
/// fails so we still attempt the cleanup.
fn plugin_state_path_via(app: &AppHandle) -> Option<PathBuf> {
    app.path()
        .app_config_dir()
        .ok()
        .map(|dir| dir.join(PLUGIN_STATE_FILENAME))
        .or_else(plugin_state_path)
}

/// Move a corrupted window state file aside so the plugin starts from defaults
/// on next launch. Errors are logged but never surfaced to the caller.
fn quarantine_state_file(path: &Path) {
    let ts = SystemTime::now()
        .duration_since(UNIX_EPOCH)
        .map(|d| d.as_secs())
        .unwrap_or(0);
    let backup = path.with_extension(format!("corrupted-{ts}.json"));
    match std::fs::rename(path, &backup) {
        Ok(_) => log::warn!("Quarantined window state file to {backup:?}"),
        Err(err) => log::warn!("Failed to quarantine window state file {path:?}: {err}"),
    }
}

/// Returns true when the JSON looks structurally suspicious enough that we
/// should not let the plugin restore from it. Conservative on purpose; only
/// matches the failure modes we have actually seen in the wild.
fn is_suspicious_state(value: &Value) -> bool {
    let walk_dimensions = |key: &str| -> Vec<u64> {
        let mut out = Vec::new();
        if let Some(top) = value.get(key).and_then(Value::as_u64) {
            out.push(top);
        }
        if let Some(obj) = value.as_object() {
            for v in obj.values() {
                if let Some(n) = v.get(key).and_then(Value::as_u64) {
                    out.push(n);
                }
            }
        }
        out
    };

    for w in walk_dimensions("width") {
        if w > SANITY_MAX_DIMENSION {
            return true;
        }
    }
    for h in walk_dimensions("height") {
        if h > SANITY_MAX_DIMENSION {
            return true;
        }
    }

    // Legacy `decorated` field; we never persist decorations any more.
    let has_decorated = |v: &Value| v.get("decorated").is_some();
    if has_decorated(value) {
        return true;
    }
    if let Some(obj) = value.as_object() {
        if obj.values().any(has_decorated) {
            return true;
        }
    }

    false
}

/// Inspect the on-disk window state file and quarantine it if it looks
/// corrupted. Safe to call on every startup; no-ops when the file is absent.
/// Does not require an `AppHandle` so it can run before plugin initialisation.
pub fn precheck_state_file() {
    let Some(path) = plugin_state_path() else {
        return;
    };
    precheck_path(&path);
}

fn precheck_path(path: &Path) {
    if !path.exists() {
        return;
    }

    let contents = match std::fs::read_to_string(path) {
        Ok(c) => c,
        Err(err) => {
            log::warn!("Failed to read {path:?}: {err}; quarantining");
            quarantine_state_file(path);
            return;
        }
    };

    match serde_json::from_str::<Value>(&contents) {
        Ok(value) => {
            if is_suspicious_state(&value) {
                log::warn!("Window state file looks corrupted; quarantining");
                quarantine_state_file(path);
            }
        }
        Err(err) => {
            log::warn!("Window state file is not valid JSON ({err}); quarantining");
            quarantine_state_file(path);
        }
    }
}

fn monitors_for(window: &WebviewWindow) -> Vec<Rect> {
    window
        .available_monitors()
        .ok()
        .unwrap_or_default()
        .into_iter()
        .map(|m| {
            let pos = m.position();
            let size = m.size();
            Rect {
                x: pos.x as i64,
                y: pos.y as i64,
                w: size.width as i64,
                h: size.height as i64,
            }
        })
        .collect()
}

fn current_window_rect(window: &WebviewWindow) -> Result<Rect, String> {
    let pos = window
        .outer_position()
        .map_err(|e| format!("outer_position failed: {e}"))?;
    let size = window
        .outer_size()
        .map_err(|e| format!("outer_size failed: {e}"))?;
    Ok(Rect {
        x: pos.x as i64,
        y: pos.y as i64,
        w: size.width as i64,
        h: size.height as i64,
    })
}

fn default_rect_for(window: &WebviewWindow) -> Rect {
    let scale = window.scale_factor().unwrap_or(1.0);
    Rect {
        x: 0,
        y: 0,
        w: (DEFAULT_WIDTH * scale).round() as i64,
        h: (DEFAULT_HEIGHT * scale).round() as i64,
    }
}

/// Validate the post-restore geometry and snap the window back to a safe
/// default when it is offscreen or absurdly oversized.
pub fn validate_and_clamp_window(window: &WebviewWindow) -> Result<(), String> {
    let current = match current_window_rect(window) {
        Ok(r) => r,
        Err(err) => {
            log::warn!("Skipping window clamp: {err}");
            return Ok(());
        }
    };
    let monitors = monitors_for(window);
    let default = default_rect_for(window);

    if let Some(safe) = clamp_decision(current, &monitors, default) {
        log::warn!(
            "Restored window geometry rejected (current={current:?}, monitors={monitors:?}); resetting to {safe:?}"
        );
        if let Err(err) = window.set_size(LogicalSize::new(DEFAULT_WIDTH, DEFAULT_HEIGHT)) {
            log::warn!("Failed to reset window size: {err}");
        }
        if let Err(err) = window.set_fullscreen(false) {
            log::warn!("Failed to clear fullscreen: {err}");
        }
        if monitors.is_empty() {
            if let Err(err) =
                window.set_position(PhysicalPosition::new(safe.x as i32, safe.y as i32))
            {
                log::warn!("Failed to set fallback position: {err}");
            }
        } else if let Err(err) = window.center() {
            log::warn!("Failed to center window: {err}");
        }
    }

    Ok(())
}

/// Tauri command: snap the main window to the default size, clear fullscreen,
/// re-center, and quarantine the saved state file so the next launch starts
/// fresh. Surfaced via the application menu and a global shortcut so users
/// can recover from an unmanageable window without editing files by hand.
#[tauri::command]
#[specta::specta]
pub async fn reset_window_state(app: AppHandle) -> Result<(), String> {
    let window = app
        .get_webview_window("main")
        .ok_or_else(|| "main window not found".to_string())?;

    if let Err(err) = window.set_fullscreen(false) {
        log::warn!("reset_window_state: clear fullscreen failed: {err}");
    }
    window
        .set_size(LogicalSize::new(DEFAULT_WIDTH, DEFAULT_HEIGHT))
        .map_err(|e| format!("set_size failed: {e}"))?;
    window.center().map_err(|e| format!("center failed: {e}"))?;
    window.show().map_err(|e| format!("show failed: {e}"))?;
    if let Err(err) = window.set_focus() {
        log::warn!("reset_window_state: set_focus failed: {err}");
    }

    if let Some(path) = plugin_state_path_via(&app) {
        if path.exists() {
            quarantine_state_file(&path);
        }
    }

    log::info!("Window state reset by user");
    Ok(())
}

#[cfg(test)]
mod tests {
    use super::*;

    fn r(x: i64, y: i64, w: i64, h: i64) -> Rect {
        Rect { x, y, w, h }
    }

    #[test]
    fn clamp_keeps_window_inside_single_monitor() {
        let monitor = r(0, 0, 1920, 1080);
        let win = r(100, 100, 1280, 720);
        let default = r(0, 0, 1150, 700);
        assert_eq!(clamp_decision(win, &[monitor], default), None);
    }

    #[test]
    fn clamp_resets_when_completely_offscreen() {
        let monitor = r(0, 0, 1920, 1080);
        let win = r(-50_000, -50_000, 800, 600);
        let default = r(0, 0, 1150, 700);
        assert_eq!(clamp_decision(win, &[monitor], default), Some(default));
    }

    #[test]
    fn clamp_resets_when_only_a_sliver_visible() {
        let monitor = r(0, 0, 1920, 1080);
        let win = r(1870, 100, 800, 600);
        let default = r(0, 0, 1150, 700);
        assert_eq!(clamp_decision(win, &[monitor], default), Some(default));
    }

    #[test]
    fn clamp_resets_when_oversized() {
        let monitor = r(0, 0, 1920, 1080);
        let win = r(0, 0, 9999, 9999);
        let default = r(0, 0, 1150, 700);
        assert_eq!(clamp_decision(win, &[monitor], default), Some(default));
    }

    #[test]
    fn clamp_resets_when_degenerate() {
        let monitor = r(0, 0, 1920, 1080);
        let win = r(100, 100, 0, 0);
        let default = r(0, 0, 1150, 700);
        assert_eq!(clamp_decision(win, &[monitor], default), Some(default));
    }

    #[test]
    fn clamp_keeps_window_spanning_two_monitors() {
        let primary = r(0, 0, 1920, 1080);
        let secondary = r(1920, 0, 2560, 1440);
        let win = r(2000, 100, 1600, 900);
        let default = r(0, 0, 1150, 700);
        assert_eq!(clamp_decision(win, &[primary, secondary], default), None);
    }

    #[test]
    fn clamp_resets_when_secondary_monitor_unplugged() {
        let primary_only = r(0, 0, 1920, 1080);
        let win = r(2000, 100, 1600, 900);
        let default = r(0, 0, 1150, 700);
        assert_eq!(clamp_decision(win, &[primary_only], default), Some(default));
    }

    #[test]
    fn clamp_resets_when_no_monitors_available() {
        let win = r(0, 0, 1280, 720);
        let default = r(0, 0, 1150, 700);
        assert_eq!(clamp_decision(win, &[], default), Some(default));
    }

    #[test]
    fn suspicious_state_flags_oversize_top_level() {
        let v: Value = serde_json::json!({"width": 99_999, "height": 200});
        assert!(is_suspicious_state(&v));
    }

    #[test]
    fn suspicious_state_flags_decorated_top_level() {
        let v: Value = serde_json::json!({"width": 1280, "height": 720, "decorated": false});
        assert!(is_suspicious_state(&v));
    }

    #[test]
    fn suspicious_state_flags_main_envelope_oversize() {
        let v: Value = serde_json::json!({
            "main": {"width": 99_999, "height": 1080}
        });
        assert!(is_suspicious_state(&v));
    }

    #[test]
    fn suspicious_state_flags_main_envelope_decorated() {
        let v: Value = serde_json::json!({
            "main": {"width": 1280, "height": 720, "decorated": false}
        });
        assert!(is_suspicious_state(&v));
    }

    #[test]
    fn suspicious_state_passes_normal_payload() {
        let v: Value = serde_json::json!({
            "main": {"width": 1280, "height": 720, "x": 100, "y": 100}
        });
        assert!(!is_suspicious_state(&v));
    }

    #[test]
    fn precheck_quarantines_oversize_file() {
        let dir = std::env::temp_dir().join(format!(
            "droidgear-window-precheck-{}",
            std::time::SystemTime::now()
                .duration_since(UNIX_EPOCH)
                .unwrap()
                .as_nanos()
        ));
        std::fs::create_dir_all(&dir).unwrap();
        let path = dir.join(".window-state.json");
        std::fs::write(
            &path,
            r#"{"main": {"width": 99999, "height": 1080, "x": 0, "y": 0}}"#,
        )
        .unwrap();

        precheck_path(&path);

        assert!(!path.exists(), "corrupted file should be moved aside");
        let entries: Vec<_> = std::fs::read_dir(&dir).unwrap().flatten().collect();
        let has_backup = entries.iter().any(|e| {
            e.file_name()
                .to_string_lossy()
                .starts_with(".window-state.corrupted-")
        });
        assert!(has_backup, "expected a quarantined backup, got {entries:?}");

        let _ = std::fs::remove_dir_all(&dir);
    }

    #[test]
    fn precheck_keeps_normal_file() {
        let dir = std::env::temp_dir().join(format!(
            "droidgear-window-precheck-ok-{}",
            std::time::SystemTime::now()
                .duration_since(UNIX_EPOCH)
                .unwrap()
                .as_nanos()
        ));
        std::fs::create_dir_all(&dir).unwrap();
        let path = dir.join(".window-state.json");
        std::fs::write(
            &path,
            r#"{"main": {"width": 1280, "height": 720, "x": 100, "y": 100}}"#,
        )
        .unwrap();

        precheck_path(&path);

        assert!(path.exists(), "valid file must be preserved");

        let _ = std::fs::remove_dir_all(&dir);
    }
}
