//! macOS: figure out which application hosts the agent session and bring it to
//! the front, with a per-target debounce so a burst of events (e.g. a `Stop`
//! plus a permission `Notification` from the same turn) collapses into a single
//! refocus.
//!
//! The host app is identified by its bundle id. macOS sets `__CFBundleIdentifier`
//! on every process launched from a GUI app, and the agent passes that
//! environment down to the hook process — so it works for JetBrains terminals
//! (PyCharm/WebStorm/GoLand), Warp, iTerm, Terminal, VS Code, etc. without any
//! per-terminal configuration.

use std::env;
use std::fs;
use std::path::PathBuf;
use std::process::Command;
use std::time::{SystemTime, UNIX_EPOCH};

const DEFAULT_DEBOUNCE_MS: u128 = 1500;

#[derive(Debug)]
pub struct BundleResolution {
    pub bundle_id: String,
    pub source: &'static str,
}

/// Resolve the bundle id of the app to focus, in priority order:
/// explicit override -> macOS-provided launcher bundle -> TERM_PROGRAM mapping.
pub fn resolve_bundle() -> Option<BundleResolution> {
    if let Some(explicit) = non_empty_env("AGENT_FOCUS_BUNDLE_ID") {
        return Some(BundleResolution {
            bundle_id: explicit,
            source: "AGENT_FOCUS_BUNDLE_ID",
        });
    }
    if let Some(bundle) = non_empty_env("__CFBundleIdentifier") {
        return Some(BundleResolution {
            bundle_id: bundle,
            source: "__CFBundleIdentifier",
        });
    }
    if let Some(term_program) = non_empty_env("TERM_PROGRAM") {
        if let Some(mapped) = bundle_for_term_program(&term_program) {
            return Some(BundleResolution {
                bundle_id: mapped.to_string(),
                source: "TERM_PROGRAM",
            });
        }
    }
    None
}

fn non_empty_env(key: &str) -> Option<String> {
    env::var(key).ok().filter(|value| !value.is_empty())
}

/// Fallback mapping for terminals that set `TERM_PROGRAM` but where
/// `__CFBundleIdentifier` may be missing (e.g. some shell configurations).
fn bundle_for_term_program(term_program: &str) -> Option<&'static str> {
    Some(match term_program {
        "Apple_Terminal" => "com.apple.Terminal",
        "iTerm.app" => "com.googlecode.iterm2",
        "WarpTerminal" => "dev.warp.Warp-Stable",
        "vscode" => "com.microsoft.VSCode",
        "Hyper" => "co.zeit.hyper",
        "WezTerm" => "com.github.wez.wezterm",
        "ghostty" | "Ghostty" => "com.mitchellh.ghostty",
        "Tabby" => "org.tabby",
        "rio" => "com.raphamorim.rio",
        "kitty" => "net.kovidgoyal.kitty",
        "Alacritty" => "org.alacritty",
        _ => return None,
    })
}

pub fn debounce_ms() -> u128 {
    non_empty_env("AGENT_FOCUS_DEBOUNCE_MS")
        .and_then(|value| value.parse().ok())
        .unwrap_or(DEFAULT_DEBOUNCE_MS)
}

fn now_ms() -> u128 {
    SystemTime::now()
        .duration_since(UNIX_EPOCH)
        .map(|elapsed| elapsed.as_millis())
        .unwrap_or(0)
}

/// The debounce is keyed by target app, so distinct apps never suppress each
/// other and bursts at the same app collapse to one refocus.
fn stamp_path(bundle_id: &str) -> PathBuf {
    let tmp = non_empty_env("TMPDIR").unwrap_or_else(|| "/tmp".to_string());
    let sanitized: String = bundle_id
        .chars()
        .map(|ch| if ch.is_ascii_alphanumeric() { ch } else { '_' })
        .collect();
    let mut path = PathBuf::from(tmp);
    path.push(format!("agent-focus-{sanitized}.stamp"));
    path
}

fn recently_focused(bundle_id: &str, window_ms: u128) -> bool {
    let Ok(contents) = fs::read_to_string(stamp_path(bundle_id)) else {
        return false;
    };
    let Ok(last) = contents.trim().parse::<u128>() else {
        return false;
    };
    now_ms().saturating_sub(last) < window_ms
}

fn record_focus(bundle_id: &str) {
    let _ = fs::write(stamp_path(bundle_id), now_ms().to_string());
}

#[derive(Debug)]
pub enum FocusOutcome {
    Activated(String),
    Debounced(String),
    NoTarget,
    Failed(String),
}

/// Bring the host app to the front. Pass `force` to bypass the debounce window
/// (used by the manual `focus` subcommand).
pub fn refocus(force: bool) -> FocusOutcome {
    let Some(resolution) = resolve_bundle() else {
        return FocusOutcome::NoTarget;
    };
    let bundle_id = resolution.bundle_id;

    if !force && recently_focused(&bundle_id, debounce_ms()) {
        return FocusOutcome::Debounced(bundle_id);
    }

    match activate(&bundle_id) {
        Ok(true) => {
            record_focus(&bundle_id);
            FocusOutcome::Activated(bundle_id)
        }
        Ok(false) => FocusOutcome::Failed(bundle_id),
        Err(err) => FocusOutcome::Failed(format!("{bundle_id}: {err}")),
    }
}

/// Activate the app by bundle id. `open -b` is the primary path; if it fails
/// (rare), fall back to an AppleScript activate.
fn activate(bundle_id: &str) -> std::io::Result<bool> {
    let opened = Command::new("/usr/bin/open")
        .arg("-b")
        .arg(bundle_id)
        .status()?;
    if opened.success() {
        return Ok(true);
    }
    let script = format!(
        "tell application id \"{}\" to activate",
        bundle_id.replace('\\', "\\\\").replace('"', "\\\"")
    );
    let scripted = Command::new("/usr/bin/osascript")
        .arg("-e")
        .arg(&script)
        .status()?;
    Ok(scripted.success())
}
