package focus

import (
	"os"
	"path/filepath"
	"strconv"
	"testing"
)

func TestBundleForTermProgram(t *testing.T) {
	cases := map[string]string{
		"Apple_Terminal": "com.apple.Terminal",
		"iTerm.app":      "com.googlecode.iterm2",
		"WarpTerminal":   "dev.warp.Warp-Stable",
		"vscode":         "com.microsoft.VSCode",
		"ghostty":        "com.mitchellh.ghostty",
		"Ghostty":        "com.mitchellh.ghostty",
		"kitty":          "net.kovidgoyal.kitty",
	}
	for term, want := range cases {
		got, ok := bundleForTermProgram(term)
		if !ok || got != want {
			t.Errorf("bundleForTermProgram(%q) = %q,%v; want %q,true", term, got, ok, want)
		}
	}
	if _, ok := bundleForTermProgram("UnknownTerm"); ok {
		t.Error("unknown TERM_PROGRAM should not map")
	}
}

func TestResolveBundlePriority(t *testing.T) {
	t.Setenv("FOCCI_BUNDLE_ID", "com.example.override")
	t.Setenv("__CFBundleIdentifier", "com.example.launcher")
	t.Setenv("TERM_PROGRAM", "iTerm.app")
	if res, ok := ResolveBundle(); !ok || res.BundleID != "com.example.override" || res.Source != "FOCCI_BUNDLE_ID" {
		t.Fatalf("override should win, got %+v ok=%v", res, ok)
	}

	t.Setenv("FOCCI_BUNDLE_ID", "")
	if res, ok := ResolveBundle(); !ok || res.BundleID != "com.example.launcher" || res.Source != "__CFBundleIdentifier" {
		t.Fatalf("launcher should win, got %+v ok=%v", res, ok)
	}

	t.Setenv("__CFBundleIdentifier", "")
	if res, ok := ResolveBundle(); !ok || res.BundleID != "com.googlecode.iterm2" || res.Source != "TERM_PROGRAM" {
		t.Fatalf("term_program should map, got %+v ok=%v", res, ok)
	}

	t.Setenv("TERM_PROGRAM", "")
	if _, ok := ResolveBundle(); ok {
		t.Fatal("no env should resolve to no target")
	}
}

func TestStampPathSanitizes(t *testing.T) {
	t.Setenv("TMPDIR", "/var/tmp")
	got := stampPath("com.apple.Terminal")
	want := filepath.Join("/var/tmp", "focci-com_apple_Terminal.stamp")
	if got != want {
		t.Fatalf("stampPath = %q; want %q", got, want)
	}
}

func TestDebounceMS(t *testing.T) {
	t.Setenv("FOCCI_DEBOUNCE_MS", "")
	if got := DebounceMS(); got != defaultDebounceMS {
		t.Errorf("default debounce = %d; want %d", got, defaultDebounceMS)
	}
	t.Setenv("FOCCI_DEBOUNCE_MS", "3000")
	if got := DebounceMS(); got != 3000 {
		t.Errorf("parsed debounce = %d; want 3000", got)
	}
	t.Setenv("FOCCI_DEBOUNCE_MS", "-5")
	if got := DebounceMS(); got != defaultDebounceMS {
		t.Errorf("negative debounce should fall back to default, got %d", got)
	}
	t.Setenv("FOCCI_DEBOUNCE_MS", "abc")
	if got := DebounceMS(); got != defaultDebounceMS {
		t.Errorf("non-numeric debounce should fall back to default, got %d", got)
	}
}

func TestRecentlyFocused(t *testing.T) {
	dir := t.TempDir()
	t.Setenv("TMPDIR", dir)
	bundle := "com.example.app"

	if recentlyFocused(bundle, 1500) {
		t.Fatal("no stamp file should mean not recently focused")
	}
	if err := os.WriteFile(stampPath(bundle), []byte(strconv.FormatInt(nowMS(), 10)), 0o644); err != nil {
		t.Fatal(err)
	}
	if !recentlyFocused(bundle, 1500) {
		t.Fatal("fresh stamp should be recent")
	}
	if err := os.WriteFile(stampPath(bundle), []byte(strconv.FormatInt(nowMS()-10000, 10)), 0o644); err != nil {
		t.Fatal(err)
	}
	if recentlyFocused(bundle, 1500) {
		t.Fatal("old stamp should not be recent")
	}
	if err := os.WriteFile(stampPath(bundle), []byte("garbage"), 0o644); err != nil {
		t.Fatal(err)
	}
	if recentlyFocused(bundle, 1500) {
		t.Fatal("unparseable stamp should not be recent")
	}
}
