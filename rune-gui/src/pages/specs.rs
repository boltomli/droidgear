//! Specs: the Markdown files under Factory's `specs` folder, with the selected one rendered by
//! rune-kit's Markdown view. This is the spike's "lists + rich text" page.

use droidgear_core::specs::{self, SpecFile};
use rune::{col, el, icon, row, text, El, ScrollSpec};
use rune_kit::{Kit, Markdown};

use crate::app::App;
use crate::icons;
use crate::t;
use crate::ui;

pub struct Specs {
    pub files: Vec<SpecFile>,
    pub selected: Option<usize>,
    pub error: Option<String>,
}

impl Specs {
    pub fn load() -> Specs {
        match specs::list_specs() {
            Ok(files) => {
                let selected = if files.is_empty() { None } else { Some(0) };
                Specs {
                    files,
                    selected,
                    error: None,
                }
            }
            Err(error) => Specs {
                files: Vec::new(),
                selected: None,
                error: Some(error),
            },
        }
    }
}

pub fn select(app: &mut App, index: usize) {
    app.specs_mut().selected = Some(index);
}

pub fn reload(app: &mut App) {
    let previous = app.specs().and_then(|s| s.selected);
    let mut fresh = Specs::load();
    fresh.selected = previous.filter(|i| *i < fresh.files.len());
    let page = app.specs_mut();
    page.files = fresh.files;
    page.selected = fresh.selected;
    page.error = fresh.error;
}

pub fn view(page: &Specs, k: &Kit) -> El {
    let mut list = col().key("files").w_full().gap(2.0).padding(6.0);
    if page.files.is_empty() {
        list = list.child(
            text(t!("droid.specs.noSpecs"))
                .key("empty")
                .padding(12.0)
                .font(k.ty.text_xsmall)
                .color(k.c.text_muted),
        );
    }
    for (index, file) in page.files.iter().enumerate() {
        let selected = page.selected == Some(index);
        list = list.child(
            row()
                .key(format!("spec:{index}"))
                .w_full()
                .h(28.0)
                .px(8.0)
                .gap(8.0)
                .items_center()
                .radius(k.radius.sm)
                .role("button")
                .label(&file.name)
                .focusable(true)
                .fill(if selected {
                    k.c.layer_02
                } else {
                    rune::Color::TRANSPARENT
                })
                .hover(if selected {
                    rune::Color::TRANSPARENT
                } else {
                    k.c.hover
                })
                .on_press(move |app: &mut App| select(app, index))
                .child(
                    icon(&icons::FILE, 13.0)
                        .key("icon")
                        .color(k.c.icon_muted)
                        .a11y_hidden(),
                )
                .child(
                    text(&file.name)
                        .key("name")
                        .font(k.ty.ui)
                        .color(if selected {
                            k.c.text_base
                        } else {
                            k.c.text_muted
                        })
                        .truncate()
                        .a11y_hidden(),
                ),
        );
    }

    let header = ui::page_header(
        k,
        t!("droid.specs.title"),
        &format!("{} {}", page.files.len(), t!("gui.specs.count")),
        Some(
            row().key("actions").gap(8.0).items_center().child(
                rune_kit::Button::new(t!("common.refresh"))
                    .variant(rune_kit::ButtonVariant::Ghost)
                    .on_press(reload)
                    .build(k),
            ),
        ),
    );

    let detail = match page.selected.and_then(|i| page.files.get(i)) {
        Some(file) => col()
            .key("detail")
            .w_full()
            .px(24.0)
            .pb(24.0)
            .gap(10.0)
            .child(
                text(&file.name)
                    .key("filename")
                    .font(k.ty.ui_medium)
                    .color(k.c.text_base),
            )
            .child(Markdown::new(&file.content).selectable(true).build(k)),
        None => col().key("detail").grow(1.0).items_center().child(
            text(t!("droid.specs.noSpecs"))
                .key("empty")
                .font(k.ty.ui)
                .color(k.c.text_muted),
        ),
    };

    let mut body = col().key("specs").w_full().h_full().child(header);
    if let Some(error) = &page.error {
        body = body.child(ui::status_line(k, error, true));
    }
    body.child(
        row()
            .key("split")
            .grow(1.0)
            .min_h(0.0)
            .w_full()
            .child(
                el().key("list-scroll")
                    .w(240.0)
                    .h_full()
                    .scroll(ScrollSpec {
                        stick_bottom: false,
                    })
                    .child(list),
            )
            .child(
                el().key("detail-scroll")
                    .grow(1.0)
                    .min_w(0.0)
                    .h_full()
                    .scroll(ScrollSpec {
                        stick_bottom: false,
                    })
                    .child(detail),
            ),
    )
}
