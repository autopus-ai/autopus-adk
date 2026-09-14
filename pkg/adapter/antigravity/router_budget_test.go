package antigravity

import (
	"context"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/insajin/autopus-adk/pkg/adapter"
	"github.com/insajin/autopus-adk/pkg/config"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

var frozenAntigravityAutoRoutes = []string{
	"setup", "status", "goal", "update", "plan", "go", "fix", "review", "sync",
	"idea", "map", "why", "verify", "secure", "test", "qa", "dev", "canary", "doctor",
}

func TestRouterBudget_FullGenerate_RootIsThinAndAllDetailsExist(t *testing.T) {
	t.Parallel()
	root := t.TempDir()
	_, err := NewWithRoot(root).Generate(context.Background(), config.DefaultFullConfig("router-budget"))
	require.NoError(t, err)

	router := readGeneratedAntigravitySurface(t, root, filepath.Join(".gemini", "skills", "auto", "SKILL.md"))
	t.Logf("generated Gemini root router: %d bytes", len([]byte(router)))
	assert.LessOrEqual(t, len([]byte(router)), 8192, "root router must stay within the byte budget")
	for _, route := range frozenAntigravityAutoRoutes {
		rel := filepath.Join(".gemini", "skills", "autopus", "auto-"+route, "SKILL.md")
		assert.Equal(t, 1, strings.Count(router, rel), "route %q must resolve exactly one detail", route)
		detail := readGeneratedAntigravitySurface(t, root, rel)
		assert.Contains(t, detail, "name: auto-"+route)
	}
	for _, alias := range []string{"browse", "stale", "spec review", "init", "platform"} {
		assert.Contains(t, router, alias, "legacy alias %q must remain routable", alias)
	}
	assert.NotContains(t, router, "Triage Process")
}

func TestRouterBudget_FullGenerate_CommandSurfaceCoversEveryFrozenRoute(t *testing.T) {
	t.Parallel()
	root := t.TempDir()
	_, err := NewWithRoot(root).Generate(context.Background(), config.DefaultFullConfig("command-surface"))
	require.NoError(t, err)

	entries, err := os.ReadDir(filepath.Join(root, ".gemini", "commands", "auto"))
	require.NoError(t, err)

	emitted := make([]string, 0, len(entries))
	for _, entry := range entries {
		require.False(t, entry.IsDir(), "command surface must stay flat: %s", entry.Name())
		emitted = append(emitted, strings.TrimSuffix(entry.Name(), ".toml"))
	}
	assert.ElementsMatch(t, frozenAntigravityAutoRoutes, emitted,
		"every frozen /auto route must have exactly one .gemini/commands/auto/<route>.toml")

	for _, route := range frozenAntigravityAutoRoutes {
		command := readGeneratedAntigravitySurface(t, root, filepath.Join(".gemini", "commands", "auto", route+".toml"))
		assert.Contains(t, command, "prompt", "route %q command must declare a prompt", route)
		assert.Contains(t, command, ".gemini/skills/autopus/auto-"+route+"/SKILL.md",
			"route %q command must load exactly its own detail", route)
	}

	rootCommand := readGeneratedAntigravitySurface(t, root, filepath.Join(".gemini", "commands", "auto.toml"))
	assert.Contains(t, rootCommand, ".gemini/skills/auto/SKILL.md",
		"auto.toml must keep routing through the thin router skill")
}

func TestWorkflowSkills_MissingElevenRoutes_AreBoundedContracts(t *testing.T) {
	t.Parallel()
	root := t.TempDir()
	_, err := NewWithRoot(root).Generate(context.Background(), config.DefaultFullConfig("route-gaps"))
	require.NoError(t, err)

	missingBeforeT5 := []string{"setup", "status", "goal", "update", "map", "why", "verify", "secure", "test", "dev", "doctor"}
	for _, route := range missingBeforeT5 {
		rel := filepath.Join(".gemini", "skills", "autopus", "auto-"+route, "SKILL.md")
		body := readGeneratedAntigravitySurface(t, root, rel)
		assert.LessOrEqual(t, len([]byte(body)), 8192, "%s detail must remain bounded", route)
		assert.Contains(t, body, "## Context Profile")
		assert.Contains(t, body, "## Contract")
	}
}

func TestWorkflowSkills_UpdateAndGenerate_ProduceMatchingDetails(t *testing.T) {
	t.Parallel()
	cfg := config.DefaultFullConfig("route-parity")
	generateRoot := t.TempDir()
	updateRoot := t.TempDir()

	_, err := NewWithRoot(generateRoot).Generate(context.Background(), cfg)
	require.NoError(t, err)
	_, err = NewWithRoot(updateRoot).Update(context.Background(), cfg)
	require.NoError(t, err)

	for _, route := range frozenAntigravityAutoRoutes {
		rel := filepath.Join(".gemini", "skills", "autopus", "auto-"+route, "SKILL.md")
		assert.Equal(t,
			readGeneratedAntigravitySurface(t, generateRoot, rel),
			readGeneratedAntigravitySurface(t, updateRoot, rel),
			rel,
		)
	}
}

func TestWorkflowSkills_GenerateAndUpdate_DoNotDuplicateCanonicalRouteTargets(t *testing.T) {
	t.Parallel()
	cfg := config.DefaultFullConfig("route-ownership")
	for _, operation := range []struct {
		name string
		run  func(string) ([]string, error)
	}{
		{name: "generate", run: func(root string) ([]string, error) {
			files, err := NewWithRoot(root).Generate(context.Background(), cfg)
			if err != nil {
				return nil, err
			}
			return antigravityMappingPaths(files.Files), nil
		}},
		{name: "update", run: func(root string) ([]string, error) {
			files, err := NewWithRoot(root).Update(context.Background(), cfg)
			if err != nil {
				return nil, err
			}
			return antigravityMappingPaths(files.Files), nil
		}},
	} {
		operation := operation
		t.Run(operation.name, func(t *testing.T) {
			paths, err := operation.run(t.TempDir())
			require.NoError(t, err)
			for _, target := range []string{
				filepath.Join(".gemini", "skills", "autopus", "auto-setup", "SKILL.md"),
				filepath.Join(antigravityPluginDir, "skills", "auto-setup", "SKILL.md"),
			} {
				assert.Equal(t, 1, countAntigravityPath(paths, target), target)
			}
		})
	}
}

func TestWorkflowSkills_GenerateAndUpdate_RespectRouteAndSharedSkillOwnership(t *testing.T) {
	t.Parallel()
	cfg := config.DefaultFullConfig("skill-ownership")
	// adaptive-quality is a long-tail catalog skill; the ownership contract
	// under test only has something to say when the full catalog is opted in.
	cfg.Skills.Compiler.Mode = config.SkillCompilerModeFull
	operations := []struct {
		name string
		run  func(string) (*adapter.PlatformFiles, error)
	}{
		{name: "generate", run: func(root string) (*adapter.PlatformFiles, error) {
			return NewWithRoot(root).Generate(context.Background(), cfg)
		}},
		{name: "update", run: func(root string) (*adapter.PlatformFiles, error) {
			return NewWithRoot(root).Update(context.Background(), cfg)
		}},
	}

	for _, operation := range operations {
		operation := operation
		t.Run(operation.name, func(t *testing.T) {
			t.Parallel()
			root := t.TempDir()
			files, err := operation.run(root)
			require.NoError(t, err)

			paths := antigravityMappingPaths(files.Files)
			seen := make(map[string]struct{}, len(paths))
			for _, path := range paths {
				_, duplicate := seen[path]
				assert.False(t, duplicate, "duplicate target path: %s", path)
				seen[path] = struct{}{}
			}

			for _, base := range []string{
				filepath.Join(".gemini", "skills", "autopus"),
				filepath.Join(antigravityPluginDir, "skills"),
			} {
				adaptivePath := filepath.Join(base, "adaptive-quality", "SKILL.md")
				autoGoPath := filepath.Join(base, "auto-go", "SKILL.md")
				assert.Equal(t, 1, countAntigravityPath(paths, adaptivePath), adaptivePath)
				assert.Equal(t, 1, countAntigravityPath(paths, autoGoPath), autoGoPath)

				adaptive := readGeneratedAntigravitySurface(t, root, adaptivePath)
				autoGo := readGeneratedAntigravitySurface(t, root, autoGoPath)
				assert.Equal(t, "adaptive-quality", generatedAntigravitySkillName(adaptivePath, adaptive))
				assert.Equal(t, "auto-go", generatedAntigravitySkillName(autoGoPath, autoGo))
				assert.Contains(t, autoGo, "\nplatform: antigravity-cli\n")
			}
		})
	}
}

func generatedAntigravitySkillName(path, content string) string {
	if strings.HasPrefix(content, "---\n") {
		if end := strings.Index(content[4:], "\n---"); end >= 0 {
			for _, line := range strings.Split(content[4:4+end], "\n") {
				if value, ok := strings.CutPrefix(strings.TrimSpace(line), "name:"); ok {
					return strings.TrimSpace(value)
				}
			}
		}
	}
	return filepath.Base(filepath.Dir(path))
}

func antigravityMappingPaths(files []adapter.FileMapping) []string {
	paths := make([]string, 0, len(files))
	for _, file := range files {
		paths = append(paths, filepath.Clean(file.TargetPath))
	}
	return paths
}

func countAntigravityPath(paths []string, target string) int {
	count := 0
	for _, path := range paths {
		if path == filepath.Clean(target) {
			count++
		}
	}
	return count
}
