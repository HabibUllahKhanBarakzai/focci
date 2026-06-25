package config

import (
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/tidwall/gjson"
)

const testCommand = "/opt/homebrew/bin/focci"

func writeFile(t *testing.T, path, content string) {
	t.Helper()
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(path, []byte(content), 0o644); err != nil {
		t.Fatal(err)
	}
}

func readFile(t *testing.T, path string) string {
	t.Helper()
	data, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	return string(data)
}

func ptr(s string) *string { return &s }

// --- Claude Code -----------------------------------------------------------

func TestInstallClaudeFresh(t *testing.T) {
	t.Setenv("HOME", t.TempDir())

	report, err := InstallClaude(ptr(testCommand))
	if err != nil {
		t.Fatal(err)
	}
	if !report.Changed {
		t.Fatal("fresh install should report a change")
	}

	stop, notification := ClaudeStatus(ClaudeSettingsPath())
	if !stop || !notification {
		t.Fatalf("expected both hooks present, got stop=%v notification=%v", stop, notification)
	}

	data := []byte(readFile(t, ClaudeSettingsPath()))
	if got := gjson.GetBytes(data, `hooks.Stop.0.hooks.0.command`).String(); got != testCommand+" claude --event stop" {
		t.Errorf("stop command = %q", got)
	}
	if got := gjson.GetBytes(data, `hooks.Notification.0.hooks.0.command`).String(); got != testCommand+" claude --event notification" {
		t.Errorf("notification command = %q", got)
	}
	matcher := gjson.GetBytes(data, `hooks.Notification.0.matcher`)
	if !matcher.Exists() || matcher.String() != "" {
		t.Errorf("notification matcher = %q exists=%v; want empty present", matcher.String(), matcher.Exists())
	}
	if gjson.GetBytes(data, `hooks.Stop.0.matcher`).Exists() {
		t.Error("stop group should not carry a matcher key")
	}
}

func TestInstallClaudeIdempotent(t *testing.T) {
	t.Setenv("HOME", t.TempDir())

	if _, err := InstallClaude(ptr(testCommand)); err != nil {
		t.Fatal(err)
	}
	first := readFile(t, ClaudeSettingsPath())

	report, err := InstallClaude(ptr(testCommand))
	if err != nil {
		t.Fatal(err)
	}
	if report.Changed {
		t.Fatal("second identical install should not report a change")
	}
	if second := readFile(t, ClaudeSettingsPath()); second != first {
		t.Error("second install should not rewrite the file")
	}
}

func TestInstallClaudePreservesUserConfig(t *testing.T) {
	t.Setenv("HOME", t.TempDir())
	writeFile(t, ClaudeSettingsPath(), `{
  "model": "opus",
  "permissions": { "allow": ["Bash"] },
  "hooks": {
    "PreToolUse": [{ "matcher": "Bash", "hooks": [{ "type": "command", "command": "echo hi" }] }]
  }
}`)

	if _, err := InstallClaude(ptr(testCommand)); err != nil {
		t.Fatal(err)
	}

	data := []byte(readFile(t, ClaudeSettingsPath()))
	if gjson.GetBytes(data, "model").String() != "opus" {
		t.Error("model key should be preserved")
	}
	if gjson.GetBytes(data, "permissions.allow.0").String() != "Bash" {
		t.Error("permissions should be preserved")
	}
	if gjson.GetBytes(data, `hooks.PreToolUse.0.hooks.0.command`).String() != "echo hi" {
		t.Error("existing PreToolUse hook should be preserved")
	}
	stop, notification := ClaudeStatus(ClaudeSettingsPath())
	if !stop || !notification {
		t.Error("our hooks should have been added alongside the user's")
	}
}

func TestUninstallClaudeKeepsOtherHooks(t *testing.T) {
	t.Setenv("HOME", t.TempDir())
	writeFile(t, ClaudeSettingsPath(), `{
  "model": "opus",
  "hooks": {
    "PreToolUse": [{ "matcher": "Bash", "hooks": [{ "type": "command", "command": "echo hi" }] }]
  }
}`)
	if _, err := InstallClaude(ptr(testCommand)); err != nil {
		t.Fatal(err)
	}

	report, err := UninstallClaude()
	if err != nil {
		t.Fatal(err)
	}
	if !report.Changed {
		t.Fatal("uninstall should report a change")
	}

	stop, notification := ClaudeStatus(ClaudeSettingsPath())
	if stop || notification {
		t.Error("our hooks should be gone")
	}
	data := []byte(readFile(t, ClaudeSettingsPath()))
	if gjson.GetBytes(data, `hooks.PreToolUse.0.hooks.0.command`).String() != "echo hi" {
		t.Error("user's PreToolUse hook should remain")
	}
	if gjson.GetBytes(data, "model").String() != "opus" {
		t.Error("model key should remain")
	}
}

func TestUninstallClaudeRemovesEmptyHooksKey(t *testing.T) {
	t.Setenv("HOME", t.TempDir())
	if _, err := InstallClaude(ptr(testCommand)); err != nil {
		t.Fatal(err)
	}

	report, err := UninstallClaude()
	if err != nil {
		t.Fatal(err)
	}
	if !report.Changed {
		t.Fatal("uninstall should report a change")
	}
	data := []byte(readFile(t, ClaudeSettingsPath()))
	if gjson.GetBytes(data, "hooks").Exists() {
		t.Error("hooks key should be removed once empty")
	}
}

func TestInstallClaudeUpdatesChangedCommand(t *testing.T) {
	t.Setenv("HOME", t.TempDir())
	if _, err := InstallClaude(ptr("/old/focci")); err != nil {
		t.Fatal(err)
	}

	report, err := InstallClaude(ptr("/new/focci"))
	if err != nil {
		t.Fatal(err)
	}
	if !report.Changed {
		t.Fatal("changing the command should report a change")
	}
	data := []byte(readFile(t, ClaudeSettingsPath()))
	if n := len(gjson.GetBytes(data, "hooks.Stop").Array()); n != 1 {
		t.Fatalf("expected a single Stop group, got %d", n)
	}
	if got := gjson.GetBytes(data, `hooks.Stop.0.hooks.0.command`).String(); got != "/new/focci claude --event stop" {
		t.Errorf("command should be updated in place, got %q", got)
	}
}

func TestInstallClaudeErrorsOnNonObjectHooks(t *testing.T) {
	t.Setenv("HOME", t.TempDir())
	writeFile(t, ClaudeSettingsPath(), `{"hooks": "nope"}`)
	if _, err := InstallClaude(ptr(testCommand)); err == nil {
		t.Fatal("expected an error when hooks is not an object")
	}
}

// --- Codex -----------------------------------------------------------------

func TestInstallCodexFresh(t *testing.T) {
	t.Setenv("HOME", t.TempDir())

	report, err := InstallCodex(ptr(testCommand), false)
	if err != nil {
		t.Fatal(err)
	}
	if !report.Changed {
		t.Fatal("fresh codex install should report a change")
	}
	content := readFile(t, CodexConfigPath())
	if !strings.Contains(content, `notify = ["`+testCommand+`", "codex"]`) {
		t.Errorf("unexpected notify line: %q", content)
	}
	if !CodexStatus(CodexConfigPath()) {
		t.Error("CodexStatus should report wired")
	}
}

func TestInstallCodexUnrelatedNoForce(t *testing.T) {
	t.Setenv("HOME", t.TempDir())
	original := "notify = [\"/other/notifier\"]\n"
	writeFile(t, CodexConfigPath(), original)

	report, err := InstallCodex(ptr(testCommand), false)
	if err != nil {
		t.Fatal(err)
	}
	if report.Changed {
		t.Fatal("an unrelated notify should not be overwritten without --force")
	}
	if report.Note == "" {
		t.Error("expected a note explaining the unrelated notify was left alone")
	}
	if got := readFile(t, CodexConfigPath()); got != original {
		t.Errorf("file should be untouched, got %q", got)
	}
}

func TestInstallCodexUnrelatedForce(t *testing.T) {
	t.Setenv("HOME", t.TempDir())
	writeFile(t, CodexConfigPath(), "notify = [\"/other/notifier\"]\n")

	report, err := InstallCodex(ptr(testCommand), true)
	if err != nil {
		t.Fatal(err)
	}
	if !report.Changed {
		t.Fatal("--force should overwrite the unrelated notify")
	}
	content := readFile(t, CodexConfigPath())
	if strings.Contains(content, "/other/notifier") {
		t.Error("the unrelated notify should be gone")
	}
	if !CodexStatus(CodexConfigPath()) {
		t.Error("CodexStatus should now report wired")
	}
}

func TestInstallCodexPreservesOtherConfig(t *testing.T) {
	t.Setenv("HOME", t.TempDir())
	writeFile(t, CodexConfigPath(), `# my codex config
model = "o3"

[tui]
theme = "dark"
`)

	if _, err := InstallCodex(ptr(testCommand), false); err != nil {
		t.Fatal(err)
	}
	content := readFile(t, CodexConfigPath())
	if !strings.Contains(content, `model = "o3"`) || !strings.Contains(content, "[tui]") || !strings.Contains(content, `theme = "dark"`) {
		t.Errorf("user config should be preserved, got:\n%s", content)
	}
	if !CodexStatus(CodexConfigPath()) {
		t.Error("notify should be wired at the top level")
	}
	if strings.Index(content, "notify") > strings.Index(content, "[tui]") {
		t.Error("notify should precede the [tui] table header")
	}
}

func TestNotifyUnderTableNotMatched(t *testing.T) {
	t.Setenv("HOME", t.TempDir())
	writeFile(t, CodexConfigPath(), "[hooks]\nnotify = [\"x\"]\n")

	if CodexStatus(CodexConfigPath()) {
		t.Error("a notify under a [table] is not the top-level notify")
	}
	if _, err := InstallCodex(ptr(testCommand), false); err != nil {
		t.Fatal(err)
	}
	content := readFile(t, CodexConfigPath())
	if !strings.Contains(content, "[hooks]") || !strings.Contains(content, `notify = ["x"]`) {
		t.Error("the [hooks] table's notify should be left untouched")
	}
	if !CodexStatus(CodexConfigPath()) {
		t.Error("a real top-level notify should now be wired")
	}
}

func TestUninstallCodexPreservesComments(t *testing.T) {
	t.Setenv("HOME", t.TempDir())
	writeFile(t, CodexConfigPath(), `# header comment
model = "o3"
notify = ["`+testCommand+`", "codex"]
other = 1
`)

	report, err := UninstallCodex()
	if err != nil {
		t.Fatal(err)
	}
	if !report.Changed {
		t.Fatal("uninstall should remove our notify")
	}
	content := readFile(t, CodexConfigPath())
	if !strings.Contains(content, "# header comment") || !strings.Contains(content, `model = "o3"`) || !strings.Contains(content, "other = 1") {
		t.Errorf("surrounding config should be preserved, got:\n%s", content)
	}
	if strings.Contains(content, "notify") {
		t.Error("notify line should be removed")
	}
}

func TestCodexMultilineArray(t *testing.T) {
	t.Setenv("HOME", t.TempDir())
	writeFile(t, CodexConfigPath(), "notify = [\n  \""+testCommand+"\",\n  \"codex\",\n]\nextra = 1\n")

	if !CodexStatus(CodexConfigPath()) {
		t.Fatal("a multi-line notify array referencing us should be detected")
	}
	if _, err := UninstallCodex(); err != nil {
		t.Fatal(err)
	}
	content := readFile(t, CodexConfigPath())
	if strings.Contains(content, "focci") || strings.Contains(content, "notify") {
		t.Errorf("the whole multi-line array should be removed, got:\n%s", content)
	}
	if !strings.Contains(content, "extra = 1") {
		t.Error("config after the array should remain")
	}
}

func TestCodexNotifyWithInlineComment(t *testing.T) {
	t.Setenv("HOME", t.TempDir())
	writeFile(t, CodexConfigPath(), "notify = [\""+testCommand+"\", \"codex\"] # trailing comment\nkeep = true\n")

	if !CodexStatus(CodexConfigPath()) {
		t.Fatal("notify with a trailing comment should still be detected")
	}
	if _, err := UninstallCodex(); err != nil {
		t.Fatal(err)
	}
	content := readFile(t, CodexConfigPath())
	if strings.Contains(content, "notify") || strings.Contains(content, "trailing comment") {
		t.Errorf("the notify line and its inline comment should be removed, got:\n%s", content)
	}
	if !strings.Contains(content, "keep = true") {
		t.Error("following config should remain")
	}
}

// TestInstallCodexDottedKeyNoForce guards the bug where a dotted notify key
// (which makes notify a table) was not detected, producing a duplicate key.
func TestInstallCodexDottedKeyNoForce(t *testing.T) {
	t.Setenv("HOME", t.TempDir())
	original := "notify.command = \"/other\"\nnotify.args = [\"x\"]\nmodel = \"o3\"\n"
	writeFile(t, CodexConfigPath(), original)

	report, err := InstallCodex(ptr(testCommand), false)
	if err != nil {
		t.Fatal(err)
	}
	if report.Changed {
		t.Fatal("a dotted-key notify is an unrelated notify; must not be overwritten without --force")
	}
	if got := readFile(t, CodexConfigPath()); got != original {
		t.Errorf("file must be untouched, got:\n%s", got)
	}
}

// TestInstallCodexDottedKeyForce verifies --force replaces a table-form notify
// with a valid single array key (no duplicate definition).
func TestInstallCodexDottedKeyForce(t *testing.T) {
	t.Setenv("HOME", t.TempDir())
	writeFile(t, CodexConfigPath(), "notify.command = \"/other\"\nnotify.args = [\"x\"]\nmodel = \"o3\"\n")

	if _, err := InstallCodex(ptr(testCommand), true); err != nil {
		t.Fatal(err)
	}
	content := readFile(t, CodexConfigPath())
	// Must be valid TOML with notify as our array and no leftover dotted keys.
	exists, refs, err := detectNotify(content)
	if err != nil {
		t.Fatalf("result must be valid TOML, got parse error %v:\n%s", err, content)
	}
	if !exists || !refs {
		t.Errorf("notify should now reference focci, got:\n%s", content)
	}
	if strings.Contains(content, "/other") {
		t.Errorf("the old dotted notify should be gone, got:\n%s", content)
	}
}

func TestFindNotifyAfterMultilineArrayValue(t *testing.T) {
	lines := strings.Split("matrix = [\n  [1, 2],\n  [3, 4],\n]\nnotify = [\""+testCommand+"\", \"codex\"]\n", "\n")
	span, ok := findNotify(lines)
	if !ok {
		t.Fatal("notify after a multi-line array should still be found")
	}
	if !strings.Contains(lines[span.startIdx], "notify") {
		t.Errorf("notify span misread: %+v", span)
	}
}

func TestNotifyAssignmentEq(t *testing.T) {
	matches := []string{`notify = ["a"]`, "notify=[\"a\"]", "  notify   = 1", "\tnotify\t=\t2"}
	for _, line := range matches {
		if notifyAssignmentEq(line) < 0 {
			t.Errorf("expected %q to be a notify assignment", line)
		}
	}
	nonMatches := []string{"notifyx = 1", "notify_extra = 2", "# notify = 1", "model = 1", "notifier = 3", "notify.command = 1"}
	for _, line := range nonMatches {
		if notifyAssignmentEq(line) >= 0 {
			t.Errorf("did not expect %q to match a bare notify", line)
		}
	}
}
