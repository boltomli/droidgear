//! A schema-driven settings form, and the tool pages that are just a schema plus a loader.
//!
//! Every tool in core exposes the same shape: `<tool>::list_<tool>_profiles()`,
//! `get_<tool>_profile(id)`, `save_<tool>_profile(profile)` and an active profile id. Profiles are
//! `serde` structs, so the form edits a `serde_json::Value` of the profile and deserializes it
//! back on save: one form implementation serves every tool, and a field the schema doesn't list
//! survives the round trip untouched.
//!
//! What the schema can express: text, secret, number, boolean, choice and text-area fields, in
//! titled sections, addressed by dotted path into the profile's JSON. Lists and maps (a tool's
//! `providers`) need their own editors and are the next phase; the page says so.

use droidgear_core::{codex, copilot, pi};
use rune::{col, el, row, text, Color, El, ScrollSpec, TextEdit};
use rune_kit::{
    Badge, Button, ButtonVariant, Field, Kit, Select, SelectAppearance, Switch, TextArea, TextInput,
};

use crate::app::App;
use crate::t;
use crate::ui;

#[derive(Clone, Copy)]
pub enum Kind {
    Text,
    /// A value shown as dots, for keys.
    Secret,
    Number,
    Bool,
    Choice(&'static [Choice]),
    TextArea,
}

#[derive(Clone, Copy)]
pub struct Choice {
    /// What the document stores. Empty clears the key (an unset optional field).
    pub value: &'static str,
    pub label_key: &'static str,
}

pub struct FieldSpec {
    /// Dotted path into the profile's JSON, e.g. `baseUrl` or `providers.x.baseUrl`.
    pub path: &'static str,
    pub label_key: &'static str,
    pub kind: Kind,
    pub placeholder: &'static str,
}

pub struct SectionSpec {
    pub title_key: &'static str,
    pub fields: &'static [FieldSpec],
}

pub struct Schema {
    pub title_key: &'static str,
    pub sections: &'static [SectionSpec],
}

impl Schema {
    pub fn fields(&self) -> impl Iterator<Item = &FieldSpec> {
        self.sections.iter().flat_map(|s| s.fields.iter())
    }
}

/// The tools with a form in this phase.
#[derive(Clone, Copy, PartialEq, Eq)]
pub enum Tool {
    Pi,
    Codex,
    Copilot,
}

impl Tool {
    pub fn schema(self) -> &'static Schema {
        match self {
            Tool::Pi => &PI,
            Tool::Codex => &CODEX,
            Tool::Copilot => &COPILOT,
        }
    }

    /// One entry per editable profile, with the one core applies marked.
    fn profiles(self) -> Result<Vec<ProfileRef>, String> {
        match self {
            // Pi's default model lives in one settings document, not in profiles.
            Tool::Pi => Ok(Vec::new()),
            Tool::Codex => {
                let active = codex::get_active_codex_profile_id().unwrap_or_default();
                let profiles = codex::list_codex_profiles()?;
                Ok(profiles
                    .into_iter()
                    .map(|p| ProfileRef {
                        active: active.as_deref() == Some(p.id.as_str()),
                        id: p.id,
                        name: p.name,
                    })
                    .collect())
            }
            Tool::Copilot => {
                let active = copilot::get_active_copilot_profile_id().unwrap_or_default();
                let profiles = copilot::list_copilot_profiles()?;
                Ok(profiles
                    .into_iter()
                    .map(|p| ProfileRef {
                        active: active.as_deref() == Some(p.id.as_str()),
                        id: p.id,
                        name: p.name,
                    })
                    .collect())
            }
        }
    }

    /// The document to edit, its subtitle (the file it lands in) and nothing else.
    fn load(self, profile: Option<&str>) -> Result<(String, serde_json::Value), String> {
        match self {
            Tool::Pi => {
                let status = pi::read_pi_settings()?;
                let settings = status.settings();
                Ok((
                    status.settings_path.clone(),
                    serde_json::to_value(&settings).map_err(|e| e.to_string())?,
                ))
            }
            Tool::Codex => {
                let profile = codex::get_codex_profile(profile.unwrap_or_default())?;
                Ok((
                    profile.name.clone(),
                    serde_json::to_value(&profile).map_err(|e| e.to_string())?,
                ))
            }
            Tool::Copilot => {
                let profile = copilot::get_copilot_profile(profile.unwrap_or_default())?;
                Ok((
                    profile.name.clone(),
                    serde_json::to_value(&profile).map_err(|e| e.to_string())?,
                ))
            }
        }
    }

    fn save(self, profile: Option<&str>, value: &serde_json::Value) -> Result<(), String> {
        match self {
            Tool::Pi => {
                let settings: droidgear_core::pi_settings::PiSettings =
                    serde_json::from_value(value.clone()).map_err(|e| e.to_string())?;
                pi::save_pi_settings(settings)
            }
            Tool::Codex => {
                let profile: codex::CodexProfile =
                    serde_json::from_value(value.clone()).map_err(|e| e.to_string())?;
                codex::save_codex_profile(profile)
            }
            Tool::Copilot => {
                let profile: copilot::CopilotProfile =
                    serde_json::from_value(value.clone()).map_err(|e| e.to_string())?;
                copilot::save_copilot_profile(profile)
            }
        }
        .map_err(|e| e.to_string())?;
        let _ = profile;
        Ok(())
    }

    /// Whether an empty profile list means "nothing to edit yet" (profiles) or "fine" (Pi).
    fn needs_profile(self) -> bool {
        !matches!(self, Tool::Pi)
    }
}

pub struct ProfileRef {
    pub id: String,
    pub name: String,
    pub active: bool,
}

pub struct FormPage {
    pub tool: Tool,
    pub schema: &'static Schema,
    pub profiles: Vec<ProfileRef>,
    pub selected: usize,
    pub doc: serde_json::Value,
    pub subtitle: String,
    /// One buffer per field, in schema order: the truth while editing.
    pub edits: Vec<TextEdit>,
    pub error: Option<String>,
    pub notice: Option<&'static str>,
    pub open_select: Option<usize>,
    pub dirty: bool,
}

impl FormPage {
    pub fn load(tool: Tool) -> FormPage {
        let schema = tool.schema();
        let mut page = FormPage {
            tool,
            schema,
            profiles: Vec::new(),
            selected: 0,
            doc: serde_json::json!({}),
            subtitle: String::new(),
            edits: Vec::new(),
            error: None,
            notice: None,
            open_select: None,
            dirty: false,
        };
        match tool.profiles() {
            Ok(profiles) => page.profiles = profiles,
            Err(error) => page.error = Some(error),
        }
        page.reload();
        page
    }

    fn profile_id(&self) -> Option<&str> {
        self.profiles.get(self.selected).map(|p| p.id.as_str())
    }

    /// Re-reads the document from core and refills the field buffers.
    pub fn reload(&mut self) {
        let id = self.profile_id().map(str::to_owned);
        match self.tool.load(id.as_deref()) {
            Ok((subtitle, doc)) => {
                self.subtitle = subtitle;
                self.doc = doc;
                self.edits = self
                    .schema
                    .fields()
                    .map(|field| {
                        TextEdit::new(
                            text_value(&self.doc, field.path)
                                .as_deref()
                                .unwrap_or_default(),
                        )
                    })
                    .collect();
                self.dirty = false;
                if self.error.is_none() {
                    self.notice = None;
                }
            }
            Err(error) => self.error = Some(error),
        }
    }

    /// Writes the field buffers back into the document and saves it.
    pub fn save(&mut self) {
        let mut doc = self.doc.clone();
        for (index, field) in self.schema.fields().enumerate() {
            match field.kind {
                Kind::Bool => {}
                Kind::Choice(_) => {}
                Kind::Text | Kind::Secret | Kind::TextArea | Kind::Number => {
                    let value = self.edits[index].text.trim().to_owned();
                    set_path(&mut doc, field.path, value);
                }
            }
        }
        let id = self.profile_id().map(str::to_owned);
        match self.tool.save(id.as_deref(), &doc) {
            Ok(()) => {
                self.doc = doc;
                self.error = None;
                self.notice = Some("common.saved");
                self.dirty = false;
            }
            Err(error) => {
                self.error = Some(error);
                self.notice = None;
            }
        }
    }
}

// ---------------------------------------------------------------------------
// JSON helpers: dotted paths, with strings written trimmed and empty ones removed.
// ---------------------------------------------------------------------------

pub fn text_value(doc: &serde_json::Value, path: &str) -> Option<String> {
    let mut current = doc;
    for part in path.split('.') {
        current = current.get(part)?;
    }
    match current {
        serde_json::Value::String(text) => Some(text.clone()),
        serde_json::Value::Number(number) => Some(number.to_string()),
        serde_json::Value::Bool(value) => Some(value.to_string()),
        _ => None,
    }
}

pub fn bool_value(doc: &serde_json::Value, path: &str) -> bool {
    let mut current = doc;
    for part in path.split('.') {
        let Some(next) = current.get(part) else {
            return false;
        };
        current = next;
    }
    current.as_bool().unwrap_or(false)
}

/// Sets `path`, writing a number where the field is a `Number` and clearing empty text so an
/// unset optional field stays absent instead of becoming `""`.
fn set_path(doc: &mut serde_json::Value, path: &str, value: String) {
    if value.is_empty() {
        if let Some(object) = doc.as_object_mut() {
            object.remove(path);
        }
        return;
    }
    let number = value.parse::<i64>().ok();
    let stored = match number {
        Some(number) => serde_json::Value::Number(number.into()),
        None => serde_json::Value::String(value),
    };
    if let Some(object) = doc.as_object_mut() {
        object.insert(path.to_owned(), stored);
    }
}

fn set_bool(doc: &mut serde_json::Value, path: &str, value: bool) {
    if let Some(object) = doc.as_object_mut() {
        object.insert(path.to_owned(), serde_json::Value::Bool(value));
    }
}

// ---------------------------------------------------------------------------
// Actions (the view's handlers land here)
// ---------------------------------------------------------------------------

pub fn toggle_bool(app: &mut App, index: usize) {
    let page = app.form_mut();
    let Some(field) = page.schema.fields().nth(index) else {
        return;
    };
    let path = field.path;
    let next = !bool_value(&page.doc, path);
    set_bool(&mut page.doc, path, next);
    page.dirty = true;
}

pub fn choose(app: &mut App, index: usize, value: &'static str) {
    let page = app.form_mut();
    let Some(field) = page.schema.fields().nth(index) else {
        return;
    };
    let path = field.path;
    page.open_select = None;
    let mut doc = page.doc.clone();
    let object = doc.as_object_mut();
    match object {
        Some(object) if value.is_empty() => {
            object.remove(path);
        }
        Some(object) => {
            object.insert(path.to_owned(), serde_json::Value::String(value.to_owned()));
        }
        None => {}
    }
    page.doc = doc;
    page.dirty = true;
}

pub fn select_profile(app: &mut App, index: usize) {
    let page = app.form_mut();
    if index == page.selected {
        return;
    }
    page.selected = index;
    page.reload();
}

pub fn toggle_select(app: &mut App, index: usize) {
    let page = app.form_mut();
    page.open_select = match page.open_select {
        Some(open) if open == index => None,
        _ => Some(index),
    };
}

pub fn close_select(app: &mut App) {
    app.form_mut().open_select = None;
}

pub fn reset(app: &mut App) {
    app.form_mut().reload();
}

// ---------------------------------------------------------------------------
// View
// ---------------------------------------------------------------------------

pub fn view(page: &FormPage, k: &Kit, layer: &mut Vec<El>) -> El {
    if page.tool.needs_profile() && page.profiles.is_empty() {
        return empty_profiles(page, k);
    }

    let actions = row()
        .key("actions")
        .gap(8.0)
        .items_center()
        .child(
            Button::new(t!("common.reset"))
                .variant(ButtonVariant::Ghost)
                .disabled(!page.dirty)
                .on_press(reset)
                .build(k),
        )
        .child(
            Button::new(t!("common.save"))
                .variant(ButtonVariant::Contrast)
                .disabled(!page.dirty)
                .on_press(|app: &mut App| app.form_mut().save())
                .build(k),
        );

    let mut body = col()
        .key("form-page")
        .w_full()
        .h_full()
        .child(ui::page_header(
            k,
            t!(page.schema.title_key),
            &page.subtitle,
            Some(actions),
        ));

    if page.tool.needs_profile() {
        let mut bar = row()
            .key("profiles")
            .w_full()
            .px(24.0)
            .pb(10.0)
            .gap(6.0)
            .items_center();
        for (index, profile) in page.profiles.iter().enumerate() {
            let selected = index == page.selected;
            let mut chip = row()
                .key(format!("profile:{}", profile.id))
                .h(26.0)
                .px(10.0)
                .gap(6.0)
                .items_center()
                .radius(k.radius.md)
                .role("button")
                .label(&profile.name)
                .focusable(true)
                .fill(if selected {
                    k.c.layer_02
                } else {
                    Color::TRANSPARENT
                })
                .hover(if selected {
                    Color::TRANSPARENT
                } else {
                    k.c.hover
                })
                .on_press(move |app: &mut App| select_profile(app, index))
                .child(
                    text(&profile.name)
                        .key("name")
                        .font(k.ty.ui)
                        .color(if selected {
                            k.c.text_base
                        } else {
                            k.c.text_muted
                        })
                        .a11y_hidden(),
                );
            if profile.active {
                chip = chip.child(Badge::new(t!("gui.profile.active")).build(k));
            }
            bar = bar.child(chip);
        }
        body = body.child(bar);
    }

    if let Some(error) = &page.error {
        body = body.child(ui::status_line(k, error, true));
    }
    if page.notice.is_some() {
        body = body.child(ui::status_line(k, t!("common.saved"), false));
    }

    let mut sections = col().key("sections").w_full().px(24.0).pb(24.0).gap(16.0);
    let mut field_index = 0;
    for section in page.schema.sections {
        let mut card = ui::card(k, 2.0);
        if !section.title_key.is_empty() {
            card = card.child(
                text(t!(section.title_key))
                    .key("section")
                    .w_full()
                    .px(10.0)
                    .pt(8.0)
                    .pb(4.0)
                    .font(k.ty.small_medium)
                    .color(k.c.text_muted),
            );
        }
        for field in section.fields {
            card = card.child(field_row(page, k, layer, field_index, field));
            field_index += 1;
        }
        sections = sections.child(card);
    }

    body.child(
        el().key("scroll")
            .grow(1.0)
            .min_h(0.0)
            .w_full()
            .scroll(ScrollSpec {
                stick_bottom: false,
            })
            .child(sections),
    )
}

/// One labelled control. Booleans and choices read the document; text kinds read their buffer.
fn field_row(page: &FormPage, k: &Kit, layer: &mut Vec<El>, index: usize, field: &FieldSpec) -> El {
    let label = t!(field.label_key);
    let control = match field.kind {
        Kind::Text => TextInput::new(&page.edits[index])
            .placeholder(field.placeholder)
            .width(None)
            .on_edit(move |app: &mut App| {
                let page = app.form_mut();
                page.dirty = true;
                &mut page.edits[index]
            })
            .build(k),
        Kind::Secret => TextInput::new(&page.edits[index])
            .placeholder(field.placeholder)
            .width(None)
            .secure()
            .on_edit(move |app: &mut App| {
                let page = app.form_mut();
                page.dirty = true;
                &mut page.edits[index]
            })
            .build(k),
        Kind::Number => TextInput::new(&page.edits[index])
            .placeholder(field.placeholder)
            .width(None)
            .on_edit(move |app: &mut App| {
                let page = app.form_mut();
                page.dirty = true;
                &mut page.edits[index]
            })
            .build(k),
        Kind::TextArea => TextArea::new(&page.edits[index])
            .placeholder(field.placeholder)
            .width(None)
            .rows(3)
            .on_edit(move |app: &mut App| {
                let page = app.form_mut();
                page.dirty = true;
                &mut page.edits[index]
            })
            .build(k),
        Kind::Bool => {
            let on = bool_value(&page.doc, field.path);
            // The row carries the accessible switch, so the control itself stays out of the tree
            // and the two can't disagree about what was pressed.
            return row()
                .key(format!("field:{index}"))
                .w_full()
                .h(38.0)
                .px(10.0)
                .items_center()
                .role("switch")
                .label(label)
                .checked(on)
                .child(
                    text(label)
                        .key("label")
                        .font(k.ty.ui)
                        .color(k.c.text_base)
                        .grow(1.0),
                )
                .child(
                    Switch::new(on)
                        .on_toggle(move |app: &mut App| toggle_bool(app, index))
                        .build(k)
                        .a11y_hidden(),
                );
        }
        Kind::Choice(choices) => {
            let current = text_value(&page.doc, field.path).unwrap_or_default();
            let labels: Vec<&'static str> = choices.iter().map(|c| t!(c.label_key)).collect();
            let selected = choices
                .iter()
                .position(|c| c.value == current)
                .map(|i| labels[i])
                .unwrap_or_else(|| t!("common.noChoices"));
            let open = page.open_select == Some(index);
            Select::new(format!("select:{index}"), &labels)
                .appearance(SelectAppearance::Field)
                .current(Some(selected))
                .open(open)
                .on_toggle(move |app: &mut App| toggle_select(app, index))
                .on_close(close_select)
                .on_choose(move |app: &mut App, value: &str, _| {
                    let Some(choice) = choices.iter().find(|c| t!(c.label_key) == value) else {
                        return;
                    };
                    choose(app, index, choice.value);
                })
                .build(k, layer)
        }
    };

    Field::new().label(label).control(control).build(k)
}

fn empty_profiles(page: &FormPage, k: &Kit) -> El {
    col()
        .key("no-profiles")
        .w_full()
        .h_full()
        .child(ui::page_header(
            k,
            t!(page.schema.title_key),
            &page.subtitle,
            None,
        ))
        .child(
            col().key("body").w_full().padding(24.0).child(
                col()
                    .key("card")
                    .w_full()
                    .padding(22.0)
                    .gap(6.0)
                    .radius(k.radius.lg)
                    .fill(k.c.bg_base)
                    .border(1.0, k.c.border_muted)
                    .child(
                        text(t!("gui.noProfiles.title"))
                            .key("title")
                            .font(k.ty.body_medium)
                            .color(k.c.text_base),
                    )
                    .child(
                        text(t!("gui.noProfiles.body"))
                            .key("body")
                            .font(k.ty.text_xsmall)
                            .color(k.c.text_muted),
                    ),
            ),
        )
}

// ---------------------------------------------------------------------------
// The schemas
// ---------------------------------------------------------------------------

const THINKING: &[Choice] = &[
    Choice {
        value: "",
        label_key: "pi.defaults.auto",
    },
    Choice {
        value: "low",
        label_key: "models.reasoningEffort.low",
    },
    Choice {
        value: "medium",
        label_key: "models.reasoningEffort.medium",
    },
    Choice {
        value: "high",
        label_key: "models.reasoningEffort.high",
    },
];

static PI: Schema = Schema {
    title_key: "pi.defaults.title",
    sections: &[SectionSpec {
        title_key: "",
        fields: &[
            FieldSpec {
                path: "defaultProvider",
                label_key: "pi.defaults.provider",
                kind: Kind::Text,
                placeholder: "wududu",
            },
            FieldSpec {
                path: "defaultModel",
                label_key: "pi.defaults.model",
                kind: Kind::Text,
                placeholder: "deepseek-v4.1-flash",
            },
            FieldSpec {
                path: "defaultThinkingLevel",
                label_key: "pi.defaults.thinkingLevel",
                kind: Kind::Choice(THINKING),
                placeholder: "",
            },
        ],
    }],
};

const CODEX_PROVIDERS: &[Choice] = &[
    Choice {
        value: "custom",
        label_key: "codex.profile.modelProviderCustom",
    },
    Choice {
        value: "openai",
        label_key: "codex.profile.modelProviderOpenai",
    },
];

const CODEX_EFFORT: &[Choice] = &[
    Choice {
        value: "",
        label_key: "models.reasoningEffort.none",
    },
    Choice {
        value: "low",
        label_key: "models.reasoningEffort.low",
    },
    Choice {
        value: "medium",
        label_key: "models.reasoningEffort.medium",
    },
    Choice {
        value: "high",
        label_key: "models.reasoningEffort.high",
    },
];

static CODEX: Schema = Schema {
    title_key: "codex.title",
    sections: &[
        SectionSpec {
            title_key: "",
            fields: &[
                FieldSpec {
                    path: "name",
                    label_key: "codex.profile.name",
                    kind: Kind::Text,
                    placeholder: "",
                },
                FieldSpec {
                    path: "description",
                    label_key: "codex.profile.description",
                    kind: Kind::TextArea,
                    placeholder: "",
                },
            ],
        },
        SectionSpec {
            title_key: "codex.modelSelection.title",
            fields: &[
                FieldSpec {
                    path: "modelProvider",
                    label_key: "codex.profile.modelProvider",
                    kind: Kind::Choice(CODEX_PROVIDERS),
                    placeholder: "",
                },
                FieldSpec {
                    path: "model",
                    label_key: "codex.modelSelection.model",
                    kind: Kind::Text,
                    placeholder: "gpt-5.4",
                },
                FieldSpec {
                    path: "modelReasoningEffort",
                    label_key: "codex.modelSelection.reasoningEffort",
                    kind: Kind::Choice(CODEX_EFFORT),
                    placeholder: "",
                },
            ],
        },
        SectionSpec {
            title_key: "codex.auth.title",
            fields: &[
                FieldSpec {
                    path: "apiKey",
                    label_key: "codex.auth.apiKey",
                    kind: Kind::Secret,
                    placeholder: "sk-…",
                },
                FieldSpec {
                    path: "apiKeyModelDiscovery",
                    label_key: "codex.profile.apiKeyModelDiscovery",
                    kind: Kind::Bool,
                    placeholder: "",
                },
            ],
        },
    ],
};

const COPILOT_TYPES: &[Choice] = &[
    Choice {
        value: "openai",
        label_key: "copilot.provider.officialOption",
    },
    Choice {
        value: "anthropic",
        label_key: "copilot.provider.byokOption",
    },
];

static COPILOT: Schema = Schema {
    title_key: "copilot.title",
    sections: &[
        SectionSpec {
            title_key: "",
            fields: &[
                FieldSpec {
                    path: "name",
                    label_key: "copilot.profile.name",
                    kind: Kind::Text,
                    placeholder: "",
                },
                FieldSpec {
                    path: "description",
                    label_key: "copilot.profile.description",
                    kind: Kind::TextArea,
                    placeholder: "",
                },
                FieldSpec {
                    path: "useOfficialAuth",
                    label_key: "copilot.provider.mode",
                    kind: Kind::Bool,
                    placeholder: "",
                },
            ],
        },
        SectionSpec {
            title_key: "copilot.provider.title",
            fields: &[
                FieldSpec {
                    path: "providerType",
                    label_key: "copilot.provider.providerType",
                    kind: Kind::Choice(COPILOT_TYPES),
                    placeholder: "",
                },
                FieldSpec {
                    path: "baseUrl",
                    label_key: "copilot.provider.baseUrl",
                    kind: Kind::Text,
                    placeholder: "https://api.githubcopilot.com",
                },
                FieldSpec {
                    path: "apiKey",
                    label_key: "copilot.provider.apiKey",
                    kind: Kind::Secret,
                    placeholder: "sk-…",
                },
                FieldSpec {
                    path: "model",
                    label_key: "copilot.provider.model",
                    kind: Kind::Text,
                    placeholder: "gpt-5.4",
                },
                FieldSpec {
                    path: "maxPromptTokens",
                    label_key: "copilot.provider.maxPromptTokens",
                    kind: Kind::Number,
                    placeholder: "128000",
                },
                FieldSpec {
                    path: "maxOutputTokens",
                    label_key: "copilot.provider.maxOutputTokens",
                    kind: Kind::Number,
                    placeholder: "16384",
                },
            ],
        },
    ],
};

#[cfg(test)]
mod tests {
    use super::*;

    #[test]
    fn text_value_reads_nested_paths() {
        let doc = serde_json::json!({"baseUrl": "https://x", "providers": {"a": {"model": "m"}}});
        assert_eq!(text_value(&doc, "baseUrl").as_deref(), Some("https://x"));
        assert_eq!(text_value(&doc, "providers.a.model").as_deref(), Some("m"));
        assert_eq!(text_value(&doc, "providers.b.model"), None);
    }

    #[test]
    fn set_path_writes_numbers_and_drops_empty_text() {
        let mut doc = serde_json::json!({"maxOutputTokens": null});
        set_path(&mut doc, "maxOutputTokens", "16384".to_owned());
        assert_eq!(doc["maxOutputTokens"], serde_json::json!(16384));
        set_path(&mut doc, "maxOutputTokens", String::new());
        assert!(doc.get("maxOutputTokens").is_none());
    }

    #[test]
    fn every_schema_key_exists_and_paths_are_unique() {
        for tool in [Tool::Pi, Tool::Codex, Tool::Copilot] {
            let schema = tool.schema();
            let mut paths = Vec::new();
            for field in schema.fields() {
                assert!(
                    !paths.contains(&field.path),
                    "duplicate path {}",
                    field.path
                );
                paths.push(field.path);
                // `t!` would fail the build for a missing key; this catches the runtime ones too.
                assert!(
                    !crate::i18n::tr(field.label_key).is_empty(),
                    "{}",
                    field.label_key
                );
                if let Kind::Choice(choices) = field.kind {
                    for choice in choices {
                        assert!(!crate::i18n::tr(choice.label_key).is_empty());
                    }
                }
            }
        }
    }
}
