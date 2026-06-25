package cmd

import (
	"fmt"
	"os"

	"github.com/spf13/cobra"

	"github.com/HabibUllahKhanBarakzai/focci/internal/focus"
)

var focusForce bool

var focusCmd = &cobra.Command{
	Use:   "focus",
	Short: "Bring the host terminal/editor to the front now (manual trigger / test)",
	Run: func(cmd *cobra.Command, args []string) {
		switch outcome := focus.Refocus(focusForce); outcome.Kind {
		case focus.Activated:
			fmt.Printf("Focused %s\n", outcome.Detail)
		case focus.Debounced:
			fmt.Printf("Skipped (debounced) %s — use --force to override\n", outcome.Detail)
		case focus.NoTarget:
			exitf(1, "No host app detected (no __CFBundleIdentifier or known TERM_PROGRAM). "+
				"Set FOCCI_BUNDLE_ID to your terminal's bundle id.\n")
		case focus.Failed:
			exitf(1, "Failed to focus %s\n", outcome.Detail)
		}
		os.Exit(0)
	},
}

func init() {
	focusCmd.Flags().BoolVar(&focusForce, "force", false, "ignore the debounce window")
}
