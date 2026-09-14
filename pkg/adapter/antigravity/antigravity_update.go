// Package antigravity implements the Antigravity CLI platform adapter.
package antigravity

import (
	"context"
	"errors"
	"fmt"
	"os"
	"path/filepath"

	"github.com/insajin/autopus-adk/pkg/adapter"
	"github.com/insajin/autopus-adk/pkg/config"
)

// Update applies incremental changes to an existing installation.
// Falls back to Generate when no manifest exists.
func (a *Adapter) Update(_ context.Context, cfg *config.HarnessConfig) (*adapter.PlatformFiles, error) {
	oldManifest, err := adapter.LoadManifest(a.root, adapterName)
	if err != nil {
		return nil, fmt.Errorf("매니페스트 로드 실패: %w", err)
	}
	if oldManifest == nil {
		oldManifest, err = adapter.LoadManifest(a.root, legacyAdapterName)
		if err != nil {
			return nil, fmt.Errorf("legacy 매니페스트 로드 실패: %w", err)
		}
	}

	newFiles, err := a.prepareFiles(cfg)
	if err != nil {
		return nil, err
	}

	plan, pf := a.buildUpdateTransactionPlan(oldManifest, newFiles)
	rollbackHooks, err := applyAntigravityManagedHookAssets(a.root, antigravityManagedHookAssets(pf.Files))
	if err != nil {
		return nil, err
	}
	if _, err := adapter.ApplyTransaction(a.root, adapterName, plan); err != nil {
		return nil, errors.Join(err, rollbackHooks())
	}

	return pf, nil
}

// prepareFiles prepares the same files as Generate but without writing to disk.
func (a *Adapter) prepareFiles(cfg *config.HarnessConfig) ([]adapter.FileMapping, error) {
	var files []adapter.FileMapping

	geminiMD, err := a.injectMarkerSection(cfg)
	if err != nil {
		return nil, fmt.Errorf("GEMINI.md 마커 주입 실패: %w", err)
	}
	files = append(files, adapter.FileMapping{
		TargetPath:      "GEMINI.md",
		OverwritePolicy: adapter.OverwriteMarker,
		Checksum:        checksum(geminiMD),
		Content:         []byte(geminiMD),
	})

	pluginFiles, err := prepareAntigravityPluginJSON()
	if err != nil {
		return nil, fmt.Errorf("antigravity plugin manifest 생성 실패: %w", err)
	}
	files = append(files, pluginFiles...)

	skillMappings, err := a.prepareSkillMappings(cfg)
	if err != nil {
		return nil, err
	}

	// Extended skills from content/skills/ via transformer
	extSkillMappings, err := a.renderExtendedSkills(cfg)
	if err != nil {
		return nil, fmt.Errorf("extended skill 준비 실패: %w", err)
	}
	extMirrors := mirrorAntigravityPluginMappings(extSkillMappings)
	files = append(files, mergeSkillMappings(skillMappings, append(extSkillMappings, extMirrors...))...)

	cmdMappings, err := a.prepareCommandMappings(cfg)
	if err != nil {
		return nil, err
	}
	files = append(files, cmdMappings...)

	ruleMappings, err := a.prepareRuleMappings(cfg)
	if err != nil {
		return nil, err
	}
	files = append(files, ruleMappings...)

	if cfg.IsFullMode() {
		agentMappings, err := a.prepareAgentMappings()
		if err != nil {
			return nil, err
		}
		files = append(files, agentMappings...)
	}

	completionHookAssets, err := prepareAntigravityCompletionHookAssets()
	if err != nil {
		return nil, err
	}
	files = append(files, completionHookAssets...)

	settingsMappings, err := a.generateSettingsWithHooks(cfg)
	if err != nil {
		return nil, err
	}
	files = append(files, settingsMappings...)

	routerMappings, err := a.prepareRouterCommand(cfg)
	if err != nil {
		return nil, err
	}
	files = append(files, routerMappings...)

	statusFiles, err := a.prepareStatusline()
	if err != nil {
		return nil, err
	}
	files = append(files, statusFiles...)

	hookMappings, err := a.prepareAntigravityHooksJSON(a.configuredAntigravityHooks(cfg))
	if err != nil {
		return nil, err
	}
	files = append(files, hookMappings...)

	resourceFiles, err := preparePluginSkillResources(files, cfg)
	if err != nil {
		return nil, err
	}
	files = append(files, resourceFiles...)

	return sanitizeUnsupportedClaudeTeamMappings(files), nil
}

func (a *Adapter) buildUpdateTransactionPlan(
	oldManifest *adapter.Manifest,
	newFiles []adapter.FileMapping,
) (adapter.TransactionPlan, *adapter.PlatformFiles) {
	finalFiles := make([]adapter.FileMapping, 0, len(newFiles))
	writes := make([]adapter.TransactionWrite, 0, len(newFiles))
	for _, file := range newFiles {
		if !isAntigravityManagedHookAsset(file) {
			action := adapter.ResolveAction(a.root, file.TargetPath, file.OverwritePolicy, oldManifest)
			if action == adapter.ActionSkip {
				continue
			}
			writes = append(writes, adapter.TransactionWrite{
				Path:    file.TargetPath,
				Content: file.Content,
				Perm:    antigravityFileMode(file.TargetPath),
			})
		}
		finalFiles = append(finalFiles, file)
	}

	// @AX:NOTE [AUTO]: File-count-only checksum is manifest bookkeeping, not a content integrity or tamper-detection mechanism.
	pf := &adapter.PlatformFiles{
		Files:    finalFiles,
		Checksum: checksum(fmt.Sprintf("%d", len(finalFiles))),
	}
	diff := adapter.BuildManifestDiff(oldManifest, newFiles, PruneRoots())
	diff.Prune = retainUserEditedPrunes(a.root, diff.Prune)

	return adapter.TransactionPlan{
		Writes:   writes,
		Removes:  adapter.TransactionRemovesFromManifestDiff(diff, false),
		Manifest: adapter.ManifestFromFiles(adapterName, pf),
	}, pf
}

// PruneRoots lists the trees where a path this adapter previously recorded may
// be deleted once it stops being generated. Ownership still comes from the
// manifest — nothing outside the recorded set is ever considered — so shared
// roots such as .agents/skills stay untouched even when they sit under a
// listed ancestor. `.agents/commands` and the plugin tree are listed because
// both once held Antigravity output that `agy` never reads.
func PruneRoots() []string {
	return []string{
		".gemini/skills/autopus",
		".gemini/commands",
		".gemini/rules/autopus",
		".gemini/agents/autopus",
		antigravityPluginDir,
		".agents/commands",
	}
}

// retainUserEditedPrunes drops obsolete paths whose bytes no longer match what
// this adapter wrote. A file the user edited is theirs; the transaction journal
// is a rollback buffer, not a durable backup, so deleting such a file would
// lose the edit at the next transaction.
func retainUserEditedPrunes(root string, entries []adapter.ManifestDiffEntry) []adapter.ManifestDiffEntry {
	kept := make([]adapter.ManifestDiffEntry, 0, len(entries))
	for _, entry := range entries {
		data, err := os.ReadFile(filepath.Join(root, filepath.FromSlash(entry.Path)))
		if err != nil {
			if os.IsNotExist(err) {
				kept = append(kept, entry)
			}
			continue
		}
		if adapter.Checksum(string(data)) != entry.OldChecksum {
			continue
		}
		kept = append(kept, entry)
	}
	return kept
}

func antigravityManagedHookAssets(files []adapter.FileMapping) []adapter.FileMapping {
	assets := make([]adapter.FileMapping, 0, len(files))
	for _, file := range files {
		if isAntigravityManagedHookAsset(file) {
			assets = append(assets, file)
		}
	}
	return assets
}

func antigravityFileMode(path string) os.FileMode {
	clean := filepath.ToSlash(path)
	if filepath.Ext(clean) == ".sh" {
		return 0755
	}
	return 0644
}
