package claude

import (
	"maps"
	"strings"
)

var managedClaudeHookCommandPrefixes = []string{
	"auto check --hygiene --arch --quiet --staged --warn-only",
	"auto react check --quiet",
	"auto rules fire ",
	// SPEC-EDITGUARD-001 REQ-EG-12: the guard's exit-masking command line.
	// Anchored with its trailing space so a later release's flags stay owned
	// while a command that merely mentions the guard does not.
	"out=$(auto guard edit ",
	"AUTOPUS_TASKCREATED_DEFAULT_MODE=",
	".claude/hooks/autopus/",
	".claude/hooks/task-created-validate.sh",
	`"${CLAUDE_PROJECT_DIR:-.}"/.claude/hooks/autopus/`,
}

// retractManagedHookEntries removes Autopus-owned handlers from every event one
// handler at a time (SPEC-EDITGUARD-001 REQ-EG-19). settings.json is shared
// with the user, so an entry can hold a user handler beside an Autopus one;
// dropping the whole entry deleted the user's handler. An entry now keeps its
// matcher, its other keys, and every handler this writer does not own, and is
// dropped only when retraction left it with no handler.
func retractManagedHookEntries(hooks map[string]any) {
	for event, raw := range hooks {
		entries, ok := raw.([]any)
		if !ok {
			continue
		}
		kept := make([]any, 0, len(entries))
		for _, entry := range entries {
			if remaining, keep := retractManagedHandlers(entry); keep {
				kept = append(kept, remaining)
			}
		}
		if len(kept) == 0 {
			delete(hooks, event)
			continue
		}
		hooks[event] = kept
	}
}

// retractManagedHandlers returns entry without its managed handlers and whether
// anything is left to keep. A value that is not an entry object holding a
// handler array is not this writer's to change and comes back unchanged, and
// so does an entry that holds no managed handler, including an empty one.
func retractManagedHandlers(entry any) (any, bool) {
	object, ok := entry.(map[string]any)
	if !ok {
		return entry, true
	}
	handlers, ok := object["hooks"].([]any)
	if !ok {
		return entry, true
	}
	kept := make([]any, 0, len(handlers))
	for _, handler := range handlers {
		if !isManagedClaudeHandler(handler) {
			kept = append(kept, handler)
		}
	}
	switch len(kept) {
	case len(handlers):
		return entry, true
	case 0:
		return nil, false
	}
	pruned := maps.Clone(object)
	pruned["hooks"] = kept
	return pruned, true
}

func isManagedClaudeHandler(handler any) bool {
	object, ok := handler.(map[string]any)
	if !ok {
		return false
	}
	command, ok := object["command"].(string)
	return ok && isManagedClaudeHookCommand(command)
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
	if isStickyCommand(trimmed) {
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
