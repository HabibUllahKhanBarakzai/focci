package cmd

import (
	"strings"

	"github.com/spf13/cobra"

	"github.com/HabibUllahKhanBarakzai/focci/internal/event"
)

var claudeCmd = &cobra.Command{
	Use:   "claude",
	Short: "Handle a Claude Code hook event (reads hook JSON on stdin)",
	// Flag parsing is disabled so the hook can never fail an agent's turn on an
	// unexpected argument; we parse the optional --event hint leniently instead.
	DisableFlagParsing: true,
	Args:               cobra.ArbitraryArgs,
	Run: func(cmd *cobra.Command, args []string) {
		applyDecision(event.DecideClaude(parseClaudeEventFlag(args), readStdin()))
	},
}

// parseClaudeEventFlag extracts an optional --event hint. Unknown or missing
// values yield nil rather than an error, so the hook degrades gracefully.
func parseClaudeEventFlag(args []string) *event.ClaudeEvent {
	for i := 0; i < len(args); i++ {
		var value string
		switch {
		case args[i] == "--event" && i+1 < len(args):
			value = args[i+1]
			i++
		case strings.HasPrefix(args[i], "--event="):
			value = strings.TrimPrefix(args[i], "--event=")
		default:
			continue
		}
		switch strings.ToLower(value) {
		case "stop":
			e := event.Stop
			return &e
		case "notification":
			e := event.Notification
			return &e
		}
	}
	return nil
}
