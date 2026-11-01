//! Model connectivity testing commands.

use droidgear_core::connectivity::{self, ConnectionDiagnostics, ModelTestResult, TestMode};
use tauri::command;

#[command]
#[specta::specta]
pub async fn test_model_connection(model_id: String) -> Result<ModelTestResult, String> {
    connectivity::test_specific_model_connection(&model_id).await
}

#[command]
#[specta::specta]
pub async fn test_all_model_connections_command() -> Result<Vec<ModelTestResult>, String> {
    connectivity::test_all_model_connections().await
}

#[command]
#[specta::specta]
pub async fn test_provider_connection(
    provider: String,
    base_url: String,
    api_key: String,
    model_id: String,
) -> Result<ConnectionDiagnostics, String> {
    let tester = connectivity::ModelTester::new();
    Ok(tester
        .test_model_direct(&provider, &base_url, &api_key, &model_id)
        .await)
}

#[command]
#[specta::specta]
pub async fn test_model_connection_with_mode(
    model_id: String,
    mode: TestMode,
    prompt: Option<String>,
) -> Result<ModelTestResult, String> {
    connectivity::test_specific_model_connection_with_mode(&model_id, mode, prompt).await
}

#[command]
#[specta::specta]
pub async fn test_all_model_connections_with_mode(
    mode: TestMode,
    prompt: Option<String>,
) -> Result<Vec<ModelTestResult>, String> {
    connectivity::test_all_model_connections_with_mode(mode, prompt).await
}
