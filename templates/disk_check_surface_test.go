package templates_test

import (
	"path/filepath"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/insajin/autopus-adk/pkg/config"
	tmpl "github.com/insajin/autopus-adk/pkg/template"
)

// A disk preflight named on one surface and missing on another lets that
// platform fan workers out onto a nearly full shared disk (SPEC-DISK-001
// REQ-DISK-GUIDE-01). Every surface where a heavy step starts carries the
// command and the exit code it stops on.
func TestDiskCheckSurfaceParity(t *testing.T) {
	t.Parallel()

	root := templateRoot()
	e := tmpl.New()
	cfg := config.DefaultFullConfig("disk-project")
	paths := map[string]string{
		"delegation":                    filepath.Join(root, "..", "content", "skills", "references", "agent-pipeline", "delegation.md"),
		"verification":                  filepath.Join(root, "..", "content", "skills", "references", "agent-pipeline", "verification.md"),
		"worktree-isolation-content":    filepath.Join(root, "..", "content", "skills", "worktree-isolation.md"),
		"worktree-isolation-gemini":     filepath.Join(root, "gemini", "skills", "worktree-isolation", "SKILL.md.tmpl"),
		"supervisor-worktree-isolation": filepath.Join(root, "shared", "supervisor-worktree-isolation.md.tmpl"),
		"omp-worktree-isolation":        filepath.Join(root, "shared", "omp-worktree-isolation.md.tmpl"),
	}
	for name, path := range paths {
		name, path := name, path
		t.Run(name, func(t *testing.T) {
			t.Parallel()
			text, err := semanticContractSurface(e, path, cfg)
			require.NoError(t, err)
			assert.Contains(t, text, "auto disk check --need", "%s should run the disk preflight", path)
			assert.Contains(t, text, "exits 3", "%s should stop on the insufficient exit code", path)
		})
	}
}

// The old worker-hygiene bullet named `df` and a fixed 5 GiB floor; the
// preflight replaces it, keeping `df` only as the fallback for an older auto.
func TestDiskCheckSurface_DelegationDropsTheFixedDfFloor(t *testing.T) {
	t.Parallel()

	text, err := semanticContractSurface(tmpl.New(),
		filepath.Join(templateRoot(), "..", "content", "skills", "references", "agent-pipeline", "delegation.md"),
		config.DefaultFullConfig("disk-project"))
	require.NoError(t, err)
	assert.NotContains(t, text, "below 5 GiB")
}
