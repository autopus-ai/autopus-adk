package adapter_test

import (
	"context"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/insajin/autopus-adk/pkg/adapter"
	"github.com/insajin/autopus-adk/pkg/adapter/antigravity"
	"github.com/insajin/autopus-adk/pkg/adapter/claude"
	"github.com/insajin/autopus-adk/pkg/adapter/codex"
	"github.com/insajin/autopus-adk/pkg/adapter/opencode"
	"github.com/insajin/autopus-adk/pkg/config"
)

// generatedTree holds the on-disk files produced by an adapter's Generate call
// for a single platform, keyed by repo-relative slash path.
type generatedTree struct {
	platform string
	files    map[string]string // relPath -> content
}

// generateTree runs Generate into a fresh TempDir and walks the produced tree.
// The on-disk output is the real oracle for the regression-0 guarantee
// (SPEC-HARNESS-WORKFLOW-001 REQ-005 / S3): it is what the user actually gets.
func generateTree(t *testing.T, platform string, gen func(root string) (*adapter.PlatformFiles, error)) generatedTree {
	t.Helper()
	root := t.TempDir()
	_, err := gen(root)
	require.NoError(t, err, "%s Generate must succeed", platform)

	tree := generatedTree{platform: platform, files: map[string]string{}}
	walkErr := filepath.WalkDir(root, func(path string, d os.DirEntry, err error) error {
		if err != nil {
			return err
		}
		if d.IsDir() {
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
		tree.files[filepath.ToSlash(rel)] = string(data)
		return nil
	})
	require.NoError(t, walkErr, "%s tree walk", platform)
	require.NotEmpty(t, tree.files, "%s must produce files", platform)
	return tree
}

// workflowJSCount counts files whose base name contains "workflow" and ends ".js".
func (g generatedTree) workflowJSCount() int {
	n := 0
	for rel := range g.files {
		base := strings.ToLower(filepath.Base(rel))
		if strings.Contains(base, "workflow") && strings.HasSuffix(base, ".js") {
			n++
		}
	}
	return n
}

// workflowFlagOccurrences counts "--workflow" substring occurrences across all content.
func (g generatedTree) workflowFlagOccurrences() int {
	n := 0
	for _, content := range g.files {
		n += strings.Count(content, "--workflow")
	}
	return n
}

// hasHarnessWorkflowSkill reports whether a harness-workflow skill file is present.
func (g generatedTree) hasHarnessWorkflowSkill() bool {
	for rel := range g.files {
		if strings.Contains(strings.ToLower(rel), "harness-workflow") {
			return true
		}
	}
	return false
}

// hasRouteAGoSurface reports whether the Route A `/auto go` pipeline surface is
// present. Non-claude platforms reference the "auto-go" detail skill/prompt;
// the claude router embeds the route inline as "/auto go".
func (g generatedTree) hasRouteAGoSurface() bool {
	for _, content := range g.files {
		if strings.Contains(content, "auto-go") ||
			strings.Contains(content, "/auto go") ||
			strings.Contains(content, "Route A") {
			return true
		}
	}
	return false
}

func ctx() context.Context { return context.Background() }

func cfgFull() *config.HarnessConfig { return config.DefaultFullConfig("workflow-parity-test") }

// TestWorkflowParity_NonClaudeHasNoWorkflowSurface is the S3 regression-0 oracle:
// codex/gemini/opencode output MUST contain no workflow*.js, no `--workflow`
// token, and no harness-workflow skill, while Route A stays intact.
func TestWorkflowParity_NonClaudeHasNoWorkflowSurface(t *testing.T) {
	t.Parallel()

	nonClaude := []struct {
		name string
		gen  func(root string) (*adapter.PlatformFiles, error)
	}{
		{"codex", func(root string) (*adapter.PlatformFiles, error) {
			return codex.NewWithRoot(root).Generate(ctx(), cfgFull())
		}},
		{"gemini", func(root string) (*adapter.PlatformFiles, error) {
			return antigravity.NewWithRoot(root).Generate(ctx(), cfgFull())
		}},
		{"opencode", func(root string) (*adapter.PlatformFiles, error) {
			return opencode.NewWithRoot(root, opencode.WithCLIVersion("1.18.7")).Generate(ctx(), cfgFull())
		}},
	}

	for _, tc := range nonClaude {
		tc := tc
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()
			tree := generateTree(t, tc.name, tc.gen)

			assert.Equal(t, 0, tree.workflowJSCount(),
				"%s MUST emit 0 workflow*.js files (claude-only surface)", tc.name)
			assert.Equal(t, 0, tree.workflowFlagOccurrences(),
				"%s MUST contain 0 `--workflow` tokens (claude-only route)", tc.name)
			assert.False(t, tree.hasHarnessWorkflowSkill(),
				"%s MUST NOT contain a harness-workflow skill file", tc.name)
			assert.True(t, tree.hasRouteAGoSurface(),
				"%s MUST preserve the Route A /auto go surface", tc.name)
		})
	}
}

// TestWorkflowParity_ClaudeHasWorkflowSurface is the positive counterpart: it
// proves the scoping is claude-only (not global suppression). Claude DOES emit
// the generated route_a.workflow.js and DOES include the harness-workflow skill.
func TestWorkflowParity_ClaudeHasWorkflowSurface(t *testing.T) {
	t.Parallel()
	tree := generateTree(t, "claude", func(root string) (*adapter.PlatformFiles, error) {
		return claude.NewWithRoot(root).Generate(ctx(), cfgFull())
	})

	_, hasWorkflowJS := tree.files[".claude/workflows/route_a.workflow.js"]
	assert.True(t, hasWorkflowJS,
		"claude MUST emit .claude/workflows/route_a.workflow.js")
	assert.GreaterOrEqual(t, tree.workflowJSCount(), 1,
		"claude MUST emit at least one workflow*.js file")
	assert.True(t, tree.hasHarnessWorkflowSkill(),
		"claude MUST include the harness-workflow skill file")
	assert.True(t, tree.hasRouteAGoSurface(),
		"claude MUST preserve the Route A /auto go surface")
}
