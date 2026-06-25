package cmd

import (
	"fmt"
	"io"
	"os"

	"github.com/HabibUllahKhanBarakzai/focci/internal/event"
	"github.com/HabibUllahKhanBarakzai/focci/internal/focus"
)

// debug logs to stderr only when FOCCI_DEBUG is set to a non-empty, non-"0"
// value. focci is purely observational; this output never affects the agent.
func debug(message string) {
	value, ok := os.LookupEnv("FOCCI_DEBUG")
	if ok && value != "" && value != "0" {
		fmt.Fprintf(os.Stderr, "[focci] %s\n", message)
	}
}

// readStdin reads stdin to a string, unless it's a terminal (manual invocation),
// in which case there's nothing piped and we must not block.
func readStdin() string {
	info, err := os.Stdin.Stat()
	if err == nil && info.Mode()&os.ModeCharDevice != 0 {
		return ""
	}
	data, _ := io.ReadAll(os.Stdin)
	return string(data)
}

// applyDecision carries an event decision through to a refocus, logging both the
// decision and its outcome under FOCCI_DEBUG.
func applyDecision(d event.Decision) {
	switch d.Kind {
	case event.Refocus:
		debug("refocus: " + d.Reason)
		outcome := focus.Refocus(false)
		debug("outcome: " + outcome.Describe())
	case event.Ignore:
		debug("ignore: " + d.Reason)
	}
}
