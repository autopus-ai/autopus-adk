package codex

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"strings"
)

func cleanCodexHooksFile(path string) error {
	data, err := os.ReadFile(path)
	if os.IsNotExist(err) {
		return nil
	}
	if err != nil {
		return err
	}
	var doc hooksDoc
	if err := json.Unmarshal(data, &doc); err != nil {
		return fmt.Errorf("hooks JSON 파싱 실패: %w", err)
	}
	for category, groups := range doc.Hooks {
		kept := make(hookGroups, 0, len(groups))
		for _, group := range groups {
			if group.Autopus {
				continue
			}
			handlers := make(hookHandlers, 0, len(group.Hooks))
			for _, handler := range group.Hooks {
				if !isAutopusHookHandler(handler) {
					handlers = append(handlers, handler)
				}
			}
			if len(handlers) == 0 {
				continue
			}
			group.Hooks = handlers
			kept = append(kept, group)
		}
		doc.Hooks[category] = kept
	}
	cleaned, err := json.MarshalIndent(doc, "", "  ")
	if err != nil {
		return err
	}
	return os.WriteFile(path, append(cleaned, '\n'), 0o644)
}

func cleanCodexMarketplaceFile(path string) error {
	data, err := os.ReadFile(path)
	if os.IsNotExist(err) {
		return nil
	}
	if err != nil {
		return err
	}
	var doc map[string]json.RawMessage
	if err := json.Unmarshal(data, &doc); err != nil {
		return fmt.Errorf("marketplace JSON 파싱 실패: %w", err)
	}
	var plugins []json.RawMessage
	if raw := doc["plugins"]; len(raw) > 0 {
		if err := json.Unmarshal(raw, &plugins); err != nil {
			return fmt.Errorf("marketplace plugins 파싱 실패: %w", err)
		}
	}
	kept := plugins[:0]
	for _, plugin := range plugins {
		if isAutopusMarketplaceEntry(plugin) {
			continue
		}
		kept = append(kept, plugin)
	}
	encodedPlugins, err := json.Marshal(kept)
	if err != nil {
		return err
	}
	doc["plugins"] = encodedPlugins
	cleaned, err := json.MarshalIndent(doc, "", "  ")
	if err != nil {
		return err
	}
	return os.WriteFile(path, append(cleaned, '\n'), 0o644)
}

func isAutopusMarketplaceEntry(raw json.RawMessage) bool {
	var entry struct {
		Name   string `json:"name"`
		Source struct {
			Path string `json:"path"`
		} `json:"source"`
	}
	if json.Unmarshal(raw, &entry) != nil {
		return false
	}
	return entry.Name == "auto" && filepath.Clean(entry.Source.Path) == filepath.Clean(".autopus/plugins/auto")
}
func mergeCodexMarketplace(existing, rendered []byte) ([]byte, error) {
	var base map[string]json.RawMessage
	if err := json.Unmarshal(existing, &base); err != nil {
		return nil, fmt.Errorf("existing marketplace JSON 파싱 실패: %w", err)
	}
	var generated map[string]json.RawMessage
	if err := json.Unmarshal(rendered, &generated); err != nil {
		return nil, fmt.Errorf("generated marketplace JSON 파싱 실패: %w", err)
	}
	var existingPlugins, generatedPlugins []json.RawMessage
	if raw := base["plugins"]; len(raw) > 0 {
		if err := json.Unmarshal(raw, &existingPlugins); err != nil {
			return nil, fmt.Errorf("existing marketplace plugins 파싱 실패: %w", err)
		}
	}
	if err := json.Unmarshal(generated["plugins"], &generatedPlugins); err != nil || len(generatedPlugins) != 1 {
		return nil, fmt.Errorf("generated marketplace plugin entry가 올바르지 않음")
	}
	kept := make([]json.RawMessage, 0, len(existingPlugins)+1)
	for _, plugin := range existingPlugins {
		var identity struct {
			Name string `json:"name"`
		}
		if json.Unmarshal(plugin, &identity) != nil || identity.Name != "auto" {
			kept = append(kept, plugin)
		}
	}
	kept = append(kept, generatedPlugins[0])
	plugins, err := json.Marshal(kept)
	if err != nil {
		return nil, err
	}
	base["plugins"] = plugins
	for key, value := range generated {
		if _, exists := base[key]; !exists {
			base[key] = value
		}
	}
	return json.MarshalIndent(base, "", "  ")
}

func cleanCodexConfigFile(path string) error {
	data, err := os.ReadFile(path)
	if os.IsNotExist(err) {
		return nil
	}
	if err != nil {
		return err
	}
	if err := validateCodexTOMLStructure(string(data)); err != nil {
		return fmt.Errorf("config TOML 파싱 실패: %w", err)
	}
	cleaned := removeAutopusCodexConfig(string(data))
	return os.WriteFile(path, []byte(cleaned), 0o644)
}

func removeAutopusCodexConfig(content string) string {
	lines := strings.Split(strings.TrimSuffix(content, "\n"), "\n")
	result := make([]string, 0, len(lines))
	section := ""
	chunkStart := 0
	chunkHasUserValue := false

	flushEmptyManagedSection := func() {
		if section == "" || !isAutopusOnlyCodexSection(section) || chunkHasUserValue {
			return
		}
		kept := result[:chunkStart]
		for _, line := range result[chunkStart:] {
			trimmed := strings.TrimSpace(line)
			if trimmed != "" && !strings.HasPrefix(trimmed, "#") {
				continue
			}
			kept = append(kept, line)
		}
		result = kept
	}

	for _, line := range lines {
		trimmed := strings.TrimSpace(line)
		if next, ok := parseStructuralCodexSection(trimmed); ok {
			flushEmptyManagedSection()
			section = next
			chunkStart = len(result)
			chunkHasUserValue = false
			result = append(result, line)
			continue
		}
		key, _, assignment := parseCodexConfigAssignment(trimmed)
		if assignment &&
			(codexManagedConfigKeys[section][key] || codexObsoleteConfigKeys[section][key]) {
			continue
		}
		if section == "" && (trimmed == codexGeneratedConfigHeader || strings.HasPrefix(trimmed, "# Project:")) {
			continue
		}
		if assignment {
			chunkHasUserValue = true
		}
		result = append(result, line)
	}
	flushEmptyManagedSection()
	return strings.TrimSpace(strings.Join(result, "\n")) + "\n"
}

// isAutopusOnlyCodexSection names the sections whose header Clean removes once
// no user assignment is left in them. [agents] qualifies because Autopus is
// what creates it: a user's own scalars there keep the header (they count as
// user values) and per-role [agents.<name>] tables are separate sections.
func isAutopusOnlyCodexSection(section string) bool {
	return section == "features.multi_agent_v2" ||
		section == "agents" ||
		section == `plugins."browser@openai-bundled"` ||
		section == "mcp_servers.context7"
}
