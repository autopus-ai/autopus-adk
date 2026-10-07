package claude

import (
	"os"
	"path"
	"path/filepath"
	"sort"
	"strings"

	"github.com/insajin/autopus-adk/pkg/adapter"
	"github.com/insajin/autopus-adk/pkg/rulecond"
)

const (
	// legacyClaudeSkillDir is the retired flat skill layout. The native layout
	// is `.claude/skills/<name>/SKILL.md`, and claude_agent_content.go already
	// treats the flat one as legacy: legacyClaudeSkillReference rewrites every
	// `.claude/skills/autopus/<name>.md` mention onto the native path. That
	// migration rewrote references but never deleted the referenced files, so
	// the flat copies outlived every reinstall.
	legacyClaudeSkillDir = ".claude/skills/autopus"
	// nativeClaudeSkillFile is the native layout's own file name. A skill
	// literally named `autopus` lives at `.claude/skills/autopus/SKILL.md`, and
	// the retired flat layout never produced that name, so it is excluded to
	// keep a user-authored skill out of the removal set.
	nativeClaudeSkillFile = "SKILL.md"
	claudeMarkdownSuffix  = ".md"
)

// obsoleteClaudeSurfacePaths returns every retired Claude-managed path still
// present under root, as repo-relative slash paths in sorted order.
//
// It is the single source of truth for the `auto doctor` report
// (validateObsoleteClaudeSurface) and the update prune
// (buildUpdateTransactionPlan), so a surface the doctor calls obsolete is
// exactly a surface the next update deletes.
//
// The set is closed. It is deliberately NOT "everything the manifest does not
// claim": `.autopus/*-manifest.json` is gitignored, so a fresh clone has no
// ownership record at all and an open-ended rule would delete user files. Both
// members below are the deletion half of a migration whose reference half
// already landed in code.
//
// The retired orchestra completion hooks under `.claude/hooks/autopus/`,
// including the orphan `hook-opencode-complete.ts`, are not members here. They
// are group S of SPEC-PANERM-001 (adapter.StaleCompletionHookScripts): the
// update deletes them through adapter.StaleCompletionScriptRemoves, which
// keeps a script that a settings file or `opencode.json` still names until the
// transaction that retracts that reference, and the doctor reports them as
// stale completion hooks rather than as an obsolete surface.
func obsoleteClaudeSurfacePaths(root string) ([]string, error) {
	paths := legacyFlatClaudeSkillPaths(root)
	relocated, err := relocatedClaudeRulePaths(root)
	if err != nil {
		return nil, err
	}
	paths = append(paths, relocated...)
	sort.Strings(paths)
	return paths, nil
}

// legacyFlatClaudeSkillPaths returns the flat `*.md` children of the retired
// skill directory. Only regular markdown files count, so a nested directory a
// user placed there survives.
func legacyFlatClaudeSkillPaths(root string) []string {
	entries, err := os.ReadDir(filepath.Join(root, filepath.FromSlash(legacyClaudeSkillDir)))
	if err != nil {
		return nil
	}
	paths := make([]string, 0, len(entries))
	for _, entry := range entries {
		if entry.IsDir() || entry.Name() == nativeClaudeSkillFile {
			continue
		}
		if strings.HasSuffix(entry.Name(), claudeMarkdownSuffix) {
			paths = append(paths, legacyClaudeSkillDir+"/"+entry.Name())
		}
	}
	return paths
}

// relocatedClaudeRulePaths returns the stale baseline copy of every hook-fired
// rule body SPEC-CONDRULE-001 relocated out of rulecond.ClaudeRulesRelDir into
// rulecond.BodyRootRelPath.
//
// The set is derived from the compiler's own output rather than from a literal
// name list: a relocated body is emitted at `BodyRootRelPath/<name>.md` and its
// pre-relocation twin was `ClaudeRulesRelDir/<name>.md` (compile_claude.go
// bodyMapping / ruleFileMapping share that `<name>`). Reading the compilation
// keeps this from drifting when a rule is reclassified — a rule that goes back
// to `always` or `paths-scoped` stops producing a body and therefore stops
// being listed here, which is required because the baseline copy becomes live
// again in that case.
func relocatedClaudeRulePaths(root string) ([]string, error) {
	surface, err := claudeConditionalRules()
	if err != nil {
		return nil, err
	}
	bodyPrefix := rulecond.BodyRootRelPath + "/"
	var paths []string
	for _, mapping := range surface.mappings {
		target := filepath.ToSlash(mapping.TargetPath)
		if !strings.HasPrefix(target, bodyPrefix) {
			continue
		}
		stale := rulecond.ClaudeRulesRelDir + "/" + path.Base(target)
		if info, statErr := os.Stat(filepath.Join(root, filepath.FromSlash(stale))); statErr != nil ||
			!info.Mode().IsRegular() {
			continue
		}
		paths = append(paths, stale)
	}
	return paths, nil
}

// validateObsoleteClaudeSurface reports every retired Claude surface the
// detector names, mirroring validateObsoleteCodexSurface so `auto doctor`
// surfaces a Claude orphan the same way it already surfaces a Codex one.
func (a *Adapter) validateObsoleteClaudeSurface(errs *[]adapter.ValidationError) error {
	paths, err := obsoleteClaudeSurfacePaths(a.root)
	if err != nil {
		return err
	}
	for _, path := range paths {
		*errs = append(*errs, adapter.ValidationError{
			File: path, Message: "obsolete Claude managed surface가 남아 있음", Level: "error",
		})
	}
	return nil
}
