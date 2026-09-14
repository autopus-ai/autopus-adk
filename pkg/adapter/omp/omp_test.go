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
	pkgcontent "github.com/insajin/autopus-adk/pkg/content"
)

func TestOMP_S1_S2_RuleTransformation(t *testing.T) {
	// S1, S2, E1: frontmatter filtering and body validation
	srcEmptyFm := `category: workflow
# Body content`
	_, err := pkgcontent.TransformRuleForOMP(srcEmptyFm)
	assert.NoError(t, err) // category is dropped, fm omitted, body non-empty so passes

	srcWithFm := `---
description: "Test Rule"
category: workflow
condition: "tool:bash"
---
# Body content`
	transformed, err := pkgcontent.TransformRuleForOMP(srcWithFm)
	assert.NoError(t, err)
	assert.Contains(t, transformed, "description: Test Rule")
	assert.Contains(t, transformed, "condition: tool:bash")
	assert.NotContains(t, transformed, "category:")

	srcEmptyBody := `---
description: "Empty"
---`
	_, err = pkgcontent.TransformRuleForOMP(srcEmptyBody)
	assert.Error(t, err) // empty body throws error
}

func TestOMP_S4_S5_S6_Lifecycle(t *testing.T) {
	// S4, S5, S6
	dir := t.TempDir()
	a := NewWithRoot(dir)

	cfg := config.DefaultFullConfig("omp-test")
	cfg.Platforms = []string{"omp"}

	pf, err := a.Generate(context.Background(), cfg)
	require.NoError(t, err)
	assert.NotNil(t, pf)

	// Check S6: generated native files without an unnecessary base config.
	assert.NoFileExists(t, filepath.Join(dir, configFile))
	assert.NoDirExists(t, filepath.Join(dir, ".omp", "agents"))
	assert.FileExists(t, filepath.Join(dir, ".omp", "skills", "auto", "SKILL.md"))
	assert.FileExists(t, filepath.Join(dir, ".omp", "commands", "auto.md"))
	assert.FileExists(t, filepath.Join(dir, ompRuleDir, ompRuleFilePrefix+"branding.md"))

	// Check S5: no CLAUDE.md shadowed
	assert.NoFileExists(t, filepath.Join(dir, ".claude", "CLAUDE.md"))

	// Check S4: Clean
	err = a.Clean(context.Background())
	require.NoError(t, err)
	assert.NoFileExists(t, filepath.Join(dir, ".omp", "agents", "executor.md"))
	assert.NoFileExists(t, filepath.Join(dir, ompRuleDir, ompRuleFilePrefix+"branding.md"))
}

func TestOMP_S10_S11_Ownership(t *testing.T) {
	mixed := PruneRoots(&config.HarnessConfig{Platforms: []string{"opencode", "codex", "omp"}})
	for _, root := range []string{".omp/skills", ".omp/commands", ".agents/commands"} {
		assert.Contains(t, mixed, root)
	}
	assert.NotContains(t, mixed, ".agents/skills")

	ompOnly := PruneRoots(&config.HarnessConfig{Platforms: []string{"omp"}})
	assert.Contains(t, ompOnly, ".agents/skills")
}

func TestOMP_E6_StaleManifestClean(t *testing.T) {
	dir := t.TempDir()
	a := NewWithRoot(dir)
	cfg := config.DefaultFullConfig("omp-test")
	cfg.Platforms = []string{"omp"}
	_, err := a.Generate(context.Background(), cfg)
	require.NoError(t, err)

	legacyPath := filepath.Join(".agents", "skills", "auto", "SKILL.md")
	skillPath := filepath.Join(dir, legacyPath)
	require.NoError(t, os.MkdirAll(filepath.Dir(skillPath), 0o755))
	require.NoError(t, os.WriteFile(skillPath, []byte("legacy omp content"), 0o644))
	manifest, err := adapter.LoadManifest(dir, adapterName)
	require.NoError(t, err)
	require.NotNil(t, manifest)
	manifest.Files[legacyPath] = adapter.ManifestFile{
		Checksum: adapter.Checksum("legacy omp content"), Policy: adapter.OverwriteAlways,
	}
	require.NoError(t, manifest.Save(dir))

	cfgMixed := config.DefaultFullConfig("omp-test")
	cfgMixed.Platforms = []string{"opencode", "omp"}
	require.NoError(t, config.Save(dir, cfgMixed))
	require.NoError(t, os.WriteFile(skillPath, []byte("opencode content"), 0o644))

	require.NoError(t, a.Clean(context.Background()))
	assertFileBytesOMP(t, skillPath, "opencode content")
	assert.NoFileExists(t, filepath.Join(dir, ".omp", "agents", "executor.md"))
}
