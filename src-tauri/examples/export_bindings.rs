//! Exports the tauri-specta TypeScript bindings to src/lib/bindings.ts.
//! Usage: cargo run -p droidgear --example export_bindings
fn main() {
    eprintln!("step1: process started");
    droidgear_lib::bindings::export_ts_bindings();
    eprintln!("step2: export done");
}
