package codex

import (
	"fmt"
	"strings"

	"github.com/insajin/autopus-adk/pkg/codexruntime"
)

var codexManagedConfigKeys = map[string]map[string]bool{
	"": {
		"model": true, "model_reasoning_effort": true, "model_reasoning_summary": true,
		"model_verbosity": true, "approval_policy": true, "sandbox_mode": true,
		"web_search": true, "project_doc_max_bytes": true,
	},
	"features": {
		"goals": true, "hooks": true, "shell_tool": true, "unified_exec": true,
	},
	// The undocumented features.multi_agent_v2 ceiling stays managed even though
	// no render emits it any more: that is what drops it from a file an older
	// Autopus wrote, leaving the documented [agents] key as the only ceiling.
	"features.multi_agent_v2": {
		"enabled": true, codexruntime.AgentConcurrencyKey: true,
	},
	"agents":                           {codexruntime.AgentConcurrencyKey: true},
	`plugins."browser@openai-bundled"`: {"enabled": true},
	"mcp_servers.context7":             {"command": true, "args": true},
}

// max_threads is the documented legacy alias of the key Autopus emits, so
// carrying both would leave two ceilings that can disagree in one file with no
// documented winner. max_depth and job_max_runtime_seconds are keys the old
// Autopus template wrote and no render emits; codex-cli 0.153.4 still parses
// them, so they are removed as stale harness output rather than as dead keys.
var codexObsoleteConfigKeys = map[string]map[string]bool{
	"agents": {
		codexruntime.LegacyAgentConcurrencyAliasKey: true, "max_depth": true, "job_max_runtime_seconds": true,
	},
}

type codexConfigEntry struct {
	key   string
	value string
}

// @AX:WARN [AUTO]: structural Codex config merge contains more than eight conditional branches.
// @AX:REASON [AUTO]: multiline TOML state, obsolete-key removal, managed-key replacement, duplicate suppression, comment preservation, and missing-section insertion converge here.
func mergeCodexConfig(existing, rendered string) (string, error) {
	if strings.TrimSpace(existing) == "" {
		return rendered, nil
	}
	if err := validateCodexTOMLStructure(existing); err != nil {
		return "", fmt.Errorf("existing Codex config TOML 파싱 실패: %w", err)
	}
	managed := collectManagedCodexConfig(rendered)
	seen := make(map[string]map[string]bool)
	lines := strings.Split(strings.TrimSuffix(existing, "\n"), "\n")
	result := make([]string, 0, len(lines)+24)
	section := ""
	var scan codexTOMLScanState

	appendMissing := func(name string) {
		entries := managed[name]
		for _, entry := range entries {
			if seen[name] != nil && seen[name][entry.key] {
				continue
			}
			result = append(result, entry.key+" = "+entry.value)
		}
	}

	for _, line := range lines {
		if scan.skipSyntaxLine(line) {
			result = append(result, line)
			continue
		}
		trimmed := strings.TrimSpace(line)
		if next, ok := parseStructuralCodexSection(trimmed); ok {
			appendMissing(section)
			section = next
			result = append(result, line)
			continue
		}
		key, value, assignment := parseCodexConfigAssignment(trimmed)
		if assignment {
			scan.observeValue(value)
		}
		if assignment && codexObsoleteConfigKeys[section][key] {
			continue
		}
		if assignment && codexManagedConfigKeys[section][key] {
			entry, ok := findManagedCodexEntry(managed[section], key)
			if !ok {
				continue
			}
			if seen[section] == nil {
				seen[section] = make(map[string]bool)
			}
			if seen[section][key] {
				continue
			}
			seen[section][key] = true
			result = append(result, replaceCodexConfigValuePreservingComment(line, entry.value))
			continue
		}
		result = append(result, line)
	}
	appendMissing(section)

	present := collectCodexSections(existing)
	for _, name := range managedCodexSectionOrder() {
		// A managed section the current render left empty must not be written:
		// a table Autopus no longer emits any key for would land as a bare,
		// meaningless header.
		if name == "" || present[name] || len(managed[name]) == 0 {
			continue
		}
		if len(result) > 0 && strings.TrimSpace(result[len(result)-1]) != "" {
			result = append(result, "")
		}
		result = append(result, "["+name+"]")
		for _, entry := range managed[name] {
			result = append(result, entry.key+" = "+entry.value)
		}
	}
	return strings.TrimRight(strings.Join(result, "\n"), "\n") + "\n", nil
}

func collectManagedCodexConfig(content string) map[string][]codexConfigEntry {
	result := make(map[string][]codexConfigEntry)
	section := ""
	for _, line := range strings.Split(content, "\n") {
		trimmed := strings.TrimSpace(line)
		if next, ok := parseStructuralCodexSection(trimmed); ok {
			section = next
			continue
		}
		key, value, ok := parseCodexConfigAssignment(trimmed)
		if !ok || !codexManagedConfigKeys[section][key] {
			continue
		}
		result[section] = append(result[section], codexConfigEntry{key: key, value: value})
	}
	return result
}

func managedCodexSectionOrder() []string {
	return []string{
		"", "features", "features.multi_agent_v2", "agents",
		`plugins."browser@openai-bundled"`, "mcp_servers.context7",
	}
}

func collectCodexSections(content string) map[string]bool {
	sections := map[string]bool{"": true}
	for _, line := range strings.Split(content, "\n") {
		if section, ok := parseStructuralCodexSection(strings.TrimSpace(line)); ok {
			sections[section] = true
		}
	}
	return sections
}

func findManagedCodexEntry(entries []codexConfigEntry, key string) (codexConfigEntry, bool) {
	for _, entry := range entries {
		if entry.key == key {
			return entry, true
		}
	}
	return codexConfigEntry{}, false
}

func replaceCodexConfigValuePreservingComment(line, value string) string {
	prefix, existingValue, ok := strings.Cut(line, "=")
	if !ok {
		return line
	}
	withoutComment := codexTOMLValueWithoutComment(existingValue)
	comment := existingValue[len(withoutComment):]
	if comment != "" && !strings.HasPrefix(comment, " ") {
		comment = " " + comment
	}
	return prefix + "= " + strings.TrimSpace(value) + comment
}

func parseStructuralCodexSection(line string) (string, bool) {
	header := strings.TrimSpace(codexTOMLValueWithoutComment(line))
	if strings.HasPrefix(header, "[[") && strings.HasSuffix(header, "]]") {
		name := strings.TrimSpace(header[2 : len(header)-2])
		return "[[" + name + "]]", name != ""
	}
	return parseCodexConfigSectionHeader(header)
}
