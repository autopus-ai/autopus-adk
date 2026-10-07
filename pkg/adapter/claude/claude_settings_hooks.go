package claude

import (
	"strings"

	"github.com/insajin/autopus-adk/pkg/adapter"
)

var managedClaudeHookCommandPrefixes = []string{
	"auto check --hygiene --arch --quiet --staged --warn-only",
	"auto react check --quiet",
	"auto rules fire ",
	"AUTOPUS_TASKCREATED_DEFAULT_MODE=",
	".claude/hooks/autopus/",
	".claude/hooks/task-created-validate.sh",
	`"${CLAUDE_PROJECT_DIR:-.}"/.claude/hooks/autopus/`,
}

// retractManagedHookEntries removes every Autopus-owned handler from every
// event. The unit is a handler (SPEC-PANERM-001 REQ-13): an entry is dropped
// only when no handler remains, so a user handler that shares an entry with a
// managed or group S one survives with its matcher and position.
func retractManagedHookEntries(hooks map[string]any) {
	adapter.RetractHookHandlers(hooks, isManagedClaudeHookCommand)
}

func isManagedClaudeHookEntry(entry any) bool {
	object, ok := entry.(map[string]any)
	if !ok {
		return false
	}
	for _, command := range entryHookCommands(object["hooks"]) {
		if isManagedClaudeHookCommand(command) {
			return true
		}
	}
	return false
}

func isManagedClaudeHookCommand(command string) bool {
	trimmed := strings.TrimSpace(command)
	if isStickyCommand(trimmed) || adapter.IsStaleCompletionHookCommand(adapterName, trimmed) {
		return true
	}
	for _, prefix := range managedClaudeHookCommandPrefixes {
		if strings.HasPrefix(trimmed, prefix) {
			return true
		}
	}
	return false
}

func claudeHookEntry(eventHook map[string]any, matcher string) map[string]any {
	entry := map[string]any{"hooks": []map[string]any{eventHook}}
	if matcher != "" {
		entry["matcher"] = matcher
	}
	return entry
}

func projectClaudeStatusLine(existing any, managed map[string]any) map[string]any {
	projected := make(map[string]any)
	if current, ok := existing.(map[string]any); ok {
		for key, value := range current {
			projected[key] = value
		}
	}
	for key, value := range managed {
		if key == "padding" {
			if _, present := projected[key]; present {
				continue
			}
		}
		projected[key] = value
	}
	return projected
}
