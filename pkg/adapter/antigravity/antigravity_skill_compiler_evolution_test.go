package antigravity_test

import (
	"bytes"
	"context"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"gopkg.in/yaml.v3"

	"github.com/insajin/autopus-adk/pkg/adapter/antigravity"
	"github.com/insajin/autopus-adk/pkg/config"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestAntigravitySkillCompiler_FullSplitFullKeepsNativeAndMirrorParity(t *testing.T) {
	t.Parallel()

	root := t.TempDir()
	antigravityAdapter := antigravity.NewWithRoot(root)
	cfg := config.DefaultFullConfig("gemini-compiler")
	cfg.Platforms = []string{"antigravity-cli"}
	// The long-tail assertions below need the opt-in full catalog; the default
	// compiler mode installs only the core subset.
	cfg.Skills.Compiler.Mode = config.SkillCompilerModeFull

	_, err := antigravityAdapter.Generate(context.Background(), cfg)
	require.NoError(t, err)

	nativeCore := antigravitySkillPath(root, ".gemini", "skills", "autopus", "planning")
	nativeWorkflow := antigravitySkillPath(root, ".gemini", "skills", "autopus", "auto-go")
	nativeLongTail := antigravitySkillPath(root, ".gemini", "skills", "autopus", "metrics")
	mirrorCore := antigravitySkillPath(root, ".agents", "plugins", "autopus", "skills", "planning")
	mirrorWorkflow := antigravitySkillPath(root, ".agents", "plugins", "autopus", "skills", "auto-go")
	mirrorLongTail := antigravitySkillPath(root, ".agents", "plugins", "autopus", "skills", "metrics")
	for _, path := range []string{
		nativeCore, nativeWorkflow, nativeLongTail,
		mirrorCore, mirrorWorkflow, mirrorLongTail,
	} {
		assert.FileExists(t, path)
	}

	_, err = antigravityAdapter.Update(context.Background(), cfg)
	require.NoError(t, err)
	settingsPath := filepath.Join(root, ".gemini", "settings.json")
	settingsBefore, err := os.ReadFile(settingsPath)
	require.NoError(t, err)

	userSkill := filepath.Join(root, ".gemini", "skills", "user-owned", "SKILL.md")
	outsidePlugin := filepath.Join(root, ".agents", "plugins", "user-owned.txt")
	outside := filepath.Join(root, "notes", "outside.txt")
	require.NoError(t, os.MkdirAll(filepath.Dir(userSkill), 0o700))
	require.NoError(t, os.WriteFile(userSkill, []byte("user native skill\n"), 0o600))
	require.NoError(t, os.MkdirAll(filepath.Dir(outsidePlugin), 0o700))
	require.NoError(t, os.WriteFile(outsidePlugin, []byte("user plugin\n"), 0o600))
	require.NoError(t, os.MkdirAll(filepath.Dir(outside), 0o700))
	require.NoError(t, os.WriteFile(outside, []byte("outside\n"), 0o600))

	cfg.Skills.Compiler.Mode = config.SkillCompilerModeSplit
	cfg.Skills.Compiler.Bundles = []string{"ops"}
	_, err = antigravityAdapter.Update(context.Background(), cfg)
	require.NoError(t, err)

	for _, path := range []string{nativeCore, nativeWorkflow, mirrorCore, mirrorWorkflow} {
		assert.FileExists(t, path)
	}
	assert.NoFileExists(t, nativeLongTail)
	assert.NoFileExists(t, mirrorLongTail)
	assertAntigravityFileBytes(t, userSkill, []byte("user native skill\n"))
	assertAntigravityFileBytes(t, outsidePlugin, []byte("user plugin\n"))
	assertAntigravityFileBytes(t, outside, []byte("outside\n"))
	settingsSplit, err := os.ReadFile(settingsPath)
	require.NoError(t, err)
	assert.True(
		t,
		bytes.Equal(settingsBefore, settingsSplit),
		"tool projection changed Gemini settings\nbefore:\n%s\nafter:\n%s",
		settingsBefore,
		settingsSplit,
	)

	cfg.Skills.Compiler.Mode = config.SkillCompilerModeFull
	cfg.Skills.Compiler.Bundles = nil
	_, err = antigravityAdapter.Update(context.Background(), cfg)
	require.NoError(t, err)

	for _, path := range []string{
		nativeCore, nativeWorkflow, nativeLongTail,
		mirrorCore, mirrorWorkflow, mirrorLongTail,
	} {
		assert.FileExists(t, path)
	}
	assertAntigravityFileBytes(t, userSkill, []byte("user native skill\n"))
	assertAntigravityFileBytes(t, outsidePlugin, []byte("user plugin\n"))
	assertAntigravityFileBytes(t, outside, []byte("outside\n"))
	settingsFullAgain, err := os.ReadFile(settingsPath)
	require.NoError(t, err)
	assert.True(
		t,
		bytes.Equal(settingsBefore, settingsFullAgain),
		"returning to full changed Gemini settings\nbefore:\n%s\nafter:\n%s",
		settingsBefore,
		settingsFullAgain,
	)
}

func TestAntigravityAgents_ProjectNativeToolsWithoutChangingBodyOrSkills(t *testing.T) {
	t.Parallel()

	root := t.TempDir()
	cfg := config.DefaultFullConfig("gemini-tools")
	cfg.Platforms = []string{"antigravity-cli"}
	_, err := antigravity.NewWithRoot(root).Generate(context.Background(), cfg)
	require.NoError(t, err)

	nativePath := filepath.Join(root, ".gemini", "agents", "autopus", "executor.md")
	mirrorPath := filepath.Join(root, ".agents", "plugins", "autopus", "agents", "executor.md")
	native := readAntigravityAgentContract(t, nativePath)
	mirror := readAntigravityAgentContract(t, mirrorPath)
	wantTools := []string{
		"read_file",
		"write_file",
		"replace",
		"grep_search",
		"glob",
		"run_shell_command",
	}

	assert.Equal(t, wantTools, native.Frontmatter.Tools)
	assert.Equal(t, wantTools, mirror.Frontmatter.Tools)
	assert.Equal(t, []string{"tdd", "ddd", "debugging", "ast-refactoring"}, native.Frontmatter.Skills)
	assert.Equal(t, native.Frontmatter.Skills, mirror.Frontmatter.Skills)
	assert.Contains(t, native.Body, "# Executor Agent")
	assert.Contains(t, native.Body, "Phase 1.5 Test Constraint")
	assert.NotContains(t, native.Frontmatter.Tools, "TodoWrite")
	assert.NotContains(t, native.Frontmatter.Tools, "Agent")
	assert.NotContains(t, native.Body, "Codex native enforcement")
}

func TestAntigravitySkillCompiler_RejectsSymlinkedNativeOrMirrorRootDuringPrune(t *testing.T) {
	t.Parallel()

	for _, managedRel := range []string{
		filepath.Join(".gemini", "skills", "autopus"),
		filepath.Join(".agents", "plugins", "autopus", "skills"),
	} {
		managedRel := managedRel
		t.Run(filepath.ToSlash(managedRel), func(t *testing.T) {
			t.Parallel()

			root := t.TempDir()
			antigravityAdapter := antigravity.NewWithRoot(root)
			cfg := config.DefaultFullConfig("gemini-symlink-prune")
			cfg.Platforms = []string{"antigravity-cli"}
			_, err := antigravityAdapter.Generate(context.Background(), cfg)
			require.NoError(t, err)

			managedRoot := filepath.Join(root, managedRel)
			require.NoError(t, os.RemoveAll(managedRoot))
			outside := t.TempDir()
			external := filepath.Join(outside, "metrics", "SKILL.md")
			externalBytes := []byte("outside-owned skill\n")
			require.NoError(t, os.MkdirAll(filepath.Dir(external), 0o755))
			require.NoError(t, os.WriteFile(external, externalBytes, 0o600))
			if err := os.Symlink(outside, managedRoot); err != nil {
				t.Skipf("symlink unavailable: %v", err)
			}

			cfg.Skills.Compiler.Mode = config.SkillCompilerModeSplit
			cfg.Skills.Compiler.Bundles = []string{"ops"}
			_, err = antigravityAdapter.Update(context.Background(), cfg)

			require.Error(t, err)
			assert.ErrorContains(t, err, "crosses symlink")
			assertAntigravityFileBytes(t, external, externalBytes)
		})
	}
}

type generatedAntigravityAgent struct {
	Frontmatter struct {
		Skills []string `yaml:"skills"`
		Tools  []string `yaml:"tools"`
	}
	Body string
}

func readAntigravityAgentContract(t *testing.T, path string) generatedAntigravityAgent {
	t.Helper()
	data, err := os.ReadFile(path)
	require.NoError(t, err)
	parts := strings.SplitN(string(data), "---", 3)
	require.Len(t, parts, 3)
	var result generatedAntigravityAgent
	require.NoError(t, yaml.Unmarshal([]byte(parts[1]), &result.Frontmatter))
	result.Body = strings.TrimSpace(parts[2])
	return result
}

func antigravitySkillPath(root string, parts ...string) string {
	return filepath.Join(append([]string{root}, append(parts, "SKILL.md")...)...)
}

func assertAntigravityFileBytes(t *testing.T, path string, want []byte) {
	t.Helper()
	got, err := os.ReadFile(path)
	require.NoError(t, err)
	assert.True(t, bytes.Equal(want, got), "%s bytes changed", path)
}
