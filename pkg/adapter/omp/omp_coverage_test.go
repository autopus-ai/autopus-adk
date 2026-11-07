package omp

import (
	"context"
	"os"
	"path/filepath"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/insajin/autopus-adk/pkg/adapter"
	"github.com/insajin/autopus-adk/pkg/config"
)

// TestOMPSkillCompilerExplicitSelection covers the explicit-only visibility
// gate. A skill marked explicit-only is emitted only when the compiler config
// names it, so this predicate decides whether the skill reaches the workspace.
func TestOMPSkillCompilerExplicitSelection(t *testing.T) {
	t.Parallel()

	assert.False(t, skillCompilerExplicitlySelects(nil, "any"),
		"a nil config selects nothing")

	cfg := config.DefaultFullConfig("omp-cov")
	cfg.Skills.Compiler.ExplicitSkills = []string{"alpha", "beta"}
	assert.True(t, skillCompilerExplicitlySelects(cfg, "beta"))
	assert.False(t, skillCompilerExplicitlySelects(cfg, "gamma"))

	assert.True(t, containsString([]string{"a", "b"}, "b"))
	assert.False(t, containsString([]string{"a"}, "z"))
	assert.False(t, containsString(nil, "a"))
}

// TestOMPGenerate_ExplicitSkillSelectionReachesAllowSkill drives the same gate
// through Generate so the AllowSkill closure in prepareExtendedSkillMappings is
// exercised with a config that names an explicit skill.
func TestOMPGenerate_ExplicitSkillSelectionReachesAllowSkill(t *testing.T) {
	t.Parallel()

	dir := t.TempDir()
	cfg := config.DefaultFullConfig("omp-cov")
	cfg.Platforms = []string{"omp"}
	// Every skill the verification harness ships is core, so the explicit
	// name is a core one; the closure still runs with an explicit selection.
	cfg.Skills.Compiler.ExplicitSkills = []string{"qa-hunt"}
	require.NoError(t, config.Save(dir, cfg))

	pf, err := NewWithRoot(dir).Generate(context.Background(), cfg)
	require.NoError(t, err)
	require.NotNil(t, pf)
	assert.FileExists(t, filepath.Join(dir, ".omp", "skills", "qa-hunt", "SKILL.md"))
}

// TestOMPRenderConfigDocument_AppendsToUnmarkedConfig covers the branch where a
// user already keeps a .omp/config.yml with no managed section: the managed
// block is appended and the user's keys survive above it.
func TestOMPRenderConfigDocument_AppendsToUnmarkedConfig(t *testing.T) {
	t.Parallel()

	dir := t.TempDir()
	path := filepath.Join(dir, configFile)
	require.NoError(t, os.MkdirAll(filepath.Dir(path), 0o755))
	require.NoError(t, os.WriteFile(path, []byte("disabledProviders:\n  - anthropic\n"), 0o644))

	doc, err := NewWithRoot(dir).renderConfigDocument(config.DefaultFullConfig("omp-cov"))
	require.NoError(t, err)
	assert.Equal(t, "disabledProviders:\n  - anthropic\n", doc)
	assert.NotContains(t, doc, "customDirectories")
}

// TestOMPIsPruneEligible_RejectsEscapingPaths pins the containment guard. A
// manifest path that escapes the workspace must never be deleted, and an empty
// root set means "no restriction" rather than "nothing eligible".
//
// The predicate is a directory-containment test, not an ownership test: every
// path below `.omp/rules` is eligible, including a user file that shares the
// directory. What protects that user file is the manifest restriction — prune
// only ever considers paths a previous manifest recorded (see
// TestOMPAcceptance_S13_UserOwnedOMPSurfacePreserved, which drives Clean over a
// workspace holding .omp/rules/mine.md).
func TestOMPIsPruneEligible_RejectsEscapingPaths(t *testing.T) {
	t.Parallel()

	roots := []string{ompRuleDir}
	for _, path := range []string{".", "../escape.md", "/etc/passwd"} {
		assert.False(t, isPruneEligible(path, roots),
			"%q escapes the workspace and must not be prunable", path)
	}
	assert.True(t, isPruneEligible("anything/at/all.md", nil),
		"an empty root set imposes no restriction")
	assert.True(t, isPruneEligible(ompRuleDir+"/"+ompRuleFilePrefix+"file-size-limit.md", roots))
	assert.True(t, isPruneEligible(ompRuleDir+"/mine.md", roots),
		"containment is prefix-blind: the manifest, not this predicate, spares user files")
	assert.False(t, isPruneEligible(".omp/RULES.md", roots),
		"a sibling outside the root is never eligible")
}

// TestOMPRemoveEmptyParents covers the walk-up: empty directories collapse, a
// non-empty directory stops the walk, and a missing directory is skipped rather
// than treated as an error.
func TestOMPRemoveEmptyParents(t *testing.T) {
	t.Parallel()

	t.Run("collapses empty parents up to root", func(t *testing.T) {
		t.Parallel()

		root := t.TempDir()
		deep := filepath.Join(root, "a", "b", "c")
		require.NoError(t, os.MkdirAll(deep, 0o755))

		require.NoError(t, removeEmptyParents(root, deep))
		assert.NoDirExists(t, filepath.Join(root, "a"))
		assert.DirExists(t, root, "the root itself is never removed")
	})

	t.Run("stops at a non-empty parent", func(t *testing.T) {
		t.Parallel()

		root := t.TempDir()
		deep := filepath.Join(root, "a", "b")
		require.NoError(t, os.MkdirAll(deep, 0o755))
		keep := filepath.Join(root, "a", "keep.md")
		require.NoError(t, os.WriteFile(keep, []byte("x\n"), 0o644))

		require.NoError(t, removeEmptyParents(root, deep))
		assert.NoDirExists(t, deep, "the empty leaf is removed")
		assert.FileExists(t, keep, "a parent holding a file survives")
	})

	t.Run("skips a directory that is already gone", func(t *testing.T) {
		t.Parallel()

		root := t.TempDir()
		require.NoError(t, removeEmptyParents(root, filepath.Join(root, "never", "existed")))
		assert.DirExists(t, root)
	})
}

// TestOMPClean_WithoutHarnessConfig covers the fallback path: `auto platform
// remove omp` can leave the workspace without a readable autopus.yaml, and
// Clean must still resolve ownership instead of failing.
func TestOMPClean_WithoutHarnessConfig(t *testing.T) {
	t.Parallel()

	dir := generateOMPOnly(t)
	require.NoError(t, os.Remove(filepath.Join(dir, "autopus.yaml")))

	require.NoError(t, NewWithRoot(dir).Clean(context.Background()))
	assert.NoFileExists(t, filepath.Join(dir, ompRuleDir, ompRuleFilePrefix+"file-size-limit.md"))
	assert.NoFileExists(t, filepath.Join(dir, ".omp", "agents", "ux-validator.md"))
}

// TestOMPClean_SkipsAlreadyDeletedManifestPath covers the branch where a user
// deleted a managed file by hand: Clean skips it instead of erroring.
func TestOMPClean_SkipsAlreadyDeletedManifestPath(t *testing.T) {
	t.Parallel()

	dir := generateOMPOnly(t)
	victim := filepath.Join(dir, ompRuleDir, ompRuleFilePrefix+"file-size-limit.md")
	require.FileExists(t, victim)
	require.NoError(t, os.Remove(victim))

	require.NoError(t, NewWithRoot(dir).Clean(context.Background()),
		"a manifest path the user already deleted must not fail Clean")
	assert.NoFileExists(t, filepath.Join(dir, ".omp", "agents", "ux-validator.md"))
}

// TestOMPWriteMapping_UnwritableTargetDirectory covers the I/O failure branch.
// The parent is made read-only so the write fails without needing root.
func TestOMPWriteMapping_UnwritableTargetDirectory(t *testing.T) {
	if os.Geteuid() == 0 {
		t.Skip("root ignores directory permissions")
	}

	root := t.TempDir()
	locked := filepath.Join(root, "locked")
	require.NoError(t, os.MkdirAll(locked, 0o755))
	require.NoError(t, os.Chmod(locked, 0o555))
	t.Cleanup(func() { _ = os.Chmod(locked, 0o755) })

	files := []adapter.FileMapping{{
		TargetPath:      filepath.Join("locked", "nested", "rule.md"),
		OverwritePolicy: adapter.OverwriteAlways,
		Content:         []byte("body\n"),
	}}
	err := writeMappings(root, files)
	require.Error(t, err, "an unwritable parent must surface as an error")
	assert.Contains(t, err.Error(), "디렉터리 생성 실패")

	// A writable parent whose target file cannot be replaced exercises the
	// write-failure branch separately from the mkdir branch.
	readOnlyTarget := filepath.Join(root, "ro")
	require.NoError(t, os.MkdirAll(readOnlyTarget, 0o555))
	t.Cleanup(func() { _ = os.Chmod(readOnlyTarget, 0o755) })
	markerFiles := []adapter.FileMapping{{
		TargetPath:      filepath.Join("ro", "marker.md"),
		OverwritePolicy: adapter.OverwriteMarker,
		Content:         []byte("body\n"),
	}}
	require.Error(t, writeMappings(root, markerFiles))
}

// TestOMPValidate_IgnoresUserFilesInRuleDirectory pins the doctor half of the
// shared-directory contract. `.omp/rules` holds ADK rules and the user's own
// rules side by side, so a surface comparison that claimed every file there was
// managed would report the user's file as an unexpected extra and fail
// `auto doctor` on a healthy workspace.
func TestOMPValidate_IgnoresUserFilesInRuleDirectory(t *testing.T) {
	dir := generateOMPOnly(t)

	require.NoError(t, os.WriteFile(
		filepath.Join(dir, ompRuleDir, "mine.md"), []byte("# my rule\n"), 0o644))
	nested := filepath.Join(dir, ompRuleDir, "nested")
	require.NoError(t, os.MkdirAll(nested, 0o755))
	require.NoError(t, os.WriteFile(
		filepath.Join(nested, "deep.md"), []byte("# nested\n"), 0o644))

	errs, err := NewWithRoot(dir).Validate(context.Background())
	require.NoError(t, err)
	assert.Empty(t, errs,
		"files without the %q prefix belong to the user and must not be reported, got %+v",
		ompRuleFilePrefix, errs)
}
