package claude_test

import (
	"bytes"
	"context"
	"os"
	"path/filepath"
	"testing"

	"github.com/insajin/autopus-adk/pkg/adapter/claude"
	"github.com/insajin/autopus-adk/pkg/config"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestClaudeSkillCompiler_FullSplitFullPrunesOnlyOwnedLongTail(t *testing.T) {
	t.Parallel()

	root := t.TempDir()
	claudeAdapter := claude.NewWithRoot(root)
	cfg := config.DefaultFullConfig("claude-compiler")
	cfg.Platforms = []string{"claude-code"}
	// This fixture is about the full -> split -> full transition, so the first
	// leg selects the full library explicitly instead of riding the default.
	cfg.Skills.Compiler.Mode = config.SkillCompilerModeFull

	_, err := claudeAdapter.Generate(context.Background(), cfg)
	require.NoError(t, err)

	core := filepath.Join(root, ".claude", "skills", "planning", "SKILL.md")
	workflow := filepath.Join(root, ".claude", "skills", "auto-go", "SKILL.md")
	longTail := filepath.Join(root, ".claude", "skills", "metrics", "SKILL.md")
	assert.FileExists(t, core)
	assert.FileExists(t, workflow)
	assert.FileExists(t, longTail)

	userSkill := filepath.Join(root, ".claude", "skills", "user-owned.md")
	outside := filepath.Join(root, "notes", "outside.txt")
	userSkillBytes := []byte("user-owned skill\n")
	outsideBytes := []byte("outside generated roots\n")
	require.NoError(t, os.WriteFile(userSkill, userSkillBytes, 0o600))
	require.NoError(t, os.MkdirAll(filepath.Dir(outside), 0o700))
	require.NoError(t, os.WriteFile(outside, outsideBytes, 0o600))

	cfg.Skills.Compiler.Mode = config.SkillCompilerModeSplit
	cfg.Skills.Compiler.Bundles = []string{"ops"}
	_, err = claudeAdapter.Update(context.Background(), cfg)
	require.NoError(t, err)

	assert.FileExists(t, core)
	assert.FileExists(t, workflow)
	assert.NoFileExists(t, longTail)
	assertFileBytes(t, userSkill, userSkillBytes)
	assertFileBytes(t, outside, outsideBytes)

	cfg.Skills.Compiler.Mode = config.SkillCompilerModeFull
	cfg.Skills.Compiler.Bundles = nil
	_, err = claudeAdapter.Update(context.Background(), cfg)
	require.NoError(t, err)

	assert.FileExists(t, core)
	assert.FileExists(t, workflow)
	assert.FileExists(t, longTail)
	assertFileBytes(t, userSkill, userSkillBytes)
	assertFileBytes(t, outside, outsideBytes)
}

func TestClaudeSkillCompiler_RejectsSymlinkedManagedRootDuringPrune(t *testing.T) {
	t.Parallel()

	root := t.TempDir()
	claudeAdapter := claude.NewWithRoot(root)
	cfg := config.DefaultFullConfig("claude-symlink-prune")
	cfg.Platforms = []string{"claude-code"}
	// The prune under test only happens because metrics was installed first,
	// which the full library selection guarantees.
	cfg.Skills.Compiler.Mode = config.SkillCompilerModeFull
	_, err := claudeAdapter.Generate(context.Background(), cfg)
	require.NoError(t, err)

	managedRoot := filepath.Join(root, ".claude", "skills", "metrics")
	require.NoError(t, os.RemoveAll(managedRoot))
	outside := t.TempDir()
	external := filepath.Join(outside, "SKILL.md")
	externalBytes := []byte("outside-owned skill\n")
	require.NoError(t, os.WriteFile(external, externalBytes, 0o600))
	if err := os.Symlink(outside, managedRoot); err != nil {
		t.Skipf("symlink unavailable: %v", err)
	}

	cfg.Skills.Compiler.Mode = config.SkillCompilerModeSplit
	cfg.Skills.Compiler.Bundles = []string{"ops"}
	_, err = claudeAdapter.Update(context.Background(), cfg)

	require.Error(t, err)
	assert.ErrorContains(t, err, "crosses symlink")
	assertFileBytes(t, external, externalBytes)
}

func assertFileBytes(t *testing.T, path string, want []byte) {
	t.Helper()
	got, err := os.ReadFile(path)
	require.NoError(t, err)
	assert.True(t, bytes.Equal(want, got), "%s bytes changed", path)
}
