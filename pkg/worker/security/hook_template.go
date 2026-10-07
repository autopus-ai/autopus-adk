package security

import (
	"encoding/json"
	"errors"
	"fmt"
	"maps"
	"os"
	"path/filepath"
	"strings"

	"github.com/insajin/autopus-adk/pkg/adapter"
)

const (
	// workerHookEvent is the settings.json event the worker handler joins.
	workerHookEvent = "PreToolUse"
	// workerHookMatcher selects the tool calls the policy validates.
	workerHookMatcher = "Bash|Write|Edit"
	// workerHookCommandPrefix marks the one handler this package owns. Setup
	// and cleanup touch no handler whose command does not start with it.
	workerHookCommandPrefix = "auto worker validate "
)

// workerHookEntry is the matcher entry of the worker validate handler in
// Claude Code's handler object format.
func workerHookEntry(policyPath string) map[string]any {
	return map[string]any{
		"matcher": workerHookMatcher,
		"hooks": []any{map[string]any{
			"type":    "command",
			"command": fmt.Sprintf("auto worker validate --policy '%s' --command \"$TOOL_INPUT\"", policyPath),
		}},
	}
}

// GenerateHookConfig returns the hooks fragment the worker adds to Claude Code
// settings.json. policyPath is the absolute path to the SecurityPolicy file.
func GenerateHookConfig(policyPath string) map[string]any {
	return map[string]any{
		"hooks": map[string]any{workerHookEvent: []any{workerHookEntry(policyPath)}},
	}
}

// WriteHookConfig adds the worker validate handler to the given directory's
// .claude/settings.json. settings.json is shared with the user and with other
// Autopus writers, so the handler is merged one handler at a time
// (SPEC-EDITGUARD-001 REQ-EG-19): a handler an earlier setup left is replaced,
// and every other key, event, entry, and handler keeps its value and order.
func WriteHookConfig(dir string, policyPath string) error {
	settingsDir := filepath.Join(dir, ".claude")
	if err := os.MkdirAll(settingsDir, 0755); err != nil {
		return fmt.Errorf("create .claude directory: %w", err)
	}

	settingsPath := filepath.Join(settingsDir, "settings.json")
	settings := make(map[string]any)
	data, err := os.ReadFile(settingsPath)
	if err == nil {
		if err := json.Unmarshal(data, &settings); err != nil {
			return fmt.Errorf("parse existing settings.json: %w", err)
		}
		if settings == nil {
			settings = make(map[string]any)
		}
	} else if !os.IsNotExist(err) {
		return fmt.Errorf("read settings.json: %w", err)
	}

	hooks := make(map[string]any)
	if existing, ok := settings["hooks"]; ok {
		if hooks, ok = existing.(map[string]any); !ok {
			return errors.New("settings.json hooks must be an object")
		}
	}
	var entries []any
	if existing, ok := hooks[workerHookEvent]; ok {
		if entries, ok = existing.([]any); !ok {
			return fmt.Errorf("settings.json hooks.%s must be an array", workerHookEvent)
		}
	}
	entries, _ = retractWorkerHandlers(entries)
	hooks[workerHookEvent] = append(entries, workerHookEntry(policyPath))
	settings["hooks"] = hooks
	return writeSettings(settingsPath, settings)
}

// RemoveHookConfig takes the worker validate handler back out of
// settings.json and leaves every other handler where it was (REQ-EG-19). An
// entry, the event, and the hooks object are dropped only when this removal
// left them empty. A file without the handler is not rewritten.
func RemoveHookConfig(dir string) error {
	settingsPath := filepath.Join(dir, ".claude", "settings.json")

	data, err := os.ReadFile(settingsPath)
	if err != nil {
		if os.IsNotExist(err) {
			return nil
		}
		return fmt.Errorf("read settings.json: %w", err)
	}

	var settings map[string]any
	if err := json.Unmarshal(data, &settings); err != nil {
		return fmt.Errorf("parse settings.json: %w", err)
	}

	hooks, _ := settings["hooks"].(map[string]any)
	entries, _ := hooks[workerHookEvent].([]any)
	kept, removed := retractWorkerHandlers(entries)
	if !removed {
		return nil
	}
	if len(kept) > 0 {
		hooks[workerHookEvent] = kept
	} else {
		delete(hooks, workerHookEvent)
	}
	if len(hooks) == 0 {
		delete(settings, "hooks")
	}
	return writeSettings(settingsPath, settings)
}

// retractWorkerHandlers removes the worker handlers from entries one handler
// at a time and reports whether it removed any. An entry keeps its matcher,
// its other keys, and every handler it holds besides the worker's, and is
// dropped only when the removal left it without a handler. A value that is
// not an entry object with a handler array comes back unchanged.
func retractWorkerHandlers(entries []any) ([]any, bool) {
	kept := make([]any, 0, len(entries))
	removed := false
	for _, entry := range entries {
		object, _ := entry.(map[string]any)
		handlers, ok := object["hooks"].([]any)
		if !ok {
			kept = append(kept, entry)
			continue
		}
		remaining := make([]any, 0, len(handlers))
		for _, handler := range handlers {
			if !isWorkerHandler(handler) {
				remaining = append(remaining, handler)
			}
		}
		if len(remaining) == len(handlers) {
			kept = append(kept, entry)
			continue
		}
		removed = true
		if len(remaining) > 0 {
			pruned := maps.Clone(object)
			pruned["hooks"] = remaining
			kept = append(kept, pruned)
		}
	}
	return kept, removed
}

// isWorkerHandler reports whether handler is the worker validate handler, in
// the handler object format or as the bare command string older releases
// wrote.
func isWorkerHandler(handler any) bool {
	command, _ := handler.(string)
	if object, ok := handler.(map[string]any); ok {
		command, _ = object["command"].(string)
	}
	return strings.HasPrefix(strings.TrimSpace(command), workerHookCommandPrefix)
}

// writeSettings writes settings in the layout the Claude Code adapter writes,
// so a setup and cleanup cycle does not reformat a file `auto update` wrote.
func writeSettings(settingsPath string, settings map[string]any) error {
	out, err := json.MarshalIndent(settings, "", "  ")
	if err != nil {
		return fmt.Errorf("marshal settings.json: %w", err)
	}
	if err := adapter.WriteFileIfChanged(settingsPath, append(out, '\n'), 0644); err != nil {
		return fmt.Errorf("write settings.json: %w", err)
	}
	return nil
}
