// Package antigravity provides settings.json management for Antigravity CLI.
package antigravity

import (
	"context"
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"

	"github.com/insajin/autopus-adk/pkg/adapter"
	"github.com/insajin/autopus-adk/pkg/config"
	"github.com/insajin/autopus-adk/pkg/content"
	"github.com/insajin/autopus-adk/templates"
)

// generateSettings renders settings.json.tmpl and returns a file mapping.
func (a *Adapter) generateSettings(cfg *config.HarnessConfig) ([]adapter.FileMapping, error) {
	newSettings, err := a.renderAuthoredSettings(cfg)
	if err != nil {
		return nil, err
	}

	settingsPath := filepath.Join(a.root, ".gemini", "settings.json")
	if data, err := os.ReadFile(settingsPath); err == nil {
		var existing map[string]any
		if err := json.Unmarshal(data, &existing); err == nil {
			newSettings = mergeSettingsMaps(existing, newSettings)
		}
	}

	out, err := json.MarshalIndent(newSettings, "", "  ")
	if err != nil {
		return nil, fmt.Errorf("gemini settings JSON 직렬화 실패: %w", err)
	}
	outStr := string(out) + "\n"

	return []adapter.FileMapping{{
		TargetPath:      filepath.Join(".gemini", "settings.json"),
		OverwritePolicy: adapter.OverwriteMerge,
		Checksum:        checksum(outStr),
		Content:         []byte(outStr),
	}}, nil
}

func (a *Adapter) renderAuthoredSettings(cfg *config.HarnessConfig) (map[string]any, error) {
	raw, err := templates.FS.ReadFile("gemini/settings/settings.json.tmpl")
	if err != nil {
		return nil, err
	}
	rendered, err := a.engine.RenderString(string(raw), cfg)
	if err != nil {
		return nil, err
	}
	var settings map[string]any
	if err := json.Unmarshal([]byte(rendered), &settings); err != nil {
		return nil, err
	}
	return settings, nil
}

func (a *Adapter) generateSettingsWithHooks(cfg *config.HarnessConfig) ([]adapter.FileMapping, error) {
	files, err := a.generateSettings(cfg)
	if err != nil || len(files) == 0 {
		return files, err
	}

	settings := make(map[string]any)
	if err := json.Unmarshal(files[0].Content, &settings); err != nil {
		return nil, fmt.Errorf("gemini settings JSON 파싱 실패: %w", err)
	}
	perms := filterUnsupportedAntigravityPermissions(content.DetectPermissions(a.root, cfg.Hooks.Permissions))
	applyAntigravityHooksAndPermissions(settings, a.configuredLegacyGeminiHooks(cfg), perms)
	return buildAntigravitySettingsMapping(settings)
}

// InstallHooks merges hooks and permissions into .gemini/settings.json.
func (a *Adapter) InstallHooks(_ context.Context, hooks []adapter.HookConfig, perms *adapter.PermissionSet) error {
	settingsDir := filepath.Join(a.root, ".gemini")
	if err := os.MkdirAll(settingsDir, 0755); err != nil {
		return fmt.Errorf("gemini 설정 디렉터리 생성 실패: %w", err)
	}

	settingsPath := filepath.Join(settingsDir, "settings.json")

	settings := readAntigravitySettings(settingsPath)
	applyAntigravityHooksAndPermissions(settings, hooks, filterUnsupportedAntigravityPermissions(perms))
	files, err := buildAntigravitySettingsMapping(settings)
	if err != nil {
		return err
	}
	files = sanitizeUnsupportedClaudeTeamMappings(files)
	return adapter.WriteFileIfChanged(settingsPath, files[0].Content, 0644)
}

func filterUnsupportedAntigravityPermissions(perms *adapter.PermissionSet) *adapter.PermissionSet {
	if perms == nil {
		return nil
	}
	filter := func(values []string) []string {
		out := make([]string, 0, len(values))
		for _, value := range values {
			switch value {
			case "TeamCreate", "TeamDelete", "SendMessage":
				continue
			default:
				out = append(out, value)
			}
		}
		return out
	}
	return &adapter.PermissionSet{
		Allow: filter(perms.Allow),
		Deny:  filter(perms.Deny),
	}
}

func applyAntigravityHooksAndPermissions(settings map[string]any, hooks []adapter.HookConfig, perms *adapter.PermissionSet) {
	if len(hooks) > 0 {
		removeAuthoredLegacyHooks(settings, append(defaultLegacyHookConfigs(), hooks...))
		hooksMap, _ := settings["hooks"].(map[string]any)
		if hooksMap == nil {
			hooksMap = make(map[string]any)
		}

		for _, h := range hooks {
			entry := map[string]any{
				"matcher": h.Matcher,
				"hooks": []any{map[string]any{
					"type":    h.Type,
					"command": h.Command,
					"timeout": h.Timeout,
				}},
			}
			entries, _ := hooksMap[h.Event].([]any)
			hooksMap[h.Event] = append(entries, entry)
		}
		settings["hooks"] = hooksMap
	}

	if perms != nil && (len(perms.Allow) > 0 || len(perms.Deny) > 0) {
		existingPerms, _ := settings["permissions"].(map[string]any)
		permMap := make(map[string]any)
		for k, v := range existingPerms {
			permMap[k] = v
		}
		if len(perms.Allow) > 0 {
			existing := toStringSlice(permMap["allow"])
			permMap["allow"] = mergeUnique(existing, perms.Allow)
		}
		if len(perms.Deny) > 0 {
			existing := toStringSlice(permMap["deny"])
			permMap["deny"] = mergeUnique(existing, perms.Deny)
		}
		settings["permissions"] = permMap
	}
}

func buildAntigravitySettingsMapping(settings map[string]any) ([]adapter.FileMapping, error) {
	out, err := json.MarshalIndent(settings, "", "  ")
	if err != nil {
		return nil, fmt.Errorf("gemini settings.json 직렬화 실패: %w", err)
	}
	content := append(out, '\n')
	return []adapter.FileMapping{{
		TargetPath:      filepath.Join(".gemini", "settings.json"),
		OverwritePolicy: adapter.OverwriteMerge,
		Checksum:        checksum(string(content)),
		Content:         content,
	}}, nil
}

func readAntigravitySettings(path string) map[string]any {
	data, err := os.ReadFile(path)
	if err != nil {
		return make(map[string]any)
	}
	var settings map[string]any
	if err := json.Unmarshal(data, &settings); err != nil {
		return make(map[string]any)
	}
	return settings
}

// mergeSettingsMaps merges new settings into existing, preserving user keys.
func mergeSettingsMaps(existing, newSettings map[string]any) map[string]any {
	for k, v := range newSettings {
		if existingSub, ok := existing[k].(map[string]any); ok {
			if newSub, ok := v.(map[string]any); ok {
				existing[k] = mergeSettingsMaps(existingSub, newSub)
				continue
			}
		}
		existing[k] = v
	}
	return existing
}

func toStringSlice(v any) []string {
	arr, ok := v.([]any)
	if !ok {
		return nil
	}
	out := make([]string, 0, len(arr))
	for _, item := range arr {
		if s, ok := item.(string); ok {
			out = append(out, s)
		}
	}
	return out
}

func mergeUnique(base, add []string) []string {
	seen := make(map[string]bool, len(base))
	for _, s := range base {
		seen[s] = true
	}
	result := append([]string{}, base...)
	for _, s := range add {
		if !seen[s] {
			result = append(result, s)
			seen[s] = true
		}
	}
	return result
}
