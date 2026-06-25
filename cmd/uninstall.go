package cmd

import (
	"fmt"
	"os"

	"github.com/spf13/cobra"

	"github.com/HabibUllahKhanBarakzai/focci/internal/config"
)

var uninstallAgent string

var uninstallCmd = &cobra.Command{
	Use:   "uninstall",
	Short: "Remove focci from your agents' configuration files",
	Run: func(cmd *cobra.Command, args []string) {
		target, err := parseAgentFlag(uninstallAgent)
		if err != nil {
			exitf(2, "%v\n", err)
		}

		failed := false
		if target.claude {
			if report, err := config.UninstallClaude(); err != nil {
				fmt.Fprintf(os.Stderr, "Claude Code uninstall failed: %v\n", err)
				failed = true
			} else {
				fmt.Println(report.DescribeUninstall("Claude Code"))
			}
		}
		if target.codex {
			if report, err := config.UninstallCodex(); err != nil {
				fmt.Fprintf(os.Stderr, "Codex uninstall failed: %v\n", err)
				failed = true
			} else {
				fmt.Println(report.DescribeUninstall("Codex"))
			}
		}
		if failed {
			os.Exit(1)
		}
		os.Exit(0)
	},
}

func init() {
	uninstallCmd.Flags().StringVar(&uninstallAgent, "agent", "all", "which agent(s) to clean up: claude|codex|all")
}
