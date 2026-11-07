package codex

import (
	"os"
	"path/filepath"
	"sort"
	"strings"
)

// obsoleteCodexSurfacePaths returns every retired Codex-managed path that is
// still present under root, as repo-relative slash paths in sorted order.
//
// It is the single source of truth for two consumers that must agree: the
// `auto doctor` report (validateObsoleteCodexSurface) and the update prune
// (buildUpdateTransactionRemoves). A surface the doctor names as obsolete is
// exactly a surface the next update deletes, so the report cannot promise a
// cleanup the installer never performs.
//
// The returned set is closed and hardcoded. It is deliberately NOT "everything
// the manifest does not claim": a manifest is absent on a fresh clone, so an
// open-ended rule would delete user-authored files. Each rule below names a
// layout the current generator provably never emits — see
// codex_latest_v2_test.go, which asserts no emitted path is a flat
// `.codex/skills/*.md`, lives under `.codex/rules/`, or starts with
// `.agents/skills/`.
func obsoleteCodexSurfacePaths(root string, openCodeOwnsSharedSkills bool) []string {
	var paths []string
	paths = append(paths, flatCodexSkillPaths(root)...)
	if info, err := os.Stat(filepath.Join(root, ".codex", "prompts")); err == nil && info.IsDir() {
		paths = append(paths, ".codex/prompts")
	}
	paths = append(paths, markdownCodexRulePaths(root, ".codex/rules")...)
	// `.agents/skills/<workflow>` is a shared surface: when OpenCode is
	// installed it owns those directories, and removing them would delete a
	// live surface of another platform.
	if !openCodeOwnsSharedSkills {
		for _, name := range allWorkflowNames() {
			legacy := ".agents/skills/" + name
			if _, err := os.Stat(filepath.Join(root, filepath.FromSlash(legacy))); err == nil {
				paths = append(paths, legacy)
			}
		}
	}
	sort.Strings(paths)
	return paths
}

// flatCodexSkillPaths returns the pre-native flat skill files that sit directly
// under `.codex/skills`. The native layout is `.codex/skills/codex-<name>/SKILL.md`
// (validateNativeCodexSkill pins it), so a regular `*.md` child of the skills
// root can only be a leftover of the retired flat layout. Directory children
// are skipped, which is what keeps a user-authored `.codex/skills/mine/SKILL.md`
// out of the set.
func flatCodexSkillPaths(root string) []string {
	entries, err := os.ReadDir(filepath.Join(root, ".codex", "skills"))
	if err != nil {
		return nil
	}
	paths := make([]string, 0, len(entries))
	for _, entry := range entries {
		if !entry.IsDir() && strings.HasSuffix(entry.Name(), ".md") {
			paths = append(paths, ".codex/skills/"+entry.Name())
		}
	}
	return paths
}

// markdownCodexRulePaths walks the retired `.codex/rules` tree and returns every
// markdown file in it. The whole subtree is retired: Codex rules are carried by
// the native skill surface now, so the generator emits nothing under it.
func markdownCodexRulePaths(root, relative string) []string {
	entries, err := os.ReadDir(filepath.Join(root, filepath.FromSlash(relative)))
	if err != nil {
		return nil
	}
	var paths []string
	for _, entry := range entries {
		child := relative + "/" + entry.Name()
		if entry.IsDir() {
			paths = append(paths, markdownCodexRulePaths(root, child)...)
			continue
		}
		if strings.HasSuffix(entry.Name(), ".md") {
			paths = append(paths, child)
		}
	}
	return paths
}
