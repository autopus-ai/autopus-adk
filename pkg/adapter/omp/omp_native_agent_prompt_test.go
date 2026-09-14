package omp

import (
	"context"
	"path/filepath"
	"strings"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/insajin/autopus-adk/pkg/config"
	pkgcontent "github.com/insajin/autopus-adk/pkg/content"
	"github.com/insajin/autopus-adk/templates"
)

// retiredOMPDispatchRoles are the per-role agent names OMP never registered.
// A generated prompt that dispatches to one of them names an agent that does
// not exist, and OMP would refuse the call.
var retiredOMPDispatchRoles = []string{
	"annotator", "architect", "debugger", "deep-worker", "devops", "executor",
	"explorer", "frontend-specialist", "perf-engineer", "planner",
	"security-auditor", "spec-writer", "tester", "ux-validator", "validator",
}

func generatedOMPBodies(t *testing.T) map[string]string {
	t.Helper()
	root := t.TempDir()
	cfg := config.DefaultFullConfig("omp-native-agents")
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

// The pipeline entrypoint is what actually decides which agent a worker runs
// on, so it must name the bundled registry and say what each agent is for.
func TestOMPGeneratedPipelineSkillSelectsBundledAgentsOnly(t *testing.T) {
	body, ok := generatedOMPBodies(t)[".omp/skills/agent-pipeline/SKILL.md"]
	require.True(t, ok, "the OMP pipeline skill must be generated")

	for _, native := range config.OMPNativeAgentNames() {
		assert.Contains(t, body, "`"+native+"`", native)
	}
	assert.Contains(t, body, "read-only exploration")
	assert.Contains(t, body, "explicit mechanical request")
	assert.Contains(t, body, config.OMPNativeAgentModelOverridesKey)
	// Responsibility belongs in the assignment, not in an agent name.
	assert.Contains(t, body, "belongs in its assignment text")
}

// No generated OMP surface may instruct a dispatch to a name OMP does not
// register. Role words may still appear as prose; a dispatch target may not.
func TestOMPGeneratedSurfacesNeverDispatchToRetiredRoles(t *testing.T) {
	bodies := generatedOMPBodies(t)
	require.NotEmpty(t, bodies)

	for path, body := range bodies {
		if !strings.HasSuffix(path, ".md") {
			continue
		}
		for _, role := range retiredOMPDispatchRoles {
			assert.NotContains(t, body, `"agent": "`+role+`"`, path)
			assert.NotContains(t, body, "selecting agent `"+role+"`", path)
		}
		assert.NotContains(t, body, `"agent": "sonic"`,
			"sonic is reserved for an explicit mechanical request: %s", path)
	}
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
