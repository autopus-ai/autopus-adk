package claude

import (
	"context"
	"path/filepath"
	"strings"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/insajin/autopus-adk/pkg/adapter"
	"github.com/insajin/autopus-adk/pkg/config"
)

// TestGenerate_SkillResourcesAreManifestOwned proves the reference bodies split
// out of a compacted skill are real installed files under harness ownership:
// they land beside their SKILL.md and appear in the manifest, so a later update
// can refresh or prune them instead of orphaning them.
func TestGenerate_SkillResourcesAreManifestOwned(t *testing.T) {
	t.Parallel()

	dir := t.TempDir()
	cfg := config.DefaultFullConfig("resource-owner")
	cfg.Platforms = []string{"claude-code"}

	pf, err := NewWithRoot(dir).Generate(context.Background(), cfg)
	require.NoError(t, err)

	manifest := adapter.ManifestFromFiles("claude-code", pf)

	resourceCount := 0
	for path, entry := range manifest.Files {
		if !strings.HasPrefix(filepath.ToSlash(path), ".claude/skills/agent-pipeline/references/") {
			continue
		}
		resourceCount++
		assert.FileExists(t, filepath.Join(dir, path))
		assert.NotEmpty(t, entry.Checksum, "manifest entry for %s must carry a checksum", path)
	}
	require.NotZero(t, resourceCount,
		"agent-pipeline reference bodies must be manifest-owned so update can refresh and prune them")

	assert.FileExists(t, filepath.Join(dir, ".claude", "skills", "agent-pipeline", "SKILL.md"),
		"resources must sit beside the skill they belong to")
}

// TestGenerate_DefaultSurfaceDropsLongTailSkills is the installed-output proof
// of the compact default: a fresh config writes the core surface and the /auto
// routes, and leaves the opt-in library uninstalled.
func TestGenerate_DefaultSurfaceDropsLongTailSkills(t *testing.T) {
	t.Parallel()

	dir := t.TempDir()
	cfg := config.DefaultFullConfig("compact-default")
	cfg.Platforms = []string{"claude-code"}

	_, err := NewWithRoot(dir).Generate(context.Background(), cfg)
	require.NoError(t, err)

	for _, name := range []string{"agent-pipeline", "planning", "review", "auto-plan", "auto"} {
		assert.FileExists(t, filepath.Join(dir, ".claude", "skills", name, "SKILL.md"),
			"%s must stay on the default native surface", name)
	}
	for _, name := range []string{"metrics", "docker", "competitive-analysis", "korean-writing-refiner"} {
		assert.NoFileExists(t, filepath.Join(dir, ".claude", "skills", name, "SKILL.md"),
			"%s is long-tail and must not be installed without an opt-in", name)
	}
}

// TestGenerate_ExplicitFullRestoresLongTailSkills covers the documented escape
// hatch end to end.
func TestGenerate_ExplicitFullRestoresLongTailSkills(t *testing.T) {
	t.Parallel()

	dir := t.TempDir()
	cfg := config.DefaultFullConfig("explicit-full")
	cfg.Platforms = []string{"claude-code"}
	cfg.Skills.Compiler.Mode = config.SkillCompilerModeFull

	_, err := NewWithRoot(dir).Generate(context.Background(), cfg)
	require.NoError(t, err)

	for _, name := range []string{"metrics", "docker", "competitive-analysis"} {
		assert.FileExists(t, filepath.Join(dir, ".claude", "skills", name, "SKILL.md"),
			"skills.compiler.mode: full must restore %s", name)
	}
}

// TestGenerate_ExplicitSkillOptInRestoresOneSkill covers the narrower opt-in a
// user reaches for far more often than mode: full.
func TestGenerate_ExplicitSkillOptInRestoresOneSkill(t *testing.T) {
	t.Parallel()

	dir := t.TempDir()
	cfg := config.DefaultFullConfig("explicit-skill")
	cfg.Platforms = []string{"claude-code"}
	cfg.Skills.Compiler.ExplicitSkills = []string{"metrics"}

	_, err := NewWithRoot(dir).Generate(context.Background(), cfg)
	require.NoError(t, err)

	assert.FileExists(t, filepath.Join(dir, ".claude", "skills", "metrics", "SKILL.md"),
		"skills.compiler.explicit_skills must install exactly the named skill")
	assert.NoFileExists(t, filepath.Join(dir, ".claude", "skills", "docker", "SKILL.md"),
		"an explicit opt-in must not drag the rest of the library back in")
}
