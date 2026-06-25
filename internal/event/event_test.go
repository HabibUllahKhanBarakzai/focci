package event

import "testing"

func isRefocus(d Decision) bool { return d.Kind == Refocus }

func TestStopEventRefocuses(t *testing.T) {
	json := `{"hook_event_name":"Stop","stop_hook_active":false}`
	if !isRefocus(DecideClaude(nil, json)) {
		t.Fatal("expected Stop to refocus")
	}
}

func TestIdleNotificationIsIgnored(t *testing.T) {
	json := `{"hook_event_name":"Notification","message":"Claude is waiting for your input"}`
	if isRefocus(DecideClaude(nil, json)) {
		t.Fatal("idle reminder should be ignored")
	}
}

func TestDoneAndWaitingNotificationIsIgnored(t *testing.T) {
	json := `{"hook_event_name":"Notification","message":"Claude is done and waiting for your next prompt"}`
	if isRefocus(DecideClaude(nil, json)) {
		t.Fatal("done-and-waiting reminder should be ignored")
	}
}

func TestPermissionNotificationRefocuses(t *testing.T) {
	json := `{"hook_event_name":"Notification","message":"Claude needs your permission to use Bash"}`
	if !isRefocus(DecideClaude(nil, json)) {
		t.Fatal("permission request should refocus")
	}
}

func TestUnknownNotificationDefaultsToRefocus(t *testing.T) {
	json := `{"hook_event_name":"Notification","message":"Something new happened"}`
	if !isRefocus(DecideClaude(nil, json)) {
		t.Fatal("unrecognized notification should default to refocus")
	}
}

func TestFallsBackToEventHintWhenStdinEmpty(t *testing.T) {
	stop := Stop
	if !isRefocus(DecideClaude(&stop, "")) {
		t.Fatal("stop hint with empty stdin should refocus")
	}
	if isRefocus(DecideClaude(nil, "")) {
		t.Fatal("no hint and empty stdin should be ignored")
	}
}

func TestHintLosesToExplicitPayloadEvent(t *testing.T) {
	// Payload says Notification+idle; hint says Stop. Payload wins -> ignore.
	stop := Stop
	json := `{"hook_event_name":"Notification","message":"Claude is waiting for your input"}`
	if isRefocus(DecideClaude(&stop, json)) {
		t.Fatal("explicit payload event should win over hint")
	}
}

func TestNotificationWithNoMessageRefocuses(t *testing.T) {
	json := `{"hook_event_name":"Notification"}`
	if !isRefocus(DecideClaude(nil, json)) {
		t.Fatal("notification with no message should refocus")
	}
}

func TestCodexTurnCompleteRefocuses(t *testing.T) {
	json := `{"type":"agent-turn-complete","turn-id":"t1"}`
	if !isRefocus(DecideCodex(json)) {
		t.Fatal("agent-turn-complete should refocus")
	}
}

func TestCodexOtherEventIgnored(t *testing.T) {
	json := `{"type":"session-start"}`
	if isRefocus(DecideCodex(json)) {
		t.Fatal("non-turn-complete codex event should be ignored")
	}
}

func TestGarbageInputDoesNotPanic(t *testing.T) {
	if isRefocus(DecideClaude(nil, "not json")) {
		t.Fatal("garbage claude input should be ignored")
	}
	if isRefocus(DecideCodex("not json")) {
		t.Fatal("garbage codex input should be ignored")
	}
}
