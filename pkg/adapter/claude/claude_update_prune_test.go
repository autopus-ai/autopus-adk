package claude

import (
	"context"
	"os"
	"path/filepath"
	"sort"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/insajin/autopus-adk/pkg/adapter"
	"github.com/insajin/autopus-adk/pkg/config"
)

// relocatedRuleFiles are the rules SPEC-CONDRULE-001 and issue #185 move out of
// the baseline rule directory into the conditional body root. The four
// skill-scoped members are the 38 KB the reporter of #185 kept paying for on
// every session of an already-installed harness, so the prune half of the fix
// is what actually reclaims it on upgrade.
var relocatedRuleFiles = []string{
	"lore-commit.md",
}

// retiredRuleFiles were relocated too, and the verification harness then
// stopped shipping them: an upgrade must remove them from both places.
var retiredRuleFiles = []string{
	"context7-docs.md",
	"doc-storage.md",
	"shell-portability.md",
	"spec-quality.md",
	"techstack-freshness.md",
	"worktree-safety.md",
}

func baselineRulePath(name string) string {
	return filepath.ToSlash(filepath.Join(".claude", "rules", "autopus", name))
}

// seedPreCondruleState reproduces a workspace installed before the relocations:
// every relocated rule body exists under the baseline rule directory and the
// manifest still claims them as managed files.
func seedPreCondruleState(t *testing.T, root string) {
	t.Helper()
	manifest, err := adapter.LoadManifest(root, adapterName)
	require.NoError(t, err)
	require.NotNil(t, manifest)

	for _, name := range append(append([]string(nil), relocatedRuleFiles...), retiredRuleFiles...) {
		rel := baselineRulePath(name)
		body := "stale baseline copy of " + name
		require.NoError(t, os.WriteFile(filepath.Join(root, filepath.FromSlash(rel)), []byte(body), 0o644))
		manifest.Files[rel] = adapter.ManifestFile{
			Checksum: adapter.Checksum(body),
			Policy:   adapter.OverwriteAlways,
		}
	}
	require.NoError(t, manifest.Save(root))
}

// TestUpdate_PrunesRelocatedBaselineRules is the B1 regression: upgrading from a
// pre-relocation manifest must delete the stale baseline copies. Before the fix
// the apply path could not prune outside `.claude/skills/autopus`, so these
// files survived every upgrade and kept loading into baseline context.
func TestUpdate_PrunesRelocatedBaselineRules(t *testing.T) {
	t.Parallel()

	root := t.TempDir()
	adapterUnderTest := NewWithRoot(root)
	cfg := config.DefaultFullConfig("prune-regression")

	_, err := adapterUnderTest.Update(context.Background(), cfg)
	require.NoError(t, err)
	seedPreCondruleState(t, root)

	_, err = adapterUnderTest.Update(context.Background(), cfg)
	require.NoError(t, err)

	for _, name := range relocatedRuleFiles {
		assert.NoFileExists(t, filepath.Join(root, ".claude", "rules", "autopus", name),
			"%s must not survive the upgrade in baseline context", name)
		assert.FileExists(t, filepath.Join(root, ".claude", "hooks", "autopus", "conditional", name),
			"%s must remain present as a relocated body", name)
	}
	for _, name := range retiredRuleFiles {
		assert.NoFileExists(t, filepath.Join(root, ".claude", "rules", "autopus", name), "%s is retired", name)
		assert.NoFileExists(t, filepath.Join(root, ".claude", "hooks", "autopus", "conditional", name), "%s is retired", name)
	}

	// Safe delete: the transaction snapshots a pruned file before removing it.
	backups, err := filepath.Glob(filepath.Join(
		root, ".autopus", "backup", "*", "transaction", adapterName,
		".claude", "rules", "autopus", "lore-commit.md"))
	require.NoError(t, err)
	assert.NotEmpty(t, backups, "a pruned managed file must be backed up before deletion")
}

// TestBuildUpdateTransactionPlan_PruneSetCoversRelocation asserts the computed
// prune set directly, independently of disk state.
func TestBuildUpdateTransactionPlan_PruneSetCoversRelocation(t *testing.T) {
	t.Parallel()

	oldManifest := &adapter.Manifest{Platform: adapterName, Files: map[string]adapter.ManifestFile{}}
	for _, name := range relocatedRuleFiles {
		oldManifest.Files[baselineRulePath(name)] = adapter.ManifestFile{
			Checksum: "stale", Policy: adapter.OverwriteAlways,
		}
	}
	oldManifest.Files[baselineRulePath("branding.md")] = adapter.ManifestFile{
		Checksum: "kept", Policy: adapter.OverwriteAlways,
	}

	newFiles := []adapter.FileMapping{{
		TargetPath:      filepath.Join(".claude", "rules", "autopus", "branding.md"),
		OverwritePolicy: adapter.OverwriteAlways,
		Checksum:        "kept",
		Content:         []byte("branding"),
	}}
	for _, name := range relocatedRuleFiles {
		newFiles = append(newFiles, adapter.FileMapping{
			TargetPath:      filepath.Join(".claude", "hooks", "autopus", "conditional", name),
			OverwritePolicy: adapter.OverwriteAlways,
			Checksum:        "relocated",
			Content:         []byte("body"),
		})
	}

	plan, _, err := NewWithRoot(t.TempDir()).buildUpdateTransactionPlan(oldManifest, newFiles)
	require.NoError(t, err)

	removed := make([]string, 0, len(plan.Removes))
	for _, remove := range plan.Removes {
		removed = append(removed, filepath.ToSlash(remove.Path))
	}
	sort.Strings(removed)

	want := make([]string, 0, len(relocatedRuleFiles))
	for _, name := range relocatedRuleFiles {
		want = append(want, baselineRulePath(name))
	}
	sort.Strings(want)
	assert.Equal(t, want, removed,
		"exactly the relocated rules are pruned; a still-generated rule is retained")
}

// TestUpdate_NeverPrunesUnmanagedFiles is the orphan-safety assertion: pruning
// is limited to paths the previous manifest claimed, so a file the user dropped
// into a compiler-owned directory is left alone.
func TestUpdate_NeverPrunesUnmanagedFiles(t *testing.T) {
	t.Parallel()

	root := t.TempDir()
	adapterUnderTest := NewWithRoot(root)
	cfg := config.DefaultFullConfig("prune-regression")

	_, err := adapterUnderTest.Update(context.Background(), cfg)
	require.NoError(t, err)

	userFile := filepath.Join(root, ".claude", "rules", "autopus", "my-own-rule.md")
	require.NoError(t, os.WriteFile(userFile, []byte("user authored"), 0o644))

	_, err = adapterUnderTest.Update(context.Background(), cfg)
	require.NoError(t, err)

	body, err := os.ReadFile(userFile)
	require.NoError(t, err, "a user file in a compiler-owned directory must survive")
	assert.Equal(t, "user authored", string(body))
}

// TestPruneRoots_CoverBothSidesOfRelocation pins the root set itself, because
// dropping either directory silently reintroduces B1 in one direction.
func TestPruneRoots_CoverBothSidesOfRelocation(t *testing.T) {
	t.Parallel()
	assert.Equal(t, []string{
		".claude/skills",
		".claude/agents/autopus",
		".claude/workflows",
		".claude/commands",
		".git/hooks",
		".claude/rules/autopus",
		".claude/hooks/autopus/conditional",
	}, PruneRoots())
}
