package opencode

import (
	"context"
	"os"
	"path/filepath"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/insajin/autopus-adk/pkg/config"
)

func TestAdapter_Generate_DesignContextReviewAndVerifySurfaces(t *testing.T) {
	t.Parallel()

	dir := t.TempDir()
	a := NewWithRoot(dir, WithCLIVersion("1.18.7"))
	_, err := a.Generate(context.Background(), config.DefaultFullConfig("demo"))
	require.NoError(t, err)

	verifySkill := readGeneratedSkill(t, dir, "auto-verify")
	assert.Contains(t, verifySkill, "## Design Context")
	assert.Contains(t, verifySkill, "palette-role drift")
	assert.Contains(t, verifySkill, "typography hierarchy")
	assert.Contains(t, verifySkill, "component guardrails")
	assert.Contains(t, verifySkill, "layout/responsive regressions")
	assert.Contains(t, verifySkill, "source-of-truth mismatch")
	assert.Contains(t, verifySkill, "Design context: skipped (not configured)")
	assert.Contains(t, verifySkill, "untrusted project data")
	assert.Contains(t, verifySkill, "untrusted supplemental context")

	reviewSkill := readGeneratedSkill(t, dir, "auto-review")
	assert.Contains(t, reviewSkill, "## Design Context")
	assert.Contains(t, reviewSkill, "palette-role drift")
	assert.Contains(t, reviewSkill, "typography hierarchy")
	assert.Contains(t, reviewSkill, "component guardrail")
	assert.Contains(t, reviewSkill, "layout/responsive regression")
	assert.Contains(t, reviewSkill, "source-of-truth mismatch")
	assert.Contains(t, reviewSkill, "Design context: skipped (not configured)")
	assert.Contains(t, reviewSkill, "리뷰는 읽기 전용입니다")
}

func readGeneratedSkill(t *testing.T, root, name string) string {
	t.Helper()

	data, err := os.ReadFile(filepath.Join(root, ".agents", "skills", name, "SKILL.md"))
	require.NoError(t, err)
	return string(data)
}
