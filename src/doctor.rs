//! `focci doctor` — print detected configuration and integration status,
//! so the user can see what would be focused and whether the hooks are wired.

use std::env;

use crate::{focus, install};

fn or_unset(key: &str) -> String {
    env::var(key).unwrap_or_else(|_| "<unset>".to_string())
}

fn yes_no(present: bool) -> &'static str {
    if present {
        "yes"
    } else {
        "no"
    }
}

pub fn run() {
    println!("focci {}", env!("CARGO_PKG_VERSION"));
    println!();

    println!("Host app detection:");
    match focus::resolve_bundle() {
        Some(resolution) => {
            println!(
                "  bundle id            : {} (via {})",
                resolution.bundle_id, resolution.source
            )
        }
        None => println!("  bundle id            : <none> — set FOCCI_BUNDLE_ID to override"),
    }
    println!(
        "  __CFBundleIdentifier : {}",
        or_unset("__CFBundleIdentifier")
    );
    println!("  TERM_PROGRAM         : {}", or_unset("TERM_PROGRAM"));
    println!("  debounce             : {} ms", focus::debounce_ms());
    println!("  binary               : {}", install::binary_command());
    println!();

    let claude_path = install::claude_settings_path();
    let (stop, notification) = install::claude_status(&claude_path);
    println!("Claude Code ({}):", claude_path.display());
    println!("  Stop hook            : {}", yes_no(stop));
    println!("  Notification hook    : {}", yes_no(notification));
    println!();

    let codex_path = install::codex_config_path();
    println!("Codex ({}):", codex_path.display());
    println!(
        "  notify wired         : {}",
        yes_no(install::codex_status(&codex_path))
    );
    println!();

    println!("Tip: switch to another window, then run `focci focus` to test.");
}
