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

func isAutopusHookHandler(handler hookHandler) bool {
	if handler.Autopus || handler.StatusMessage == autopusHookStatusMessage {
		return true
	}
	command := strings.TrimSpace(handler.Command)
	return command == "auto check --hygiene --arch --quiet --staged --warn-only" ||
		command == "auto react check --quiet" ||
		strings.Contains(command, "/.codex/hooks/autopus/hook-codex-") ||
		strings.Contains(command, ".claude/hooks/autopus/hook-codex-") ||
		adapter.IsStaleCompletionHookCommand(adapterName, command)
}
