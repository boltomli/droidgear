//! BYOK models: the list `~/.factory/config.json` holds, and the editor for one of them.
//!
//! This is the spike's "list + form dialog" page. It reads and writes through
//! `droidgear_core::factory_settings`, the same functions the Tauri commands and the TUI call.

use droidgear_core::factory_settings::{self, CustomModel, Provider};
use rune::{col, el, row, text, Color, El, ScrollSpec, TextEdit};
use rune_kit::{
    Badge, Button, ButtonVariant, Dialog, Field, IconButton, IconButtonVariant, Kit, Select,
    Switch, TextInput,
};

use crate::app::App;
use crate::icons;
use crate::t;
use crate::ui;

/// The providers Factory BYOK accepts, with the label each shows in the picker.
const PROVIDERS: &[Provider] = &[
    Provider::Anthropic,
    Provider::Openai,
    Provider::GenericChatCompletionApi,
];

fn provider_label(index: usize) -> &'static str {
    match index {
        0 => t!("models.providerAnthropic"),
        1 => t!("models.providerOpenAI"),
        _ => t!("models.providerGeneric"),
    }
}

fn provider_index(provider: &Provider) -> usize {
    PROVIDERS
        .iter()
        .position(|p| p == provider)
        .unwrap_or(PROVIDERS.len() - 1)
}

pub struct Models {
    pub models: Vec<CustomModel>,
    pub config_path: String,
    pub error: Option<String>,
    pub notice: Option<&'static str>,
    pub dialog: Option<Draft>,
    pub pending_delete: Option<usize>,
}

/// One model being edited. `index` is `None` for a model being added.
pub struct Draft {
    pub index: Option<usize>,
    pub model_id: TextEdit,
    pub display_name: TextEdit,
    pub base_url: TextEdit,
    pub api_key: TextEdit,
    pub max_tokens: TextEdit,
    pub provider: usize,
    pub no_image: bool,
    pub select_open: bool,
    pub error: Option<String>,
}

impl Models {
    pub fn load() -> Models {
        let (models, error) = match factory_settings::load_custom_models() {
            Ok(models) => (models, None),
            Err(error) => (Vec::new(), Some(error)),
        };
        Models {
            models,
            config_path: factory_settings::get_config_path().unwrap_or_default(),
            error,
            notice: None,
            dialog: None,
            pending_delete: None,
        }
    }

    fn reload(&mut self) {
        match factory_settings::load_custom_models() {
            Ok(models) => {
                self.models = models;
                self.error = None;
            }
            Err(error) => self.error = Some(error),
        }
    }
}

impl Draft {
    pub fn new(source: Option<(usize, &CustomModel)>) -> Draft {
        let (index, model) = match source {
            Some((index, model)) => (Some(index), model),
            None => (
                None,
                &CustomModel {
                    model: String::new(),
                    id: None,
                    index: None,
                    display_name: None,
                    base_url: String::new(),
                    api_key: String::new(),
                    provider: Provider::Anthropic,
                    max_output_tokens: None,
                    no_image_support: None,
                    extra_args: None,
                    extra_headers: None,
                },
            ),
        };
        Draft {
            index,
            model_id: TextEdit::new(&model.model),
            display_name: TextEdit::new(model.display_name.as_deref().unwrap_or_default()),
            base_url: TextEdit::new(&model.base_url),
            api_key: TextEdit::new(&model.api_key),
            max_tokens: TextEdit::new(
                model
                    .max_output_tokens
                    .map(|v| v.to_string())
                    .unwrap_or_default()
                    .as_str(),
            ),
            provider: provider_index(&model.provider),
            no_image: model.no_image_support.unwrap_or(false),
            select_open: false,
            error: None,
        }
    }

    /// The model this draft describes, or why it can't be saved yet.
    fn build(&self, existing: Option<&CustomModel>) -> Result<CustomModel, String> {
        let model_id = self.model_id.text.trim().to_owned();
        if model_id.is_empty() {
            return Err(t!("gui.validation.required").to_owned());
        }
        let base_url = self.base_url.text.trim().to_owned();
        if base_url.is_empty() {
            return Err(t!("gui.validation.required").to_owned());
        }
        let display_name = self.display_name.text.trim().to_owned();
        let max_output_tokens = match self.max_tokens.text.trim() {
            "" => None,
            digits => Some(
                digits
                    .parse::<u32>()
                    .map_err(|_| t!("gui.validation.number").to_owned())?,
            ),
        };
        // Keep everything the editor doesn't expose (extra args/headers, index) as it was.
        let mut model = existing.cloned().unwrap_or(CustomModel {
            model: String::new(),
            id: None,
            index: None,
            display_name: None,
            base_url: String::new(),
            api_key: String::new(),
            provider: Provider::Anthropic,
            max_output_tokens: None,
            no_image_support: None,
            extra_args: None,
            extra_headers: None,
        });
        model.model = model_id;
        model.base_url = base_url;
        model.api_key = self.api_key.text.trim().to_owned();
        model.provider = PROVIDERS[self.provider].clone();
        model.display_name = if display_name.is_empty() {
            None
        } else {
            Some(display_name)
        };
        model.max_output_tokens = max_output_tokens;
        model.no_image_support = if self.no_image { Some(true) } else { None };
        Ok(model)
    }
}

// ---------------------------------------------------------------------------
// Actions
// ---------------------------------------------------------------------------

pub fn open_new(app: &mut App) {
    app.models_mut().dialog = Some(Draft::new(None));
}

pub fn open_edit(app: &mut App, index: usize) {
    let page = app.models_mut();
    if let Some(model) = page.models.get(index) {
        page.dialog = Some(Draft::new(Some((index, model))));
    }
}

pub fn close_dialog(app: &mut App) {
    app.models_mut().dialog = None;
}

pub fn save_draft(app: &mut App) {
    let page = app.models_mut();
    let Some(draft) = &page.dialog else { return };
    let existing = draft.index.and_then(|i| page.models.get(i));
    let model = match draft.build(existing) {
        Ok(model) => model,
        Err(error) => {
            if let Some(draft) = &mut page.dialog {
                draft.error = Some(error);
            }
            return;
        }
    };

    let mut models = page.models.clone();
    match draft.index {
        Some(index) => {
            let mut model = model;
            // A model whose first two fields changed reads as a new model to Factory, so it keeps
            // its slot but takes a fresh id, as the Tauri editor does.
            if let Some(previous) = models.get(index) {
                if previous.model != model.model || previous.base_url != model.base_url {
                    model.id = None;
                }
            }
            models[index] = model;
        }
        None => models.push(model),
    }

    match factory_settings::save_custom_models(models) {
        Ok(()) => {
            page.dialog = None;
            page.notice = Some("common.saved");
            page.reload();
        }
        Err(error) => {
            page.error = Some(error);
        }
    }
}

pub fn delete(app: &mut App, index: usize) {
    let page = app.models_mut();
    let mut models = page.models.clone();
    if index < models.len() {
        models.remove(index);
    }
    match factory_settings::save_custom_models(models) {
        Ok(()) => {
            page.pending_delete = None;
            page.notice = Some("common.deleted");
            page.reload();
        }
        Err(error) => page.error = Some(error),
    }
}

// ---------------------------------------------------------------------------
// View
// ---------------------------------------------------------------------------

pub fn view(page: &Models, k: &Kit, layer: &mut Vec<El>) -> El {
    if page.dialog.is_some() {
        dialog(page, k, layer);
    }
    if let Some(index) = page.pending_delete {
        confirm_delete(page, index, k, layer);
    }

    let actions = row().key("actions").gap(8.0).items_center().child(
        Button::new(t!("models.addModel"))
            .icon(&icons::PLUS)
            .on_press(|app: &mut App| open_new(app))
            .build(k),
    );

    let mut body = col().key("models").w_full().h_full().child(ui::page_header(
        k,
        t!("models.title"),
        &page.config_path,
        Some(actions),
    ));

    if let Some(error) = &page.error {
        body = body.child(ui::status_line(k, error, true));
    }
    if page.notice.is_some() {
        body = body.child(ui::status_line(k, t!("common.saved"), false));
    }

    let mut list = col().key("list").w_full().gap(6.0).px(20.0).pb(20.0);
    if page.models.is_empty() {
        list = list.child(
            col()
                .key("empty")
                .w_full()
                .padding(28.0)
                .gap(6.0)
                .items_center()
                .radius(k.radius.lg)
                .border(1.0, k.c.border_muted)
                .child(
                    text(t!("models.noModels"))
                        .key("title")
                        .font(k.ty.body_medium)
                        .color(k.c.text_base),
                )
                .child(
                    text(t!("models.noModelsHint"))
                        .key("hint")
                        .font(k.ty.text_xsmall)
                        .color(k.c.text_muted),
                ),
        );
    } else {
        for (index, model) in page.models.iter().enumerate() {
            list = list.child(model_row(k, index, model));
        }
    }

    body.child(
        el().key("scroll")
            .grow(1.0)
            .min_h(0.0)
            .w_full()
            .scroll(ScrollSpec {
                stick_bottom: false,
            })
            .child(list),
    )
}

fn model_row(k: &Kit, index: usize, model: &CustomModel) -> El {
    let name = model
        .display_name
        .clone()
        .unwrap_or_else(|| model.model.clone());
    row()
        .key(format!("model:{index}"))
        .w_full()
        .padding(10.0)
        .gap(12.0)
        .items_center()
        .radius(k.radius.md)
        .fill(k.c.bg_base)
        .border(1.0, k.c.border_muted)
        .hover(k.c.hover)
        .child(
            col()
                .key("text")
                .grow(1.0)
                .min_w(0.0)
                .gap(2.0)
                .child(
                    row()
                        .key("line1")
                        .gap(8.0)
                        .items_center()
                        .child(
                            text(&name)
                                .key("name")
                                .font(k.ty.ui_medium)
                                .color(k.c.text_base)
                                .truncate(),
                        )
                        .child(
                            Badge::new(provider_label(provider_index(&model.provider))).build(k),
                        ),
                )
                .child(
                    text(&model.model)
                        .key("model")
                        .font(k.ty.mono)
                        .color(k.c.text_muted)
                        .truncate(),
                )
                .child(
                    text(&model.base_url)
                        .key("url")
                        .font(k.ty.text_xsmall)
                        .color(k.c.text_faint)
                        .truncate(),
                ),
        )
        .child(
            IconButton::new(&icons::PENCIL)
                .variant(IconButtonVariant::Ghost)
                .label(t!("models.editModel"))
                .on_press(move |app: &mut App| open_edit(app, index))
                .build(k),
        )
        .child(
            IconButton::new(&icons::TRASH)
                .variant(IconButtonVariant::Ghost)
                .label(t!("models.deleteModel"))
                .on_press(move |app: &mut App| app.models_mut().pending_delete = Some(index))
                .build(k),
        )
}

fn dialog(page: &Models, k: &Kit, layer: &mut Vec<El>) {
    let Some(draft) = &page.dialog else { return };
    let title = if draft.index.is_some() {
        t!("models.editModel")
    } else {
        t!("models.addModel")
    };
    let mut form = col().key("form").w_full().px(16.0).pb(16.0).gap(12.0);

    form = form.child(field(
        k,
        t!("models.model"),
        TextInput::new(&draft.model_id)
            .placeholder("claude-sonnet-4-5")
            .width(None)
            .on_edit(|app: &mut App| &mut app.models_mut().dialog.as_mut().unwrap().model_id)
            .build(k),
    ));
    form = form.child(field(
        k,
        t!("models.displayName"),
        TextInput::new(&draft.display_name)
            .width(None)
            .on_edit(|app: &mut App| &mut app.models_mut().dialog.as_mut().unwrap().display_name)
            .build(k),
    ));
    form = form.child(field(
        k,
        t!("models.apiUrl"),
        TextInput::new(&draft.base_url)
            .placeholder("https://api.anthropic.com")
            .width(None)
            .on_edit(|app: &mut App| &mut app.models_mut().dialog.as_mut().unwrap().base_url)
            .build(k),
    ));
    form = form.child(field(
        k,
        t!("models.apiKey"),
        TextInput::new(&draft.api_key)
            .placeholder("sk-…")
            .width(None)
            .secure()
            .on_edit(|app: &mut App| &mut app.models_mut().dialog.as_mut().unwrap().api_key)
            .build(k),
    ));
    form = form.child(field(
        k,
        t!("models.maxTokens"),
        TextInput::new(&draft.max_tokens)
            .placeholder(t!("models.maxTokensPlaceholder"))
            .width(None)
            .on_edit(|app: &mut App| &mut app.models_mut().dialog.as_mut().unwrap().max_tokens)
            .build(k),
    ));

    let labels = [provider_label(0), provider_label(1), provider_label(2)];
    form = form.child(field(
        k,
        t!("models.provider"),
        Select::new("provider", &labels)
            .appearance(rune_kit::SelectAppearance::Field)
            .current(Some(provider_label(draft.provider)))
            .open(draft.select_open)
            .on_toggle(|app: &mut App| {
                let draft = app.models_mut().dialog.as_mut().unwrap();
                draft.select_open = !draft.select_open;
            })
            .on_close(|app: &mut App| {
                if let Some(draft) = &mut app.models_mut().dialog {
                    draft.select_open = false;
                }
            })
            .on_choose(|app: &mut App, value: &str, _| {
                if let Some(draft) = &mut app.models_mut().dialog {
                    draft.select_open = false;
                    draft.provider = (0..PROVIDERS.len())
                        .find(|i| provider_label(*i) == value)
                        .unwrap_or(draft.provider);
                }
            })
            .build(k, layer),
    ));

    form = form.child(
        row()
            .key("no-image")
            .w_full()
            .items_center()
            .child(
                Switch::new(draft.no_image)
                    .label(t!("models.noImageSupport"))
                    .on_toggle(|app: &mut App| {
                        let draft = app.models_mut().dialog.as_mut().unwrap();
                        draft.no_image = !draft.no_image;
                    })
                    .build(k),
            )
            .child(el().key("spacer").grow(1.0)),
    );

    if let Some(error) = &draft.error {
        form = form.child(
            text(error)
                .key("error")
                .w_full()
                .font(k.ty.text_xsmall)
                .color(k.c.danger.fg),
        );
    }

    let footer = row()
        .key("footer")
        .w_full()
        .pt(4.0)
        .gap(8.0)
        .justify_end()
        .child(
            Button::new(t!("common.cancel"))
                .variant(ButtonVariant::Ghost)
                .on_press(close_dialog)
                .build(k),
        )
        .child(
            Button::new(t!("common.save"))
                .variant(ButtonVariant::Contrast)
                .on_press(save_draft)
                .build(k),
        );

    Dialog::new("model-dialog", title)
        .width(520.0)
        .fit()
        .open(true)
        .on_close(close_dialog)
        .child(form.child(footer))
        .build(k, layer);
}

fn confirm_delete(page: &Models, index: usize, k: &Kit, layer: &mut Vec<El>) {
    let name = page
        .models
        .get(index)
        .map(|m| m.display_name.clone().unwrap_or_else(|| m.model.clone()))
        .unwrap_or_default();
    let footer = row()
        .key("footer")
        .w_full()
        .gap(8.0)
        .justify_end()
        .child(
            Button::new(t!("common.cancel"))
                .variant(ButtonVariant::Ghost)
                .on_press(|app: &mut App| app.models_mut().pending_delete = None)
                .build(k),
        )
        .child(
            Button::new(t!("common.delete"))
                .variant(ButtonVariant::Danger)
                .on_press(move |app: &mut App| delete(app, index))
                .build(k),
        );
    Dialog::new("model-delete", t!("models.deleteModel"))
        .description(&name)
        .width(420.0)
        .fit()
        .open(true)
        .on_close(|app: &mut App| app.models_mut().pending_delete = None)
        .child(footer)
        .build(k, layer);
}

/// A labelled control, using the kit's field so the label and control spacing match every other
/// form in the app.
fn field(k: &Kit, label: &str, control: El) -> El {
    let _ = Color::TRANSPARENT;
    Field::new().label(label).control(control).build(k)
}

#[cfg(test)]
mod tests {
    use super::*;

    fn draft(model: &str, base_url: &str) -> Draft {
        let mut draft = Draft::new(None);
        draft.model_id = TextEdit::new(model);
        draft.base_url = TextEdit::new(base_url);
        draft
    }

    #[test]
    fn a_draft_needs_a_model_and_a_base_url() {
        assert!(draft("", "https://x").build(None).is_err());
        assert!(draft("m", "").build(None).is_err());
        assert!(draft("m", "https://x").build(None).is_ok());
    }

    #[test]
    fn max_tokens_must_be_a_number() {
        let mut draft = draft("m", "https://x");
        draft.max_tokens = TextEdit::new("16384");
        assert_eq!(draft.build(None).unwrap().max_output_tokens, Some(16384));
        draft.max_tokens = TextEdit::new("lots");
        assert!(draft.build(None).is_err());
    }

    #[test]
    fn fields_the_editor_does_not_show_survive_an_edit() {
        let existing = CustomModel {
            model: "m".into(),
            id: Some("custom:m-0".into()),
            index: Some(0),
            display_name: None,
            base_url: "https://x".into(),
            api_key: "sk-1".into(),
            provider: Provider::Openai,
            max_output_tokens: None,
            no_image_support: None,
            extra_args: Some(std::collections::HashMap::from([(
                "temperature".to_owned(),
                serde_json::json!(0.2),
            )])),
            extra_headers: None,
        };
        let mut draft = Draft::new(Some((0, &existing)));
        draft.model_id = TextEdit::new("m2");
        let built = draft.build(Some(&existing)).unwrap();
        assert_eq!(built.model, "m2");
        assert!(built.extra_args.is_some());
        // The id stays here; `save_draft` clears it only when Factory would read the model as a
        // different one.
        assert_eq!(built.id.as_deref(), Some("custom:m-0"));
        assert_eq!(built.provider, Provider::Openai);
    }
}
