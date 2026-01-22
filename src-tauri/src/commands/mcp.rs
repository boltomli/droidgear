//! MCP (Model Context Protocol) server configuration management.
//!
//! Handles reading and writing MCP server configurations in ~/.factory/mcp.json

use serde::{Deserialize, Serialize};
use serde_json::Value;
use specta::Type;
use std::collections::HashMap;
use std::path::PathBuf;

use super::paths;

// ============================================================================
// Types
// ============================================================================

/// MCP server type
#[derive(Debug, Clone, Serialize, Deserialize, Type, PartialEq)]
#[serde(rename_all = "lowercase")]
pub enum McpServerType {
    Stdio,
    Http,
}

/// MCP server configuration
#[derive(Debug, Clone, Serialize, Deserialize, Type)]
#[serde(rename_all = "camelCase")]
pub struct McpServerConfig {
    /// Server type (stdio or http)
    #[serde(rename = "type")]
    pub server_type: McpServerType,
    /// Whether the server is disabled
    #[serde(default)]
    pub disabled: bool,
    /// Command to run (stdio only)
    #[serde(skip_serializing_if = "Option::is_none")]
    pub command: Option<String>,
    /// Command arguments (stdio only)
    #[serde(skip_serializing_if = "Option::is_none")]
    pub args: Option<Vec<String>>,
    /// Environment variables (stdio only)
    #[serde(skip_serializing_if = "Option::is_none")]
    pub env: Option<HashMap<String, String>>,
    /// HTTP URL (http only)
    #[serde(skip_serializing_if = "Option::is_none")]
    pub url: Option<String>,
    /// HTTP headers (http only)
    #[serde(skip_serializing_if = "Option::is_none")]
    pub headers: Option<HashMap<String, String>>,
}

/// MCP server entry with name
#[derive(Debug, Clone, Serialize, Deserialize, Type)]
#[serde(rename_all = "camelCase")]
pub struct McpServer {
    /// Server name (unique identifier)
    pub name: String,
    /// Server configuration
    pub config: McpServerConfig,
}

// ============================================================================
// Helper Functions
// ============================================================================

/// Gets the path to ~/.factory/mcp.json
fn get_mcp_config_path() -> Result<PathBuf, String> {
    let factory_dir = paths::get_factory_home()?;

    // Ensure .factory directory exists
    if !factory_dir.exists() {
        std::fs::create_dir_all(&factory_dir)
            .map_err(|e| format!("Failed to create .factory directory: {e}"))?;
    }

    Ok(factory_dir.join("mcp.json"))
}

/// Reads the MCP config file
fn read_mcp_file() -> Result<Value, String> {
    let config_path = get_mcp_config_path()?;

    if !config_path.exists() {
        return Ok(serde_json::json!({ "mcpServers": {} }));
    }

    let contents = std::fs::read_to_string(&config_path)
        .map_err(|e| format!("Failed to read MCP config file: {e}"))?;

    if contents.trim().is_empty() {
        return Ok(serde_json::json!({ "mcpServers": {} }));
    }

    serde_json::from_str(&contents).map_err(|e| format!("Failed to parse MCP config JSON: {e}"))
}

/// Writes the MCP config file (atomic write)
fn write_mcp_file(config: &Value) -> Result<(), String> {
    let config_path = get_mcp_config_path()?;

    // Resolve symlink to get the actual file path
    let actual_path = if config_path.is_symlink() {
        std::fs::canonicalize(&config_path)
            .map_err(|e| format!("Failed to resolve symlink: {e}"))?
    } else {
        config_path
    };

    let temp_path = actual_path.with_extension("tmp");

    let json_content = serde_json::to_string_pretty(config)
        .map_err(|e| format!("Failed to serialize MCP config: {e}"))?;

    std::fs::write(&temp_path, json_content)
        .map_err(|e| format!("Failed to write MCP config file: {e}"))?;

    std::fs::rename(&temp_path, &actual_path).map_err(|e| {
        let _ = std::fs::remove_file(&temp_path);
        format!("Failed to finalize MCP config file: {e}")
    })?;

    Ok(())
}

// ============================================================================
// Tauri Commands
// ============================================================================

/// Loads all MCP servers from ~/.factory/mcp.json
#[tauri::command]
#[specta::specta]
pub async fn load_mcp_servers() -> Result<Vec<McpServer>, String> {
    log::debug!("Loading MCP servers from config");

    let config = read_mcp_file()?;

    let servers: Vec<McpServer> = config
        .get("mcpServers")
        .and_then(|v| v.as_object())
        .map(|obj| {
            obj.iter()
                .filter_map(|(name, value)| {
                    let config: McpServerConfig = serde_json::from_value(value.clone()).ok()?;
                    Some(McpServer {
                        name: name.clone(),
                        config,
                    })
                })
                .collect()
        })
        .unwrap_or_default();

    log::info!("Loaded {} MCP servers", servers.len());
    Ok(servers)
}

/// Saves an MCP server (creates or updates)
#[tauri::command]
#[specta::specta]
pub async fn save_mcp_server(server: McpServer) -> Result<(), String> {
    log::debug!("Saving MCP server: {}", server.name);

    let mut config = read_mcp_file()?;

    let server_value = serde_json::to_value(&server.config)
        .map_err(|e| format!("Failed to serialize server config: {e}"))?;

    if let Some(obj) = config.as_object_mut() {
        let mcp_servers = obj
            .entry("mcpServers")
            .or_insert_with(|| serde_json::json!({}));

        if let Some(servers_obj) = mcp_servers.as_object_mut() {
            servers_obj.insert(server.name.clone(), server_value);
        }
    }

    write_mcp_file(&config)?;

    log::info!("Successfully saved MCP server: {}", server.name);
    Ok(())
}

/// Deletes an MCP server by name
#[tauri::command]
#[specta::specta]
pub async fn delete_mcp_server(name: String) -> Result<(), String> {
    log::debug!("Deleting MCP server: {}", name);

    let mut config = read_mcp_file()?;

    if let Some(obj) = config.as_object_mut() {
        if let Some(mcp_servers) = obj.get_mut("mcpServers") {
            if let Some(servers_obj) = mcp_servers.as_object_mut() {
                servers_obj.remove(&name);
            }
        }
    }

    write_mcp_file(&config)?;

    log::info!("Successfully deleted MCP server: {}", name);
    Ok(())
}

/// Toggles an MCP server's disabled state
#[tauri::command]
#[specta::specta]
pub async fn toggle_mcp_server(name: String, disabled: bool) -> Result<(), String> {
    log::debug!("Toggling MCP server {}: disabled={}", name, disabled);

    let mut config = read_mcp_file()?;

    if let Some(obj) = config.as_object_mut() {
        if let Some(mcp_servers) = obj.get_mut("mcpServers") {
            if let Some(servers_obj) = mcp_servers.as_object_mut() {
                if let Some(server) = servers_obj.get_mut(&name) {
                    if let Some(server_obj) = server.as_object_mut() {
                        server_obj.insert("disabled".to_string(), serde_json::json!(disabled));
                    }
                } else {
                    return Err(format!("Server not found: {name}"));
                }
            }
        }
    }

    write_mcp_file(&config)?;

    log::info!(
        "Successfully toggled MCP server {}: disabled={}",
        name,
        disabled
    );
    Ok(())
}
