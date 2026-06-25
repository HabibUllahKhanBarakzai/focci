// Package event decides whether an incoming agent event should refocus the
// user's terminal.
//
// The whole point of focci is to bring the user back *when it matters* (a turn
// finished, or the agent is blocked waiting for an approval) and to stay out of
// the way otherwise — in particular to ignore the repeating "waiting for your
// input" idle reminders that would otherwise yank focus back every time they
// re-fire.
package event

import (
	"encoding/json"
	"fmt"
	"strings"
)

// ClaudeEvent is which Claude Code hook event we are handling, when stdin does
// not carry it.
type ClaudeEvent int

const (
	Stop ClaudeEvent = iota
	Notification
)

// Kind is the outcome of evaluating an event.
type Kind int

const (
	Refocus Kind = iota
	Ignore
)

// Decision carries the verdict plus a human-readable reason, surfaced only under
// FOCCI_DEBUG — it never affects the agent.
type Decision struct {
	Kind   Kind
	Reason string
}

func refocus(reason string) Decision { return Decision{Refocus, reason} }
func ignore(reason string) Decision  { return Decision{Ignore, reason} }

type claudePayload struct {
	HookEventName *string `json:"hook_event_name"`
	Message       *string `json:"message"`
}

// isIdleMessage is true for the repeating idle / "waiting for your input"
// reminders. These must be ignored so the user is not pulled back to the
// terminal every time the reminder re-fires after they have already returned and
// moved on.
func isIdleMessage(message string) bool {
	lowered := strings.ToLower(message)
	return strings.Contains(lowered, "waiting for your input") ||
		strings.Contains(lowered, "waiting for your next prompt")
}

// isPermissionMessage is true for permission / approval prompts — the agent is
// blocked on the user, so we do want to surface the terminal.
func isPermissionMessage(message string) bool {
	lowered := strings.ToLower(message)
	return strings.Contains(lowered, "needs your permission") ||
		strings.Contains(lowered, "needs you to approve")
}

// DecideClaude decides for a Claude Code hook. hint comes from the --event flag
// and is only used when the stdin payload lacks hook_event_name.
func DecideClaude(hint *ClaudeEvent, stdinJSON string) Decision {
	var payload claudePayload
	// A parse failure leaves payload zero-valued; we fall back to the hint.
	_ = json.Unmarshal([]byte(strings.TrimSpace(stdinJSON)), &payload)

	event := hint
	if payload.HookEventName != nil {
		switch {
		case strings.EqualFold(*payload.HookEventName, "stop"):
			e := Stop
			event = &e
		case strings.EqualFold(*payload.HookEventName, "notification"):
			e := Notification
			event = &e
		}
	}

	if event == nil {
		return ignore("claude: could not determine event")
	}

	switch *event {
	case Stop:
		return refocus("claude:stop (turn finished)")
	case Notification:
		if payload.Message == nil {
			return refocus("claude:notification (no message)")
		}
		message := *payload.Message
		switch {
		case isIdleMessage(message):
			return ignore(fmt.Sprintf("claude:notification idle reminder: %q", message))
		case isPermissionMessage(message):
			return refocus(fmt.Sprintf("claude:notification permission request: %q", message))
		default:
			return refocus(fmt.Sprintf(
				"claude:notification (unrecognized message, defaulting to refocus): %q", message))
		}
	}
	return ignore("claude: could not determine event")
}

type codexPayload struct {
	Type *string `json:"type"`
}

// DecideCodex decides for a Codex notify payload. Codex only emits
// agent-turn-complete to notify; we refocus on that and ignore everything else
// (forward-compat).
func DecideCodex(payloadJSON string) Decision {
	var payload codexPayload
	_ = json.Unmarshal([]byte(strings.TrimSpace(payloadJSON)), &payload)
	if payload.Type == nil {
		return ignore("codex: missing event type")
	}
	switch *payload.Type {
	case "agent-turn-complete":
		return refocus("codex:agent-turn-complete")
	default:
		return ignore(fmt.Sprintf("codex: ignored event type %q", *payload.Type))
	}
}
