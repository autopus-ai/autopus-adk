package claude

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

// legacyFlatSkill is one file of the retired flat skill layout. The native
// layout the compiler emits is `.claude/skills/<name>/SKILL.md`.
const legacyFlatSkill = ".claude/skills/autopus/adaptive-quality.md"

// staleBaselineRule is the pre-relocation copy of a hook-fired rule body.
// SPEC-CONDRULE-001 moved the body to `.claude/hooks/autopus/conditional/`.
const staleBaselineRule = ".claude/rules/autopus/lore-commit.md"

func writeClaudeFile(t *testing.T, root, rel, body string) {
	t.Helper()
	abs := filepath.Join(root, filepath.FromSlash(rel))
	require.NoError(t, os.MkdirAll(filepath.Dir(abs), 0o755))
	require.NoError(t, os.WriteFile(abs, []byte(body), 0o644))
}

// TestUpdate_RemovesRetiredClaudeSurfaceWithoutManifest is the fresh-clone
// regression for Claude.
//
// `.autopus/claude-code-manifest.json` is gitignored while the generated files
// are committed, so a cloned workspace carries orphans with no ownership
// record. The manifest-only prune iterates old.Files, which is nil here, so it
// was structurally blind to them; the update then wrote a manifest without
// them, making the orphan permanent across every later update too.
func TestUpdate_RemovesRetiredClaudeSurfaceWithoutManifest(t *testing.T) {
	t.Parallel()
	root := t.TempDir()
	writeClaudeFile(t, root, legacyFlatSkill, "flat legacy skill")
	writeClaudeFile(t, root, staleBaselineRule, "stale baseline copy")

	require.NoFileExists(t, filepath.Join(root, ".autopus", "claude-code-manifest.json"),
		"the reproduction requires the no-manifest arm")

	_, err := NewWithRoot(root).Update(context.Background(), config.DefaultFullConfig("orphan-project"))
	require.NoError(t, err)

	assert.NoFileExists(t, filepath.Join(root, filepath.FromSlash(legacyFlatSkill)),
		"the flat skill copy must not survive an update that had no manifest")
	assert.NoFileExists(t, filepath.Join(root, filepath.FromSlash(staleBaselineRule)),
		"the stale baseline rule copy must not survive an update that had no manifest")

	// The relocation target and the native skill layout must both be live.
	assert.FileExists(t, filepath.Join(root, ".claude", "hooks", "autopus", "conditional", "lore-commit.md"))
	assert.FileExists(t, filepath.Join(root, ".claude", "skills", "planning", "SKILL.md"))
	// No empty husk left where the retired layout used to live.
	assert.NoDirExists(t, filepath.Join(root, ".claude", "skills", "autopus"))
}

// TestUpdate_PreservesUserFilesInsideClaudePruneRoots is the safety half. Every
// path below sits inside a PruneRoots subtree with no manifest to vouch for it,
// so only the closed detector set keeps them alive.
func TestUpdate_PreservesUserFilesInsideClaudePruneRoots(t *testing.T) {
	t.Parallel()
	root := t.TempDir()
	survivors := map[string]string{
		// Native/user skill layout: a directory, not a flat `*.md`.
		".claude/skills/custom/SKILL.md": "user skill",
		// A skill literally named `autopus` occupies the retired directory
		// path with the native file name. The flat layout never produced it.
		".claude/skills/autopus/SKILL.md": "user skill named autopus",
		// A user rule in the compiler-owned baseline rule directory.
		".claude/rules/autopus/my-own-rule.md": "user rule",
	}
	for rel, body := range survivors {
		writeClaudeFile(t, root, rel, body)
	}

	_, err := NewWithRoot(root).Update(context.Background(), config.DefaultFullConfig("orphan-project"))
	require.NoError(t, err)

	for rel, body := range survivors {
		data, readErr := os.ReadFile(filepath.Join(root, filepath.FromSlash(rel)))
		require.NoError(t, readErr, "%s must survive: it is not in the retired set", rel)
		assert.Equal(t, body, string(data), rel)
	}

	// A live generated rule shares the directory with the stale copies the
	// detector removes; it must be emitted, not deleted.
	assert.FileExists(t, filepath.Join(root, ".claude", "rules", "autopus", "branding.md"))
}

// TestValidate_ReportsObsoleteClaudeSurface is the doctor half: before this the
// Claude adapter had no obsolete-surface detector at all, so 51 orphans in a
// real workspace reported as zero findings while the Codex equivalents reported
// as errors.
func TestValidate_ReportsObsoleteClaudeSurface(t *testing.T) {
	t.Parallel()
	root := t.TempDir()
	adapterUnderTest := NewWithRoot(root)
	_, err := adapterUnderTest.Update(context.Background(), config.DefaultFullConfig("orphan-project"))
	require.NoError(t, err)

	writeClaudeFile(t, root, legacyFlatSkill, "flat legacy skill")
	writeClaudeFile(t, root, staleBaselineRule, "stale baseline copy")
	writeClaudeFile(t, root, ".claude/skills/custom/SKILL.md", "user skill")
	writeClaudeFile(t, root, ".claude/rules/autopus/my-own-rule.md", "user rule")

	errs, err := adapterUnderTest.Validate(context.Background())
	require.NoError(t, err)

	obsolete := make([]string, 0, 2)
	for _, item := range errs {
		if item.Message != "obsolete Claude managed surface가 남아 있음" {
			continue
		}
		assert.Equal(t, "error", item.Level, item.File)
		obsolete = append(obsolete, item.File)
	}
	assert.Equal(t, []string{staleBaselineRule, legacyFlatSkill}, obsolete,
		"exactly the retired surfaces are reported; user files in the same directories are not")
}

// TestBuildUpdateTransactionRemoves_DeduplicatesDetectorAndManifestPrune covers
// the overlap arm: when a retired surface IS recorded in the previous manifest,
// both sources name it and the plan must still remove it once.
func TestBuildUpdateTransactionRemoves_DeduplicatesDetectorAndManifestPrune(t *testing.T) {
	t.Parallel()
	root := t.TempDir()
	writeClaudeFile(t, root, legacyFlatSkill, "flat legacy skill")

	oldManifest := &adapter.Manifest{Platform: adapterName, Files: map[string]adapter.ManifestFile{
		legacyFlatSkill: {Checksum: "stale", Policy: adapter.OverwriteAlways},
	}}
	diff := adapter.BuildManifestDiff(oldManifest, nil, PruneRoots())

	removes, err := NewWithRoot(root).buildUpdateTransactionRemoves(diff)
	require.NoError(t, err)

	count := 0
	for _, remove := range removes {
		if filepath.ToSlash(remove.Path) == legacyFlatSkill {
			count++
		}
	}
	assert.Equal(t, 1, count, "a surface named by both sources is planned once")
}
