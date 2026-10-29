package omp

import (
	"context"
	"path/filepath"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/insajin/autopus-adk/pkg/config"
	pkgcontent "github.com/insajin/autopus-adk/pkg/content"
	"github.com/insajin/autopus-adk/templates"
)

func generatedOMPBodies(t *testing.T) map[string]string {
	t.Helper()
	root := t.TempDir()
	cfg := config.DefaultFullConfig("omp-generated-bodies")
	cfg.Platforms = []string{"omp"}
	require.NoError(t, config.Save(root, cfg))
	generated, err := NewWithRoot(root).Generate(context.Background(), cfg)
	require.NoError(t, err)
	bodies := make(map[string]string, len(generated.Files))
	for _, file := range generated.Files {
		bodies[filepath.ToSlash(file.TargetPath)] = string(file.Content)
	}
	return bodies
}

// The retired autopus_* model aliases only existed to bind generated agent
// definitions. No generated OMP surface may still reference one.
func TestOMPGeneratedSurfacesCarryNoRetiredModelAlias(t *testing.T) {
	for path, body := range generatedOMPBodies(t) {
		assert.NotContains(t, body, "@autopus_", path)
		assert.NotContains(t, body, "modelRoles", path)
	}
}

// Isolated fan-out runs on the bundled default agent. The native template is
// the seam that decides this, so it is read through the same normalization the
// generator applies rather than depending on the skill being compiled in.
func TestOMPIsolationTemplateTargetsBundledDefault(t *testing.T) {
	raw, err := templates.FS.ReadFile(ompNativeExtendedSkillTemplates["worktree-isolation"])
	require.NoError(t, err)
	body := pkgcontent.NormalizeOMPSemanticReferences(string(raw))

	assert.Contains(t, body, "omit the agent field")
	assert.Contains(t, body, "bundled `task` agent")
	assert.Contains(t, body, "`security-reviewer`")
	assert.NotContains(t, body, "a discovered custom role")
	assert.NotContains(t, body, "Autopus executor")
}
