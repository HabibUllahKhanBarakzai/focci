package config

import (
	"bytes"
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"strings"

	"github.com/tidwall/gjson"
	"github.com/tidwall/sjson"
)

type hookEntryJSON struct {
	Type    string `json:"type"`
	Command string `json:"command"`
}

type hookGroupJSON struct {
	Matcher *string         `json:"matcher,omitempty"`
	Hooks   []hookEntryJSON `json:"hooks"`
}

// readJSONObject returns the raw bytes of a JSON object document. A missing or
// blank file yields an empty object; anything that is valid JSON but not an
// object, or unparseable, is an error.
func readJSONObject(path string) ([]byte, error) {
	data, err := os.ReadFile(path)
	if err != nil {
		if errors.Is(err, os.ErrNotExist) {
			return []byte("{}"), nil
		}
		return nil, err
	}
	if strings.TrimSpace(string(data)) == "" {
		return []byte("{}"), nil
	}
	if !gjson.ValidBytes(data) {
		return nil, fmt.Errorf("could not parse %s", path)
	}
	if !gjson.ParseBytes(data).IsObject() {
		return nil, fmt.Errorf("%s is not a JSON object", path)
	}
	return data, nil
}

// writeJSONObject pretty-prints (2-space, trailing newline) and writes. Key order
// and unknown keys are preserved because json.Indent only reflows whitespace and
// the sjson edits are surgical.
func writeJSONObject(path string, data []byte) error {
	if err := ensureParent(path); err != nil {
		return err
	}
	var buf bytes.Buffer
	if err := json.Indent(&buf, data, "", "  "); err != nil {
		return err
	}
	buf.WriteByte('\n')
	return os.WriteFile(path, buf.Bytes(), 0o644)
}

// hookIsOurs reports whether a hook entry belongs to us, for the given --event
// marker.
func hookIsOurs(entry gjson.Result, eventMarker string) bool {
	command := entry.Get("command")
	if command.Type != gjson.String {
		return false
	}
	value := command.String()
	return strings.Contains(value, marker) && strings.Contains(value, eventMarker)
}

// ensureCommandGroup ensures hooks[event] contains a command hook running
// command. It returns the (possibly updated) document and whether it changed.
func ensureCommandGroup(data []byte, event string, matcher *string, command, eventMarker string) ([]byte, bool) {
	base := "hooks." + event
	groups := gjson.GetBytes(data, base)

	// If one of our hooks is already registered, update its command if needed.
	if groups.IsArray() {
		for groupIdx, group := range groups.Array() {
			hooks := group.Get("hooks")
			if !hooks.IsArray() {
				continue
			}
			for entryIdx, entry := range hooks.Array() {
				if hookIsOurs(entry, eventMarker) {
					if entry.Get("command").String() == command {
						return data, false
					}
					path := fmt.Sprintf("%s.%d.hooks.%d.command", base, groupIdx, entryIdx)
					updated, _ := sjson.SetBytes(data, path, command)
					return updated, true
				}
			}
		}
	}

	// Otherwise append a fresh group, coercing a non-array value to an array
	// first (mirrors resetting a malformed value).
	if !groups.IsArray() {
		data, _ = sjson.SetRawBytes(data, base, []byte("[]"))
	}
	group := hookGroupJSON{Matcher: matcher, Hooks: []hookEntryJSON{{Type: "command", Command: command}}}
	updated, _ := sjson.SetBytes(data, base+".-1", group)
	return updated, true
}

// removeCommandGroup removes our hooks from hooks[event], pruning emptied groups
// and the event key itself. It returns the document and whether anything was
// removed.
func removeCommandGroup(data []byte, event, eventMarker string) ([]byte, bool) {
	base := "hooks." + event
	groups := gjson.GetBytes(data, base)
	if !groups.IsArray() {
		return data, false
	}

	changed := false
	keptGroups := make([][]byte, 0, len(groups.Array()))
	for _, group := range groups.Array() {
		groupRaw := []byte(group.Raw)
		hooks := group.Get("hooks")
		if !hooks.IsArray() {
			// A group with no hooks array is left untouched.
			keptGroups = append(keptGroups, groupRaw)
			continue
		}

		keptEntries := make([][]byte, 0, len(hooks.Array()))
		removedAny := false
		for _, entry := range hooks.Array() {
			if hookIsOurs(entry, eventMarker) {
				removedAny = true
				continue
			}
			keptEntries = append(keptEntries, []byte(entry.Raw))
		}
		if removedAny {
			changed = true
			groupRaw, _ = sjson.SetRawBytes(groupRaw, "hooks", joinRaw(keptEntries))
		}

		// Drop groups whose hooks array is now (or was already) empty.
		current := gjson.GetBytes(groupRaw, "hooks")
		if current.IsArray() && len(current.Array()) == 0 {
			continue
		}
		keptGroups = append(keptGroups, groupRaw)
	}

	if len(keptGroups) == 0 {
		updated, _ := sjson.DeleteBytes(data, base)
		return updated, changed
	}
	updated, _ := sjson.SetRawBytes(data, base, joinRaw(keptGroups))
	return updated, changed
}

func joinRaw(items [][]byte) []byte {
	var buf bytes.Buffer
	buf.WriteByte('[')
	for i, item := range items {
		if i > 0 {
			buf.WriteByte(',')
		}
		buf.Write(item)
	}
	buf.WriteByte(']')
	return buf.Bytes()
}

// InstallClaude registers focci's Stop and Notification hooks. commandOverride,
// when non-nil, replaces the detected binary path.
func InstallClaude(commandOverride *string) (Report, error) {
	path := ClaudeSettingsPath()
	data, err := readJSONObject(path)
	if err != nil {
		return Report{}, err
	}
	command := BinaryCommand()
	if commandOverride != nil {
		command = *commandOverride
	}

	if existing := gjson.GetBytes(data, "hooks"); existing.Exists() && !existing.IsObject() {
		return Report{}, errors.New(`"hooks" in settings.json is not an object`)
	}

	stopCommand := command + " claude --event stop"
	notificationCommand := command + " claude --event notification"

	data, stopChanged := ensureCommandGroup(data, "Stop", nil, stopCommand, "--event stop")
	emptyMatcher := ""
	data, notifChanged := ensureCommandGroup(data, "Notification", &emptyMatcher, notificationCommand, "--event notification")

	changed := stopChanged || notifChanged
	if changed {
		if err := backup(path); err != nil {
			return Report{}, err
		}
		if err := writeJSONObject(path, data); err != nil {
			return Report{}, err
		}
	}
	return Report{Path: path, Changed: changed}, nil
}

// UninstallClaude removes focci's hooks, pruning empty groups and the hooks key.
func UninstallClaude() (Report, error) {
	path := ClaudeSettingsPath()
	data, err := readJSONObject(path)
	if err != nil {
		return Report{}, err
	}

	changed := false
	if gjson.GetBytes(data, "hooks").IsObject() {
		var stopChanged, notifChanged bool
		data, stopChanged = removeCommandGroup(data, "Stop", "--event stop")
		data, notifChanged = removeCommandGroup(data, "Notification", "--event notification")
		if hooks := gjson.GetBytes(data, "hooks"); hooks.IsObject() && len(hooks.Map()) == 0 {
			data, _ = sjson.DeleteBytes(data, "hooks")
		}
		changed = stopChanged || notifChanged
	}

	if changed {
		if err := backup(path); err != nil {
			return Report{}, err
		}
		if err := writeJSONObject(path, data); err != nil {
			return Report{}, err
		}
	}
	return Report{Path: path, Changed: changed}, nil
}

// ClaudeStatus reports (stopHookPresent, notificationHookPresent).
func ClaudeStatus(path string) (bool, bool) {
	data, err := readJSONObject(path)
	if err != nil {
		return false, false
	}
	if !gjson.GetBytes(data, "hooks").IsObject() {
		return false, false
	}
	return eventHasOurHook(data, "Stop", "--event stop"),
		eventHasOurHook(data, "Notification", "--event notification")
}

func eventHasOurHook(data []byte, event, eventMarker string) bool {
	groups := gjson.GetBytes(data, "hooks."+event)
	if !groups.IsArray() {
		return false
	}
	for _, group := range groups.Array() {
		hooks := group.Get("hooks")
		if !hooks.IsArray() {
			continue
		}
		for _, entry := range hooks.Array() {
			if hookIsOurs(entry, eventMarker) {
				return true
			}
		}
	}
	return false
}
