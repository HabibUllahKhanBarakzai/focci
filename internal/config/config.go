// Package config wires focci into the agents' own configuration files,
// idempotently.
//
//   - Claude Code: register Stop and Notification command hooks in
//     ~/.claude/settings.json.
//   - Codex: set notify = ["<binary>", "codex"] in ~/.codex/config.toml.
//
// Existing files are preserved: we only add/replace our own entries, back up the
// file before writing, and never clobber an unrelated Codex notify unless
// forced.
package config

import (
	"fmt"
	"os"
	"path/filepath"
)

// marker is the substring used to recognize hook/notify entries that belong to
// us.
const marker = "focci"

// Report describes the result of an install/uninstall against one config file.
type Report struct {
	Path    string
	Changed bool
	Note    string // empty == no note
}

func (r Report) DescribeInstall(agent string) string {
	state := "already configured"
	if r.Changed {
		state = "configured"
	}
	return r.format(agent, state)
}

func (r Report) DescribeUninstall(agent string) string {
	state := "not present"
	if r.Changed {
		state = "removed"
	}
	return r.format(agent, state)
}

func (r Report) format(agent, state string) string {
	out := fmt.Sprintf("%s: %s (%s)", agent, state, r.Path)
	if r.Note != "" {
		out += fmt.Sprintf("\n  note: %s", r.Note)
	}
	return out
}

func home() string {
	if value, ok := os.LookupEnv("HOME"); ok {
		return value
	}
	return "."
}

// ClaudeSettingsPath is the path to Claude Code's settings.json.
func ClaudeSettingsPath() string {
	return filepath.Join(home(), ".claude", "settings.json")
}

// CodexConfigPath is the path to Codex's config.toml.
func CodexConfigPath() string {
	return filepath.Join(home(), ".codex", "config.toml")
}

// BinaryCommand is the command string written into configs: the absolute path of
// this binary so the hook resolves regardless of the GUI process's PATH.
func BinaryCommand() string {
	exe, err := os.Executable()
	if err == nil {
		if resolved, err := filepath.EvalSymlinks(exe); err == nil {
			return resolved
		}
	}
	return marker
}

func fileExists(path string) bool {
	_, err := os.Stat(path)
	return err == nil
}

// backup copies path to path.bak (preserving mode) when it exists.
func backup(path string) error {
	if !fileExists(path) {
		return nil
	}
	info, err := os.Stat(path)
	if err != nil {
		return err
	}
	data, err := os.ReadFile(path)
	if err != nil {
		return err
	}
	backupPath := filepath.Join(filepath.Dir(path), filepath.Base(path)+".bak")
	return os.WriteFile(backupPath, data, info.Mode().Perm())
}

func ensureParent(path string) error {
	dir := filepath.Dir(path)
	if dir == "" {
		return nil
	}
	return os.MkdirAll(dir, 0o755)
}
