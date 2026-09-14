package omp

import (
	"context"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/insajin/autopus-adk/pkg/adapter"
	"github.com/insajin/autopus-adk/pkg/config"
)

func manifestPaths(t *testing.T, dir string) []string {
	t.Helper()
	m, err := adapter.LoadManifest(dir, adapterName)
	require.NoError(t, err)
	require.NotNil(t, m, "omp manifest must exist after Generate")
	paths := make([]string, 0, len(m.Files))
	for p := range m.Files {
		paths = append(paths, filepath.ToSlash(p))
	}
	return paths
}

func countPathsWithPrefix(paths []string, prefix string) int {
	n := 0
	for _, p := range paths {
		if strings.HasPrefix(p, prefix) {
			n++
		}
	}
	return n
}

func TestOMPAcceptance_PlainInstallOmitsBaseConfig(t *testing.T) {
	dir := generateOMPOnly(t)
	assert.NoFileExists(t, filepath.Join(dir, configFile))
	assert.NotContains(t, manifestPaths(t, dir), configFile)
}

// TestOMPAcceptance_S6_RuleAndAgentCounts covers REQ-002/REQ-005 surface scope.
func TestOMPAcceptance_S6_RuleAndAgentCounts(t *testing.T) {
	dir := generateOMPOnly(t)
	paths := manifestPaths(t, dir)

	assert.Equal(t, 14, countPathsWithPrefix(paths, ompRuleDir+"/"+ompRuleFilePrefix),
		"manifest must record 14 rules as .omp/rules/autopus-*.md")
	assert.Equal(t, 0, countPathsWithPrefix(paths, ".omp/agents/"),
		"agent definitions are OMP's own; a managed file there would shadow a bundled agent")
	assert.Equal(t, len(workflowSpecs), countPathsWithPrefix(paths, ".omp/commands/"))
	assert.Greater(t, countPathsWithPrefix(paths, ".omp/skills/"), len(workflowSpecs))
	assert.NotContains(t, paths, configFile)
}

// TestOMPAcceptance_S5_NoClaudeShadowing covers REQ-008 and the InstallHooks contract.
func TestOMPAcceptance_S5_NoClaudeShadowing(t *testing.T) {
	dir := t.TempDir()
	agentsMD := filepath.Join(dir, "AGENTS.md")
	require.NoError(t, os.WriteFile(agentsMD, []byte("# Root context\n"), 0o644))
	before, err := os.ReadFile(agentsMD)
	require.NoError(t, err)

	cfg := config.DefaultFullConfig("omp-acceptance")
	cfg.Platforms = []string{"omp"}
	require.NoError(t, config.Save(dir, cfg))

	a := NewWithRoot(dir)
	ctx := context.Background()
	_, err = a.Generate(ctx, cfg)
	require.NoError(t, err)
	_, err = a.Update(ctx, cfg)
	require.NoError(t, err)

	assert.False(t, a.SupportsHooks(), "omp declares no hook support")

	snapshot := snapshotTree(t, dir)
	require.NoError(t, a.InstallHooks(ctx, nil, nil), "InstallHooks must return without error")
	assert.Equal(t, snapshot, snapshotTree(t, dir),
		"InstallHooks must not create or modify any file")

	assert.NoFileExists(t, filepath.Join(dir, ".claude", "CLAUDE.md"))
	for _, p := range manifestPaths(t, dir) {
		assert.False(t, strings.Contains(p, ".claude/"),
			"manifest must not record a .claude/ path, found %q", p)
	}

	after, err := os.ReadFile(agentsMD)
	require.NoError(t, err)
	assert.Equal(t, before, after, "root AGENTS.md must stay byte-identical")
}

// snapshotTree records every regular file path and its bytes below root.
func snapshotTree(t *testing.T, root string) map[string]string {
	t.Helper()
	out := make(map[string]string)
	err := filepath.Walk(root, func(path string, info os.FileInfo, err error) error {
		if err != nil {
			return err
		}
		if info.IsDir() {
			return nil
		}
		data, readErr := os.ReadFile(path)
		if readErr != nil {
			return readErr
		}
		rel, relErr := filepath.Rel(root, path)
		if relErr != nil {
			return relErr
		}
		out[filepath.ToSlash(rel)] = string(data)
		return nil
	})
	require.NoError(t, err)
	return out
}

// TestOMPAcceptance_S4_OwnershipBoundaryAndManifestScope covers REQ-001/REQ-009.
func TestOMPAcceptance_S4_OwnershipBoundaryAndManifestScope(t *testing.T) {
	dir := t.TempDir()
	cfg := config.DefaultFullConfig("omp-acceptance")
	cfg.Platforms = []string{"opencode", "omp"}
	require.NoError(t, config.Save(dir, cfg))

	// opencode already owns the shared skill surface.
	opencodeSkill := filepath.Join(dir, ".agents", "skills", "auto", "SKILL.md")
	require.NoError(t, os.MkdirAll(filepath.Dir(opencodeSkill), 0o755))
	require.NoError(t, os.WriteFile(opencodeSkill, []byte("opencode owned skill\n"), 0o644))

	// User-authored rules that omp never creates. `.agents/rules/` is no longer an
	// omp target at all; `.agents/rules/autopus/` is a legacy prune root kept only
	// so an upgrade removes what an older manifest recorded there.
	userRule := filepath.Join(dir, ".agents", "rules", "my-own-rule.md")
	require.NoError(t, os.MkdirAll(filepath.Dir(userRule), 0o755))
	require.NoError(t, os.WriteFile(userRule, []byte("user rule\n"), 0o644))
	userNamespacedRule := filepath.Join(dir, ".agents", "rules", "autopus", "user-extra.md")
	require.NoError(t, os.MkdirAll(filepath.Dir(userNamespacedRule), 0o755))
	require.NoError(t, os.WriteFile(userNamespacedRule, []byte("user extra\n"), 0o644))

	a := NewWithRoot(dir)
	ctx := context.Background()
	_, err := a.Generate(ctx, cfg)
	require.NoError(t, err)

	paths := manifestPaths(t, dir)
	assert.Equal(t, 14, countPathsWithPrefix(paths, ompRuleDir+"/"+ompRuleFilePrefix))
	assert.Equal(t, 0, countPathsWithPrefix(paths, ".omp/agents/"))
	assert.Equal(t, len(workflowSpecs), countPathsWithPrefix(paths, ".omp/commands/"))
	assert.Greater(t, countPathsWithPrefix(paths, ".omp/skills/"), len(workflowSpecs))
	assert.NotContains(t, paths, configFile)
	for _, forbidden := range []string{".agents/skills/", ".agents/commands/", ".agents/plugins/", ".agents/hooks.json"} {
		assert.Equal(t, 0, countPathsWithPrefix(paths, forbidden),
			"manifest must not record a foreign path under %s", forbidden)
	}

	require.NoError(t, a.Clean(ctx))

	skillAfter, err := os.ReadFile(opencodeSkill)
	require.NoError(t, err, "opencode skill file must survive omp Clean")
	assert.Equal(t, "opencode owned skill\n", string(skillAfter))

	userRuleAfter, err := os.ReadFile(userRule)
	require.NoError(t, err)
	assert.Equal(t, "user rule\n", string(userRuleAfter))
	userNamespacedAfter, err := os.ReadFile(userNamespacedRule)
	require.NoError(t, err)
	assert.Equal(t, "user extra\n", string(userNamespacedAfter))
}
