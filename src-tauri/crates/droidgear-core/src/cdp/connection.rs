use anyhow::{bail, Context};
use serde::Deserialize;

use crate::cdp::{CDP_COMMAND_TIMEOUT, CDP_CONNECT_RETRIES, CDP_CONNECT_TIMEOUT, CDP_RETRY_DELAY};

#[derive(Clone, Debug, Deserialize, Eq, PartialEq)]
pub struct CdpTarget {
    pub id: String,
    #[serde(rename = "type")]
    pub target_type: String,
    #[serde(default)]
    pub title: String,
    #[serde(default)]
    pub url: String,
    #[serde(default, rename = "webSocketDebuggerUrl")]
    pub web_socket_debugger_url: Option<String>,
}

pub async fn list_targets(debug_port: u16) -> anyhow::Result<Vec<CdpTarget>> {
    let url = format!("http://127.0.0.1:{debug_port}/json");
    let client = reqwest::Client::builder()
        .no_proxy()
        .timeout(CDP_COMMAND_TIMEOUT)
        .build()
        .context("failed to build CDP HTTP client")?;
    let response = client
        .get(url)
        .send()
        .await
        .context("failed to query CDP targets")?
        .error_for_status()
        .context("CDP target query failed")?;

    response
        .json::<Vec<CdpTarget>>()
        .await
        .context("failed to deserialize CDP targets")
}

pub fn find_codex_target(targets: &[CdpTarget]) -> anyhow::Result<CdpTarget> {
    let pages = targets.iter().filter(|target| {
        target.target_type == "page"
            && target
                .web_socket_debugger_url
                .as_deref()
                .is_some_and(|url| !url.is_empty())
    });

    let mut first_page = None;
    for target in pages {
        first_page.get_or_insert(target);
        let haystack = format!("{} {}", target.title, target.url).to_lowercase();
        if haystack.contains("codex") {
            return Ok(target.clone());
        }
    }

    if let Some(target) = first_page {
        return Ok(target.clone());
    }

    bail!("No injectable Codex page target found")
}

pub async fn connect_to_page(
    debug_port: u16,
) -> anyhow::Result<
    tokio_tungstenite::WebSocketStream<tokio_tungstenite::MaybeTlsStream<tokio::net::TcpStream>>,
> {
    let target = retry_find_target(debug_port).await?;
    let websocket_url = target
        .web_socket_debugger_url
        .as_deref()
        .ok_or_else(|| anyhow::anyhow!("selected CDP target has no websocket URL"))?;

    let (socket, _) = tokio::time::timeout(
        CDP_CONNECT_TIMEOUT,
        tokio_tungstenite::connect_async(websocket_url),
    )
    .await
    .with_context(|| {
        format!(
            "timed out connecting CDP websocket after {}s",
            CDP_CONNECT_TIMEOUT.as_secs()
        )
    })?
    .context("failed to connect CDP websocket")?;

    Ok(socket)
}

async fn retry_find_target(debug_port: u16) -> anyhow::Result<CdpTarget> {
    let mut last_error = None;
    for _ in 0..CDP_CONNECT_RETRIES {
        match list_targets(debug_port).await {
            Ok(targets) => match find_codex_target(&targets) {
                Ok(target) => return Ok(target),
                Err(err) => last_error = Some(err),
            },
            Err(err) => last_error = Some(err),
        }
        tokio::time::sleep(CDP_RETRY_DELAY).await;
    }
    Err(last_error.unwrap_or_else(|| anyhow::anyhow!("CDP connection timed out")))
}
