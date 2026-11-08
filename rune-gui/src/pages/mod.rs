//! Page routing. Each page owns its state and renders itself; `Page` is what the current
//! sidebar entry loaded.

pub mod form;
pub mod models;
pub mod specs;

use rune::{col, el, text, El, ScrollSpec};
use rune_kit::Kit;

use crate::app::App;
use crate::t;
use crate::ui;

pub enum Page {
    Models(Box<models::Models>),
    Specs(specs::Specs),
    Form(Box<form::FormPage>),
    /// A page the spike hasn't ported yet: it names the phase that will.
    Soon,
    Blank,
}

pub fn view(s: &App, k: &Kit, layer: &mut Vec<El>) -> El {
    match &s.page {
        Page::Models(page) => models::view(page, k, layer),
        Page::Specs(page) => specs::view(page, k),
        Page::Form(page) => form::view(page, k, layer),
        Page::Soon => soon(s, k),
        Page::Blank => col(),
    }
}

fn soon(s: &App, k: &Kit) -> El {
    col()
        .key("soon")
        .w_full()
        .h_full()
        .child(ui::page_header(
            k,
            t!(s.id.label_key()),
            t!(s.view.label_key()),
            None,
        ))
        .child(
            el().key("scroll")
                .grow(1.0)
                .min_h(0.0)
                .w_full()
                .scroll(ScrollSpec {
                    stick_bottom: false,
                })
                .child(
                    col().key("body").px(24.0).pb(24.0).gap(10.0).child(
                        col()
                            .key("card")
                            .w_full()
                            .padding(22.0)
                            .gap(8.0)
                            .radius(k.radius.lg)
                            .fill(k.c.bg_base)
                            .border(1.0, k.c.border_muted)
                            .child(
                                text(t!("gui.soon.title"))
                                    .key("title")
                                    .font(k.ty.body_medium)
                                    .color(k.c.text_base),
                            )
                            .child(
                                text(t!("gui.soon.body"))
                                    .key("body")
                                    .font(k.ty.text_xsmall)
                                    .color(k.c.text_muted),
                            ),
                    ),
                ),
        )
}
