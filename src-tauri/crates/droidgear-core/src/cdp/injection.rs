use anyhow::{bail, Context};
use futures_util::{SinkExt, StreamExt};
use serde_json::{json, Value};
use std::sync::atomic::{AtomicU64, Ordering};
use tokio_tungstenite::tungstenite::Message;

use crate::cdp::{CDP_COMMAND_TIMEOUT, CDP_CONNECT_TIMEOUT};

static NEXT_MESSAGE_ID: AtomicU64 = AtomicU64::new(1);

pub async fn inject_script(debug_port: u16, script: &str) -> anyhow::Result<Value> {
    let websocket_url = {
        let targets = super::connection::list_targets(debug_port).await?;
        let target = super::connection::find_codex_target(&targets)?;
        target
            .web_socket_debugger_url
            .as_deref()
            .ok_or_else(|| anyhow::anyhow!("selected CDP target has no websocket URL"))?
            .to_string()
    };

    let (socket, _) = tokio::time::timeout(
        CDP_CONNECT_TIMEOUT,
        tokio_tungstenite::connect_async(&websocket_url),
    )
    .await
    .with_context(|| {
        format!(
            "timed out connecting CDP websocket after {}s",
            CDP_CONNECT_TIMEOUT.as_secs()
        )
    })?
    .context("failed to connect CDP websocket")?;

    let (mut write, mut read) = socket.split();

    let id = NEXT_MESSAGE_ID.fetch_add(1, Ordering::Relaxed);
    let params = json!({
        "expression": script,
        "awaitPromise": false,
        "allowUnsafeEvalBlockedByCSP": true,
    });
    let command = json!({
        "id": id,
        "method": "Runtime.evaluate",
        "params": params,
    });

    write
        .send(Message::Text(command.to_string()))
        .await
        .context("failed to send CDP Runtime.evaluate command")?;

    let result = tokio::time::timeout(CDP_COMMAND_TIMEOUT, async {
        while let Some(message) = read.next().await {
            let message = message.context("failed to read CDP websocket message")?;
            let Message::Text(text) = message else {
                continue;
            };
            let value: Value =
                serde_json::from_str(&text).context("failed to parse CDP message")?;
            if value.get("id").and_then(Value::as_u64) == Some(id) {
                return Ok(value);
            }
        }
        bail!("CDP websocket closed before response");
    })
    .await
    .with_context(|| {
        format!(
            "timed out waiting for Runtime.evaluate response after {}s",
            CDP_COMMAND_TIMEOUT.as_secs()
        )
    })??;

    if let Some(error) = result.get("error") {
        bail!("CDP Runtime.evaluate failed: {error}");
    }

    Ok(result)
}
