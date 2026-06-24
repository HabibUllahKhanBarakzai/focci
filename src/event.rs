//! Decide whether an incoming agent event should refocus the user's terminal.
//!
//! The whole point of focci is to bring the user back *when it matters*
//! (a turn finished, or the agent is blocked waiting for an approval) and to
//! stay out of the way otherwise — in particular to ignore the repeating
//! "waiting for your input" idle reminders that would otherwise yank focus
//! back every time they re-fire.

use serde::Deserialize;

/// Which Claude Code hook event we are handling, when stdin does not carry it.
#[derive(Debug, Clone, Copy, PartialEq, Eq)]
pub enum ClaudeEvent {
    Stop,
    Notification,
}

/// The outcome of evaluating an event. The string is a human-readable reason,
/// surfaced only under `FOCCI_DEBUG` — it never affects the agent.
#[derive(Debug)]
pub enum Decision {
    Refocus(String),
    Ignore(String),
}

#[derive(Debug, Default, Deserialize)]
struct ClaudePayload {
    #[serde(default)]
    hook_event_name: Option<String>,
    #[serde(default)]
    message: Option<String>,
}

/// True for the repeating idle / "waiting for your input" reminders. These must
/// be ignored so the user is not pulled back to the terminal every time the
/// reminder re-fires after they have already returned and moved on.
fn is_idle_message(message: &str) -> bool {
    let lowered = message.to_lowercase();
    lowered.contains("waiting for your input") || lowered.contains("waiting for your next prompt")
}

/// True for permission / approval prompts — the agent is blocked on the user,
/// so we do want to surface the terminal.
fn is_permission_message(message: &str) -> bool {
    let lowered = message.to_lowercase();
    lowered.contains("needs your permission") || lowered.contains("needs you to approve")
}

/// Decide for a Claude Code hook. `hint` comes from the `--event` flag and is
/// only used when the stdin payload lacks `hook_event_name`.
pub fn decide_claude(hint: Option<ClaudeEvent>, stdin_json: &str) -> Decision {
    let payload: ClaudePayload = serde_json::from_str(stdin_json.trim()).unwrap_or_default();

    let event = match payload.hook_event_name.as_deref() {
        Some(name) if name.eq_ignore_ascii_case("stop") => Some(ClaudeEvent::Stop),
        Some(name) if name.eq_ignore_ascii_case("notification") => Some(ClaudeEvent::Notification),
        _ => hint,
    };

    match event {
        Some(ClaudeEvent::Stop) => Decision::Refocus("claude:stop (turn finished)".to_string()),
        Some(ClaudeEvent::Notification) => match payload.message.as_deref() {
            Some(message) if is_idle_message(message) => {
                Decision::Ignore(format!("claude:notification idle reminder: {message:?}"))
            }
            Some(message) if is_permission_message(message) => Decision::Refocus(format!(
                "claude:notification permission request: {message:?}"
            )),
            Some(message) => Decision::Refocus(format!(
                "claude:notification (unrecognized message, defaulting to refocus): {message:?}"
            )),
            None => Decision::Refocus("claude:notification (no message)".to_string()),
        },
        None => Decision::Ignore("claude: could not determine event".to_string()),
    }
}

#[derive(Debug, Default, Deserialize)]
struct CodexPayload {
    #[serde(rename = "type")]
    event_type: Option<String>,
}

/// Decide for a Codex `notify` payload. Codex only emits `agent-turn-complete`
/// to `notify`; we refocus on that and ignore everything else (forward-compat).
pub fn decide_codex(payload_json: &str) -> Decision {
    let payload: CodexPayload = serde_json::from_str(payload_json.trim()).unwrap_or_default();
    match payload.event_type.as_deref() {
        Some("agent-turn-complete") => Decision::Refocus("codex:agent-turn-complete".to_string()),
        Some(other) => Decision::Ignore(format!("codex: ignored event type {other:?}")),
        None => Decision::Ignore("codex: missing event type".to_string()),
    }
}

#[cfg(test)]
mod tests {
    use super::*;

    fn is_refocus(decision: &Decision) -> bool {
        matches!(decision, Decision::Refocus(_))
    }

    #[test]
    fn stop_event_refocuses() {
        let json = r#"{"hook_event_name":"Stop","stop_hook_active":false}"#;
        assert!(is_refocus(&decide_claude(None, json)));
    }

    #[test]
    fn idle_notification_is_ignored() {
        let json =
            r#"{"hook_event_name":"Notification","message":"Claude is waiting for your input"}"#;
        assert!(!is_refocus(&decide_claude(None, json)));
    }

    #[test]
    fn done_and_waiting_notification_is_ignored() {
        let json = r#"{"hook_event_name":"Notification","message":"Claude is done and waiting for your next prompt"}"#;
        assert!(!is_refocus(&decide_claude(None, json)));
    }

    #[test]
    fn permission_notification_refocuses() {
        let json = r#"{"hook_event_name":"Notification","message":"Claude needs your permission to use Bash"}"#;
        assert!(is_refocus(&decide_claude(None, json)));
    }

    #[test]
    fn unknown_notification_defaults_to_refocus() {
        let json = r#"{"hook_event_name":"Notification","message":"Something new happened"}"#;
        assert!(is_refocus(&decide_claude(None, json)));
    }

    #[test]
    fn falls_back_to_event_hint_when_stdin_empty() {
        assert!(is_refocus(&decide_claude(Some(ClaudeEvent::Stop), "")));
        assert!(!is_refocus(&decide_claude(None, "")));
    }

    #[test]
    fn hint_loses_to_explicit_payload_event() {
        // Payload says Notification+idle; hint says Stop. Payload wins -> ignore.
        let json =
            r#"{"hook_event_name":"Notification","message":"Claude is waiting for your input"}"#;
        assert!(!is_refocus(&decide_claude(Some(ClaudeEvent::Stop), json)));
    }

    #[test]
    fn codex_turn_complete_refocuses() {
        let json = r#"{"type":"agent-turn-complete","turn-id":"t1"}"#;
        assert!(is_refocus(&decide_codex(json)));
    }

    #[test]
    fn codex_other_event_ignored() {
        let json = r#"{"type":"session-start"}"#;
        assert!(!is_refocus(&decide_codex(json)));
    }

    #[test]
    fn garbage_input_does_not_panic() {
        assert!(!is_refocus(&decide_claude(None, "not json")));
        assert!(!is_refocus(&decide_codex("not json")));
    }
}
