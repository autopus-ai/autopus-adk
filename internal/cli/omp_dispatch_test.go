package cli

import (
	"context"
	"path/filepath"
	"strings"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/insajin/autopus-adk/pkg/adapter"
	"github.com/insajin/autopus-adk/pkg/adapter/omp"
	"github.com/insajin/autopus-adk/pkg/config"
)

// ompDispatchConfig builds a full-mode config for the given platform list.
func ompDispatchConfig(platforms ...string) *config.HarnessConfig {
	cfg := config.DefaultFullConfig("omp-dispatch")
	cfg.Platforms = platforms
	return cfg
}

// TestGeneratePreviewMappings_OMPResolvesToAdapter covers S8/REQ-010 at the
// `auto update --preview` dispatch site: omp must reach its adapter instead of
// the unknown-platform branch that every other identifier table already avoids.
func TestGeneratePreviewMappings_OMPResolvesToAdapter(t *testing.T) {
	t.Parallel()

	files, err := generatePreviewMappings(context.Background(), t.TempDir(), ompDispatchConfig("omp"), "omp")
	require.NoError(t, err)
	require.NotEmpty(t, files, "omp preview must produce mappings")

	surfaces := map[string]int{}
	for _, file := range files {
		target := filepath.ToSlash(file.TargetPath)
		switch {
		case strings.HasPrefix(target, ".omp/rules/autopus-"):
			surfaces["rules"]++
		case strings.HasPrefix(target, ".omp/agents/"):
			surfaces["agents"]++
		case strings.HasPrefix(target, ".omp/commands/"):
			surfaces["commands"]++
		}
	}
	assert.Equal(t, 14, surfaces["rules"], "omp preview must plan 14 rules")
	assert.Equal(t, 0, surfaces["agents"], "omp reuses its bundled agents, so none are planned")
	assert.Equal(t, 20, surfaces["commands"], "omp preview must plan 20 commands")

	_, unknownErr := generatePreviewMappings(context.Background(), t.TempDir(), ompDispatchConfig("omp"), "not-a-platform")
	require.Error(t, unknownErr, "the unknown-platform branch must still reject a bogus identifier")
	assert.Contains(t, unknownErr.Error(), "알 수 없는 플랫폼")
}

// TestPreviewPruneRoots_MatchOMPApplyPath is the omp half of the B1 symmetry
// invariant. previewPruneRoots previously restated a hardcoded omp root list
// that neither matched nor tracked the adapter, so `auto update --plan`
// announced prunes the apply path could not perform. The manifest is rewritten
// either way, so those files became permanent orphans no later run proposed
// again.
func TestPreviewPruneRoots_MatchOMPApplyPath(t *testing.T) {
	t.Parallel()

	for _, platforms := range [][]string{
		{"omp"},
		{"opencode", "omp"},
		{"codex", "omp"},
		{"antigravity-cli", "omp"},
	} {
		cfg := ompDispatchConfig(platforms...)
		assert.Equal(t, omp.PruneRoots(cfg), previewPruneRoots("omp", cfg),
			"preview must announce exactly the prunes the apply path performs for %v", platforms)
	}
}

// TestPreviewAndApplyComputeSameOMPPruneSet exercises the invariant through the
// shared diff builder rather than through the root lists alone, and pins the
// ownership-transition oracle: once opencode takes the command surface, the
// .md commands omp wrote under .agents/commands are orphans (opencode serves
// commands from .opencode/commands/), while .agents/skills/auto/SKILL.md now
// belongs to opencode and must survive.
//
// The legacy .agents/rules/autopus/branding.md entry pins the upgrade path:
// rules now live at .omp/rules/autopus-<name>.md, so a path a pre-relocation
// manifest recorded is no longer emitted and must be pruned. That only works
// while the legacy directory stays in the prune root list, which is why
// relocation adds .omp/rules instead of replacing .agents/rules/autopus.
func TestPreviewAndApplyComputeSameOMPPruneSet(t *testing.T) {
	t.Parallel()

	cfg := ompDispatchConfig("opencode", "omp")
	oldManifest := &adapter.Manifest{Platform: "omp", Files: map[string]adapter.ManifestFile{
		".omp/rules/autopus-branding.md":    {Checksum: "kept", Policy: adapter.OverwriteAlways},
		".agents/rules/autopus/branding.md": {Checksum: "legacy", Policy: adapter.OverwriteAlways},
		".agents/commands/auto.md":          {Checksum: "stale", Policy: adapter.OverwriteAlways},
		".agents/commands/auto-plan.md":     {Checksum: "stale", Policy: adapter.OverwriteAlways},
		".agents/skills/auto/SKILL.md":      {Checksum: "yielded", Policy: adapter.OverwriteAlways},
		"AGENTS.md":                         {Checksum: "outside", Policy: adapter.OverwriteMarker},
	}}
	newFiles := []adapter.FileMapping{{
		TargetPath:      ".omp/rules/autopus-branding.md",
		OverwritePolicy: adapter.OverwriteAlways,
		Checksum:        "kept",
	}}

	previewPrune := prunePaths(adapter.BuildManifestDiff(oldManifest, newFiles, previewPruneRoots("omp", cfg)))
	applyPrune := prunePaths(adapter.BuildManifestDiff(oldManifest, newFiles, omp.PruneRoots(cfg)))

	assert.Equal(t, previewPrune, applyPrune, "preview and apply prune sets must match")
	assert.Equal(t, []string{
		".agents/commands/auto-plan.md",
		".agents/commands/auto.md",
		".agents/rules/autopus/branding.md",
	}, applyPrune)
	assert.NotContains(t, applyPrune, ".agents/skills/auto/SKILL.md",
		"a surface omp yielded in place must never be pruned by omp")
	assert.NotContains(t, applyPrune, "AGENTS.md",
		"a managed file outside the compiler-owned roots is never pruned")
	assert.NotContains(t, applyPrune, ".omp/rules/autopus-branding.md",
		"a rule the current run still emits is never pruned")
}
