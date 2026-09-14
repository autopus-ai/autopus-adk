package claude_test

import (
	"context"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/insajin/autopus-adk/pkg/adapter/claude"
	"github.com/insajin/autopus-adk/pkg/config"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

var frozenAutoRoutes = []string{
	"setup", "status", "goal", "update", "plan", "go", "fix", "review", "sync",
	"idea", "map", "why", "verify", "secure", "test", "qa", "dev", "canary", "doctor",
}

func TestRouterBudget_FullGenerate_RootIsThinAndEveryRouteHasOneDetail(t *testing.T) {
	t.Parallel()
	root := t.TempDir()
	_, err := claude.NewWithRoot(root).Generate(context.Background(), config.DefaultFullConfig("router-budget"))
	require.NoError(t, err)

	router := readClaudeSurface(t, root, ".claude/skills/auto/SKILL.md")
	t.Logf("generated Claude root router: %d bytes", len([]byte(router)))
	assert.LessOrEqual(t, len([]byte(router)), 8192, "root router must stay within the byte budget")
	for _, route := range frozenAutoRoutes {
		detailPath := filepath.Join(".claude", "skills", "auto-"+route, "SKILL.md")
		assert.Equal(t, 1, strings.Count(router, detailPath), "route %q must resolve exactly one detail", route)
		detail := readClaudeSurface(t, root, detailPath)
		assert.Contains(t, detail, "# auto-"+route)
	}
	for _, alias := range []string{"browse", "stale", "spec review", "init", "platform"} {
		assert.Contains(t, router, alias, "legacy alias %q must remain routable", alias)
	}
	assert.NotContains(t, router, "Triage Process")
	assert.NoFileExists(t, filepath.Join(root, ".claude", "skills", "auto-workflows", "SKILL.md"))
}

func TestWorkflowSkills_UpdateAndGenerate_ProduceMatchingDetails(t *testing.T) {
	t.Parallel()
	cfg := config.DefaultFullConfig("router-parity")
	generateRoot := t.TempDir()
	updateRoot := t.TempDir()

	_, err := claude.NewWithRoot(generateRoot).Generate(context.Background(), cfg)
	require.NoError(t, err)
	_, err = claude.NewWithRoot(updateRoot).Update(context.Background(), cfg)
	require.NoError(t, err)

	for _, route := range frozenAutoRoutes {
		rel := filepath.Join(".claude", "skills", "auto-"+route, "SKILL.md")
		assert.Equal(t, readClaudeSurface(t, generateRoot, rel), readClaudeSurface(t, updateRoot, rel), rel)
	}
}

func readClaudeSurface(t *testing.T, root, rel string) string {
	t.Helper()
	body, err := os.ReadFile(filepath.Join(root, rel))
	require.NoError(t, err, rel)
	return string(body)
}
