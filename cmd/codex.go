package cmd

import (
	"strings"

	"github.com/spf13/cobra"

	"github.com/HabibUllahKhanBarakzai/focci/internal/event"
)

var codexCmd = &cobra.Command{
	Use:                "codex [payload]",
	Short:              "Handle a Codex notify event (JSON from the last argument, else stdin)",
	DisableFlagParsing: true,
	Args:               cobra.ArbitraryArgs,
	Run: func(cmd *cobra.Command, args []string) {
		applyDecision(event.DecideCodex(codexPayload(args)))
	},
}

// codexPayload returns the first positional (non-flag) argument Codex appends,
// falling back to stdin when none is present.
func codexPayload(args []string) string {
	for _, arg := range args {
		if !strings.HasPrefix(arg, "-") {
			return arg
		}
	}
	return readStdin()
}
