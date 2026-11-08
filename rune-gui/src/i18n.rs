//! Translations: the build script turns `locales/{en,zh}.json` into a catalog, and `t!` resolves
//! keys in a `const`, so a key that doesn't exist fails the build instead of rendering blank.

include!(concat!(env!("OUT_DIR"), "/i18n.rs"));

/// The app-wide translator. `set_locale` switches every later lookup at once.
pub static I18N: rune_i18n::I18n = rune_i18n::I18n::new(&CATALOG);

/// The locale a fresh launch starts in: the OS's preferred languages, matched to the catalog,
/// unless saved preferences name one.
pub fn system_locale() -> usize {
    let languages = rune_i18n::system_languages();
    I18N.negotiate(languages.iter().map(String::as_str))
}

/// Runtime lookup for a key the view only knows as a value (a page's label, a tool's name).
/// Literal keys should use `t!`, which checks them at compile time instead.
pub fn tr(key: &'static str) -> &'static str {
    I18N.text(rune_i18n::Key::named(KEYS, key))
}

/// `t!("models.title")`, or `t!("models.addCount", "count" => n)` for a template.
#[macro_export]
macro_rules! t {
    ($key:literal) => {{
        const KEY: ::rune_i18n::Key = ::rune_i18n::Key::named($crate::i18n::KEYS, $key);
        $crate::i18n::I18N.text(KEY)
    }};
    ($key:literal, $($name:literal => $value:expr),+ $(,)?) => {{
        const KEY: ::rune_i18n::Key = ::rune_i18n::Key::named($crate::i18n::KEYS, $key);
        $crate::i18n::I18N.format(KEY, &[$(($name, &$value as &dyn ::core::fmt::Display)),+])
    }};
    ($key:expr) => {{
        $crate::i18n::tr($key)
    }};
}
