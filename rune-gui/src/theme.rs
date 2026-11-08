//! DroidGear's theme, ported from `src/theme-variables.css`.
//!
//! That file is a shadcn token set in OKLCH, so the values copy across unchanged: `--background`,
//! `--foreground`, `--primary` and friends become the seed Rune's kit derives its semantic tokens
//! from, and the deliberate overrides below keep the rest on DroidGear's palette (borders, muted
//! text, the destructive colour, the 10 px radius and 13/14 px type scale).

use rune::{oklch, oklcha, Color, Shadow, TextStyle};
use rune_kit::{Kit, PaletteFocus, PaletteSeed, Radius, StateColors};

/// The shadcn tokens, per appearance. The whole set is mirrored here even where the kit derives a
/// value of its own, so the file stays a readable counterpart of `theme-variables.css`.
#[allow(dead_code)]
pub struct Tokens {
    pub bg: Color,
    pub fg: Color,
    pub card: Color,
    pub popover: Color,
    pub primary: Color,
    pub secondary: Color,
    pub muted: Color,
    pub muted_fg: Color,
    pub accent: Color,
    pub destructive: Color,
    pub border: Color,
    pub input: Color,
    pub ring: Color,
    pub sidebar: Color,
    pub sidebar_border: Color,
}

pub fn tokens(dark: bool) -> Tokens {
    if dark {
        Tokens {
            bg: oklch(0.145, 0.0, 0.0),
            fg: oklch(0.985, 0.0, 0.0),
            card: oklch(0.205, 0.0, 0.0),
            popover: oklch(0.205, 0.0, 0.0),
            primary: oklch(0.87, 0.0, 0.0),
            secondary: oklch(0.269, 0.0, 0.0),
            muted: oklch(0.269, 0.0, 0.0),
            muted_fg: oklch(0.708, 0.0, 0.0),
            accent: oklch(0.371, 0.0, 0.0),
            destructive: oklch(0.704, 0.191, 22.216),
            border: oklcha(1.0, 0.0, 0.0, 0.10),
            input: oklcha(1.0, 0.0, 0.0, 0.15),
            ring: oklch(0.556, 0.0, 0.0),
            sidebar: oklch(0.205, 0.0, 0.0),
            sidebar_border: oklcha(1.0, 0.0, 0.0, 0.10),
        }
    } else {
        Tokens {
            bg: oklch(1.0, 0.0, 0.0),
            fg: oklch(0.145, 0.0, 0.0),
            card: oklch(1.0, 0.0, 0.0),
            popover: oklch(1.0, 0.0, 0.0),
            primary: oklch(0.205, 0.0, 0.0),
            secondary: oklch(0.97, 0.0, 0.0),
            muted: oklch(0.97, 0.0, 0.0),
            muted_fg: oklch(0.556, 0.0, 0.0),
            accent: oklch(0.97, 0.0, 0.0),
            destructive: oklch(0.58, 0.22, 27.0),
            border: oklch(0.922, 0.0, 0.0),
            input: oklch(0.922, 0.0, 0.0),
            ring: oklch(0.708, 0.0, 0.0),
            sidebar: oklch(0.985, 0.0, 0.0),
            sidebar_border: oklch(0.922, 0.0, 0.0),
        }
    }
}

/// Rune-kit's token schema, filled from DroidGear's palette.
pub fn kit(dark: bool) -> Kit {
    let t = tokens(dark);
    let mut k = Kit::from_palette_focus(
        PaletteSeed {
            bg: t.bg,
            fg: t.fg,
            // shadcn's switch, primary button and focus rings all read `--primary`.
            accent: t.primary,
        },
        PaletteFocus::Ink,
    );

    // Tailwind's radius scale around `--radius: 0.625rem`.
    k.radius = Radius {
        xs: 4.0,
        sm: 6.0,
        md: 8.0,
        lg: 10.0,
        xl: 14.0,
    };

    k.c.page = t.bg;
    k.c.bg_deep = t.bg;
    k.c.bg_base = t.card;
    k.c.layer_01 = t.sidebar;
    k.c.layer_02 = t.muted;
    k.c.layer_03 = t.muted;
    k.c.layer_04 = t.accent;
    k.c.surface_base = t.muted;
    k.c.surface_weak = t.muted;
    k.c.surface_raised = t.card;
    k.c.surface_raised_base = t.card;
    k.c.surface_float = t.popover;
    k.c.text_base = t.fg;
    k.c.text_muted = t.muted_fg;
    k.c.text_faint = t.muted_fg.alpha(0.75);
    k.c.border_base = t.border;
    k.c.border_muted = t.border;
    k.c.border_weak = t.border;
    k.c.border_strong = t.input;
    k.c.border_focus = t.ring;
    k.c.hover = t.fg.alpha(if dark { 0.07 } else { 0.05 });
    k.c.pressed = t.fg.alpha(if dark { 0.12 } else { 0.09 });
    // shadcn dims behind a dialog with `bg-black/50`; a dark page needs more of it to read.
    k.c.scrim = Color::BLACK.alpha(if dark { 0.55 } else { 0.35 });
    k.c.danger = StateColors {
        bg: t.destructive.alpha(0.14),
        fg: t.destructive,
        border: t.destructive.alpha(0.55),
        icon: t.destructive,
    };

    // shadcn draws inputs with a `--input` border and no fill of their own; rune-kit's input is a
    // filled box, so give it a border and the hairline shadow that lifts it off the card.
    let elevation = std::mem::take(&mut k.elevation).with_input(vec![
        Shadow {
            y: 1.0,
            blur: 2.0,
            color: Color::BLACK.alpha(if dark { 0.28 } else { 0.06 }),
            ..Shadow::default()
        },
        Shadow {
            spread: 0.5,
            color: t.input,
            ..Shadow::default()
        },
    ]);
    k.elevation = elevation;

    // shadcn's scale: text-sm (14/20) for controls, text-xs (12/16) for captions.
    let ui = TextStyle::new(13.0, 400.0).lh(18.0);
    k.ty = rune_kit::Type {
        ui,
        ui_medium: TextStyle {
            weight: 500.0,
            ..ui
        },
        ui_compact: ui.lh(16.0),
        small: TextStyle::new(11.0, 400.0).lh(14.0),
        small_medium: TextStyle::new(11.0, 500.0).lh(14.0),
        body: TextStyle::new(14.0, 400.0).lh(20.0),
        body_medium: TextStyle::new(14.0, 500.0).lh(20.0),
        text_small: TextStyle::new(13.0, 400.0).lh(18.0),
        text_small_medium: TextStyle::new(13.0, 500.0).lh(18.0),
        text_xsmall: TextStyle::new(12.0, 400.0).lh(16.0),
        text_xsmall_medium: TextStyle::new(12.0, 500.0).lh(16.0),
        mono: TextStyle::new(12.0, 400.0).mono().lh(18.0),
    };
    k
}
