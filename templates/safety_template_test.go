package templates_test

import (
	"path/filepath"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/insajin/autopus-adk/pkg/config"
	tmpl "github.com/insajin/autopus-adk/pkg/template"
)

func TestWorktreeSafetyTemplateContracts(t *testing.T) {
	t.Parallel()

	e := tmpl.New()
	cfg := config.DefaultFullConfig("worktree-safety-project")
	root := templateRoot()
	templatePaths := []string{
		filepath.Join(root, "claude", "commands", "auto-router.md.tmpl"),
		filepath.Join(root, "gemini", "commands", "auto-router.md.tmpl"),
		filepath.Join(root, "gemini", "skills", "worktree-isolation", "SKILL.md.tmpl"),
	}
	expected := []string{
		"fifo_task_id",
		"worktree_slot_cap",
		"Slot reclaim",
		"worktree_isolation_unavailable",
		"preserved_for_manual_review",
	}

	for _, path := range templatePaths {
		path := path
		t.Run(filepath.Base(path), func(t *testing.T) {
			t.Parallel()
			result, err := semanticContractSurface(e, path, cfg)
			require.NoError(t, err)
			for _, phrase := range expected {
				assert.Contains(t, result, phrase)
			}
		})
	}
}
