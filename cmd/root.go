// Package cmd wires focci's command-line interface with Cobra.
package cmd

import (
	"fmt"
	"os"

	"github.com/spf13/cobra"
)

const version = "0.1.0"

var rootCmd = &cobra.Command{
	Use:   "focci",
	Short: "Refocus your terminal/editor when an AI coding agent needs your attention.",
	Long: "focci refocuses your terminal/editor the moment an AI coding agent (Claude Code, Codex) " +
		"finishes a turn or needs your attention — and ignores the repeating idle reminders so it does " +
		"not keep yanking you back once you have moved on.",
	Version:       version,
	SilenceUsage:  true,
	SilenceErrors: false,
}

// Execute runs the root command. The hook subcommands manage their own exit
// codes; usage errors surface as a non-zero exit here.
func Execute() {
	rootCmd.SetVersionTemplate("focci {{.Version}}\n")
	if err := rootCmd.Execute(); err != nil {
		os.Exit(1)
	}
}

func init() {
	rootCmd.AddCommand(claudeCmd, codexCmd, focusCmd, installCmd, uninstallCmd, doctorCmd)
}

// exitf prints to stderr and exits with the given code.
func exitf(code int, format string, args ...any) {
	fmt.Fprintf(os.Stderr, format, args...)
	os.Exit(code)
}
