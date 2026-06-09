//! Channel export template management commands.
//!
//! Provides Tauri commands for CRUD and execution of export templates.
//! Templates are stored in ~/.droidgear/export-templates.yaml.

use droidgear_core::channel_export::{self, ExportResult, ExportTemplate};

/// Load all export templates from config file.
#[tauri::command]
#[specta::specta]
pub fn load_export_templates() -> Result<Vec<ExportTemplate>, String> {
    channel_export::load_export_templates()
}

/// Save (create or update) an export template by name.
/// If a template with the same name exists, it's replaced.
#[tauri::command]
#[specta::specta]
pub fn save_export_template(template: ExportTemplate) -> Result<(), String> {
    let mut templates = channel_export::load_export_templates()?;

    // Replace if name exists, otherwise add
    let pos = templates.iter().position(|t| t.name == template.name);
    match pos {
        Some(idx) => {
            templates[idx] = template;
        }
        None => {
            templates.push(template);
        }
    }

    channel_export::save_export_templates(&templates)
}

/// Delete an export template by name.
#[tauri::command]
#[specta::specta]
pub fn delete_export_template(name: String) -> Result<(), String> {
    let mut templates = channel_export::load_export_templates()?;
    let before = templates.len();
    templates.retain(|t| t.name != name);
    if templates.len() == before {
        return Err(format!("Template '{}' not found", name));
    }
    channel_export::save_export_templates(&templates)
}

/// Run an export template by name and return the result.
#[tauri::command]
#[specta::specta]
pub fn run_export_template(name: String) -> Result<ExportResult, String> {
    let templates = channel_export::load_export_templates()?;
    let template = templates
        .iter()
        .find(|t| t.name == name)
        .ok_or_else(|| format!("Template '{}' not found", name))?;
    channel_export::run_export(template)
}
