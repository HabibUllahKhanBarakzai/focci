// Package focus figures out which application hosts the agent session and brings
// it to the front (macOS), with a per-target debounce so a burst of events
// (e.g. a Stop plus a permission Notification from the same turn) collapses into
// a single refocus.
//
// The host app is identified by its bundle id. macOS sets __CFBundleIdentifier
// on every process launched from a GUI app, and the agent passes that
// environment down to the hook process — so it works for JetBrains terminals
// (PyCharm/WebStorm/GoLand), Warp, iTerm, Terminal, VS Code, etc. without any
// per-terminal configuration.
package focus

import (
	"errors"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"strconv"
	"strings"
	"time"
)

const defaultDebounceMS int64 = 1500

// BundleResolution is the app to focus plus where its id came from.
type BundleResolution struct {
	BundleID string
	Source   string
}

func nonEmptyEnv(key string) (string, bool) {
	value := os.Getenv(key)
	if value == "" {
		return "", false
	}
	return value, true
}

// ResolveBundle resolves the bundle id of the app to focus, in priority order:
// explicit override -> macOS-provided launcher bundle -> TERM_PROGRAM mapping.
func ResolveBundle() (BundleResolution, bool) {
	if explicit, ok := nonEmptyEnv("FOCCI_BUNDLE_ID"); ok {
		return BundleResolution{BundleID: explicit, Source: "FOCCI_BUNDLE_ID"}, true
	}
	if bundle, ok := nonEmptyEnv("__CFBundleIdentifier"); ok {
		return BundleResolution{BundleID: bundle, Source: "__CFBundleIdentifier"}, true
	}
	if termProgram, ok := nonEmptyEnv("TERM_PROGRAM"); ok {
		if mapped, ok := bundleForTermProgram(termProgram); ok {
			return BundleResolution{BundleID: mapped, Source: "TERM_PROGRAM"}, true
		}
	}
	return BundleResolution{}, false
}

// bundleForTermProgram is a fallback mapping for terminals that set TERM_PROGRAM
// but where __CFBundleIdentifier may be missing (e.g. some shell configurations).
func bundleForTermProgram(termProgram string) (string, bool) {
	switch termProgram {
	case "Apple_Terminal":
		return "com.apple.Terminal", true
	case "iTerm.app":
		return "com.googlecode.iterm2", true
	case "WarpTerminal":
		return "dev.warp.Warp-Stable", true
	case "vscode":
		return "com.microsoft.VSCode", true
	case "Hyper":
		return "co.zeit.hyper", true
	case "WezTerm":
		return "com.github.wez.wezterm", true
	case "ghostty", "Ghostty":
		return "com.mitchellh.ghostty", true
	case "Tabby":
		return "org.tabby", true
	case "rio":
		return "com.raphamorim.rio", true
	case "kitty":
		return "net.kovidgoyal.kitty", true
	case "Alacritty":
		return "org.alacritty", true
	default:
		return "", false
	}
}

// DebounceMS is the debounce window in milliseconds (FOCCI_DEBOUNCE_MS, default
// 1500). Invalid or negative values fall back to the default.
func DebounceMS() int64 {
	if value, ok := nonEmptyEnv("FOCCI_DEBOUNCE_MS"); ok {
		if parsed, err := strconv.ParseUint(value, 10, 64); err == nil {
			return int64(parsed)
		}
	}
	return defaultDebounceMS
}

func nowMS() int64 {
	return time.Now().UnixMilli()
}

// stampPath is keyed by target app, so distinct apps never suppress each other
// and bursts at the same app collapse to one refocus.
func stampPath(bundleID string) string {
	tmp, ok := nonEmptyEnv("TMPDIR")
	if !ok {
		tmp = "/tmp"
	}
	var sanitized strings.Builder
	for _, ch := range bundleID {
		if (ch >= 'a' && ch <= 'z') || (ch >= 'A' && ch <= 'Z') || (ch >= '0' && ch <= '9') {
			sanitized.WriteRune(ch)
		} else {
			sanitized.WriteByte('_')
		}
	}
	return filepath.Join(tmp, fmt.Sprintf("focci-%s.stamp", sanitized.String()))
}

func recentlyFocused(bundleID string, windowMS int64) bool {
	contents, err := os.ReadFile(stampPath(bundleID))
	if err != nil {
		return false
	}
	last, err := strconv.ParseInt(strings.TrimSpace(string(contents)), 10, 64)
	if err != nil {
		return false
	}
	elapsed := nowMS() - last
	if elapsed < 0 { // a future stamp means "not recent enough"
		elapsed = 0
	}
	return elapsed < windowMS
}

func recordFocus(bundleID string) {
	_ = os.WriteFile(stampPath(bundleID), []byte(strconv.FormatInt(nowMS(), 10)), 0o644)
}

// OutcomeKind is the result of attempting a refocus.
type OutcomeKind int

const (
	Activated OutcomeKind = iota
	Debounced
	NoTarget
	Failed
)

// Outcome is the result of Refocus; Detail holds the bundle id or failure info.
type Outcome struct {
	Kind   OutcomeKind
	Detail string
}

func (o Outcome) Describe() string {
	switch o.Kind {
	case Activated:
		return fmt.Sprintf("Activated(%q)", o.Detail)
	case Debounced:
		return fmt.Sprintf("Debounced(%q)", o.Detail)
	case NoTarget:
		return "NoTarget"
	case Failed:
		return fmt.Sprintf("Failed(%q)", o.Detail)
	}
	return ""
}

// Refocus brings the host app to the front. Pass force to bypass the debounce
// window (used by the manual focus subcommand).
func Refocus(force bool) Outcome {
	resolution, ok := ResolveBundle()
	if !ok {
		return Outcome{Kind: NoTarget}
	}
	bundleID := resolution.BundleID

	if !force && recentlyFocused(bundleID, DebounceMS()) {
		return Outcome{Kind: Debounced, Detail: bundleID}
	}

	activated, err := activate(bundleID)
	if err != nil {
		return Outcome{Kind: Failed, Detail: fmt.Sprintf("%s: %v", bundleID, err)}
	}
	if activated {
		recordFocus(bundleID)
		return Outcome{Kind: Activated, Detail: bundleID}
	}
	return Outcome{Kind: Failed, Detail: bundleID}
}

// activate activates the app by bundle id. `open -b` is the primary path; if it
// fails (rare), fall back to an AppleScript activate. An error is returned only
// when a helper process cannot be spawned at all — a non-zero exit just means
// "not activated" and triggers the fallback.
func activate(bundleID string) (bool, error) {
	activated, err := runStatus("/usr/bin/open", "-b", bundleID)
	if err != nil {
		return false, err
	}
	if activated {
		return true, nil
	}

	escaped := strings.ReplaceAll(bundleID, `\`, `\\`)
	escaped = strings.ReplaceAll(escaped, `"`, `\"`)
	script := fmt.Sprintf(`tell application id "%s" to activate`, escaped)
	return runStatus("/usr/bin/osascript", "-e", script)
}

// runStatus runs a command and reports whether it exited successfully. A non-zero
// exit is reported as (false, nil); only a spawn failure returns a non-nil error.
func runStatus(name string, args ...string) (bool, error) {
	err := exec.Command(name, args...).Run()
	if err == nil {
		return true, nil
	}
	var exitErr *exec.ExitError
	if errors.As(err, &exitErr) {
		return false, nil
	}
	return false, err
}
