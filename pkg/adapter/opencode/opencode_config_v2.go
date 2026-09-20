package opencode

import (
	"fmt"
	"net/url"
	"path/filepath"
	"strings"
)

// Native V2 values take precedence; legacy tuples remain intact when selected.
func effectivePluginConfig(doc map[string]any, v2 bool) (string, []any, error) {
	if value, ok := doc["plugins"]; ok {
		if !v2 {
			return "", nil, fmt.Errorf("native V2 plugins require OpenCode V2")
		}
		if entries, ok := value.([]any); ok {
			if err := validatePluginEntries(entries, true); err == nil {
				return "plugins", entries, nil
			}
		}
		// An invalid native value is not safe for the adapter to rewrite silently.
		return "", nil, fmt.Errorf("invalid native plugins array")
	}
	if value, ok := doc["plugin"]; ok {
		entries, ok := value.([]any)
		if !ok {
			return "", nil, fmt.Errorf("invalid legacy plugin array")
		}
		if err := validatePluginEntries(entries, false); err != nil {
			return "", nil, err
		}
		return "plugin", entries, nil
	}
	if v2 {
		return "plugins", []any{}, nil
	}
	return "plugin", []any{}, nil
}
func pluginEntryPath(entry any) string {
	switch value := entry.(type) {
	case string:
		return value
	case []any:
		if len(value) > 0 {
			path, _ := value[0].(string)
			return path
		}
	case map[string]any:
		path, _ := value["package"].(string)
		return path
	}
	return ""
}
func validatePluginEntries(entries []any, native bool) error {
	for _, entry := range entries {
		path := pluginEntryPath(entry)
		if strings.TrimSpace(path) == "" {
			return fmt.Errorf("invalid plugin entry")
		}
		switch value := entry.(type) {
		case string:
		case []any:
			if native || len(value) != 2 {
				return fmt.Errorf("invalid plugin tuple")
			}
			if _, ok := value[1].(map[string]any); !ok {
				return fmt.Errorf("invalid plugin tuple options")
			}
		case map[string]any:
			if !native {
				return fmt.Errorf("native plugin object in legacy array")
			}
			if options, exists := value["options"]; exists {
				if _, ok := options.(map[string]any); !ok {
					return fmt.Errorf("invalid plugin options")
				}
			}
		default:
			return fmt.Errorf("invalid plugin entry")
		}
	}
	return nil
}
func pluginPathIdentity(path, root string) string {
	if strings.HasPrefix(path, "file:") {
		u, err := url.Parse(path)
		if err != nil || u.Host != "" || u.RawQuery != "" || u.Fragment != "" {
			return path
		}
		path = u.Path
	}
	if filepath.IsAbs(path) {
		return filepath.Clean(path)
	}
	if strings.HasPrefix(path, "./") || strings.HasPrefix(path, "../") || strings.HasPrefix(path, ".opencode/") {
		joined := filepath.Join(root, filepath.FromSlash(path))
		if absolute, err := filepath.Abs(joined); err == nil {
			return filepath.Clean(absolute)
		}
		return filepath.Clean(joined)
	}
	return path
}
func managedEntry(path, target, root string) bool {
	return !strings.HasPrefix(path, "-") && pluginPathIdentity(path, root) == pluginPathIdentity(target, root)
}
func pluginControlMatches(value, target, root string) bool {
	if value == "*" {
		return true
	}
	autopus := strings.HasSuffix(target, "/autopus-hooks.js")
	if autopus && value == "autopus.hooks" {
		return true
	}
	if autopus && strings.HasSuffix(value, ".*") && strings.HasPrefix("autopus.hooks", strings.TrimSuffix(value, "*")) {
		return true
	}
	return managedEntry(value, target, root)
}
func pluginRegistrationState(entries []any, target, root string) (present, enabled, disabled bool) {
	for _, entry := range entries {
		path := pluginEntryPath(entry)
		if strings.HasPrefix(path, "-") {
			if pluginControlMatches(strings.TrimPrefix(path, "-"), target, root) {
				enabled = false
				disabled = true
			}
			continue
		}
		if managedEntry(path, target, root) {
			present = true
			enabled = true
			disabled = false
			continue
		}
		if pluginControlMatches(path, target, root) {
			enabled = true
			disabled = false
		}
	}
	return
}
func managedPluginRegistered(doc map[string]any, target string, v2 bool, root string) bool {
	_, entries, err := effectivePluginConfig(doc, v2)
	if err != nil {
		return false
	}
	present, enabled, _ := pluginRegistrationState(entries, target, root)
	return present && enabled
}
func mergePluginConfig(doc map[string]any, managed []string, v2 bool, root string) error {
	key, entries, err := effectivePluginConfig(doc, v2)
	if err != nil {
		return err
	}
	entries = append([]any(nil), entries...)
	for _, target := range managed {
		// Retain the last managed declaration (including its options) and controls.
		last := -1
		for i, entry := range entries {
			if managedEntry(pluginEntryPath(entry), target, root) {
				last = i
			}
		}
		kept := []any{}
		for i, entry := range entries {
			if i != last && managedEntry(pluginEntryPath(entry), target, root) {
				continue
			}
			if i == last && v2 {
				entry = normalizeManagedEntry(entry)
			}
			kept = append(kept, entry)
		}
		entries = kept
		present, _, disabled := pluginRegistrationState(entries, target, root)
		if !present && !disabled {
			path := target
			if v2 {
				path = normalizeManagedPath(path)
			}
			entries = append(entries, path)
		}
		if key == "plugins" {
			if legacy, ok := doc["plugin"].([]any); ok {
				keptLegacy := []any{}
				for _, entry := range legacy {
					if !managedEntry(pluginEntryPath(entry), target, root) {
						keptLegacy = append(keptLegacy, entry)
					}
				}
				doc["plugin"] = keptLegacy
			}
		}
	}
	doc[key] = entries
	return nil
}
func normalizeManagedPath(path string) string {
	if strings.HasPrefix(path, ".opencode/") {
		return "./" + path
	}
	return path
}
func normalizeManagedEntry(entry any) any {
	switch value := entry.(type) {
	case string:
		return normalizeManagedPath(value)
	case []any:
		copy := append([]any(nil), value...)
		copy[0] = normalizeManagedPath(pluginEntryPath(value))
		return copy
	case map[string]any:
		copy := map[string]any{}
		for k, v := range value {
			copy[k] = v
		}
		copy["package"] = normalizeManagedPath(pluginEntryPath(value))
		return copy
	}
	return entry
}
