package adapter

import (
	"bytes"
	"encoding/json"
	"os"
	"path"
	"path/filepath"
	"sort"
	"strings"
)

// staleCompletionHookScripts is group S of SPEC-PANERM-001: the orchestra
// completion and ready hook scripts that generation installed per platform
// until the pane backend was retired. The set is closed and keyed by path, not
// by manifest ownership, so a fresh clone whose gitignored manifest is gone
// converges too, and nothing outside it is ever deleted. Update retraction and
// the doctor report read the same declaration, so doctor names what update
// removes.
//
// OpenCode owns no script: its group S members are the opencode.json plugin
// entries that load one of these files (pkg/adapter/opencode).
var staleCompletionHookScripts = map[string][]string{
	// content/embed.go copied every group H .sh asset into the Claude hook
	// directory; the .ts is an orphan that only an out-of-band opencode.json
	// entry can load.
	"claude-code": {
		".claude/hooks/autopus/hook-claude-sessionstart.sh",
		".claude/hooks/autopus/hook-claude-stop.sh",
		".claude/hooks/autopus/hook-codex-sessionstart.sh",
		".claude/hooks/autopus/hook-codex-stop.sh",
		".claude/hooks/autopus/hook-gemini-afteragent.sh",
		".claude/hooks/autopus/hook-gemini-sessionstart.sh",
		".claude/hooks/autopus/hook-gemini-stop.sh",
		".claude/hooks/autopus/hook-opencode-complete.ts",
	},
	"codex": {
		".codex/hooks/autopus/hook-codex-sessionstart.sh",
		".codex/hooks/autopus/hook-codex-stop.sh",
	},
	"antigravity-cli": {
		".gemini/hooks/autopus/hook-gemini-afteragent.sh",
		".gemini/hooks/autopus/hook-gemini-stop.sh",
	},
}

// staleCompletionSettingsFiles are the project files whose content can point
// at a group S script: each platform's hook settings, the OpenCode config, and
// Claude Code's local settings, which Claude runs like the shared file but
// update never writes.
var staleCompletionSettingsFiles = []string{
	".claude/settings.json", ".claude/settings.local.json", ".codex/hooks.json", ".agents/hooks.json",
	".gemini/settings.json", "opencode.json",
}

// generatedHookLaunchers are the (prefix, suffix) forms that generation
// wrapped around a group S script path, the bare path included. Only these
// may hold shell operators; pkg/content generated the prefixed forms at B.
var generatedHookLaunchers = [][2]string{
	{"", ""},
	{`"${CLAUDE_PROJECT_DIR:-.}"/`, ""},
	{`"${GEMINI_PROJECT_DIR:-$(git rev-parse --show-toplevel 2>/dev/null || pwd)}"/`, ""},
	{`"$(cd .. && pwd)/`, `"`},
	{`"$(git rev-parse --show-toplevel 2>/dev/null || pwd)/`, `"`},
}

// StaleCompletionHookScripts returns the group S scripts of platform as sorted
// root-relative slash paths, or nil for a platform that owns none.
func StaleCompletionHookScripts(platform string) []string {
	scripts := append([]string(nil), staleCompletionHookScripts[platform]...)
	sort.Strings(scripts)
	return scripts
}

// AllStaleCompletionHookScripts returns every platform's group S scripts in
// sorted order: the targets an OpenCode plugin entry is matched against.
func AllStaleCompletionHookScripts() []string {
	var scripts []string
	for _, members := range staleCompletionHookScripts {
		scripts = append(scripts, members...)
	}
	sort.Strings(scripts)
	return scripts
}

// IsStaleCompletionHookCommand reports whether a settings handler command is
// nothing but a launch of one of platform's group S scripts. After surrounding
// blanks are trimmed, the command is either a generated launcher form around
// the script's root-relative path, or a single path word that ends in it at a
// path boundary (an absolute, $HOME, or quoted project-root prefix). A command
// that carries shell operators, an interpreter, or arguments does other work,
// so it is a user command and stays (its script stays with it).
func IsStaleCompletionHookCommand(platform, command string) bool {
	trimmed := strings.TrimSpace(command)
	for _, script := range staleCompletionHookScripts[platform] {
		for _, launcher := range generatedHookLaunchers {
			if trimmed == launcher[0]+script+launcher[1] {
				return true
			}
		}
	}
	if strings.ContainsAny(trimmed, ";&|<>`\n\t ") || strings.Contains(trimmed, "$(") {
		return false
	}
	unquoted := strings.TrimRight(trimmed, `"'`)
	for _, script := range staleCompletionHookScripts[platform] {
		rest, ok := strings.CutSuffix(unquoted, script)
		if ok && (rest == "" || strings.ContainsAny(rest[len(rest)-1:], `/"'`)) {
			return true
		}
	}
	return false
}

// RetractHookHandlers removes every handler whose command owned matches from a
// nested hook settings map ({event: [{matcher, hooks: [handler, ...]}]}) and
// returns how many it removed. The retraction unit is a handler: an entry is
// dropped only when no handler remains and an event only when no entry
// remains, so a user handler that shares an entry with a managed one keeps its
// command, matcher, timeout, and relative order. Values of any other shape
// stay untouched.
func RetractHookHandlers(hooks map[string]any, owned func(command string) bool) int {
	removed := 0
	for event, raw := range hooks {
		entries, ok := raw.([]any)
		if !ok {
			continue
		}
		kept := make([]any, 0, len(entries))
		for _, entry := range entries {
			next, count := retractEntryHandlers(entry, owned)
			removed += count
			if next != nil {
				kept = append(kept, next)
			}
		}
		if len(kept) == 0 {
			delete(hooks, event)
			continue
		}
		hooks[event] = kept
	}
	return removed
}

// retractEntryHandlers returns the entry without its owned handlers, nil when
// none remains, and the number removed.
func retractEntryHandlers(entry any, owned func(string) bool) (any, int) {
	object, ok := entry.(map[string]any)
	if !ok {
		return entry, 0
	}
	handlers := hookHandlerList(object["hooks"])
	kept := make([]any, 0, len(handlers))
	for _, handler := range handlers {
		fields, isObject := handler.(map[string]any)
		command, _ := fields["command"].(string)
		if !isObject || !owned(command) {
			kept = append(kept, handler)
		}
	}
	removed := len(handlers) - len(kept)
	switch {
	case removed == 0:
		return entry, 0
	case len(kept) == 0:
		return nil, removed
	}
	next := make(map[string]any, len(object))
	for key, value := range object {
		next[key] = value
	}
	next["hooks"] = kept
	return next, removed
}

// hookHandlerList accepts the decoded settings shape and the shape adapters
// build before serialization; any other value yields nil.
func hookHandlerList(raw any) []any {
	switch typed := raw.(type) {
	case []any:
		return typed
	case []map[string]any:
		handlers := make([]any, 0, len(typed))
		for _, handler := range typed {
			handlers = append(handlers, handler)
		}
		return handlers
	}
	return nil
}

// StaleCompletionScriptRemoves plans the removal of the members of scripts
// that exist under root as regular files reachable without a symlink. A script
// stays while the transaction's final state still names it, so no handler or
// plugin entry is left pointing at a deleted file: the content of a planned
// write, or the on-disk bytes of a settings file the plan does not rewrite,
// another platform's included (only the opencode transaction retracts an
// opencode.json entry). A settings file that cannot be read keeps every
// script. Whatever stays is still reported by doctor.
func StaleCompletionScriptRemoves(root string, scripts []string, writes []TransactionWrite) []TransactionRemove {
	texts, ok := finalStaleCompletionSettings(root, writes)
	if !ok {
		return nil
	}
	var removes []TransactionRemove
	for _, script := range PresentStaleCompletionScripts(root, scripts) {
		if !settingsNameScript(texts, script) {
			removes = append(removes, TransactionRemove{Path: script})
		}
	}
	return removes
}

// PresentStaleCompletionScripts returns, in input order, the members of
// scripts that exist under root as regular files whose path crosses no
// symlink.
func PresentStaleCompletionScripts(root string, scripts []string) []string {
	var present []string
	for _, script := range scripts {
		if RejectSymlinkComponents(root, filepath.FromSlash(script)) != nil {
			continue
		}
		info, err := os.Lstat(filepath.Join(root, filepath.FromSlash(script)))
		if err == nil && info.Mode().IsRegular() {
			present = append(present, script)
		}
	}
	return present
}

func finalStaleCompletionSettings(root string, writes []TransactionWrite) ([][]byte, bool) {
	planned := make(map[string][]byte, len(writes))
	for _, write := range writes {
		planned[filepath.ToSlash(filepath.Clean(write.Path))] = write.Content
	}
	texts := make([][]byte, 0, len(staleCompletionSettingsFiles))
	for _, rel := range staleCompletionSettingsFiles {
		if content, ok := planned[rel]; ok {
			texts = append(texts, content)
			continue
		}
		data, err := os.ReadFile(filepath.Join(root, filepath.FromSlash(rel)))
		switch {
		case err == nil:
			texts = append(texts, data)
		case !os.IsNotExist(err):
			return nil, false
		}
	}
	return texts, true
}

// settingsNameScript reports whether a settings text names script: a decoded
// JSON string that contains its root-relative path, or, for text that does
// not decode, its file name anywhere in the raw bytes. Matching the path, not
// the file name, keeps .codex/hooks.json naming the Codex copy of
// hook-codex-stop.sh from pinning the Claude copy.
func settingsNameScript(texts [][]byte, script string) bool {
	for _, text := range texts {
		var doc any
		if err := json.Unmarshal(text, &doc); err != nil {
			if bytes.Contains(text, []byte(path.Base(script))) {
				return true
			}
			continue
		}
		if jsonStringContains(doc, script) {
			return true
		}
	}
	return false
}

func jsonStringContains(value any, needle string) bool {
	switch typed := value.(type) {
	case string:
		return strings.Contains(typed, needle)
	case []any:
		for _, item := range typed {
			if jsonStringContains(item, needle) {
				return true
			}
		}
	case map[string]any:
		for key, item := range typed {
			if strings.Contains(key, needle) || jsonStringContains(item, needle) {
				return true
			}
		}
	}
	return false
}
