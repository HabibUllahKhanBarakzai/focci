package cmd

import (
	"fmt"
	"os"

	"github.com/spf13/cobra"

	"github.com/HabibUllahKhanBarakzai/focci/internal/config"
)

var (
	installAgent   string
	installCommand string
	installForce   bool
)

var installCmd = &cobra.Command{
	Use:   "install",
	Short: "Wire focci into your agents' configuration files",
	Run: func(cmd *cobra.Command, args []string) {
		target, err := parseAgentFlag(installAgent)
		if err != nil {
			exitf(2, "%v\n", err)
		}
		// Distinguish an explicit (possibly empty) --command from no override.
		var command *string
		if cmd.Flags().Changed("command") {
			command = &installCommand
		}

		failed := false
		if target.claude {
			if report, err := config.InstallClaude(command); err != nil {
				fmt.Fprintf(os.Stderr, "Claude Code install failed: %v\n", err)
				failed = true
			} else {
				fmt.Println(report.DescribeInstall("Claude Code"))
			}
		}
		if target.codex {
			if report, err := config.InstallCodex(command, installForce); err != nil {
				fmt.Fprintf(os.Stderr, "Codex install failed: %v\n", err)
				failed = true
			} else {
				fmt.Println(report.DescribeInstall("Codex"))
			}
		}
		if failed {
			os.Exit(1)
		}
		os.Exit(0)
	},
}

func init() {
	installCmd.Flags().StringVar(&installAgent, "agent", "all", "which agent(s) to configure: claude|codex|all")
	installCmd.Flags().StringVar(&installCommand, "command", "", "override the command written into configs (default: this binary's path)")
	installCmd.Flags().BoolVar(&installForce, "force", false, "for Codex: overwrite an existing unrelated notify setting")
}
