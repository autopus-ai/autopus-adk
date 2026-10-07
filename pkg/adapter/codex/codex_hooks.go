package codex

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"

	"github.com/insajin/autopus-adk/pkg/adapter"
	"github.com/insajin/autopus-adk/pkg/config"
	"github.com/insajin/autopus-adk/pkg/content"
)

// generateHooks renders hooks.json template and merges with existing user hooks.
// Legacy Autopus hooks are recognized by their marker or managed command;
// user hooks are preserved during merge. Generated JSON contains only fields
// from the official Codex hook schema.
func (a *Adapter) generateHooks(cfg *config.HarnessConfig) ([]adapter.FileMapping, error) {
	rendered, err := a.renderHooksTemplate(cfg)
	if err != nil {
		return nil, err
	}

	targetPath := filepath.Join(a.root, ".codex", "hooks.json")
	merged, err := mergeHooks(targetPath, rendered)
	if err != nil {
		return nil, err
	}

	if err := writeCodexManagedFile(a.root, filepath.Join(".codex", "hooks.json"), merged, 0o644); err != nil {
		return nil, fmt.Errorf("codex hooks.json 쓰기 실패: %w", err)
	}

	return []adapter.FileMapping{{
		TargetPath:      filepath.Join(".codex", "hooks.json"),
		OverwritePolicy: adapter.OverwriteMerge,
		Checksum:        checksum(string(merged)),
		Content:         merged,
	}}, nil
}

// prepareHooksFile returns hooks.json file mapping without writing to disk.
// Uses merge policy to preserve user hooks on application.
func (a *Adapter) prepareHooksFile(cfg *config.HarnessConfig) ([]adapter.FileMapping, error) {
	rendered, err := a.renderHooksTemplate(cfg)
	if err != nil {
		return nil, err
	}

	targetPath := filepath.Join(a.root, ".codex", "hooks.json")
	merged, err := mergeHooks(targetPath, rendered)
	if err != nil {
		return nil, err
	}

	return []adapter.FileMapping{{
		TargetPath:      filepath.Join(".codex", "hooks.json"),
		OverwritePolicy: adapter.OverwriteMerge,
		Checksum:        checksum(string(merged)),
		Content:         merged,
	}}, nil
}

func (a *Adapter) prepareGitHookFiles(cfg *config.HarnessConfig) ([]adapter.FileMapping, error) {
	_, gitHooks, err := content.GenerateProjectHookConfigs(cfg, adapterName, false)
	if err != nil {
		return nil, fmt.Errorf("git hooks 생성 실패: %w", err)
	}

	files := make([]adapter.FileMapping, 0, len(gitHooks))
	for _, gh := range gitHooks {
		files = append(files, adapter.FileMapping{
			TargetPath:      gh.Path,
			OverwritePolicy: adapter.OverwriteAlways,
			Checksum:        checksum(gh.Content),
			Content:         []byte(gh.Content),
		})
	}

	return adapter.FilterUnsupportedRootGitHookFiles(a.root, files), nil
}

// renderHooksTemplate renders the codex hooks.json template.
func (a *Adapter) renderHooksTemplate(cfg *config.HarnessConfig) (string, error) {
	hooks, _, err := content.GenerateProjectHookConfigs(cfg, adapterName, true)
	if err != nil {
		return "", fmt.Errorf("codex hooks 생성 실패: %w", err)
	}

	doc := hooksDoc{Hooks: make(map[string]hookGroups)}
	for _, hook := range hooks {
		doc.Hooks[hook.Event] = append(doc.Hooks[hook.Event], hookGroup{
			Matcher: hook.Matcher,
			Hooks: hookHandlers{{
				Type:          hook.Type,
				Command:       hook.Command,
				Timeout:       hook.Timeout,
				Env:           hook.Env,
				StatusMessage: autopusHookStatusMessage,
			}},
		})
	}

	rendered, err := json.MarshalIndent(doc, "", "  ")
	if err != nil {
		return "", fmt.Errorf("codex hooks JSON 직렬화 실패: %w", err)
	}
	return string(rendered), nil
}

// mergeHooks reads existing hooks.json from disk, preserves user hooks, and
// upserts Autopus-managed hooks from the rendered template.
func mergeHooks(existingPath, rendered string) ([]byte, error) {
	// Parse rendered autopus hooks and stamp them with marker
	var autopusDoc hooksDoc
	if err := json.Unmarshal([]byte(rendered), &autopusDoc); err != nil {
		return nil, fmt.Errorf("rendered hooks JSON 파싱 실패: %w", err)
	}
	stampAutopusMarker(&autopusDoc)

	// Read existing file — if missing or invalid, use autopus-only result
	if info, err := os.Lstat(existingPath); err == nil && info.Mode()&os.ModeSymlink != 0 {
		return nil, fmt.Errorf("existing hooks JSON is a symlink: %s", existingPath)
	} else if err != nil && !os.IsNotExist(err) {
		return nil, fmt.Errorf("existing hooks JSON 검사 실패: %w", err)
	}

	existingData, err := os.ReadFile(existingPath)
	if err != nil {
		if os.IsNotExist(err) {
			return json.MarshalIndent(autopusDoc, "", "  ")
		}
		return nil, fmt.Errorf("existing hooks JSON 읽기 실패: %w", err)
	}

	var existingDoc hooksDoc
	if err := json.Unmarshal(existingData, &existingDoc); err != nil {
		return nil, fmt.Errorf("existing hooks JSON 파싱 실패: %w", err)
	}

	// Merge each hook category: keep user hooks, upsert autopus hooks
	merged := mergeHookCategories(existingDoc, autopusDoc)
	return json.MarshalIndent(merged, "", "  ")
}

// installGitHooks generates and writes git hooks as fallback.
func (a *Adapter) installGitHooks(cfg *config.HarnessConfig) error {
	files, err := a.prepareGitHookFiles(cfg)
	if err != nil {
		return err
	}

	for _, file := range files {
		ghPath := filepath.Join(a.root, file.TargetPath)
		if err := os.MkdirAll(filepath.Dir(ghPath), 0755); err != nil {
			return fmt.Errorf("git hook 디렉터리 생성 실패: %w", err)
		}
		if err := os.WriteFile(ghPath, file.Content, 0755); err != nil {
			return fmt.Errorf("git hook 쓰기 실패 %s: %w", file.TargetPath, err)
		}
	}
	return nil
}
