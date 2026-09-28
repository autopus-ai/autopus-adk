package claude

import (
	"context"
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"strings"

	"github.com/insajin/autopus-adk/pkg/adapter"
	"github.com/insajin/autopus-adk/pkg/config"
	"github.com/insajin/autopus-adk/pkg/content"
)

func (a *Adapter) prepareHooksAndPermissionsFiles(cfg *config.HarnessConfig) ([]adapter.FileMapping, error) {
	if err := adapter.RejectSymlinkComponents(a.root, filepath.Join(".claude", "settings.json")); err != nil {
		return nil, fmt.Errorf("settings.json 경로 확인 실패: %w", err)
	}
	a.statusLineMode = resolveStatusLineMode(cfg, InspectStatusLine(a.root))
	a.suppressCommitAttribution = commitAttributionForbidden(cfg)
	hookConfigs, gitHooks, err := content.GenerateProjectHookConfigs(cfg, "claude-code", a.SupportsHooks())
	if err != nil {
		return nil, fmt.Errorf("hook config 준비 실패: %w", err)
	}
	perms := content.DetectPermissions(a.root, cfg.Hooks.Permissions)
	settings, err := a.prepareSettingsMapping(hookConfigs, perms)
	if err != nil {
		return nil, fmt.Errorf("hooks/permissions 준비 실패: %w", err)
	}
	ledger, err := a.preparePermissionLedgerMapping(perms)
	if err != nil {
		return nil, fmt.Errorf("permission ledger 준비 실패: %w", err)
	}
	files := []adapter.FileMapping{settings, ledger}
	for _, gh := range gitHooks {
		files = append(files, adapter.FileMapping{
			TargetPath:      gh.Path,
			OverwritePolicy: adapter.OverwriteAlways,
			Checksum:        checksum(gh.Content),
			Content:         []byte(gh.Content),
		})
	}
	return files, nil
}

// InstallHooks는 .claude/settings.json에 훅과 권한을 Claude Code 중첩 스키마로 설치한다.
func (a *Adapter) InstallHooks(_ context.Context, hooks []adapter.HookConfig, perms *adapter.PermissionSet) error {
	mapping, err := a.prepareSettingsMapping(hooks, perms)
	if err != nil {
		return err
	}
	return writeClaudeMapping(a.root, mapping)
}

// @AX:WARN [AUTO]: settings projection contains more than eight conditional branches.
// @AX:REASON [AUTO]: malformed input rejection, user hook preservation, managed-hook retraction, permissions, and status-line precedence converge here.
func (a *Adapter) prepareSettingsMapping(hooks []adapter.HookConfig, perms *adapter.PermissionSet) (adapter.FileMapping, error) {
	var settings map[string]interface{}
	data, err := os.ReadFile(filepath.Join(a.root, ".claude", "settings.json"))
	switch {
	case err == nil:
		if err := json.Unmarshal(data, &settings); err != nil {
			return adapter.FileMapping{}, fmt.Errorf("settings.json 파싱 실패: %w", err)
		}
		if settings == nil {
			settings = make(map[string]interface{})
		}
	case os.IsNotExist(err):
		settings = make(map[string]interface{})
	default:
		return adapter.FileMapping{}, fmt.Errorf("settings.json 읽기 실패: %w", err)
	}

	// Retract stale managed entries before appending the desired set. Event keys
	// and entries owned by the user remain byte-semantically intact.
	hooksMap := make(map[string]any)
	if existing, exists := settings["hooks"]; exists {
		typed, ok := existing.(map[string]any)
		if !ok {
			return adapter.FileMapping{}, fmt.Errorf("settings.json hooks 필드는 객체여야 함")
		}
		for event, entries := range typed {
			hooksMap[event] = entries
		}
	}
	retractManagedHookEntries(hooksMap)
	for _, hook := range hooks {
		handler := map[string]any{
			"type":    hook.Type,
			"command": hook.Command,
		}
		if hook.Timeout > 0 {
			handler["timeout"] = hook.Timeout
		}
		entry := claudeHookEntry(handler, hook.Matcher)
		hooksMap[hook.Event] = appendHookEntry(hooksMap[hook.Event], entry)
	}
	if len(hooksMap) == 0 {
		delete(settings, "hooks")
	} else {
		settings["hooks"] = hooksMap
	}

	// Merge permissions: append autopus defaults to existing user permissions.
	if perms != nil && (len(perms.Allow) > 0 || len(perms.Deny) > 0) {
		existingPerms, _ := settings["permissions"].(map[string]any)
		permMap := make(map[string]any)
		for k, v := range existingPerms {
			permMap[k] = v
		}
		existingAllow := toStringSlice(permMap["allow"])
		filteredAllow := withoutObsoleteClaudePermissions(existingAllow)
		if len(perms.Allow) > 0 || len(filteredAllow) != len(existingAllow) {
			permMap["allow"] = mergeUnique(filteredAllow, perms.Allow)
		}
		if len(perms.Deny) > 0 {
			existing := toStringSlice(permMap["deny"])
			permMap["deny"] = mergeUnique(existing, perms.Deny)
		}
		settings["permissions"] = permMap
	}

	mode := a.statusLineMode
	if !mode.IsValid() {
		mode = resolveStatusLineMode(nil, statusLineStateFromValue(settings["statusLine"]))
	}
	switch mode {
	case config.StatusLineModeMerge:
		settings["statusLine"] = projectClaudeStatusLine(settings["statusLine"], defaultClaudeCombinedStatusLine())
	case config.StatusLineModeReplace:
		settings["statusLine"] = projectClaudeStatusLine(settings["statusLine"], defaultClaudeStatusLine())
	}

	if a.suppressCommitAttribution {
		settings["attribution"] = projectSuppressedCommitAttribution(settings["attribution"])
	}

	out, err := json.MarshalIndent(settings, "", "  ")
	if err != nil {
		return adapter.FileMapping{}, fmt.Errorf("settings.json 직렬화 실패: %w", err)
	}
	content := append(out, '\n')
	return adapter.FileMapping{
		TargetPath:      filepath.Join(".claude", "settings.json"),
		OverwritePolicy: adapter.OverwriteMerge,
		Checksum:        checksum(string(content)),
		Content:         content,
	}, nil
}

func writeClaudeMapping(root string, file adapter.FileMapping) error {
	targetPath := filepath.Join(root, file.TargetPath)
	if err := os.MkdirAll(filepath.Dir(targetPath), 0755); err != nil {
		return fmt.Errorf("디렉터리 생성 실패 %s: %w", filepath.Dir(targetPath), err)
	}
	if err := adapter.WriteFileIfChanged(targetPath, file.Content, claudeFileMode(file.TargetPath)); err != nil {
		return fmt.Errorf("파일 쓰기 실패 %s: %w", file.TargetPath, err)
	}
	return nil
}

func claudeFileMode(path string) os.FileMode {
	clean := filepath.ToSlash(path)
	if clean == claudePermissionLedgerPath {
		return 0600
	}
	if strings.HasPrefix(clean, ".git/hooks/") || strings.HasSuffix(clean, ".sh") {
		return 0755
	}
	return 0644
}

// toStringSlice converts an any (typically []any from JSON) to []string.
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

// mergeUnique appends items from add to base, skipping duplicates.
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

func withoutObsoleteClaudePermissions(values []string) []string {
	result := make([]string, 0, len(values))
	for _, value := range values {
		if value != "TeamCreate" && value != "TeamDelete" {
			result = append(result, value)
		}
	}
	return result
}

func defaultClaudeStatusLine() map[string]any {
	return map[string]any{
		"type":    "command",
		"command": autopusClaudeStatusLineCommand,
		"padding": 1,
	}
}

func defaultClaudeCombinedStatusLine() map[string]any {
	return map[string]any{
		"type":    "command",
		"command": autopusClaudeCombinedStatusLineCommand,
		"padding": 1,
	}
}
