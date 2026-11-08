//! The window chrome: a sidebar of tools and their pages, and the page header/frame the pages
//! fill. Everything here is layout plus the kit's tokens; pages build their own bodies.

use rune::{col, el, icon, row, text, Align, Color, Cx, El, Icon, ScrollSpec};
use rune_kit::{Badge, Kit};

use crate::app::{App, PageId, View};
use crate::icons;
use crate::t;

const SIDEBAR: f32 = 236.0;

/// Every tool, in the order the Tauri build lists them.
const TOOLS: &[View] = &[
    View::Droid,
    View::OpenCode,
    View::Codex,
    View::Claude,
    View::OpenClaw,
    View::Hermes,
    View::Pi,
    View::Omp,
    View::Copilot,
    View::Dsh,
    View::Channels,
];

pub fn shell(s: &App, k: &Kit, cx: &mut Cx<App>, layer: &mut Vec<El>) -> El {
    let hair = 1.0 / cx.env().scale;
    row()
        .key("shell")
        .w_full()
        .h_full()
        .fill(k.c.page)
        .child(sidebar(s, k, hair))
        .child(el().key("gutter").w(hair).h_full().fill(k.c.border_muted))
        .child(page_body(s, k, hair, layer))
}

fn sidebar(s: &App, k: &Kit, hair: f32) -> El {
    let mut items = col().key("nav").w_full().gap(1.0).pb(8.0);
    for view in TOOLS {
        let view = *view;
        let active = s.view == view;
        items = items.child(tool_row(k, view, active));
        if active {
            let mut pages = col().key("pages").w_full().gap(1.0).pb(4.0);
            for id in view.pages() {
                let id = *id;
                pages = pages.child(page_row(s, k, view, id, format!("page:{}", id.label_key())));
            }
            items = items.child(pages);
        }
    }

    col()
        .key("sidebar")
        .w(SIDEBAR)
        .h_full()
        .fill(k.c.layer_01)
        .child(
            row()
                .key("brand")
                .h(46.0)
                .px(14.0)
                .gap(8.0)
                .items_center()
                .child(
                    text(t!("app.name"))
                        .key("name")
                        .font(k.ty.body_medium)
                        .color(k.c.text_base),
                )
                .child(Badge::new(crate::APP_VERSION).build(k)),
        )
        .child(
            el().key("nav-scroll")
                .grow(1.0)
                .min_h(0.0)
                .w_full()
                .px(8.0)
                .scroll(ScrollSpec {
                    stick_bottom: false,
                })
                .child(items),
        )
        .child(el().key("sep").h(hair).w_full().fill(k.c.border_muted))
        .child(footer(s, k))
}

fn tool_row(k: &Kit, view: View, active: bool) -> El {
    nav_row(
        NavRow {
            key: format!("tool:{}", view.label_key()),
            label: t!(view.label_key()),
            icon: Some(view.icon()),
            selected: active,
            bold: true,
            indent: 0.0,
        },
        k,
        move |app: &mut App| app.open(view, PageId::Settings),
    )
}

fn page_row(s: &App, k: &Kit, view: View, id: PageId, key: String) -> El {
    nav_row(
        NavRow {
            key,
            label: t!(id.label_key()),
            icon: None,
            selected: s.id == id,
            bold: false,
            indent: 20.0,
        },
        k,
        move |app: &mut App| app.open(view, id),
    )
}

struct NavRow {
    key: String,
    label: &'static str,
    icon: Option<&'static Icon>,
    selected: bool,
    bold: bool,
    indent: f32,
}

fn nav_row(nav: NavRow, k: &Kit, on_press: impl Fn(&mut App) + 'static) -> El {
    let NavRow {
        key,
        label,
        icon: nav_icon,
        selected,
        bold,
        indent,
    } = nav;
    let ink = if selected {
        k.c.text_base
    } else {
        k.c.text_muted
    };
    let mut e = row()
        .key(key)
        .h(30.0)
        .ml(indent)
        .px(8.0)
        .gap(8.0)
        .items_center()
        .radius(k.radius.md)
        .role("button")
        .label(label)
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
        .on_press(on_press);
    if let Some(nav_icon) = nav_icon {
        e = e.child(icon(nav_icon, 15.0).key("icon").color(ink).a11y_hidden());
    }
    e.child(
        text(label)
            .key("label")
            .font(if bold { k.ty.ui_medium } else { k.ty.ui })
            .color(ink)
            .truncate()
            .a11y_hidden(),
    )
}

fn footer(s: &App, k: &Kit) -> El {
    let locale = crate::i18n::I18N.locales()[s.locale].label();
    row()
        .key("footer")
        .h(42.0)
        .px(10.0)
        .gap(6.0)
        .items_center()
        .child(pill_button(
            "appearance",
            &icons::SUN,
            t!(s.appearance.label_key()),
            k,
            |app: &mut App| app.cycle_appearance(),
        ))
        .child(el().key("spacer").grow(1.0))
        .child(pill_button(
            "language",
            &icons::GLOBE,
            locale,
            k,
            |app: &mut App| app.cycle_locale(),
        ))
}

fn pill_button(
    key: &str,
    pill_icon: &'static Icon,
    label: &str,
    k: &Kit,
    on_press: impl Fn(&mut App) + 'static,
) -> El {
    row()
        .key(key)
        .h(26.0)
        .px(8.0)
        .gap(6.0)
        .items_center()
        .radius(k.radius.md)
        .role("button")
        .label(label)
        .focusable(true)
        .hover(k.c.hover)
        .on_press(on_press)
        .child(
            icon(pill_icon, 14.0)
                .key("icon")
                .color(k.c.icon_muted)
                .a11y_hidden(),
        )
        .child(
            text(label)
                .key("label")
                .font(k.ty.small)
                .color(k.c.text_muted)
                .a11y_hidden(),
        )
}

fn page_body(s: &App, k: &Kit, hair: f32, layer: &mut Vec<El>) -> El {
    col()
        .key("main")
        .grow(1.0)
        .min_w(0.0)
        .h_full()
        .child(
            row()
                .key("header")
                .h(46.0)
                .px(20.0)
                .gap(8.0)
                .items_center()
                .child(
                    text(t!(s.view.label_key()))
                        .key("tool")
                        .font(k.ty.ui)
                        .color(k.c.text_muted),
                )
                .child(text("/").key("slash").font(k.ty.ui).color(k.c.text_faint))
                .child(
                    text(t!(s.id.label_key()))
                        .key("page")
                        .font(k.ty.ui_medium)
                        .color(k.c.text_base),
                ),
        )
        .child(el().key("rule").h(hair).w_full().fill(k.c.border_muted))
        .child(
            col()
                .key("content")
                .grow(1.0)
                .min_h(0.0)
                .w_full()
                .child(crate::pages::view(s, k, layer)),
        )
}

// ---------------------------------------------------------------------------
// Building blocks the pages share.
// ---------------------------------------------------------------------------

/// A page title with an optional right-hand row of actions.
pub fn page_header(k: &Kit, title: &str, subtitle: &str, actions: Option<El>) -> El {
    let mut head = row()
        .key("page-header")
        .w_full()
        .px(24.0)
        .pt(20.0)
        .pb(12.0)
        .gap(12.0)
        .items_center()
        .child(
            col()
                .key("titles")
                .gap(2.0)
                .grow(1.0)
                .min_w(0.0)
                .child(
                    text(title)
                        .key("title")
                        .font(rune::TextStyle::new(17.0, 600.0).lh(24.0))
                        .color(k.c.text_base),
                )
                .child(
                    text(subtitle)
                        .key("subtitle")
                        .font(k.ty.text_xsmall)
                        .color(k.c.text_muted)
                        .truncate(),
                ),
        );
    if let Some(actions) = actions {
        head = head.child(actions);
    }
    head
}

/// A raised surface: the card a settings section or a list lives in.
#[allow(dead_code)]
pub fn card(k: &Kit, gap: f32) -> El {
    col()
        .key("card")
        .w_full()
        .padding(6.0)
        .gap(gap)
        .radius(k.radius.lg)
        .fill(k.c.bg_base)
        .border(1.0, k.c.border_muted)
}

/// A section label above a card, as the Tauri pages show above each group.
#[allow(dead_code)]
pub fn section_label(k: &Kit, label: &str) -> El {
    text(label)
        .key("section")
        .w_full()
        .px(4.0)
        .pb(4.0)
        .font(k.ty.small_medium)
        .color(k.c.text_muted)
}

/// A one-line status message: the page's error, or its "saved" confirmation.
pub fn status_line(k: &Kit, message: &str, is_error: bool) -> El {
    text(message)
        .key("status")
        .w_full()
        .px(4.0)
        .font(k.ty.text_xsmall)
        .color(if is_error {
            k.c.danger.fg
        } else {
            k.c.text_muted
        })
}

/// Right-aligned text for a row's trailing slot.
#[allow(dead_code)]
pub fn trailing(k: &Kit, value: &str) -> El {
    text(value)
        .key("trailing")
        .font(k.ty.text_xsmall)
        .color(k.c.text_muted)
        .align(Align::End)
}

/// A muted monospace line (paths, ids, URLs).
#[allow(dead_code)]
pub fn mono(k: &Kit, value: &str) -> El {
    text(value)
        .key("mono")
        .font(k.ty.mono)
        .color(k.c.text_muted)
        .truncate()
}
