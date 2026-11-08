//! DroidGear's native GUI, built with Rune over `droidgear-core`.
//!
//! The Tauri build talks to core through 34 command wrappers, specta bindings, zustand stores and
//! TanStack Query; this one calls core from the view's handlers, so the whole IPC layer is gone.
//! `cargo run` opens the window; `cargo run -- --snap shot.png --theme dark --size 1280x820`
//! renders it headless, which is how the pages are checked without a display.
//!
//! `DROIDGEAR_TRACE_STARTUP=1` prints where launch time goes, from `main` to the first view build.
//! [PERFORMANCE.md](PERFORMANCE.md) has the launch numbers and what was tried to improve them.

mod app;
mod i18n;
mod icons;
mod pages;
mod theme;
mod ui;

use std::sync::OnceLock;
use std::time::Instant;

use rune::{AppSpec, Cx, El, Key, KeyEvent, Tx};

use app::{App, Appearance};

/// Read from `package.json` by the build script, so the two GUIs report the same version.
pub const APP_VERSION: &str = env!("DROIDGEAR_VERSION");

/// Milliseconds since `main` was entered, printed when `DROIDGEAR_TRACE_STARTUP` is set. Launch
/// work is invisible in a profiler otherwise: it happens before the first frame.
pub fn trace(label: &str) {
    static START: OnceLock<Instant> = OnceLock::new();
    let start = *START.get_or_init(Instant::now);
    if std::env::var_os("DROIDGEAR_TRACE_STARTUP").is_some() {
        eprintln!(
            "dg {:>7.2} ms  {label}",
            start.elapsed().as_secs_f64() * 1000.0
        );
    }
}

fn view(s: &App, cx: &mut Cx<App>) -> El {
    trace("first view build");
    let dark = match s.appearance {
        Appearance::System => cx.system_dark(),
        Appearance::Light => false,
        Appearance::Dark => true,
    };
    let k = theme::kit(dark);
    let mut layer = Vec::new();
    let body = ui::shell(s, &k, cx, &mut layer);
    rune_kit::root(body, layer)
}

fn on_key(s: &mut App, ev: &KeyEvent, _tx: &mut Tx) -> bool {
    let primary = ev.mods.cmd || ev.mods.ctrl;
    match ev.key {
        // The sidebar's two switches, so the spike can be exercised without a pointer.
        Key::Command('d') if primary => {
            s.cycle_appearance();
            true
        }
        Key::Command('l') if primary => {
            s.cycle_locale();
            true
        }
        Key::Command('n') if primary => {
            pages::models::open_new(s);
            true
        }
        Key::Command('s') if primary => {
            pages::models::save_draft(s);
            true
        }
        Key::Escape => {
            if s.models().map(|m| m.dialog.is_some()).unwrap_or(false) {
                pages::models::close_dialog(s);
                return true;
            }
            false
        }
        _ => false,
    }
}

fn main() {
    trace("main");

    rune::run(AppSpec {
        on_key: Some(on_key),
        families: ("Inter", "ui-monospace"),
        min_size: Some((880.0, 560.0)),
        // `--scene <name>` renders a page or an overlay without driving the pointer.
        scenes: &[
            ("models", |_s| {}),
            ("add-model", |s| pages::models::open_new(s)),
            ("edit-model", |s| pages::models::open_edit(s, 0)),
            ("delete-model", |s| s.models_mut().pending_delete = Some(0)),
            ("specs", |s| s.open(app::View::Droid, app::PageId::Specs)),
            ("pi", |s| s.open(app::View::Pi, app::PageId::Settings)),
            ("codex", |s| s.open(app::View::Codex, app::PageId::Settings)),
            ("copilot", |s| {
                s.open(app::View::Copilot, app::PageId::Settings)
            }),
            ("light", |s| s.appearance = Appearance::Light),
            ("english", |_s| {
                i18n::I18N.set_locale(i18n::I18N.find_locale("en").unwrap());
            }),
        ],
        ..AppSpec::new("DroidGear", (1280.0, 820.0), App::init, view)
    });
}
