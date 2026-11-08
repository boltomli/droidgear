//! Icons: 16 pt stroke SVGs drawn in the current text colour, the subset of lucide the sidebar
//! and the forms use. The bodies are the lucide paths, so the shapes match the Tauri build.
//! The set grows as pages land, so icons nothing renders yet are fine.
#![allow(dead_code)]

use rune::Icon;

macro_rules! icon {
    ($name:literal, $body:literal) => {
        Icon {
            name: $name,
            view: 16.0,
            body: $body,
        }
    };
}

pub static DROID: Icon = icon!(
    "droid",
    r#"<rect x="2.5" y="3.5" width="11" height="4" rx="1" stroke="currentColor" stroke-width="1.2"/><rect x="2.5" y="8.5" width="11" height="4" rx="1" stroke="currentColor" stroke-width="1.2"/><path d="M5 5.5h.01M5 10.5h.01" stroke="currentColor" stroke-width="1.4" stroke-linecap="round"/>"#
);

pub static OPENCODE: Icon = icon!(
    "opencode",
    r#"<rect x="2" y="3" width="12" height="10" rx="1.5" stroke="currentColor" stroke-width="1.2"/><path d="M4.5 6.5l1.8 1.8-1.8 1.8M8.5 10.5h3" stroke="currentColor" stroke-width="1.2" stroke-linecap="round" stroke-linejoin="round"/>"#
);

pub static CODEX: Icon = icon!(
    "codex",
    r#"<path d="M5.5 5L2.5 8l3 3M10.5 5l3 3-3 3M9 3.5l-2 9" stroke="currentColor" stroke-width="1.2" stroke-linecap="round" stroke-linejoin="round"/>"#
);

pub static CLAUDE: Icon = icon!(
    "claude",
    r#"<path d="M13 2.5c-1 3.5-3 5.5-6.5 6.5M3 13.5c1-3.5 3-5.5 6.5-6.5" stroke="currentColor" stroke-width="1.2" stroke-linecap="round"/><path d="M2.5 4.5c3 1.2 5.5 3.6 7 7M13.5 11.5c-3-1.2-5.5-3.6-7-7" stroke="currentColor" stroke-width="1.1" stroke-linecap="round" opacity="0.75"/>"#
);

pub static OPENCLAW: Icon = icon!(
    "openclaw",
    r#"<circle cx="8" cy="8" r="2.6" stroke="currentColor" stroke-width="1.2"/><path d="M8 1.8v2M8 12.2v2M1.8 8h2M12.2 8h2M3.6 3.6l1.4 1.4M11 11l1.4 1.4M12.4 3.6L11 5M5 11l-1.4 1.4" stroke="currentColor" stroke-width="1.1" stroke-linecap="round"/>"#
);

pub static HERMES: Icon = icon!(
    "hermes",
    r#"<rect x="4.5" y="4.5" width="7" height="7" rx="1" stroke="currentColor" stroke-width="1.2"/><path d="M6.5 1.8v2.7M9.5 1.8v2.7M6.5 11.5v2.7M9.5 11.5v2.7M1.8 6.5h2.7M1.8 9.5h2.7M11.5 6.5h2.7M11.5 9.5h2.7" stroke="currentColor" stroke-width="1.1" stroke-linecap="round"/>"#
);

pub static PI: Icon = icon!(
    "pi",
    r#"<path d="M5.5 3.5v7a2 2 0 002 2h1" stroke="currentColor" stroke-width="1.2" stroke-linecap="round"/><path d="M3 5.5h5" stroke="currentColor" stroke-width="1.2" stroke-linecap="round"/><path d="M11 5.5v7M11 5.5h1.5a1.5 1.5 0 010 3H11" stroke="currentColor" stroke-width="1.2" stroke-linecap="round"/>"#
);

pub static OMP: Icon = icon!(
    "omp",
    r#"<circle cx="8" cy="8" r="5.5" stroke="currentColor" stroke-width="1.2"/><path d="M8 2.5v11M2.5 8h11" stroke="currentColor" stroke-width="1.1"/><path d="M5 5l6 6M11 5l-6 6" stroke="currentColor" stroke-width="1" opacity="0.6"/>"#
);

pub static COPILOT: Icon = icon!(
    "copilot",
    r#"<path d="M3 6.5A2.5 2.5 0 015.5 4h5A2.5 2.5 0 0113 6.5v3A4.5 4.5 0 018.5 14h-1A4.5 4.5 0 013 9.5z" stroke="currentColor" stroke-width="1.2"/><path d="M6 8.5h.01M10 8.5h.01" stroke="currentColor" stroke-width="1.6" stroke-linecap="round"/>"#
);

pub static DSH: Icon = icon!(
    "dsh",
    r#"<path d="M2.5 12.5V7a5.5 5.5 0 0111 0v5.5" stroke="currentColor" stroke-width="1.2" stroke-linecap="round"/><path d="M2.5 9.5h11M6 9.5V13M10 9.5V13" stroke="currentColor" stroke-width="1.1" stroke-linecap="round"/>"#
);

pub static CHANNELS: Icon = icon!(
    "channels",
    r#"<path d="M2.5 4.5h11M2.5 8h11M2.5 11.5h7" stroke="currentColor" stroke-width="1.2" stroke-linecap="round"/>"#
);

pub static SETTINGS: Icon = icon!(
    "settings",
    r#"<path d="M2.5 5h11M2.5 11h11" stroke="currentColor" stroke-width="1.2" stroke-linecap="round"/><circle cx="6" cy="5" r="1.7" fill="currentColor"/><circle cx="10.5" cy="11" r="1.7" fill="currentColor"/>"#
);

pub static SUN: Icon = icon!(
    "sun",
    r#"<circle cx="8" cy="8" r="2.8" stroke="currentColor" stroke-width="1.2"/><path d="M8 1.5v1.8M8 12.7v1.8M1.5 8h1.8M12.7 8h1.8M3.4 3.4l1.3 1.3M11.3 11.3l1.3 1.3M12.6 3.4l-1.3 1.3M4.7 11.3l-1.3 1.3" stroke="currentColor" stroke-width="1.1" stroke-linecap="round"/>"#
);

pub static GLOBE: Icon = icon!(
    "globe",
    r#"<circle cx="8" cy="8" r="5.5" stroke="currentColor" stroke-width="1.2"/><path d="M2.5 8h11M8 2.5c1.6 1.7 2.4 3.6 2.4 5.5S9.6 12.3 8 13.5C6.4 11.8 5.6 9.9 5.6 8S6.4 4.2 8 2.5z" stroke="currentColor" stroke-width="1.1"/>"#
);

pub static PLUS: Icon = icon!(
    "plus",
    r#"<path d="M8 3.5v9M3.5 8h9" stroke="currentColor" stroke-width="1.4" stroke-linecap="round"/>"#
);

pub static TRASH: Icon = icon!(
    "trash",
    r#"<path d="M3 4.5h10M6.5 4.5V3h3v1.5M4.5 4.5l.6 8.2a1 1 0 001 .8h3.8a1 1 0 001-.8l.6-8.2" stroke="currentColor" stroke-width="1.2" stroke-linecap="round" stroke-linejoin="round"/>"#
);

pub static PENCIL: Icon = icon!(
    "pencil",
    r#"<path d="M11.2 2.8l2 2L6 12l-2.7.7L4 10z" stroke="currentColor" stroke-width="1.2" stroke-linejoin="round"/>"#
);

pub static CHECK: Icon = icon!(
    "check",
    r#"<path d="M3.5 8.5l3 3 6-7" stroke="currentColor" stroke-width="1.5" stroke-linecap="round" stroke-linejoin="round"/>"#
);

pub static FILE: Icon = icon!(
    "file",
    r#"<path d="M4 2.5h5l3 3v8h-8z" stroke="currentColor" stroke-width="1.2" stroke-linejoin="round"/><path d="M9 2.5v3h3" stroke="currentColor" stroke-width="1.2" stroke-linejoin="round"/>"#
);
