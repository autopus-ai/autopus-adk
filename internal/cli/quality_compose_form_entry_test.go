package cli

// SPEC-MODELUX-002 T7 (REQ-MUX2-01, REQ-MUX2-14, REQ-MUX2-16; acceptance S2,
// S4, S5, S6, S8, S12, S14): the real `auto quality` takes the form flow in
// a terminal, generates its approved preview with --apply from the run's one
// catalog probe, and ends quietly on Ctrl+C.

import (
	"bytes"
	"io"
	"path/filepath"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/insajin/autopus-adk/pkg/config"
)

// runQualityForms runs the real `auto quality` over dir with the answers as
// form input; deps, when set, replaces the OMP dependencies.
func runQualityForms(t *testing.T, dir string, deps *ompPlatformDependencies, in io.Reader, args ...string) (string, error) {
	t.Helper()
	root := NewRootCmd()
	if deps != nil {
		old, _, err := root.Find([]string{"quality"})
		require.NoError(t, err)
		root.RemoveCommand(old)
		root.AddCommand(newQualityCmdWithOMPDependencies(*deps))
	}
	var out bytes.Buffer
	root.SetOut(&out)
	root.SetErr(&out)
	root.SetIn(in)
	root.SetArgs(append([]string{"--config", filepath.Join(dir, "autopus.yaml"), "quality"}, args...))
	err := root.Execute()
	return out.String(), err
}

// S5 and S2: the Codex tab with --apply generates exactly the preview from
// the run's one probe; the agents keep the cells the table showed.
func TestQualityComposeForms_CodexTabApplyGeneratesThePreview(t *testing.T) {
	run := newComposeUpdateRun(t, true, "")
	useAccessibleComposeForms(t)
	dir := writeComposeFixture(t, "claude-code", "codex")

	out, err := runQualityForms(t, dir, nil, formInput(composeFormLines(composeFormS5, "2\n", "\n")...), "--apply")

	require.NoError(t, err, out)
	assert.Contains(t, out, "quality.applied_platforms = 2")
	_, codexMarks := assertGeneratedEqualsTable(t, dir, plainComposeHeaders(composeRenderedTable(t, out, composeFormPreviewMarker)))
	assert.Equal(t, 2, codexMarks)
	assert.Equal(t, config.CodexProfile{Model: "gpt-6-luna", Effort: "max"}, readComposeCodexProfile(t, dir, "reviewer"))
	assert.Equal(t, config.CodexProfile{Model: "gpt-6-luna", Effort: "max"}, readComposeCodexProfile(t, dir, "frontend-specialist"))
	assert.Equal(t, composeTableClaudeCell{model: "claude-opus-5-5", effort: "max"}, readComposeTableClaudeProfile(t, dir, "reviewer"))
	assert.Len(t, *run.probes, 1)
	assert.False(t, run.adapterProbed(t))
}

// S4: 모든 도구 같게 with --apply binds custom and generates its preview.
func TestQualityComposeForms_AllToolsApplyGeneratesThePreview(t *testing.T) {
	newComposeUpdateRun(t, true, "")
	useAccessibleComposeForms(t)
	dir := writeComposeFixture(t, "claude-code", "codex")

	out, err := runQualityForms(t, dir, nil, formInput("2\n", "1\n", "\n", "2\n", "\n", "2\n", "\n"), "--apply")

	require.NoError(t, err, out)
	assert.Contains(t, out, "quality.applied_platforms = 2")
	assertGeneratedEqualsTable(t, dir, plainComposeHeaders(composeRenderedTable(t, out, composeFormPreviewMarker)))
	assert.Equal(t, composeTableClaudeCell{model: "fable", effort: "max"}, readComposeTableClaudeProfile(t, dir, "frontend-specialist"))
	assert.Equal(t, config.CodexProfile{Model: "gpt-6-astra", Effort: "max"}, readComposeCodexProfile(t, dir, "frontend-specialist"))
	assert.Equal(t, "custom", composeBindLoad(t, dir).Quality.Default)
}

// S6: the Claude tab's per-agent edit with --apply.
func TestQualityComposeForms_ClaudeAgentApplyGeneratesThePreview(t *testing.T) {
	newComposeUpdateRun(t, true, "")
	useAccessibleComposeForms(t)
	dir := writeComposeFixture(t, "claude-code", "codex")

	out, err := runQualityForms(t, dir, nil, formInput("2\n", "2\n", "2\n", "2\n", "3\n", "\n", "\n", "2\n", "\n"), "--apply")

	require.NoError(t, err, out)
	claudeMarks, codexMarks := assertGeneratedEqualsTable(t, dir, plainComposeHeaders(composeRenderedTable(t, out, composeFormPreviewMarker)))
	// The top-rung reviewer keeps the top-rung cell under claude-custom.
	assert.Equal(t, 3, claudeMarks)
	assert.Zero(t, codexMarks)
	assert.Equal(t, composeTableClaudeCell{model: "opus", effort: "high"}, readComposeTableClaudeProfile(t, dir, "frontend-specialist"))
	assert.Equal(t, composeTableClaudeCell{model: "sonnet", effort: "medium"}, readComposeTableClaudeProfile(t, dir, "ux-validator"))
	assert.Equal(t, config.CodexProfile{Model: "gpt-6-luna", Effort: "max"}, readComposeCodexProfile(t, dir, "frontend-specialist"))
}

// S14: under CAT-B the opus option of 검증·리뷰 names the substitute its
// non-top-rung members take, the preview marks it, and the generation writes it.
func TestQualityComposeForms_SubstituteIsLabelledPreviewedAndGenerated(t *testing.T) {
	newComposeUpdateRun(t, true, "")
	composeSessionCatalog(t, "CAT-B")
	useAccessibleComposeForms(t)
	dir := writeComposeFixture(t, "claude-code", "codex")

	// 구성 편집, Codex, 역할 그룹, 구현 kept, 검증·리뷰 on opus, 적용.
	lines := []string{"2\n", "3\n", "\n", "\n", "\n", "3\n", "\n", "2\n", "\n"}
	out, err := runQualityForms(t, dir, nil, formInput(lines...), "--apply")

	require.NoError(t, err, out)
	assert.Contains(t, out, "3. 6-astra · max / 6-sol · xhigh (대체)  [opus]\n")
	preview := composeRenderedTable(t, out, composeFormPreviewMarker)
	assert.Equal(t, "6-sol · xhigh (대체)*", preview["ux-validator"]["Codex (개별 설정: codex-custom)"])
	// ux-validator and the analysis-rung qa-explorer run on the substitute;
	// reviewer keeps the top-rung Astra.
	assert.Contains(t, out, "Codex catalog: 읽음 · 대체 2칸\n")
	assert.Equal(t, config.CodexProfile{Model: "gpt-6-sol", Effort: "xhigh"}, readComposeCodexProfile(t, dir, "ux-validator"))
	assert.Equal(t, config.CodexProfile{Model: "gpt-6-astra", Effort: "max"}, readComposeCodexProfile(t, dir, "reviewer"))
}

// S12: Ctrl+C at the confirmation ends `auto quality` without an error and
// without a write.
func TestQualityComposeForms_InterruptEndsWithoutError(t *testing.T) {
	newComposeUpdateRun(t, true, "")
	abortComposeFormAt(t, 8)
	dir := writeComposeFixture(t, "claude-code", "codex")
	before := composeSessionHash(t, dir)

	out, err := runQualityForms(t, dir, nil, formInput(composeFormS5...), "--apply")

	require.NoError(t, err, out)
	assert.Contains(t, out, "중단했습니다. 구성을 더 바꾸지 않습니다.\n")
	assert.NotContains(t, out, "quality.applied_platforms")
	assert.Equal(t, before, composeSessionHash(t, dir))
}
