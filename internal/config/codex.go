package config

import (
	"errors"
	"fmt"
	"os"
	"strings"

	toml "github.com/pelletier/go-toml/v2"
)

// Codex's notify lives as a single top-level key in config.toml. Detection uses
// a real TOML parser (so dotted keys and [notify] tables are recognized and we
// never produce a duplicate key), while writes are surgical line edits so the
// rest of the user's config — comments and formatting — is preserved verbatim.

// readTOML returns the file contents; a missing file is an empty document.
func readTOML(path string) (string, error) {
	data, err := os.ReadFile(path)
	if err != nil {
		if errors.Is(err, os.ErrNotExist) {
			return "", nil
		}
		return "", err
	}
	return string(data), nil
}

// detectNotify reports whether a top-level notify key exists (in any form) and
// whether it is an array referencing focci.
func detectNotify(content string) (exists, referencesUs bool, err error) {
	var tree map[string]any
	if err := toml.Unmarshal([]byte(content), &tree); err != nil {
		return false, false, err
	}
	value, ok := tree["notify"]
	if !ok {
		return false, false, nil
	}
	if arr, ok := value.([]any); ok {
		for _, elem := range arr {
			if s, ok := elem.(string); ok && strings.Contains(s, marker) {
				return true, true, nil
			}
		}
	}
	return true, false, nil
}

const unrelatedNotifyNote = "an unrelated `notify` is already set; left untouched. Re-run with --force to " +
	"overwrite, or chain focci from your existing notifier."

// InstallCodex sets notify to focci. An unrelated existing notify is left alone
// unless force is set.
func InstallCodex(commandOverride *string, force bool) (Report, error) {
	path := CodexConfigPath()
	content, err := readTOML(path)
	if err != nil {
		return Report{}, err
	}
	command := BinaryCommand()
	if commandOverride != nil {
		command = *commandOverride
	}

	exists, referencesUs, err := detectNotify(content)
	if err != nil {
		return Report{}, fmt.Errorf("could not parse %s: %w", path, err)
	}
	if exists && !referencesUs && !force {
		return Report{Path: path, Changed: false, Note: unrelatedNotifyNote}, nil
	}

	assignment := fmt.Sprintf("notify = [%q, %q]", command, "codex")
	newContent, err := writeNotify(content, exists, assignment, command)
	if err != nil {
		return Report{}, err
	}

	if err := backup(path); err != nil {
		return Report{}, err
	}
	if err := ensureParent(path); err != nil {
		return Report{}, err
	}
	if err := os.WriteFile(path, []byte(newContent), 0o644); err != nil {
		return Report{}, err
	}
	return Report{Path: path, Changed: true}, nil
}

// writeNotify produces the new file content with focci's notify in place.
func writeNotify(content string, exists bool, assignment, command string) (string, error) {
	if !exists {
		if content == "" {
			return assignment + "\n", nil
		}
		// Prepend so the key lands in the root table, above any [table] headers.
		return assignment + "\n" + content, nil
	}

	// Replace a locatable bare assignment in place, preserving comments.
	lines := strings.Split(content, "\n")
	if span, ok := findNotify(lines); ok {
		return spliceLines(lines, span, assignment), nil
	}

	// notify exists in a non-bare form (a dotted key or [notify] table). Rewrite
	// via the parser so no duplicate definition is left behind. This is the only
	// path that does not preserve comments, and only under --force.
	return rewriteNotifyViaParser(content, []string{command, "codex"})
}

// UninstallCodex removes focci's notify when present.
func UninstallCodex() (Report, error) {
	path := CodexConfigPath()
	content, err := readTOML(path)
	if err != nil {
		return Report{}, err
	}
	_, referencesUs, err := detectNotify(content)
	if err != nil {
		return Report{}, fmt.Errorf("could not parse %s: %w", path, err)
	}
	if !referencesUs {
		return Report{Path: path, Changed: false}, nil
	}

	lines := strings.Split(content, "\n")
	var newContent string
	if span, ok := findNotify(lines); ok {
		newContent = spliceLines(lines, span, "")
	} else {
		newContent, err = rewriteNotifyViaParser(content, nil)
		if err != nil {
			return Report{}, err
		}
	}

	if err := backup(path); err != nil {
		return Report{}, err
	}
	if err := os.WriteFile(path, []byte(newContent), 0o644); err != nil {
		return Report{}, err
	}
	return Report{Path: path, Changed: true}, nil
}

// CodexStatus reports whether focci's notify is wired.
func CodexStatus(path string) bool {
	content, err := readTOML(path)
	if err != nil {
		return false
	}
	_, referencesUs, err := detectNotify(content)
	if err != nil {
		return false
	}
	return referencesUs
}

// rewriteNotifyViaParser sets (or, when value is nil, deletes) the top-level
// notify key by re-serializing the document. Comments are not preserved.
func rewriteNotifyViaParser(content string, value []string) (string, error) {
	var tree map[string]any
	if err := toml.Unmarshal([]byte(content), &tree); err != nil {
		return "", err
	}
	if value == nil {
		delete(tree, "notify")
	} else {
		tree["notify"] = value
	}
	out, err := toml.Marshal(tree)
	if err != nil {
		return "", err
	}
	return string(out), nil
}

// --- surgical line scanning ------------------------------------------------

type notifySpan struct {
	startIdx int
	endIdx   int
}

// spliceLines replaces the span lines with replacement (dropped entirely when
// replacement is empty) and rejoins.
func spliceLines(lines []string, span notifySpan, replacement string) string {
	out := make([]string, 0, len(lines))
	out = append(out, lines[:span.startIdx]...)
	if replacement != "" {
		out = append(out, replacement)
	}
	out = append(out, lines[span.endIdx+1:]...)
	return strings.Join(out, "\n")
}

// findNotify locates a top-level bare `notify = ...` assignment (one appearing
// before any [table] header). Bracket depth is tracked across lines so a
// multi-line array value belonging to another key is not mistaken for a table
// header or a second assignment.
func findNotify(lines []string) (notifySpan, bool) {
	depth := 0
	for i, line := range lines {
		if depth == 0 {
			if strings.HasPrefix(strings.TrimSpace(line), "[") {
				return notifySpan{}, false // first table header ends the root region
			}
			if eq := notifyAssignmentEq(line); eq >= 0 {
				return notifySpan{startIdx: i, endIdx: notifyValueEndLine(lines, i, eq)}, true
			}
		}
		depth = scanLineDepth(line, depth)
	}
	return notifySpan{}, false
}

// notifyAssignmentEq returns the index of the '=' for a bare `notify` key
// assignment on this line, or -1 if the line is not such an assignment.
func notifyAssignmentEq(line string) int {
	rest := strings.TrimLeft(line, " \t")
	leading := len(line) - len(rest)
	if !strings.HasPrefix(rest, "notify") {
		return -1
	}
	after := strings.TrimLeft(rest[len("notify"):], " \t")
	if !strings.HasPrefix(after, "=") {
		return -1
	}
	return leading + len(rest) - len(after)
}

func firstNonSpace(s string) byte {
	for i := 0; i < len(s); i++ {
		if s[i] != ' ' && s[i] != '\t' {
			return s[i]
		}
	}
	return 0
}

// notifyValueEndLine returns the index of the last line the value spans. A value
// that opens a '[' continues until the brackets balance (ignoring brackets in
// strings and comments); any other value lives on its starting line.
func notifyValueEndLine(lines []string, startIdx, eqCol int) int {
	if firstNonSpace(lines[startIdx][eqCol+1:]) != '[' {
		return startIdx
	}
	depth := 0
	started := false
	for i := startIdx; i < len(lines); i++ {
		line := lines[i]
		col := 0
		if i == startIdx {
			col = eqCol + 1
		}
		var inString byte // 0, '"' or '\''
		for col < len(line) {
			c := line[col]
			if inString != 0 {
				if inString == '"' && c == '\\' {
					col += 2 // skip escaped char in a basic string
					continue
				}
				if c == inString {
					inString = 0
				}
				col++
				continue
			}
			switch c {
			case '#':
				col = len(line) // comment runs to end of line
				continue
			case '"', '\'':
				inString = c
			case '[':
				depth++
				started = true
			case ']':
				depth--
				if started && depth == 0 {
					return i
				}
			}
			col++
		}
	}
	return len(lines) - 1
}

// scanLineDepth returns the bracket depth at the end of a line, given the depth
// at its start, ignoring brackets inside strings and trailing comments.
func scanLineDepth(line string, depth int) int {
	var inString byte // 0, '"' or '\''
	for col := 0; col < len(line); col++ {
		c := line[col]
		if inString != 0 {
			if inString == '"' && c == '\\' {
				col++ // skip the escaped char in a basic string
				continue
			}
			if c == inString {
				inString = 0
			}
			continue
		}
		switch c {
		case '#':
			return depth // comment runs to end of line
		case '"', '\'':
			inString = c
		case '[':
			depth++
		case ']':
			if depth > 0 {
				depth--
			}
		}
	}
	return depth
}
