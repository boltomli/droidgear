//! App state, navigation and the two pieces of persistence the GUI needs on its own: the
//! appearance and the language, both read from the same `preferences.json` the Tauri build writes.
//!
//! Everything else comes from `droidgear-core`, called directly: no command layer, no bindings.

use std::path::PathBuf;

use crate::i18n::I18N;
use crate::pages::{form, models, specs, Page};

#[derive(Clone, Copy, PartialEq, Eq, Debug)]
pub enum Appearance {
    System,
    Light,
    Dark,
}

impl Appearance {
    pub fn label_key(self) -> &'static str {
        match self {
            Appearance::System => "preferences.appearance.theme.system",
            Appearance::Light => "preferences.appearance.theme.light",
            Appearance::Dark => "preferences.appearance.theme.dark",
        }
    }
}

/// A tool tab in the sidebar, in the order the Tauri build shows them.
#[derive(Clone, Copy, PartialEq, Eq, Debug)]
pub enum View {
    Droid,
    OpenCode,
    Codex,
    Claude,
    OpenClaw,
    Hermes,
    Pi,
    Omp,
    Copilot,
    Dsh,
    Channels,
}

/// A page inside a tool.
#[derive(Clone, Copy, PartialEq, Eq, Debug)]
pub enum PageId {
    Settings,
    Models,
    Mcp,
    Sessions,
    Specs,
    Missions,
    Terminal,
    TrustedFolders,
    LegacyVersions,
}

impl View {
    pub fn label_key(self) -> &'static str {
        match self {
            View::Droid => "sidebar.droid",
            View::OpenCode => "sidebar.opencode",
            View::Codex => "sidebar.codex",
            View::Claude => "sidebar.claude",
            View::OpenClaw => "sidebar.openclaw",
            View::Hermes => "sidebar.hermes",
            View::Pi => "sidebar.pi",
            View::Omp => "sidebar.omp",
            View::Copilot => "sidebar.copilot",
            View::Dsh => "sidebar.dsh",
            View::Channels => "sidebar.channels",
        }
    }

    pub fn icon(self) -> &'static rune::Icon {
        use crate::icons;
        match self {
            View::Droid => &icons::DROID,
            View::OpenCode => &icons::OPENCODE,
            View::Codex => &icons::CODEX,
            View::Claude => &icons::CLAUDE,
            View::OpenClaw => &icons::OPENCLAW,
            View::Hermes => &icons::HERMES,
            View::Pi => &icons::PI,
            View::Omp => &icons::OMP,
            View::Copilot => &icons::COPILOT,
            View::Dsh => &icons::DSH,
            View::Channels => &icons::CHANNELS,
        }
    }

    /// The pages this tool shows under its sidebar row. P0 has Models and Specs for Droid; the
    /// rest arrive with the form engine in P1.
    pub fn pages(self) -> &'static [PageId] {
        match self {
            View::Droid => &[
                PageId::Settings,
                PageId::Models,
                PageId::Mcp,
                PageId::Sessions,
                PageId::Specs,
                PageId::Missions,
                PageId::Terminal,
                PageId::TrustedFolders,
                PageId::LegacyVersions,
            ],
            _ => &[PageId::Settings],
        }
    }
}

impl PageId {
    pub fn label_key(self) -> &'static str {
        match self {
            PageId::Settings => "droid.features.settings",
            PageId::Models => "droid.features.models",
            PageId::Mcp => "droid.features.mcp",
            PageId::Sessions => "droid.features.sessions",
            PageId::Specs => "droid.features.specs",
            PageId::Missions => "droid.features.missions",
            PageId::Terminal => "droid.features.terminal",
            PageId::TrustedFolders => "droid.features.trustedFolders",
            PageId::LegacyVersions => "droid.features.legacyVersions",
        }
    }
}

pub struct App {
    pub view: View,
    pub id: PageId,
    pub page: Page,
    pub appearance: Appearance,
    pub locale: usize,
    /// Set for one commit after a save, so a page can show its "saved" line.
    pub flash: Option<&'static str>,
    prefs: serde_json::Value,
}

impl App {
    pub fn init() -> App {
        crate::trace("init: start");
        let prefs = load_prefs();
        crate::trace("init: preferences");
        let appearance = match prefs.get("theme").and_then(|v| v.as_str()) {
            Some("light") => Appearance::Light,
            Some("dark") => Appearance::Dark,
            Some("system") => Appearance::System,
            _ => Appearance::System,
        };
        let locale = match prefs.get("language").and_then(|v| v.as_str()) {
            Some(id) => I18N
                .find_locale(id)
                .unwrap_or_else(crate::i18n::system_locale),
            None => crate::i18n::system_locale(),
        };
        I18N.set_locale(locale);
        crate::trace("init: appearance + language");

        let mut app = App {
            view: View::Droid,
            id: PageId::Models,
            page: Page::Blank,
            appearance,
            locale,
            flash: None,
            prefs,
        };
        app.open(View::Droid, PageId::Models);
        crate::trace("init: first page loaded");
        app
    }

    /// Switches page, loading its data from core.
    pub fn open(&mut self, view: View, id: PageId) {
        self.view = view;
        self.id = id;
        self.flash = None;
        self.page = match (view, id) {
            (View::Droid, PageId::Models) => Page::Models(Box::new(models::Models::load())),
            (View::Droid, PageId::Specs) => Page::Specs(specs::Specs::load()),
            (View::Pi, PageId::Settings) => {
                Page::Form(Box::new(form::FormPage::load(form::Tool::Pi)))
            }
            (View::Codex, PageId::Settings) => {
                Page::Form(Box::new(form::FormPage::load(form::Tool::Codex)))
            }
            (View::Copilot, PageId::Settings) => {
                Page::Form(Box::new(form::FormPage::load(form::Tool::Copilot)))
            }
            _ => Page::Soon,
        };
    }

    pub fn cycle_appearance(&mut self) {
        self.appearance = match self.appearance {
            Appearance::System => Appearance::Light,
            Appearance::Light => Appearance::Dark,
            Appearance::Dark => Appearance::System,
        };
        let value = match self.appearance {
            Appearance::System => "system",
            Appearance::Light => "light",
            Appearance::Dark => "dark",
        };
        self.prefs["theme"] = serde_json::Value::String(value.to_owned());
        save_prefs(&self.prefs);
    }

    pub fn cycle_locale(&mut self) {
        let count = I18N.locales().len();
        self.locale = (self.locale + 1) % count;
        I18N.set_locale(self.locale);
        self.prefs["language"] =
            serde_json::Value::String(I18N.locales()[self.locale].id().to_owned());
        save_prefs(&self.prefs);
    }

    pub fn models(&self) -> Option<&models::Models> {
        match &self.page {
            Page::Models(m) => Some(m),
            _ => None,
        }
    }

    pub fn models_mut(&mut self) -> &mut models::Models {
        match &mut self.page {
            Page::Models(m) => m,
            _ => panic!("models page is not open"),
        }
    }

    pub fn form_mut(&mut self) -> &mut form::FormPage {
        match &mut self.page {
            Page::Form(page) => page,
            _ => panic!("form page is not open"),
        }
    }

    pub fn specs(&self) -> Option<&specs::Specs> {
        match &self.page {
            Page::Specs(s) => Some(s),
            _ => None,
        }
    }

    pub fn specs_mut(&mut self) -> &mut specs::Specs {
        match &mut self.page {
            Page::Specs(s) => s,
            _ => panic!("specs page is not open"),
        }
    }
}

// ---------------------------------------------------------------------------
// Preferences: the same JSON the Tauri build keeps, so both GUIs agree.
// ---------------------------------------------------------------------------

fn prefs_path() -> Option<PathBuf> {
    dirs::data_dir().map(|dir| dir.join("com.droidgear.app").join("preferences.json"))
}

fn load_prefs() -> serde_json::Value {
    let Some(path) = prefs_path() else {
        return serde_json::json!({});
    };
    let Ok(text) = std::fs::read_to_string(&path) else {
        return serde_json::json!({});
    };
    serde_json::from_str(&text).unwrap_or_else(|_| serde_json::json!({}))
}

fn save_prefs(prefs: &serde_json::Value) {
    let Some(path) = prefs_path() else { return };
    let Ok(text) = serde_json::to_string_pretty(prefs) else {
        return;
    };
    if let Some(parent) = path.parent() {
        let _ = std::fs::create_dir_all(parent);
    }
    let _ = std::fs::write(&path, text);
}
