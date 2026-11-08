//! Generates the message catalog from the app's own `locales/*.json`.
//!
//! `locales/en.json` and `locales/zh.json` are already flat (`"models.title": "…"`), which is the
//! layout `rune_i18n::codegen` wants, so the build only has to lay them out as
//! `<layer>/<locale>.json` beside a `locales.json` manifest.
//!
//! The generated `i18n.rs` is `include!`d by `src/i18n.rs`: it holds the sorted key list (so a
//! mistyped key is a build error, not a blank label), one text blob per locale, and the `Catalog`.

use std::path::PathBuf;
use std::{env, fs};

/// The locales the GUI ships, source locale first. `tag` is what the OS's preferred languages are
/// matched against; `label` is what a language picker shows.
const LOCALES: &[(&str, &str, &str)] = &[("en", "en", "English"), ("zh", "zh", "简体中文")];

fn main() {
    let manifest = PathBuf::from(env::var("CARGO_MANIFEST_DIR").unwrap());
    let out = PathBuf::from(env::var("OUT_DIR").unwrap());
    let locales_dir = manifest.join("../locales");
    let catalogs = out.join("catalogs");

    let app_dir = catalogs.join("app");
    fs::create_dir_all(&app_dir).expect("create catalog dir");

    let mut manifest_json = String::from("{\"layers\":[\"app\"],\"locales\":[");
    for (i, (id, tag, label)) in LOCALES.iter().enumerate() {
        let source = locales_dir.join(format!("{id}.json"));
        println!("cargo:rerun-if-changed={}", source.display());
        let text = fs::read_to_string(&source)
            .unwrap_or_else(|e| panic!("read {}: {e}", source.display()));
        // Round-trip so a malformed catalog fails here, with the file's name, not in the app.
        let value: serde_json::Value = serde_json::from_str(&text)
            .unwrap_or_else(|e| panic!("parse {}: {e}", source.display()));
        if !value.is_object() {
            panic!("{} must be a flat JSON object", source.display());
        }
        fs::write(app_dir.join(format!("{id}.json")), text).expect("write catalog");

        if i > 0 {
            manifest_json.push(',');
        }
        manifest_json.push_str(&format!(
            "{{\"id\":\"{id}\",\"tag\":\"{tag}\",\"label\":\"{label}\"}}"
        ));
    }
    manifest_json.push_str("]}");
    fs::write(catalogs.join("locales.json"), manifest_json).expect("write manifest");

    rune_i18n::codegen::generate(&catalogs, &out).expect("generate i18n catalog");
    println!("cargo:rerun-if-changed=src");

    // One version for both GUIs: the app's own, not this crate's placeholder.
    let package = manifest.join("../package.json");
    println!("cargo:rerun-if-changed={}", package.display());
    let text = fs::read_to_string(&package).expect("read ../package.json");
    let value: serde_json::Value = serde_json::from_str(&text).expect("parse ../package.json");
    let version = value["version"].as_str().expect("package.json version");
    println!("cargo:rustc-env=DROIDGEAR_VERSION={version}");
}
