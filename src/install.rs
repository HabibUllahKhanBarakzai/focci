//! Wire focci into the agents' own configuration files, idempotently.
//!
//! - Claude Code: register `Stop` and `Notification` command hooks in
//!   `~/.claude/settings.json`.
//! - Codex: set `notify = ["<binary>", "codex"]` in `~/.codex/config.toml`.
//!
//! Existing files are preserved: we only add/replace our own entries, back up
//! the file before writing, and never clobber an unrelated Codex `notify`
//! unless `--force` is given.

use std::fs;
use std::io::{Error, ErrorKind};
use std::path::{Path, PathBuf};

use serde_json::{json, Map, Value};
use toml_edit::{Array, DocumentMut};

/// Marker substring used to recognize hook/notify entries that belong to us.
const MARKER: &str = "focci";

pub struct InstallReport {
    pub path: PathBuf,
    pub changed: bool,
    pub note: Option<String>,
}

impl InstallReport {
    pub fn describe_install(&self, agent: &str) -> String {
        let state = if self.changed {
            "configured"
        } else {
            "already configured"
        };
        self.format(agent, state)
    }

    pub fn describe_uninstall(&self, agent: &str) -> String {
        let state = if self.changed {
            "removed"
        } else {
            "not present"
        };
        self.format(agent, state)
    }

    fn format(&self, agent: &str, state: &str) -> String {
        let mut out = format!("{agent}: {state} ({})", self.path.display());
        if let Some(note) = &self.note {
            out.push_str(&format!("\n  note: {note}"));
        }
        out
    }
}

fn home() -> PathBuf {
    std::env::var_os("HOME")
        .map(PathBuf::from)
        .unwrap_or_else(|| PathBuf::from("."))
}

pub fn claude_settings_path() -> PathBuf {
    home().join(".claude").join("settings.json")
}

pub fn codex_config_path() -> PathBuf {
    home().join(".codex").join("config.toml")
}

/// The command string written into configs: the absolute path of this binary so
/// the hook resolves regardless of the GUI process's PATH.
pub fn binary_command() -> String {
    std::env::current_exe()
        .and_then(|path| path.canonicalize())
        .map(|path| path.to_string_lossy().into_owned())
        .unwrap_or_else(|_| MARKER.to_string())
}

fn io_error(message: impl Into<String>) -> Error {
    Error::new(ErrorKind::InvalidData, message.into())
}

fn backup(path: &Path) -> std::io::Result<()> {
    if path.exists() {
        let backup_path = path.with_file_name(format!(
            "{}.bak",
            path.file_name()
                .and_then(|name| name.to_str())
                .unwrap_or("config")
        ));
        fs::copy(path, backup_path)?;
    }
    Ok(())
}

fn ensure_parent(path: &Path) -> std::io::Result<()> {
    if let Some(parent) = path.parent() {
        fs::create_dir_all(parent)?;
    }
    Ok(())
}

// --- Claude Code -----------------------------------------------------------

fn read_json_object(path: &Path) -> std::io::Result<Map<String, Value>> {
    match fs::read_to_string(path) {
        Ok(text) if !text.trim().is_empty() => match serde_json::from_str(&text) {
            Ok(Value::Object(map)) => Ok(map),
            Ok(_) => Err(io_error(format!("{} is not a JSON object", path.display()))),
            Err(err) => Err(io_error(format!(
                "could not parse {}: {err}",
                path.display()
            ))),
        },
        Ok(_) => Ok(Map::new()),
        Err(err) if err.kind() == ErrorKind::NotFound => Ok(Map::new()),
        Err(err) => Err(err),
    }
}

fn write_json_object(path: &Path, root: &Map<String, Value>) -> std::io::Result<()> {
    ensure_parent(path)?;
    let mut text = serde_json::to_string_pretty(&Value::Object(root.clone()))?;
    text.push('\n');
    fs::write(path, text)
}

/// Does this hook entry already belong to us, for the given `--event` marker?
fn hook_is_ours(entry: &Value, event_marker: &str) -> bool {
    entry
        .get("command")
        .and_then(Value::as_str)
        .map(|command| command.contains(MARKER) && command.contains(event_marker))
        .unwrap_or(false)
}

/// Ensure `hooks[event]` contains a command hook running `command`. Returns
/// true if the document changed.
fn ensure_command_group(
    hooks: &mut Map<String, Value>,
    event: &str,
    matcher: Option<&str>,
    command: &str,
    event_marker: &str,
) -> bool {
    let groups = hooks
        .entry(event)
        .or_insert_with(|| Value::Array(Vec::new()));
    if !groups.is_array() {
        *groups = Value::Array(Vec::new());
    }
    let groups = groups
        .as_array_mut()
        .expect("groups coerced to array above");

    // If one of our hooks is already registered, update its command if needed.
    for group in groups.iter_mut() {
        let Some(entries) = group.get_mut("hooks").and_then(Value::as_array_mut) else {
            continue;
        };
        for entry in entries.iter_mut() {
            if hook_is_ours(entry, event_marker) {
                if entry.get("command").and_then(Value::as_str) == Some(command) {
                    return false;
                }
                entry["command"] = Value::String(command.to_string());
                return true;
            }
        }
    }

    // Otherwise append a fresh group.
    let mut group = Map::new();
    if let Some(matcher) = matcher {
        group.insert("matcher".to_string(), Value::String(matcher.to_string()));
    }
    group.insert(
        "hooks".to_string(),
        json!([{ "type": "command", "command": command }]),
    );
    groups.push(Value::Object(group));
    true
}

/// Remove our hooks from `hooks[event]`, pruning emptied groups and the event
/// key itself. Returns true if anything was removed.
fn remove_command_group(hooks: &mut Map<String, Value>, event: &str, event_marker: &str) -> bool {
    let Some(groups) = hooks.get_mut(event).and_then(Value::as_array_mut) else {
        return false;
    };
    let mut changed = false;
    for group in groups.iter_mut() {
        if let Some(entries) = group.get_mut("hooks").and_then(Value::as_array_mut) {
            let before = entries.len();
            entries.retain(|entry| !hook_is_ours(entry, event_marker));
            changed |= entries.len() != before;
        }
    }
    groups.retain(|group| {
        group
            .get("hooks")
            .and_then(Value::as_array)
            .map(|entries| !entries.is_empty())
            .unwrap_or(true)
    });
    let now_empty = groups.is_empty();
    if now_empty {
        hooks.remove(event);
    }
    changed
}

pub fn install_claude(command_override: Option<&str>) -> std::io::Result<InstallReport> {
    let path = claude_settings_path();
    let mut root = read_json_object(&path)?;
    let command = command_override
        .map(str::to_string)
        .unwrap_or_else(binary_command);

    let stop_command = format!("{command} claude --event stop");
    let notification_command = format!("{command} claude --event notification");

    let hooks_value = root
        .entry("hooks")
        .or_insert_with(|| Value::Object(Map::new()));
    let hooks = hooks_value
        .as_object_mut()
        .ok_or_else(|| io_error("\"hooks\" in settings.json is not an object"))?;

    let stop_changed = ensure_command_group(hooks, "Stop", None, &stop_command, "--event stop");
    let notif_changed = ensure_command_group(
        hooks,
        "Notification",
        Some(""),
        &notification_command,
        "--event notification",
    );

    let changed = stop_changed || notif_changed;
    if changed {
        backup(&path)?;
        write_json_object(&path, &root)?;
    }
    Ok(InstallReport {
        path,
        changed,
        note: None,
    })
}

pub fn uninstall_claude() -> std::io::Result<InstallReport> {
    let path = claude_settings_path();
    let mut root = read_json_object(&path)?;

    let changed = if let Some(hooks) = root.get_mut("hooks").and_then(Value::as_object_mut) {
        let stop_changed = remove_command_group(hooks, "Stop", "--event stop");
        let notif_changed = remove_command_group(hooks, "Notification", "--event notification");
        if hooks.is_empty() {
            root.remove("hooks");
        }
        stop_changed || notif_changed
    } else {
        false
    };

    if changed {
        backup(&path)?;
        write_json_object(&path, &root)?;
    }
    Ok(InstallReport {
        path,
        changed,
        note: None,
    })
}

/// (stop_hook_present, notification_hook_present) for `doctor`.
pub fn claude_status(path: &Path) -> (bool, bool) {
    let Ok(root) = read_json_object(path) else {
        return (false, false);
    };
    let Some(hooks) = root.get("hooks").and_then(Value::as_object) else {
        return (false, false);
    };
    (
        event_has_our_hook(hooks, "Stop", "--event stop"),
        event_has_our_hook(hooks, "Notification", "--event notification"),
    )
}

fn event_has_our_hook(hooks: &Map<String, Value>, event: &str, event_marker: &str) -> bool {
    hooks
        .get(event)
        .and_then(Value::as_array)
        .map(|groups| {
            groups.iter().any(|group| {
                group
                    .get("hooks")
                    .and_then(Value::as_array)
                    .map(|entries| {
                        entries
                            .iter()
                            .any(|entry| hook_is_ours(entry, event_marker))
                    })
                    .unwrap_or(false)
            })
        })
        .unwrap_or(false)
}

// --- Codex -----------------------------------------------------------------

fn read_toml_document(path: &Path) -> std::io::Result<DocumentMut> {
    match fs::read_to_string(path) {
        Ok(text) => text
            .parse::<DocumentMut>()
            .map_err(|err| io_error(format!("could not parse {}: {err}", path.display()))),
        Err(err) if err.kind() == ErrorKind::NotFound => Ok(DocumentMut::new()),
        Err(err) => Err(err),
    }
}

fn notify_references_us(document: &DocumentMut) -> bool {
    document
        .get("notify")
        .and_then(|item| item.as_array())
        .map(|array| {
            array
                .iter()
                .filter_map(|value| value.as_str())
                .any(|value| value.contains(MARKER))
        })
        .unwrap_or(false)
}

pub fn install_codex(
    command_override: Option<&str>,
    force: bool,
) -> std::io::Result<InstallReport> {
    let path = codex_config_path();
    let mut document = read_toml_document(&path)?;
    let command = command_override
        .map(str::to_string)
        .unwrap_or_else(binary_command);

    let has_notify = document.get("notify").is_some();
    if has_notify && !notify_references_us(&document) && !force {
        return Ok(InstallReport {
            path,
            changed: false,
            note: Some(
                "an unrelated `notify` is already set; left untouched. Re-run with --force to \
                 overwrite, or chain focci from your existing notifier."
                    .to_string(),
            ),
        });
    }

    let mut array = Array::new();
    array.push(command.as_str());
    array.push("codex");
    array.fmt();
    document["notify"] = toml_edit::value(array);

    backup(&path)?;
    ensure_parent(&path)?;
    fs::write(&path, document.to_string())?;
    Ok(InstallReport {
        path,
        changed: true,
        note: None,
    })
}

pub fn uninstall_codex() -> std::io::Result<InstallReport> {
    let path = codex_config_path();
    let mut document = read_toml_document(&path)?;

    let changed = if notify_references_us(&document) {
        document.remove("notify");
        true
    } else {
        false
    };

    if changed {
        backup(&path)?;
        fs::write(&path, document.to_string())?;
    }
    Ok(InstallReport {
        path,
        changed,
        note: None,
    })
}

pub fn codex_status(path: &Path) -> bool {
    read_toml_document(path)
        .map(|document| notify_references_us(&document))
        .unwrap_or(false)
}
