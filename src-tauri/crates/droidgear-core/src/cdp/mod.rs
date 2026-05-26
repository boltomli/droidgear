//! Chrome DevTools Protocol integration for Codex desktop app injection.
//!
//! Windows only — Codex on macOS/Linux can still use the existing CLI launch path.

pub mod connection;
pub mod injection;

pub use connection::{connect_to_page, find_codex_target, list_targets, CdpTarget};
pub use injection::inject_script;

use std::time::Duration;

pub const CDP_DEFAULT_PORT: u16 = 9229;
pub const CDP_CONNECT_RETRIES: u32 = 20;
pub const CDP_RETRY_DELAY: Duration = Duration::from_millis(500);
pub const CDP_CONNECT_TIMEOUT: Duration = Duration::from_secs(5);
pub const CDP_COMMAND_TIMEOUT: Duration = Duration::from_secs(5);
