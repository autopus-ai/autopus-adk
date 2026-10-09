package codex

import (
	"encoding/json"
	"strings"

	"github.com/insajin/autopus-adk/pkg/adapter"
)

const autopusHookStatusMessage = "Running Autopus hook"

// hooksDoc represents the top-level hooks.json structure.
type hooksDoc struct {
	Description string                     `json:"description,omitempty"`
	Hooks       map[string]hookGroups      `json:"hooks"`
	Extra       map[string]json.RawMessage `json:"-"`
}

// hookGroup is Codex's event-level matcher group. Command handlers must be
// nested under hooks; legacy flat entries are accepted by UnmarshalJSON and
// normalized into this shape during the next merge.
type hookGroup struct {
	Matcher string                     `json:"matcher,omitempty"`
	Hooks   hookHandlers               `json:"hooks"`
	Extra   map[string]json.RawMessage `json:"-"`
	Autopus bool                       `json:"-"`
}

type hookHandler struct {
	Type                string                     `json:"type,omitempty"`
	Command             string                     `json:"command"`
	CommandWindows      string                     `json:"command_windows,omitempty"`
	CommandWindowsCamel string                     `json:"commandWindows,omitempty"`
	Timeout             int                        `json:"timeout,omitempty"`
	Env                 map[string]string          `json:"env,omitempty"`
	StatusMessage       string                     `json:"statusMessage,omitempty"`
	Async               *bool                      `json:"async,omitempty"`
	Extra               map[string]json.RawMessage `json:"-"`
	Autopus             bool                       `json:"-"`
}

// hookGroups ensures nil event groups serialize as [] rather than null.
type hookGroups []hookGroup

func (g hookGroups) MarshalJSON() ([]byte, error) {
	if g == nil {
		return []byte("[]"), nil
	}

	type alias hookGroups
	return json.Marshal(alias(g))
}

type hookHandlers []hookHandler

func (h hookHandlers) MarshalJSON() ([]byte, error) {
	if h == nil {
		return []byte("[]"), nil
	}
	type alias hookHandlers
	return json.Marshal(alias(h))
}

// stampAutopusMarker marks all hooks in the document as Autopus-managed.
func stampAutopusMarker(doc *hooksDoc) {
	for cat, entries := range doc.Hooks {
		for i := range entries {
			entries[i].Autopus = true
			for handlerIndex := range entries[i].Hooks {
				entries[i].Hooks[handlerIndex].StatusMessage = autopusHookStatusMessage
			}
		}
		doc.Hooks[cat] = entries
	}
}

// mergeHookCategories merges existing and autopus hook documents.
// User hooks (Autopus==false) are preserved; autopus hooks are replaced.
func mergeHookCategories(existing, autopus hooksDoc) hooksDoc {
	result := hooksDoc{
		Description: existing.Description,
		Hooks:       make(map[string]hookGroups),
		Extra:       mergeJSONExtras(autopus.Extra, existing.Extra),
	}
	if result.Description == "" {
		result.Description = autopus.Description
	}

	cats := make(map[string]bool)
	for category := range existing.Hooks {
		cats[category] = true
	}
	for category := range autopus.Hooks {
		cats[category] = true
	}

	for category := range cats {
		merged := make(hookGroups, 0, len(existing.Hooks[category])+len(autopus.Hooks[category]))
		for _, group := range existing.Hooks[category] {
			if group.Autopus {
				continue
			}
			keptHandlers := make(hookHandlers, 0, len(group.Hooks))
			hadManagedHandler := false
			for _, handler := range group.Hooks {
				if isAutopusHookHandler(handler) {
					hadManagedHandler = true
					continue
				}
				keptHandlers = append(keptHandlers, handler)
			}
			if hadManagedHandler && len(keptHandlers) == 0 {
				continue
			}
			group.Hooks = keptHandlers
			merged = append(merged, group)
		}
		merged = append(merged, autopus.Hooks[category]...)
		result.Hooks[category] = merged
	}

	return result
}

// isAutopusHookHandler reports whether a hooks.json handler is Autopus-owned.
// A handler that names a group S completion script (SPEC-PANERM-001) is owned
// exactly when its command is a launch generation wrote, the predicate `auto
// doctor` reports by, whatever marker it carries: a user command that runs the
// script and then its own work keeps both. Other handlers are owned by the
// Autopus marker or status message, which also covers shapes an earlier
// release generated, or by a current generated command line, but never when
// the command chains further shell work; only the edit-guard line carries
// operators of its own.
func isAutopusHookHandler(handler hookHandler) bool {
	command := strings.TrimSpace(handler.Command)
	if namesStaleCompletionScript(command) {
		return adapter.IsStaleCompletionHookCommand(adapterName, command) || command == legacyCodexStopHookCommand
	}
	switch {
	case strings.HasPrefix(command, codexEditGuardCommandPrefix):
		// The edit-guard command line (SPEC-EDITGUARD-001) stays managed even
		// without its status message, so regeneration never runs it twice.
		return true
	case adapter.HasShellOperator(command):
		return false
	}
	return handler.Autopus || handler.StatusMessage == autopusHookStatusMessage ||
		command == "auto check --hygiene --arch --quiet --staged --warn-only" ||
		command == "auto react check --quiet"
}

// legacyCodexStopHookCommand is the Stop command Codex generation registered
// while its completion hook still lived in the Claude hook directory, before
// the Codex adapter installed its own copy under .codex/hooks/autopus/.
const legacyCodexStopHookCommand = ".claude/hooks/autopus/hook-codex-stop.sh"

// namesStaleCompletionScript reports whether command mentions the path of any
// platform's group S script.
func namesStaleCompletionScript(command string) bool {
	for _, script := range adapter.AllStaleCompletionHookScripts() {
		if strings.Contains(command, script) {
			return true
		}
	}
	return false
}

// codexEditGuardCommandPrefix anchors the registered edit-guard command line.
const codexEditGuardCommandPrefix = "out=$(auto guard edit "
