package omp

import (
	"context"
	"os"
	"path/filepath"
	"runtime"
	"strings"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/insajin/autopus-adk/pkg/adapter"
	"github.com/insajin/autopus-adk/pkg/config"
)

func TestOMPGenerate_UsesOnlyNativeSkillsAndCommandsWithoutBaseConfig(t *testing.T) {
	root := t.TempDir()
	cfg := config.DefaultFullConfig("omp-native")
	cfg.Platforms = []string{"omp"}
	require.NoError(t, config.Save(root, cfg))

	_, err := NewWithRoot(root).Generate(context.Background(), cfg)
	require.NoError(t, err)

	paths := manifestPaths(t, root)
	assert.Greater(t, countPathsWithPrefix(paths, ".omp/skills/"), 20)
	assert.Equal(t, len(workflowSpecs), countPathsWithPrefix(paths, ".omp/commands/"))
	assert.Equal(t, 0, countPathsWithPrefix(paths, ".omp/agents/"))
	assert.Equal(t, 14, countPathsWithPrefix(paths, ompRuleDir+"/"+ompRuleFilePrefix))
	assert.NotContains(t, paths, configFile)
	assert.NoFileExists(t, filepath.Join(root, configFile))
	for _, path := range paths {
		assert.False(t, strings.HasPrefix(path, ".agents/skills/"), path)
		assert.False(t, strings.HasPrefix(path, ".agents/commands/"), path)
	}
	goalSkill, err := os.ReadFile(filepath.Join(root, ".omp", "skills", "auto-goal", "SKILL.md"))
	require.NoError(t, err)
	frontmatter := strings.SplitN(string(goalSkill), "---", 3)
	require.Len(t, frontmatter, 3)
	assert.NotContains(t, frontmatter[1], "Codex")
	assert.Contains(t, frontmatter[1], "goal")
}

func TestOMPGenerate_PreservesUserBaseConfigWhenNoManagedClaimsExist(t *testing.T) {
	root := t.TempDir()
	cfg := config.DefaultFullConfig("omp-user-config")
	cfg.Platforms = []string{"omp"}
	require.NoError(t, config.Save(root, cfg))
	path := filepath.Join(root, configFile)
	require.NoError(t, os.MkdirAll(filepath.Dir(path), 0o700))
	original := []byte("theme:\n  dark: user-theme\n")
	require.NoError(t, os.WriteFile(path, original, 0o600))

	_, err := NewWithRoot(root).Generate(context.Background(), cfg)
	require.NoError(t, err)

	actual, err := os.ReadFile(path)
	require.NoError(t, err)
	assert.Equal(t, original, actual)
	assert.NotContains(t, manifestPaths(t, root), configFile)
}

func TestOMPValidate_DetectsManagedContentTamperingAndSymlinkReplacement(t *testing.T) {
	if runtime.GOOS == "windows" {
		t.Skip("symlink fixture")
	}
	root := generateLatestOMP(t)
	adapter := NewWithRoot(root)
	target := filepath.Join(root, ".omp", "commands", "auto.md")

	require.NoError(t, os.WriteFile(target, []byte("tampered\n"), 0o644))
	findings, err := adapter.Validate(context.Background())
	require.NoError(t, err)
	assertOMPValidationFinding(t, findings, ".omp/commands/auto.md", "checksum")

	require.NoError(t, os.Remove(target))
	external := filepath.Join(t.TempDir(), "auto.md")
	require.NoError(t, os.WriteFile(external, []byte("external\n"), 0o644))
	require.NoError(t, os.Symlink(external, target))
	findings, err = adapter.Validate(context.Background())
	require.NoError(t, err)
	assertOMPValidationFinding(t, findings, ".omp/commands/auto.md", "regular file")
}

func TestOMPValidateExpectedMappings_RejectsSymlinkParentBeforeReading(t *testing.T) {
	if runtime.GOOS == "windows" {
		t.Skip("symlink fixture")
	}
	root := t.TempDir()
	outside := t.TempDir()
	content := "external content must not be read\n"
	require.NoError(t, os.WriteFile(filepath.Join(outside, "managed.md"), []byte(content), 0o644))
	require.NoError(t, os.Symlink(outside, filepath.Join(root, "linked")))

	findings := NewWithRoot(root).validateOMPExpectedMappings([]adapter.FileMapping{{
		TargetPath: "linked/managed.md",
		Checksum:   adapter.Checksum(content),
	}})

	require.Len(t, findings, 1)
	assert.Equal(t, "linked/managed.md", findings[0].File)
	assert.Contains(t, findings[0].Message, "symlink")
}

func TestOMPValidate_AllowsMissingBaseConfigButRejectsWeakSensitivePermissions(t *testing.T) {
	root := generateLatestOMP(t)
	adapter := NewWithRoot(root)

	findings, err := adapter.Validate(context.Background())
	require.NoError(t, err)
	for _, finding := range findings {
		assert.NotEqual(t, configFile, filepath.ToSlash(finding.File), finding.Message)
	}

	path := filepath.Join(root, configFile)
	require.NoError(t, os.WriteFile(path, []byte("tools:\n  intentTracing: true\n"), 0o644))
	findings, err = adapter.Validate(context.Background())
	require.NoError(t, err)
	assertOMPValidationFinding(t, findings, configFile, "permission")
}

func generateLatestOMP(t *testing.T) string {
	t.Helper()
	root := t.TempDir()
	cfg := config.DefaultFullConfig("omp-latest")
	cfg.Platforms = []string{"omp"}
	require.NoError(t, config.Save(root, cfg))
	_, err := NewWithRoot(root).Generate(context.Background(), cfg)
	require.NoError(t, err)
	return root
}

func assertOMPValidationFinding(t *testing.T, findings []adapter.ValidationError, path, message string) {
	t.Helper()
	for _, finding := range findings {
		if filepath.ToSlash(finding.File) == path && strings.Contains(strings.ToLower(finding.Message), strings.ToLower(message)) {
			return
		}
	}
	t.Fatalf("missing validation finding path=%s message=%s findings=%+v", path, message, findings)
}
