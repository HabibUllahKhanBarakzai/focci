package cmd

import (
	"fmt"
	"os"

	"github.com/spf13/cobra"

	"github.com/HabibUllahKhanBarakzai/focci/internal/config"
	"github.com/HabibUllahKhanBarakzai/focci/internal/focus"
)

var doctorCmd = &cobra.Command{
	Use:   "doctor",
	Short: "Print detected configuration and integration status",
	Run: func(cmd *cobra.Command, args []string) {
		runDoctor()
	},
}

func orUnset(key string) string {
	if value, ok := os.LookupEnv(key); ok {
		return value
	}
	return "<unset>"
}

func yesNo(present bool) string {
	if present {
		return "yes"
	}
	return "no"
}

func runDoctor() {
	fmt.Printf("focci %s\n", version)
	fmt.Println()

	fmt.Println("Host app detection:")
	if resolution, ok := focus.ResolveBundle(); ok {
		fmt.Printf("  bundle id            : %s (via %s)\n", resolution.BundleID, resolution.Source)
	} else {
		fmt.Println("  bundle id            : <none> — set FOCCI_BUNDLE_ID to override")
	}
	fmt.Printf("  __CFBundleIdentifier : %s\n", orUnset("__CFBundleIdentifier"))
	fmt.Printf("  TERM_PROGRAM         : %s\n", orUnset("TERM_PROGRAM"))
	fmt.Printf("  debounce             : %d ms\n", focus.DebounceMS())
	fmt.Printf("  binary               : %s\n", config.BinaryCommand())
	fmt.Println()

	claudePath := config.ClaudeSettingsPath()
	stop, notification := config.ClaudeStatus(claudePath)
	fmt.Printf("Claude Code (%s):\n", claudePath)
	fmt.Printf("  Stop hook            : %s\n", yesNo(stop))
	fmt.Printf("  Notification hook    : %s\n", yesNo(notification))
	fmt.Println()

	codexPath := config.CodexConfigPath()
	fmt.Printf("Codex (%s):\n", codexPath)
	fmt.Printf("  notify wired         : %s\n", yesNo(config.CodexStatus(codexPath)))
	fmt.Println()

	fmt.Println("Tip: switch to another window, then run `focci focus` to test.")
}
