package adapter_test

import (
	"context"
	"os"
	"path/filepath"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/insajin/autopus-adk/pkg/adapter"
	"github.com/insajin/autopus-adk/pkg/adapter/codex"
	"github.com/insajin/autopus-adk/pkg/adapter/opencode"
	"github.com/insajin/autopus-adk/pkg/config"
)

func TestE2EMixedCodexOpencode_SharedFilesOwnedByOpencode(t *testing.T) {
	t.Parallel()

	dir := t.TempDir()
	cfg := config.DefaultFullConfig("e2e-mixed")
	cfg.Platforms = []string{"codex", "opencode"}

	codexAdapter := codex.NewWithRoot(dir)
	opencodeAdapter := opencode.NewWithRoot(dir, opencode.WithCLIVersion("1.18.7"))

	_, err := codexAdapter.Generate(context.Background(), cfg)
	require.NoError(t, err)
	_, err = opencodeAdapter.Update(context.Background(), cfg)
	require.NoError(t, err)

	codexManifest, err := adapter.LoadManifest(dir, "codex")
	require.NoError(t, err)
	require.NotNil(t, codexManifest)

	assert.NotContains(t, codexManifest.Files, "AGENTS.md")
	assert.NotContains(t, codexManifest.Files, filepath.Join(".agents", "skills", "auto", "SKILL.md"))

	for rel, meta := range codexManifest.Files {
		data, readErr := os.ReadFile(filepath.Join(dir, rel))
		require.NoError(t, readErr, "tracked file must exist: %s", rel)
		assert.Equal(t, meta.Checksum, adapter.Checksum(string(data)), "checksum should match: %s", rel)
	}
}

func TestE2EMixedCodexOpencode_CodexSkipsSharedSurfaceWrites(t *testing.T) {
	t.Parallel()

	dir := t.TempDir()
	cfg := config.DefaultFullConfig("e2e-mixed-write")
	cfg.Platforms = []string{"codex", "opencode"}

	codexAdapter := codex.NewWithRoot(dir)
	opencodeAdapter := opencode.NewWithRoot(dir, opencode.WithCLIVersion("1.18.7"))

	_, err := codexAdapter.Generate(context.Background(), cfg)
	require.NoError(t, err)

	_, statErr := os.Stat(filepath.Join(dir, "AGENTS.md"))
	assert.True(t, os.IsNotExist(statErr), "codex should not create AGENTS.md in mixed mode")
	_, statErr = os.Stat(filepath.Join(dir, ".agents", "skills", "auto", "SKILL.md"))
	assert.True(t, os.IsNotExist(statErr), "codex should not create shared skills in mixed mode")

	_, err = opencodeAdapter.Update(context.Background(), cfg)
	require.NoError(t, err)

	agentsBefore, err := os.ReadFile(filepath.Join(dir, "AGENTS.md"))
	require.NoError(t, err)
	skillBefore, err := os.ReadFile(filepath.Join(dir, ".agents", "skills", "auto", "SKILL.md"))
	require.NoError(t, err)

	_, err = codexAdapter.Update(context.Background(), cfg)
	require.NoError(t, err)

	agentsAfter, err := os.ReadFile(filepath.Join(dir, "AGENTS.md"))
	require.NoError(t, err)
	skillAfter, err := os.ReadFile(filepath.Join(dir, ".agents", "skills", "auto", "SKILL.md"))
	require.NoError(t, err)

	assert.Equal(t, string(agentsBefore), string(agentsAfter), "codex update must not rewrite AGENTS.md in mixed mode")
	assert.Equal(t, string(skillBefore), string(skillAfter), "codex update must not rewrite shared skills in mixed mode")
}

func TestE2EMixedCodexOpencode_CodexValidateSkipsSharedSurfaceChecks(t *testing.T) {
	t.Parallel()

	dir := t.TempDir()
	cfg := config.DefaultFullConfig("e2e-mixed-validate")
	cfg.Platforms = []string{"codex", "opencode"}

	codexAdapter := codex.NewWithRoot(dir)

	_, err := codexAdapter.Generate(context.Background(), cfg)
	require.NoError(t, err)
	_, err = opencode.NewWithRoot(dir, opencode.WithCLIVersion("1.18.7")).Update(context.Background(), cfg)
	require.NoError(t, err)

	errs, err := codexAdapter.Validate(context.Background())
	require.NoError(t, err)

	for _, validationErr := range errs {
		assert.NotEqual(t, "AGENTS.md", validationErr.File, "codex validate should skip shared AGENTS.md checks in mixed mode")
		assert.NotEqual(t, filepath.Join(".agents", "skills", "auto", "SKILL.md"), validationErr.File, "codex validate should skip shared skill checks in mixed mode")
	}
}

func TestE2EMixedCodexOpencode_CodexCleanPreservesSharedSurface(t *testing.T) {
	t.Parallel()

	dir := t.TempDir()
	cfg := config.DefaultFullConfig("e2e-mixed-clean")
	cfg.Platforms = []string{"codex", "opencode"}

	codexAdapter := codex.NewWithRoot(dir)
	opencodeAdapter := opencode.NewWithRoot(dir, opencode.WithCLIVersion("1.18.7"))

	_, err := codexAdapter.Generate(context.Background(), cfg)
	require.NoError(t, err)
	_, err = opencodeAdapter.Update(context.Background(), cfg)
	require.NoError(t, err)

	agentsBefore, err := os.ReadFile(filepath.Join(dir, "AGENTS.md"))
	require.NoError(t, err)
	skillBefore, err := os.ReadFile(filepath.Join(dir, ".agents", "skills", "auto", "SKILL.md"))
	require.NoError(t, err)

	err = codexAdapter.Clean(context.Background())
	require.NoError(t, err)

	_, statErr := os.Stat(filepath.Join(dir, ".codex", "skills"))
	assert.True(t, os.IsNotExist(statErr), "codex clean should remove codex-managed skills")

	agentsAfter, err := os.ReadFile(filepath.Join(dir, "AGENTS.md"))
	require.NoError(t, err)
	skillAfter, err := os.ReadFile(filepath.Join(dir, ".agents", "skills", "auto", "SKILL.md"))
	require.NoError(t, err)

	assert.Equal(t, string(agentsBefore), string(agentsAfter), "codex clean must preserve OpenCode-owned AGENTS.md")
	assert.Equal(t, string(skillBefore), string(skillAfter), "codex clean must preserve OpenCode-owned shared skills")
}
