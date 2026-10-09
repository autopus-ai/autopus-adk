package claude

import (
	"context"
	"fmt"
	"path/filepath"

	"github.com/insajin/autopus-adk/pkg/adapter"
	"github.com/insajin/autopus-adk/pkg/config"
	"github.com/insajin/autopus-adk/pkg/rulecond"
)

// PruneRoots are the compiler-owned directories an update may delete stale
// managed files from. It is the single source of truth for both the apply path
// below and the `auto update` preview, which must agree: a preview that
// promises a prune the apply path cannot perform leaves orphaned files behind
// forever, because the manifest is rewritten either way and never proposes the
// prune again.
//
// Only compiler-owned directories belong here. Pruning is additionally limited
// to paths recorded in the previous manifest (see BuildManifestDiff), so a file
// the user created in one of these directories is never eligible.
//
// The rule directory and the conditional body root are both listed because
// SPEC-CONDRULE-001 moves a hook-fired rule body between them: relocating out
// of `.claude/rules/autopus` must delete the stale baseline copy, and a rule
// reclassified back to `always` must delete the stale relocated body.
func PruneRoots() []string {
	return []string{
		".claude/skills",
		".claude/agents/autopus",
		".claude/workflows",
		".claude/commands",
		".git/hooks",
		rulecond.ClaudeRulesRelDir,
		rulecond.BodyRootRelPath,
	}
}

// Update는 매니페스트 기반으로 파일을 업데이트한다.
// 사용자가 수정한 파일은 백업 후 덮어쓰고, 삭제한 파일은 재생성하지 않는다.
func (a *Adapter) Update(_ context.Context, cfg *config.HarnessConfig) (*adapter.PlatformFiles, error) {
	oldManifest, err := adapter.LoadManifest(a.root, adapterName)
	if err != nil {
		return nil, fmt.Errorf("매니페스트 로드 실패: %w", err)
	}

	newFiles, err := a.prepareFiles(cfg)
	if err != nil {
		return nil, err
	}
	hookFiles, err := a.prepareHooksAndPermissionsFiles(cfg)
	if err != nil {
		return nil, err
	}
	newFiles = append(newFiles, hookFiles...)

	plan, pf, err := a.buildUpdateTransactionPlan(oldManifest, newFiles)
	if err != nil {
		return nil, err
	}
	if _, err := adapter.ApplyTransaction(a.root, adapterName, plan); err != nil {
		return nil, err
	}

	return pf, nil
}

func (a *Adapter) buildUpdateTransactionPlan(
	oldManifest *adapter.Manifest,
	newFiles []adapter.FileMapping,
) (adapter.TransactionPlan, *adapter.PlatformFiles, error) {
	finalFiles := make([]adapter.FileMapping, 0, len(newFiles))
	skippedPaths := make([]string, 0)

	for _, file := range newFiles {
		action := adapter.ResolveAction(a.root, file.TargetPath, file.OverwritePolicy, oldManifest)
		if action == adapter.ActionSkip {
			skippedPaths = append(skippedPaths, file.TargetPath)
			continue
		}
		finalFiles = append(finalFiles, file)
	}

	pf := &adapter.PlatformFiles{
		Files:    finalFiles,
		Checksum: checksum(fmt.Sprintf("%d", len(finalFiles))),
	}
	manifest := adapter.ManifestFromFiles(adapterName, pf)
	if oldManifest != nil {
		for _, path := range skippedPaths {
			if prev, ok := oldManifest.Files[path]; ok {
				manifest.Files[path] = prev
			}
		}
	}
	diff := adapter.BuildManifestDiff(oldManifest, newFiles, PruneRoots())
	writes := adapter.TransactionWritesFromFiles(finalFiles, claudeFileMode)
	removes, err := a.buildUpdateTransactionRemoves(diff, writes)
	if err != nil {
		return adapter.TransactionPlan{}, nil, err
	}

	return adapter.TransactionPlan{
		Writes:   writes,
		Removes:  removes,
		Manifest: manifest,
	}, pf, nil
}

// buildUpdateTransactionRemoves plans every deletion an update performs.
//
// Three sources feed it. The manifest diff catches any managed file
// the previous install recorded and this one no longer emits, which is the wide
// net but only exists while a manifest survives. The obsolete-surface detector
// catches a closed, hardcoded set of retired layouts regardless of the manifest,
// which is what makes a fresh clone converge: `.autopus/*-manifest.json` is
// gitignored while the generated files are committed, so a cloned workspace has
// orphans with no ownership record and a manifest-only prune is structurally
// blind to them — it rewrites the manifest without them and never proposes the
// prune again, making the orphan permanent. The group S declaration
// (adapter.StaleCompletionHookScripts) adds the retired completion hook
// scripts, minus any that the transaction's final state still names, so the
// transaction that retracts a handler is the one that deletes its script.
func (a *Adapter) buildUpdateTransactionRemoves(
	diff adapter.ManifestDiff,
	writes []adapter.TransactionWrite,
) ([]adapter.TransactionRemove, error) {
	removes := adapter.TransactionRemovesFromManifestDiff(diff, false)
	planned := make(map[string]bool, len(removes))
	for _, remove := range removes {
		planned[filepath.ToSlash(filepath.Clean(remove.Path))] = true
	}

	obsolete, err := obsoleteClaudeSurfacePaths(a.root)
	if err != nil {
		return nil, err
	}
	for _, path := range obsolete {
		if planned[path] {
			continue
		}
		planned[path] = true
		removes = append(removes, adapter.TransactionRemove{Path: path, Recursive: true})
	}
	scripts := adapter.StaleCompletionHookScripts(adapterName)
	for _, remove := range adapter.StaleCompletionScriptRemoves(a.root, scripts, writes) {
		if !planned[remove.Path] {
			planned[remove.Path] = true
			removes = append(removes, remove)
		}
	}
	return adapter.FilterUnsupportedRootGitHookRemoves(a.root, removes), nil
}
