package omp

import "github.com/insajin/autopus-adk/pkg/config"

// PruneRoots are the compiler-owned directories omp may delete stale
// manifest-recorded files from. It is the single source of truth for Update,
// Clean, and the `auto update` preview, which must agree: a preview that
// promises a prune the apply path cannot perform leaves orphaned files behind
// forever, because the manifest is rewritten either way and never proposes the
// prune again.
//
// Pruning is additionally limited to paths recorded in the previous manifest
// (see adapter.BuildManifestDiff and Clean), so a file omp never wrote is never
// eligible: antigravity's .agents/commands/*.toml survives because omp never
// records it, and so does every file under .omp/rules that lacks the
// ompRuleFilePrefix namespace — generated rules share that directory with the
// user's own rules, and only the prefixed files omp recorded are ever pruned.
//
// Eligibility is decided per surface, not per operation. .agents/commands stays
// listed even after omp yields the command surface, because opencode serves
// commands from .opencode/commands/ — nothing ever overwrites the .md files omp
// wrote under .agents/commands, so dropping the root would strand all of them.
// .agents/skills is the opposite case: OpenCode owns that shared surface in
// mixed installations, so OMP must yield it instead of pruning an in-place file.
// @AX:ANCHOR [AUTO]: Public prune-root contract shared by update, clean, preview, and manifest reconciliation.
// @AX:REASON [AUTO]: Removing or narrowing this function can orphan generated OMP surfaces or delete another platform's ownership.
func PruneRoots(cfg *config.HarnessConfig) []string {
	roots := ompExclusivePruneRoots()
	if cfg != nil && !ompConfigHasPlatform(cfg, "opencode") {
		roots = append(roots, ".agents/skills")
	}
	return roots
}

// ompExclusivePruneRoots are the surfaces omp owns no matter which other
// platforms are active. Clean falls back to this set when the harness config
// cannot be read: without the platform list, assuming OMP still owns the shared
// skill surface could delete OpenCode's live `.agents/skills` files. Losing
// another platform's live file is worse than leaving one legacy OMP file behind.
func ompExclusivePruneRoots() []string {
	return []string{
		ompRuleDir,
		".omp/agents",
		".omp/skills",
		".omp/commands",
		ompStopGateTarget,
		ompContextBridgeTarget,
		ompNativePipelineRouteTarget,
		DefaultOMPModelOverlayPath,
		OMPModelReceiptRelativePath,
		OMPModelProjectOwnershipRelativePath,

		// Legacy manifest-owned surfaces remain prune-eligible during cutover.
		// BuildManifestDiff and Clean still require an old manifest entry, so
		// user-authored files under these roots are never selected.
		".agents/rules/autopus",
		".agents/commands",
		configFile,
	}
}

func ompConfigHasPlatform(cfg *config.HarnessConfig, name string) bool {
	for _, platform := range cfg.Platforms {
		if platform == name {
			return true
		}
	}
	return false
}
