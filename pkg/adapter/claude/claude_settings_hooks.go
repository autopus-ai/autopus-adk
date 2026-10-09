package claude

import (
	"strings"

	"github.com/insajin/autopus-adk/pkg/adapter"
)

// managedClaudeHookInvocations are the `auto` commands generation registers.
// Each stays owned whatever arguments a later release appends to it, but not
// once the command chains further shell work (adapter.HasShellOperator).
var managedClaudeHookInvocations = []string{
	"auto check --hygiene --arch --quiet --staged --warn-only",
	"auto react check --quiet",
	"auto rules fire ",
}

// SPEC-EDITGUARD-001 REQ-EG-12: the guard's exit-masking command line, the one
// generated command whose own text chains operators. Anchored with its
// trailing space so a later release's flags stay owned while a command that
// merely mentions the guard does not.
const claudeEditGuardCommandPrefix = "out=$(auto guard edit "

// The TaskCreated hook (features.cc21) runs this script, bare in earlier
// releases and behind the default-mode assignment since then.
const (
	taskCreatedScript     = ".claude/hooks/task-created-validate.sh"
	taskCreatedModePrefix = "AUTOPUS_TASKCREATED_DEFAULT_MODE="
)

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

// isManagedClaudeHookCommand reports whether a settings handler command is one
// Autopus generated. Commands are matched against the shapes generation emits,
// never a path prefix: a user handler that runs a generated script or
// invocation and then its own work, or a user script placed under
// .claude/hooks/autopus/, does more than Autopus registered, and retracting it
// would delete that work and, once no settings file names it, the script too.
// The group S completion launchers are owned through the predicate `auto
// doctor` reports by, so doctor and update name the same handlers.
func isManagedClaudeHookCommand(command string) bool {
	trimmed := strings.TrimSpace(command)
	switch {
	case adapter.IsStaleCompletionHookCommand(adapterName, trimmed):
		return true
	case strings.HasPrefix(trimmed, claudeEditGuardCommandPrefix):
		return true
	case adapter.HasShellOperator(trimmed):
		return false
	case isStickyCommand(trimmed), isTaskCreatedCommand(trimmed):
		return true
	}
	for _, invocation := range managedClaudeHookInvocations {
		if strings.HasPrefix(trimmed, invocation) {
			return true
		}
	}
	return false
}

// isTaskCreatedCommand matches the TaskCreated hook command: the script alone,
// or the script after a single default-mode assignment.
func isTaskCreatedCommand(command string) bool {
	if command == taskCreatedScript {
		return true
	}
	assignment, ok := strings.CutSuffix(command, " "+taskCreatedScript)
	if !ok {
		return false
	}
	mode, ok := strings.CutPrefix(assignment, taskCreatedModePrefix)
	return ok && mode != "" && !strings.ContainsAny(mode, " \t\"'")
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
