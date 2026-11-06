package claude

import (
	"context"
	"path/filepath"
	"sort"
	"strings"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/insajin/autopus-adk/pkg/adapter"
	"github.com/insajin/autopus-adk/pkg/config"
)

// ruleTargets returns the sorted rule-related target paths of a mapping set.
func ruleTargets(files []adapter.FileMapping) []string {
	targets := make([]string, 0, len(files))
	for _, file := range files {
		target := filepath.ToSlash(file.TargetPath)
		if !strings.Contains(target, "rules") {
			continue
		}
		targets = append(targets, target)
	}
	sort.Strings(targets)
	return targets
}

// TestClaudeRuleRoutingAgreesAcrossPaths verifies Generate and the pure mapping
// compiler expose the same rule targets.
func TestClaudeRuleRoutingAgreesAcrossPaths(t *testing.T) {
	t.Parallel()
	cfg := config.DefaultFullConfig("condrule")

	generated, err := NewWithRoot(t.TempDir()).Generate(context.Background(), cfg)
	require.NoError(t, err)
	prepared, err := NewWithRoot(t.TempDir()).prepareFiles(cfg)
	require.NoError(t, err)

	assert.Equal(t, ruleTargets(generated.Files), ruleTargets(prepared))
}

// TestClaudeConditionalSurfaceRoutesEachClassOnce asserts the routing decision
// itself: a relocated rule leaves the copy path exactly once, and the rule the
// template path owns is never emitted twice.
func TestClaudeConditionalSurfaceRoutesEachClassOnce(t *testing.T) {
	t.Parallel()

	surface, err := claudeConditionalRules()
	require.NoError(t, err)

	assert.True(t, surface.relocates("lore-commit.md"))
	assert.True(t, surface.relocates(fileSizeLimitRuleFile),
		"a paths-scoped rule also leaves the verbatim copy path")
	assert.False(t, surface.relocates("language-policy.md"),
		"an always rule keeps its unchanged copy path")

	for _, file := range surface.mappings {
		assert.NotEqual(t, fileSizeLimitRuleFile, filepath.Base(file.TargetPath),
			"file-size-limit is rendered from its template, never compiled")
	}
}
